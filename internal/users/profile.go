package users

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"ai-token/internal/passwords"
)

var (
	ErrCurrentPasswordInvalid = errors.New("current password is invalid")
	ErrPasswordInvalid        = errors.New("new password is invalid")
)

type Profile struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	DisplayName string     `json:"display_name"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
}

type ProfileService interface {
	GetProfile(context.Context, string) (Profile, error)
	UpdateProfile(context.Context, string, string) (Profile, error)
	ChangeEmail(context.Context, string, string, string) (Profile, error)
	ChangePassword(context.Context, string, string, string) error
}

func (s *SQLAdminService) GetProfile(ctx context.Context, userID string) (Profile, error) {
	if s == nil || s.db == nil {
		return Profile{}, ErrUnavailable
	}
	userID = strings.TrimSpace(userID)
	if !validUUID(userID) {
		return Profile{}, ErrInvalid
	}
	var profile Profile
	var lastLogin sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT id::text, email, display_name, status, created_at, last_login_at
		FROM users
		WHERE id = $1::uuid AND deleted_at IS NULL
	`, userID).Scan(
		&profile.ID,
		&profile.Email,
		&profile.DisplayName,
		&profile.Status,
		&profile.CreatedAt,
		&lastLogin,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, err
	}
	if lastLogin.Valid {
		value := lastLogin.Time
		profile.LastLoginAt = &value
	}
	return profile, nil
}

func (s *SQLAdminService) UpdateProfile(ctx context.Context, userID, displayName string) (Profile, error) {
	if s == nil || s.db == nil {
		return Profile{}, ErrUnavailable
	}
	userID = strings.TrimSpace(userID)
	displayName = strings.TrimSpace(displayName)
	if !validUUID(userID) || displayName == "" || len(displayName) > 100 {
		return Profile{}, ErrInvalid
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE users
		SET display_name = $1, updated_at = now()
		WHERE id = $2::uuid AND deleted_at IS NULL
	`, displayName, userID)
	if err != nil {
		return Profile{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return Profile{}, err
	}
	if affected != 1 {
		return Profile{}, ErrNotFound
	}
	return s.GetProfile(ctx, userID)
}

func (s *SQLAdminService) ChangeEmail(ctx context.Context, userID, currentPassword, email string) (Profile, error) {
	if s == nil || s.db == nil {
		return Profile{}, ErrUnavailable
	}
	userID = strings.TrimSpace(userID)
	email = normalizeEmail(email)
	if !validUUID(userID) || currentPassword == "" || !validEmail(email) {
		return Profile{}, ErrInvalid
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Profile{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var currentEmail, passwordHash, status string
	if err := tx.QueryRowContext(ctx, `
		SELECT email, password_hash, status
		FROM users
		WHERE id = $1::uuid AND deleted_at IS NULL
		FOR UPDATE
	`, userID).Scan(&currentEmail, &passwordHash, &status); errors.Is(err, sql.ErrNoRows) {
		return Profile{}, ErrNotFound
	} else if err != nil {
		return Profile{}, err
	}
	if status != "active" || !passwords.Verify(currentPassword, passwordHash) {
		return Profile{}, ErrCurrentPasswordInvalid
	}
	if strings.EqualFold(currentEmail, email) {
		if err := tx.Commit(); err != nil {
			return Profile{}, err
		}
		return s.GetProfile(ctx, userID)
	}

	var exists bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM users
			WHERE lower(email) = $1 AND id <> $2::uuid AND deleted_at IS NULL
		)
	`, email, userID).Scan(&exists); err != nil {
		return Profile{}, err
	}
	if exists {
		return Profile{}, ErrEmailExists
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE users
		SET email = $1, updated_at = now()
		WHERE id = $2::uuid AND deleted_at IS NULL
	`, email, userID); err != nil {
		return Profile{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE web_sessions
		SET revoked_at = COALESCE(revoked_at, now())
		WHERE user_id = $1::uuid AND revoked_at IS NULL
	`, userID); err != nil {
		return Profile{}, err
	}
	if err := tx.Commit(); err != nil {
		return Profile{}, err
	}
	return s.GetProfile(ctx, userID)
}

func (s *SQLAdminService) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	userID = strings.TrimSpace(userID)
	if !validUUID(userID) || currentPassword == "" || newPassword == "" {
		return ErrInvalid
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var passwordHash, status string
	if err := tx.QueryRowContext(ctx, `
		SELECT password_hash, status
		FROM users
		WHERE id = $1::uuid AND deleted_at IS NULL
		FOR UPDATE
	`, userID).Scan(&passwordHash, &status); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if status != "active" || !passwords.Verify(currentPassword, passwordHash) {
		return ErrCurrentPasswordInvalid
	}
	newHash, err := passwords.Hash(newPassword)
	if err != nil {
		return ErrPasswordInvalid
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE users
		SET password_hash = $1, password_changed_at = now(), updated_at = now()
		WHERE id = $2::uuid AND deleted_at IS NULL
	`, newHash, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE web_sessions
		SET revoked_at = COALESCE(revoked_at, now())
		WHERE user_id = $1::uuid AND revoked_at IS NULL
	`, userID); err != nil {
		return err
	}
	return tx.Commit()
}
