package adminsettings

import (
	"context"
	"database/sql"
	"errors"
	"math/big"
	"net"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ai-token/internal/auth"
	"ai-token/internal/ids"
)

var (
	ErrInvalidEmailSettings    = errors.New("invalid email settings")
	ErrInvalidFeatureSettings  = errors.New("invalid feature settings")
	ErrInvalidBalanceThreshold = errors.New("invalid balance threshold")
	ErrInvalidRechargeURL      = errors.New("invalid recharge url")
	ErrEmailSMTPRequired       = errors.New("email smtp required")
	ErrInvalidEmailSMTP        = errors.New("invalid email smtp")
	ErrEmailTemplateNotFound   = errors.New("email template not found")
)

type EmailSettings struct {
	EmailEnabled           bool      `json:"email_enabled"`
	SMTPHost               string    `json:"smtp_host"`
	SMTPPort               int       `json:"smtp_port"`
	SMTPUsername           string    `json:"smtp_username"`
	SMTPPasswordConfigured bool      `json:"smtp_password_configured"`
	SMTPFromEmail          string    `json:"smtp_from_email"`
	SMTPFromName           string    `json:"smtp_from_name"`
	SMTPTLS                bool      `json:"smtp_tls"`
	SMTPConfigured         bool      `json:"smtp_configured"`
	PublicBaseURL          string    `json:"public_base_url"`
	BalanceThreshold       string    `json:"balance_threshold"`
	RechargeURL            string    `json:"recharge_url"`
	UpdatedAt              time.Time `json:"updated_at,omitempty"`
	UpdatedBy              string    `json:"updated_by,omitempty"`
}

type EmailSettingsUpdate struct {
	SMTPHost          string
	SMTPPort          int
	SMTPUsername      string
	SMTPPassword      string
	SMTPPasswordClear bool
	SMTPFromEmail     string
	SMTPFromName      string
	SMTPTLS           bool
	PublicBaseURL     string
}

type SiteSettingsUpdate struct {
	SiteName       string
	SiteLogoURL    string
	SiteFaviconURL string
}

type FeatureSettings struct {
	EmailEnabled                bool      `json:"email_enabled"`
	RegistrationEnabled         bool      `json:"registration_enabled"`
	ModelStatusEnabled          bool      `json:"model_status_enabled"`
	TOTPEnabled                 bool      `json:"totp_enabled"`
	EmailVerificationEnabled    bool      `json:"email_verification_enabled"`
	EmailPasswordResetEnabled   bool      `json:"email_password_reset_enabled"`
	EmailSubscriptionEnabled    bool      `json:"email_subscription_enabled"`
	EmailLowBalanceAlertEnabled bool      `json:"email_low_balance_alert_enabled"`
	EmailRechargeSuccessEnabled bool      `json:"email_recharge_success_enabled"`
	EmailUsageLimitAlertEnabled bool      `json:"email_usage_limit_alert_enabled"`
	EmailContentAuditEnabled    bool      `json:"email_content_audit_enabled"`
	EmailAccountDisabledEnabled bool      `json:"email_account_disabled_enabled"`
	EmailCyberPolicyEnabled     bool      `json:"email_cyber_policy_enabled"`
	EmailOperationsEnabled      bool      `json:"email_operations_enabled"`
	BalanceThreshold            string    `json:"balance_threshold"`
	RechargeURL                 string    `json:"recharge_url"`
	UpdatedAt                   time.Time `json:"updated_at,omitempty"`
	UpdatedBy                   string    `json:"updated_by,omitempty"`
}

type FeatureSettingsUpdate struct {
	EmailEnabled                bool   `json:"email_enabled"`
	RegistrationEnabled         bool   `json:"registration_enabled"`
	ModelStatusEnabled          bool   `json:"model_status_enabled"`
	TOTPEnabled                 *bool  `json:"totp_enabled"`
	EmailVerificationEnabled    bool   `json:"email_verification_enabled"`
	EmailPasswordResetEnabled   bool   `json:"email_password_reset_enabled"`
	EmailSubscriptionEnabled    bool   `json:"email_subscription_enabled"`
	EmailLowBalanceAlertEnabled bool   `json:"email_low_balance_alert_enabled"`
	EmailRechargeSuccessEnabled bool   `json:"email_recharge_success_enabled"`
	EmailUsageLimitAlertEnabled bool   `json:"email_usage_limit_alert_enabled"`
	EmailContentAuditEnabled    bool   `json:"email_content_audit_enabled"`
	EmailAccountDisabledEnabled bool   `json:"email_account_disabled_enabled"`
	EmailCyberPolicyEnabled     bool   `json:"email_cyber_policy_enabled"`
	EmailOperationsEnabled      bool   `json:"email_operations_enabled"`
	BalanceThreshold            string `json:"balance_threshold"`
	RechargeURL                 string `json:"recharge_url"`
}

type EmailTemplate struct {
	ID        string    `json:"id"`
	EventCode string    `json:"event_code"`
	Language  string    `json:"language"`
	Subject   string    `json:"subject"`
	HTMLBody  string    `json:"html_body"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type EmailTemplateMutation struct {
	EventCode string `json:"event_code"`
	Language  string `json:"language"`
	Subject   string `json:"subject"`
	HTMLBody  string `json:"html_body"`
	Enabled   *bool  `json:"enabled,omitempty"`
}

type EmailSettingsProvider interface {
	GetEmailSettings(context.Context) (EmailSettings, error)
	UpdateEmailSettings(context.Context, string, EmailSettingsUpdate) (EmailSettings, error)
	TestSMTPConnection(context.Context) error
	SendTestEmail(context.Context, string) error
}

type SiteSettingsProvider interface {
	UpdateSiteSettings(context.Context, string, SiteSettingsUpdate) (SystemSettings, error)
}

type FeatureSettingsProvider interface {
	GetFeatureSettings(context.Context) (FeatureSettings, error)
	UpdateFeatureSettings(context.Context, string, FeatureSettingsUpdate) (FeatureSettings, error)
}

type EmailTemplateService interface {
	ListEmailTemplates(context.Context) ([]EmailTemplate, error)
	CreateEmailTemplate(context.Context, string, EmailTemplateMutation) (EmailTemplate, error)
	UpdateEmailTemplate(context.Context, string, string, EmailTemplateMutation) (EmailTemplate, error)
	DeleteEmailTemplate(context.Context, string, string) error
}

func (s *Service) EmailEnabled(ctx context.Context) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("email settings service is not configured")
	}
	values, err := s.loadSettings(ctx)
	if err != nil {
		return false, err
	}
	return parseSettingBool(values["email_enabled"], false), nil
}

func (s *Service) EmailFeatureEnabled(ctx context.Context, event string) (bool, error) {
	enabled, err := s.EmailEnabled(ctx)
	if err != nil || !enabled {
		return enabled, err
	}
	key := map[string]string{
		"email_verification":      "email_verification_enabled",
		"password_reset":          "email_password_reset_enabled",
		"subscription_started":    "email_subscription_enabled",
		"subscription_expiring":   "email_subscription_enabled",
		"low_balance":             "email_low_balance_alert_enabled",
		"recharge_success":        "email_recharge_success_enabled",
		"usage_limit_alert":       "email_usage_limit_alert_enabled",
		"content_audit_violation": "email_content_audit_enabled",
		"account_disabled":        "email_account_disabled_enabled",
		"cyber_policy_notice":     "email_cyber_policy_enabled",
		"ops_alert":               "email_operations_enabled",
		"ops_daily_report":        "email_operations_enabled",
	}[strings.TrimSpace(event)]
	if key == "" {
		return true, nil
	}
	values, err := s.loadSettings(ctx)
	if err != nil {
		return false, err
	}
	return parseSettingBool(values[key], true), nil
}

func (s *Service) GetEmailSettings(ctx context.Context) (EmailSettings, error) {
	values, err := s.loadSettings(ctx)
	if err != nil {
		return EmailSettings{}, err
	}
	smtp, err := s.readSMTPSettings(ctx, values)
	if err != nil {
		return EmailSettings{}, err
	}
	settings := EmailSettings{
		EmailEnabled:           parseSettingBool(values["email_enabled"], false),
		SMTPHost:               smtp.Host,
		SMTPPort:               smtp.Port,
		SMTPUsername:           smtp.Username,
		SMTPPasswordConfigured: strings.TrimSpace(smtp.Password) != "",
		SMTPFromEmail:          smtp.From,
		SMTPFromName:           smtp.FromName,
		SMTPTLS:                smtp.TLS,
		SMTPConfigured:         smtp.Configured,
		PublicBaseURL:          smtp.BaseURL,
		BalanceThreshold:       normalizeDecimalOrZero(values["email_balance_threshold"]),
		RechargeURL:            strings.TrimSpace(values["email_recharge_url"]),
	}
	settings.UpdatedAt, settings.UpdatedBy = latestSettingUpdate(values)
	return settings, nil
}

func (s *Service) GetFeatureSettings(ctx context.Context) (FeatureSettings, error) {
	values, err := s.loadSettings(ctx)
	if err != nil {
		return FeatureSettings{}, err
	}
	result := FeatureSettings{
		EmailEnabled:                parseSettingBool(values["email_enabled"], false),
		RegistrationEnabled:         parseSettingBool(values["registration_enabled"], false),
		ModelStatusEnabled:          parseSettingBool(values["model_status_enabled"], true),
		TOTPEnabled:                 parseSettingBool(values["totp_enabled"], false),
		EmailVerificationEnabled:    parseSettingBool(values["email_verification_enabled"], true),
		EmailPasswordResetEnabled:   parseSettingBool(values["email_password_reset_enabled"], true),
		EmailSubscriptionEnabled:    parseSettingBool(values["email_subscription_enabled"], true),
		EmailLowBalanceAlertEnabled: parseSettingBool(values["email_low_balance_alert_enabled"], false),
		EmailRechargeSuccessEnabled: parseSettingBool(values["email_recharge_success_enabled"], false),
		EmailUsageLimitAlertEnabled: parseSettingBool(values["email_usage_limit_alert_enabled"], false),
		EmailContentAuditEnabled:    parseSettingBool(values["email_content_audit_enabled"], false),
		EmailAccountDisabledEnabled: parseSettingBool(values["email_account_disabled_enabled"], false),
		EmailCyberPolicyEnabled:     parseSettingBool(values["email_cyber_policy_enabled"], false),
		EmailOperationsEnabled:      parseSettingBool(values["email_operations_enabled"], false),
		BalanceThreshold:            normalizeDecimalOrZero(values["email_balance_threshold"]),
		RechargeURL:                 strings.TrimSpace(values["email_recharge_url"]),
	}
	result.UpdatedAt, result.UpdatedBy = latestSettingUpdate(values)
	return result, nil
}

func (s *Service) UpdateEmailSettings(ctx context.Context, actorID string, request EmailSettingsUpdate) (EmailSettings, error) {
	if s == nil || s.db == nil || strings.TrimSpace(actorID) == "" {
		return EmailSettings{}, ErrInvalidEmailSettings
	}
	request.SMTPHost = strings.TrimSpace(request.SMTPHost)
	request.SMTPUsername = strings.TrimSpace(request.SMTPUsername)
	request.SMTPPassword = strings.TrimSpace(request.SMTPPassword)
	request.SMTPFromEmail = strings.TrimSpace(request.SMTPFromEmail)
	request.SMTPFromName = strings.TrimSpace(request.SMTPFromName)
	request.PublicBaseURL = strings.TrimRight(strings.TrimSpace(request.PublicBaseURL), "/")
	if !validHTTPSURL(request.PublicBaseURL) {
		return EmailSettings{}, ErrInvalidEmailSettings
	}
	if request.SMTPPort == 0 {
		request.SMTPPort = 587
	}
	values, err := s.loadSettings(ctx)
	if err != nil {
		return EmailSettings{}, err
	}
	current, err := s.readSMTPSettings(ctx, values)
	if err != nil {
		return EmailSettings{}, err
	}
	password := current.Password
	if request.SMTPPasswordClear {
		password = ""
	} else if request.SMTPPassword != "" {
		password = request.SMTPPassword
	}
	if request.SMTPHost != "" {
		if !validSMTPHost(request.SMTPHost) || request.SMTPPort < 1 || request.SMTPPort > 65535 || !validSMTPFromEmail(request.SMTPFromEmail) || len(request.SMTPFromName) > 200 || strings.ContainsAny(request.SMTPFromName, "\r\n") || !validHTTPSURL(request.PublicBaseURL) || (request.SMTPUsername == "") != (password == "") {
			return EmailSettings{}, ErrInvalidEmailSettings
		}
	} else if request.SMTPUsername != "" || password != "" || request.SMTPFromEmail != "" || request.SMTPFromName != "" {
		return EmailSettings{}, ErrInvalidEmailSettings
	}
	if request.SMTPHost != "" {
		if _, err := auth.NewSMTPNotifier(auth.SMTPSettings{Host: request.SMTPHost, Port: request.SMTPPort, From: request.SMTPFromEmail, FromName: request.SMTPFromName, Username: request.SMTPUsername, Password: password, BaseURL: request.PublicBaseURL, TLS: request.SMTPTLS}); err != nil {
			return EmailSettings{}, ErrInvalidEmailSettings
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return EmailSettings{}, err
	}
	defer func() { _ = tx.Rollback() }()
	items := []struct{ key, value string }{
		{"smtp_host", request.SMTPHost},
		{"smtp_port", strconv.Itoa(request.SMTPPort)},
		{"smtp_username", request.SMTPUsername},
		{"smtp_from_email", request.SMTPFromEmail},
		{"smtp_from_name", request.SMTPFromName},
		{"smtp_tls", strconv.FormatBool(request.SMTPTLS)},
		{"public_base_url", request.PublicBaseURL},
	}
	if request.SMTPHost != "" {
		items = append(items, struct{ key, value string }{"smtp_addr", net.JoinHostPort(request.SMTPHost, strconv.Itoa(request.SMTPPort))}, struct{ key, value string }{"smtp_from", request.SMTPFromEmail})
	} else {
		items = append(items, struct{ key, value string }{"smtp_addr", ""}, struct{ key, value string }{"smtp_from", ""})
	}
	for _, item := range items {
		if _, err := tx.ExecContext(ctx, `INSERT INTO platform_settings (key, value, updated_by, updated_at) VALUES ($1, $2, $3, now()) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = EXCLUDED.updated_at`, item.key, item.value, actorID); err != nil {
			return EmailSettings{}, err
		}
	}
	if request.SMTPPasswordClear {
		if _, err := tx.ExecContext(ctx, `DELETE FROM platform_settings WHERE key = 'smtp_password'`); err != nil {
			return EmailSettings{}, err
		}
	} else if request.SMTPPassword != "" {
		if s.box == nil {
			return EmailSettings{}, ErrInvalidEmailSettings
		}
		encrypted, err := s.box.Seal([]byte(request.SMTPPassword))
		if err != nil {
			return EmailSettings{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO platform_settings (key, value, updated_by, updated_at) VALUES ('smtp_password', $1, $2, now()) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = EXCLUDED.updated_at`, encrypted, actorID); err != nil {
			return EmailSettings{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return EmailSettings{}, err
	}
	return s.GetEmailSettings(ctx)
}

func (s *Service) UpdateSiteSettings(ctx context.Context, actorID string, request SiteSettingsUpdate) (SystemSettings, error) {
	if s == nil || s.db == nil || strings.TrimSpace(actorID) == "" {
		return SystemSettings{}, ErrInvalidSystemSettings
	}
	request.SiteName = strings.TrimSpace(request.SiteName)
	request.SiteLogoURL = strings.TrimSpace(request.SiteLogoURL)
	request.SiteFaviconURL = strings.TrimSpace(request.SiteFaviconURL)
	if request.SiteName == "" || len(request.SiteName) > 100 || !validAssetURL(request.SiteLogoURL) || !validAssetURL(request.SiteFaviconURL) {
		return SystemSettings{}, ErrInvalidSystemSettings
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SystemSettings{}, err
	}
	defer func() { _ = tx.Rollback() }()
	for _, item := range []struct{ key, value string }{
		{"site_name", request.SiteName},
		{"site_logo_url", request.SiteLogoURL},
		{"site_favicon_url", request.SiteFaviconURL},
	} {
		if _, err := tx.ExecContext(ctx, `INSERT INTO platform_settings (key, value, updated_by, updated_at) VALUES ($1, $2, $3, now()) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = EXCLUDED.updated_at`, item.key, item.value, actorID); err != nil {
			return SystemSettings{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return SystemSettings{}, err
	}
	return s.GetSystemSettings(ctx)
}

func (s *Service) UpdateFeatureSettings(ctx context.Context, actorID string, request FeatureSettingsUpdate) (FeatureSettings, error) {
	if s == nil || s.db == nil || strings.TrimSpace(actorID) == "" {
		return FeatureSettings{}, ErrInvalidFeatureSettings
	}
	request.BalanceThreshold = strings.TrimSpace(request.BalanceThreshold)
	request.RechargeURL = strings.TrimRight(strings.TrimSpace(request.RechargeURL), "/")
	if request.BalanceThreshold != "" && !validNonNegativeDecimal(request.BalanceThreshold) {
		return FeatureSettings{}, ErrInvalidBalanceThreshold
	}
	if !validHTTPSURL(request.RechargeURL) {
		return FeatureSettings{}, ErrInvalidRechargeURL
	}
	if request.BalanceThreshold == "" {
		request.BalanceThreshold = "0"
	}
	values, err := s.loadSettings(ctx)
	if err != nil {
		return FeatureSettings{}, err
	}
	totpEnabled := parseSettingBool(values["totp_enabled"], false)
	if request.TOTPEnabled != nil {
		totpEnabled = *request.TOTPEnabled
	}
	if request.EmailEnabled {
		smtp, err := s.readSMTPSettings(ctx, nil)
		if err != nil {
			return FeatureSettings{}, err
		}
		if !smtp.Configured {
			return FeatureSettings{}, ErrEmailSMTPRequired
		}
		if _, err := auth.NewSMTPNotifier(smtp); err != nil {
			return FeatureSettings{}, ErrInvalidEmailSMTP
		}
	}
	items := []struct{ key, value string }{
		{"email_enabled", strconv.FormatBool(request.EmailEnabled)},
		{"registration_enabled", strconv.FormatBool(request.RegistrationEnabled)},
		{"model_status_enabled", strconv.FormatBool(request.ModelStatusEnabled)},
		{"totp_enabled", strconv.FormatBool(totpEnabled)},
		{"email_verification_enabled", strconv.FormatBool(request.EmailVerificationEnabled)},
		{"email_password_reset_enabled", strconv.FormatBool(request.EmailPasswordResetEnabled)},
		{"email_subscription_enabled", strconv.FormatBool(request.EmailSubscriptionEnabled)},
		{"email_low_balance_alert_enabled", strconv.FormatBool(request.EmailLowBalanceAlertEnabled)},
		{"email_recharge_success_enabled", strconv.FormatBool(request.EmailRechargeSuccessEnabled)},
		{"email_usage_limit_alert_enabled", strconv.FormatBool(request.EmailUsageLimitAlertEnabled)},
		{"email_content_audit_enabled", strconv.FormatBool(request.EmailContentAuditEnabled)},
		{"email_account_disabled_enabled", strconv.FormatBool(request.EmailAccountDisabledEnabled)},
		{"email_cyber_policy_enabled", strconv.FormatBool(request.EmailCyberPolicyEnabled)},
		{"email_operations_enabled", strconv.FormatBool(request.EmailOperationsEnabled)},
		{"email_balance_threshold", request.BalanceThreshold},
		{"email_recharge_url", request.RechargeURL},
	}
	if !totpEnabled {
		items = append(items, struct{ key, value string }{"admin_mfa_enabled", "false"})
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return FeatureSettings{}, err
	}
	defer func() { _ = tx.Rollback() }()
	for _, item := range items {
		if _, err := tx.ExecContext(ctx, `INSERT INTO platform_settings (key, value, updated_by, updated_at) VALUES ($1, $2, $3, now()) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = EXCLUDED.updated_at`, item.key, item.value, actorID); err != nil {
			return FeatureSettings{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return FeatureSettings{}, err
	}
	return s.GetFeatureSettings(ctx)
}

// EnsureFeatureDefaults writes only settings that do not exist yet. This keeps
// the deployment-time registration default as an initial value while allowing
// administrators to manage the feature from the database afterwards.
func (s *Service) EnsureFeatureDefaults(ctx context.Context, registrationEnabled bool) error {
	if s == nil || s.db == nil {
		return errors.New("feature settings service is not configured")
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO platform_settings (key, value)
		VALUES ('registration_enabled', $1)
		ON CONFLICT (key) DO NOTHING
	`, strconv.FormatBool(registrationEnabled))
	return err
}

func (s *Service) TestSMTPConnection(ctx context.Context) error {
	settings, err := s.readSMTPSettings(ctx, nil)
	if err != nil {
		return err
	}
	if !settings.Configured {
		return ErrInvalidEmailSettings
	}
	return auth.TestSMTPConnection(ctx, settings)
}

func (s *Service) SendTestEmail(ctx context.Context, email string) error {
	if _, err := mail.ParseAddress(strings.TrimSpace(email)); err != nil {
		return ErrInvalidEmailSettings
	}
	settings, err := s.readSMTPSettings(ctx, nil)
	if err != nil {
		return err
	}
	if !settings.Configured {
		return ErrInvalidEmailSettings
	}
	notifier, err := auth.NewSMTPNotifier(settings)
	if err != nil {
		return err
	}
	return notifier.SendTestEmail(ctx, email)
}

func (s *Service) ListEmailTemplates(ctx context.Context) ([]EmailTemplate, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("email template service is not configured")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id::text, event_code, language, subject, html_body, enabled, created_at, updated_at FROM email_templates ORDER BY event_code, language`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]EmailTemplate, 0)
	for rows.Next() {
		var item EmailTemplate
		if err := rows.Scan(&item.ID, &item.EventCode, &item.Language, &item.Subject, &item.HTMLBody, &item.Enabled, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) GetEmailTemplate(ctx context.Context, event, language string) (auth.EmailTemplate, error) {
	if s == nil || s.db == nil {
		return auth.EmailTemplate{}, ErrEmailTemplateNotFound
	}
	var item auth.EmailTemplate
	err := s.db.QueryRowContext(ctx, `SELECT event_code, language, subject, html_body, enabled FROM email_templates WHERE event_code = $1 AND language = $2`, strings.TrimSpace(event), strings.TrimSpace(language)).Scan(&item.EventCode, &item.Language, &item.Subject, &item.HTMLBody, &item.Enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.EmailTemplate{}, ErrEmailTemplateNotFound
	}
	return item, err
}

func (s *Service) CreateEmailTemplate(ctx context.Context, actorID string, request EmailTemplateMutation) (EmailTemplate, error) {
	request, err := validateEmailTemplate(request)
	if err != nil || strings.TrimSpace(actorID) == "" {
		return EmailTemplate{}, ErrInvalidEmailSettings
	}
	id, err := ids.New()
	if err != nil {
		return EmailTemplate{}, err
	}
	enabled := true
	if request.Enabled != nil {
		enabled = *request.Enabled
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO email_templates (id, event_code, language, subject, html_body, enabled, created_by, updated_by) VALUES ($1::uuid, $2, $3, $4, $5, $6, $7::uuid, $7::uuid)`, id, request.EventCode, request.Language, request.Subject, request.HTMLBody, enabled, actorID)
	if err != nil {
		return EmailTemplate{}, err
	}
	return s.getEmailTemplateByEvent(ctx, request.EventCode, request.Language)
}

func (s *Service) UpdateEmailTemplate(ctx context.Context, actorID, templateID string, request EmailTemplateMutation) (EmailTemplate, error) {
	request, err := validateEmailTemplate(request)
	if err != nil || strings.TrimSpace(actorID) == "" || !ids.Valid(templateID) {
		return EmailTemplate{}, ErrInvalidEmailSettings
	}
	enabled := true
	if request.Enabled != nil {
		enabled = *request.Enabled
	}
	result, err := s.db.ExecContext(ctx, `UPDATE email_templates SET event_code = $2, language = $3, subject = $4, html_body = $5, enabled = $6, updated_by = $7::uuid, updated_at = now() WHERE id = $1::uuid`, templateID, request.EventCode, request.Language, request.Subject, request.HTMLBody, enabled, actorID)
	if err != nil {
		return EmailTemplate{}, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return EmailTemplate{}, ErrEmailTemplateNotFound
	}
	return s.getEmailTemplate(ctx, templateID)
}

func (s *Service) DeleteEmailTemplate(ctx context.Context, actorID, templateID string) error {
	if s == nil || s.db == nil || strings.TrimSpace(actorID) == "" || !ids.Valid(templateID) {
		return ErrInvalidEmailSettings
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM email_templates WHERE id = $1::uuid`, templateID)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return ErrEmailTemplateNotFound
	}
	return nil
}

func (s *Service) readSMTPSettings(ctx context.Context, values map[string]string) (auth.SMTPSettings, error) {
	if values == nil {
		var err error
		values, err = s.loadSettings(ctx)
		if err != nil {
			return auth.SMTPSettings{}, err
		}
	}
	host := strings.TrimSpace(values["smtp_host"])
	port, _ := strconv.Atoi(strings.TrimSpace(values["smtp_port"]))
	legacyAddress := strings.TrimSpace(values["smtp_addr"])
	if host == "" && legacyAddress != "" {
		if legacyHost, legacyPort, err := net.SplitHostPort(legacyAddress); err == nil {
			host = legacyHost
			if port == 0 {
				port, _ = strconv.Atoi(legacyPort)
			}
		}
	}
	if port == 0 {
		port = 587
	}
	from := strings.TrimSpace(values["smtp_from_email"])
	fromName := strings.TrimSpace(values["smtp_from_name"])
	if from == "" {
		legacyFrom := strings.TrimSpace(values["smtp_from"])
		if parsed, err := mail.ParseAddress(legacyFrom); err == nil {
			from, fromName = parsed.Address, firstNonEmpty(fromName, parsed.Name)
		} else {
			from = legacyFrom
		}
	}
	tls := true
	if raw, ok := values["smtp_tls"]; ok && strings.TrimSpace(raw) != "" {
		tls = parseSettingBool(raw, true)
	}
	password := ""
	if encrypted := strings.TrimSpace(values["smtp_password"]); encrypted != "" {
		if s.box == nil {
			return auth.SMTPSettings{}, errors.New("smtp secret box is not configured")
		}
		plain, err := s.box.Open(encrypted)
		if err != nil {
			return auth.SMTPSettings{}, errors.New("smtp password is invalid")
		}
		password = string(plain)
	}
	baseURL := strings.TrimRight(strings.TrimSpace(values["public_base_url"]), "/")
	result := auth.SMTPSettings{Host: host, Port: port, From: from, FromName: fromName, Username: strings.TrimSpace(values["smtp_username"]), Password: password, BaseURL: baseURL, SiteName: strings.TrimSpace(values["site_name"]), TLS: tls}
	result.Configured = validSMTPHost(host) && port > 0 && port <= 65535 && validSMTPFromEmail(from) && ((result.Username == "") == (password == ""))
	if host != "" {
		result.Address = net.JoinHostPort(host, strconv.Itoa(port))
	}
	return result, nil
}

func (s *Service) loadSettings(ctx context.Context) (map[string]string, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("settings service is not configured")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT key, value, updated_at, updated_by::text FROM platform_settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make(map[string]string)
	for rows.Next() {
		var key, value string
		var updatedAt sql.NullTime
		var updatedBy sql.NullString
		if err := rows.Scan(&key, &value, &updatedAt, &updatedBy); err != nil {
			return nil, err
		}
		values[key] = value
		if updatedAt.Valid {
			values["__updated_at"] = updatedAt.Time.UTC().Format(time.RFC3339Nano)
			values["__updated_by"] = updatedBy.String
		}
	}
	return values, rows.Err()
}

func latestSettingUpdate(values map[string]string) (time.Time, string) {
	value, err := time.Parse(time.RFC3339Nano, values["__updated_at"])
	if err != nil {
		return time.Time{}, ""
	}
	return value, values["__updated_by"]
}

func (s *Service) getEmailTemplate(ctx context.Context, id string) (EmailTemplate, error) {
	var item EmailTemplate
	err := s.db.QueryRowContext(ctx, `SELECT id::text, event_code, language, subject, html_body, enabled, created_at, updated_at FROM email_templates WHERE id = $1::uuid`, id).Scan(&item.ID, &item.EventCode, &item.Language, &item.Subject, &item.HTMLBody, &item.Enabled, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return EmailTemplate{}, ErrEmailTemplateNotFound
	}
	return item, err
}

func (s *Service) getEmailTemplateByEvent(ctx context.Context, event, language string) (EmailTemplate, error) {
	var item EmailTemplate
	err := s.db.QueryRowContext(ctx, `SELECT id::text, event_code, language, subject, html_body, enabled, created_at, updated_at FROM email_templates WHERE event_code = $1 AND language = $2`, event, language).Scan(&item.ID, &item.EventCode, &item.Language, &item.Subject, &item.HTMLBody, &item.Enabled, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return EmailTemplate{}, ErrEmailTemplateNotFound
	}
	return item, err
}

func validateEmailTemplate(request EmailTemplateMutation) (EmailTemplateMutation, error) {
	request.EventCode = strings.ToLower(strings.TrimSpace(request.EventCode))
	request.Language = strings.ToLower(strings.TrimSpace(request.Language))
	request.Subject = strings.TrimSpace(request.Subject)
	request.HTMLBody = strings.TrimSpace(request.HTMLBody)
	if request.EventCode == "" || len(request.EventCode) > 100 || request.Language != "zh" && request.Language != "en" || request.Subject == "" || len(request.Subject) > 300 || request.HTMLBody == "" || len(request.HTMLBody) > 100000 || strings.ContainsAny(request.Subject, "\r\n") || strings.Contains(strings.ToLower(request.HTMLBody), "<script") || strings.Contains(strings.ToLower(request.HTMLBody), "javascript:") {
		return EmailTemplateMutation{}, ErrInvalidEmailSettings
	}
	for _, char := range request.EventCode {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' && char != '-' && char != '.' {
			return EmailTemplateMutation{}, ErrInvalidEmailSettings
		}
	}
	return request, nil
}

func validSMTPHost(value string) bool {
	return value != "" && len(value) <= 253 && !strings.ContainsAny(value, ":/\\ \t\r\n")
}

func validSMTPFromEmail(value string) bool {
	parsed, err := mail.ParseAddress(value)
	return err == nil && parsed.Address == value
}

func validHTTPSURL(value string) bool {
	if value == "" {
		return true
	}
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == ""
}

func parseSettingBool(value string, fallback bool) bool {
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func normalizeDecimalOrZero(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "0"
	}
	parsed, ok := new(big.Float).SetString(value)
	if !ok || parsed.Sign() < 0 {
		return "0"
	}
	return value
}

func validNonNegativeDecimal(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 32 {
		return false
	}
	parsed, ok := new(big.Float).SetString(value)
	return ok && parsed.Sign() >= 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

var _ auth.SMTPSettingsProvider = (*Service)(nil)
var _ auth.EmailFeatureProvider = (*Service)(nil)
var _ auth.EmailEventFeatureProvider = (*Service)(nil)
var _ auth.EmailTemplateProvider = (*Service)(nil)
