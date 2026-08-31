package config

import (
	"encoding/hex"
	"os"
	"testing"
	"time"
)

func TestLoadDefaultsWithoutDatabase(t *testing.T) {
	clearConfigEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":8080" || cfg.DatabaseURL != "" || !cfg.CookieSecure {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadParsesSecurityConfiguration(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("HTTP_ADDR", ":9090")
	t.Setenv("COOKIE_SECURE", "false")
	t.Setenv("SESSION_TTL", "2h")
	t.Setenv("LOGIN_MAX_FAILURES", "7")
	t.Setenv("MFA_ENCRYPTION_KEY", hex.EncodeToString(make([]byte, 32)))
	t.Setenv("HTTP_READ_TIMEOUT", "45s")
	t.Setenv("HTTP_WRITE_TIMEOUT", "5m")
	t.Setenv("HTTP_IDLE_TIMEOUT", "90s")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":9090" || cfg.CookieSecure || cfg.SessionTTL.Hours() != 2 ||
		cfg.LoginMaxFailures != 7 || len(cfg.MFAEncryptionKey) != 32 ||
		cfg.HTTPReadTimeout != 45*time.Second || cfg.HTTPWriteTimeout != 5*time.Minute || cfg.HTTPIdleTimeout != 90*time.Second {
		t.Fatalf("unexpected configuration: %+v", cfg)
	}
}

func TestLoadRejectsInvalidKey(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("MFA_ENCRYPTION_KEY", "bad")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid key to be rejected")
	}
}

func TestLoadRejectsInvalidTrustedProxyCIDR(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("TRUSTED_PROXY_CIDRS", "not-a-cidr")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid trusted proxy CIDR to be rejected")
	}
}

func TestLoadRejectsBroadTrustedProxyCIDR(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("TRUSTED_PROXY_CIDRS", "0.0.0.0/0")
	if _, err := Load(); err == nil {
		t.Fatal("expected default route trusted proxy CIDR to be rejected")
	}
}

func TestLoadRejectsInvalidProductionCORSOrigin(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("HTTP_ADDR", "127.0.0.1:8080")
	t.Setenv("DATABASE_URL", "postgres://app:password@db.example.com:5432/gateway")
	t.Setenv("TOKEN_PEPPER", "token-pepper-with-at-least-thirty-two-chars")
	t.Setenv("SESSION_PEPPER", "session-pepper-with-at-least-thirty-two")
	t.Setenv("MFA_ENCRYPTION_KEY", hex.EncodeToString(make([]byte, 32)))
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://gateway.example.com")
	t.Setenv("REGISTRATION_ENABLED", "false")
	t.Setenv("PUBLIC_BASE_URL", "https://gateway.example.com")
	t.Setenv("SMTP_ADDR", "smtp.example.com:587")
	t.Setenv("SMTP_FROM", "no-reply@example.com")
	t.Setenv("TRUSTED_PROXY_CIDRS", "127.0.0.1/32")
	if _, err := Load(); err == nil {
		t.Fatal("expected insecure production CORS origin to be rejected")
	}
}

func TestLoadRejectsIncompleteProductionConfig(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("HTTP_ADDR", "127.0.0.1:8080")
	if _, err := Load(); err == nil {
		t.Fatal("expected incomplete production configuration to be rejected")
	}
}

func TestLoadRejectsPublicHTTPAddrAndResetBaseURLInProduction(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("HTTP_ADDR", ":8080")
	t.Setenv("DATABASE_URL", "postgres://app:password@db.example.com:5432/gateway")
	t.Setenv("TOKEN_PEPPER", "token-pepper-with-at-least-thirty-two-chars")
	t.Setenv("SESSION_PEPPER", "session-pepper-with-a-different-long-value")
	t.Setenv("MFA_ENCRYPTION_KEY", hex.EncodeToString(make([]byte, 32)))
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://gateway.example.com")
	t.Setenv("REGISTRATION_ENABLED", "false")
	t.Setenv("PUBLIC_BASE_URL", "http://gateway.example.com")
	t.Setenv("SMTP_ADDR", "smtp.example.com:587")
	t.Setenv("SMTP_FROM", "no-reply@example.com")
	t.Setenv("TRUSTED_PROXY_CIDRS", "127.0.0.1/32")
	if _, err := Load(); err == nil {
		t.Fatal("expected insecure production listener or reset URL to be rejected")
	}
}

func TestLoadAcceptsCompleteProductionConfig(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("HTTP_ADDR", "127.0.0.1:8080")
	t.Setenv("DATABASE_URL", "postgres://app:password@db.example.com:5432/gateway")
	t.Setenv("TOKEN_PEPPER", "token-pepper-with-at-least-thirty-two-chars")
	t.Setenv("SESSION_PEPPER", "session-pepper-with-at-least-thirty-two")
	t.Setenv("MFA_ENCRYPTION_KEY", hex.EncodeToString(make([]byte, 32)))
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://gateway.example.com")
	t.Setenv("REGISTRATION_ENABLED", "false")
	t.Setenv("PUBLIC_BASE_URL", "https://gateway.example.com")
	t.Setenv("SMTP_ADDR", "smtp.example.com:587")
	t.Setenv("SMTP_FROM", "no-reply@example.com")
	t.Setenv("TRUSTED_PROXY_CIDRS", "127.0.0.1/32,::1/128")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DeploymentMode != "production" || !cfg.CookieSecure {
		t.Fatalf("unexpected production configuration: %+v", cfg)
	}
}

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"HTTP_ADDR", "HTTP_READ_TIMEOUT", "HTTP_WRITE_TIMEOUT", "HTTP_IDLE_TIMEOUT", "DATABASE_URL", "MIGRATIONS_DIR", "TOKEN_PEPPER",
		"SESSION_PEPPER", "MFA_ENCRYPTION_KEY", "COOKIE_SECURE",
		"SESSION_TTL", "LOGIN_MAX_FAILURES", "LOGIN_FAILURE_WINDOW",
		"LOGIN_LOCK_DURATION", "APP_ENV", "CORS_ALLOWED_ORIGINS",
		"REGISTRATION_ENABLED", "PUBLIC_BASE_URL", "SMTP_ADDR", "SMTP_FROM",
		"SMTP_USERNAME", "SMTP_PASSWORD",
		"TRUSTED_PROXY_CIDRS",
	} {
		t.Setenv(name, "")
		_ = os.Unsetenv(name)
	}
}
