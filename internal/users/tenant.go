package users

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"ai-token/internal/ids"
)

var (
	ErrTenantAccessDenied    = errors.New("tenant management access denied")
	ErrMemberNotFound        = errors.New("tenant member is not found")
	ErrMemberExists          = errors.New("tenant member already exists")
	ErrMemberInvalid         = errors.New("invalid tenant member request")
	ErrLastTenantOwner       = errors.New("the last tenant owner cannot be removed or demoted")
	ErrProjectNotFound       = errors.New("project is not found")
	ErrProjectExists         = errors.New("project already exists")
	ErrProjectInvalid        = errors.New("invalid project request")
	ErrProjectMemberExists   = errors.New("project member already exists")
	ErrProjectMemberNotFound = errors.New("project member is not found")
	ErrLastProjectAdmin      = errors.New("the last project administrator cannot be removed or demoted")
)

type TenantMember struct {
	TenantID     string    `json:"tenant_id"`
	UserID       string    `json:"user_id"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"display_name"`
	Role         string    `json:"role"`
	Status       string    `json:"status"`
	ProjectCount int       `json:"project_count"`
	CreatedAt    time.Time `json:"created_at"`
}

type Project struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Status      string    `json:"status"`
	CreatedBy   string    `json:"created_by"`
	MemberCount int       `json:"member_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ProjectMember struct {
	ProjectID   string    `json:"project_id"`
	UserID      string    `json:"user_id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"created_at"`
}

type MemberMutation struct {
	Email  string
	Role   string
	Status string
}

type ProjectMutation struct {
	Name   string
	Slug   string
	Status string
}

type ProjectMemberMutation struct {
	UserID string
	Email  string
	Role   string
}

type TenantService interface {
	ListMembers(context.Context, string, string) ([]TenantMember, error)
	AddMember(context.Context, string, string, MemberMutation) (TenantMember, error)
	UpdateMember(context.Context, string, string, string, MemberMutation) (TenantMember, error)
	RemoveMember(context.Context, string, string, string) error
	ListProjects(context.Context, string, string) ([]Project, error)
	CreateProject(context.Context, string, string, ProjectMutation) (Project, error)
	UpdateProject(context.Context, string, string, string, ProjectMutation) (Project, error)
	DeleteProject(context.Context, string, string, string) error
	ListProjectMembers(context.Context, string, string, string) ([]ProjectMember, error)
	AddProjectMember(context.Context, string, string, string, ProjectMemberMutation) (ProjectMember, error)
	UpdateProjectMember(context.Context, string, string, string, string, ProjectMemberMutation) (ProjectMember, error)
	RemoveProjectMember(context.Context, string, string, string, string) error
}

func (s *SQLAdminService) ListMembers(ctx context.Context, tenantID, actorID string) ([]TenantMember, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	if err := s.requireTenantManager(ctx, tenantID, actorID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT tm.tenant_id::text, tm.user_id::text, u.email, u.display_name,
		       tm.role_code, tm.status, COUNT(DISTINCT p.id)::int, tm.created_at
		FROM tenant_members tm
		JOIN users u ON u.id = tm.user_id AND u.deleted_at IS NULL
		LEFT JOIN project_members pm ON pm.user_id = tm.user_id
		LEFT JOIN projects p ON p.id = pm.project_id AND p.tenant_id = tm.tenant_id AND p.deleted_at IS NULL
		WHERE tm.tenant_id = $1::uuid AND tm.status <> 'removed'
		GROUP BY tm.tenant_id, tm.user_id, u.email, u.display_name, tm.role_code, tm.status, tm.created_at
		ORDER BY tm.created_at, tm.user_id
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]TenantMember, 0)
	for rows.Next() {
		var item TenantMember
		if err := rows.Scan(&item.TenantID, &item.UserID, &item.Email, &item.DisplayName, &item.Role, &item.Status, &item.ProjectCount, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *SQLAdminService) AddMember(ctx context.Context, actorID, tenantID string, request MemberMutation) (TenantMember, error) {
	request.Email = normalizeEmail(request.Email)
	request.Role = strings.ToLower(strings.TrimSpace(request.Role))
	if !validUUID(actorID) || !validUUID(tenantID) || !validTenantMemberRole(request.Role) || !validEmail(request.Email) {
		return TenantMember{}, ErrMemberInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return TenantMember{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockTenantMembership(ctx, tx, tenantID); err != nil {
		return TenantMember{}, err
	}
	actorRole, err := tenantRoleTx(ctx, tx, tenantID, actorID)
	if err != nil {
		return TenantMember{}, err
	}
	if !canManageRole(actorRole, request.Role) {
		return TenantMember{}, ErrTenantAccessDenied
	}
	var userID string
	if err := tx.QueryRowContext(ctx, `SELECT id::text FROM users WHERE lower(email) = $1 AND status = 'active' AND deleted_at IS NULL`, request.Email).Scan(&userID); errors.Is(err, sql.ErrNoRows) {
		return TenantMember{}, ErrMemberNotFound
	} else if err != nil {
		return TenantMember{}, err
	}
	var currentStatus string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM tenant_members WHERE tenant_id = $1::uuid AND user_id = $2::uuid FOR UPDATE`, tenantID, userID).Scan(&currentStatus); err == nil {
		if currentStatus != "removed" {
			return TenantMember{}, ErrMemberExists
		}
		if _, err := tx.ExecContext(ctx, `UPDATE tenant_members SET role_code = $3, status = 'active', created_by = $4::uuid, updated_at = now() WHERE tenant_id = $1::uuid AND user_id = $2::uuid`, tenantID, userID, request.Role, actorID); err != nil {
			return TenantMember{}, err
		}
		if err := tx.Commit(); err != nil {
			return TenantMember{}, err
		}
		return s.getMember(ctx, tenantID, userID)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return TenantMember{}, err
	}
	memberID, err := ids.New()
	if err != nil {
		return TenantMember{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO tenant_members (id, tenant_id, user_id, role_code, status, created_by) VALUES ($1::uuid, $2::uuid, $3::uuid, $4, 'active', $5::uuid)`, memberID, tenantID, userID, request.Role, actorID); err != nil {
		return TenantMember{}, err
	}
	if err := tx.Commit(); err != nil {
		return TenantMember{}, err
	}
	return s.getMember(ctx, tenantID, userID)
}

func (s *SQLAdminService) UpdateMember(ctx context.Context, actorID, tenantID, userID string, request MemberMutation) (TenantMember, error) {
	request.Role = strings.ToLower(strings.TrimSpace(request.Role))
	request.Status = strings.ToLower(strings.TrimSpace(request.Status))
	if !validUUID(actorID) || !validUUID(tenantID) || !validUUID(userID) || !validTenantMemberRole(request.Role) || (request.Status != "active" && request.Status != "suspended") {
		return TenantMember{}, ErrMemberInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return TenantMember{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockTenantMembership(ctx, tx, tenantID); err != nil {
		return TenantMember{}, err
	}
	actorRole, err := tenantRoleTx(ctx, tx, tenantID, actorID)
	if err != nil {
		return TenantMember{}, err
	}
	if !canManageRole(actorRole, request.Role) {
		return TenantMember{}, ErrTenantAccessDenied
	}
	var currentRole, currentStatus string
	if err := tx.QueryRowContext(ctx, `SELECT role_code, status FROM tenant_members WHERE tenant_id = $1::uuid AND user_id = $2::uuid FOR UPDATE`, tenantID, userID).Scan(&currentRole, &currentStatus); errors.Is(err, sql.ErrNoRows) {
		return TenantMember{}, ErrMemberNotFound
	} else if err != nil {
		return TenantMember{}, err
	}
	if currentStatus == "removed" {
		return TenantMember{}, ErrMemberNotFound
	}
	if actorRole != "tenant_owner" && (currentRole == "tenant_owner" || currentRole == "tenant_admin" || request.Role == "tenant_owner") {
		return TenantMember{}, ErrTenantAccessDenied
	}
	if currentRole == "tenant_owner" && (request.Role != "tenant_owner" || request.Status != "active") {
		var otherOwners int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*)::int FROM tenant_members WHERE tenant_id = $1::uuid AND user_id <> $2::uuid AND role_code = 'tenant_owner' AND status = 'active'`, tenantID, userID).Scan(&otherOwners); err != nil {
			return TenantMember{}, err
		}
		if otherOwners == 0 {
			return TenantMember{}, ErrLastTenantOwner
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE tenant_members SET role_code = $3, status = $4, updated_at = now() WHERE tenant_id = $1::uuid AND user_id = $2::uuid`, tenantID, userID, request.Role, request.Status); err != nil {
		return TenantMember{}, err
	}
	if request.Status != "active" || request.Role == "viewer" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE api_tokens
			SET status = 'revoked', revoked_at = COALESCE(revoked_at, now())
			WHERE tenant_id = $1::uuid AND created_by = $2::uuid AND status = 'active'
		`, tenantID, userID); err != nil {
			return TenantMember{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return TenantMember{}, err
	}
	return s.getMember(ctx, tenantID, userID)
}

func (s *SQLAdminService) RemoveMember(ctx context.Context, actorID, tenantID, userID string) error {
	if !validUUID(actorID) || !validUUID(tenantID) || !validUUID(userID) {
		return ErrMemberInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockTenantMembership(ctx, tx, tenantID); err != nil {
		return err
	}
	actorRole, err := tenantRoleTx(ctx, tx, tenantID, actorID)
	if err != nil {
		return err
	}
	var role, status string
	if err := tx.QueryRowContext(ctx, `SELECT role_code, status FROM tenant_members WHERE tenant_id = $1::uuid AND user_id = $2::uuid FOR UPDATE`, tenantID, userID).Scan(&role, &status); errors.Is(err, sql.ErrNoRows) {
		return ErrMemberNotFound
	} else if err != nil {
		return err
	}
	if actorID == userID || (actorRole != "tenant_owner" && (role == "tenant_owner" || role == "tenant_admin")) {
		return ErrTenantAccessDenied
	}
	if role == "tenant_owner" {
		var otherOwners int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*)::int FROM tenant_members WHERE tenant_id = $1::uuid AND user_id <> $2::uuid AND role_code = 'tenant_owner' AND status = 'active'`, tenantID, userID).Scan(&otherOwners); err != nil {
			return err
		}
		if otherOwners == 0 {
			return ErrLastTenantOwner
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE tenant_members SET status = 'removed', updated_at = now() WHERE tenant_id = $1::uuid AND user_id = $2::uuid AND status <> 'removed'`, tenantID, userID)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected != 1 {
		return ErrMemberNotFound
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM project_members pm
		USING projects p
		WHERE pm.project_id = p.id AND pm.user_id = $2::uuid
		  AND p.tenant_id = $1::uuid
	`, tenantID, userID); err != nil {
		return err
	}
	// A token is owned by its creator. Revoke it when the creator leaves the
	// tenant so the database state matches the resolver's membership check.
	if _, err := tx.ExecContext(ctx, `
		UPDATE api_tokens
		SET status = 'revoked', revoked_at = COALESCE(revoked_at, now())
		WHERE tenant_id = $1::uuid AND created_by = $2::uuid AND status = 'active'
	`, tenantID, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLAdminService) ListProjects(ctx context.Context, tenantID, actorID string) ([]Project, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	if !validUUID(tenantID) || !validUUID(actorID) {
		return nil, ErrProjectInvalid
	}
	role, err := s.tenantRole(ctx, tenantID, actorID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id::text, p.tenant_id::text, p.name, p.slug, p.status, p.created_by::text,
		       COUNT(DISTINCT pm.user_id)::int, p.created_at, p.updated_at
		FROM projects p
		LEFT JOIN project_members pm ON pm.project_id = p.id
		WHERE p.tenant_id = $1::uuid AND p.deleted_at IS NULL
		  AND ($2 IN ('tenant_owner', 'tenant_admin') OR (p.status = 'active' AND EXISTS (
			SELECT 1 FROM project_members own WHERE own.project_id = p.id AND own.user_id = $3::uuid
		)))
		GROUP BY p.id
		ORDER BY p.created_at DESC, p.id DESC
	`, tenantID, role, actorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Project, 0)
	for rows.Next() {
		var item Project
		if err := rows.Scan(&item.ID, &item.TenantID, &item.Name, &item.Slug, &item.Status, &item.CreatedBy, &item.MemberCount, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *SQLAdminService) CreateProject(ctx context.Context, actorID, tenantID string, request ProjectMutation) (Project, error) {
	request, err := validateProject(request)
	if err != nil || !validUUID(actorID) || !validUUID(tenantID) {
		return Project{}, ErrProjectInvalid
	}
	if err := s.requireTenantManager(ctx, tenantID, actorID); err != nil {
		return Project{}, err
	}
	id, err := ids.New()
	if err != nil {
		return Project{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Project{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockTenantMembership(ctx, tx, tenantID); err != nil {
		return Project{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO projects (id, tenant_id, name, slug, status, created_by) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6::uuid)`, id, tenantID, request.Name, request.Slug, request.Status, actorID); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "projects_tenant_slug_unique") {
			return Project{}, ErrProjectExists
		}
		return Project{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO project_members (project_id, user_id, role_code) VALUES ($1::uuid, $2::uuid, 'project_admin') ON CONFLICT DO NOTHING`, id, actorID); err != nil {
		return Project{}, err
	}
	if err = tx.Commit(); err != nil {
		return Project{}, err
	}
	return s.getProject(ctx, tenantID, id)
}

func (s *SQLAdminService) UpdateProject(ctx context.Context, actorID, tenantID, projectID string, request ProjectMutation) (Project, error) {
	request, err := validateProject(request)
	if err != nil || !validUUID(actorID) || !validUUID(tenantID) || !validUUID(projectID) {
		return Project{}, ErrProjectInvalid
	}
	if err := s.requireTenantManager(ctx, tenantID, actorID); err != nil {
		return Project{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Project{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockProjectMembership(ctx, tx, projectID); err != nil {
		return Project{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE projects SET name = $3, slug = $4, status = $5, updated_at = now() WHERE id = $1::uuid AND tenant_id = $2::uuid AND deleted_at IS NULL`, projectID, tenantID, request.Name, request.Slug, request.Status)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "projects_tenant_slug_unique") {
			return Project{}, ErrProjectExists
		}
		return Project{}, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return Project{}, ErrProjectNotFound
	}
	if request.Status == "disabled" {
		if _, err := tx.ExecContext(ctx, `UPDATE api_tokens SET status = 'revoked', revoked_at = COALESCE(revoked_at, now()) WHERE project_id = $1::uuid AND status = 'active'`, projectID); err != nil {
			return Project{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Project{}, err
	}
	return s.getProject(ctx, tenantID, projectID)
}

func (s *SQLAdminService) DeleteProject(ctx context.Context, actorID, tenantID, projectID string) error {
	if !validUUID(actorID) || !validUUID(tenantID) || !validUUID(projectID) {
		return ErrProjectInvalid
	}
	if err := s.requireTenantManager(ctx, tenantID, actorID); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockProjectMembership(ctx, tx, projectID); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE projects SET status = 'disabled', deleted_at = now(), updated_at = now() WHERE id = $1::uuid AND tenant_id = $2::uuid AND deleted_at IS NULL`, projectID, tenantID)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return ErrProjectNotFound
	}
	if _, err := tx.ExecContext(ctx, `UPDATE api_tokens SET status = 'revoked', revoked_at = COALESCE(revoked_at, now()) WHERE project_id = $1::uuid AND status = 'active'`, projectID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLAdminService) ListProjectMembers(ctx context.Context, actorID, tenantID, projectID string) ([]ProjectMember, error) {
	if !validUUID(actorID) || !validUUID(tenantID) || !validUUID(projectID) {
		return nil, ErrProjectInvalid
	}
	if err := s.requireProjectManager(ctx, tenantID, projectID, actorID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT pm.project_id::text, pm.user_id::text, u.email, u.display_name, pm.role_code, pm.created_at FROM project_members pm JOIN users u ON u.id = pm.user_id AND u.status = 'active' AND u.deleted_at IS NULL JOIN tenant_members tm ON tm.tenant_id = $1::uuid AND tm.user_id = pm.user_id AND tm.status = 'active' JOIN projects p ON p.id = pm.project_id AND p.tenant_id = $1::uuid AND p.status = 'active' AND p.deleted_at IS NULL WHERE pm.project_id = $2::uuid ORDER BY pm.created_at, pm.user_id`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ProjectMember, 0)
	for rows.Next() {
		var item ProjectMember
		if err := rows.Scan(&item.ProjectID, &item.UserID, &item.Email, &item.DisplayName, &item.Role, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *SQLAdminService) AddProjectMember(ctx context.Context, actorID, tenantID, projectID string, request ProjectMemberMutation) (ProjectMember, error) {
	if !validUUID(actorID) || !validUUID(tenantID) || !validUUID(projectID) || !validProjectRole(request.Role) {
		return ProjectMember{}, ErrMemberInvalid
	}
	if err := s.requireProjectManager(ctx, tenantID, projectID, actorID); err != nil {
		return ProjectMember{}, err
	}
	request.UserID = strings.TrimSpace(request.UserID)
	request.Email = normalizeEmail(request.Email)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ProjectMember{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockProjectMembership(ctx, tx, projectID); err != nil {
		return ProjectMember{}, err
	}
	var userID string
	if request.UserID != "" {
		if !validUUID(request.UserID) {
			return ProjectMember{}, ErrMemberInvalid
		}
		if err := tx.QueryRowContext(ctx, `SELECT id::text FROM users WHERE id = $1::uuid AND status = 'active' AND deleted_at IS NULL`, request.UserID).Scan(&userID); errors.Is(err, sql.ErrNoRows) {
			return ProjectMember{}, ErrMemberNotFound
		} else if err != nil {
			return ProjectMember{}, err
		}
	} else if validEmail(request.Email) {
		if err := tx.QueryRowContext(ctx, `SELECT id::text FROM users WHERE lower(email) = $1 AND status = 'active' AND deleted_at IS NULL`, request.Email).Scan(&userID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ProjectMember{}, ErrMemberNotFound
			}
			return ProjectMember{}, err
		}
	} else {
		return ProjectMember{}, ErrMemberInvalid
	}
	var tenantMember bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM tenant_members WHERE tenant_id = $1::uuid AND user_id = $2::uuid AND status = 'active')`, tenantID, userID).Scan(&tenantMember); err != nil {
		return ProjectMember{}, err
	}
	if !tenantMember {
		return ProjectMember{}, ErrMemberNotFound
	}
	var projectExists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM projects WHERE id = $1::uuid AND tenant_id = $2::uuid AND status = 'active' AND deleted_at IS NULL)`, projectID, tenantID).Scan(&projectExists); err != nil {
		return ProjectMember{}, err
	}
	if !projectExists {
		return ProjectMember{}, ErrProjectNotFound
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM project_members WHERE project_id = $1::uuid AND user_id = $2::uuid)`, projectID, userID).Scan(&exists); err != nil {
		return ProjectMember{}, err
	}
	if exists {
		return ProjectMember{}, ErrProjectMemberExists
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO project_members (project_id, user_id, role_code) VALUES ($1::uuid, $2::uuid, $3)`, projectID, userID, request.Role); err != nil {
		return ProjectMember{}, err
	}
	if err := tx.Commit(); err != nil {
		return ProjectMember{}, err
	}
	return s.getProjectMember(ctx, projectID, userID)
}

func (s *SQLAdminService) UpdateProjectMember(ctx context.Context, actorID, tenantID, projectID, userID string, request ProjectMemberMutation) (ProjectMember, error) {
	if !validUUID(actorID) || !validUUID(tenantID) || !validUUID(projectID) || !validUUID(userID) || !validProjectRole(request.Role) {
		return ProjectMember{}, ErrMemberInvalid
	}
	if err := s.requireProjectManager(ctx, tenantID, projectID, actorID); err != nil {
		return ProjectMember{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ProjectMember{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockProjectMembership(ctx, tx, projectID); err != nil {
		return ProjectMember{}, err
	}
	var currentRole string
	if err := tx.QueryRowContext(ctx, `SELECT pm.role_code FROM project_members pm JOIN projects p ON p.id = pm.project_id WHERE pm.project_id = $1::uuid AND pm.user_id = $2::uuid AND p.tenant_id = $3::uuid AND p.status = 'active' AND p.deleted_at IS NULL FOR UPDATE`, projectID, userID, tenantID).Scan(&currentRole); errors.Is(err, sql.ErrNoRows) {
		return ProjectMember{}, ErrProjectMemberNotFound
	} else if err != nil {
		return ProjectMember{}, err
	}
	if currentRole == "project_admin" && request.Role != "project_admin" {
		var otherAdmins int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*)::int FROM project_members pm JOIN tenant_members tm ON tm.tenant_id = (SELECT tenant_id FROM projects WHERE id = pm.project_id) AND tm.user_id = pm.user_id AND tm.status = 'active' JOIN users u ON u.id = pm.user_id AND u.status = 'active' AND u.deleted_at IS NULL WHERE pm.project_id = $1::uuid AND pm.user_id <> $2::uuid AND pm.role_code = 'project_admin'`, projectID, userID).Scan(&otherAdmins); err != nil {
			return ProjectMember{}, err
		}
		if otherAdmins == 0 {
			return ProjectMember{}, ErrLastProjectAdmin
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE project_members pm SET role_code = $3 FROM projects p WHERE pm.project_id = $1::uuid AND pm.user_id = $2::uuid AND p.id = pm.project_id AND p.tenant_id = $4::uuid AND p.status = 'active' AND p.deleted_at IS NULL`, projectID, userID, request.Role, tenantID)
	if err != nil {
		return ProjectMember{}, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return ProjectMember{}, ErrProjectMemberNotFound
	}
	if request.Role == "viewer" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE api_tokens
			SET status = 'revoked', revoked_at = COALESCE(revoked_at, now())
			WHERE project_id = $1::uuid AND created_by = $2::uuid AND status = 'active'
		`, projectID, userID); err != nil {
			return ProjectMember{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return ProjectMember{}, err
	}
	return s.getProjectMember(ctx, projectID, userID)
}

func (s *SQLAdminService) RemoveProjectMember(ctx context.Context, actorID, tenantID, projectID, userID string) error {
	if !validUUID(actorID) || !validUUID(tenantID) || !validUUID(projectID) || !validUUID(userID) {
		return ErrMemberInvalid
	}
	if err := s.requireProjectManager(ctx, tenantID, projectID, actorID); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockProjectMembership(ctx, tx, projectID); err != nil {
		return err
	}
	var currentRole string
	if err := tx.QueryRowContext(ctx, `SELECT pm.role_code FROM project_members pm JOIN projects p ON p.id = pm.project_id WHERE pm.project_id = $1::uuid AND pm.user_id = $2::uuid AND p.tenant_id = $3::uuid AND p.status = 'active' AND p.deleted_at IS NULL FOR UPDATE`, projectID, userID, tenantID).Scan(&currentRole); errors.Is(err, sql.ErrNoRows) {
		return ErrProjectMemberNotFound
	} else if err != nil {
		return err
	}
	if currentRole == "project_admin" {
		var otherAdmins int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*)::int FROM project_members pm JOIN tenant_members tm ON tm.tenant_id = (SELECT tenant_id FROM projects WHERE id = pm.project_id) AND tm.user_id = pm.user_id AND tm.status = 'active' JOIN users u ON u.id = pm.user_id AND u.status = 'active' AND u.deleted_at IS NULL WHERE pm.project_id = $1::uuid AND pm.user_id <> $2::uuid AND pm.role_code = 'project_admin'`, projectID, userID).Scan(&otherAdmins); err != nil {
			return err
		}
		if otherAdmins == 0 {
			return ErrLastProjectAdmin
		}
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM project_members pm USING projects p WHERE pm.project_id = $1::uuid AND pm.user_id = $2::uuid AND p.id = pm.project_id AND p.tenant_id = $3::uuid AND p.status = 'active' AND p.deleted_at IS NULL`, projectID, userID, tenantID)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return ErrProjectMemberNotFound
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE api_tokens
		SET status = 'revoked', revoked_at = COALESCE(revoked_at, now())
		WHERE project_id = $1::uuid AND created_by = $2::uuid AND status = 'active'
	`, projectID, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLAdminService) tenantRole(ctx context.Context, tenantID, userID string) (string, error) {
	if s == nil || s.db == nil {
		return "", ErrUnavailable
	}
	return tenantRoleDB(ctx, s.db, tenantID, userID)
}
func (s *SQLAdminService) requireTenantManager(ctx context.Context, tenantID, userID string) error {
	role, err := s.tenantRole(ctx, tenantID, userID)
	if err != nil {
		return err
	}
	if role != "tenant_owner" && role != "tenant_admin" {
		return ErrTenantAccessDenied
	}
	return nil
}

func (s *SQLAdminService) requireProjectManager(ctx context.Context, tenantID, projectID, userID string) error {
	role, err := s.tenantRole(ctx, tenantID, userID)
	if err != nil {
		return err
	}
	if role == "tenant_owner" || role == "tenant_admin" {
		return nil
	}
	var allowed bool
	err = s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM project_members pm
			JOIN projects p ON p.id = pm.project_id
			WHERE pm.project_id = $1::uuid AND pm.user_id = $2::uuid
			  AND pm.role_code = 'project_admin'
			  AND p.tenant_id = $3::uuid AND p.status = 'active' AND p.deleted_at IS NULL
		)
	`, projectID, userID, tenantID).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrTenantAccessDenied
	}
	return nil
}
func tenantRoleDB(ctx context.Context, db *sql.DB, tenantID, userID string) (string, error) {
	if !validUUID(tenantID) || !validUUID(userID) {
		return "", ErrTenantAccessDenied
	}
	var role string
	err := db.QueryRowContext(ctx, `SELECT tm.role_code FROM tenant_members tm JOIN tenants t ON t.id = tm.tenant_id AND t.status = 'active' AND t.deleted_at IS NULL JOIN users u ON u.id = tm.user_id AND u.status = 'active' AND u.deleted_at IS NULL WHERE tm.tenant_id = $1::uuid AND tm.user_id = $2::uuid AND tm.status = 'active'`, tenantID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrTenantAccessDenied
	}
	return role, err
}
func tenantRoleTx(ctx context.Context, tx *sql.Tx, tenantID, userID string) (string, error) {
	var role string
	err := tx.QueryRowContext(ctx, `SELECT tm.role_code FROM tenant_members tm JOIN tenants t ON t.id = tm.tenant_id AND t.status = 'active' AND t.deleted_at IS NULL JOIN users u ON u.id = tm.user_id AND u.status = 'active' AND u.deleted_at IS NULL WHERE tm.tenant_id = $1::uuid AND tm.user_id = $2::uuid AND tm.status = 'active'`, tenantID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrTenantAccessDenied
	}
	return role, err
}

func lockTenantMembership(ctx context.Context, tx *sql.Tx, tenantID string) error {
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('ai-token:tenant-members:' || $1, 0))`, tenantID)
	return err
}

func lockProjectMembership(ctx context.Context, tx *sql.Tx, projectID string) error {
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('ai-token:project-members:' || $1, 0))`, projectID)
	return err
}
func canManageRole(actorRole, targetRole string) bool {
	return actorRole == "tenant_owner" || (actorRole == "tenant_admin" && targetRole != "tenant_owner" && targetRole != "tenant_admin")
}
func validTenantMemberRole(role string) bool {
	return role == "tenant_owner" || role == "tenant_admin" || role == "developer" || role == "viewer"
}
func validProjectRole(role string) bool {
	return role == "project_admin" || role == "developer" || role == "viewer"
}

func (s *SQLAdminService) getMember(ctx context.Context, tenantID, userID string) (TenantMember, error) {
	var item TenantMember
	err := s.db.QueryRowContext(ctx, `SELECT tm.tenant_id::text, tm.user_id::text, u.email, u.display_name, tm.role_code, tm.status, (SELECT COUNT(*)::int FROM project_members pm JOIN projects p ON p.id = pm.project_id AND p.tenant_id = tm.tenant_id AND p.deleted_at IS NULL WHERE pm.user_id = tm.user_id), tm.created_at FROM tenant_members tm JOIN users u ON u.id = tm.user_id WHERE tm.tenant_id = $1::uuid AND tm.user_id = $2::uuid AND tm.status <> 'removed'`, tenantID, userID).Scan(&item.TenantID, &item.UserID, &item.Email, &item.DisplayName, &item.Role, &item.Status, &item.ProjectCount, &item.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return TenantMember{}, ErrMemberNotFound
	}
	return item, err
}
func (s *SQLAdminService) getProject(ctx context.Context, tenantID, projectID string) (Project, error) {
	var item Project
	err := s.db.QueryRowContext(ctx, `SELECT p.id::text, p.tenant_id::text, p.name, p.slug, p.status, p.created_by::text, (SELECT COUNT(*)::int FROM project_members pm WHERE pm.project_id = p.id), p.created_at, p.updated_at FROM projects p WHERE p.id = $1::uuid AND p.tenant_id = $2::uuid AND p.deleted_at IS NULL`, projectID, tenantID).Scan(&item.ID, &item.TenantID, &item.Name, &item.Slug, &item.Status, &item.CreatedBy, &item.MemberCount, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, ErrProjectNotFound
	}
	return item, err
}
func (s *SQLAdminService) getProjectMember(ctx context.Context, projectID, userID string) (ProjectMember, error) {
	var item ProjectMember
	err := s.db.QueryRowContext(ctx, `SELECT pm.project_id::text, pm.user_id::text, u.email, u.display_name, pm.role_code, pm.created_at FROM project_members pm JOIN users u ON u.id = pm.user_id AND u.status = 'active' AND u.deleted_at IS NULL JOIN projects p ON p.id = pm.project_id AND p.status = 'active' AND p.deleted_at IS NULL WHERE pm.project_id = $1::uuid AND pm.user_id = $2::uuid`, projectID, userID).Scan(&item.ProjectID, &item.UserID, &item.Email, &item.DisplayName, &item.Role, &item.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ProjectMember{}, ErrProjectMemberNotFound
	}
	return item, err
}

func validateProject(request ProjectMutation) (ProjectMutation, error) {
	request.Name = strings.TrimSpace(request.Name)
	request.Slug = strings.ToLower(strings.TrimSpace(request.Slug))
	request.Status = strings.ToLower(strings.TrimSpace(request.Status))
	if request.Status == "" {
		request.Status = "active"
	}
	if request.Name == "" || len(request.Name) > 128 || !validProjectSlug(request.Slug) || (request.Status != "active" && request.Status != "disabled") {
		return ProjectMutation{}, ErrProjectInvalid
	}
	return request, nil
}
func validProjectSlug(value string) bool {
	if len(value) < 1 || len(value) > 64 || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, c := range value[1:] {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' && c != '_' {
			return false
		}
	}
	return true
}
