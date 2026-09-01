package groups

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math/big"
	"strings"

	"ai-token/internal/ids"
)

func (s *SQLService) List(ctx context.Context) ([]Summary, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT rg.id::text, rg.code, rg.name, rg.description, rg.status,
		       rg.multiplier::text, rg.rpm_limit, rg.billing_type, rg.priority,
		       rg.created_at, rg.updated_at,
		       COALESCE(
		           jsonb_agg(
		               jsonb_build_object(
		                   'id', c.id::text,
		                   'name', c.name,
		                   'provider', c.provider,
		                   'status', c.status
		               ) ORDER BY c.name
		           ) FILTER (WHERE c.id IS NOT NULL),
		           '[]'::jsonb
		       ) AS channels_json,
		       COALESCE((
		           SELECT jsonb_agg(DISTINCT m.model_name ORDER BY m.model_name)
		           FROM routing_group_channels rgc2
		           JOIN channels c2 ON c2.id = rgc2.channel_id AND c2.status = 'active'
		             AND (c2.auto_disabled_until IS NULL OR c2.auto_disabled_until <= now()) AND c2.deleted_at IS NULL
		           JOIN channel_models cm2 ON cm2.channel_id = c2.id AND cm2.enabled = true
	             AND (cm2.auto_disabled_until IS NULL OR cm2.auto_disabled_until <= now())
		           JOIN models m ON m.id = cm2.model_id AND m.status = 'active'
		           WHERE rgc2.group_id = rg.id
		       ), '[]'::jsonb) AS models_json
		FROM routing_groups rg
		LEFT JOIN routing_group_channels rgc ON rgc.group_id = rg.id
		LEFT JOIN channels c ON c.id = rgc.channel_id AND c.deleted_at IS NULL
		WHERE rg.deleted_at IS NULL
		GROUP BY rg.id, rg.code, rg.name, rg.description, rg.status,
		         rg.multiplier, rg.rpm_limit, rg.billing_type, rg.priority,
		         rg.created_at, rg.updated_at
		ORDER BY rg.priority DESC, rg.code ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]Summary, 0)
	for rows.Next() {
		var (
			group       Summary
			channelsRaw []byte
			modelsRaw   []byte
		)
		if err := rows.Scan(
			&group.ID,
			&group.Code,
			&group.Name,
			&group.Description,
			&group.Status,
			&group.Multiplier,
			&group.RPMLimit,
			&group.BillingType,
			&group.Priority,
			&group.CreatedAt,
			&group.UpdatedAt,
			&channelsRaw,
			&modelsRaw,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(channelsRaw, &group.Channels); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(modelsRaw, &group.Models); err != nil {
			return nil, err
		}
		result = append(result, group)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *SQLService) ListTokenGroups(ctx context.Context) ([]TokenGroupSummary, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT rg.id::text, rg.code, rg.name, rg.multiplier::text, rg.billing_type, rg.status,
		       COALESCE((
		           SELECT jsonb_agg(DISTINCT m.model_name ORDER BY m.model_name)
		           FROM routing_group_channels rgc
		           JOIN channels c ON c.id = rgc.channel_id AND c.status = 'active'
		             AND (c.auto_disabled_until IS NULL OR c.auto_disabled_until <= now()) AND c.deleted_at IS NULL
	           JOIN channel_models cm ON cm.channel_id = c.id AND cm.enabled = true
	             AND (cm.auto_disabled_until IS NULL OR cm.auto_disabled_until <= now())
		           JOIN models m ON m.id = cm.model_id AND m.status = 'active'
		           WHERE rgc.group_id = rg.id
		       ), '[]'::jsonb)
		FROM routing_groups rg
		WHERE rg.status = 'active' AND rg.deleted_at IS NULL
		ORDER BY rg.priority DESC, rg.code ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]TokenGroupSummary, 0)
	for rows.Next() {
		var (
			group     TokenGroupSummary
			modelsRaw []byte
		)
		if err := rows.Scan(&group.ID, &group.Code, &group.Name, &group.Multiplier, &group.BillingType, &group.Status, &modelsRaw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(modelsRaw, &group.Models); err != nil {
			return nil, err
		}
		result = append(result, group)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *SQLService) Create(ctx context.Context, actorID string, request Mutation) (Summary, error) {
	request, err := request.validate()
	if err != nil {
		return Summary{}, err
	}
	if s == nil || s.db == nil {
		return Summary{}, ErrUnavailable
	}
	groupID, err := ids.New()
	if err != nil {
		return Summary{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Summary{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO routing_groups (
			id, code, name, description, status, multiplier, rpm_limit,
			billing_type, priority, created_by, updated_by
		) VALUES ($1, $2, $3, $4, $5, $6::numeric, $7, $8, $9,
		          NULLIF($10, '')::uuid, NULLIF($10, '')::uuid)
	`, groupID, request.Code, request.Name, request.Description, request.Status,
		request.Multiplier, request.RPMLimit, request.BillingType, request.Priority, actorID); err != nil {
		return Summary{}, err
	}
	if err := replaceChannels(ctx, tx, groupID, request.ChannelIDs); err != nil {
		return Summary{}, err
	}
	if err := tx.Commit(); err != nil {
		return Summary{}, err
	}
	return s.get(ctx, groupID)
}

func (s *SQLService) Update(ctx context.Context, actorID, groupID string, request Mutation) (Summary, error) {
	request, err := request.validate()
	if err != nil {
		return Summary{}, err
	}
	if s == nil || s.db == nil {
		return Summary{}, ErrUnavailable
	}
	groupID = strings.TrimSpace(groupID)
	if groupID == "" || !ids.Valid(groupID) {
		return Summary{}, ErrInvalidRequest
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Summary{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var currentCode string
	if err := tx.QueryRowContext(ctx, `
		SELECT code FROM routing_groups WHERE id = $1 AND deleted_at IS NULL FOR UPDATE
	`, groupID).Scan(&currentCode); errors.Is(err, sql.ErrNoRows) {
		return Summary{}, ErrGroupNotFound
	} else if err != nil {
		return Summary{}, err
	}
	if currentCode == "default" && (request.Code != "default" || request.Status != StatusActive) {
		return Summary{}, ErrDefaultGroupProtected
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE routing_groups
		SET code = $2,
		    name = $3,
		    description = $4,
		    status = $5,
		    multiplier = $6::numeric,
		    rpm_limit = $7,
		    billing_type = $8,
		    priority = $9,
		    updated_by = NULLIF($10, '')::uuid,
		    updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
	`, groupID, request.Code, request.Name, request.Description, request.Status,
		request.Multiplier, request.RPMLimit, request.BillingType, request.Priority, actorID); err != nil {
		return Summary{}, err
	}
	if err := replaceChannels(ctx, tx, groupID, request.ChannelIDs); err != nil {
		return Summary{}, err
	}
	if err := tx.Commit(); err != nil {
		return Summary{}, err
	}
	_ = currentCode
	return s.get(ctx, groupID)
}

func (s *SQLService) Delete(ctx context.Context, actorID, groupID string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	groupID = strings.TrimSpace(groupID)
	if groupID == "" || !ids.Valid(groupID) {
		return ErrInvalidRequest
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var code string
	err = tx.QueryRowContext(ctx, `
		SELECT code FROM routing_groups WHERE id = $1 AND deleted_at IS NULL FOR UPDATE
	`, groupID).Scan(&code)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrGroupNotFound
	}
	if err != nil {
		return err
	}
	if code == "default" {
		return ErrDefaultGroupProtected
	}
	var inUse bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM api_tokens
			WHERE group_id = $1
			  AND status = 'active'
			  AND (expires_at IS NULL OR expires_at > now())
		)
	`, groupID).Scan(&inUse); err != nil {
		return err
	}
	if inUse {
		return ErrGroupInUse
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE routing_groups
		SET status = 'disabled', deleted_at = now(), updated_by = NULLIF($2, '')::uuid,
		    updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
	`, groupID, actorID)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected != 1 {
		return ErrGroupNotFound
	}
	return tx.Commit()
}

func (s *SQLService) get(ctx context.Context, groupID string) (Summary, error) {
	groups, err := s.List(ctx)
	if err != nil {
		return Summary{}, err
	}
	for _, group := range groups {
		if group.ID == groupID {
			return group, nil
		}
	}
	return Summary{}, ErrGroupNotFound
}

func replaceChannels(ctx context.Context, tx *sql.Tx, groupID string, channelIDs []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM routing_group_channels WHERE group_id = $1`, groupID); err != nil {
		return err
	}
	for _, channelID := range channelIDs {
		if !ids.Valid(channelID) {
			return ErrChannelNotFound
		}
		result, err := tx.ExecContext(ctx, `
			INSERT INTO routing_group_channels (group_id, channel_id)
			SELECT $1, id FROM channels WHERE id = $2 AND deleted_at IS NULL
			ON CONFLICT DO NOTHING
		`, groupID, channelID)
		if err != nil {
			return err
		}
		if affected, err := result.RowsAffected(); err != nil {
			return err
		} else if affected != 1 {
			return ErrChannelNotFound
		}
	}
	return nil
}

func normalizeMultiplier(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && char != '.' {
			return "", false
		}
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 || (len(parts) == 2 && len(parts[1]) > 6) {
		return "", false
	}
	rat, ok := new(big.Rat).SetString(value)
	if !ok || rat.Sign() <= 0 || rat.Cmp(new(big.Rat).SetInt64(1000)) > 0 {
		return "", false
	}
	return rat.FloatString(6), true
}
