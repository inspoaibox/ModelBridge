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
		SELECT id::text, status, multiplier::text, rpm_limit, billing_type, metering_mode
		FROM routing_groups
		WHERE id = $1::uuid AND deleted_at IS NULL
	`, groupID).Scan(
		&policy.ID,
		&policy.Status,
		&policy.Multiplier,
		&policy.RPMLimit,
		&policy.BillingType,
		&policy.MeteringMode,
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

func (r *SQLChannelRouter) RecordChannelModelFailure(ctx context.Context, channelID, model string, statusCode int) error {
	if r == nil || r.db == nil {
		return ErrUnavailable
	}
	channelID = strings.TrimSpace(channelID)
	model = strings.TrimSpace(model)
	if channelID == "" || !ids.Valid(channelID) || model == "" || statusCode < 0 || statusCode > 599 {
		return ErrInvalidRequest
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE channel_models cm
		SET consecutive_failures = cm.consecutive_failures + 1,
		    last_failure_status = NULLIF($3, 0),
		    last_failure_at = now(),
		    health_status = CASE
		        WHEN cm.consecutive_failures + 1 >= $4 THEN 'unavailable'
		        ELSE 'degraded'
		    END,
		    auto_disabled_until = CASE
		        WHEN cm.consecutive_failures + 1 >= $4
		        THEN now() + $5::interval
		        ELSE cm.auto_disabled_until
		    END,
		    updated_at = now()
		FROM models m, channels c
		WHERE cm.channel_id = $1::uuid
		  AND cm.model_id = m.id
		  AND c.id = cm.channel_id
		  AND m.provider = c.provider
		  AND m.model_name = $2
	`, channelID, model, statusCode, channelFailureThreshold, channelCircuitDuration.String())
	return err
}

func (r *SQLChannelRouter) RecordChannelModelSuccess(ctx context.Context, channelID, model string) error {
	if r == nil || r.db == nil {
		return ErrUnavailable
	}
	channelID = strings.TrimSpace(channelID)
	model = strings.TrimSpace(model)
	if channelID == "" || !ids.Valid(channelID) || model == "" {
		return ErrInvalidRequest
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE channel_models cm
		SET consecutive_failures = 0,
		    auto_disabled_until = NULL,
		    last_failure_status = NULL,
		    last_success_at = now(),
		    health_status = 'normal',
		    updated_at = now()
		FROM models m, channels c
		WHERE cm.channel_id = $1::uuid
		  AND cm.model_id = m.id
		  AND c.id = cm.channel_id
		  AND m.provider = c.provider
		  AND m.model_name = $2
	`, channelID, model)
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
		       m.model_name, cm.upstream_model_name, c.upstream_cost_discount::text,
		       c.priority, c.weight
		FROM models m
		JOIN channel_models cm ON cm.model_id = m.id
		JOIN channels c ON c.id = cm.channel_id
		WHERE m.model_name = $1
		  AND m.status = 'active'
		  AND cm.enabled = true
		  AND c.status = 'active'
		  AND (c.auto_disabled_until IS NULL OR c.auto_disabled_until <= now())
		  AND (cm.auto_disabled_until IS NULL OR cm.auto_disabled_until <= now())
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
			&channel.ModelName,
			&channel.UpstreamModelName,
			&channel.UpstreamCostDiscount,
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

func (r *SQLChannelRouter) DiscoveryConfig(ctx context.Context, channelID string) (ChannelDiscoveryConfig, error) {
	if r == nil || r.db == nil {
		return ChannelDiscoveryConfig{}, ErrUnavailable
	}
	channelID = strings.TrimSpace(channelID)
	if channelID == "" || !ids.Valid(channelID) {
		return ChannelDiscoveryConfig{}, ErrInvalidRequest
	}
	var config ChannelDiscoveryConfig
	err := r.db.QueryRowContext(ctx, `
		SELECT provider, base_url, credential_ref
		FROM channels
		WHERE id = $1::uuid
		  AND deleted_at IS NULL
	`, channelID).Scan(&config.Provider, &config.BaseURL, &config.CredentialRef)
	if errors.Is(err, sql.ErrNoRows) {
		return ChannelDiscoveryConfig{}, ErrChannelNotFound
	}
	if err != nil {
		return ChannelDiscoveryConfig{}, err
	}
	return config, nil
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
			id, name, provider, base_url, credential_ref, status, upstream_cost_discount,
			upstream_integration, upstream_account_credential_ref, upstream_account_user_id, upstream_account_sync_status,
			priority, weight,
			created_by, updated_by
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7::numeric,
			$8, '', $9, $10,
			$11, $12,
			NULLIF($13, '')::uuid, NULLIF($13, '')::uuid
		)
	`, channelID, request.Name, request.Provider, request.BaseURL, "pending:"+channelID,
		request.Status, request.UpstreamCostDiscount, request.UpstreamIntegration,
		request.UpstreamAccountUserID,
		accountSyncStatus(request.UpstreamIntegration, request.UpstreamAccountCredential, request.UpstreamAccountUserID),
		request.Priority, request.Weight, actorID)
	if err != nil {
		return ChannelSummary{}, err
	}
	credentialRef, err := r.insertSecret(ctx, tx, channelID, actorID, request.APIKey)
	if err != nil {
		return ChannelSummary{}, err
	}
	accountCredentialRef := ""
	if request.UpstreamAccountCredential != "" {
		accountCredentialRef, err = r.insertAccountSecret(ctx, tx, channelID, actorID, request.UpstreamAccountCredential)
		if err != nil {
			return ChannelSummary{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE channels
		SET credential_ref = $2,
		    upstream_account_credential_ref = $3,
		    updated_at = now()
		WHERE id = $1
	`, channelID, credentialRef, accountCredentialRef); err != nil {
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
	currentAccountRef, currentIntegration, currentAccountUserID, currentAccountStatus, err := r.currentAccountConfig(ctx, tx, channelID)
	if err != nil {
		return ChannelSummary{}, err
	}
	accountCredentialRef := currentAccountRef
	accountUserID := request.UpstreamAccountUserID
	accountSyncStatus := currentAccountStatus
	if accountSyncStatus == "" {
		accountSyncStatus = "not_configured"
	}
	resetAccountSnapshot := currentIntegration != request.UpstreamIntegration || currentAccountUserID != accountUserID
	if request.ClearUpstreamAccountCredential {
		if _, err := tx.ExecContext(ctx, `
			UPDATE channel_account_secrets
			SET revoked_at = now()
			WHERE channel_id = $1
			  AND revoked_at IS NULL
		`, channelID); err != nil {
			return ChannelSummary{}, err
		}
		accountCredentialRef = ""
		accountSyncStatus = "not_configured"
		resetAccountSnapshot = true
	}
	if request.UpstreamAccountCredential != "" {
		if r.box == nil {
			return ChannelSummary{}, ErrUnavailable
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE channel_account_secrets
			SET revoked_at = now()
			WHERE channel_id = $1
			  AND revoked_at IS NULL
		`, channelID); err != nil {
			return ChannelSummary{}, err
		}
		accountCredentialRef, err = r.insertAccountSecret(ctx, tx, channelID, actorID, request.UpstreamAccountCredential)
		if err != nil {
			return ChannelSummary{}, err
		}
		accountSyncStatus = "pending"
		resetAccountSnapshot = true
	}
	if request.UpstreamAccountCredential == "" && !request.ClearUpstreamAccountCredential {
		if resetAccountSnapshot && accountCredentialRef != "" {
			accountSyncStatus = "pending"
		}
		if currentAccountRef == "" {
			accountSyncStatus = "not_configured"
		}
		if resetAccountSnapshot && accountCredentialRef == "" {
			accountSyncStatus = "not_configured"
		}
	}
	if request.UpstreamIntegration != UpstreamIntegrationNewAPI {
		accountUserID = ""
		resetAccountSnapshot = resetAccountSnapshot || currentAccountUserID != ""
	}
	if accountCredentialRef == "" {
		accountSyncStatus = "not_configured"
		resetAccountSnapshot = true
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
		    upstream_cost_discount = $7::numeric,
		    upstream_integration = $8,
		    upstream_account_credential_ref = $9,
		    upstream_account_user_id = $10,
		    upstream_account_sync_status = $11,
		    upstream_account_sync_error = CASE WHEN $11 IN ('pending', 'not_configured') THEN '' ELSE upstream_account_sync_error END,
		    upstream_balance = CASE WHEN $12::boolean THEN NULL ELSE upstream_balance END,
		    upstream_balance_unit = CASE WHEN $12::boolean THEN '' ELSE upstream_balance_unit END,
		    upstream_balance_total = CASE WHEN $12::boolean THEN NULL ELSE upstream_balance_total END,
		    upstream_balance_used = CASE WHEN $12::boolean THEN NULL ELSE upstream_balance_used END,
		    upstream_account_plan_name = CASE WHEN $12::boolean THEN '' ELSE upstream_account_plan_name END,
		    upstream_rate_multiplier = CASE WHEN $12::boolean THEN NULL ELSE upstream_rate_multiplier END,
		    upstream_account_synced_at = CASE WHEN $12::boolean THEN NULL ELSE upstream_account_synced_at END,
		    consecutive_failures = CASE WHEN $6 = 'active' THEN 0 ELSE consecutive_failures END,
		    auto_disabled_until = CASE WHEN $6 = 'active' THEN NULL ELSE auto_disabled_until END,
		    last_failure_status = CASE WHEN $6 = 'active' THEN NULL ELSE last_failure_status END,
		    last_success_at = CASE WHEN $6 = 'active' THEN NULL ELSE last_success_at END,
		    priority = $13,
		    weight = $14,
		    updated_by = NULLIF($15, '')::uuid,
		    updated_at = now()
		WHERE id = $1
		  AND deleted_at IS NULL
	`, channelID, request.Name, request.Provider, request.BaseURL, credentialRef,
		request.Status, request.UpstreamCostDiscount, request.UpstreamIntegration, accountCredentialRef,
		accountUserID, accountSyncStatus, resetAccountSnapshot, request.Priority, request.Weight, actorID)
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
		    last_success_at = CASE WHEN $2 = 'active' THEN NULL ELSE last_success_at END,
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
	if status == "active" {
		if _, err := r.db.ExecContext(ctx, `
		UPDATE channel_models
		SET consecutive_failures = 0,
		    auto_disabled_until = NULL,
		    last_failure_status = NULL,
		    last_failure_at = NULL,
		    last_success_at = NULL,
		    health_status = 'unknown',
		    updated_at = now()
			WHERE channel_id = $1::uuid
		`, channelID); err != nil {
			return ChannelSummary{}, err
		}
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
	if _, err := tx.ExecContext(ctx, `
		UPDATE channel_account_secrets
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

func (r *SQLChannelRouter) insertAccountSecret(
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
		INSERT INTO channel_account_secrets (
			id, channel_id, encrypted_secret, secret_prefix, secret_suffix,
			created_by, rotated_at
		) VALUES (
			$1, $2, $3, $4, $5, NULLIF($6, '')::uuid, now()
		)
	`, secretID, channelID, []byte(encrypted), prefix, suffix, actorID)
	if err != nil {
		return "", err
	}
	return "account-secret:" + secretID, nil
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

func (r *SQLChannelRouter) currentAccountConfig(ctx context.Context, tx *sql.Tx, channelID string) (string, string, string, string, error) {
	var credentialRef, integration, userID, status string
	err := tx.QueryRowContext(ctx, `
		SELECT upstream_account_credential_ref, upstream_integration,
		       upstream_account_user_id, upstream_account_sync_status
		FROM channels
		WHERE id = $1
		  AND deleted_at IS NULL
	`, channelID).Scan(&credentialRef, &integration, &userID, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", "", ErrChannelNotFound
	}
	if err != nil {
		return "", "", "", "", err
	}
	return credentialRef, integration, userID, status, nil
}

func accountSyncStatus(integration, credential, userID string) string {
	if strings.TrimSpace(credential) == "" {
		return "not_configured"
	}
	return "pending"
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
		capabilities := modelCapabilities(provider, firstNonEmpty(model.UpstreamModel, model.Model))
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
	if canonicalProvider(provider) == ProviderVolcengine && strings.Contains(name, "seedance") {
		spec, err := seedanceModelSpecFor(model)
		if err == nil {
			capabilities := map[string]any{
				"modalities":                 []string{"video"},
				"input_modalities":           []string{"text", "image", "video", "audio"},
				"output_modalities":          []string{"video"},
				"video_generation":           true,
				"audio_input":                true,
				"generate_audio":             true,
				"web_search":                 true,
				"official_sdk":               true,
				"protocol":                   "ark_content_generation",
				"seedance_version":           spec.version,
				"default_duration_seconds":   spec.defaultDurationSeconds,
				"max_duration_seconds":       spec.maxDurationSeconds,
				"duration_supports_auto":     true,
				"max_reference_images":       spec.maxReferenceImages,
				"max_reference_videos":       spec.maxReferenceVideos,
				"max_reference_audios":       spec.maxReferenceAudios,
				"audio_only_reference":       spec.audioOnlyReference,
				"reference_image_roles":      []string{"reference_image", "first_frame", "last_frame"},
				"reference_video_role":       "reference_video",
				"reference_audio_role":       "reference_audio",
				"supports_output_format":     spec.supportsOutputFormat,
				"supports_omni_task_type":    spec.supportsOmniTaskType,
				"supports_return_last_frame": spec.supportsReturnLastFrame,
				"supports_4k":                false,
			}
			if _, ok := spec.resolutions["4k"]; ok {
				capabilities["supports_4k"] = true
			}
			data, marshalErr := json.Marshal(capabilities)
			if marshalErr == nil {
				return string(data)
			}
		}
	}
	if strings.Contains(name, "embedding") || strings.Contains(name, "embed") {
		return `{"modalities":["text","embedding"],"embedding":true,"official_sdk":true}`
	}
	if strings.Contains(name, "video") || strings.Contains(name, "veo") || strings.Contains(name, "sora") || strings.Contains(name, "kling") || strings.Contains(name, "seedance") {
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
	case ProviderVolcengine:
		return `{"modalities":["text","video"],"video_generation":true,"official_sdk":true,"protocol":"ark_content_generation"}`
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
			channel              ChannelSummary
			modelsRaw            []byte
			createdAt            time.Time
			updatedAt            time.Time
			autoDisabledUntil    sql.NullTime
			lastFailureStatus    sql.NullInt64
			upstreamBalance      sql.NullString
			upstreamBalanceTotal sql.NullString
			upstreamBalanceUsed  sql.NullString
			upstreamRate         sql.NullString
			upstreamSyncedAt     sql.NullTime
			upstreamAttemptAt    sql.NullTime
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
			&channel.UpstreamCostDiscount,
			&channel.UpstreamIntegration,
			&channel.UpstreamAccountUserID,
			&channel.HasUpstreamAccountCredential,
			&upstreamBalance,
			&channel.UpstreamBalanceUnit,
			&upstreamBalanceTotal,
			&upstreamBalanceUsed,
			&channel.UpstreamAccountPlanName,
			&upstreamRate,
			&channel.UpstreamAccountSyncStatus,
			&channel.UpstreamAccountSyncError,
			&upstreamSyncedAt,
			&upstreamAttemptAt,
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
		if upstreamBalance.Valid {
			value := upstreamBalance.String
			channel.UpstreamBalance = &value
		}
		if upstreamBalanceTotal.Valid {
			value := upstreamBalanceTotal.String
			channel.UpstreamBalanceTotal = &value
		}
		if upstreamBalanceUsed.Valid {
			value := upstreamBalanceUsed.String
			channel.UpstreamBalanceUsed = &value
		}
		if upstreamRate.Valid {
			value := upstreamRate.String
			channel.UpstreamRateMultiplier = &value
		}
		if upstreamSyncedAt.Valid {
			value := upstreamSyncedAt.Time
			channel.UpstreamAccountSyncedAt = &value
		}
		if upstreamAttemptAt.Valid {
			value := upstreamAttemptAt.Time
			channel.UpstreamAccountLastAttemptAt = &value
		}
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
		       c.status, c.upstream_cost_discount::text,
		       c.upstream_integration,
		       c.upstream_account_user_id,
		       CASE
		           WHEN c.upstream_account_credential_ref = '' THEN false
		           ELSE cas.id IS NOT NULL
		       END AS has_upstream_account_credential,
		       c.upstream_balance::text,
		       c.upstream_balance_unit,
		       c.upstream_balance_total::text,
		       c.upstream_balance_used::text,
		       c.upstream_account_plan_name,
		       c.upstream_rate_multiplier::text,
		       c.upstream_account_sync_status,
		       c.upstream_account_sync_error,
		       c.upstream_account_synced_at,
		       c.upstream_account_last_attempt_at,
		       c.priority, c.weight, c.consecutive_failures,
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
		LEFT JOIN channel_account_secrets cas
		       ON c.upstream_account_credential_ref = 'account-secret:' || cas.id::text
		      AND cas.revoked_at IS NULL
		LEFT JOIN channel_models cm ON cm.channel_id = c.id
		LEFT JOIN models m ON m.id = cm.model_id
		WHERE ` + where + `
		GROUP BY c.id, c.name, c.provider, c.base_url, c.credential_ref,
		         cs.id, cs.secret_prefix, cs.secret_suffix,
		         cas.id,
		         c.status, c.upstream_cost_discount, c.priority, c.weight, c.consecutive_failures,
		         c.upstream_integration, c.upstream_account_credential_ref,
		         c.upstream_account_user_id,
		         c.upstream_balance, c.upstream_balance_unit,
		         c.upstream_balance_total, c.upstream_balance_used,
		         c.upstream_account_plan_name, c.upstream_rate_multiplier,
		         c.upstream_account_sync_status, c.upstream_account_sync_error,
		         c.upstream_account_synced_at, c.upstream_account_last_attempt_at,
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
	case ProviderVolcengine:
		return "volcengine_content_generation"
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
