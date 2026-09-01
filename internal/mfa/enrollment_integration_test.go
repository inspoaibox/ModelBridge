package mfa

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	dbpkg "ai-token/internal/db"
	"ai-token/internal/ids"
)

func TestEnrollmentLifecycleWithPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not configured")
	}

	ctx := context.Background()
	conn, err := dbpkg.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}

	userID, err := ids.New()
	if err != nil {
		t.Fatal(err)
	}
	cleanup := func() {
		_, _ = conn.ExecContext(ctx, `DELETE FROM mfa_credentials WHERE user_id = $1`, userID)
		_, _ = conn.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)
	}
	t.Cleanup(func() { _ = conn.Close() })
	t.Cleanup(cleanup)
	if _, err := conn.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, display_name, status)
		VALUES ($1, $2, 'test-password-hash', 'TOTP Test User', 'active')
	`, userID, "totp-test-"+strings.ReplaceAll(userID, "-", "")+"@example.invalid"); err != nil {
		t.Fatal(err)
	}

	box, err := NewSecretBox(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewEnrollmentService(conn, box, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	enrollment, err := service.Begin(ctx, userID, "AI Token Gateway", "totp-test@example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	if enrollment.Secret == "" || !strings.HasPrefix(enrollment.OTPAuthURL, "otpauth://totp/") {
		t.Fatalf("unexpected enrollment payload: %#v", enrollment)
	}
	if got, want := strings.ToUpper(enrollment.Secret), strings.ToUpper(secretFromURL(t, enrollment.OTPAuthURL)); got != want {
		t.Fatalf("otpauth URL secret mismatch: got %q, want %q", got, want)
	}

	code, err := Code(enrollment.Secret, time.Now(), 6)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Confirm(ctx, userID, enrollment.ID, "000000"); !errors.Is(err, ErrMFAInvalidCode) {
		t.Fatalf("expected invalid enrollment code, got %v", err)
	}
	if err := service.Confirm(ctx, userID, enrollment.ID, code); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Begin(ctx, userID, "AI Token Gateway", "totp-test@example.invalid"); !errors.Is(err, ErrMFAAlreadyEnabled) {
		t.Fatalf("expected duplicate enrollment to be rejected, got %v", err)
	}
	status, err := service.Status(ctx, userID)
	if err != nil || !status.Enabled {
		t.Fatalf("expected enabled status, got %#v, %v", status, err)
	}
	if err := service.Verify(ctx, userID, code); err != nil {
		t.Fatalf("expected step-up TOTP verification, got %v", err)
	}
	if err := service.Disable(ctx, userID, "000000"); !errors.Is(err, ErrMFAInvalidCode) {
		t.Fatalf("expected invalid disable code, got %v", err)
	}
	if err := service.Disable(ctx, userID, code); err != nil {
		t.Fatal(err)
	}
	status, err = service.Status(ctx, userID)
	if err != nil || status.Enabled {
		t.Fatalf("expected disabled status, got %#v, %v", status, err)
	}
}

func secretFromURL(t *testing.T, value string) string {
	t.Helper()
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "otpauth" {
		t.Fatalf("invalid otpauth URL: %s", value)
	}
	secret := parsed.Query().Get("secret")
	if secret == "" {
		t.Fatal("otpauth URL has no secret")
	}
	return secret
}
