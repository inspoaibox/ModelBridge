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
	ErrAdminUnavailable = errors.New("token admin service is unavailable")
	ErrTokenNotFound    = errors.New("api token is not found")
	ErrGroupNotFound    = errors.New("token group is not found")
	ErrAdminInvalid     = errors.New("invalid token admin request")
	ErrTokenRateLimited = errors.New("api token rate limit exceeded")
)

type Summary struct {
	ID                      string     `json:"id"`
	Name                    string     `json:"name"`
	TokenPrefix             string     `json:"token_prefix"`
	CreatedBy               string     `json:"created_by,omitempty"`
	TenantID                string     `json:"tenant_id"`
	ProjectID               string     `json:"project_id"`
	GroupID                 string     `json:"group_id,omitempty"`
	GroupCode               string     `json:"group_code,omitempty"`
	Status                  string     `json:"status"`
	NetworkAllowlistEnabled bool       `json:"network_allowlist_enabled"`
	AllowedIPCount          int        `json:"allowed_ip_count"`
	AllowedDomainCount      int        `json:"allowed_domain_count"`
	ExpiresAt               *time.Time `json:"expires_at,omitempty"`
	LastUsedAt              *time.Time `json:"last_used_at,omitempty"`
	CreatedAt               time.Time  `json:"created_at"`
}

type AdminService interface {
	List(context.Context) ([]Summary, error)
	SetGroup(context.Context, string, string) (Summary, error)
}

type AdminCreator interface {
	Create(context.Context, CreateRequest) (IssuedToken, error)
}

type AdminRevoker interface {
	Revoke(context.Context, string) error
}

type SQLAdminService struct {
	db     *sql.DB
	issuer *Issuer
}

func NewAdminService(db *sql.DB, issuers ...*Issuer) (*SQLAdminService, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	var issuer *Issuer
	if len(issuers) > 0 {
		issuer = issuers[0]
	}
	return &SQLAdminService{db: db, issuer: issuer}, nil
}

func (s *SQLAdminService) List(ctx context.Context) ([]Summary, error) {
	if s == nil || s.db == nil {
		return nil, ErrAdminUnavailable
	}
	return listSummaries(ctx, s.db, "")
}

func (s *SQLAdminService) SetGroup(ctx context.Context, tokenID, groupID string) (Summary, error) {
	if s == nil || s.db == nil {
		return Summary{}, ErrAdminUnavailable
	}
	tokenID = strings.TrimSpace(tokenID)
	groupID = strings.TrimSpace(groupID)
	if !ids.Valid(tokenID) || !ids.Valid(groupID) {
		return Summary{}, ErrAdminInvalid
	}

	var exists bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM api_tokens WHERE id = $1::uuid)`, tokenID).Scan(&exists); err != nil {
		return Summary{}, err
	}
	if !exists {
		return Summary{}, ErrTokenNotFound
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM routing_groups
			WHERE id = $1::uuid AND status = 'active' AND deleted_at IS NULL
		)
	`, groupID).Scan(&exists); err != nil {
		return Summary{}, err
	}
	if !exists {
		return Summary{}, ErrGroupNotFound
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE api_tokens SET group_id = $2::uuid WHERE id = $1::uuid`, tokenID, groupID); err != nil {
		return Summary{}, err
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

func (s *SQLAdminService) Create(ctx context.Context, request CreateRequest) (IssuedToken, error) {
	if s == nil || s.db == nil || s.issuer == nil {
		return IssuedToken{}, ErrAdminUnavailable
	}
	if !ids.Valid(request.TenantID) || !ids.Valid(request.ProjectID) || !ids.Valid(request.CreatedBy) || strings.TrimSpace(request.Name) == "" {
		return IssuedToken{}, ErrAdminInvalid
	}
	return s.issuer.IssueInGroup(
		ctx,
		request.TenantID,
		request.ProjectID,
		request.CreatedBy,
		request.Name,
		[]string{"model:use"},
		request.AllowedModels,
		request.AllowedIPs,
		request.AllowedDomains,
		request.RateLimit,
		request.ExpiresAt,
		request.GroupID,
	)
}

func (s *SQLAdminService) Revoke(ctx context.Context, tokenID string) error {
	if s == nil || s.db == nil {
		return ErrAdminUnavailable
	}
	tokenID = strings.TrimSpace(tokenID)
	if !ids.Valid(tokenID) {
		return ErrAdminInvalid
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE api_tokens
		SET status = 'revoked', revoked_at = COALESCE(revoked_at, now())
		WHERE id = $1::uuid AND status <> 'revoked'
	`, tokenID)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected == 0 {
		return ErrTokenNotFound
	}
	return nil
}
