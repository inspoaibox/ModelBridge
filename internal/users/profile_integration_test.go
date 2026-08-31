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

func TestProfileChangesVerifyPasswordAndRevokeSessions(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not configured")
	}

	ctx := context.Background()
	conn, err := dbpkg.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	userID, err := ids.New()
	if err != nil {
		t.Fatal(err)
	}
	sessionOne, err := ids.New()
	if err != nil {
		t.Fatal(err)
	}
	sessionTwo, err := ids.New()
	if err != nil {
		t.Fatal(err)
	}
	email := "profile-test-" + strings.ReplaceAll(userID, "-", "") + "@example.invalid"
	newEmail := "profile-updated-" + strings.ReplaceAll(userID, "-", "") + "@example.invalid"
	oldPassword := "profile-old-password-123"
	newPassword := "profile-new-password-456"
	passwordHash, err := passwords.Hash(oldPassword)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = conn.ExecContext(ctx, `DELETE FROM web_sessions WHERE user_id = $1`, userID)
		_, _ = conn.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})

	if _, err := conn.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, display_name, status)
		VALUES ($1, $2, $3, 'Profile Test User', 'active')
	`, userID, email, passwordHash); err != nil {
		t.Fatal(err)
	}
	for _, sessionID := range []string{sessionOne, sessionTwo} {
		if _, err := conn.ExecContext(ctx, `
			INSERT INTO web_sessions (id, user_id, audience, session_hash, auth_strength, expires_at, last_seen_at)
			VALUES ($1, $2, 'console', $3, 'password', $4, $4)
		`, sessionID, userID, "profile-session-"+sessionID, time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}

	service, err := NewAdminService(conn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ChangeEmail(ctx, userID, "wrong-password", newEmail); !errors.Is(err, ErrCurrentPasswordInvalid) {
		t.Fatalf("expected invalid current password, got %v", err)
	}
	profile, err := service.UpdateProfile(ctx, userID, "Updated Profile User")
	if err != nil || profile.DisplayName != "Updated Profile User" {
		t.Fatalf("expected profile update, got %#v, %v", profile, err)
	}
	profile, err = service.ChangeEmail(ctx, userID, oldPassword, newEmail)
	if err != nil || profile.Email != newEmail {
		t.Fatalf("expected email update, got %#v, %v", profile, err)
	}
	if err := service.ChangePassword(ctx, userID, oldPassword, newPassword); err != nil {
		t.Fatal(err)
	}
	var storedHash string
	if err := conn.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE id = $1`, userID).Scan(&storedHash); err != nil {
		t.Fatal(err)
	}
	if !passwords.Verify(newPassword, storedHash) {
		t.Fatal("new password was not stored as a valid hash")
	}
	var revoked int
	if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM web_sessions WHERE user_id = $1 AND revoked_at IS NOT NULL`, userID).Scan(&revoked); err != nil {
		t.Fatal(err)
	}
	if revoked != 2 {
		t.Fatalf("expected all sessions to be revoked, got %d", revoked)
	}
}

func TestSetStatusProtectsLastPlatformAdministrator(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not configured")
	}
	ctx := context.Background()
	conn, err := dbpkg.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	ownerID, err := ids.New()
	if err != nil {
		t.Fatal(err)
	}
	actorID, err := ids.New()
	if err != nil {
		t.Fatal(err)
	}
	roleID, err := ids.New()
	if err != nil {
		t.Fatal(err)
	}
	passwordHash, err := passwords.Hash("platform-admin-test-password")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = conn.ExecContext(ctx, `DELETE FROM platform_user_roles WHERE user_id IN ($1, $2)`, ownerID, actorID)
		_, _ = conn.ExecContext(ctx, `DELETE FROM platform_roles WHERE id = $1`, roleID)
		_, _ = conn.ExecContext(ctx, `DELETE FROM users WHERE id IN ($1, $2)`, ownerID, actorID)
	})
	for _, item := range []struct{ id, email string }{
		{ownerID, "last-owner-" + strings.ReplaceAll(ownerID, "-", "") + "@example.invalid"},
		{actorID, "admin-actor-" + strings.ReplaceAll(actorID, "-", "") + "@example.invalid"},
	} {
		if _, err := conn.ExecContext(ctx, `INSERT INTO users (id, email, password_hash, display_name, status) VALUES ($1, $2, $3, 'Platform admin test', 'active')`, item.id, item.email, passwordHash); err != nil {
			t.Fatal(err)
		}
	}
	roleCode := "platform-admin-test-" + strings.ReplaceAll(roleID[:8], "-", "")
	if _, err := conn.ExecContext(ctx, `INSERT INTO platform_roles (id, code, name, status) VALUES ($1, $2, 'Platform admin test', 'active')`, roleID, roleCode); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO platform_user_roles (user_id, role_id) VALUES ($1, $3), ($2, $3)`, ownerID, actorID, roleID); err != nil {
		t.Fatal(err)
	}
	// The fixture must be the only active platform-admin population in the
	// assertion. Existing bootstrap data is preserved and restored below.
	var existingAdminIDs []string
	rows, err := conn.QueryContext(ctx, `
		SELECT DISTINCT u.id::text
		FROM users u
		JOIN platform_user_roles ur ON ur.user_id = u.id
		JOIN platform_roles pr ON pr.id = ur.role_id AND pr.status = 'active'
		WHERE u.status = 'active' AND u.deleted_at IS NULL
		  AND u.id NOT IN ($1, $2)
	`, ownerID, actorID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		existingAdminIDs = append(existingAdminIDs, id)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if len(existingAdminIDs) > 0 {
		t.Skip("existing platform administrators are present; use an isolated database for this test")
	}
	for _, id := range existingAdminIDs {
		if _, err := conn.ExecContext(ctx, `UPDATE users SET status = 'disabled' WHERE id = $1`, id); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, id := range existingAdminIDs {
			_, _ = conn.ExecContext(ctx, `UPDATE users SET status = 'active' WHERE id = $1`, id)
		}
	})
	service, err := NewAdminService(conn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetStatus(ctx, actorID, ownerID, "disabled"); err != nil {
		t.Fatalf("expected one of two platform administrators to be disabled: %v", err)
	}
	if _, err := service.SetStatus(ctx, ownerID, actorID, "disabled"); !errors.Is(err, ErrLastPlatformAdmin) {
		t.Fatalf("expected last platform administrator protection, got %v", err)
	}
}
