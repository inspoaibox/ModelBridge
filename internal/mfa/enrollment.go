package mfa

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"ai-token/internal/ids"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

var (
	ErrEnrollmentExpired  = errors.New("mfa enrollment expired")
	ErrEnrollmentInvalid  = errors.New("mfa enrollment is invalid")
	ErrMFAAlreadyEnabled  = errors.New("mfa is already enabled")
	ErrMFANotEnabled      = errors.New("mfa is not enabled")
	ErrMFAInvalidCode     = errors.New("mfa code is invalid")
	ErrMFAServiceDisabled = errors.New("mfa service is disabled")
	ErrMFAThrottled       = errors.New("mfa verification temporarily throttled")
)

const (
	stepUpMaxFailures = 5
	stepUpWindow      = 15 * time.Minute
	stepUpLockFor     = 15 * time.Minute
)

type Enrollment struct {
	ID         string
	Secret     string
	OTPAuthURL string
	ExpiresAt  time.Time
}

type Status struct {
	Enabled    bool       `json:"enabled"`
	EnrolledAt *time.Time `json:"enrolled_at,omitempty"`
}

type EnrollmentService struct {
	db  *sql.DB
	box *SecretBox
	ttl time.Duration
	now func() time.Time
}

func NewEnrollmentService(db *sql.DB, box *SecretBox, ttl time.Duration) (*EnrollmentService, error) {
	if db == nil || box == nil {
		return nil, errors.New("database and secret box are required")
	}
	if ttl <= 0 {
		return nil, errors.New("enrollment ttl must be positive")
	}
	return &EnrollmentService{
		db:  db,
		box: box,
		ttl: ttl,
		now: time.Now,
	}, nil
}

func (s *EnrollmentService) Begin(
	ctx context.Context,
	userID string,
	issuer string,
	account string,
) (Enrollment, error) {
	if s == nil || s.db == nil || s.box == nil {
		return Enrollment{}, errors.New("mfa enrollment service is not configured")
	}
	if !ids.Valid(strings.TrimSpace(userID)) || strings.TrimSpace(issuer) == "" || strings.TrimSpace(account) == "" {
		return Enrollment{}, errors.New("user, issuer and account are required")
	}
	var active bool
	if err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM mfa_credentials
			WHERE user_id = $1 AND type = 'totp' AND status = 'active' AND revoked_at IS NULL
		)
	`, userID).Scan(&active); err != nil {
		return Enrollment{}, err
	}
	if active {
		return Enrollment{}, ErrMFAAlreadyEnabled
	}
	secret, err := NewSecret()
	if err != nil {
		return Enrollment{}, err
	}
	encrypted, err := s.box.Seal([]byte(secret))
	if err != nil {
		return Enrollment{}, err
	}
	if _, err := s.db.ExecContext(ctx, `
		UPDATE mfa_credentials
		SET status = 'revoked', revoked_at = now()
		WHERE user_id = $1 AND type = 'totp' AND status = 'pending' AND revoked_at IS NULL
	`, userID); err != nil {
		return Enrollment{}, err
	}
	enrollmentID, err := ids.New()
	if err != nil {
		return Enrollment{}, err
	}
	expiresAt := s.now().Add(s.ttl)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO mfa_credentials (
			id, user_id, type, label, status, encrypted_secret, expires_at
		) VALUES ($1, $2, 'totp', 'authenticator', 'pending', $3, $4)
	`, enrollmentID, userID, []byte(encrypted), expiresAt)
	if err != nil {
		return Enrollment{}, err
	}

	return Enrollment{
		ID:         enrollmentID,
		Secret:     secret,
		OTPAuthURL: otpAuthURL(issuer, account, secret),
		ExpiresAt:  expiresAt,
	}, nil
}

func (s *EnrollmentService) Status(ctx context.Context, userID string) (Status, error) {
	if s == nil || s.db == nil || s.box == nil {
		return Status{}, ErrMFAServiceDisabled
	}
	userID = strings.TrimSpace(userID)
	if !ids.Valid(userID) {
		return Status{}, ErrEnrollmentInvalid
	}
	var enrolledAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT verified_at
		FROM mfa_credentials
		WHERE user_id = $1
		  AND type = 'totp'
		  AND status = 'active'
		  AND revoked_at IS NULL
		  AND (expires_at IS NULL OR expires_at > now())
		ORDER BY created_at DESC
		LIMIT 1
	`, userID).Scan(&enrolledAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Status{}, nil
	}
	if err != nil {
		return Status{}, err
	}
	status := Status{Enabled: true}
	if enrolledAt.Valid {
		value := enrolledAt.Time
		status.EnrolledAt = &value
	}
	return status, nil
}

func (s *EnrollmentService) Confirm(
	ctx context.Context,
	userID string,
	enrollmentID string,
	code string,
) error {
	if s == nil || s.db == nil || s.box == nil {
		return errors.New("mfa enrollment service is not configured")
	}
	if !ids.Valid(strings.TrimSpace(userID)) || !ids.Valid(strings.TrimSpace(enrollmentID)) {
		return ErrEnrollmentInvalid
	}
	if throttled, err := s.stepUpThrottled(ctx, userID); err != nil {
		return err
	} else if throttled {
		return ErrMFAThrottled
	}
	var encrypted []byte
	var expiresAt time.Time
	err := s.db.QueryRowContext(ctx, `
		SELECT encrypted_secret, expires_at
		FROM mfa_credentials
		WHERE id = $1
		  AND user_id = $2
		  AND type = 'totp'
		  AND status = 'pending'
		  AND revoked_at IS NULL
	`, enrollmentID, userID).Scan(&encrypted, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrEnrollmentInvalid
	}
	if err != nil {
		return err
	}
	if !expiresAt.After(s.now()) {
		return ErrEnrollmentExpired
	}
	secret, err := s.box.Open(string(encrypted))
	if err != nil {
		return ErrEnrollmentInvalid
	}
	if !Verify(string(secret), strings.TrimSpace(code), s.now(), 1) {
		if err := s.recordStepUpFailure(ctx, userID); err != nil {
			return err
		}
		return ErrMFAInvalidCode
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE mfa_credentials
		SET status = 'active', verified_at = now(), expires_at = NULL
		WHERE id = $1 AND user_id = $2 AND status = 'pending'
	`, enrollmentID, userID)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		if err != nil {
			return err
		}
		return ErrEnrollmentInvalid
	}
	if err := s.clearStepUpFailures(ctx, userID); err != nil {
		return err
	}
	return err
}

func (s *EnrollmentService) Disable(ctx context.Context, userID, code string) error {
	if s == nil || s.db == nil || s.box == nil {
		return ErrMFAServiceDisabled
	}
	userID = strings.TrimSpace(userID)
	code = strings.TrimSpace(code)
	if !ids.Valid(userID) || code == "" {
		return ErrMFAInvalidCode
	}
	if throttled, err := s.stepUpThrottled(ctx, userID); err != nil {
		return err
	} else if throttled {
		return ErrMFAThrottled
	}

	var credentialID string
	var encrypted []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT id::text, encrypted_secret
		FROM mfa_credentials
		WHERE user_id = $1
		  AND type = 'totp'
		  AND status = 'active'
		  AND revoked_at IS NULL
		  AND (expires_at IS NULL OR expires_at > now())
		ORDER BY created_at DESC
		LIMIT 1
	`, userID).Scan(&credentialID, &encrypted)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrMFANotEnabled
	}
	if err != nil {
		return err
	}
	secret, err := s.box.Open(string(encrypted))
	if err != nil {
		return ErrEnrollmentInvalid
	}
	if !Verify(string(secret), code, s.now(), 1) {
		if err := s.recordStepUpFailure(ctx, userID); err != nil {
			return err
		}
		return ErrMFAInvalidCode
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE mfa_credentials
		SET status = 'revoked', revoked_at = now(), last_used_at = now()
		WHERE id = $1 AND user_id = $2 AND status = 'active' AND revoked_at IS NULL
	`, credentialID, userID)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected != 1 {
		return ErrMFANotEnabled
	}
	return s.clearStepUpFailures(ctx, userID)
}

// Verify checks an active TOTP credential for a step-up protected operation.
// The code is never persisted; successful verification only updates usage time.
func (s *EnrollmentService) Verify(ctx context.Context, userID, code string) error {
	if s == nil || s.db == nil || s.box == nil {
		return ErrMFAServiceDisabled
	}
	userID = strings.TrimSpace(userID)
	code = strings.TrimSpace(code)
	if !ids.Valid(userID) || code == "" {
		return ErrMFAInvalidCode
	}
	throttled, err := s.stepUpThrottled(ctx, userID)
	if err != nil {
		return err
	}
	if throttled {
		return ErrMFAThrottled
	}
	var credentialID string
	var encrypted []byte
	err = s.db.QueryRowContext(ctx, `
		SELECT id::text, encrypted_secret
		FROM mfa_credentials
		WHERE user_id = $1
		  AND type = 'totp'
		  AND status = 'active'
		  AND revoked_at IS NULL
		  AND (expires_at IS NULL OR expires_at > now())
		ORDER BY created_at DESC
		LIMIT 1
	`, userID).Scan(&credentialID, &encrypted)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrMFANotEnabled
	}
	if err != nil {
		return err
	}
	secret, err := s.box.Open(string(encrypted))
	if err != nil {
		return ErrMFAInvalidCode
	}
	if !Verify(string(secret), code, s.now(), 1) {
		if recordErr := s.recordStepUpFailure(ctx, userID); recordErr != nil {
			return recordErr
		}
		return ErrMFAInvalidCode
	}
	_, err = s.db.ExecContext(ctx, `UPDATE mfa_credentials SET last_used_at = now() WHERE id = $1::uuid`, credentialID)
	if err == nil {
		err = s.clearStepUpFailures(ctx, userID)
	}
	return err
}

func stepUpSubjectHash(userID string) string {
	digest := sha256.Sum256([]byte("mfa-step-up:" + strings.TrimSpace(userID)))
	return hex.EncodeToString(digest[:])
}

func (s *EnrollmentService) stepUpThrottled(ctx context.Context, userID string) (bool, error) {
	var lockedUntil sql.NullTime
	err := s.db.QueryRowContext(ctx, `SELECT locked_until FROM login_throttles WHERE subject_hash = $1`, stepUpSubjectHash(userID)).Scan(&lockedUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return lockedUntil.Valid && lockedUntil.Time.After(s.now()), nil
}

func (s *EnrollmentService) recordStepUpFailure(ctx context.Context, userID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	key := stepUpSubjectHash(userID)
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key); err != nil {
		return err
	}
	now := s.now()
	var count int
	var firstFailedAt time.Time
	if err := tx.QueryRowContext(ctx, `SELECT failure_count, first_failed_at FROM login_throttles WHERE subject_hash = $1 FOR UPDATE`, key).Scan(&count, &firstFailedAt); errors.Is(err, sql.ErrNoRows) {
		_, err = tx.ExecContext(ctx, `INSERT INTO login_throttles (subject_hash, failure_count, first_failed_at, locked_until, updated_at) VALUES ($1, 1, $2, NULL, $2)`, key, now)
	} else if err != nil {
		return err
	} else if firstFailedAt.Before(now.Add(-stepUpWindow)) {
		_, err = tx.ExecContext(ctx, `UPDATE login_throttles SET failure_count = 1, first_failed_at = $2, locked_until = NULL, updated_at = $2 WHERE subject_hash = $1`, key, now)
	} else {
		lockedUntil := any(nil)
		if count+1 >= stepUpMaxFailures {
			lockedUntil = now.Add(stepUpLockFor)
		}
		_, err = tx.ExecContext(ctx, `UPDATE login_throttles SET failure_count = failure_count + 1, locked_until = $2, updated_at = $3 WHERE subject_hash = $1`, key, lockedUntil, now)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *EnrollmentService) clearStepUpFailures(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM login_throttles WHERE subject_hash = $1`, stepUpSubjectHash(userID))
	return err
}

func otpAuthURL(issuer, account, secret string) string {
	rawSecret, err := decodeSecret(secret)
	if err != nil {
		return ""
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      strings.TrimSpace(issuer),
		AccountName: strings.TrimSpace(account),
		Period:      defaultPeriod,
		Secret:      rawSecret,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return ""
	}
	return key.String()
}
