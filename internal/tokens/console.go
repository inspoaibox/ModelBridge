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
	ErrConsoleUnavailable = errors.New("token console service is unavailable")
	ErrConsoleInvalid     = errors.New("invalid token console request")
)

type CreateRequest struct {
	TenantID       string
	ProjectID      string
	CreatedBy      string
	Name           string
	AllowedModels  []string
	AllowedIPs     []string
	AllowedDomains []string
	RateLimit      any
	ExpiresAt      *time.Time
	GroupID        string
}

type ConsoleService interface {
	ListOwned(context.Context, string, string) ([]Summary, error)
	Create(context.Context, CreateRequest) (IssuedToken, error)
	RevokeOwned(context.Context, string, string, string) error
}

type SQLConsoleService struct {
	db     *sql.DB
	issuer *Issuer
}

func NewConsoleService(db *sql.DB, issuer *Issuer) (*SQLConsoleService, error) {
	if db == nil || issuer == nil {
		return nil, errors.New("database and token issuer are required")
	}
	return &SQLConsoleService{db: db, issuer: issuer}, nil
}

func (s *SQLConsoleService) ListOwned(ctx context.Context, tenantID, createdBy string) ([]Summary, error) {
	if s == nil || s.db == nil {
		return nil, ErrConsoleUnavailable
	}
	tenantID = strings.TrimSpace(tenantID)
	createdBy = strings.TrimSpace(createdBy)
	if !ids.Valid(tenantID) || !ids.Valid(createdBy) {
		return nil, ErrConsoleInvalid
	}
	return listSummaries(ctx, s.db, `
		WHERE t.tenant_id = $1::uuid
		  AND t.created_by = $2::uuid
	`, tenantID, createdBy)
}

func (s *SQLConsoleService) Create(ctx context.Context, request CreateRequest) (IssuedToken, error) {
	if s == nil || s.issuer == nil {
		return IssuedToken{}, ErrConsoleUnavailable
	}
	request.TenantID = strings.TrimSpace(request.TenantID)
	request.ProjectID = strings.TrimSpace(request.ProjectID)
	request.CreatedBy = strings.TrimSpace(request.CreatedBy)
	request.Name = strings.TrimSpace(request.Name)
	if !ids.Valid(request.TenantID) || !ids.Valid(request.ProjectID) || !ids.Valid(request.CreatedBy) || request.Name == "" || len(request.Name) > 100 {
		return IssuedToken{}, ErrConsoleInvalid
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

func (s *SQLConsoleService) RevokeOwned(ctx context.Context, tokenID, tenantID, createdBy string) error {
	if s == nil || s.db == nil {
		return ErrConsoleUnavailable
	}
	tokenID = strings.TrimSpace(tokenID)
	tenantID = strings.TrimSpace(tenantID)
	createdBy = strings.TrimSpace(createdBy)
	if !ids.Valid(tokenID) || !ids.Valid(tenantID) || !ids.Valid(createdBy) {
		return ErrConsoleInvalid
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE api_tokens
		SET status = 'revoked', revoked_at = COALESCE(revoked_at, now())
		WHERE id = $1::uuid
		  AND tenant_id = $2::uuid
		  AND created_by = $3::uuid
		  AND status <> 'revoked'
	`, tokenID, tenantID, createdBy)
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

func listSummaries(ctx context.Context, db *sql.DB, predicate string, args ...any) ([]Summary, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT t.id::text, t.name, t.token_prefix, t.tenant_id::text,
		       t.project_id::text, COALESCE(t.group_id::text, ''),
		       COALESCE(rg.code, ''), t.status,
		       jsonb_array_length(COALESCE(t.allowed_ips_json, '[]'::jsonb)),
		       jsonb_array_length(COALESCE(t.allowed_domains_json, '[]'::jsonb)),
		       t.expires_at,
		       t.last_used_at, t.created_at, t.created_by::text
		FROM api_tokens t
		LEFT JOIN routing_groups rg ON rg.id = t.group_id AND rg.deleted_at IS NULL
	`+predicate+`
		ORDER BY t.created_at DESC, t.id DESC
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Summary, 0)
	for rows.Next() {
		var (
			item               Summary
			expiresAt          sql.NullTime
			lastUsedAt         sql.NullTime
			allowedIPCount     int
			allowedDomainCount int
		)
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.TokenPrefix,
			&item.TenantID,
			&item.ProjectID,
			&item.GroupID,
			&item.GroupCode,
			&item.Status,
			&allowedIPCount,
			&allowedDomainCount,
			&expiresAt,
			&lastUsedAt,
			&item.CreatedAt,
			&item.CreatedBy,
		); err != nil {
			return nil, err
		}
		if expiresAt.Valid {
			value := expiresAt.Time
			item.ExpiresAt = &value
		}
		if lastUsedAt.Valid {
			value := lastUsedAt.Time
			item.LastUsedAt = &value
		}
		item.AllowedIPCount = allowedIPCount
		item.AllowedDomainCount = allowedDomainCount
		item.NetworkAllowlistEnabled = allowedIPCount > 0 || allowedDomainCount > 0
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
