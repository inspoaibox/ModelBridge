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
	"ai-token/internal/passwords"
	"ai-token/internal/tokens"
)

var ErrInvalidResetToken = errors.New("invalid password reset token")

type PasswordResetProvider interface {
	Request(ctx context.Context, email, ip string) (string, bool, error)
	Confirm(ctx context.Context, token, newPassword string) error
}

type PasswordResetNotifier interface {
	SendPasswordReset(ctx context.Context, email, token string) error
}

type SQLPasswordResetService struct {
	db          *sql.DB
	tokenHasher *tokens.Hasher
	throttle    *RequestThrottle
	ttl         time.Duration
	now         func() time.Time
}

func NewSQLPasswordResetService(db *sql.DB, tokenHasher *tokens.Hasher, ttl time.Duration, maxRequests int, window, lockFor time.Duration) (*SQLPasswordResetService, error) {
	throttle, err := NewRequestThrottle(db, tokenHasher, maxRequests, window, lockFor)
	if err != nil || ttl <= 0 {
		return nil, errors.New("invalid password reset configuration")
	}
	return &SQLPasswordResetService{
		db:          db,
		tokenHasher: tokenHasher,
		throttle:    throttle,
		ttl:         ttl,
		now:         time.Now,
	}, nil
}

func (s *SQLPasswordResetService) Request(ctx context.Context, email, ip string) (string, bool, error) {
	if s == nil || s.db == nil || s.tokenHasher == nil || s.throttle == nil {
		return "", false, errors.New("password reset service is not configured")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return "", false, nil
	}
	ip = strings.TrimSpace(ip)
	if ip == "" {
		ip = "unknown"
	}
	allowed, err := s.throttle.Allow(ctx, "password-reset-ip", ip)
	if err != nil {
		return "", false, err
	}
	if !allowed {
		return "", false, nil
	}
	allowed, err = s.throttle.Allow(ctx, "password-reset-email", email)
	if err != nil {
		return "", false, err
	}
	if !allowed {
		return "", false, nil
	}

	var userID, status string
	err = s.db.QueryRowContext(ctx, `
		SELECT id::text, status
		FROM users
		WHERE lower(email) = $1 AND deleted_at IS NULL
	`, email).Scan(&userID, &status)
	if errors.Is(err, sql.ErrNoRows) || status != "active" {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", false, err
	}
	plain := "reset_" + base64.RawURLEncoding.EncodeToString(raw)
	tokenID, err := ids.New()
	if err != nil {
		return "", false, err
	}
	expiresAt := s.now().Add(s.ttl)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO password_reset_tokens (
			id, user_id, token_hash, requested_ip_hash, expires_at
		) VALUES ($1, $2, $3, $4, $5)
	`, tokenID, userID, s.tokenHasher.Digest(plain), s.tokenHasher.Digest(ip), expiresAt)
	if err != nil {
		return "", false, err
	}
	return plain, true, nil
}

func (s *SQLPasswordResetService) Confirm(ctx context.Context, token, newPassword string) error {
	if s == nil || s.db == nil || s.tokenHasher == nil {
		return errors.New("password reset service is not configured")
	}
	if !strings.HasPrefix(token, "reset_") {
		return ErrInvalidResetToken
	}
	passwordHash, err := passwords.Hash(newPassword)
	if err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var tokenID, userID, email string
	err = tx.QueryRowContext(ctx, `
		SELECT prt.id::text, prt.user_id::text, u.email
		FROM password_reset_tokens prt
		JOIN users u ON u.id = prt.user_id
		WHERE prt.token_hash = $1
		  AND prt.used_at IS NULL
		  AND prt.expires_at > now()
		  FOR UPDATE
	`, s.tokenHasher.Digest(token)).Scan(&tokenID, &userID, &email)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInvalidResetToken
	}
	if err != nil {
		return err
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE users
		SET password_hash = $1, password_changed_at = now(), updated_at = now()
		WHERE id = $2 AND status = 'active' AND deleted_at IS NULL
	`, passwordHash, userID)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		if err != nil {
			return err
		}
		return ErrInvalidResetToken
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE password_reset_tokens SET used_at = now() WHERE id = $1
	`, tokenID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE web_sessions SET revoked_at = COALESCE(revoked_at, now())
		WHERE user_id = $1 AND revoked_at IS NULL
	`, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM login_throttles
		WHERE subject_hash = $1
	`, s.tokenHasher.Digest(strings.ToLower(strings.TrimSpace(email)))); err != nil {
		return err
	}
	return tx.Commit()
}
