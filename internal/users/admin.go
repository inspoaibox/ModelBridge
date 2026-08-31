package users

import (
	"context"
	"database/sql"
	"errors"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"ai-token/internal/ids"
	"ai-token/internal/passwords"
)

var (
	ErrUnavailable       = errors.New("user admin service is unavailable")
	ErrNotFound          = errors.New("user is not found")
	ErrInvalid           = errors.New("invalid user admin request")
	ErrSelfUpdate        = errors.New("an administrator cannot change their own account")
	ErrEmailExists       = errors.New("email is already registered")
	ErrTenantNotFound    = errors.New("tenant is not found")
	ErrTenantRoleInvalid = errors.New("tenant role is invalid")
	ErrLastPlatformAdmin = errors.New("the last active platform administrator cannot be disabled")
)

type Summary struct {
	ID            string     `json:"id"`
	Email         string     `json:"email"`
	DisplayName   string     `json:"display_name"`
	Status        string     `json:"status"`
	PlatformRoles []string   `json:"platform_roles"`
	TenantCount   int        `json:"tenant_count"`
	TenantNames   []string   `json:"tenant_names"`
	TenantIDs     []string   `json:"tenant_ids"`
	CreatedAt     time.Time  `json:"created_at"`
	LastLoginAt   *time.Time `json:"last_login_at,omitempty"`
}

type TenantSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type CreateRequest struct {
	Email       string
	DisplayName string
	Password    string
	TenantID    string
	TenantRole  string
}

type UpdateRequest struct {
	Email       string
	DisplayName string
	Password    string
}

type AdminService interface {
	List(context.Context) ([]Summary, error)
	ListTenants(context.Context) ([]TenantSummary, error)
	Create(context.Context, string, CreateRequest) (Summary, error)
	Update(context.Context, string, string, UpdateRequest) (Summary, error)
	SetStatus(context.Context, string, string, string) (Summary, error)
}

type SQLAdminService struct {
	db *sql.DB
}

func NewAdminService(db *sql.DB) (*SQLAdminService, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &SQLAdminService{db: db}, nil
}

func (s *SQLAdminService) List(ctx context.Context) ([]Summary, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id::text, u.email, u.display_name, u.status,
		       COALESCE((
		           SELECT string_agg(DISTINCT r.code, ',' ORDER BY r.code)
		           FROM platform_user_roles ur
		           JOIN platform_roles r ON r.id = ur.role_id AND r.status = 'active'
		           WHERE ur.user_id = u.id
		       ), ''),
		       COALESCE((
		           SELECT COUNT(*)::int
		           FROM tenant_members tm
		           JOIN tenants t ON t.id = tm.tenant_id
		           WHERE tm.user_id = u.id AND tm.status = 'active' AND t.deleted_at IS NULL
		       ), 0),
		       COALESCE((
		           SELECT string_agg(DISTINCT t.name, ',' ORDER BY t.name)
		           FROM tenant_members tm
		           JOIN tenants t ON t.id = tm.tenant_id
		           WHERE tm.user_id = u.id AND tm.status = 'active' AND t.deleted_at IS NULL
		       ), ''),
		       COALESCE((
		           SELECT string_agg(DISTINCT t.id::text, ',' ORDER BY t.id::text)
		           FROM tenant_members tm
		           JOIN tenants t ON t.id = tm.tenant_id
		           WHERE tm.user_id = u.id AND tm.status = 'active' AND t.deleted_at IS NULL
		       ), ''),
		       u.created_at, u.last_login_at
		FROM users u
		WHERE u.deleted_at IS NULL
		ORDER BY u.created_at DESC, u.id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Summary, 0)
	for rows.Next() {
		var (
			item        Summary
			roles       string
			tenantNames string
			tenantIDs   string
			lastLogin   sql.NullTime
		)
		if err := rows.Scan(
			&item.ID,
			&item.Email,
			&item.DisplayName,
			&item.Status,
			&roles,
			&item.TenantCount,
			&tenantNames,
			&tenantIDs,
			&item.CreatedAt,
			&lastLogin,
		); err != nil {
			return nil, err
		}
		item.PlatformRoles = splitCommaList(roles)
		item.TenantNames = splitCommaList(tenantNames)
		item.TenantIDs = splitCommaList(tenantIDs)
		if lastLogin.Valid {
			value := lastLogin.Time
			item.LastLoginAt = &value
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *SQLAdminService) ListTenants(ctx context.Context) ([]TenantSummary, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id::text, name, slug
		FROM tenants
		WHERE status = 'active' AND deleted_at IS NULL
		ORDER BY name, id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]TenantSummary, 0)
	for rows.Next() {
		var item TenantSummary
		if err := rows.Scan(&item.ID, &item.Name, &item.Slug); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *SQLAdminService) Create(ctx context.Context, actorID string, request CreateRequest) (Summary, error) {
	if s == nil || s.db == nil {
		return Summary{}, ErrUnavailable
	}
	actorID = strings.TrimSpace(actorID)
	request.Email = normalizeEmail(request.Email)
	request.DisplayName = strings.TrimSpace(request.DisplayName)
	request.Password = strings.TrimSpace(request.Password)
	request.TenantID = strings.TrimSpace(request.TenantID)
	request.TenantRole = strings.ToLower(strings.TrimSpace(request.TenantRole))
	if request.TenantRole == "" {
		request.TenantRole = "developer"
	}
	if !validUUID(actorID) || !validAdminAccountFields(request.Email, request.DisplayName) ||
		request.Password == "" || !validUUID(request.TenantID) || !validTenantRole(request.TenantRole) {
		if !validTenantRole(request.TenantRole) && validAdminAccountFields(request.Email, request.DisplayName) {
			return Summary{}, ErrTenantRoleInvalid
		}
		return Summary{}, ErrInvalid
	}
	passwordHash, err := passwords.Hash(request.Password)
	if err != nil {
		return Summary{}, ErrInvalid
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Summary{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var tenantStatus string
	if err := tx.QueryRowContext(ctx, `
		SELECT status FROM tenants WHERE id = $1::uuid AND deleted_at IS NULL
	`, request.TenantID).Scan(&tenantStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Summary{}, ErrTenantNotFound
		}
		return Summary{}, err
	}
	if tenantStatus != "active" {
		return Summary{}, ErrInvalid
	}

	var exists bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM users WHERE lower(email) = $1 AND deleted_at IS NULL)
	`, request.Email).Scan(&exists); err != nil {
		return Summary{}, err
	}
	if exists {
		return Summary{}, ErrEmailExists
	}

	userID, err := ids.New()
	if err != nil {
		return Summary{}, err
	}
	memberID, err := ids.New()
	if err != nil {
		return Summary{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, display_name, status, password_changed_at)
		VALUES ($1::uuid, $2, $3, $4, 'active', now())
	`, userID, request.Email, passwordHash, request.DisplayName); err != nil {
		return Summary{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO tenant_members (id, tenant_id, user_id, role_code, status, created_by)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, 'active', $5::uuid)
	`, memberID, request.TenantID, userID, request.TenantRole, actorID); err != nil {
		return Summary{}, err
	}
	if request.TenantRole == "developer" || request.TenantRole == "viewer" {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO project_members (project_id, user_id, role_code)
			SELECT p.id, $1::uuid, $2
			FROM projects p
			WHERE p.tenant_id = $3::uuid AND p.status = 'active' AND p.deleted_at IS NULL
		ON CONFLICT (project_id, user_id) DO NOTHING
		`, userID, request.TenantRole, request.TenantID); err != nil {
			return Summary{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Summary{}, err
	}
	return s.findSummary(ctx, userID)
}

func (s *SQLAdminService) Update(ctx context.Context, actorID, userID string, request UpdateRequest) (Summary, error) {
	if s == nil || s.db == nil {
		return Summary{}, ErrUnavailable
	}
	actorID = strings.TrimSpace(actorID)
	userID = strings.TrimSpace(userID)
	request.Email = normalizeEmail(request.Email)
	request.DisplayName = strings.TrimSpace(request.DisplayName)
	request.Password = strings.TrimSpace(request.Password)
	if !validUUID(actorID) || !validUUID(userID) || actorID == userID {
		if actorID == userID && validUUID(actorID) {
			return Summary{}, ErrSelfUpdate
		}
		return Summary{}, ErrInvalid
	}
	if !validAdminAccountFields(request.Email, request.DisplayName) {
		return Summary{}, ErrInvalid
	}

	var passwordHash string
	if request.Password != "" {
		var err error
		passwordHash, err = passwords.Hash(request.Password)
		if err != nil {
			return Summary{}, ErrInvalid
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Summary{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var currentEmail string
	if err := tx.QueryRowContext(ctx, `
		SELECT email
		FROM users
		WHERE id = $1::uuid AND deleted_at IS NULL
		FOR UPDATE
	`, userID).Scan(&currentEmail); errors.Is(err, sql.ErrNoRows) {
		return Summary{}, ErrNotFound
	} else if err != nil {
		return Summary{}, err
	}

	var exists bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM users
			WHERE lower(email) = $1 AND id <> $2::uuid AND deleted_at IS NULL
		)
	`, request.Email, userID).Scan(&exists); err != nil {
		return Summary{}, err
	}
	if exists {
		return Summary{}, ErrEmailExists
	}

	var result sql.Result
	if passwordHash == "" {
		result, err = tx.ExecContext(ctx, `
			UPDATE users SET email = $1, display_name = $2, updated_at = now()
			WHERE id = $3::uuid AND deleted_at IS NULL
		`, request.Email, request.DisplayName, userID)
	} else {
		result, err = tx.ExecContext(ctx, `
			UPDATE users
			SET email = $1, display_name = $2, password_hash = $3,
				password_changed_at = now(), updated_at = now()
			WHERE id = $4::uuid AND deleted_at IS NULL
		`, request.Email, request.DisplayName, passwordHash, userID)
	}
	if err != nil {
		return Summary{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return Summary{}, err
	}
	if affected == 0 {
		return Summary{}, ErrNotFound
	}
	if passwordHash != "" || !strings.EqualFold(currentEmail, request.Email) {
		if _, err := tx.ExecContext(ctx, `
			UPDATE web_sessions
			SET revoked_at = COALESCE(revoked_at, now())
			WHERE user_id = $1::uuid AND revoked_at IS NULL
		`, userID); err != nil {
			return Summary{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Summary{}, err
	}
	return s.findSummary(ctx, userID)
}

func (s *SQLAdminService) SetStatus(ctx context.Context, actorID, userID, status string) (Summary, error) {
	if s == nil || s.db == nil {
		return Summary{}, ErrUnavailable
	}
	actorID = strings.TrimSpace(actorID)
	userID = strings.TrimSpace(userID)
	status = strings.ToLower(strings.TrimSpace(status))
	if !validUUID(actorID) || !validUUID(userID) || !validUserStatus(status) {
		return Summary{}, ErrInvalid
	}
	if actorID == userID {
		return Summary{}, ErrSelfUpdate
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Summary{}, err
	}
	defer func() { _ = tx.Rollback() }()
	// Serialize administrator status changes so concurrent updates cannot
	// disable every active platform administrator at once.
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('ai-token:platform-admin-status', 0))`); err != nil {
		return Summary{}, err
	}
	if status != "active" {
		var targetIsPlatformAdmin bool
		if err := tx.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM platform_user_roles ur
				JOIN platform_roles pr ON pr.id = ur.role_id AND pr.status = 'active'
				WHERE ur.user_id = $1::uuid
			)
		`, userID).Scan(&targetIsPlatformAdmin); err != nil {
			return Summary{}, err
		}
		if targetIsPlatformAdmin {
			var activePlatformAdmins int
			if err := tx.QueryRowContext(ctx, `
				SELECT COUNT(DISTINCT u.id)
				FROM users u
				JOIN platform_user_roles ur ON ur.user_id = u.id
				JOIN platform_roles pr ON pr.id = ur.role_id AND pr.status = 'active'
				WHERE u.status = 'active' AND u.deleted_at IS NULL
			`).Scan(&activePlatformAdmins); err != nil {
				return Summary{}, err
			}
			if activePlatformAdmins <= 1 {
				return Summary{}, ErrLastPlatformAdmin
			}
		}
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE users
		SET status = $3, updated_at = now()
		WHERE id = $2::uuid AND deleted_at IS NULL
	`, actorID, userID, status)
	if err != nil {
		return Summary{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return Summary{}, err
	}
	if affected == 0 {
		return Summary{}, ErrNotFound
	}
	if status != "active" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE web_sessions
			SET revoked_at = COALESCE(revoked_at, now())
			WHERE user_id = $1::uuid AND revoked_at IS NULL
		`, userID); err != nil {
			return Summary{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Summary{}, err
	}
	items, err := s.List(ctx)
	if err != nil {
		return Summary{}, err
	}
	for _, item := range items {
		if item.ID == userID {
			return item, nil
		}
	}
	return Summary{}, ErrNotFound
}

func (s *SQLAdminService) findSummary(ctx context.Context, userID string) (Summary, error) {
	items, err := s.List(ctx)
	if err != nil {
		return Summary{}, err
	}
	for _, item := range items {
		if item.ID == userID {
			return item, nil
		}
	}
	return Summary{}, ErrNotFound
}

func validAdminAccountFields(email, displayName string) bool {
	return validEmail(email) && displayName != "" && len(displayName) <= 100
}

func validEmail(value string) bool {
	if value == "" || len(value) > 254 {
		return false
	}
	parsed, err := mail.ParseAddress(value)
	return err == nil && parsed.Address == value
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func validTenantRole(value string) bool {
	switch value {
	case "tenant_admin", "developer", "viewer":
		return true
	default:
		return false
	}
}

func validUserStatus(value string) bool {
	switch value {
	case "active", "locked", "disabled":
		return true
	default:
		return false
	}
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

func validUUID(value string) bool {
	return uuidPattern.MatchString(value)
}

func splitCommaList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{}
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}
