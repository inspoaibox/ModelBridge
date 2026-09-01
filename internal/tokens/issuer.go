package tokens

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"ai-token/internal/ids"
)

type Issuer struct {
	db     *sql.DB
	hasher *Hasher
}

type IssuedToken struct {
	ID        string
	Plaintext string
	Prefix    string
	ExpiresAt *time.Time
}

func NewIssuer(db *sql.DB, hasher *Hasher) (*Issuer, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	if hasher == nil {
		return nil, errors.New("token hasher is required")
	}
	return &Issuer{db: db, hasher: hasher}, nil
}

func (i *Issuer) Issue(
	ctx context.Context,
	tenantID string,
	projectID string,
	createdBy string,
	name string,
	scopes []string,
	allowedModels []string,
	allowedIPs []string,
	rateLimit any,
	expiresAt *time.Time,
) (IssuedToken, error) {
	return i.IssueInGroup(ctx, tenantID, projectID, createdBy, name, scopes, allowedModels, allowedIPs, nil, rateLimit, expiresAt, "")
}

func (i *Issuer) IssueInGroup(
	ctx context.Context,
	tenantID string,
	projectID string,
	createdBy string,
	name string,
	scopes []string,
	allowedModels []string,
	allowedIPs []string,
	allowedDomains []string,
	rateLimit any,
	expiresAt *time.Time,
	groupID string,
) (IssuedToken, error) {
	if i == nil || i.db == nil || i.hasher == nil {
		return IssuedToken{}, errors.New("token issuer is not configured")
	}
	name = strings.TrimSpace(name)
	if !ids.Valid(tenantID) || !ids.Valid(projectID) || !ids.Valid(createdBy) || name == "" || len(name) > 100 {
		return IssuedToken{}, errors.New("token scope and name are required")
	}
	if expiresAt != nil && !expiresAt.After(time.Now()) {
		return IssuedToken{}, errors.New("token expiry must be in the future")
	}
	allowedIPs, err := normalizeAllowedIPs(allowedIPs)
	if err != nil {
		return IssuedToken{}, err
	}
	allowedDomains, err = normalizeAllowedDomains(allowedDomains)
	if err != nil {
		return IssuedToken{}, err
	}
	if strings.TrimSpace(groupID) != "" {
		if !ids.Valid(groupID) {
			return IssuedToken{}, ErrGroupNotFound
		}
		var groupExists bool
		if err := i.db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM routing_groups
				WHERE id = $1::uuid AND status = 'active' AND deleted_at IS NULL
			)
		`, strings.TrimSpace(groupID)).Scan(&groupExists); err != nil {
			return IssuedToken{}, err
		}
		if !groupExists {
			return IssuedToken{}, ErrGroupNotFound
		}
	}

	var projectExists bool
	if err := i.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM projects
			WHERE id = $1::uuid
			  AND tenant_id = $2::uuid
			  AND status = 'active'
			  AND deleted_at IS NULL
		)
	`, projectID, tenantID).Scan(&projectExists); err != nil {
		return IssuedToken{}, err
	}
	if !projectExists {
		return IssuedToken{}, errors.New("token project is not available")
	}
	var creatorCanIssue bool
	if err := i.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM users u
			JOIN tenant_members tm ON tm.user_id = u.id
			JOIN projects p ON p.tenant_id = tm.tenant_id
			WHERE u.id = $1::uuid
			  AND u.status = 'active' AND u.deleted_at IS NULL
			  AND tm.tenant_id = $2::uuid AND tm.status = 'active'
			  AND p.id = $3::uuid AND p.status = 'active' AND p.deleted_at IS NULL
			  AND (tm.role_code IN ('tenant_owner', 'tenant_admin') OR EXISTS (
				  SELECT 1 FROM project_members pm
				  WHERE pm.project_id = p.id AND pm.user_id = u.id
				    AND pm.role_code IN ('project_admin', 'developer')
				))
		)
	`, createdBy, tenantID, projectID).Scan(&creatorCanIssue); err != nil {
		return IssuedToken{}, err
	}
	if !creatorCanIssue {
		return IssuedToken{}, errors.New("token creator is not an active tenant member")
	}

	plain, prefix, digest, err := i.hasher.Generate()
	if err != nil {
		return IssuedToken{}, err
	}
	tokenID, err := ids.New()
	if err != nil {
		return IssuedToken{}, err
	}
	scopesJSON, err := json.Marshal(cleanStrings(scopes))
	if err != nil {
		return IssuedToken{}, err
	}
	modelsJSON, err := json.Marshal(cleanStrings(allowedModels))
	if err != nil {
		return IssuedToken{}, err
	}
	ipsJSON, err := json.Marshal(cleanStrings(allowedIPs))
	if err != nil {
		return IssuedToken{}, err
	}
	domainsJSON, err := json.Marshal(cleanStrings(allowedDomains))
	if err != nil {
		return IssuedToken{}, err
	}
	normalizedRateLimit, err := normalizeRateLimit(rateLimit)
	if err != nil {
		return IssuedToken{}, err
	}
	rateLimitJSON, err := json.Marshal(normalizedRateLimit)
	if err != nil {
		return IssuedToken{}, err
	}

	var groupValue any
	if strings.TrimSpace(groupID) != "" {
		groupValue = strings.TrimSpace(groupID)
	} else {
		var defaultGroupID string
		if err := i.db.QueryRowContext(ctx, `SELECT id::text FROM routing_groups WHERE code = 'default' AND status = 'active' AND deleted_at IS NULL LIMIT 1`).Scan(&defaultGroupID); errors.Is(err, sql.ErrNoRows) {
			return IssuedToken{}, ErrGroupNotFound
		} else if err != nil {
			return IssuedToken{}, err
		}
		groupValue = defaultGroupID
	}
	_, err = i.db.ExecContext(ctx, `
		INSERT INTO api_tokens (
			id, tenant_id, project_id, created_by, name, token_prefix,
			 token_hash, scopes_json, allowed_models_json, allowed_ips_json,
			allowed_domains_json, rate_limit_json, expires_at, group_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14::uuid)
	`, tokenID, tenantID, projectID, createdBy, name, prefix, digest,
		scopesJSON, modelsJSON, ipsJSON, domainsJSON, rateLimitJSON, expiresAt, groupValue)
	if err != nil {
		return IssuedToken{}, err
	}

	return IssuedToken{
		ID:        tokenID,
		Plaintext: plain,
		Prefix:    prefix,
		ExpiresAt: expiresAt,
	}, nil
}

func normalizeRateLimit(value any) (map[string]int64, error) {
	if value == nil {
		return map[string]int64{}, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, errors.New("invalid token rate limit")
	}
	var input map[string]json.RawMessage
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, errors.New("invalid token rate limit")
	}
	result := map[string]int64{}
	aliases := map[string][]string{
		"rpm":         {"rpm", "requests_per_minute"},
		"tpm":         {"tpm", "tokens_per_minute"},
		"concurrency": {"concurrency", "max_concurrent"},
	}
	for canonical, keys := range aliases {
		for _, key := range keys {
			message, ok := input[key]
			if !ok {
				continue
			}
			var number json.Number
			if err := json.Unmarshal(message, &number); err != nil {
				return nil, errors.New("invalid token rate limit")
			}
			parsed, err := number.Int64()
			if err != nil || parsed < 0 || parsed > 10_000_000 {
				return nil, errors.New("invalid token rate limit")
			}
			result[canonical] = parsed
			break
		}
	}
	return result, nil
}

// AcquireToken applies the configured RPM, TPM and concurrency limits. The
// callback is idempotent and must be called when the request has finished.
func (i *Issuer) AcquireToken(ctx context.Context, tokenID string, estimatedTokens int64) (func(), error) {
	if i == nil || i.db == nil {
		return nil, errors.New("token limiter is not configured")
	}
	tokenID = strings.TrimSpace(tokenID)
	if !ids.Valid(tokenID) {
		return nil, errors.New("token is required")
	}
	if estimatedTokens < 1 {
		estimatedTokens = 1
	}
	var raw []byte
	if err := i.db.QueryRowContext(ctx, `
		SELECT rate_limit_json FROM api_tokens
		WHERE id = $1::uuid AND status = 'active'
	`, tokenID).Scan(&raw); err != nil {
		return nil, err
	}
	limits, err := normalizeRateLimitJSON(raw)
	if err != nil {
		return nil, err
	}
	if limits.RPM == 0 && limits.TPM == 0 && limits.Concurrency == 0 {
		return func() {}, nil
	}
	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if limits.RPM > 0 || limits.TPM > 0 {
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM api_token_rate_windows
			WHERE token_id = $1::uuid
			  AND window_start < date_trunc('minute', now()) - interval '2 minutes'
		`, tokenID); err != nil {
			return nil, err
		}
		var allowed bool
		if err := tx.QueryRowContext(ctx, `
			WITH consumed AS (
				INSERT INTO api_token_rate_windows (token_id, window_start, request_count, token_count)
				VALUES ($1::uuid, date_trunc('minute', now()), 1, $2)
				ON CONFLICT (token_id, window_start) DO UPDATE
				SET request_count = api_token_rate_windows.request_count + 1,
				    token_count = api_token_rate_windows.token_count + $2
				WHERE ($3 = 0 OR api_token_rate_windows.request_count < $3)
				  AND ($4 = 0 OR api_token_rate_windows.token_count + $2 <= $4)
				RETURNING 1
			)
			SELECT EXISTS (SELECT 1 FROM consumed)
		`, tokenID, estimatedTokens, limits.RPM, limits.TPM).Scan(&allowed); err != nil {
			return nil, err
		} else if !allowed {
			return nil, ErrTokenRateLimited
		}
	}
	if limits.Concurrency > 0 {
		var allowed bool
		if err := tx.QueryRowContext(ctx, `
			WITH acquired AS (
				INSERT INTO api_token_concurrency (token_id, active_count)
				VALUES ($1::uuid, 1)
				ON CONFLICT (token_id) DO UPDATE
				SET active_count = api_token_concurrency.active_count + 1
				WHERE api_token_concurrency.active_count < $2
				RETURNING 1
			)
			SELECT EXISTS (SELECT 1 FROM acquired)
		`, tokenID, limits.Concurrency).Scan(&allowed); err != nil {
			return nil, err
		} else if !allowed {
			return nil, ErrTokenRateLimited
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			if limits.Concurrency > 0 {
				_, _ = i.db.ExecContext(context.Background(), `
					UPDATE api_token_concurrency
					SET active_count = GREATEST(active_count - 1, 0)
					WHERE token_id = $1::uuid
				`, tokenID)
			}
		})
	}, nil
}

type rateLimitPolicy struct {
	RPM         int64
	TPM         int64
	Concurrency int64
}

func normalizeRateLimitJSON(raw []byte) (rateLimitPolicy, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return rateLimitPolicy{}, nil
	}
	var input map[string]json.RawMessage
	if err := json.Unmarshal(raw, &input); err != nil {
		return rateLimitPolicy{}, errors.New("invalid token rate limit")
	}
	result := rateLimitPolicy{}
	for canonical, target := range map[string]*int64{
		"rpm": &result.RPM, "tpm": &result.TPM, "concurrency": &result.Concurrency,
	} {
		message, ok := input[canonical]
		if !ok {
			continue
		}
		var number json.Number
		if err := json.Unmarshal(message, &number); err != nil {
			return rateLimitPolicy{}, fmt.Errorf("invalid token rate limit: %w", err)
		}
		parsed, err := number.Int64()
		if err != nil || parsed < 0 {
			return rateLimitPolicy{}, errors.New("invalid token rate limit")
		}
		*target = parsed
	}
	return result, nil
}

func (i *Issuer) Revoke(ctx context.Context, tokenID, tenantID string) error {
	if i == nil || i.db == nil {
		return errors.New("token issuer is not configured")
	}
	if tokenID == "" || tenantID == "" {
		return errors.New("token and tenant are required")
	}
	_, err := i.db.ExecContext(ctx, `
		UPDATE api_tokens
		SET status = 'revoked', revoked_at = COALESCE(revoked_at, now())
		WHERE id = $1 AND tenant_id = $2 AND status <> 'revoked'
	`, tokenID, tenantID)
	return err
}

func cleanStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
