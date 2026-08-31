package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"ai-token/internal/mfa"
	"ai-token/internal/passwords"
	"ai-token/internal/tokens"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrLoginThrottled     = errors.New("login temporarily throttled")
	ErrMFARequired        = errors.New("mfa is required")
	ErrMFAInvalid         = errors.New("invalid mfa code")
	ErrMFAUnavailable     = errors.New("mfa is unavailable")
)

type LoginRequest struct {
	Email    string
	Password string
	TenantID string
	MFACode  string
	IP       string
}

type LoginProvider interface {
	Login(ctx context.Context, request LoginRequest, audience Audience) (IssuedSession, error)
	Logout(ctx context.Context, sessionSecret string) error
}

type MFAEnrollmentProvider interface {
	Begin(ctx context.Context, userID, issuer, account string) (mfa.Enrollment, error)
	Confirm(ctx context.Context, userID, enrollmentID, code string) error
}

type MFASettingsProvider interface {
	MFAEnrollmentProvider
	Status(ctx context.Context, userID string) (mfa.Status, error)
	Disable(ctx context.Context, userID, code string) error
}

type Services struct {
	Login                 LoginProvider
	Registration          RegistrationProvider
	PasswordReset         PasswordResetProvider
	PasswordResetNotifier PasswordResetNotifier
	MFA                   MFAEnrollmentProvider
	SecuritySettings      SecuritySettingsProvider
}

type SQLLoginService struct {
	db            *sql.DB
	sessions      *SessionIssuer
	subjectHasher *tokens.Hasher
	security      AdminMFAReader
	mfaBox        *mfa.SecretBox
	maxFailures   int
	failureWindow time.Duration
	lockDuration  time.Duration
	now           func() time.Time
}

func NewSQLLoginService(
	db *sql.DB,
	sessions *SessionIssuer,
	subjectHasher *tokens.Hasher,
	security AdminMFAReader,
	mfaBox *mfa.SecretBox,
	maxFailures int,
	failureWindow time.Duration,
	lockDuration time.Duration,
) (*SQLLoginService, error) {
	if db == nil || sessions == nil || subjectHasher == nil {
		return nil, errors.New("database, session issuer and subject hasher are required")
	}
	if maxFailures < 1 || failureWindow <= 0 || lockDuration <= 0 {
		return nil, errors.New("invalid login throttle configuration")
	}
	return &SQLLoginService{
		db:            db,
		sessions:      sessions,
		subjectHasher: subjectHasher,
		security:      security,
		mfaBox:        mfaBox,
		maxFailures:   maxFailures,
		failureWindow: failureWindow,
		lockDuration:  lockDuration,
		now:           time.Now,
	}, nil
}

func (s *SQLLoginService) Login(ctx context.Context, request LoginRequest, audience Audience) (IssuedSession, error) {
	if s == nil || s.db == nil || s.sessions == nil || s.subjectHasher == nil {
		return IssuedSession{}, errors.New("login service is not configured")
	}
	if audience != AudienceAdmin && audience != AudienceConsole {
		return IssuedSession{}, ErrInvalidCredentials
	}

	email := normalizeEmail(request.Email)
	if email == "" || request.Password == "" {
		return IssuedSession{}, ErrInvalidCredentials
	}
	subjectHash := s.subjectHasher.Digest(email)
	clientIP := strings.TrimSpace(request.IP)
	if clientIP == "" {
		clientIP = "unknown"
	}
	ipHash := s.subjectHasher.Digest("login-ip:" + clientIP)
	throttled, err := s.isThrottled(ctx, subjectHash)
	if err != nil {
		return IssuedSession{}, err
	}
	if throttled {
		return IssuedSession{}, ErrLoginThrottled
	}
	ipThrottled, err := s.isThrottled(ctx, ipHash)
	if err != nil {
		return IssuedSession{}, err
	}
	if ipThrottled {
		return IssuedSession{}, ErrLoginThrottled
	}

	var userID, passwordHash, status string
	err = s.db.QueryRowContext(ctx, `
		SELECT id::text, password_hash, status
		FROM users
		WHERE lower(email) = $1 AND deleted_at IS NULL
	`, email).Scan(&userID, &passwordHash, &status)
	if err != nil || status != "active" || !passwords.Verify(request.Password, passwordHash) {
		if throttleErr := s.recordLoginFailure(ctx, subjectHash, ipHash); throttleErr != nil {
			return IssuedSession{}, throttleErr
		}
		return IssuedSession{}, ErrInvalidCredentials
	}

	if audience == AudienceAdmin {
		if request.TenantID != "" || !s.hasPlatformRole(ctx, userID) {
			return IssuedSession{}, ErrInvalidCredentials
		}
	} else if !s.hasTenantMembership(ctx, userID, request.TenantID) {
		return IssuedSession{}, ErrInvalidCredentials
	}

	mfaSubjectHash := s.subjectHasher.Digest("mfa:" + email)
	mfaThrottled, err := s.isThrottled(ctx, mfaSubjectHash)
	if err != nil {
		return IssuedSession{}, err
	}
	if mfaThrottled {
		return IssuedSession{}, ErrLoginThrottled
	}
	recordMFAFailure := func(loginErr error) (IssuedSession, error) {
		if errors.Is(loginErr, ErrMFAInvalid) || errors.Is(loginErr, ErrMFARequired) {
			if throttleErr := s.recordLoginFailure(ctx, mfaSubjectHash, ipHash); throttleErr != nil {
				return IssuedSession{}, throttleErr
			}
		}
		return IssuedSession{}, loginErr
	}

	authStrength := "password"
	if audience == AudienceAdmin {
		adminMFAEnabled, err := s.adminMFAEnabled(ctx)
		if err != nil {
			return IssuedSession{}, err
		}
		if adminMFAEnabled {
			hasMFA, err := s.verifyConfiguredMFA(ctx, userID, request.MFACode)
			if err != nil {
				return recordMFAFailure(err)
			}
			if !hasMFA {
				return IssuedSession{}, ErrMFARequired
			}
			authStrength = "password_mfa"
		}
	} else {
		configured, err := s.hasConfiguredMFA(ctx, userID)
		if err != nil {
			return IssuedSession{}, err
		}
		if configured {
			if _, err := s.verifyConfiguredMFA(ctx, userID, request.MFACode); err != nil {
				return recordMFAFailure(err)
			}
			authStrength = "password_mfa"
		}
	}

	session, err := s.sessions.Issue(ctx, userID, request.TenantID, audience, authStrength)
	if err != nil {
		return IssuedSession{}, err
	}
	if err := s.clearThrottle(ctx, subjectHash); err != nil {
		return IssuedSession{}, err
	}
	if err := s.clearThrottle(ctx, mfaSubjectHash); err != nil {
		return IssuedSession{}, err
	}
	if err := s.clearThrottle(ctx, ipHash); err != nil {
		return IssuedSession{}, err
	}
	return session, nil
}

func (s *SQLLoginService) recordLoginFailure(ctx context.Context, subjectHash, ipHash string) error {
	if err := s.recordFailure(ctx, subjectHash); err != nil {
		return err
	}
	if ipHash != "" && ipHash != subjectHash {
		return s.recordFailure(ctx, ipHash)
	}
	return nil
}

func (s *SQLLoginService) Logout(ctx context.Context, sessionSecret string) error {
	if s == nil || s.sessions == nil {
		return errors.New("login service is not configured")
	}
	return s.sessions.Revoke(ctx, sessionSecret)
}

func (s *SQLLoginService) adminMFAEnabled(ctx context.Context) (bool, error) {
	if s == nil || s.security == nil {
		return false, nil
	}
	return s.security.AdminMFAEnabled(ctx)
}

func (s *SQLLoginService) verifyConfiguredMFA(
	ctx context.Context,
	userID string,
	code string,
) (bool, error) {
	var encryptedSecret []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT encrypted_secret
		FROM mfa_credentials
		WHERE user_id = $1
		  AND type = 'totp'
		  AND status = 'active'
		  AND revoked_at IS NULL
		  AND (expires_at IS NULL OR expires_at > now())
		ORDER BY created_at DESC
		LIMIT 1
	`, userID).Scan(&encryptedSecret)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrMFAUnavailable
	}
	if err != nil {
		return false, err
	}
	if s.mfaBox == nil {
		return false, ErrMFAUnavailable
	}
	secret, err := s.mfaBox.Open(string(encryptedSecret))
	if err != nil {
		return false, ErrMFAUnavailable
	}
	if strings.TrimSpace(code) == "" {
		return false, ErrMFARequired
	}
	if !mfa.Verify(string(secret), strings.TrimSpace(code), s.now(), 1) {
		return false, ErrMFAInvalid
	}
	return true, nil
}

func (s *SQLLoginService) hasConfiguredMFA(ctx context.Context, userID string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM mfa_credentials
			WHERE user_id = $1
			  AND type = 'totp'
			  AND status = 'active'
			  AND revoked_at IS NULL
			  AND (expires_at IS NULL OR expires_at > now())
		)
	`, userID).Scan(&exists)
	return exists, err
}

func (s *SQLLoginService) hasPlatformRole(ctx context.Context, userID string) bool {
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM platform_user_roles ur
			JOIN platform_roles r ON r.id = ur.role_id
			WHERE ur.user_id = $1 AND r.status = 'active'
		)
	`, userID).Scan(&exists)
	return err == nil && exists
}

func (s *SQLLoginService) hasTenantMembership(ctx context.Context, userID, tenantID string) bool {
	if tenantID == "" {
		return false
	}
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM tenant_members tm
			JOIN tenants t ON t.id = tm.tenant_id
			WHERE tm.user_id = $1
			  AND tm.tenant_id = $2
			  AND tm.status = 'active'
			  AND t.status = 'active'
			  AND t.deleted_at IS NULL
		)
	`, userID, tenantID).Scan(&exists)
	return err == nil && exists
}

func (s *SQLLoginService) isThrottled(ctx context.Context, subjectHash string) (bool, error) {
	var lockedUntil sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT locked_until
		FROM login_throttles
		WHERE subject_hash = $1
	`, subjectHash).Scan(&lockedUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return lockedUntil.Valid && lockedUntil.Time.After(s.now()), nil
}

func (s *SQLLoginService) recordFailure(ctx context.Context, subjectHash string) error {
	now := s.now()
	windowStart := now.Add(-s.failureWindow)
	lockUntil := now.Add(s.lockDuration)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO login_throttles (
			subject_hash, failure_count, first_failed_at, locked_until, updated_at
		) VALUES ($1, 1, $2, NULL, $2)
		ON CONFLICT (subject_hash) DO UPDATE SET
			failure_count = CASE
				WHEN login_throttles.first_failed_at < $3 THEN 1
				ELSE login_throttles.failure_count + 1
			END,
			first_failed_at = CASE
				WHEN login_throttles.first_failed_at < $3 THEN $2
				ELSE login_throttles.first_failed_at
			END,
			locked_until = CASE
				WHEN login_throttles.first_failed_at < $3 THEN NULL
				WHEN login_throttles.failure_count + 1 >= $4 THEN $5
				ELSE login_throttles.locked_until
			END,
			updated_at = $2
	`, subjectHash, now, windowStart, s.maxFailures, lockUntil)
	return err
}

func (s *SQLLoginService) clearThrottle(ctx context.Context, subjectHash string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM login_throttles WHERE subject_hash = $1
	`, subjectHash)
	return err
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
