package auth

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	dbpkg "ai-token/internal/db"
	"ai-token/internal/ids"
	"ai-token/internal/passwords"
	"ai-token/internal/tokens"
)

func TestSQLResolverBindsAPITokenToCreatorAndTenantMembership(t *testing.T) {
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

	userID, _ := ids.New()
	tenantID, _ := ids.New()
	projectID, _ := ids.New()
	email := "resolver-" + strings.ReplaceAll(userID, "-", "") + "@example.invalid"
	hasher, err := tokens.NewHasher("resolver-token-pepper-012345678901234567890")
	if err != nil {
		t.Fatal(err)
	}
	sessionHasher, err := tokens.NewHasher("resolver-session-pepper-012345678901234567890")
	if err != nil {
		t.Fatal(err)
	}
	passwordHash, err := passwords.Hash("resolver-test-password")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = conn.ExecContext(ctx, `DELETE FROM api_tokens WHERE created_by = $1`, userID)
		_, _ = conn.ExecContext(ctx, `DELETE FROM project_members WHERE project_id = $1`, projectID)
		_, _ = conn.ExecContext(ctx, `DELETE FROM projects WHERE id = $1`, projectID)
		_, _ = conn.ExecContext(ctx, `DELETE FROM tenant_members WHERE tenant_id = $1`, tenantID)
		_, _ = conn.ExecContext(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
		_, _ = conn.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)
	}()
	if _, err := conn.ExecContext(ctx, `INSERT INTO users (id, email, password_hash, display_name, status) VALUES ($1, $2, $3, 'Resolver test', 'active')`, userID, email, passwordHash); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO tenants (id, name, slug, status) VALUES ($1, 'Resolver tenant', $2, 'active')`, tenantID, "resolver-"+strings.ReplaceAll(tenantID, "-", "")); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO projects (id, tenant_id, name, slug, status, created_by) VALUES ($1, $2, 'Resolver project', 'resolver', 'active', $3)`, projectID, tenantID, userID); err != nil {
		t.Fatal(err)
	}
	memberID, _ := ids.New()
	if _, err := conn.ExecContext(ctx, `INSERT INTO tenant_members (id, tenant_id, user_id, role_code, status, created_by) VALUES ($1, $2, $3, 'developer', 'active', $3)`, memberID, tenantID, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO project_members (project_id, user_id, role_code) VALUES ($1, $2, 'developer')`, projectID, userID); err != nil {
		t.Fatal(err)
	}
	issuer, err := tokens.NewIssuer(conn, hasher)
	if err != nil {
		t.Fatal(err)
	}
	issued, err := issuer.Issue(ctx, tenantID, projectID, userID, "resolver token", []string{"model:use"}, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := NewSQLResolver(conn, hasher, sessionHasher)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Resolve(ctx, issued.Plaintext); err != nil {
		t.Fatalf("active creator token should resolve: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `UPDATE users SET status = 'disabled' WHERE id = $1`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Resolve(ctx, issued.Plaintext); err == nil {
		t.Fatal("disabled creator token must not resolve")
	}
	if _, err := conn.ExecContext(ctx, `UPDATE users SET status = 'active' WHERE id = $1`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `UPDATE tenant_members SET status = 'suspended' WHERE tenant_id = $1 AND user_id = $2`, tenantID, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Resolve(ctx, issued.Plaintext); err == nil {
		t.Fatal("suspended tenant membership token must not resolve")
	}
}
