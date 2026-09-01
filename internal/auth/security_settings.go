package auth

import (
	"context"
	"time"
)

type SecuritySettings struct {
	AdminMFAEnabled bool      `json:"admin_mfa_enabled"`
	UpdatedAt       time.Time `json:"updated_at"`
	UpdatedBy       string    `json:"updated_by"`
}

type AdminMFAReader interface {
	AdminMFAEnabled(ctx context.Context) (bool, error)
}

type TOTPFeatureReader interface {
	TOTPEnabled(ctx context.Context) (bool, error)
}

type SecuritySettingsProvider interface {
	AdminMFAReader
	GetAdminSecuritySettings(ctx context.Context) (SecuritySettings, error)
	UpdateAdminMFAEnabled(ctx context.Context, enabled bool, actorID string) (SecuritySettings, error)
}
