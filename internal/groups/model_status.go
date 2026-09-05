package groups

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"ai-token/internal/ids"
)

// ModelStatus is the tenant-safe health summary for one model route. It is
// derived from channel routing state; upstream credentials and endpoints are
// intentionally excluded.
type ModelStatus struct {
	Model                string     `json:"model"`
	Provider             string     `json:"provider"`
	Status               string     `json:"status"`
	TotalRoutes          int        `json:"total_routes"`
	AvailableRoutes      int        `json:"available_routes"`
	ObservedRoutes       int        `json:"observed_routes"`
	ConsecutiveFailures  int        `json:"consecutive_failures"`
	LastSuccessAt        *time.Time `json:"last_success_at,omitempty"`
	LastFailureAt        *time.Time `json:"last_failure_at,omitempty"`
	LastLatencyMS        int64      `json:"last_latency_ms"`
	AvailabilityRealtime float64    `json:"availability_realtime"`
	Availability24h      float64    `json:"availability_24h"`
	RequestCount24h      int        `json:"request_count_24h"`
	Availability7d       float64    `json:"availability_7d"`
	RequestCount7d       int        `json:"request_count_7d"`
	RecentStatuses       []string   `json:"recent_statuses,omitempty"`
	LastRequestAt        *time.Time `json:"last_request_at,omitempty"`
	LastRequestStatus    string     `json:"last_request_status,omitempty"`
	LastFailureReason    string     `json:"last_failure_reason,omitempty"`
}

type ModelStatusGroup struct {
	ID                   string        `json:"group_id"`
	Code                 string        `json:"group_code"`
	Name                 string        `json:"group_name"`
	Status               string        `json:"status"`
	GroupStatus          string        `json:"group_status"`
	Multiplier           string        `json:"multiplier"`
	RPMLimit             int           `json:"rpm_limit"`
	BillingType          string        `json:"billing_type"`
	MeteringMode         string        `json:"metering_mode"`
	MonitorID            string        `json:"monitor_id,omitempty"`
	MonitorName          string        `json:"monitor_name,omitempty"`
	MonitorMode          string        `json:"monitor_mode,omitempty"`
	SelectionMode        string        `json:"selection_mode,omitempty"`
	PrimaryModel         string        `json:"primary_model,omitempty"`
	ProbeIntervalSeconds int           `json:"probe_interval_seconds,omitempty"`
	RecentRequestLimit   int           `json:"recent_request_limit"`
	LastProbeStartedAt   *time.Time    `json:"last_probe_started_at,omitempty"`
	LastProbeFinishedAt  *time.Time    `json:"last_probe_finished_at,omitempty"`
	LastProbeStatus      string        `json:"last_probe_status,omitempty"`
	LastProbeError       string        `json:"last_probe_error,omitempty"`
	Models               []ModelStatus `json:"models"`
	UpdatedAt            time.Time     `json:"updated_at"`
}

type ModelStatusReport struct {
	UpdatedAt time.Time          `json:"updated_at"`
	Groups    []ModelStatusGroup `json:"groups"`
}

type ModelStatusLister interface {
	ListModelStatuses(context.Context, string) (ModelStatusReport, error)
}

// AdminModelStatusLister exposes the same routing view without tenant
// filtering. It is intended for the operations console only; credentials and
// upstream URLs are never included in the report.
type AdminModelStatusLister interface {
	ListAdminModelStatuses(context.Context) (ModelStatusReport, error)
}

// ListModelStatuses returns the current routing view for groups visible to a
// tenant and covered by an enabled admin monitor. "all" monitor scopes follow
// the group's current enabled model mappings; "selected" scopes keep their
// configured models even when every current route is unavailable. Active
// groups are available to choose; a disabled group remains visible only when
// one of the tenant's live tokens is bound to it. A model is normal when every
// configured route is currently eligible for selection, degraded when only
// some are eligible, and unavailable when no route can currently be selected.
// Active monitor probe health is included in this status; passive monitors
// rely on real customer traffic observations.
// A disabled group is reported separately so the UI can distinguish
// configuration state from route health.
func (s *SQLService) ListModelStatuses(ctx context.Context, tenantID string) (ModelStatusReport, error) {
	if s == nil || s.db == nil {
		return ModelStatusReport{}, ErrUnavailable
	}
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" || !ids.Valid(tenantID) {
		return ModelStatusReport{}, ErrInvalidRequest
	}

	rows, err := s.db.QueryContext(ctx, `
		WITH monitor_models AS (
		    SELECT mmc.id AS monitor_id,
		           mmc.selection_mode,
		           mmc.recent_request_limit,
		           rg.id AS group_id,
		           rg.code,
		           rg.name,
		           rg.status,
		           rg.multiplier,
		           rg.rpm_limit,
		           rg.billing_type,
		           rg.metering_mode,
		           mmc.mode,
		           rg.priority,
		           rg.updated_at,
		           pm.model_name AS primary_model,
		           m.id AS model_id,
		           m.model_name,
		           m.provider,
		           m.status AS model_status
		    FROM model_monitor_configs mmc
		    JOIN routing_groups rg ON rg.id = mmc.group_id
		      AND rg.deleted_at IS NULL
		    LEFT JOIN models pm ON pm.id = mmc.primary_model_id
		    JOIN models m ON true
		    WHERE mmc.enabled = true
		      AND (rg.status = 'active' OR EXISTS (
		          SELECT 1
		          FROM api_tokens tok
		          JOIN projects tok_project ON tok_project.id = tok.project_id
		           AND tok_project.tenant_id = tok.tenant_id
		           AND tok_project.status = 'active'
		           AND tok_project.deleted_at IS NULL
		          JOIN users tok_user ON tok_user.id = tok.created_by
		           AND tok_user.status = 'active'
		           AND tok_user.deleted_at IS NULL
		          WHERE (tok.group_id = rg.id OR (tok.group_id IS NULL AND rg.code = 'default'))
		            AND tok.tenant_id = $1::uuid
		            AND tok.status = 'active'
		            AND (tok.expires_at IS NULL OR tok.expires_at > now())
		      ))
		      AND (
		          (mmc.selection_mode = 'all' AND m.status = 'active' AND EXISTS (
		              SELECT 1
		              FROM routing_group_channels rgc0
		              JOIN channels c0 ON c0.id = rgc0.channel_id
		                AND c0.deleted_at IS NULL
		              JOIN channel_models cm0 ON cm0.channel_id = c0.id
		                AND cm0.model_id = m.id
		                AND cm0.enabled = true
		              WHERE rgc0.group_id = mmc.group_id
		          ))
		          OR (mmc.selection_mode = 'selected' AND EXISTS (
		              SELECT 1
		              FROM model_monitor_config_models cmm0
		              WHERE cmm0.config_id = mmc.id
		                AND cmm0.model_id = m.id
		          ))
		      )
		), health_requests AS (
			SELECT id, group_id, model_id, status, latency_ms, created_at, failure_reason
			FROM model_requests
			UNION ALL
			SELECT id, group_id, model_id, status, latency_ms, created_at, failure_reason
			FROM model_probe_requests
		)
		SELECT mm.group_id::text, mm.code, mm.name, mm.status,
		       mm.multiplier::text, mm.rpm_limit, mm.billing_type, mm.metering_mode, mm.updated_at,
		       mm.recent_request_limit,
		       mm.primary_model,
		       mm.model_id::text, mm.model_name, mm.provider,
		       COUNT(cm.model_id)::int,
		       COUNT(cm.model_id) FILTER (WHERE mm.model_status = 'active'
		           AND cm.enabled = true
		           AND c.status = 'active'
		           AND (c.auto_disabled_until IS NULL OR c.auto_disabled_until <= now())
		           AND (cm.auto_disabled_until IS NULL OR cm.auto_disabled_until <= now()))::int,
		       COUNT(cm.model_id) FILTER (WHERE cm.health_status IN ('degraded', 'unavailable')
		           OR (mm.mode = 'active' AND cm.probe_health IN ('degraded', 'unavailable')))::int,
		       COUNT(cm.model_id) FILTER (WHERE cm.last_success_at IS NOT NULL
		           OR cm.last_failure_at IS NOT NULL
		           OR (mm.mode = 'active' AND (cm.probe_last_success_at IS NOT NULL OR cm.probe_last_failure_at IS NOT NULL)))::int,
		       GREATEST(COALESCE(MAX(cm.consecutive_failures), 0), COALESCE(MAX(cm.probe_consecutive_failures) FILTER (WHERE mm.mode = 'active'), 0))::int,
		       GREATEST(MAX(cm.last_success_at), MAX(cm.probe_last_success_at)),
		       GREATEST(MAX(cm.last_failure_at), MAX(cm.probe_last_failure_at)),
		       COALESCE((SELECT COUNT(*)::int
           FROM health_requests mr
		           WHERE mr.group_id = mm.group_id AND mr.model_id = mm.model_id
		             AND mr.status IN ('settled', 'settlement_pending', 'failed')
		             AND mr.created_at >= now() - interval '24 hours'), 0)::int,
		       COALESCE((SELECT COUNT(*)::int
           FROM health_requests mr
		           WHERE mr.group_id = mm.group_id AND mr.model_id = mm.model_id
		             AND mr.status IN ('settled', 'settlement_pending')
		             AND mr.created_at >= now() - interval '24 hours'), 0)::int,
		       COALESCE((SELECT COUNT(*)::int
           FROM health_requests mr
		           WHERE mr.group_id = mm.group_id AND mr.model_id = mm.model_id
		             AND mr.status IN ('settled', 'settlement_pending', 'failed')
		             AND mr.created_at >= now() - interval '7 days'), 0)::int,
		       COALESCE((SELECT COUNT(*)::int
           FROM health_requests mr
		           WHERE mr.group_id = mm.group_id AND mr.model_id = mm.model_id
		             AND mr.status IN ('settled', 'settlement_pending')
		             AND mr.created_at >= now() - interval '7 days'), 0)::int,
		       COALESCE((SELECT jsonb_agg(recent.status ORDER BY recent.created_at ASC, recent.id ASC)
		           FROM (
		               SELECT mr.id, mr.status, mr.created_at
           FROM health_requests mr
		               WHERE mr.group_id = mm.group_id AND mr.model_id = mm.model_id
		               ORDER BY mr.created_at DESC, mr.id DESC
		               LIMIT mm.recent_request_limit
		           ) recent), '[]'::jsonb),
		       (SELECT mr.latency_ms
               FROM health_requests mr
		           WHERE mr.group_id = mm.group_id AND mr.model_id = mm.model_id
		           ORDER BY mr.created_at DESC, mr.id DESC
		           LIMIT 1),
		       (SELECT mr.status
           FROM health_requests mr
		           WHERE mr.group_id = mm.group_id AND mr.model_id = mm.model_id
		           ORDER BY mr.created_at DESC, mr.id DESC
		           LIMIT 1),
		       (SELECT mr.created_at
           FROM health_requests mr
		           WHERE mr.group_id = mm.group_id AND mr.model_id = mm.model_id
		           ORDER BY mr.created_at DESC, mr.id DESC
		           LIMIT 1),
		       (SELECT COALESCE(mr.failure_reason, '')
           FROM health_requests mr
		           WHERE mr.group_id = mm.group_id AND mr.model_id = mm.model_id
		             AND mr.status = 'failed'
		           ORDER BY mr.created_at DESC, mr.id DESC
		           LIMIT 1)
		FROM monitor_models mm
		LEFT JOIN routing_group_channels rgc ON rgc.group_id = mm.group_id
		LEFT JOIN channels c ON c.id = rgc.channel_id AND c.deleted_at IS NULL
		LEFT JOIN channel_models cm ON cm.channel_id = c.id
		  AND cm.model_id = mm.model_id
		GROUP BY mm.group_id, mm.code, mm.name, mm.status, mm.multiplier,
		         mm.rpm_limit, mm.billing_type, mm.metering_mode, mm.priority,
		         mm.updated_at, mm.recent_request_limit, mm.primary_model, mm.model_id,
		         mm.model_name, mm.provider, mm.model_status
		ORDER BY mm.priority DESC, mm.code ASC, mm.provider ASC, mm.model_name ASC
		`, tenantID)
	if err != nil {
		return ModelStatusReport{}, err
	}
	defer rows.Close()

	type groupIndex struct {
		index int
	}
	groups := make([]ModelStatusGroup, 0)
	byID := make(map[string]groupIndex)
	for rows.Next() {
		var (
			group                                                                            ModelStatusGroup
			groupStatus                                                                      string
			modelID                                                                          sql.NullString
			modelName, provider                                                              sql.NullString
			totalRoutes, availableRoutes, degradedRoutes, observedRoutes                     int
			failures, requestCount24h, successfulCount24h, requestCount7d, successfulCount7d int
			lastSuccess, lastFailure, lastRequestAt                                          sql.NullTime
			lastLatency                                                                      sql.NullInt64
			lastRequestStatus, lastFailureReason                                             sql.NullString
			primaryModel                                                                     sql.NullString
			recentStatusesRaw                                                                []byte
		)
		if err := rows.Scan(
			&group.ID, &group.Code, &group.Name, &groupStatus,
			&group.Multiplier, &group.RPMLimit, &group.BillingType, &group.MeteringMode, &group.UpdatedAt,
			&group.RecentRequestLimit, &primaryModel, &modelID, &modelName, &provider,
			&totalRoutes, &availableRoutes, &degradedRoutes, &observedRoutes,
			&failures, &lastSuccess, &lastFailure, &requestCount24h, &successfulCount24h,
			&requestCount7d, &successfulCount7d,
			&recentStatusesRaw, &lastLatency, &lastRequestStatus, &lastRequestAt, &lastFailureReason,
		); err != nil {
			return ModelStatusReport{}, err
		}
		group.GroupStatus = strings.ToLower(strings.TrimSpace(groupStatus))
		if primaryModel.Valid {
			group.PrimaryModel = strings.TrimSpace(primaryModel.String)
		}
		if existing, ok := byID[group.ID]; ok {
			group = groups[existing.index]
		} else {
			group.Models = make([]ModelStatus, 0)
			byID[group.ID] = groupIndex{index: len(groups)}
			groups = append(groups, group)
		}
		if !modelID.Valid || !modelName.Valid || strings.TrimSpace(modelName.String) == "" {
			continue
		}
		model := ModelStatus{
			Model:               strings.TrimSpace(modelName.String),
			Provider:            strings.ToLower(strings.TrimSpace(provider.String)),
			TotalRoutes:         totalRoutes,
			AvailableRoutes:     availableRoutes,
			ObservedRoutes:      observedRoutes,
			ConsecutiveFailures: failures,
			Status:              modelRouteStatus(group.GroupStatus, totalRoutes, availableRoutes, degradedRoutes, observedRoutes),
			RequestCount24h:     requestCount24h,
			RequestCount7d:      requestCount7d,
		}
		model.AvailabilityRealtime = routeAvailability(group.GroupStatus, totalRoutes, availableRoutes)
		if requestCount24h > 0 {
			model.Availability24h = float64(successfulCount24h) / float64(requestCount24h) * 100
		}
		if requestCount7d > 0 {
			model.Availability7d = float64(successfulCount7d) / float64(requestCount7d) * 100
		}
		if lastSuccess.Valid {
			value := lastSuccess.Time.UTC()
			model.LastSuccessAt = &value
		}
		if lastFailure.Valid {
			value := lastFailure.Time.UTC()
			model.LastFailureAt = &value
		}
		if lastLatency.Valid {
			model.LastLatencyMS = lastLatency.Int64
		}
		if lastRequestAt.Valid {
			value := lastRequestAt.Time.UTC()
			model.LastRequestAt = &value
		}
		if lastRequestStatus.Valid {
			model.LastRequestStatus = strings.TrimSpace(lastRequestStatus.String)
		}
		// Failure reasons are internal diagnostics. Keep them available to the
		// admin report, but do not expose upstream/provider details to tenants.
		if len(recentStatusesRaw) > 0 && string(recentStatusesRaw) != "null" {
			_ = json.Unmarshal(recentStatusesRaw, &model.RecentStatuses)
		}
		groups[byID[group.ID].index].Models = append(groups[byID[group.ID].index].Models, model)
	}
	if err := rows.Err(); err != nil {
		return ModelStatusReport{}, err
	}
	for index := range groups {
		groups[index].Status = groupRouteStatus(groups[index].GroupStatus, groups[index].Models)
	}
	return ModelStatusReport{UpdatedAt: time.Now().UTC(), Groups: groups}, nil
}

func routeAvailability(groupStatus string, totalRoutes, availableRoutes int) float64 {
	if strings.TrimSpace(groupStatus) != StatusActive || totalRoutes <= 0 {
		return 0
	}
	if availableRoutes < 0 {
		availableRoutes = 0
	}
	if availableRoutes > totalRoutes {
		availableRoutes = totalRoutes
	}
	return float64(availableRoutes) / float64(totalRoutes) * 100
}

func modelRouteStatus(groupStatus string, totalRoutes, availableRoutes, degradedRoutes, observedRoutes int) string {
	if strings.TrimSpace(groupStatus) != StatusActive {
		return "disabled"
	}
	if totalRoutes == 0 || availableRoutes == 0 {
		return "unavailable"
	}
	if degradedRoutes > 0 {
		return "degraded"
	}
	if availableRoutes < totalRoutes {
		return "degraded"
	}
	// A configured route is not proven healthy until a real request or an
	// active monitor probe has recorded either a success or a failure. Passive
	// monitors still wait for real traffic because they have no probe result.
	if observedRoutes < totalRoutes {
		return "pending"
	}
	return "normal"
}

func groupRouteStatus(groupStatus string, models []ModelStatus) string {
	if strings.TrimSpace(groupStatus) != StatusActive {
		return "disabled"
	}
	if len(models) == 0 {
		return "unavailable"
	}
	hasAvailable, hasUnavailable, hasPending, hasDegraded := false, false, false, false
	for _, model := range models {
		if model.Status == "normal" || model.Status == "degraded" {
			hasAvailable = true
		}
		if model.Status == "degraded" {
			hasDegraded = true
		}
		if model.Status == "pending" {
			hasPending = true
			hasAvailable = true
		}
		if model.Status == "unavailable" || model.Status == "disabled" {
			hasUnavailable = true
		}
	}
	if !hasAvailable {
		return "unavailable"
	}
	if hasUnavailable {
		return "degraded"
	}
	if hasDegraded {
		return "degraded"
	}
	if hasPending {
		return "pending"
	}
	return "normal"
}

var _ ModelStatusLister = (*SQLService)(nil)

// ListAdminModelStatuses returns only models covered by an enabled admin
// monitor configuration. "all" configurations resolve their model set from
// the current group mappings on every read, while "selected" configurations
// retain explicitly selected model records even when all routes are down.
func (s *SQLService) ListAdminModelStatuses(ctx context.Context) (ModelStatusReport, error) {
	if s == nil || s.db == nil {
		return ModelStatusReport{}, ErrUnavailable
	}

	rows, err := s.db.QueryContext(ctx, `
		WITH monitor_models AS (
			SELECT mmc.id AS monitor_id, mmc.name AS monitor_name,
			       mmc.selection_mode, mmc.mode, mmc.probe_interval_seconds,
			       mmc.recent_request_limit,
			       mmc.last_probe_started_at, mmc.last_probe_finished_at,
			       mmc.last_probe_status, mmc.last_probe_error,
			       rg.id AS group_id, rg.code, rg.name, rg.status,
			       rg.multiplier, rg.rpm_limit, rg.billing_type, rg.metering_mode, rg.updated_at,
			       pm.model_name AS primary_model,
			       m.id AS model_id, m.model_name, m.provider, m.status AS model_status
			FROM model_monitor_configs mmc
			JOIN routing_groups rg ON rg.id = mmc.group_id
			  AND rg.deleted_at IS NULL
			LEFT JOIN models pm ON pm.id = mmc.primary_model_id
			JOIN models m ON true
			WHERE mmc.enabled = true
			  AND (
			      (mmc.selection_mode = 'all' AND m.status = 'active' AND EXISTS (
			          SELECT 1
			          FROM routing_group_channels rgc0
			          JOIN channels c0 ON c0.id = rgc0.channel_id
			            AND c0.deleted_at IS NULL
			          JOIN channel_models cm0 ON cm0.channel_id = c0.id
			            AND cm0.model_id = m.id
			            AND cm0.enabled = true
			          WHERE rgc0.group_id = mmc.group_id
			      ))
			      OR (mmc.selection_mode = 'selected' AND EXISTS (
			          SELECT 1
			          FROM model_monitor_config_models cmm0
			          WHERE cmm0.config_id = mmc.id
			            AND cmm0.model_id = m.id
			      ))
			  )
		), health_requests AS (
			SELECT id, group_id, model_id, status, latency_ms, created_at, failure_reason
			FROM model_requests
			UNION ALL
			SELECT id, group_id, model_id, status, latency_ms, created_at, failure_reason
			FROM model_probe_requests
		)
		SELECT mm.monitor_id::text, mm.monitor_name, mm.selection_mode, mm.mode,
		       mm.probe_interval_seconds, mm.recent_request_limit, mm.last_probe_started_at,
		       mm.last_probe_finished_at, mm.last_probe_status, mm.last_probe_error,
		       mm.group_id::text, mm.code, mm.name, mm.status,
		       mm.multiplier::text, mm.rpm_limit, mm.billing_type, mm.metering_mode, mm.updated_at,
		       mm.primary_model, mm.model_name, mm.provider,
		       COUNT(cm.model_id)::int,
		       COUNT(cm.model_id) FILTER (WHERE mm.model_status = 'active'
		           AND cm.enabled = true
		           AND c.status = 'active'
		           AND (c.auto_disabled_until IS NULL OR c.auto_disabled_until <= now())
		           AND (cm.auto_disabled_until IS NULL OR cm.auto_disabled_until <= now()))::int,
		       COUNT(cm.model_id) FILTER (WHERE cm.health_status IN ('degraded', 'unavailable')
		           OR (mm.mode = 'active' AND cm.probe_health IN ('degraded', 'unavailable')))::int,
		       COUNT(cm.model_id) FILTER (WHERE cm.last_success_at IS NOT NULL
		           OR cm.last_failure_at IS NOT NULL
		           OR (mm.mode = 'active' AND (cm.probe_last_success_at IS NOT NULL OR cm.probe_last_failure_at IS NOT NULL)))::int,
		       GREATEST(COALESCE(MAX(cm.consecutive_failures), 0), COALESCE(MAX(cm.probe_consecutive_failures) FILTER (WHERE mm.mode = 'active'), 0))::int,
		       GREATEST(MAX(cm.last_success_at), MAX(cm.probe_last_success_at)),
		       GREATEST(MAX(cm.last_failure_at), MAX(cm.probe_last_failure_at)),
		       COALESCE((SELECT COUNT(*)::int
           FROM health_requests mr
		           WHERE mr.group_id = mm.group_id AND mr.model_id = mm.model_id
		             AND mr.status IN ('settled', 'settlement_pending', 'failed')
		             AND mr.created_at >= now() - interval '24 hours'), 0)::int,
		       COALESCE((SELECT COUNT(*)::int
           FROM health_requests mr
		           WHERE mr.group_id = mm.group_id AND mr.model_id = mm.model_id
		             AND mr.status IN ('settled', 'settlement_pending')
		             AND mr.created_at >= now() - interval '24 hours'), 0)::int,
		       COALESCE((SELECT COUNT(*)::int
           FROM health_requests mr
		           WHERE mr.group_id = mm.group_id AND mr.model_id = mm.model_id
		             AND mr.status IN ('settled', 'settlement_pending', 'failed')
		             AND mr.created_at >= now() - interval '7 days'), 0)::int,
		       COALESCE((SELECT COUNT(*)::int
           FROM health_requests mr
		           WHERE mr.group_id = mm.group_id AND mr.model_id = mm.model_id
		             AND mr.status IN ('settled', 'settlement_pending')
		             AND mr.created_at >= now() - interval '7 days'), 0)::int,
		       COALESCE((SELECT jsonb_agg(recent.status ORDER BY recent.created_at ASC, recent.id ASC)
		           FROM (
		               SELECT mr.id, mr.status, mr.created_at
           FROM health_requests mr
		               WHERE mr.group_id = mm.group_id AND mr.model_id = mm.model_id
		               ORDER BY mr.created_at DESC, mr.id DESC
		               LIMIT mm.recent_request_limit
		           ) recent), '[]'::jsonb),
		       (SELECT mr.latency_ms
           FROM health_requests mr
		           WHERE mr.group_id = mm.group_id AND mr.model_id = mm.model_id
		           ORDER BY mr.created_at DESC, mr.id DESC
		           LIMIT 1),
		       (SELECT mr.status
               FROM health_requests mr
		           WHERE mr.group_id = mm.group_id AND mr.model_id = mm.model_id
		           ORDER BY mr.created_at DESC, mr.id DESC
		           LIMIT 1),
		       (SELECT mr.created_at
           FROM health_requests mr
		           WHERE mr.group_id = mm.group_id AND mr.model_id = mm.model_id
		           ORDER BY mr.created_at DESC, mr.id DESC
		           LIMIT 1),
		       (SELECT COALESCE(mr.failure_reason, '')
           FROM health_requests mr
		           WHERE mr.group_id = mm.group_id AND mr.model_id = mm.model_id
		             AND mr.status = 'failed'
		           ORDER BY mr.created_at DESC, mr.id DESC
		           LIMIT 1)
		FROM monitor_models mm
		LEFT JOIN routing_group_channels rgc ON rgc.group_id = mm.group_id
		LEFT JOIN channels c ON c.id = rgc.channel_id AND c.deleted_at IS NULL
		LEFT JOIN channel_models cm ON cm.channel_id = c.id
		  AND cm.model_id = mm.model_id
		GROUP BY mm.monitor_id, mm.monitor_name, mm.selection_mode, mm.mode,
		         mm.probe_interval_seconds, mm.recent_request_limit, mm.last_probe_started_at,
		         mm.last_probe_finished_at, mm.last_probe_status,
		         mm.last_probe_error, mm.group_id, mm.code, mm.name,
		         mm.status, mm.multiplier, mm.rpm_limit, mm.billing_type, mm.metering_mode,
		         mm.updated_at, mm.primary_model, mm.model_id, mm.model_name, mm.provider, mm.model_status
		ORDER BY mm.code ASC, mm.provider ASC, mm.model_name ASC
	`)
	if err != nil {
		return ModelStatusReport{}, err
	}
	defer rows.Close()

	groups := make([]ModelStatusGroup, 0)
	byID := make(map[string]int)
	for rows.Next() {
		var (
			group                                                                            ModelStatusGroup
			monitorID, monitorName, selectionMode                                            string
			monitorMode, lastProbeStatus, lastProbeError                                     string
			probeInterval, recentRequestLimit                                                int
			lastProbeStarted, lastProbeFinished                                              sql.NullTime
			groupStatus                                                                      string
			modelName, provider                                                              sql.NullString
			primaryModel                                                                     sql.NullString
			totalRoutes, availableRoutes, degradedRoutes, observedRoutes                     int
			failures, requestCount24h, successfulCount24h, requestCount7d, successfulCount7d int
			lastSuccess, lastFailure, lastRequestAt                                          sql.NullTime
			lastLatency                                                                      sql.NullInt64
			lastRequestStatus, lastFailureReason                                             sql.NullString
			recentStatusesRaw                                                                []byte
		)
		if err := rows.Scan(
			&monitorID, &monitorName, &selectionMode, &monitorMode,
			&probeInterval, &recentRequestLimit, &lastProbeStarted, &lastProbeFinished,
			&lastProbeStatus, &lastProbeError,
			&group.ID, &group.Code, &group.Name, &groupStatus,
			&group.Multiplier, &group.RPMLimit, &group.BillingType, &group.MeteringMode, &group.UpdatedAt,
			&primaryModel, &modelName, &provider, &totalRoutes, &availableRoutes, &degradedRoutes, &observedRoutes,
			&failures, &lastSuccess, &lastFailure, &requestCount24h, &successfulCount24h, &requestCount7d,
			&successfulCount7d, &recentStatusesRaw, &lastLatency,
			&lastRequestStatus, &lastRequestAt, &lastFailureReason,
		); err != nil {
			return ModelStatusReport{}, err
		}
		group.MonitorID = monitorID
		group.MonitorName = monitorName
		group.SelectionMode = selectionMode
		group.MonitorMode = monitorMode
		group.ProbeIntervalSeconds = probeInterval
		group.RecentRequestLimit = recentRequestLimit
		group.LastProbeStatus = lastProbeStatus
		group.LastProbeError = lastProbeError
		if lastProbeStarted.Valid {
			value := lastProbeStarted.Time.UTC()
			group.LastProbeStartedAt = &value
		}
		if lastProbeFinished.Valid {
			value := lastProbeFinished.Time.UTC()
			group.LastProbeFinishedAt = &value
		}
		group.GroupStatus = strings.ToLower(strings.TrimSpace(groupStatus))
		if primaryModel.Valid {
			group.PrimaryModel = strings.TrimSpace(primaryModel.String)
		}
		index, ok := byID[group.ID]
		if !ok {
			group.Models = make([]ModelStatus, 0)
			byID[group.ID] = len(groups)
			groups = append(groups, group)
			index = len(groups) - 1
		}
		if !modelName.Valid || strings.TrimSpace(modelName.String) == "" {
			continue
		}
		model := ModelStatus{
			Model:               strings.TrimSpace(modelName.String),
			Provider:            strings.ToLower(strings.TrimSpace(provider.String)),
			TotalRoutes:         totalRoutes,
			AvailableRoutes:     availableRoutes,
			ObservedRoutes:      observedRoutes,
			ConsecutiveFailures: failures,
			Status:              modelRouteStatus(group.GroupStatus, totalRoutes, availableRoutes, degradedRoutes, observedRoutes),
			RequestCount24h:     requestCount24h,
			RequestCount7d:      requestCount7d,
		}
		model.AvailabilityRealtime = routeAvailability(group.GroupStatus, totalRoutes, availableRoutes)
		if requestCount24h > 0 {
			model.Availability24h = float64(successfulCount24h) / float64(requestCount24h) * 100
		}
		if requestCount7d > 0 {
			model.Availability7d = float64(successfulCount7d) / float64(requestCount7d) * 100
		}
		if lastSuccess.Valid {
			value := lastSuccess.Time.UTC()
			model.LastSuccessAt = &value
		}
		if lastFailure.Valid {
			value := lastFailure.Time.UTC()
			model.LastFailureAt = &value
		}
		if lastLatency.Valid {
			model.LastLatencyMS = lastLatency.Int64
		}
		if lastRequestAt.Valid {
			value := lastRequestAt.Time.UTC()
			model.LastRequestAt = &value
		}
		if lastRequestStatus.Valid {
			model.LastRequestStatus = strings.TrimSpace(lastRequestStatus.String)
		}
		if lastFailureReason.Valid {
			model.LastFailureReason = strings.TrimSpace(lastFailureReason.String)
		}
		if len(recentStatusesRaw) > 0 && string(recentStatusesRaw) != "null" {
			_ = json.Unmarshal(recentStatusesRaw, &model.RecentStatuses)
		}
		groups[index].Models = append(groups[index].Models, model)
	}
	if err := rows.Err(); err != nil {
		return ModelStatusReport{}, err
	}
	for index := range groups {
		groups[index].Status = groupRouteStatus(groups[index].GroupStatus, groups[index].Models)
	}
	return ModelStatusReport{UpdatedAt: time.Now().UTC(), Groups: groups}, nil
}

var _ AdminModelStatusLister = (*SQLService)(nil)
