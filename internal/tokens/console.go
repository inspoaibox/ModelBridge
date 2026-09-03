package tokens

import (
	"context"
	"database/sql"
	"encoding/json"
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
	SpendLimit     string
	ExpiresAt      *time.Time
	GroupID        string
}

type UpdateRequest struct {
	TokenID        string
	TenantID       string
	CreatedBy      string
	ProjectID      string
	Name           string
	AllowedModels  []string
	AllowedIPs     []string
	AllowedDomains []string
	RateLimit      any
	SpendLimit     string
	ExpiresAt      *time.Time
	GroupID        string
}

type ConsoleService interface {
	ListOwned(context.Context, string, string) ([]Summary, error)
	Create(context.Context, CreateRequest) (IssuedToken, error)
	UpdateOwned(context.Context, UpdateRequest) (Summary, error)
	SetStatusOwned(context.Context, string, string, string, string) error
	DeleteOwned(context.Context, string, string, string) error
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
		  AND t.deleted_at IS NULL
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
	return s.issuer.IssueInGroupWithSpendLimit(
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
		request.SpendLimit,
	)
}

func (s *SQLConsoleService) UpdateOwned(ctx context.Context, request UpdateRequest) (Summary, error) {
	if s == nil || s.db == nil {
		return Summary{}, ErrConsoleUnavailable
	}
	request.TokenID = strings.TrimSpace(request.TokenID)
	request.TenantID = strings.TrimSpace(request.TenantID)
	request.CreatedBy = strings.TrimSpace(request.CreatedBy)
	request.ProjectID = strings.TrimSpace(request.ProjectID)
	request.Name = strings.TrimSpace(request.Name)
	request.GroupID = strings.TrimSpace(request.GroupID)
	if !ids.Valid(request.TokenID) || !ids.Valid(request.TenantID) || !ids.Valid(request.CreatedBy) ||
		!ids.Valid(request.ProjectID) || !ids.Valid(request.GroupID) || request.Name == "" || len(request.Name) > 100 {
		return Summary{}, ErrConsoleInvalid
	}
	if request.ExpiresAt != nil && !request.ExpiresAt.After(time.Now()) {
		return Summary{}, ErrConsoleInvalid
	}
	allowedIPs, err := normalizeAllowedIPs(request.AllowedIPs)
	if err != nil {
		return Summary{}, err
	}
	allowedDomains, err := normalizeAllowedDomains(request.AllowedDomains)
	if err != nil {
		return Summary{}, err
	}
	rateLimit, err := normalizeRateLimit(request.RateLimit)
	if err != nil {
		return Summary{}, err
	}
	spendLimit, err := normalizeSpendLimit(request.SpendLimit)
	if err != nil {
		return Summary{}, err
	}
	modelsJSON, err := json.Marshal(cleanStrings(request.AllowedModels))
	if err != nil {
		return Summary{}, err
	}
	ipsJSON, err := json.Marshal(allowedIPs)
	if err != nil {
		return Summary{}, err
	}
	domainsJSON, err := json.Marshal(allowedDomains)
	if err != nil {
		return Summary{}, err
	}
	rateLimitJSON, err := json.Marshal(rateLimit)
	if err != nil {
		return Summary{}, err
	}

	var projectExists bool
	if err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM projects
			WHERE id = $1::uuid AND tenant_id = $2::uuid
			  AND status = 'active' AND deleted_at IS NULL
		)
	`, request.ProjectID, request.TenantID).Scan(&projectExists); err != nil {
		return Summary{}, err
	}
	if !projectExists {
		return Summary{}, ErrTokenNotFound
	}
	var groupExists bool
	if err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM routing_groups
			WHERE id = $1::uuid AND status = 'active' AND deleted_at IS NULL
		)
	`, request.GroupID).Scan(&groupExists); err != nil {
		return Summary{}, err
	}
	if !groupExists {
		return Summary{}, ErrGroupNotFound
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE api_tokens
		SET project_id = $4::uuid,
		    name = $5,
		    allowed_models_json = $6,
		    allowed_ips_json = $7,
		    allowed_domains_json = $8,
		    rate_limit_json = $9,
		    spend_limit = $10::numeric,
		    expires_at = $11,
		    group_id = $12::uuid
		WHERE id = $1::uuid
		  AND tenant_id = $2::uuid
		  AND created_by = $3::uuid
		  AND deleted_at IS NULL
		  AND status IN ('active', 'disabled')
	`, request.TokenID, request.TenantID, request.CreatedBy, request.ProjectID, request.Name,
		modelsJSON, ipsJSON, domainsJSON, rateLimitJSON, spendLimit, request.ExpiresAt, request.GroupID)
	if err != nil {
		return Summary{}, err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return Summary{}, err
	} else if affected == 0 {
		return Summary{}, ErrTokenNotFound
	}
	return s.ownedSummary(ctx, request.TokenID, request.TenantID, request.CreatedBy)
}

func (s *SQLConsoleService) SetStatusOwned(ctx context.Context, tokenID, tenantID, createdBy, status string) error {
	if s == nil || s.db == nil {
		return ErrConsoleUnavailable
	}
	tokenID = strings.TrimSpace(tokenID)
	tenantID = strings.TrimSpace(tenantID)
	createdBy = strings.TrimSpace(createdBy)
	status = strings.ToLower(strings.TrimSpace(status))
	if !ids.Valid(tokenID) || !ids.Valid(tenantID) || !ids.Valid(createdBy) || (status != "active" && status != "disabled" && status != "revoked") {
		return ErrConsoleInvalid
	}
	var query string
	switch status {
	case "active":
		query = `
			UPDATE api_tokens
			SET status = 'active'
			WHERE id = $1::uuid AND tenant_id = $2::uuid AND created_by = $3::uuid
			  AND deleted_at IS NULL AND status = 'disabled'
			  AND (expires_at IS NULL OR expires_at > now())`
	case "disabled":
		query = `
			UPDATE api_tokens
			SET status = 'disabled'
			WHERE id = $1::uuid AND tenant_id = $2::uuid AND created_by = $3::uuid
			  AND deleted_at IS NULL AND status = 'active'
			  AND (expires_at IS NULL OR expires_at > now())`
	case "revoked":
		query = `
			UPDATE api_tokens
			SET status = 'revoked', revoked_at = COALESCE(revoked_at, now())
			WHERE id = $1::uuid AND tenant_id = $2::uuid AND created_by = $3::uuid
			  AND deleted_at IS NULL AND status IN ('active', 'disabled')`
	}
	result, err := s.db.ExecContext(ctx, query, tokenID, tenantID, createdBy)
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

func (s *SQLConsoleService) DeleteOwned(ctx context.Context, tokenID, tenantID, createdBy string) error {
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
		SET deleted_at = COALESCE(deleted_at, now()),
		    status = 'revoked',
		    revoked_at = COALESCE(revoked_at, now())
		WHERE id = $1::uuid
		  AND tenant_id = $2::uuid
		  AND created_by = $3::uuid
		  AND deleted_at IS NULL
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

func (s *SQLConsoleService) ownedSummary(ctx context.Context, tokenID, tenantID, createdBy string) (Summary, error) {
	items, err := listSummaries(ctx, s.db, `
		WHERE t.id = $1::uuid
		  AND t.tenant_id = $2::uuid
		  AND t.created_by = $3::uuid
		  AND t.deleted_at IS NULL
	`, tokenID, tenantID, createdBy)
	if err != nil {
		return Summary{}, err
	}
	if len(items) == 0 {
		return Summary{}, ErrTokenNotFound
	}
	return items[0], nil
}

func listSummaries(ctx context.Context, db *sql.DB, predicate string, args ...any) ([]Summary, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT t.id::text, t.name, t.token_prefix, t.tenant_id::text,
		       t.project_id::text, COALESCE(t.group_id::text, ''),
		       COALESCE(rg.code, ''),
		       CASE WHEN t.status IN ('active', 'disabled') AND t.expires_at IS NOT NULL AND t.expires_at <= now()
		            THEN 'expired' ELSE t.status END,
		       jsonb_array_length(COALESCE(t.allowed_ips_json, '[]'::jsonb)),
		       jsonb_array_length(COALESCE(t.allowed_domains_json, '[]'::jsonb)),
		       t.allowed_models_json, t.allowed_ips_json, t.allowed_domains_json,
		       t.rate_limit_json, t.spend_limit::text, t.spent_amount::text, t.expires_at,
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
			allowedModelsRaw   []byte
			allowedIPsRaw      []byte
			allowedDomainsRaw  []byte
			rateLimitRaw       []byte
			spendLimit         string
			spentAmount        string
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
			&allowedModelsRaw,
			&allowedIPsRaw,
			&allowedDomainsRaw,
			&rateLimitRaw,
			&spendLimit,
			&spentAmount,
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
		_ = json.Unmarshal(allowedModelsRaw, &item.AllowedModels)
		_ = json.Unmarshal(allowedIPsRaw, &item.AllowedIPs)
		_ = json.Unmarshal(allowedDomainsRaw, &item.AllowedDomains)
		_ = json.Unmarshal(rateLimitRaw, &item.RateLimit)
		item.SpendLimit = spendLimit
		item.SpentAmount = spentAmount
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
