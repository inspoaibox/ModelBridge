package auth

import (
	"context"
	"database/sql"
	"errors"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"ai-token/internal/ids"
	"ai-token/internal/passwords"
	"ai-token/internal/tokens"
)

var (
	ErrRegistrationInvalid   = errors.New("invalid registration request")
	ErrEmailAlreadyExists    = errors.New("email is already registered")
	ErrTenantAlreadyExists   = errors.New("tenant slug is already registered")
	ErrRegistrationThrottled = errors.New("registration temporarily throttled")
)

type RegistrationRequest struct {
	Email       string
	Password    string
	DisplayName string
	TenantName  string
	TenantSlug  string
	ProjectName string
	ClientIP    string
}

type RegisteredAccount struct {
	UserID                    string `json:"user_id"`
	TenantID                  string `json:"tenant_id"`
	ProjectID                 string `json:"project_id"`
	EmailVerificationRequired bool   `json:"email_verification_required"`
}

type RegistrationProvider interface {
	Register(context.Context, RegistrationRequest) (RegisteredAccount, error)
}

type SQLRegistrationService struct {
	db       *sql.DB
	throttle *RequestThrottle
	notifier EmailVerificationNotifier
}

func NewSQLRegistrationService(db *sql.DB, hasher *tokens.Hasher, maxRequests int, window, lockFor time.Duration) (*SQLRegistrationService, error) {
	throttle, err := NewRequestThrottle(db, hasher, maxRequests, window, lockFor)
	if err != nil {
		return nil, err
	}
	return &SQLRegistrationService{db: db, throttle: throttle}, nil
}

func NewSQLRegistrationServiceWithNotifier(db *sql.DB, hasher *tokens.Hasher, maxRequests int, window, lockFor time.Duration, notifier EmailVerificationNotifier) (*SQLRegistrationService, error) {
	service, err := NewSQLRegistrationService(db, hasher, maxRequests, window, lockFor)
	if err != nil {
		return nil, err
	}
	if notifier == nil {
		return nil, errors.New("email verification notifier is required")
	}
	service.notifier = notifier
	return service, nil
}

func (s *SQLRegistrationService) Register(ctx context.Context, request RegistrationRequest) (RegisteredAccount, error) {
	if s == nil || s.db == nil || s.throttle == nil {
		return RegisteredAccount{}, errors.New("registration service is not configured")
	}
	request.Email = normalizeEmail(request.Email)
	request.DisplayName = strings.TrimSpace(request.DisplayName)
	request.TenantName = strings.TrimSpace(request.TenantName)
	request.TenantSlug = normalizeSlug(request.TenantSlug)
	request.ProjectName = strings.TrimSpace(request.ProjectName)
	request.ClientIP = strings.TrimSpace(request.ClientIP)
	if request.ClientIP == "" {
		request.ClientIP = "unknown"
	}
	allowed, err := s.throttle.Allow(ctx, "registration-ip", request.ClientIP)
	if err != nil {
		return RegisteredAccount{}, err
	}
	if !allowed {
		return RegisteredAccount{}, ErrRegistrationThrottled
	}
	if request.Email != "" {
		allowed, err = s.throttle.Allow(ctx, "registration-email", request.Email)
		if err != nil {
			return RegisteredAccount{}, err
		}
		if !allowed {
			return RegisteredAccount{}, ErrRegistrationThrottled
		}
	}
	if request.ProjectName == "" {
		request.ProjectName = "Default Project"
	}
	if !validRegistrationEmail(request.Email) || request.Password == "" ||
		request.DisplayName == "" || len(request.DisplayName) > 100 ||
		request.TenantName == "" || len(request.TenantName) > 120 ||
		!validTenantSlug(request.TenantSlug) || len(request.ProjectName) > 100 {
		return RegisteredAccount{}, ErrRegistrationInvalid
	}

	passwordHash, err := passwords.Hash(request.Password)
	if err != nil {
		return RegisteredAccount{}, ErrRegistrationInvalid
	}
	emailEnabled, err := notifierEmailEnabled(ctx, s.notifier, "email_verification")
	if err != nil {
		return RegisteredAccount{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RegisteredAccount{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var exists bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM users WHERE lower(email) = $1 AND deleted_at IS NULL)
	`, request.Email).Scan(&exists); err != nil {
		return RegisteredAccount{}, err
	}
	if exists {
		return RegisteredAccount{}, ErrEmailAlreadyExists
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM tenants WHERE lower(slug) = $1 AND deleted_at IS NULL)
	`, request.TenantSlug).Scan(&exists); err != nil {
		return RegisteredAccount{}, err
	}
	if exists {
		return RegisteredAccount{}, ErrTenantAlreadyExists
	}

	userID, err := ids.New()
	if err != nil {
		return RegisteredAccount{}, err
	}
	tenantID, err := ids.New()
	if err != nil {
		return RegisteredAccount{}, err
	}
	projectID, err := ids.New()
	if err != nil {
		return RegisteredAccount{}, err
	}
	memberID, err := ids.New()
	if err != nil {
		return RegisteredAccount{}, err
	}
	accountID, err := ids.New()
	if err != nil {
		return RegisteredAccount{}, err
	}

	userStatus := "active"
	if emailEnabled {
		userStatus = "pending"
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, display_name, status, password_changed_at)
		VALUES ($1, $2, $3, $4, $5, now())
	`, userID, request.Email, passwordHash, request.DisplayName, userStatus); err != nil {
		return RegisteredAccount{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO tenants (id, name, slug, status)
		VALUES ($1, $2, $3, 'active')
	`, tenantID, request.TenantName, request.TenantSlug); err != nil {
		return RegisteredAccount{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO projects (id, tenant_id, name, slug, status, created_by)
		VALUES ($1, $2, $3, 'default', 'active', $4)
	`, projectID, tenantID, request.ProjectName, userID); err != nil {
		return RegisteredAccount{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO tenant_members (id, tenant_id, user_id, role_code, status, created_by)
		VALUES ($1, $2, $3, 'tenant_owner', 'active', $3)
	`, memberID, tenantID, userID); err != nil {
		return RegisteredAccount{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO project_members (project_id, user_id, role_code)
		VALUES ($1, $2, 'project_admin')
	`, projectID, userID); err != nil {
		return RegisteredAccount{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ledger_accounts (id, tenant_id, account_type, currency, status)
		VALUES ($1, $2, 'prepaid_balance', 'USD', 'active')
	`, accountID, tenantID); err != nil {
		return RegisteredAccount{}, err
	}
	if err := tx.Commit(); err != nil {
		return RegisteredAccount{}, err
	}
	account := RegisteredAccount{UserID: userID, TenantID: tenantID, ProjectID: projectID, EmailVerificationRequired: emailEnabled}
	if emailEnabled {
		token, err := s.issueEmailVerificationToken(ctx, userID, request.ClientIP)
		if err != nil {
			return RegisteredAccount{}, err
		}
		if err := s.notifier.SendEmailVerification(ctx, request.Email, token); err != nil {
			return RegisteredAccount{}, ErrEmailVerificationDelivery
		}
	}
	return account, nil
}

var tenantSlugPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{1,62}[a-z0-9])?$`)

func normalizeSlug(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func validTenantSlug(value string) bool {
	return tenantSlugPattern.MatchString(value)
}

func validRegistrationEmail(value string) bool {
	if value == "" || len(value) > 254 {
		return false
	}
	parsed, err := mail.ParseAddress(value)
	return err == nil && parsed.Address == value
}
