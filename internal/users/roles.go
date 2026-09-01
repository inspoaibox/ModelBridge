package users

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"

	"ai-token/internal/ids"
)

var (
	ErrPlatformRoleAccessDenied  = errors.New("platform role management access denied")
	ErrPlatformRoleNotFound      = errors.New("platform role is not found")
	ErrPlatformRoleExists        = errors.New("platform role already exists")
	ErrPlatformRoleInvalid       = errors.New("invalid platform role request")
	ErrPlatformRoleProtected     = errors.New("the platform owner role cannot be disabled or renamed")
	ErrPlatformPermissionUnknown = errors.New("platform permission is not found")
	ErrLastPlatformRoleAdmin     = errors.New("the last active platform administrator cannot be unbound")
	ErrPlatformMFARequired       = errors.New("the user must enroll TOTP before becoming a platform administrator")
)

type PlatformPermission struct {
	ID       string `json:"id"`
	Resource string `json:"resource"`
	Action   string `json:"action"`
	Name     string `json:"name"`
}

type PlatformRole struct {
	ID          string    `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	MemberCount int       `json:"member_count"`
	Permissions []string  `json:"permissions"`
	CreatedAt   time.Time `json:"created_at"`
}

type PlatformRoleMutation struct {
	Code        string
	Name        string
	Status      string
	Permissions []string
}

type PlatformRoleService interface {
	ListPlatformPermissions(context.Context) ([]PlatformPermission, error)
	ListPlatformRoles(context.Context) ([]PlatformRole, error)
	CreatePlatformRole(context.Context, string, PlatformRoleMutation) (PlatformRole, error)
	UpdatePlatformRole(context.Context, string, string, PlatformRoleMutation) (PlatformRole, error)
	DisablePlatformRole(context.Context, string, string) error
	GetPlatformUserRoles(context.Context, string) ([]PlatformRole, error)
	SetPlatformUserRoles(context.Context, string, string, []string) ([]PlatformRole, error)
}

var platformRoleCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,63}$`)

func (s *SQLAdminService) ListPlatformPermissions(ctx context.Context) ([]PlatformPermission, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id::text, resource, action, name
		FROM platform_permissions
		ORDER BY resource, action
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]PlatformPermission, 0)
	for rows.Next() {
		var item PlatformPermission
		if err := rows.Scan(&item.ID, &item.Resource, &item.Action, &item.Name); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *SQLAdminService) ListPlatformRoles(ctx context.Context) ([]PlatformRole, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id::text, r.code, r.name, r.status,
		       COUNT(DISTINCT u.id) FILTER (WHERE u.status = 'active' AND u.deleted_at IS NULL)::int,
		       COALESCE((
		           SELECT jsonb_agg(p.name ORDER BY p.name)
		           FROM platform_role_permissions rp
		           JOIN platform_permissions p ON p.id = rp.permission_id
		           WHERE rp.role_id = r.id
		       ), '[]'::jsonb),
		       r.created_at
		FROM platform_roles r
		LEFT JOIN platform_user_roles ur ON ur.role_id = r.id
		LEFT JOIN users u ON u.id = ur.user_id
		GROUP BY r.id, r.code, r.name, r.status, r.created_at
		ORDER BY CASE WHEN r.code = 'platform_owner' THEN 0 ELSE 1 END, r.code
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]PlatformRole, 0)
	for rows.Next() {
		var item PlatformRole
		var permissionsRaw []byte
		if err := rows.Scan(&item.ID, &item.Code, &item.Name, &item.Status, &item.MemberCount, &permissionsRaw, &item.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(permissionsRaw, &item.Permissions); err != nil {
			return nil, err
		}
		if item.Permissions == nil {
			item.Permissions = []string{}
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *SQLAdminService) CreatePlatformRole(ctx context.Context, actorID string, request PlatformRoleMutation) (PlatformRole, error) {
	if s == nil || s.db == nil {
		return PlatformRole{}, ErrUnavailable
	}
	request, permissionIDs, err := s.validatePlatformRoleMutation(ctx, request, true)
	if err != nil {
		return PlatformRole{}, err
	}
	if err := s.requirePlatformOwner(ctx, actorID); err != nil {
		return PlatformRole{}, err
	}
	roleID, err := ids.New()
	if err != nil {
		return PlatformRole{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PlatformRole{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO platform_roles (id, code, name, status)
		VALUES ($1::uuid, $2, $3, $4)
	`, roleID, request.Code, request.Name, request.Status); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "platform_roles_code_key") || strings.Contains(strings.ToLower(err.Error()), "platform_role_code") {
			return PlatformRole{}, ErrPlatformRoleExists
		}
		return PlatformRole{}, err
	}
	if err := replacePlatformRolePermissions(ctx, tx, roleID, permissionIDs); err != nil {
		return PlatformRole{}, err
	}
	if err := tx.Commit(); err != nil {
		return PlatformRole{}, err
	}
	return s.platformRole(ctx, roleID)
}

func (s *SQLAdminService) UpdatePlatformRole(ctx context.Context, actorID, roleID string, request PlatformRoleMutation) (PlatformRole, error) {
	if !validUUID(roleID) {
		return PlatformRole{}, ErrPlatformRoleInvalid
	}
	request, permissionIDs, err := s.validatePlatformRoleMutation(ctx, request, false)
	if err != nil {
		return PlatformRole{}, err
	}
	if err := s.requirePlatformOwner(ctx, actorID); err != nil {
		return PlatformRole{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PlatformRole{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('ai-token:platform-role-bindings', 0))`); err != nil {
		return PlatformRole{}, err
	}
	var currentCode, currentName, currentStatus string
	if err := tx.QueryRowContext(ctx, `SELECT code, name, status FROM platform_roles WHERE id = $1::uuid FOR UPDATE`, roleID).Scan(&currentCode, &currentName, &currentStatus); errors.Is(err, sql.ErrNoRows) {
		return PlatformRole{}, ErrPlatformRoleNotFound
	} else if err != nil {
		return PlatformRole{}, err
	}
	if request.Code == "" {
		request.Code = currentCode
	}
	// The owner role is the break-glass root of the platform. Keeping its
	// permission set immutable prevents an owner from accidentally removing
	// the very permissions needed to recover role administration.
	if currentCode == "platform_owner" {
		return PlatformRole{}, ErrPlatformRoleProtected
	}
	if currentStatus == "active" && request.Status != "active" {
		if err := ensurePlatformAdminRemains(ctx, tx, roleID, request.Status); err != nil {
			return PlatformRole{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE platform_roles SET code = $2, name = $3, status = $4 WHERE id = $1::uuid
	`, roleID, request.Code, request.Name, request.Status); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "platform_roles_code_key") || strings.Contains(strings.ToLower(err.Error()), "platform_role_code") {
			return PlatformRole{}, ErrPlatformRoleExists
		}
		return PlatformRole{}, err
	}
	if err := replacePlatformRolePermissions(ctx, tx, roleID, permissionIDs); err != nil {
		return PlatformRole{}, err
	}
	if err := revokePlatformRoleSessions(ctx, tx, roleID); err != nil {
		return PlatformRole{}, err
	}
	if err := tx.Commit(); err != nil {
		return PlatformRole{}, err
	}
	return s.platformRole(ctx, roleID)
}

func (s *SQLAdminService) DisablePlatformRole(ctx context.Context, actorID, roleID string) error {
	if !validUUID(roleID) {
		return ErrPlatformRoleInvalid
	}
	if err := s.requirePlatformOwner(ctx, actorID); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('ai-token:platform-role-bindings', 0))`); err != nil {
		return err
	}
	var code, status string
	if err := tx.QueryRowContext(ctx, `SELECT code, status FROM platform_roles WHERE id = $1::uuid FOR UPDATE`, roleID).Scan(&code, &status); errors.Is(err, sql.ErrNoRows) {
		return ErrPlatformRoleNotFound
	} else if err != nil {
		return err
	}
	if code == "platform_owner" {
		return ErrPlatformRoleProtected
	}
	if status == "active" {
		if err := ensurePlatformAdminRemains(ctx, tx, roleID, "disabled"); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE platform_roles SET status = 'disabled' WHERE id = $1::uuid`, roleID)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected != 1 {
		return ErrPlatformRoleNotFound
	}
	if err := revokePlatformRoleSessions(ctx, tx, roleID); err != nil {
		return err
	}
	return tx.Commit()
}

// SetPlatformUserRoles binds existing users to platform roles. It never creates
// a user: all customer accounts still come from the public console register
// flow. Only a platform owner can perform this privilege-changing operation.
func (s *SQLAdminService) GetPlatformUserRoles(ctx context.Context, userID string) ([]PlatformRole, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	if !validUUID(userID) {
		return nil, ErrPlatformRoleInvalid
	}
	return s.platformUserRoles(ctx, userID)
}

func (s *SQLAdminService) SetPlatformUserRoles(ctx context.Context, actorID, userID string, roleIDs []string) ([]PlatformRole, error) {
	if !validUUID(userID) || !validUUID(actorID) || actorID == userID {
		return nil, ErrPlatformRoleInvalid
	}
	if err := s.requirePlatformOwner(ctx, actorID); err != nil {
		return nil, err
	}
	roleIDs = uniqueRoleIDs(roleIDs)
	for _, roleID := range roleIDs {
		if !validUUID(roleID) {
			return nil, ErrPlatformRoleInvalid
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('ai-token:platform-role-bindings', 0))`); err != nil {
		return nil, err
	}
	var userExists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1::uuid AND status = 'active' AND deleted_at IS NULL)`, userID).Scan(&userExists); err != nil {
		return nil, err
	}
	if !userExists {
		return nil, ErrNotFound
	}
	permissionRoleIDs := make([]string, 0, len(roleIDs))
	for _, roleID := range roleIDs {
		var code string
		if err := tx.QueryRowContext(ctx, `SELECT code FROM platform_roles WHERE id = $1::uuid AND status = 'active'`, roleID).Scan(&code); errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPlatformRoleNotFound
		} else if err != nil {
			return nil, err
		}
		permissionRoleIDs = append(permissionRoleIDs, roleID)
	}
	if len(permissionRoleIDs) > 0 {
		enforced, err := platformMFAEnforced(ctx, tx)
		if err != nil {
			return nil, err
		}
		if enforced {
			hasMFA, err := userHasActiveTOTP(ctx, tx, userID)
			if err != nil {
				return nil, err
			}
			if !hasMFA {
				return nil, ErrPlatformMFARequired
			}
		}
	}
	var targetIsAdmin bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM platform_user_roles ur
			JOIN platform_roles r ON r.id = ur.role_id AND r.status = 'active'
			WHERE ur.user_id = $1::uuid
		)
	`, userID).Scan(&targetIsAdmin); err != nil {
		return nil, err
	}
	if targetIsAdmin && len(permissionRoleIDs) == 0 {
		var otherAdmins int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(DISTINCT u.id)::int
			FROM users u
			JOIN platform_user_roles ur ON ur.user_id = u.id
			JOIN platform_roles r ON r.id = ur.role_id AND r.status = 'active'
			WHERE u.status = 'active' AND u.deleted_at IS NULL AND u.id <> $1::uuid
		`, userID).Scan(&otherAdmins); err != nil {
			return nil, err
		}
		if otherAdmins == 0 {
			return nil, ErrLastPlatformRoleAdmin
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM platform_user_roles WHERE user_id = $1::uuid`, userID); err != nil {
		return nil, err
	}
	for _, roleID := range permissionRoleIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO platform_user_roles (user_id, role_id) VALUES ($1::uuid, $2::uuid)`, userID, roleID); err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE web_sessions SET revoked_at = COALESCE(revoked_at, now()) WHERE user_id = $1::uuid AND audience = 'admin' AND revoked_at IS NULL`, userID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.platformUserRoles(ctx, userID)
}

func (s *SQLAdminService) validatePlatformRoleMutation(ctx context.Context, request PlatformRoleMutation, creating bool) (PlatformRoleMutation, []string, error) {
	if s == nil || s.db == nil {
		return PlatformRoleMutation{}, nil, ErrUnavailable
	}
	request.Code = strings.ToLower(strings.TrimSpace(request.Code))
	request.Name = strings.TrimSpace(request.Name)
	request.Status = strings.ToLower(strings.TrimSpace(request.Status))
	if request.Status == "" {
		request.Status = "active"
	}
	if creating && !platformRoleCodePattern.MatchString(request.Code) {
		return PlatformRoleMutation{}, nil, ErrPlatformRoleInvalid
	}
	if !creating && request.Code != "" && !platformRoleCodePattern.MatchString(request.Code) {
		return PlatformRoleMutation{}, nil, ErrPlatformRoleInvalid
	}
	if request.Name == "" || len(request.Name) > 100 || (request.Status != "active" && request.Status != "disabled") {
		return PlatformRoleMutation{}, nil, ErrPlatformRoleInvalid
	}
	permissionIDs, err := s.permissionIDs(ctx, request.Permissions)
	if err != nil {
		return PlatformRoleMutation{}, nil, err
	}
	return request, permissionIDs, nil
}

func (s *SQLAdminService) permissionIDs(ctx context.Context, names []string) ([]string, error) {
	seen := map[string]struct{}{}
	cleaned := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		cleaned = append(cleaned, name)
	}
	sort.Strings(cleaned)
	if len(cleaned) == 0 {
		return []string{}, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id::text, name FROM platform_permissions WHERE name = ANY($1::text[])`, cleaned)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	idsByName := make(map[string]string, len(cleaned))
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		idsByName[strings.ToLower(name)] = id
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]string, 0, len(cleaned))
	for _, name := range cleaned {
		id, ok := idsByName[name]
		if !ok {
			return nil, ErrPlatformPermissionUnknown
		}
		result = append(result, id)
	}
	return result, nil
}

func replacePlatformRolePermissions(ctx context.Context, tx *sql.Tx, roleID string, permissionIDs []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM platform_role_permissions WHERE role_id = $1::uuid`, roleID); err != nil {
		return err
	}
	for _, permissionID := range permissionIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO platform_role_permissions (role_id, permission_id) VALUES ($1::uuid, $2::uuid)`, roleID, permissionID); err != nil {
			return err
		}
	}
	return nil
}

func revokePlatformRoleSessions(ctx context.Context, tx *sql.Tx, roleID string) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE web_sessions
		SET revoked_at = COALESCE(revoked_at, now())
		WHERE audience = 'admin'
		  AND revoked_at IS NULL
		  AND user_id IN (
		      SELECT user_id
		      FROM platform_user_roles
		      WHERE role_id = $1::uuid
		  )
	`, roleID)
	return err
}

func (s *SQLAdminService) requirePlatformOwner(ctx context.Context, actorID string) error {
	if s == nil || s.db == nil || !validUUID(actorID) {
		return ErrPlatformRoleAccessDenied
	}
	var allowed bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM platform_user_roles ur
			JOIN platform_roles r ON r.id = ur.role_id AND r.status = 'active'
			JOIN users u ON u.id = ur.user_id AND u.status = 'active' AND u.deleted_at IS NULL
			WHERE ur.user_id = $1::uuid AND r.code = 'platform_owner'
		)
	`, actorID).Scan(&allowed)
	if err != nil || !allowed {
		return ErrPlatformRoleAccessDenied
	}
	return nil
}

func ensurePlatformAdminRemains(ctx context.Context, tx *sql.Tx, roleID, nextStatus string) error {
	var activeAdmins int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT u.id)::int
		FROM users u
		JOIN platform_user_roles ur ON ur.user_id = u.id
		JOIN platform_roles r ON r.id = ur.role_id
		WHERE u.status = 'active' AND u.deleted_at IS NULL
		  AND (r.status = 'active' OR (r.id = $1::uuid AND $2 = 'active'))
		  AND (r.id <> $1::uuid OR $2 = 'active')
	`, roleID, nextStatus).Scan(&activeAdmins); err != nil {
		return err
	}
	if activeAdmins == 0 {
		return ErrLastPlatformRoleAdmin
	}
	return nil
}

func (s *SQLAdminService) platformRole(ctx context.Context, roleID string) (PlatformRole, error) {
	roles, err := s.ListPlatformRoles(ctx)
	if err != nil {
		return PlatformRole{}, err
	}
	for _, role := range roles {
		if role.ID == roleID {
			return role, nil
		}
	}
	return PlatformRole{}, ErrPlatformRoleNotFound
}

func (s *SQLAdminService) platformUserRoles(ctx context.Context, userID string) ([]PlatformRole, error) {
	roles, err := s.ListPlatformRoles(ctx)
	if err != nil {
		return nil, err
	}
	assigned := make([]PlatformRole, 0)
	rows, err := s.db.QueryContext(ctx, `SELECT role_id::text FROM platform_user_roles WHERE user_id = $1::uuid`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	idsSet := map[string]struct{}{}
	for rows.Next() {
		var roleID string
		if err := rows.Scan(&roleID); err != nil {
			return nil, err
		}
		idsSet[roleID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, role := range roles {
		if _, ok := idsSet[role.ID]; ok {
			assigned = append(assigned, role)
		}
	}
	return assigned, nil
}

func uniqueRoleIDs(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
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

func platformMFAEnforced(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}) (bool, error) {
	var value string
	err := queryer.QueryRowContext(ctx, `SELECT value FROM platform_settings WHERE key = 'admin_mfa_enabled'`).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(value), "true"), nil
}

func userHasActiveTOTP(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, userID string) (bool, error) {
	var exists bool
	err := queryer.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM mfa_credentials
			WHERE user_id = $1::uuid
			  AND type = 'totp'
			  AND status = 'active'
			  AND revoked_at IS NULL
			  AND (expires_at IS NULL OR expires_at > now())
		)
	`, userID).Scan(&exists)
	return exists, err
}
