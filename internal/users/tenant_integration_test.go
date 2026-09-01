package users

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	dbpkg "ai-token/internal/db"
	"ai-token/internal/ids"
	"ai-token/internal/passwords"
)

func TestTenantProjectAndMemberLifecycle(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	conn, err := dbpkg.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ownerID, _ := ids.New()
	memberID, _ := ids.New()
	tenantID, _ := ids.New()
	ownerEmail := "tenant-owner-" + strings.ReplaceAll(ownerID, "-", "") + "@example.invalid"
	memberEmail := "tenant-member-" + strings.ReplaceAll(memberID, "-", "") + "@example.invalid"
	hash, err := passwords.Hash("tenant-test-password")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = conn.ExecContext(ctx, `DELETE FROM project_members WHERE user_id IN ($1, $2)`, ownerID, memberID)
		_, _ = conn.ExecContext(ctx, `DELETE FROM projects WHERE tenant_id = $1`, tenantID)
		_, _ = conn.ExecContext(ctx, `DELETE FROM tenant_members WHERE tenant_id = $1`, tenantID)
		_, _ = conn.ExecContext(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
		_, _ = conn.ExecContext(ctx, `DELETE FROM users WHERE id IN ($1, $2)`, ownerID, memberID)
	}()
	for _, item := range []struct{ id, email string }{{ownerID, ownerEmail}, {memberID, memberEmail}} {
		if _, err := conn.ExecContext(ctx, `INSERT INTO users (id, email, password_hash, display_name, status) VALUES ($1, $2, $3, 'Tenant test user', 'active')`, item.id, item.email, hash); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO tenants (id, name, slug, status) VALUES ($1, 'Tenant lifecycle', $2, 'active')`, tenantID, "tenant-"+strings.ReplaceAll(tenantID, "-", "")); err != nil {
		t.Fatal(err)
	}
	ownerMemberID, _ := ids.New()
	if _, err := conn.ExecContext(ctx, `INSERT INTO tenant_members (id, tenant_id, user_id, role_code, status, created_by) VALUES ($1, $2, $3, 'tenant_owner', 'active', $3)`, ownerMemberID, tenantID, ownerID); err != nil {
		t.Fatal(err)
	}
	service, err := NewAdminService(conn)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(ctx, ownerID, tenantID, ProjectMutation{Name: "Analytics", Slug: "analytics"})
	if err != nil {
		t.Fatal(err)
	}
	if project.ID == "" || project.MemberCount != 1 {
		t.Fatalf("project owner membership missing: %#v", project)
	}
	if _, err := service.AddMember(ctx, ownerID, tenantID, MemberMutation{Email: memberEmail, Role: "developer"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AddProjectMember(ctx, ownerID, tenantID, project.ID, ProjectMemberMutation{Email: memberEmail, Role: "viewer"}); err != nil {
		t.Fatal(err)
	}
	projectMembers, err := service.ListProjectMembers(ctx, ownerID, tenantID, project.ID)
	if err != nil || len(projectMembers) != 2 {
		t.Fatalf("expected two project members, got %#v, %v", projectMembers, err)
	}
	if _, err := service.UpdateProjectMember(ctx, ownerID, tenantID, project.ID, memberID, ProjectMemberMutation{Role: "developer"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.UpdateMember(ctx, ownerID, tenantID, memberID, MemberMutation{Role: "developer", Status: "suspended"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateProject(ctx, memberID, tenantID, ProjectMutation{Name: "Forbidden", Slug: "forbidden"}); !errors.Is(err, ErrTenantAccessDenied) {
		t.Fatalf("developer must not create project: %v", err)
	}
	if err := service.RemoveMember(ctx, ownerID, tenantID, ownerID); !errors.Is(err, ErrTenantAccessDenied) {
		t.Fatalf("owner must not remove self: %v", err)
	}
	if err := service.RemoveMember(ctx, ownerID, tenantID, memberID); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteProject(ctx, ownerID, tenantID, project.ID); err != nil {
		t.Fatal(err)
	}
}
