package adminsettings

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"strings"
	"time"

	"ai-token/internal/auth"
)

var (
	ErrAdminMFAEnrollmentRequired = errors.New("admin mfa enrollment required")
	ErrInvalidSystemSettings      = errors.New("invalid system settings")
)

const DefaultSiteName = "AI Token Gateway"

type SystemSettings struct {
	AdminMFAEnabled bool      `json:"admin_mfa_enabled"`
	SiteName        string    `json:"site_name"`
	SiteLogoURL     string    `json:"site_logo_url"`
	SiteFaviconURL  string    `json:"site_favicon_url"`
	UpdatedAt       time.Time `json:"updated_at,omitempty"`
	UpdatedBy       string    `json:"updated_by,omitempty"`
}

type SystemSettingsUpdate struct {
	SiteName       string
	SiteLogoURL    string
	SiteFaviconURL string
}

type SystemSettingsProvider interface {
	GetSystemSettings(context.Context) (SystemSettings, error)
	UpdateSystemSettings(context.Context, string, SystemSettingsUpdate) (SystemSettings, error)
}

type Service struct {
	db *sql.DB
}

func New(db *sql.DB) (*Service, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &Service{
		db: db,
	}, nil
}

func (s *Service) AdminMFAEnabled(ctx context.Context) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("security settings service is not configured")
	}
	var enabled bool
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(value = 'true', false)
		FROM platform_settings
		WHERE key = 'admin_mfa_enabled'
	`).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return enabled, err
}

func (s *Service) GetAdminSecuritySettings(ctx context.Context) (auth.SecuritySettings, error) {
	if s == nil || s.db == nil {
		return auth.SecuritySettings{}, errors.New("security settings service is not configured")
	}
	var (
		enabled   bool
		updatedAt sql.NullTime
		updatedBy sql.NullString
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(value = 'true', false), updated_at, updated_by::text
		FROM platform_settings
		WHERE key = 'admin_mfa_enabled'
	`).Scan(&enabled, &updatedAt, &updatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.SecuritySettings{AdminMFAEnabled: false}, nil
	}
	if err != nil {
		return auth.SecuritySettings{}, err
	}
	result := auth.SecuritySettings{AdminMFAEnabled: enabled}
	if updatedAt.Valid {
		result.UpdatedAt = updatedAt.Time
	}
	if updatedBy.Valid {
		result.UpdatedBy = updatedBy.String
	}
	return result, nil
}

func (s *Service) UpdateAdminMFAEnabled(ctx context.Context, enabled bool, actorID string) (auth.SecuritySettings, error) {
	if s == nil || s.db == nil {
		return auth.SecuritySettings{}, errors.New("security settings service is not configured")
	}
	if actorID == "" {
		return auth.SecuritySettings{}, errors.New("actor is required")
	}
	if enabled {
		ok, err := s.allPlatformAdminsHaveMFA(ctx)
		if err != nil {
			return auth.SecuritySettings{}, err
		}
		if !ok {
			return auth.SecuritySettings{}, ErrAdminMFAEnrollmentRequired
		}
	}
	var updatedAt time.Time
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO platform_settings (
			key, value, updated_by, updated_at
		) VALUES ('admin_mfa_enabled', CASE WHEN $1 THEN 'true' ELSE 'false' END, $2, now())
		ON CONFLICT (key) DO UPDATE SET
			value = EXCLUDED.value,
			updated_by = EXCLUDED.updated_by,
			updated_at = EXCLUDED.updated_at
		RETURNING value = 'true', updated_at
	`, enabled, actorID).Scan(&enabled, &updatedAt)
	if err != nil {
		return auth.SecuritySettings{}, err
	}
	return auth.SecuritySettings{
		AdminMFAEnabled: enabled,
		UpdatedAt:       updatedAt,
		UpdatedBy:       actorID,
	}, nil
}

func (s *Service) GetSystemSettings(ctx context.Context) (SystemSettings, error) {
	if s == nil || s.db == nil {
		return SystemSettings{}, errors.New("system settings service is not configured")
	}
	security, err := s.GetAdminSecuritySettings(ctx)
	if err != nil {
		return SystemSettings{}, err
	}
	settings := SystemSettings{
		AdminMFAEnabled: security.AdminMFAEnabled,
		UpdatedAt:       security.UpdatedAt,
		UpdatedBy:       security.UpdatedBy,
		SiteName:        DefaultSiteName,
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT key, value, updated_at, updated_by::text
		FROM platform_settings
		WHERE key IN ('site_name', 'site_logo_url', 'site_favicon_url')
	`)
	if err != nil {
		return SystemSettings{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		var updatedAt sql.NullTime
		var updatedBy sql.NullString
		if err := rows.Scan(&key, &value, &updatedAt, &updatedBy); err != nil {
			return SystemSettings{}, err
		}
		switch key {
		case "site_name":
			if strings.TrimSpace(value) != "" {
				settings.SiteName = strings.TrimSpace(value)
			}
		case "site_logo_url":
			settings.SiteLogoURL = strings.TrimSpace(value)
		case "site_favicon_url":
			settings.SiteFaviconURL = strings.TrimSpace(value)
		}
		if updatedAt.Valid && updatedAt.Time.After(settings.UpdatedAt) {
			settings.UpdatedAt = updatedAt.Time
			settings.UpdatedBy = updatedBy.String
		}
	}
	if err := rows.Err(); err != nil {
		return SystemSettings{}, err
	}
	return settings, nil
}

func (s *Service) UpdateSystemSettings(ctx context.Context, actorID string, request SystemSettingsUpdate) (SystemSettings, error) {
	if s == nil || s.db == nil {
		return SystemSettings{}, errors.New("system settings service is not configured")
	}
	actorID = strings.TrimSpace(actorID)
	request.SiteName = strings.TrimSpace(request.SiteName)
	request.SiteLogoURL = strings.TrimSpace(request.SiteLogoURL)
	request.SiteFaviconURL = strings.TrimSpace(request.SiteFaviconURL)
	if actorID == "" || request.SiteName == "" || len(request.SiteName) > 100 ||
		!validAssetURL(request.SiteLogoURL) || !validAssetURL(request.SiteFaviconURL) {
		return SystemSettings{}, ErrInvalidSystemSettings
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SystemSettings{}, err
	}
	defer func() { _ = tx.Rollback() }()
	for _, item := range []struct {
		key   string
		value string
	}{
		{key: "site_name", value: request.SiteName},
		{key: "site_logo_url", value: request.SiteLogoURL},
		{key: "site_favicon_url", value: request.SiteFaviconURL},
	} {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO platform_settings (key, value, updated_by, updated_at)
			VALUES ($1, $2, $3, now())
			ON CONFLICT (key) DO UPDATE SET
				value = EXCLUDED.value,
				updated_by = EXCLUDED.updated_by,
				updated_at = EXCLUDED.updated_at
		`, item.key, item.value, actorID); err != nil {
			return SystemSettings{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return SystemSettings{}, err
	}
	return s.GetSystemSettings(ctx)
}

func validAssetURL(value string) bool {
	if value == "" {
		return true
	}
	if strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") {
		return true
	}
	parsed, err := url.Parse(value)
	return err == nil && parsed.Host != "" && parsed.Scheme == "https"
}

func (s *Service) hasActiveAdminMFA(ctx context.Context, userID string) (bool, error) {
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

func (s *Service) allPlatformAdminsHaveMFA(ctx context.Context) (bool, error) {
	var complete bool
	err := s.db.QueryRowContext(ctx, `
		SELECT NOT EXISTS (
			SELECT 1
			FROM platform_user_roles ur
			JOIN platform_roles r ON r.id = ur.role_id AND r.status = 'active'
			JOIN users u ON u.id = ur.user_id AND u.status = 'active' AND u.deleted_at IS NULL
			WHERE NOT EXISTS (
				SELECT 1
				FROM mfa_credentials m
				WHERE m.user_id = u.id
				  AND m.type = 'totp'
				  AND m.status = 'active'
				  AND m.revoked_at IS NULL
				  AND (m.expires_at IS NULL OR m.expires_at > now())
			)
		)
	`).Scan(&complete)
	return complete, err
}
