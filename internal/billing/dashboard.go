package billing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ConsoleDashboardQuery is always scoped by the HTTP layer to one tenant and
// the projects visible to the signed-in console principal.
type ConsoleDashboardQuery struct {
	TenantID   string
	UserID     string
	ProjectIDs []string
	From       *time.Time
	To         *time.Time
}

type ConsoleDashboardModel struct {
	Model       string `json:"model"`
	Provider    string `json:"provider"`
	Requests    int64  `json:"requests"`
	TotalTokens int64  `json:"total_tokens"`
	TotalCost   string `json:"total_cost"`
}

type ConsoleDashboardTrendPoint struct {
	At           time.Time `json:"at"`
	Requests     int64     `json:"requests"`
	InputTokens  int64     `json:"input_tokens"`
	OutputTokens int64     `json:"output_tokens"`
	TotalTokens  int64     `json:"total_tokens"`
	TotalCost    string    `json:"total_cost"`
}

type ConsoleDashboardReport struct {
	CollectedAt         time.Time                    `json:"collected_at"`
	SystemStatus        string                       `json:"system_status"`
	UptimeSeconds       int64                        `json:"uptime_seconds"`
	RangeFrom           time.Time                    `json:"range_from"`
	RangeTo             time.Time                    `json:"range_to"`
	TotalRequests       int64                        `json:"total_requests"`
	TotalAPIKeys        int64                        `json:"total_api_keys"`
	ActiveAPIKeys       int64                        `json:"active_api_keys"`
	TodayRequests       int64                        `json:"today_requests"`
	TotalTokens         int64                        `json:"total_tokens"`
	TodayTokens         int64                        `json:"today_tokens"`
	TotalInputTokens    int64                        `json:"total_input_tokens"`
	TotalOutputTokens   int64                        `json:"total_output_tokens"`
	TodayInputTokens    int64                        `json:"today_input_tokens"`
	TodayOutputTokens   int64                        `json:"today_output_tokens"`
	TotalCost           string                       `json:"total_cost"`
	TodayCost           string                       `json:"today_cost"`
	TotalCostByCurrency map[string]string            `json:"total_cost_by_currency,omitempty"`
	TodayCostByCurrency map[string]string            `json:"today_cost_by_currency,omitempty"`
	RealtimeRPM         int64                        `json:"realtime_rpm"`
	RealtimeTPM         int64                        `json:"realtime_tpm"`
	Models              []ConsoleDashboardModel      `json:"models"`
	TokenTrend          []ConsoleDashboardTrendPoint `json:"token_trend"`
}

type ConsoleDashboardReporter interface {
	GetConsoleDashboard(context.Context, ConsoleDashboardQuery) (ConsoleDashboardReport, error)
}

func (s *SQLService) GetConsoleDashboard(ctx context.Context, query ConsoleDashboardQuery) (ConsoleDashboardReport, error) {
	if s == nil || s.db == nil {
		return ConsoleDashboardReport{}, errors.New("dashboard service is not configured")
	}
	query.TenantID = strings.TrimSpace(query.TenantID)
	if query.TenantID == "" {
		return ConsoleDashboardReport{}, ErrInvalidRequest
	}
	query.UserID = strings.TrimSpace(query.UserID)
	if query.UserID == "" {
		return ConsoleDashboardReport{}, ErrInvalidRequest
	}
	now := time.Now().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if query.From == nil {
		value := todayStart
		query.From = &value
	}
	if query.To == nil {
		value := now
		query.To = &value
	}
	if !query.From.Before(*query.To) {
		return ConsoleDashboardReport{}, ErrInvalidRequest
	}

	report := ConsoleDashboardReport{CollectedAt: now, RangeFrom: *query.From, RangeTo: *query.To}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)::bigint,
		       COUNT(*) FILTER (WHERE status = 'active' AND (expires_at IS NULL OR expires_at > now()))::bigint
		FROM api_tokens
		WHERE tenant_id = $1::uuid AND created_by = $2::uuid AND deleted_at IS NULL AND status <> 'revoked'
	`, query.TenantID, query.UserID).Scan(&report.TotalAPIKeys, &report.ActiveAPIKeys); err != nil {
		return ConsoleDashboardReport{}, err
	}
	allWhere, allArgs := usageWhere(UsageQuery{TenantID: query.TenantID, ProjectIDs: query.ProjectIDs})
	if err := s.scanDashboardTotals(ctx, allWhere, allArgs, &report.TotalRequests, &report.TotalInputTokens, &report.TotalOutputTokens, &report.TotalTokens, &report.TotalCost); err != nil {
		return ConsoleDashboardReport{}, err
	}
	todayWhere, todayArgs := usageWhere(UsageQuery{TenantID: query.TenantID, ProjectIDs: query.ProjectIDs, From: &todayStart, To: &now})
	if err := s.scanDashboardTotals(ctx, todayWhere, todayArgs, &report.TodayRequests, &report.TodayInputTokens, &report.TodayOutputTokens, &report.TodayTokens, &report.TodayCost); err != nil {
		return ConsoleDashboardReport{}, err
	}
	rangeWhere, rangeArgs := usageWhere(UsageQuery{TenantID: query.TenantID, ProjectIDs: query.ProjectIDs, From: query.From, To: query.To})
	if err := s.scanDashboardModels(ctx, rangeWhere, rangeArgs, &report.Models); err != nil {
		return ConsoleDashboardReport{}, err
	}
	if err := s.scanDashboardTrend(ctx, rangeWhere, rangeArgs, query.To.Sub(*query.From) <= 48*time.Hour, &report.TokenTrend); err != nil {
		return ConsoleDashboardReport{}, err
	}
	recentFrom := now.Add(-5 * time.Minute)
	recentWhere, recentArgs := usageWhere(UsageQuery{TenantID: query.TenantID, ProjectIDs: query.ProjectIDs, From: &recentFrom, To: &now})
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)::bigint,
		       COALESCE(SUM(mr.input_tokens + mr.output_tokens), 0)::bigint
		FROM model_requests mr
		JOIN api_tokens tok ON tok.id = mr.token_id
		JOIN tenants ten ON ten.id = mr.tenant_id
		JOIN models mod ON mod.id = mr.model_id
		LEFT JOIN routing_groups grp ON grp.id = mr.group_id
		WHERE `+recentWhere, recentArgs...).Scan(&report.RealtimeRPM, &report.RealtimeTPM); err != nil {
		return ConsoleDashboardReport{}, err
	}
	// RPM/TPM are rates, not five-minute totals.
	report.RealtimeRPM = report.RealtimeRPM * 12
	report.RealtimeTPM = report.RealtimeTPM * 12
	return report, nil
}

func (s *SQLService) scanDashboardTotals(ctx context.Context, where string, args []any, requests, input, output, total *int64, cost *string) error {
	return s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)::bigint,
		       COALESCE(SUM(mr.input_tokens), 0)::bigint,
		       COALESCE(SUM(mr.output_tokens), 0)::bigint,
		       COALESCE(SUM(mr.input_tokens + mr.output_tokens), 0)::bigint,
		       COALESCE(SUM(mr.settled_amount) FILTER (WHERE mr.status = 'settled'), 0)::text
		FROM model_requests mr
		JOIN api_tokens tok ON tok.id = mr.token_id
		JOIN tenants ten ON ten.id = mr.tenant_id
		JOIN models mod ON mod.id = mr.model_id
		LEFT JOIN routing_groups grp ON grp.id = mr.group_id
		WHERE `+where, args...).Scan(requests, input, output, total, cost)
}

func (s *SQLService) scanDashboardModels(ctx context.Context, where string, args []any, result *[]ConsoleDashboardModel) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT mod.model_name, mod.provider, COUNT(*)::bigint,
		       COALESCE(SUM(mr.input_tokens + mr.output_tokens), 0)::bigint,
		       COALESCE(SUM(mr.settled_amount) FILTER (WHERE mr.status = 'settled'), 0)::text
		FROM model_requests mr
		JOIN api_tokens tok ON tok.id = mr.token_id
		JOIN tenants ten ON ten.id = mr.tenant_id
		JOIN models mod ON mod.id = mr.model_id
		LEFT JOIN routing_groups grp ON grp.id = mr.group_id
		WHERE `+where+`
		GROUP BY mod.model_name, mod.provider
		ORDER BY COUNT(*) DESC, mod.model_name
		LIMIT 100`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := make([]ConsoleDashboardModel, 0)
	for rows.Next() {
		var item ConsoleDashboardModel
		if err := rows.Scan(&item.Model, &item.Provider, &item.Requests, &item.TotalTokens, &item.TotalCost); err != nil {
			return err
		}
		item.TotalCost = normalizeDecimalText(item.TotalCost)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	*result = items
	return nil
}

func (s *SQLService) scanDashboardTrend(ctx context.Context, where string, args []any, hourly bool, result *[]ConsoleDashboardTrendPoint) error {
	bucket := "day"
	if hourly {
		bucket = "hour"
	}
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT date_trunc('%s', mr.created_at), COUNT(*)::bigint,
		       COALESCE(SUM(mr.input_tokens), 0)::bigint,
		       COALESCE(SUM(mr.output_tokens), 0)::bigint,
		       COALESCE(SUM(mr.input_tokens + mr.output_tokens), 0)::bigint,
		       COALESCE(SUM(mr.settled_amount) FILTER (WHERE mr.status = 'settled'), 0)::text
		FROM model_requests mr
		JOIN api_tokens tok ON tok.id = mr.token_id
		JOIN tenants ten ON ten.id = mr.tenant_id
		JOIN models mod ON mod.id = mr.model_id
		LEFT JOIN routing_groups grp ON grp.id = mr.group_id
		WHERE %s
		GROUP BY 1
		ORDER BY 1`, bucket, where), args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := make([]ConsoleDashboardTrendPoint, 0)
	for rows.Next() {
		var item ConsoleDashboardTrendPoint
		if err := rows.Scan(&item.At, &item.Requests, &item.InputTokens, &item.OutputTokens, &item.TotalTokens, &item.TotalCost); err != nil {
			return err
		}
		item.At = item.At.UTC()
		item.TotalCost = normalizeDecimalText(item.TotalCost)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	*result = items
	return nil
}

var _ ConsoleDashboardReporter = (*SQLService)(nil)
