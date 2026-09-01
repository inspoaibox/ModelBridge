package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"ai-token/internal/ids"
)

var (
	ErrEmailVerificationInvalid  = errors.New("email verification token is invalid")
	ErrEmailVerificationDelivery = errors.New("email verification delivery failed")
	ErrEmailVerificationRequired = errors.New("email verification is required")
)

type EmailVerificationNotifier interface {
	SendEmailVerification(context.Context, string, string) error
}

type EmailVerificationService interface {
	ConfirmEmail(context.Context, string) error
	RequestEmailVerification(context.Context, string, string) error
}

const emailVerificationTTL = 30 * time.Minute

func newEmailVerificationToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "verify_" + base64.RawURLEncoding.EncodeToString(raw), nil
}

func (s *SQLRegistrationService) ConfirmEmail(ctx context.Context, token string) error {
	if s == nil || s.db == nil || s.throttle == nil || s.throttle.hasher == nil {
		return ErrEmailVerificationInvalid
	}
	if s.notifier == nil {
		return ErrEmailVerificationRequired
	}
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, "verify_") || len(token) > 128 {
		return ErrEmailVerificationInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var userID string
	if err := tx.QueryRowContext(ctx, `
		SELECT evt.user_id::text
		FROM email_verification_tokens evt
		JOIN users u ON u.id = evt.user_id
		WHERE evt.token_hash = $1
		  AND evt.used_at IS NULL
		  AND evt.expires_at > now()
		  AND u.status = 'pending'
		  AND u.deleted_at IS NULL
		FOR UPDATE
	`, s.throttle.hasher.Digest(token)).Scan(&userID); errors.Is(err, sql.ErrNoRows) {
		return ErrEmailVerificationInvalid
	} else if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE users
		SET status = 'active', email_verified_at = now(), updated_at = now()
		WHERE id = $1::uuid AND status = 'pending' AND deleted_at IS NULL
	`, userID)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		if err != nil {
			return err
		}
		return ErrEmailVerificationInvalid
	}
	if _, err := tx.ExecContext(ctx, `UPDATE email_verification_tokens SET used_at = now() WHERE user_id = $1::uuid AND used_at IS NULL`, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLRegistrationService) RequestEmailVerification(ctx context.Context, email, clientIP string) error {
	if s == nil || s.db == nil || s.throttle == nil || s.notifier == nil {
		return ErrEmailVerificationRequired
	}
	enabled, err := notifierEmailEnabled(ctx, s.notifier, "email_verification")
	if err != nil {
		return err
	}
	if !enabled {
		return ErrEmailVerificationRequired
	}
	email = normalizeEmail(email)
	clientIP = strings.TrimSpace(clientIP)
	if clientIP == "" {
		clientIP = "unknown"
	}
	allowed, err := s.throttle.Allow(ctx, "email-verification-ip", clientIP)
	if err != nil {
		return err
	}
	if !allowed {
		return nil
	}
	allowed, err = s.throttle.Allow(ctx, "email-verification-email", email)
	if err != nil {
		return err
	}
	if !allowed {
		return nil
	}
	var userID, status string
	if err := s.db.QueryRowContext(ctx, `SELECT id::text, status FROM users WHERE lower(email) = $1 AND deleted_at IS NULL`, email).Scan(&userID, &status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if status != "pending" {
		return nil
	}
	token, err := s.issueEmailVerificationToken(ctx, userID, clientIP)
	if err != nil {
		return err
	}
	if err := s.notifier.SendEmailVerification(ctx, email, token); err != nil {
		return ErrEmailVerificationDelivery
	}
	return nil
}

func notifierEmailEnabled(ctx context.Context, notifier any, event string) (bool, error) {
	if notifier == nil {
		return false, nil
	}
	if provider, ok := notifier.(EmailFeatureProvider); ok {
		enabled, err := provider.EmailEnabled(ctx)
		if err != nil || !enabled {
			return enabled, err
		}
	}
	if provider, ok := notifier.(EmailEventFeatureProvider); ok {
		return provider.EmailFeatureEnabled(ctx, event)
	}
	return true, nil
}

func (s *SQLRegistrationService) issueEmailVerificationToken(ctx context.Context, userID, clientIP string) (string, error) {
	token, err := newEmailVerificationToken()
	if err != nil {
		return "", err
	}
	tokenID, err := ids.New()
	if err != nil {
		return "", err
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE email_verification_tokens
		SET used_at = COALESCE(used_at, now())
		WHERE user_id = $1::uuid AND used_at IS NULL
	`, userID)
	if err != nil {
		return "", err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO email_verification_tokens (id, user_id, token_hash, requested_ip_hash, expires_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5)
	`, tokenID, userID, s.throttle.hasher.Digest(token), s.throttle.hasher.Digest(clientIP), time.Now().Add(emailVerificationTTL))
	if err != nil {
		return "", err
	}
	return token, nil
}
