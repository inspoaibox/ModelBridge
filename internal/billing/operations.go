package billing

import (
	"context"
	"errors"
	"time"
)

type OperationsSnapshot struct {
	Users             int64     `json:"users"`
	Tenants           int64     `json:"tenants"`
	Channels          int64     `json:"channels"`
	ActiveChannels    int64     `json:"active_channels"`
	Groups            int64     `json:"groups"`
	ActiveGroups      int64     `json:"active_groups"`
	Tokens            int64     `json:"tokens"`
	ActiveTokens      int64     `json:"active_tokens"`
	Requests24h       int64     `json:"requests_24h"`
	FailedRequests24h int64     `json:"failed_requests_24h"`
	Spend24h          string    `json:"spend_24h"`
	AverageLatencyMS  float64   `json:"average_latency_ms"`
	CollectedAt       time.Time `json:"collected_at"`
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
			(SELECT COUNT(*) FROM api_tokens WHERE status <> 'revoked'),
			(SELECT COUNT(*) FROM api_tokens WHERE status = 'active' AND (expires_at IS NULL OR expires_at > now())),
			(SELECT COUNT(*) FROM model_requests WHERE created_at >= now() - interval '24 hours'),
			(SELECT COUNT(*) FROM model_requests WHERE status = 'failed' AND created_at >= now() - interval '24 hours'),
			(SELECT COALESCE(SUM(settled_amount), 0)::text FROM model_requests
				WHERE status = 'settled' AND created_at >= now() - interval '24 hours'),
			(SELECT COALESCE(AVG(latency_ms) FILTER (WHERE latency_ms > 0), 0) FROM model_requests
				WHERE created_at >= now() - interval '24 hours')
	`).Scan(
		&snapshot.Users, &snapshot.Tenants, &snapshot.Channels, &snapshot.ActiveChannels,
		&snapshot.Groups, &snapshot.ActiveGroups, &snapshot.Tokens, &snapshot.ActiveTokens,
		&snapshot.Requests24h, &snapshot.FailedRequests24h, &snapshot.Spend24h,
		&snapshot.AverageLatencyMS,
	)
	if err != nil {
		return OperationsSnapshot{}, err
	}
	snapshot.CollectedAt = time.Now().UTC()
	return snapshot, nil
}

var _ OperationsReporter = (*SQLService)(nil)
