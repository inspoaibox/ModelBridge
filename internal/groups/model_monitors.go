package groups

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"ai-token/internal/ids"
)

func (s *SQLService) ListAdminModelMonitors(ctx context.Context) ([]ModelMonitor, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT mmc.id::text, mmc.group_id::text, rg.code, rg.name, mmc.name,
		       mmc.selection_mode, mmc.mode, mmc.probe_interval_seconds,
		       mmc.recent_request_limit,
		       mmc.enabled, mmc.last_probe_started_at, mmc.last_probe_finished_at,
		       mmc.last_probe_status, mmc.last_probe_error,
		       mmc.created_at, mmc.updated_at,
		       COALESCE((
		           SELECT jsonb_agg(DISTINCT m.model_name ORDER BY m.model_name)
		           FROM model_monitor_config_models cmm
		           JOIN models m ON m.id = cmm.model_id AND m.status = 'active'
		           WHERE cmm.config_id = mmc.id
		       ), '[]'::jsonb),
		       COALESCE((
		           SELECT jsonb_agg(DISTINCT m.model_name ORDER BY m.model_name)
		           FROM routing_group_channels rgc
		           JOIN channels c ON c.id = rgc.channel_id
		             AND c.deleted_at IS NULL
		           JOIN channel_models cm ON cm.channel_id = c.id
		             AND cm.enabled = true
		           JOIN models m ON m.id = cm.model_id AND m.status = 'active'
		           WHERE rgc.group_id = mmc.group_id
		       ), '[]'::jsonb)
		FROM model_monitor_configs mmc
		JOIN routing_groups rg ON rg.id = mmc.group_id AND rg.deleted_at IS NULL
		ORDER BY mmc.updated_at DESC, mmc.id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]ModelMonitor, 0)
	for rows.Next() {
		var (
			item                        ModelMonitor
			lastStarted, lastFinished   sql.NullTime
			modelNamesRaw, availableRaw []byte
		)
		if err := rows.Scan(
			&item.ID, &item.GroupID, &item.GroupCode, &item.GroupName, &item.Name,
			&item.SelectionMode, &item.Mode, &item.ProbeIntervalSeconds,
			&item.RecentRequestLimit,
			&item.Enabled, &lastStarted, &lastFinished, &item.LastProbeStatus,
			&item.LastProbeError, &item.CreatedAt, &item.UpdatedAt,
			&modelNamesRaw, &availableRaw,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(modelNamesRaw, &item.ModelNames); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(availableRaw, &item.AvailableModels); err != nil {
			return nil, err
		}
		if item.SelectionMode == MonitorSelectionAll {
			item.ModelNames = append([]string(nil), item.AvailableModels...)
		}
		if lastStarted.Valid {
			value := lastStarted.Time.UTC()
			item.LastProbeStartedAt = &value
		}
		if lastFinished.Valid {
			value := lastFinished.Time.UTC()
			item.LastProbeFinishedAt = &value
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *SQLService) CreateAdminModelMonitor(ctx context.Context, actorID string, request ModelMonitorMutation) (ModelMonitor, error) {
	return s.saveAdminModelMonitor(ctx, actorID, "", request)
}

func (s *SQLService) UpdateAdminModelMonitor(ctx context.Context, actorID, monitorID string, request ModelMonitorMutation) (ModelMonitor, error) {
	return s.saveAdminModelMonitor(ctx, actorID, strings.TrimSpace(monitorID), request)
}

func (s *SQLService) saveAdminModelMonitor(ctx context.Context, actorID, monitorID string, request ModelMonitorMutation) (ModelMonitor, error) {
	request, err := request.validate()
	if err != nil {
		return ModelMonitor{}, err
	}
	if s == nil || s.db == nil {
		return ModelMonitor{}, ErrUnavailable
	}
	if monitorID != "" && !ids.Valid(monitorID) {
		return ModelMonitor{}, ErrInvalidRequest
	}

	monitorIDValue := monitorID
	if monitorIDValue == "" {
		monitorIDValue, err = ids.New()
		if err != nil {
			return ModelMonitor{}, err
		}
	}
	sameGroup := false
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ModelMonitor{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var groupStatus string
	if err := tx.QueryRowContext(ctx, `
		SELECT status
		FROM routing_groups
		WHERE id = $1::uuid AND deleted_at IS NULL
		FOR UPDATE
	`, request.GroupID).Scan(&groupStatus); errors.Is(err, sql.ErrNoRows) {
		return ModelMonitor{}, ErrGroupNotFound
	} else if err != nil {
		return ModelMonitor{}, err
	}
	if request.Mode == MonitorModeActive && groupStatus != StatusActive {
		return ModelMonitor{}, ErrMonitorGroupInactive
	}

	if monitorID == "" {
		var exists bool
		if err := tx.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM model_monitor_configs
				WHERE group_id = $1::uuid
			)
		`, request.GroupID).Scan(&exists); err != nil {
			return ModelMonitor{}, err
		}
		if exists {
			return ModelMonitor{}, ErrMonitorInUse
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO model_monitor_configs (
				id, group_id, name, selection_mode, mode, probe_interval_seconds,
				recent_request_limit, enabled, created_by, updated_by
			) VALUES (
				$1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8,
				NULLIF($9, '')::uuid, NULLIF($9, '')::uuid
			)
		`, monitorIDValue, request.GroupID, request.Name, request.SelectionMode,
			request.Mode, request.ProbeIntervalSeconds, request.RecentRequestLimit, request.Enabled, actorID); err != nil {
			return ModelMonitor{}, err
		}
	} else {
		var currentGroupID string
		if err := tx.QueryRowContext(ctx, `
			SELECT group_id::text
			FROM model_monitor_configs
			WHERE id = $1::uuid
			FOR UPDATE
		`, monitorID).Scan(&currentGroupID); errors.Is(err, sql.ErrNoRows) {
			return ModelMonitor{}, ErrMonitorNotFound
		} else if err != nil {
			return ModelMonitor{}, err
		}
		sameGroup = currentGroupID == request.GroupID
		if currentGroupID != request.GroupID {
			var exists bool
			if err := tx.QueryRowContext(ctx, `
				SELECT EXISTS (
					SELECT 1 FROM model_monitor_configs
					WHERE group_id = $1::uuid AND id <> $2::uuid
				)
			`, request.GroupID, monitorID).Scan(&exists); err != nil {
				return ModelMonitor{}, err
			}
			if exists {
				return ModelMonitor{}, ErrMonitorInUse
			}
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE model_monitor_configs
			SET group_id = $2::uuid,
			    name = $3,
			    selection_mode = $4,
			    mode = $5,
			    probe_interval_seconds = $6,
			    recent_request_limit = $7,
			    enabled = $8,
			    probe_started_at = NULL,
			    last_probe_started_at = NULL,
			    last_probe_finished_at = NULL,
			    last_probe_status = '',
			    last_probe_error = '',
			    updated_by = NULLIF($9, '')::uuid,
			    updated_at = now()
			WHERE id = $1::uuid
		`, monitorID, request.GroupID, request.Name, request.SelectionMode,
			request.Mode, request.ProbeIntervalSeconds, request.RecentRequestLimit, request.Enabled, actorID); err != nil {
			return ModelMonitor{}, err
		}
	}

	if request.SelectionMode == MonitorSelectionSelected {
		for _, modelName := range request.ModelNames {
			var found bool
			err := tx.QueryRowContext(ctx, `
				WITH candidates AS (
					SELECT DISTINCT cm.model_id
					FROM routing_group_channels rgc
					JOIN channels c ON c.id = rgc.channel_id AND c.deleted_at IS NULL
					JOIN channel_models cm ON cm.channel_id = c.id
					JOIN models m ON m.id = cm.model_id AND m.status = 'active'
					WHERE rgc.group_id = $2::uuid AND m.model_name = $3

					UNION

					SELECT cmm.model_id
					FROM model_monitor_config_models cmm
					JOIN models m ON m.id = cmm.model_id AND m.status = 'active'
					WHERE cmm.config_id = $1::uuid
					  AND $4::boolean
					  AND m.model_name = $3
				), inserted AS (
					INSERT INTO model_monitor_config_models (config_id, model_id)
					SELECT $1::uuid, model_id FROM candidates
					ON CONFLICT DO NOTHING
				)
				SELECT EXISTS (SELECT 1 FROM candidates)
			`, monitorIDValue, request.GroupID, modelName, sameGroup).Scan(&found)
			if err != nil {
				return ModelMonitor{}, err
			}
			if !found {
				return ModelMonitor{}, ErrInvalidRequest
			}
		}
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM model_monitor_config_models cmm
			USING models m
			WHERE cmm.config_id = $1::uuid
			  AND cmm.model_id = m.id
			  AND NOT (m.model_name = ANY($2::text[]))
		`, monitorIDValue, request.ModelNames); err != nil {
			return ModelMonitor{}, err
		}
	} else if _, err := tx.ExecContext(ctx, `
		DELETE FROM model_monitor_config_models WHERE config_id = $1::uuid
	`, monitorIDValue); err != nil {
		return ModelMonitor{}, err
	}
	if err := tx.Commit(); err != nil {
		return ModelMonitor{}, err
	}
	return s.getAdminModelMonitor(ctx, monitorIDValue)
}

func (s *SQLService) DeleteAdminModelMonitor(ctx context.Context, actorID, monitorID string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	monitorID = strings.TrimSpace(monitorID)
	if monitorID == "" || !ids.Valid(monitorID) {
		return ErrInvalidRequest
	}
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM model_monitor_configs
		WHERE id = $1::uuid
	`, monitorID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrMonitorNotFound
	}
	_ = actorID
	return nil
}

func (s *SQLService) ClaimDueActiveModelMonitor(ctx context.Context) (*ModelMonitor, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var monitorID string
	err = tx.QueryRowContext(ctx, `
		SELECT mmc.id::text
		FROM model_monitor_configs mmc
		JOIN routing_groups rg ON rg.id = mmc.group_id
		  AND rg.deleted_at IS NULL
		  AND rg.status = 'active'
		WHERE mmc.enabled = true
		  AND mmc.mode = 'active'
		  AND (
		      (mmc.probe_started_at IS NULL AND mmc.last_probe_finished_at IS NULL)
		      OR (mmc.probe_started_at IS NULL AND mmc.last_probe_finished_at +
		          make_interval(secs => mmc.probe_interval_seconds) <= now())
		      OR mmc.probe_started_at < now() - interval '10 minutes'
		  )
		ORDER BY COALESCE(mmc.last_probe_finished_at, '-infinity'::timestamptz), mmc.id
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`).Scan(&monitorID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE model_monitor_configs
		SET probe_started_at = now(),
		    last_probe_started_at = now(),
		    last_probe_status = '',
		    last_probe_error = '',
		    updated_at = now()
		WHERE id = $1::uuid
	`, monitorID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	item, err := s.getAdminModelMonitor(ctx, monitorID)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *SQLService) ClaimActiveModelMonitor(ctx context.Context, monitorID string) (*ModelMonitor, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	monitorID = strings.TrimSpace(monitorID)
	if monitorID == "" || !ids.Valid(monitorID) {
		return nil, ErrInvalidRequest
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var mode string
	var enabled bool
	var probeStartedAt sql.NullTime
	var groupStatus string
	err = tx.QueryRowContext(ctx, `
		SELECT mmc.mode, mmc.enabled, mmc.probe_started_at, rg.status
		FROM model_monitor_configs mmc
		JOIN routing_groups rg ON rg.id = mmc.group_id
		  AND rg.deleted_at IS NULL
		WHERE mmc.id = $1::uuid
		FOR UPDATE
	`, monitorID).Scan(&mode, &enabled, &probeStartedAt, &groupStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMonitorNotFound
	}
	if err != nil {
		return nil, err
	}
	if mode != MonitorModeActive || !enabled {
		return nil, ErrMonitorModeInvalid
	}
	if groupStatus != StatusActive {
		return nil, ErrMonitorGroupInactive
	}
	if probeStartedAt.Valid {
		return nil, ErrMonitorBusy
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE model_monitor_configs
		SET probe_started_at = now(),
		    last_probe_started_at = now(),
		    last_probe_status = '',
		    last_probe_error = '',
		    updated_at = now()
		WHERE id = $1::uuid
	`, monitorID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	item, err := s.getAdminModelMonitor(ctx, monitorID)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *SQLService) CompleteActiveModelMonitor(ctx context.Context, monitorID, status, probeError string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	monitorID = strings.TrimSpace(monitorID)
	status = strings.ToLower(strings.TrimSpace(status))
	if monitorID == "" || !ids.Valid(monitorID) ||
		(status != MonitorProbeSuccess && status != MonitorProbeFailed && status != MonitorProbeSkipped) {
		return ErrInvalidRequest
	}
	if len(probeError) > 1000 {
		probeError = probeError[:1000]
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE model_monitor_configs
		SET probe_started_at = NULL,
		    last_probe_finished_at = now(),
		    last_probe_status = $2,
		    last_probe_error = $3,
		    updated_at = now()
		WHERE id = $1::uuid
	`, monitorID, status, strings.TrimSpace(probeError))
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrMonitorNotFound
	}
	return nil
}

func (s *SQLService) getAdminModelMonitor(ctx context.Context, monitorID string) (ModelMonitor, error) {
	items, err := s.ListAdminModelMonitors(ctx)
	if err != nil {
		return ModelMonitor{}, err
	}
	for _, item := range items {
		if item.ID == monitorID {
			return item, nil
		}
	}
	return ModelMonitor{}, ErrMonitorNotFound
}
