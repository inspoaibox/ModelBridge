package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"ai-token/internal/tokens"
)

type SQLResolver struct {
	db            *sql.DB
	tokenHasher   *tokens.Hasher
	sessionHasher *tokens.Hasher
}

func NewSQLResolver(db *sql.DB, tokenHasher, sessionHasher *tokens.Hasher) (*SQLResolver, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	if tokenHasher == nil || sessionHasher == nil {
		return nil, errors.New("token and session hashers are required")
	}
	return &SQLResolver{
		db:            db,
		tokenHasher:   tokenHasher,
		sessionHasher: sessionHasher,
	}, nil
}

func (r *SQLResolver) Resolve(ctx context.Context, credential string) (*Principal, error) {
	if r == nil || r.db == nil || r.tokenHasher == nil {
		return nil, errors.New("token resolver is not configured")
	}

	var (
		id             string
		tenantID       string
		projectID      string
		groupID        sql.NullString
		tokenScopes    []byte
		allowedModels  []byte
		allowedIPs     []byte
		allowedDomains []byte
		status         string
		expiresAt      sql.NullTime
	)
	err := r.db.QueryRowContext(ctx, `
		SELECT tok.id::text, tok.tenant_id::text, tok.project_id::text,
		       COALESCE(tok.group_id::text, (
		           SELECT id::text FROM routing_groups
		           WHERE code = 'default' AND status = 'active' AND deleted_at IS NULL
		           LIMIT 1
		       )),
		       tok.scopes_json, tok.allowed_models_json, tok.allowed_ips_json, tok.allowed_domains_json,
		       tok.status, tok.expires_at
		FROM api_tokens tok
		JOIN users creator ON creator.id = tok.created_by
		JOIN tenant_members creator_member ON creator_member.tenant_id = tok.tenant_id
		  AND creator_member.user_id = tok.created_by
		  AND creator_member.status = 'active'
		JOIN tenants t ON t.id = tok.tenant_id
		JOIN projects p ON p.id = tok.project_id AND p.tenant_id = tok.tenant_id
		WHERE tok.token_hash = $1
		  AND tok.status = 'active'
		  AND creator.status = 'active'
		  AND creator.deleted_at IS NULL
		  AND (tok.expires_at IS NULL OR tok.expires_at > now())
		  AND t.status = 'active'
		  AND t.deleted_at IS NULL
		  AND p.status = 'active'
		  AND p.deleted_at IS NULL
		  AND (
		      creator_member.role_code IN ('tenant_owner', 'tenant_admin')
		      OR EXISTS (
		          SELECT 1
		          FROM project_members creator_project_member
		          WHERE creator_project_member.project_id = tok.project_id
		            AND creator_project_member.user_id = tok.created_by
		            AND creator_project_member.role_code IN ('project_admin', 'developer')
		      )
		  )
	`, r.tokenHasher.Digest(credential)).Scan(
		&id, &tenantID, &projectID, &groupID, &tokenScopes, &allowedModels, &allowedIPs, &allowedDomains, &status, &expiresAt,
	)
	if err != nil {
		return nil, err
	}
	if status != "active" || (expiresAt.Valid && !expiresAt.Time.After(timeNow())) {
		return nil, sql.ErrNoRows
	}
	_, _ = r.db.ExecContext(ctx, `
		UPDATE api_tokens SET last_used_at = now() WHERE id = $1::uuid AND status = 'active'
	`, id)

	scopes, err := stringSet(tokenScopes)
	if err != nil {
		return nil, fmt.Errorf("decode token scopes: %w", err)
	}
	models, err := stringSet(allowedModels)
	if err != nil {
		return nil, fmt.Errorf("decode token model allowlist: %w", err)
	}
	ips, err := stringSet(allowedIPs)
	if err != nil {
		return nil, fmt.Errorf("decode token ip allowlist: %w", err)
	}
	domains, err := stringSet(allowedDomains)
	if err != nil {
		return nil, fmt.Errorf("decode token domain allowlist: %w", err)
	}

	return &Principal{
		ID:             id,
		Type:           PrincipalAPIToken,
		Audience:       AudienceRelay,
		TenantID:       tenantID,
		GroupID:        groupID.String,
		ProjectIDs:     map[string]struct{}{projectID: {}},
		ProjectRoles:   map[string]string{projectID: "developer"},
		Scopes:         scopes,
		Permissions:    map[string]struct{}{},
		AllowedModels:  models,
		AllowedIPs:     ips,
		AllowedDomains: domains,
		TokenID:        id,
	}, nil
}

func (r *SQLResolver) ResolveSession(ctx context.Context, sessionID string, audience Audience) (*Principal, error) {
	if r == nil || r.db == nil || r.sessionHasher == nil {
		return nil, errors.New("session resolver is not configured")
	}
	if audience != AudienceAdmin && audience != AudienceConsole {
		return nil, errors.New("unsupported session audience")
	}

	var (
		sessionDBID  string
		userID       string
		tenantID     sql.NullString
		tenantStatus sql.NullString
		authStrength string
		userStatus   string
	)
	err := r.db.QueryRowContext(ctx, `
		SELECT ws.id::text, ws.user_id::text, ws.tenant_id::text,
		       ws.auth_strength, u.status, t.status
		FROM web_sessions ws
		JOIN users u ON u.id = ws.user_id
		LEFT JOIN tenants t ON t.id = ws.tenant_id
		WHERE ws.session_hash = $1
		  AND ws.audience = $2
		  AND ws.revoked_at IS NULL
		  AND ws.expires_at > now()
		  AND u.status = 'active'
		  AND u.deleted_at IS NULL
		  AND (ws.audience <> 'console' OR (t.status = 'active' AND t.deleted_at IS NULL))
	`, r.sessionHasher.Digest(sessionID), string(audience)).Scan(
		&sessionDBID, &userID, &tenantID, &authStrength, &userStatus, &tenantStatus,
	)
	if err != nil {
		return nil, err
	}
	if userStatus != "active" {
		return nil, sql.ErrNoRows
	}

	principal := &Principal{
		ID:           userID,
		Type:         PrincipalTenantUser,
		Audience:     audience,
		Permissions:  map[string]struct{}{},
		Scopes:       map[string]struct{}{},
		SessionID:    sessionDBID,
		AuthStrength: authStrength,
	}

	switch audience {
	case AudienceAdmin:
		principal.Type = PrincipalPlatformUser
		if err := r.loadPlatformAccess(ctx, principal); err != nil {
			return nil, err
		}
	case AudienceConsole:
		if !tenantID.Valid || tenantID.String == "" || !tenantStatus.Valid || tenantStatus.String != "active" {
			return nil, errors.New("console session has no tenant")
		}
		principal.TenantID = tenantID.String
		if err := r.loadTenantAccess(ctx, principal); err != nil {
			return nil, err
		}
	}

	return principal, nil
}

func (r *SQLResolver) loadPlatformAccess(ctx context.Context, principal *Principal) error {
	rows, err := r.db.QueryContext(ctx, `
		SELECT r.code, p.resource || ':' || p.action
		FROM platform_user_roles ur
		JOIN platform_roles r ON r.id = ur.role_id AND r.status = 'active'
		JOIN platform_role_permissions rp ON rp.role_id = r.id
		JOIN platform_permissions p ON p.id = rp.permission_id
		WHERE ur.user_id = $1
	`, principal.ID)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var role, permission string
		if err := rows.Scan(&role, &permission); err != nil {
			return err
		}
		principal.Roles = appendUnique(principal.Roles, role)
		principal.Permissions[permission] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(principal.Roles) == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *SQLResolver) loadTenantAccess(ctx context.Context, principal *Principal) error {
	var role string
	if err := r.db.QueryRowContext(ctx, `
		SELECT role_code
		FROM tenant_members
		WHERE tenant_id = $1
		  AND user_id = $2
		  AND status = 'active'
	`, principal.TenantID, principal.ID).Scan(&role); err != nil {
		return err
	}

	principal.Roles = []string{role}
	for _, permission := range tenantRolePermissions[role] {
		principal.Permissions[permission] = struct{}{}
	}

	if role == "tenant_owner" || role == "tenant_admin" {
		rows, err := r.db.QueryContext(ctx, `
			SELECT id::text
			FROM projects
			WHERE tenant_id = $1 AND status = 'active' AND deleted_at IS NULL
		`, principal.TenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var projectID string
			if err := rows.Scan(&projectID); err != nil {
				return err
			}
			if principal.ProjectIDs == nil {
				principal.ProjectIDs = map[string]struct{}{}
			}
			principal.ProjectIDs[projectID] = struct{}{}
			if principal.ProjectRoles == nil {
				principal.ProjectRoles = map[string]string{}
			}
			principal.ProjectRoles[projectID] = "project_admin"
		}
		return rows.Err()
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT pm.project_id::text, pm.role_code
		FROM project_members pm
		JOIN projects p ON p.id = pm.project_id
		WHERE pm.user_id = $1
		  AND p.tenant_id = $2
		  AND p.status = 'active'
		  AND p.deleted_at IS NULL
	`, principal.ID, principal.TenantID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var projectID, projectRole string
		if err := rows.Scan(&projectID, &projectRole); err != nil {
			return err
		}
		if principal.ProjectIDs == nil {
			principal.ProjectIDs = map[string]struct{}{}
		}
		principal.ProjectIDs[projectID] = struct{}{}
		if principal.ProjectRoles == nil {
			principal.ProjectRoles = map[string]string{}
		}
		principal.ProjectRoles[projectID] = projectRole
		if projectRole == "project_admin" {
			principal.Permissions["project:update"] = struct{}{}
		}
	}
	return rows.Err()
}

var tenantRolePermissions = map[string][]string{
	"tenant_owner": {
		"tenant:read", "tenant:update", "member:invite", "member:remove",
	"project:read", "project:update", "token:read", "token:create", "token:revoke",
		"usage:read", "billing:read", "model:use", "model:status:read",
	},
	"tenant_admin": {
		"tenant:read", "member:invite", "member:remove",
		"project:read", "project:update", "token:read", "token:create", "token:revoke",
		"usage:read", "billing:read", "model:use", "model:status:read",
	},
	"developer": {
		"project:read", "token:read", "token:create", "token:revoke", "usage:read", "model:use", "model:status:read",
	},
	"viewer": {
		"project:read", "usage:read", "billing:read", "model:status:read",
	},
}

func stringSet(raw []byte) (map[string]struct{}, error) {
	if len(raw) == 0 {
		return map[string]struct{}{}, nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			result[value] = struct{}{}
		}
	}
	return result, nil
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

var timeNow = func() time.Time {
	return time.Now()
}
