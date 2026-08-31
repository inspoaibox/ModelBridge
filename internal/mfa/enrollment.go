package mfa

import (
	"context"
	"database/sql"
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
	return nil
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
