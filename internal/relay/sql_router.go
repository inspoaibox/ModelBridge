package relay

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ai-token/internal/ids"
)

type SQLChannelRouter struct {
	db  *sql.DB
	box SecretBox
}

func NewSQLChannelRouter(db *sql.DB, boxes ...SecretBox) (*SQLChannelRouter, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	var box SecretBox
	if len(boxes) > 0 {
		box = boxes[0]
	}
	return &SQLChannelRouter{db: db, box: box}, nil
}

func (r *SQLChannelRouter) Select(ctx context.Context, model string) (Channel, error) {
	candidates, err := r.SelectCandidates(ctx, model)
	if err != nil {
		return Channel{}, err
	}
	if len(candidates) == 0 {
		return Channel{}, ErrModelNotFound
	}
	return candidates[0], nil
}

func (r *SQLChannelRouter) SelectCandidates(ctx context.Context, model string) ([]Channel, error) {
	return r.selectCandidates(ctx, model, "")
}

func (r *SQLChannelRouter) SelectCandidatesForGroup(ctx context.Context, model, groupID string) ([]Channel, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" || !ids.Valid(groupID) {
		return nil, ErrInvalidRequest
	}
	return r.selectCandidates(ctx, model, groupID)
}

func (r *SQLChannelRouter) ResolveGroupPolicy(ctx context.Context, groupID string) (GroupPolicy, error) {
	if r == nil || r.db == nil {
		return GroupPolicy{}, ErrUnavailable
	}
	groupID = strings.TrimSpace(groupID)
	if groupID == "" || !ids.Valid(groupID) {
		return GroupPolicy{}, ErrInvalidRequest
	}
	var policy GroupPolicy
	err := r.db.QueryRowContext(ctx, `
		SELECT id::text, status, multiplier::text, rpm_limit, billing_type
		FROM routing_groups
		WHERE id = $1::uuid AND deleted_at IS NULL
	`, groupID).Scan(
		&policy.ID,
		&policy.Status,
		&policy.Multiplier,
		&policy.RPMLimit,
		&policy.BillingType,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return GroupPolicy{}, ErrGroupUnavailable
	}
	if err != nil {
		return GroupPolicy{}, err
	}
	return policy, nil
}

func (r *SQLChannelRouter) ConsumeGroupRPM(ctx context.Context, groupID string, limit int) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrUnavailable
	}
	groupID = strings.TrimSpace(groupID)
	if groupID == "" || !ids.Valid(groupID) || limit <= 0 {
		return false, ErrInvalidRequest
	}
	if _, err := r.db.ExecContext(ctx, `
		DELETE FROM routing_group_rpm_windows
		WHERE group_id = $1::uuid
		  AND window_start < date_trunc('minute', now()) - interval '2 minutes'
	`, groupID); err != nil {
		return false, err
	}
	var allowed bool
	err := r.db.QueryRowContext(ctx, `
		WITH consumed AS (
			INSERT INTO routing_group_rpm_windows (group_id, window_start, request_count)
			VALUES ($1::uuid, date_trunc('minute', now()), 1)
			ON CONFLICT (group_id, window_start) DO UPDATE
			SET request_count = routing_group_rpm_windows.request_count + 1
			WHERE routing_group_rpm_windows.request_count < $2
			RETURNING 1
		)
		SELECT EXISTS (SELECT 1 FROM consumed)
	`, groupID, limit).Scan(&allowed)
	return allowed, err
}

const (
	channelFailureThreshold = 3
	channelCircuitDuration  = 5 * time.Minute
)

func (r *SQLChannelRouter) RecordChannelFailure(ctx context.Context, channelID string, statusCode int) error {
	if r == nil || r.db == nil {
		return ErrUnavailable
	}
	channelID = strings.TrimSpace(channelID)
	if channelID == "" || !ids.Valid(channelID) || statusCode < 0 || statusCode > 599 {
		return ErrInvalidRequest
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE channels
		SET consecutive_failures = consecutive_failures + 1,
		    last_failure_status = NULLIF($2, 0),
		    last_failure_at = now(),
		    auto_disabled_until = CASE
		        WHEN consecutive_failures + 1 >= $3
		        THEN now() + $4::interval
		        ELSE auto_disabled_until
		    END,
		    updated_at = now()
		WHERE id = $1::uuid AND deleted_at IS NULL
	`, channelID, statusCode, channelFailureThreshold, channelCircuitDuration.String())
	return err
}

func (r *SQLChannelRouter) RecordChannelSuccess(ctx context.Context, channelID string) error {
	if r == nil || r.db == nil {
		return ErrUnavailable
	}
	channelID = strings.TrimSpace(channelID)
	if channelID == "" || !ids.Valid(channelID) {
		return ErrInvalidRequest
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE channels
		SET consecutive_failures = 0,
		    auto_disabled_until = NULL,
		    last_failure_status = NULL,
		    last_success_at = now(),
		    updated_at = now()
		WHERE id = $1::uuid AND deleted_at IS NULL
	`, channelID)
	return err
}

func (r *SQLChannelRouter) selectCandidates(ctx context.Context, model, groupID string) ([]Channel, error) {
	if r == nil || r.db == nil {
		return nil, ErrUnavailable
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, ErrInvalidRequest
	}
	query := `
		SELECT c.id::text, c.name, c.provider, c.base_url, c.credential_ref,
		       cm.upstream_model_name, c.priority, c.weight
		FROM models m
		JOIN channel_models cm ON cm.model_id = m.id
		JOIN channels c ON c.id = cm.channel_id
		WHERE m.model_name = $1
		  AND m.status = 'active'
		  AND cm.enabled = true
		  AND c.status = 'active'
		  AND (c.auto_disabled_until IS NULL OR c.auto_disabled_until <= now())
		  AND c.deleted_at IS NULL
	`
	args := []any{model}
	if groupID != "" {
		query += `
		  AND EXISTS (
		      SELECT 1
		      FROM routing_group_channels rgc
		      JOIN routing_groups rg ON rg.id = rgc.group_id
		      WHERE rgc.channel_id = c.id
		        AND rg.id = $2::uuid
		        AND rg.status = 'active'
		        AND rg.deleted_at IS NULL
		  )
		`
		args = append(args, groupID)
	}
	query += ` ORDER BY c.priority DESC, c.updated_at ASC, c.id ASC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	candidates := make([]Channel, 0)
	for rows.Next() {
		var channel Channel
		if err := rows.Scan(
			&channel.ID,
			&channel.Name,
			&channel.Provider,
			&channel.BaseURL,
			&channel.CredentialRef,
			&channel.UpstreamModelName,
			&channel.Priority,
			&channel.Weight,
		); err != nil {
			return nil, err
		}
		candidates = append(candidates, channel)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return candidates, nil
}

func (r *SQLChannelRouter) List(ctx context.Context) ([]ChannelSummary, error) {
	return r.list(ctx, "")
}

func (r *SQLChannelRouter) GetChannel(ctx context.Context, channelID string) (ChannelSummary, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID != "" && !ids.Valid(channelID) {
		return ChannelSummary{}, ErrInvalidRequest
	}
	channels, err := r.list(ctx, channelID)
	if err != nil {
		return ChannelSummary{}, err
	}
	if len(channels) == 0 {
		return ChannelSummary{}, ErrChannelNotFound
	}
	return channels[0], nil
}

func (r *SQLChannelRouter) CredentialRef(ctx context.Context, channelID string) (string, error) {
	if r == nil || r.db == nil {
		return "", ErrUnavailable
	}
	channelID = strings.TrimSpace(channelID)
	if channelID == "" || !ids.Valid(channelID) {
		return "", ErrInvalidRequest
	}

	var credentialRef string
	err := r.db.QueryRowContext(ctx, `
		SELECT credential_ref
		FROM channels
		WHERE id = $1
		  AND deleted_at IS NULL
	`, channelID).Scan(&credentialRef)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrChannelNotFound
	}
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(credentialRef) == "" {
		return "", ErrCredentialUnavailable
	}
	return credentialRef, nil
}

func (r *SQLChannelRouter) CreateChannel(
	ctx context.Context,
	actorID string,
	request ChannelMutation,
) (ChannelSummary, error) {
	request, err := request.validate(true)
	if err != nil {
		return ChannelSummary{}, err
	}
	if r == nil || r.db == nil || r.box == nil {
		return ChannelSummary{}, ErrUnavailable
	}
	channelID, err := ids.New()
	if err != nil {
		return ChannelSummary{}, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return ChannelSummary{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO channels (
			id, name, provider, base_url, credential_ref, status, priority, weight,
			created_by, updated_by
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8,
			NULLIF($9, '')::uuid, NULLIF($9, '')::uuid
		)
	`, channelID, request.Name, request.Provider, request.BaseURL, "pending:"+channelID,
		request.Status, request.Priority, request.Weight, actorID)
	if err != nil {
		return ChannelSummary{}, err
	}
	credentialRef, err := r.insertSecret(ctx, tx, channelID, actorID, request.APIKey)
	if err != nil {
		return ChannelSummary{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE channels
		SET credential_ref = $2,
		    updated_at = now()
		WHERE id = $1
	`, channelID, credentialRef); err != nil {
		return ChannelSummary{}, err
	}
	if err := r.replaceChannelModels(ctx, tx, channelID, request.Provider, request.Models); err != nil {
		return ChannelSummary{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO routing_group_channels (group_id, channel_id)
		SELECT id, $1
		FROM routing_groups
		WHERE code = 'default' AND deleted_at IS NULL
		ON CONFLICT DO NOTHING
	`, channelID); err != nil {
		return ChannelSummary{}, err
	}
	if err := tx.Commit(); err != nil {
		return ChannelSummary{}, err
	}
	return r.GetChannel(ctx, channelID)
}

func (r *SQLChannelRouter) UpdateChannel(
	ctx context.Context,
	actorID string,
	channelID string,
	request ChannelMutation,
) (ChannelSummary, error) {
	request, err := request.validate(false)
	if err != nil {
		return ChannelSummary{}, err
	}
	if r == nil || r.db == nil {
		return ChannelSummary{}, ErrUnavailable
	}
	channelID = strings.TrimSpace(channelID)
	if channelID == "" || !ids.Valid(channelID) {
		return ChannelSummary{}, ErrInvalidRequest
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return ChannelSummary{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	credentialRef, err := r.currentCredentialRef(ctx, tx, channelID)
	if err != nil {
		return ChannelSummary{}, err
	}
	if request.APIKey != "" {
		if r.box == nil {
			return ChannelSummary{}, ErrUnavailable
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE channel_secrets
			SET revoked_at = now()
			WHERE channel_id = $1
			  AND revoked_at IS NULL
		`, channelID); err != nil {
			return ChannelSummary{}, err
		}
		credentialRef, err = r.insertSecret(ctx, tx, channelID, actorID, request.APIKey)
		if err != nil {
			return ChannelSummary{}, err
		}
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE channels
		SET name = $2,
		    provider = $3,
		    base_url = $4,
		    credential_ref = $5,
		    status = $6,
		    consecutive_failures = CASE WHEN $6 = 'active' THEN 0 ELSE consecutive_failures END,
		    auto_disabled_until = CASE WHEN $6 = 'active' THEN NULL ELSE auto_disabled_until END,
		    last_failure_status = CASE WHEN $6 = 'active' THEN NULL ELSE last_failure_status END,
		    priority = $7,
		    weight = $8,
		    updated_by = NULLIF($9, '')::uuid,
		    updated_at = now()
		WHERE id = $1
		  AND deleted_at IS NULL
	`, channelID, request.Name, request.Provider, request.BaseURL, credentialRef,
		request.Status, request.Priority, request.Weight, actorID)
	if err != nil {
		return ChannelSummary{}, err
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		if err != nil {
			return ChannelSummary{}, err
		}
		return ChannelSummary{}, ErrChannelNotFound
	}
	if err := r.replaceChannelModels(ctx, tx, channelID, request.Provider, request.Models); err != nil {
		return ChannelSummary{}, err
	}
	if err := tx.Commit(); err != nil {
		return ChannelSummary{}, err
	}
	return r.GetChannel(ctx, channelID)
}

func (r *SQLChannelRouter) SetChannelStatus(
	ctx context.Context,
	actorID string,
	channelID string,
	status string,
) (ChannelSummary, error) {
	if r == nil || r.db == nil {
		return ChannelSummary{}, ErrUnavailable
	}
	channelID = strings.TrimSpace(channelID)
	status = normalizeChannelStatus(status)
	if channelID == "" || !ids.Valid(channelID) || !validChannelStatus(status) {
		return ChannelSummary{}, ErrInvalidRequest
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE channels
		SET status = $2,
		    consecutive_failures = CASE WHEN $2 = 'active' THEN 0 ELSE consecutive_failures END,
		    auto_disabled_until = CASE WHEN $2 = 'active' THEN NULL ELSE auto_disabled_until END,
		    last_failure_status = CASE WHEN $2 = 'active' THEN NULL ELSE last_failure_status END,
		    updated_by = NULLIF($3, '')::uuid,
		    updated_at = now()
		WHERE id = $1
		  AND deleted_at IS NULL
	`, channelID, status, actorID)
	if err != nil {
		return ChannelSummary{}, err
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		if err != nil {
			return ChannelSummary{}, err
		}
		return ChannelSummary{}, ErrChannelNotFound
	}
	return r.GetChannel(ctx, channelID)
}

func (r *SQLChannelRouter) DeleteChannel(ctx context.Context, actorID string, channelID string) error {
	if r == nil || r.db == nil {
		return ErrUnavailable
	}
	channelID = strings.TrimSpace(channelID)
	if channelID == "" || !ids.Valid(channelID) {
		return ErrInvalidRequest
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	result, err := tx.ExecContext(ctx, `
		UPDATE channels
		SET status = 'disabled',
		    deleted_at = now(),
		    updated_by = NULLIF($2, '')::uuid,
		    updated_at = now()
		WHERE id = $1
		  AND deleted_at IS NULL
	`, channelID, actorID)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		if err != nil {
			return err
		}
		return ErrChannelNotFound
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE channel_secrets
		SET revoked_at = now()
		WHERE channel_id = $1
		  AND revoked_at IS NULL
	`, channelID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *SQLChannelRouter) insertSecret(
	ctx context.Context,
	tx *sql.Tx,
	channelID string,
	actorID string,
	secret string,
) (string, error) {
	if r == nil || r.box == nil {
		return "", ErrUnavailable
	}
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return "", ErrCredentialRequired
	}
	secretID, err := ids.New()
	if err != nil {
		return "", err
	}
	encrypted, err := r.box.Seal([]byte(secret))
	if err != nil {
		return "", ErrCredentialUnavailable
	}
	prefix, suffix := secretPreviewParts(secret)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO channel_secrets (
			id, channel_id, secret_kind, encrypted_secret,
			secret_prefix, secret_suffix, created_by, rotated_at
		) VALUES (
			$1, $2, 'api_key', $3,
			$4, $5, NULLIF($6, '')::uuid, now()
		)
	`, secretID, channelID, []byte(encrypted), prefix, suffix, actorID)
	if err != nil {
		return "", err
	}
	return "secret:" + secretID, nil
}

func (r *SQLChannelRouter) currentCredentialRef(ctx context.Context, tx *sql.Tx, channelID string) (string, error) {
	var credentialRef string
	err := tx.QueryRowContext(ctx, `
		SELECT credential_ref
		FROM channels
		WHERE id = $1
		  AND deleted_at IS NULL
	`, channelID).Scan(&credentialRef)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrChannelNotFound
	}
	if err != nil {
		return "", err
	}
	return credentialRef, nil
}

func (r *SQLChannelRouter) replaceChannelModels(
	ctx context.Context,
	tx *sql.Tx,
	channelID string,
	provider string,
	models []ChannelModelMutation,
) error {
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM channel_models WHERE channel_id = $1
	`, channelID); err != nil {
		return err
	}
	for _, model := range models {
		capabilities := modelCapabilities(provider, model.Model)
		modelID, err := ids.New()
		if err != nil {
			return err
		}
		var storedModelID string
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO models (
				id, provider, model_name, protocol_family, capabilities_json, status
			) VALUES (
				$1, $2, $3, $4, $5::jsonb, 'active'
			)
			ON CONFLICT (provider, model_name) DO UPDATE SET
				protocol_family = EXCLUDED.protocol_family,
				capabilities_json = EXCLUDED.capabilities_json,
				status = 'active',
				updated_at = now()
			RETURNING id::text
		`, modelID, provider, model.Model, protocolFamily(provider), capabilities).Scan(&storedModelID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO channel_models (
				channel_id, model_id, upstream_model_name, enabled, health_status
			) VALUES (
				$1, $2, $3, $4, 'unknown'
			)
		`, channelID, storedModelID, model.UpstreamModel, channelModelEnabled(model)); err != nil {
			return err
		}
	}
	return nil
}

func modelCapabilities(provider, model string) string {
	name := strings.ToLower(strings.TrimSpace(model))
	if strings.Contains(name, "embedding") || strings.Contains(name, "embed") {
		return `{"modalities":["text","embedding"],"embedding":true,"official_sdk":true}`
	}
	if strings.Contains(name, "video") || strings.Contains(name, "veo") || strings.Contains(name, "sora") || strings.Contains(name, "kling") {
		return `{"modalities":["text","video"],"video_generation":true,"official_sdk":true}`
	}
	if strings.Contains(name, "image") || strings.Contains(name, "dall-e") || strings.Contains(name, "imagen") || strings.Contains(name, "flux") {
		return `{"modalities":["text","image"],"image_generation":true,"official_sdk":true}`
	}
	if strings.Contains(name, "audio") || strings.Contains(name, "whisper") || strings.Contains(name, "transcri") || strings.Contains(name, "speech") || strings.Contains(name, "tts") {
		return `{"modalities":["text","audio"],"audio":true,"official_sdk":true}`
	}
	switch canonicalProvider(provider) {
	case ProviderGrok:
		return `{"modalities":["text"],"openai_compatible":true,"official_sdk":false,"streaming":true,"tool_calling":true,"multimodal_input":true}`
	case ProviderGemini:
		return `{"modalities":["text"],"official_sdk":true,"streaming":true,"tool_calling":true,"multimodal_input":true}`
	case ProviderAnthropic:
		return `{"modalities":["text","image"],"official_sdk":true,"streaming":true,"tool_calling":true,"multimodal_input":true}`
	default:
		return `{"modalities":["text","image"],"official_sdk":true,"streaming":true,"tool_calling":true,"multimodal_input":true}`
	}
}

func (r *SQLChannelRouter) list(ctx context.Context, channelID string) ([]ChannelSummary, error) {
	if r == nil || r.db == nil {
		return nil, ErrUnavailable
	}
	where := "c.deleted_at IS NULL"
	args := []any{}
	if channelID != "" {
		where += fmt.Sprintf(" AND c.id = $%d", len(args)+1)
		args = append(args, channelID)
	}
	rows, err := r.db.QueryContext(ctx, channelListQuery(where), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	channels := []ChannelSummary{}
	for rows.Next() {
		var (
			channel           ChannelSummary
			modelsRaw         []byte
			createdAt         time.Time
			updatedAt         time.Time
			autoDisabledUntil sql.NullTime
			lastFailureStatus sql.NullInt64
		)
		if err := rows.Scan(
			&channel.ID,
			&channel.Name,
			&channel.Provider,
			&channel.BaseURL,
			&channel.CredentialRef,
			&channel.CredentialMode,
			&channel.CredentialPreview,
			&channel.HasCredential,
			&channel.Status,
			&channel.Priority,
			&channel.Weight,
			&channel.ConsecutiveFailures,
			&autoDisabledUntil,
			&lastFailureStatus,
			&createdAt,
			&updatedAt,
			&modelsRaw,
		); err != nil {
			return nil, err
		}
		if strings.HasPrefix(channel.CredentialRef, "secret:") {
			channel.CredentialRef = "secret:stored"
		}
		if err := json.Unmarshal(modelsRaw, &channel.Models); err != nil {
			return nil, err
		}
		channel.CreatedAt = createdAt
		channel.UpdatedAt = updatedAt
		if autoDisabledUntil.Valid {
			value := autoDisabledUntil.Time
			channel.AutoDisabledUntil = &value
		}
		if lastFailureStatus.Valid {
			value := int(lastFailureStatus.Int64)
			channel.LastFailureStatus = &value
		}
		channels = append(channels, channel)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return channels, nil
}

func channelListQuery(where string) string {
	return `
		SELECT c.id::text, c.name, c.provider, c.base_url, c.credential_ref,
		       CASE
		           WHEN c.credential_ref LIKE 'secret:%' THEN 'secret'
		           WHEN c.credential_ref LIKE 'env:%' THEN 'env'
		           ELSE 'external'
		       END AS credential_mode,
		       CASE
		           WHEN c.credential_ref LIKE 'secret:%' THEN COALESCE(NULLIF(cs.secret_prefix || '...' || cs.secret_suffix, '...'), 'stored')
		           WHEN c.credential_ref LIKE 'env:%' THEN c.credential_ref
		           ELSE 'configured'
		       END AS credential_preview,
		       CASE
		           WHEN c.credential_ref LIKE 'secret:%' THEN cs.id IS NOT NULL
		           ELSE c.credential_ref <> ''
		       END AS has_credential,
		       c.status, c.priority, c.weight, c.consecutive_failures,
		       c.auto_disabled_until, c.last_failure_status,
		       c.created_at, c.updated_at,
		       COALESCE(
		           jsonb_agg(
		               jsonb_build_object(
		                   'model', m.model_name,
		                   'provider', m.provider,
		                   'upstream_model', cm.upstream_model_name,
		                   'enabled', cm.enabled,
		                   'health_status', cm.health_status
		               )
		               ORDER BY m.provider, m.model_name
		           ) FILTER (WHERE m.id IS NOT NULL),
		           '[]'::jsonb
		       ) AS models_json
		FROM channels c
		LEFT JOIN channel_secrets cs
		       ON c.credential_ref = 'secret:' || cs.id::text
		      AND cs.revoked_at IS NULL
		LEFT JOIN channel_models cm ON cm.channel_id = c.id
		LEFT JOIN models m ON m.id = cm.model_id
		WHERE ` + where + `
		GROUP BY c.id, c.name, c.provider, c.base_url, c.credential_ref,
		         cs.id, cs.secret_prefix, cs.secret_suffix,
		         c.status, c.priority, c.weight, c.consecutive_failures,
		         c.auto_disabled_until, c.last_failure_status,
		         c.created_at, c.updated_at
		ORDER BY c.priority DESC, c.provider ASC, c.name ASC
	`
}

func protocolFamily(provider string) string {
	switch canonicalProvider(provider) {
	case ProviderOpenAI:
		return "openai_chat_completions"
	case ProviderAnthropic:
		return "anthropic_messages"
	case ProviderGrok:
		return "xai_chat_completions"
	case ProviderGemini:
		return "gemini_generate_content"
	default:
		return "unknown"
	}
}

func secretPreviewParts(secret string) (string, string) {
	secret = strings.TrimSpace(secret)
	if len(secret) <= 8 {
		return "", ""
	}
	prefixLength := 6
	if len(secret) < prefixLength {
		prefixLength = len(secret)
	}
	return secret[:prefixLength], secret[len(secret)-4:]
}
