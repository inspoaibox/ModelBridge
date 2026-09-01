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
	Model               string     `json:"model"`
	Provider            string     `json:"provider"`
	Status              string     `json:"status"`
	TotalRoutes         int        `json:"total_routes"`
	AvailableRoutes     int        `json:"available_routes"`
	ObservedRoutes      int        `json:"observed_routes"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	LastSuccessAt       *time.Time `json:"last_success_at,omitempty"`
	LastFailureAt       *time.Time `json:"last_failure_at,omitempty"`
	LastLatencyMS       int64      `json:"last_latency_ms"`
	Availability7d      float64    `json:"availability_7d"`
	RequestCount7d      int        `json:"request_count_7d"`
	RecentStatuses      []string   `json:"recent_statuses,omitempty"`
	LastRequestAt       *time.Time `json:"last_request_at,omitempty"`
	LastRequestStatus   string     `json:"last_request_status,omitempty"`
	LastFailureReason   string     `json:"last_failure_reason,omitempty"`
}

type ModelStatusGroup struct {
	ID          string        `json:"group_id"`
	Code        string        `json:"group_code"`
	Name        string        `json:"group_name"`
	Status      string        `json:"status"`
	GroupStatus string        `json:"group_status"`
	Multiplier  string        `json:"multiplier"`
	RPMLimit    int           `json:"rpm_limit"`
	BillingType string        `json:"billing_type"`
	Models      []ModelStatus `json:"models"`
	UpdatedAt   time.Time     `json:"updated_at"`
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
// tenant. Active groups are available to choose; a disabled group remains
// visible only when one of the tenant's live tokens is bound to it. A model is
// normal when every configured route is currently eligible
// for selection, degraded when only some are eligible, and unavailable when
// no route can currently be selected. A disabled group is reported separately
// so the UI can distinguish configuration state from route health.
func (s *SQLService) ListModelStatuses(ctx context.Context, tenantID string) (ModelStatusReport, error) {
	if s == nil || s.db == nil {
		return ModelStatusReport{}, ErrUnavailable
	}
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" || !ids.Valid(tenantID) {
		return ModelStatusReport{}, ErrInvalidRequest
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT rg.id::text, rg.code, rg.name, rg.status,
		       rg.multiplier::text, rg.rpm_limit, rg.billing_type, rg.updated_at,
		       m.model_name, m.provider,
		       COUNT(m.id)::int,
		       COUNT(m.id) FILTER (WHERE c.status = 'active'
		           AND (c.auto_disabled_until IS NULL OR c.auto_disabled_until <= now())
		           AND (cm.auto_disabled_until IS NULL OR cm.auto_disabled_until <= now()))::int,
		       COUNT(m.id) FILTER (WHERE cm.last_success_at IS NOT NULL
		           OR cm.last_failure_at IS NOT NULL)::int,
		       COALESCE(MAX(cm.consecutive_failures), 0)::int,
		       MAX(cm.last_success_at), MAX(cm.last_failure_at)
		FROM routing_groups rg
		LEFT JOIN routing_group_channels rgc ON rgc.group_id = rg.id
		LEFT JOIN channels c ON c.id = rgc.channel_id AND c.deleted_at IS NULL
		LEFT JOIN channel_models cm ON cm.channel_id = c.id AND cm.enabled = true
		LEFT JOIN models m ON m.id = cm.model_id AND m.status = 'active'
		WHERE rg.deleted_at IS NULL
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
		GROUP BY rg.id, rg.code, rg.name, rg.status, rg.multiplier,
		         rg.rpm_limit, rg.billing_type, rg.updated_at,
		         m.model_name, m.provider
		ORDER BY rg.priority DESC, rg.code ASC, m.provider ASC, m.model_name ASC
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
			group                                        ModelStatusGroup
			groupStatus                                  string
			modelName, provider                          sql.NullString
			totalRoutes, availableRoutes, observedRoutes int
			failures                                     int
			lastSuccess, lastFailure                     sql.NullTime
		)
		if err := rows.Scan(
			&group.ID, &group.Code, &group.Name, &groupStatus,
			&group.Multiplier, &group.RPMLimit, &group.BillingType, &group.UpdatedAt,
			&modelName, &provider, &totalRoutes, &availableRoutes, &observedRoutes,
			&failures, &lastSuccess, &lastFailure,
		); err != nil {
			return ModelStatusReport{}, err
		}
		group.GroupStatus = strings.ToLower(strings.TrimSpace(groupStatus))
		if existing, ok := byID[group.ID]; ok {
			group = groups[existing.index]
		} else {
			group.Models = make([]ModelStatus, 0)
			byID[group.ID] = groupIndex{index: len(groups)}
			groups = append(groups, group)
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
			Status:              modelRouteStatus(group.GroupStatus, totalRoutes, availableRoutes, observedRoutes),
		}
		if lastSuccess.Valid {
			value := lastSuccess.Time.UTC()
			model.LastSuccessAt = &value
		}
		if lastFailure.Valid {
			value := lastFailure.Time.UTC()
			model.LastFailureAt = &value
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

func modelRouteStatus(groupStatus string, totalRoutes, availableRoutes, observedRoutes int) string {
	if strings.TrimSpace(groupStatus) != StatusActive {
		return "disabled"
	}
	if totalRoutes == 0 || availableRoutes == 0 {
		return "unavailable"
	}
	if availableRoutes < totalRoutes {
		return "degraded"
	}
	// A configured route is not proven healthy until a real request has
	// recorded either a success or a failure. This avoids presenting an
	// untested channel as healthy immediately after it is added.
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

// ListAdminModelStatuses returns every non-deleted routing group and every
// active model mapped to one of its channels. Request metrics are derived
// from real model_requests rows; no upstream probe is performed here.
func (s *SQLService) ListAdminModelStatuses(ctx context.Context) (ModelStatusReport, error) {
	if s == nil || s.db == nil {
		return ModelStatusReport{}, ErrUnavailable
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT rg.id::text, rg.code, rg.name, rg.status,
		       rg.multiplier::text, rg.rpm_limit, rg.billing_type, rg.updated_at,
		       m.model_name, m.provider,
		       COUNT(m.id)::int,
		       COUNT(m.id) FILTER (WHERE cm.enabled = true
		           AND c.status = 'active'
		           AND (c.auto_disabled_until IS NULL OR c.auto_disabled_until <= now())
		           AND (cm.auto_disabled_until IS NULL OR cm.auto_disabled_until <= now()))::int,
		       COUNT(m.id) FILTER (WHERE cm.last_success_at IS NOT NULL
		           OR cm.last_failure_at IS NOT NULL)::int,
		       COALESCE(MAX(cm.consecutive_failures), 0)::int,
		       MAX(cm.last_success_at), MAX(cm.last_failure_at),
		       COALESCE((SELECT COUNT(*)::int
		           FROM model_requests mr
		           WHERE mr.group_id = rg.id AND mr.model_id = m.id
		             AND mr.status IN ('settled', 'settlement_pending', 'failed')
		             AND mr.created_at >= now() - interval '7 days'), 0)::int,
		       COALESCE((SELECT COUNT(*)::int
		           FROM model_requests mr
		           WHERE mr.group_id = rg.id AND mr.model_id = m.id
		             AND mr.status IN ('settled', 'settlement_pending')
		             AND mr.created_at >= now() - interval '7 days'), 0)::int,
		       COALESCE((SELECT jsonb_agg(recent.status ORDER BY recent.created_at DESC, recent.id DESC)
		           FROM (
		               SELECT mr.id, mr.status, mr.created_at
		               FROM model_requests mr
		               WHERE mr.group_id = rg.id AND mr.model_id = m.id
		               ORDER BY mr.created_at DESC, mr.id DESC
		               LIMIT 60
		           ) recent), '[]'::jsonb),
		       (SELECT mr.latency_ms
		           FROM model_requests mr
		           WHERE mr.group_id = rg.id AND mr.model_id = m.id
		           ORDER BY mr.created_at DESC, mr.id DESC
		           LIMIT 1),
		       (SELECT mr.status
		           FROM model_requests mr
		           WHERE mr.group_id = rg.id AND mr.model_id = m.id
		           ORDER BY mr.created_at DESC, mr.id DESC
		           LIMIT 1),
		       (SELECT mr.created_at
		           FROM model_requests mr
		           WHERE mr.group_id = rg.id AND mr.model_id = m.id
		           ORDER BY mr.created_at DESC, mr.id DESC
		           LIMIT 1),
		       (SELECT COALESCE(mr.failure_reason, '')
		           FROM model_requests mr
		           WHERE mr.group_id = rg.id AND mr.model_id = m.id
		             AND mr.status = 'failed'
		           ORDER BY mr.created_at DESC, mr.id DESC
		           LIMIT 1)
		FROM routing_groups rg
		LEFT JOIN routing_group_channels rgc ON rgc.group_id = rg.id
		LEFT JOIN channels c ON c.id = rgc.channel_id AND c.deleted_at IS NULL
		LEFT JOIN channel_models cm ON cm.channel_id = c.id
		LEFT JOIN models m ON m.id = cm.model_id AND m.status = 'active'
		WHERE rg.deleted_at IS NULL
		GROUP BY rg.id, rg.code, rg.name, rg.status, rg.multiplier,
		         rg.rpm_limit, rg.billing_type, rg.updated_at,
		         m.id, m.model_name, m.provider
		ORDER BY rg.priority DESC, rg.code ASC, m.provider ASC, m.model_name ASC
	`)
	if err != nil {
		return ModelStatusReport{}, err
	}
	defer rows.Close()

	groups := make([]ModelStatusGroup, 0)
	byID := make(map[string]int)
	for rows.Next() {
		var (
			group                                        ModelStatusGroup
			groupStatus                                  string
			modelName, provider                          sql.NullString
			totalRoutes, availableRoutes, observedRoutes int
			failures, requestCount7d, successfulCount7d  int
			lastSuccess, lastFailure, lastRequestAt      sql.NullTime
			lastLatency                                  sql.NullInt64
			lastRequestStatus, lastFailureReason         sql.NullString
			recentStatusesRaw                            []byte
		)
		if err := rows.Scan(
			&group.ID, &group.Code, &group.Name, &groupStatus,
			&group.Multiplier, &group.RPMLimit, &group.BillingType, &group.UpdatedAt,
			&modelName, &provider, &totalRoutes, &availableRoutes, &observedRoutes,
			&failures, &lastSuccess, &lastFailure, &requestCount7d,
			&successfulCount7d, &recentStatusesRaw, &lastLatency,
			&lastRequestStatus, &lastRequestAt, &lastFailureReason,
		); err != nil {
			return ModelStatusReport{}, err
		}
		group.GroupStatus = strings.ToLower(strings.TrimSpace(groupStatus))
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
			Status:              modelRouteStatus(group.GroupStatus, totalRoutes, availableRoutes, observedRoutes),
			RequestCount7d:      requestCount7d,
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
