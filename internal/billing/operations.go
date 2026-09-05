package billing

import (
	"context"
	"errors"
	"strings"
	"time"
)

type OperationsModelUsage struct {
	Model       string `json:"model"`
	Provider    string `json:"provider"`
	Requests    int64  `json:"requests"`
	TotalTokens int64  `json:"total_tokens"`
	TotalSpend  string `json:"total_spend"`
}

type OperationsTrendPoint struct {
	At           time.Time `json:"at"`
	Requests     int64     `json:"requests"`
	InputTokens  int64     `json:"input_tokens"`
	OutputTokens int64     `json:"output_tokens"`
	TotalTokens  int64     `json:"total_tokens"`
	TotalSpend   string    `json:"total_spend"`
}

type OperationsSnapshot struct {
	Users                 int64                  `json:"users"`
	Tenants               int64                  `json:"tenants"`
	Channels              int64                  `json:"channels"`
	ActiveChannels        int64                  `json:"active_channels"`
	Groups                int64                  `json:"groups"`
	ActiveGroups          int64                  `json:"active_groups"`
	Tokens                int64                  `json:"tokens"`
	ActiveTokens          int64                  `json:"active_tokens"`
	Requests24h           int64                  `json:"requests_24h"`
	FailedRequests24h     int64                  `json:"failed_requests_24h"`
	Spend24h              string                 `json:"spend_24h"`
	AverageLatencyMS      float64                `json:"average_latency_ms"`
	TotalTokens           int64                  `json:"total_tokens"`
	TodayRequests         int64                  `json:"today_requests"`
	TodayInputTokens      int64                  `json:"today_input_tokens"`
	TodayOutputTokens     int64                  `json:"today_output_tokens"`
	TodayTokens           int64                  `json:"today_tokens"`
	TodaySpend            string                 `json:"today_spend"`
	TodayRechargeOrders   int64                  `json:"today_recharge_orders"`
	TodayRechargeAmount   string                 `json:"today_recharge_amount"`
	TodayCreditedAmount   string                 `json:"today_credited_amount"`
	TotalRechargeOrders   int64                  `json:"total_recharge_orders"`
	TotalRechargeAmount   string                 `json:"total_recharge_amount"`
	TotalCreditedAmount   string                 `json:"total_credited_amount"`
	PendingRechargeOrders int64                  `json:"pending_recharge_orders"`
	ModelUsage            []OperationsModelUsage `json:"model_usage"`
	UsageTrend            []OperationsTrendPoint `json:"usage_trend"`
	CollectedAt           time.Time              `json:"collected_at"`
}

type OperationsReporter interface {
	GetOperationsSnapshot(context.Context) (OperationsSnapshot, error)
}

func (s *SQLService) GetOperationsSnapshot(ctx context.Context) (OperationsSnapshot, error) {
	if s == nil || s.db == nil {
		return OperationsSnapshot{}, errors.New("operations service is not configured")
	}
	var snapshot OperationsSnapshot
	err := s.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM users WHERE deleted_at IS NULL),
			(SELECT COUNT(*) FROM tenants WHERE status = 'active' AND deleted_at IS NULL),
			(SELECT COUNT(*) FROM channels WHERE deleted_at IS NULL),
			(SELECT COUNT(*) FROM channels WHERE status = 'active' AND deleted_at IS NULL
				AND (auto_disabled_until IS NULL OR auto_disabled_until <= now())),
			(SELECT COUNT(*) FROM routing_groups WHERE deleted_at IS NULL),
			(SELECT COUNT(*) FROM routing_groups WHERE status = 'active' AND deleted_at IS NULL),
			(SELECT COUNT(*) FROM api_tokens WHERE status <> 'revoked' AND deleted_at IS NULL),
			(SELECT COUNT(*) FROM api_tokens WHERE status = 'active' AND deleted_at IS NULL AND (expires_at IS NULL OR expires_at > now())),
			(SELECT COUNT(*) FROM model_requests WHERE created_at >= now() - interval '24 hours'),
			(SELECT COUNT(*) FROM model_requests WHERE status = 'failed' AND created_at >= now() - interval '24 hours'),
			(SELECT COALESCE(SUM(settled_amount), 0)::text FROM model_requests
				WHERE status = 'settled' AND created_at >= now() - interval '24 hours'),
			(SELECT COALESCE(AVG(latency_ms) FILTER (WHERE latency_ms > 0), 0) FROM model_requests
				WHERE created_at >= now() - interval '24 hours'),
			(SELECT COALESCE(SUM(input_tokens + output_tokens), 0)::bigint FROM model_requests),
			(SELECT COUNT(*) FROM model_requests WHERE created_at >= date_trunc('day', now())),
			(SELECT COALESCE(SUM(input_tokens), 0)::bigint FROM model_requests WHERE created_at >= date_trunc('day', now())),
			(SELECT COALESCE(SUM(output_tokens), 0)::bigint FROM model_requests WHERE created_at >= date_trunc('day', now())),
			(SELECT COALESCE(SUM(input_tokens + output_tokens), 0)::bigint FROM model_requests WHERE created_at >= date_trunc('day', now())),
			(SELECT COALESCE(SUM(settled_amount), 0)::text FROM model_requests WHERE status = 'settled' AND created_at >= date_trunc('day', now())),
			(SELECT COUNT(*) FROM payment_orders WHERE status = 'paid' AND paid_at >= date_trunc('day', now())),
			(SELECT COALESCE(SUM(amount), 0)::text FROM payment_orders WHERE status = 'paid' AND paid_at >= date_trunc('day', now())),
			(SELECT COALESCE(SUM(credited_amount), 0)::text FROM payment_orders WHERE status = 'paid' AND paid_at >= date_trunc('day', now())),
			(SELECT COUNT(*) FROM payment_orders WHERE status = 'paid'),
			(SELECT COALESCE(SUM(amount), 0)::text FROM payment_orders WHERE status = 'paid'),
			(SELECT COALESCE(SUM(credited_amount), 0)::text FROM payment_orders WHERE status = 'paid'),
			(SELECT COUNT(*) FROM payment_orders WHERE status = 'pending')
	`).Scan(
		&snapshot.Users, &snapshot.Tenants, &snapshot.Channels, &snapshot.ActiveChannels,
		&snapshot.Groups, &snapshot.ActiveGroups, &snapshot.Tokens, &snapshot.ActiveTokens,
		&snapshot.Requests24h, &snapshot.FailedRequests24h, &snapshot.Spend24h,
		&snapshot.AverageLatencyMS, &snapshot.TotalTokens, &snapshot.TodayRequests,
		&snapshot.TodayInputTokens, &snapshot.TodayOutputTokens, &snapshot.TodayTokens,
		&snapshot.TodaySpend, &snapshot.TodayRechargeOrders, &snapshot.TodayRechargeAmount,
		&snapshot.TodayCreditedAmount, &snapshot.TotalRechargeOrders, &snapshot.TotalRechargeAmount,
		&snapshot.TotalCreditedAmount, &snapshot.PendingRechargeOrders,
	)
	if err != nil {
		return OperationsSnapshot{}, err
	}
	snapshot.Spend24h = strings.TrimSpace(snapshot.Spend24h)
	snapshot.TodaySpend = strings.TrimSpace(snapshot.TodaySpend)
	snapshot.TodayRechargeAmount = strings.TrimSpace(snapshot.TodayRechargeAmount)
	snapshot.TodayCreditedAmount = strings.TrimSpace(snapshot.TodayCreditedAmount)
	snapshot.TotalRechargeAmount = strings.TrimSpace(snapshot.TotalRechargeAmount)
	snapshot.TotalCreditedAmount = strings.TrimSpace(snapshot.TotalCreditedAmount)
	if snapshot.Spend24h == "" {
		snapshot.Spend24h = "0"
	}
	if snapshot.TodaySpend == "" {
		snapshot.TodaySpend = "0"
	}
	if snapshot.TodayRechargeAmount == "" {
		snapshot.TodayRechargeAmount = "0"
	}
	if snapshot.TodayCreditedAmount == "" {
		snapshot.TodayCreditedAmount = "0"
	}
	if snapshot.TotalRechargeAmount == "" {
		snapshot.TotalRechargeAmount = "0"
	}
	if snapshot.TotalCreditedAmount == "" {
		snapshot.TotalCreditedAmount = "0"
	}
	if snapshot.ModelUsage, err = s.listOperationsModelUsage(ctx); err != nil {
		return OperationsSnapshot{}, err
	}
	if snapshot.UsageTrend, err = s.listOperationsTrend(ctx); err != nil {
		return OperationsSnapshot{}, err
	}
	snapshot.CollectedAt = time.Now().UTC()
	return snapshot, nil
}

func (s *SQLService) listOperationsModelUsage(ctx context.Context) ([]OperationsModelUsage, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT mod.model_name, mod.provider, COUNT(*)::bigint,
		       COALESCE(SUM(mr.input_tokens + mr.output_tokens), 0)::bigint,
		       COALESCE(SUM(mr.settled_amount) FILTER (WHERE mr.status = 'settled'), 0)::text
		FROM model_requests mr
		JOIN models mod ON mod.id = mr.model_id
		WHERE mr.created_at >= now() - interval '30 days'
		GROUP BY mod.model_name, mod.provider
		ORDER BY COUNT(*) DESC, mod.model_name
		LIMIT 12`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]OperationsModelUsage, 0)
	for rows.Next() {
		var item OperationsModelUsage
		if err := rows.Scan(&item.Model, &item.Provider, &item.Requests, &item.TotalTokens, &item.TotalSpend); err != nil {
			return nil, err
		}
		item.TotalSpend = strings.TrimSpace(item.TotalSpend)
		if item.TotalSpend == "" {
			item.TotalSpend = "0"
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *SQLService) listOperationsTrend(ctx context.Context) ([]OperationsTrendPoint, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT date_trunc('day', created_at), COUNT(*)::bigint,
		       COALESCE(SUM(input_tokens), 0)::bigint,
		       COALESCE(SUM(output_tokens), 0)::bigint,
		       COALESCE(SUM(input_tokens + output_tokens), 0)::bigint,
		       COALESCE(SUM(settled_amount) FILTER (WHERE status = 'settled'), 0)::text
		FROM model_requests
		WHERE created_at >= now() - interval '30 days'
		GROUP BY 1 ORDER BY 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]OperationsTrendPoint, 0)
	for rows.Next() {
		var item OperationsTrendPoint
		if err := rows.Scan(&item.At, &item.Requests, &item.InputTokens, &item.OutputTokens, &item.TotalTokens, &item.TotalSpend); err != nil {
			return nil, err
		}
		item.At = item.At.UTC()
		item.TotalSpend = strings.TrimSpace(item.TotalSpend)
		if item.TotalSpend == "" {
			item.TotalSpend = "0"
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

var _ OperationsReporter = (*SQLService)(nil)
