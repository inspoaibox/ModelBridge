package config

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DeploymentMode      string
	RegistrationEnabled bool
	HTTPAddr            string
	HTTPReadTimeout     time.Duration
	HTTPWriteTimeout    time.Duration
	HTTPIdleTimeout     time.Duration
	WebDir              string
	DatabaseURL         string
	MigrationsDir       string
	TokenPepper         string
	SessionPepper       string
	MFAEncryptionKey    []byte
	CookieSecure        bool
	SessionTTL          time.Duration
	LoginMaxFailures    int
	LoginWindow         time.Duration
	LoginLockDuration   time.Duration
	PublicBaseURL       string
	SMTPAddress         string
	SMTPFrom            string
	SMTPUsername        string
	SMTPPassword        string
	TrustedProxyCIDRs   string
	AdminEntryPath      string
}

func Load() (Config, error) {
	deploymentMode := strings.ToLower(stringEnv("APP_ENV", "development"))
	if deploymentMode != "development" && deploymentMode != "test" && deploymentMode != "production" {
		return Config{}, errors.New("APP_ENV must be development, test, or production")
	}
	registrationEnabled, err := boolEnv("REGISTRATION_ENABLED", deploymentMode != "production")
	if err != nil {
		return Config{}, err
	}
	cookieSecure, err := boolEnv("COOKIE_SECURE", true)
	if err != nil {
		return Config{}, err
	}
	sessionTTL, err := durationEnv("SESSION_TTL", 12*time.Hour)
	if err != nil {
		return Config{}, err
	}
	loginWindow, err := durationEnv("LOGIN_FAILURE_WINDOW", 15*time.Minute)
	if err != nil {
		return Config{}, err
	}
	loginLockDuration, err := durationEnv("LOGIN_LOCK_DURATION", 15*time.Minute)
	if err != nil {
		return Config{}, err
	}
	maxFailures, err := intEnv("LOGIN_MAX_FAILURES", 5)
	if err != nil || maxFailures < 1 {
		return Config{}, errors.New("LOGIN_MAX_FAILURES must be a positive integer")
	}
	if sessionTTL <= 0 || loginWindow <= 0 || loginLockDuration <= 0 {
		return Config{}, errors.New("security durations must be positive")
	}
	httpAddr := stringEnv("HTTP_ADDR", ":8080")
	httpReadTimeout, err := durationEnv("HTTP_READ_TIMEOUT", 60*time.Second)
	if err != nil {
		return Config{}, err
	}
	httpWriteTimeout, err := durationEnv("HTTP_WRITE_TIMEOUT", 15*time.Minute)
	if err != nil {
		return Config{}, err
	}
	httpIdleTimeout, err := durationEnv("HTTP_IDLE_TIMEOUT", 2*time.Minute)
	if err != nil {
		return Config{}, err
	}
	if httpReadTimeout <= 0 || httpWriteTimeout <= 0 || httpIdleTimeout <= 0 {
		return Config{}, errors.New("HTTP timeouts must be positive")
	}

	mfaKey, err := secretKeyEnv("MFA_ENCRYPTION_KEY")
	if err != nil {
		return Config{}, err
	}
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	tokenPepper := strings.TrimSpace(os.Getenv("TOKEN_PEPPER"))
	sessionPepper := strings.TrimSpace(os.Getenv("SESSION_PEPPER"))
	publicBaseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL")), "/")
	smtpAddress := strings.TrimSpace(os.Getenv("SMTP_ADDR"))
	smtpFrom := strings.TrimSpace(os.Getenv("SMTP_FROM"))
	smtpUsername := strings.TrimSpace(os.Getenv("SMTP_USERNAME"))
	smtpPassword := strings.TrimSpace(os.Getenv("SMTP_PASSWORD"))
	trustedProxyCIDRs := strings.TrimSpace(os.Getenv("TRUSTED_PROXY_CIDRS"))
	if err := validateTrustedProxyCIDRs(trustedProxyCIDRs); err != nil {
		return Config{}, err
	}
	adminEntryPath := strings.TrimSpace(os.Getenv("ADMIN_ENTRY_PATH"))
	if err := validateAdminEntryPath(adminEntryPath); err != nil {
		return Config{}, err
	}
	allowedOrigins := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS"))
	if err := validateCORSAllowedOrigins(allowedOrigins, deploymentMode == "production"); err != nil {
		return Config{}, err
	}
	if deploymentMode == "production" {
		if err := validateProductionHTTPAddr(httpAddr); err != nil {
			return Config{}, err
		}
		if databaseURL == "" {
			return Config{}, errors.New("DATABASE_URL is required in production")
		}
		if !cookieSecure {
			return Config{}, errors.New("COOKIE_SECURE must be true in production")
		}
		if len(tokenPepper) < 32 || len(sessionPepper) < 32 {
			return Config{}, errors.New("TOKEN_PEPPER and SESSION_PEPPER must each be at least 32 characters in production")
		}
		if tokenPepper == sessionPepper {
			return Config{}, errors.New("TOKEN_PEPPER and SESSION_PEPPER must be different in production")
		}
		if len(mfaKey) != 32 {
			return Config{}, errors.New("MFA_ENCRYPTION_KEY is required in production")
		}
		if allowedOrigins == "" {
			return Config{}, errors.New("CORS_ALLOWED_ORIGINS must be explicitly configured in production")
		}
		if strings.TrimSpace(os.Getenv("REGISTRATION_ENABLED")) == "" {
			return Config{}, errors.New("REGISTRATION_ENABLED must be explicitly configured in production")
		}
		if publicBaseURL != "" {
			if err := validatePublicBaseURL(publicBaseURL, true); err != nil {
				return Config{}, err
			}
		}
		if smtpAddress != "" || smtpFrom != "" || smtpUsername != "" || smtpPassword != "" {
			if smtpAddress == "" || smtpFrom == "" {
				return Config{}, errors.New("SMTP_ADDR and SMTP_FROM must be configured together")
			}
		}
		if trustedProxyCIDRs == "" {
			return Config{}, errors.New("TRUSTED_PROXY_CIDRS must be explicitly configured in production")
		}
		if adminEntryPath == "" {
			return Config{}, errors.New("ADMIN_ENTRY_PATH must be explicitly configured in production")
		}
	}

	return Config{
		DeploymentMode:      deploymentMode,
		RegistrationEnabled: registrationEnabled,
		HTTPAddr:            httpAddr,
		HTTPReadTimeout:     httpReadTimeout,
		HTTPWriteTimeout:    httpWriteTimeout,
		HTTPIdleTimeout:     httpIdleTimeout,
		WebDir:              stringEnv("WEB_DIR", "web"),
		DatabaseURL:         databaseURL,
		MigrationsDir:       stringEnv("MIGRATIONS_DIR", "migrations"),
		TokenPepper:         tokenPepper,
		SessionPepper:       sessionPepper,
		MFAEncryptionKey:    mfaKey,
		CookieSecure:        cookieSecure,
		SessionTTL:          sessionTTL,
		LoginMaxFailures:    maxFailures,
		LoginWindow:         loginWindow,
		LoginLockDuration:   loginLockDuration,
		PublicBaseURL:       publicBaseURL,
		SMTPAddress:         smtpAddress,
		SMTPFrom:            smtpFrom,
		SMTPUsername:        smtpUsername,
		SMTPPassword:        smtpPassword,
		TrustedProxyCIDRs:   trustedProxyCIDRs,
		AdminEntryPath:      adminEntryPath,
	}, nil
}

func validateTrustedProxyCIDRs(value string) error {
	if value == "" {
		return nil
	}
	for _, item := range strings.Split(value, ",") {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(item))
		if err != nil {
			return errors.New("TRUSTED_PROXY_CIDRS must contain valid CIDR prefixes")
		}
		if prefix.Bits() == 0 {
			return errors.New("TRUSTED_PROXY_CIDRS must not contain a default route")
		}
	}
	return nil
}

func validateAdminEntryPath(value string) error {
	if value == "" {
		return nil
	}
	if !strings.HasPrefix(value, "/admin-") || len(value) < len("/admin-")+16 || len(value) > 160 {
		return errors.New("ADMIN_ENTRY_PATH must use /admin- followed by at least 16 URL-safe characters")
	}
	for _, char := range value[len("/admin-"):] {
		if (char < 'a' || char > 'z') &&
			(char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') &&
			char != '-' &&
			char != '_' {
			return errors.New("ADMIN_ENTRY_PATH must contain URL-safe characters only")
		}
	}
	return nil
}

func validateProductionHTTPAddr(value string) error {
	host, port, err := net.SplitHostPort(strings.TrimSpace(value))
	if err != nil || host == "" || port == "" {
		return errors.New("HTTP_ADDR must bind to 127.0.0.1 or ::1 in production")
	}
	parsedHost, err := netip.ParseAddr(host)
	if err != nil || !parsedHost.IsLoopback() {
		return errors.New("HTTP_ADDR must bind to 127.0.0.1 or ::1 in production")
	}
	parsedPort, err := strconv.Atoi(port)
	if err != nil || parsedPort < 1 || parsedPort > 65535 {
		return errors.New("HTTP_ADDR must use a valid port in production")
	}
	return nil
}

func validatePublicBaseURL(value string, requireHTTPS bool) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("PUBLIC_BASE_URL must contain a valid origin")
	}
	if requireHTTPS && parsed.Scheme != "https" {
		return errors.New("PUBLIC_BASE_URL must use HTTPS in production")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("PUBLIC_BASE_URL must use HTTP or HTTPS")
	}
	return nil
}

func validateCORSAllowedOrigins(value string, requireHTTPS bool) error {
	if value == "" {
		return nil
	}
	for _, item := range strings.Split(value, ",") {
		parsed, err := url.Parse(strings.TrimSpace(item))
		if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
			return errors.New("CORS_ALLOWED_ORIGINS must contain valid origins only")
		}
		if requireHTTPS && parsed.Scheme != "https" {
			return errors.New("CORS_ALLOWED_ORIGINS must use HTTPS in production")
		}
		if !requireHTTPS && parsed.Scheme != "http" && parsed.Scheme != "https" {
			return errors.New("CORS_ALLOWED_ORIGINS must use HTTP or HTTPS")
		}
	}
	return nil
}

func stringEnv(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func boolEnv(name string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, errors.New(name + " must be true or false")
	}
	return parsed, nil
}

func durationEnv(name string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, errors.New(name + " must be a valid duration")
	}
	return parsed, nil
}

func intEnv(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, errors.New(name + " must be an integer")
	}
	return parsed, nil
}

func secretKeyEnv(name string) ([]byte, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return nil, nil
	}
	if decoded, err := hex.DecodeString(value); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(value); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if decoded, err := base64.StdEncoding.DecodeString(value); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	return nil, errors.New(name + " must be a 32-byte hex or base64 key")
}
