package tokens

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"ai-token/internal/ids"
)

var (
	ErrAdminUnavailable       = errors.New("token admin service is unavailable")
	ErrTokenNotFound          = errors.New("api token is not found")
	ErrGroupNotFound          = errors.New("token group is not found")
	ErrAdminInvalid           = errors.New("invalid token admin request")
	ErrTokenRateLimited       = errors.New("api token rate limit exceeded")
	ErrTokenSpendLimitInvalid = errors.New("invalid api token spend limit")
)

type Summary struct {
	ID                      string           `json:"id"`
	Name                    string           `json:"name"`
	TokenPrefix             string           `json:"token_prefix"`
	CreatedBy               string           `json:"created_by,omitempty"`
	TenantID                string           `json:"tenant_id"`
	ProjectID               string           `json:"project_id"`
	GroupID                 string           `json:"group_id,omitempty"`
	GroupCode               string           `json:"group_code,omitempty"`
	Status                  string           `json:"status"`
	NetworkAllowlistEnabled bool             `json:"network_allowlist_enabled"`
	AllowedIPCount          int              `json:"allowed_ip_count"`
	AllowedDomainCount      int              `json:"allowed_domain_count"`
	AllowedModels           []string         `json:"allowed_models,omitempty"`
	AllowedIPs              []string         `json:"allowed_ips,omitempty"`
	AllowedDomains          []string         `json:"allowed_domains,omitempty"`
	RateLimit               map[string]int64 `json:"rate_limit,omitempty"`
	SpendLimit              string           `json:"spend_limit"`
	SpentAmount             string           `json:"spent_amount"`
	ExpiresAt               *time.Time       `json:"expires_at,omitempty"`
	LastUsedAt              *time.Time       `json:"last_used_at,omitempty"`
	CreatedAt               time.Time        `json:"created_at"`
}

type AdminService interface {
	List(context.Context) ([]Summary, error)
	Pause(context.Context, string) (Summary, error)
}

type SQLAdminService struct {
	db *sql.DB
}

func NewAdminService(db *sql.DB, _ ...*Issuer) (*SQLAdminService, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &SQLAdminService{db: db}, nil
}

func (s *SQLAdminService) List(ctx context.Context) ([]Summary, error) {
	if s == nil || s.db == nil {
		return nil, ErrAdminUnavailable
	}
	return listSummaries(ctx, s.db, "WHERE t.deleted_at IS NULL")
}

func (s *SQLAdminService) Pause(ctx context.Context, tokenID string) (Summary, error) {
	if s == nil || s.db == nil {
		return Summary{}, ErrAdminUnavailable
	}
	tokenID = strings.TrimSpace(tokenID)
	if !ids.Valid(tokenID) {
		return Summary{}, ErrAdminInvalid
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE api_tokens
		SET status = 'disabled'
		WHERE id = $1::uuid
		  AND deleted_at IS NULL
		  AND status = 'active'
		  AND (expires_at IS NULL OR expires_at > now())
	`, tokenID)
	if err != nil {
		return Summary{}, err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return Summary{}, err
	} else if affected == 0 {
		return Summary{}, ErrTokenNotFound
	}
	items, err := s.List(ctx)
	if err != nil {
		return Summary{}, err
	}
	for _, item := range items {
		if item.ID == tokenID {
			return item, nil
		}
	}
	return Summary{}, ErrTokenNotFound
}
