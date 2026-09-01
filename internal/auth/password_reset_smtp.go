package auth

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/mail"
	"net/smtp"
	"net/url"
	"strconv"
	"strings"
)

var ErrEmailDisabled = errors.New("email system is disabled")

type SMTPPasswordResetNotifier struct {
	address  string
	from     string
	fromName string
	host     string
	auth     smtp.Auth
	baseURL  string
	siteName string
	tls      bool
}

// SMTPSettings is the internal, decrypted SMTP configuration used by the
// password reset notifier. HTTP handlers must expose only the configured
// boolean, never Password.
type SMTPSettings struct {
	Address    string
	Host       string
	Port       int
	From       string
	FromName   string
	Username   string
	Password   string
	BaseURL    string
	SiteName   string
	TLS        bool
	Configured bool
}

type SMTPSettingsProvider interface {
	GetSMTPSettings(context.Context) (SMTPSettings, error)
}

type EmailFeatureProvider interface {
	EmailEnabled(context.Context) (bool, error)
}

type EmailEventFeatureProvider interface {
	EmailFeatureEnabled(context.Context, string) (bool, error)
}

type EmailTemplate struct {
	EventCode string
	Language  string
	Subject   string
	HTMLBody  string
	Enabled   bool
}

type EmailTemplateProvider interface {
	GetEmailTemplate(context.Context, string, string) (EmailTemplate, error)
}

// DynamicSMTPPasswordResetNotifier reads settings for every message so an
// administrator can rotate SMTP credentials without restarting the process.
// Fallback is used only for fields that have not been configured in the DB.
type DynamicSMTPPasswordResetNotifier struct {
	provider SMTPSettingsProvider
	fallback SMTPSettings
}

func NewDynamicSMTPPasswordResetNotifier(provider SMTPSettingsProvider, fallback SMTPSettings) *DynamicSMTPPasswordResetNotifier {
	return &DynamicSMTPPasswordResetNotifier{provider: provider, fallback: fallback}
}

func (n *DynamicSMTPPasswordResetNotifier) SendPasswordReset(ctx context.Context, email, token string) error {
	return n.sendTemplateMail(ctx, "password_reset", email, map[string]string{
		"reset_url": n.fallback.BaseURL + "/#reset?token=" + url.QueryEscape(token),
	}, "AI Token Gateway password reset", "Use this link to reset your password. It expires in 30 minutes.", token)
}

func (n *DynamicSMTPPasswordResetNotifier) SendEmailVerification(ctx context.Context, email, token string) error {
	return n.sendTemplateMail(ctx, "email_verification", email, map[string]string{
		"verification_url": n.fallback.BaseURL + "/#verify-email?token=" + url.QueryEscape(token),
	}, "AI Token Gateway email verification", "Use this link to verify your email address. It expires in 30 minutes.", token)
}

func (n *DynamicSMTPPasswordResetNotifier) EmailEnabled(ctx context.Context) (bool, error) {
	if n == nil {
		return false, errors.New("smtp notifier is not configured")
	}
	if provider, ok := n.provider.(EmailFeatureProvider); ok {
		return provider.EmailEnabled(ctx)
	}
	return n.fallback.Configured, nil
}

func (n *DynamicSMTPPasswordResetNotifier) notifier(ctx context.Context) (*SMTPPasswordResetNotifier, error) {
	if n == nil {
		return nil, errors.New("smtp notifier is not configured")
	}
	enabled, err := n.EmailEnabled(ctx)
	if err != nil {
		return nil, err
	}
	if !enabled {
		return nil, ErrEmailDisabled
	}
	settings := n.fallback
	if n.provider != nil {
		loaded, err := n.provider.GetSMTPSettings(ctx)
		if err != nil {
			return nil, err
		}
		if loaded.Configured {
			settings = loaded
			if strings.TrimSpace(settings.BaseURL) == "" {
				settings.BaseURL = n.fallback.BaseURL
			}
		} else {
			settings = mergeSMTPSettings(settings, loaded)
		}
	}
	return NewSMTPNotifier(settings)
}

func (n *DynamicSMTPPasswordResetNotifier) sendTemplateMail(ctx context.Context, event, email string, values map[string]string, defaultSubject, defaultText, token string) error {
	notifier, err := n.notifier(ctx)
	if err != nil {
		return err
	}
	if provider, ok := n.provider.(EmailEventFeatureProvider); ok {
		enabled, featureErr := provider.EmailFeatureEnabled(ctx, event)
		if featureErr != nil {
			return featureErr
		}
		if !enabled {
			return ErrEmailDisabled
		}
	}
	if event == "password_reset" {
		values["reset_url"] = notifier.baseURL + "/#reset?token=" + url.QueryEscape(token)
	}
	if event == "email_verification" {
		values["verification_url"] = notifier.baseURL + "/#verify-email?token=" + url.QueryEscape(token)
	}
	values["site_name"] = notifier.siteName
	values["user_email"] = strings.TrimSpace(email)
	values["token"] = token
	subject, htmlBody := defaultSubject, "<p>"+defaultText+"</p>"
	if provider, ok := n.provider.(EmailTemplateProvider); ok {
		if template, templateErr := provider.GetEmailTemplate(ctx, event, "zh"); templateErr == nil && template.Enabled {
			subject, htmlBody = template.Subject, template.HTMLBody
		}
	}
	return notifier.sendTemplateMail(ctx, email, renderEmailTemplate(subject, values), renderEmailTemplate(htmlBody, values), defaultText)
}

func mergeSMTPSettings(fallback, configured SMTPSettings) SMTPSettings {
	if strings.TrimSpace(configured.Address) != "" {
		fallback.Address = configured.Address
	}
	if strings.TrimSpace(configured.Host) != "" {
		fallback.Host = configured.Host
	}
	if configured.Port > 0 {
		fallback.Port = configured.Port
	}
	if strings.TrimSpace(configured.From) != "" {
		fallback.From = configured.From
	}
	if strings.TrimSpace(configured.FromName) != "" {
		fallback.FromName = configured.FromName
	}
	if strings.TrimSpace(configured.Username) != "" {
		fallback.Username = configured.Username
	}
	if strings.TrimSpace(configured.Password) != "" {
		fallback.Password = configured.Password
	}
	if strings.TrimSpace(configured.BaseURL) != "" {
		fallback.BaseURL = configured.BaseURL
	}
	if strings.TrimSpace(configured.SiteName) != "" {
		fallback.SiteName = configured.SiteName
	}
	fallback.TLS = configured.TLS
	return fallback
}

func NewSMTPPasswordResetNotifier(address, from, username, password, baseURL string) (*SMTPPasswordResetNotifier, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, errors.New("smtp address, sender, and public base URL are required")
	}
	notifier, err := NewSMTPNotifier(SMTPSettings{Address: address, From: from, Username: username, Password: password, BaseURL: baseURL, TLS: true})
	if err != nil {
		return nil, err
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("public base URL must be an HTTPS URL")
	}
	notifier.baseURL = baseURL
	return notifier, nil
}

func NewSMTPNotifier(settings SMTPSettings) (*SMTPPasswordResetNotifier, error) {
	address := strings.TrimSpace(settings.Address)
	host := strings.TrimSpace(settings.Host)
	port := settings.Port
	if address != "" {
		parsedHost, parsedPort, err := net.SplitHostPort(address)
		if err != nil {
			return nil, errors.New("smtp address must include host and port")
		}
		host = parsedHost
		if port == 0 {
			if parsed, scanErr := strconv.Atoi(parsedPort); scanErr != nil {
				return nil, errors.New("smtp port is invalid")
			} else {
				port = parsed
			}
		}
	}
	if host == "" {
		return nil, errors.New("smtp host is required")
	}
	if port <= 0 || port > 65535 {
		return nil, errors.New("smtp port is invalid")
	}
	if address == "" {
		address = net.JoinHostPort(host, strconv.Itoa(port))
	}
	from := strings.TrimSpace(settings.From)
	if from == "" {
		return nil, errors.New("smtp sender is required")
	}
	fromAddress, err := mail.ParseAddress(from)
	if err != nil {
		return nil, errors.New("smtp sender must be a valid email address")
	}
	username := strings.TrimSpace(settings.Username)
	password := settings.Password
	if (username == "") != (strings.TrimSpace(password) == "") {
		return nil, errors.New("smtp username and password must be configured together")
	}
	var smtpAuth smtp.Auth
	if username != "" {
		smtpAuth = smtp.PlainAuth("", username, password, host)
	}
	fromName := strings.TrimSpace(settings.FromName)
	if fromName == "" {
		fromName = fromAddress.Name
	}
	siteName := strings.TrimSpace(settings.SiteName)
	if siteName == "" {
		siteName = "AI Token Gateway"
	}
	return &SMTPPasswordResetNotifier{address: address, from: fromAddress.Address, fromName: fromName, host: host, auth: smtpAuth, baseURL: strings.TrimRight(strings.TrimSpace(settings.BaseURL), "/"), siteName: siteName, tls: settings.TLS}, nil
}

func (n *SMTPPasswordResetNotifier) SendPasswordReset(ctx context.Context, email, token string) error {
	if n == nil || strings.TrimSpace(email) == "" || strings.TrimSpace(token) == "" {
		return errors.New("password reset notifier is not configured")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	link := n.baseURL + "/#reset?token=" + url.QueryEscape(token)
	return n.sendTemplateMail(ctx, email, "AI Token Gateway password reset", "<p>Use this link to reset your password. It expires in 30 minutes.</p><p><a href=\""+link+"\">Reset password</a></p>", "Use this link to reset your password. It expires in 30 minutes.\r\n\r\n"+link+"\r\n")
}

func (n *SMTPPasswordResetNotifier) SendEmailVerification(ctx context.Context, email, token string) error {
	if n == nil || strings.TrimSpace(email) == "" || strings.TrimSpace(token) == "" {
		return errors.New("email verification notifier is not configured")
	}
	link := n.baseURL + "/#verify-email?token=" + url.QueryEscape(token)
	return n.sendTemplateMail(ctx, email, "AI Token Gateway email verification", "<p>Use this link to verify your email address. It expires in 30 minutes.</p><p><a href=\""+link+"\">Verify email</a></p>", "Use this link to verify your email address. It expires in 30 minutes.\r\n\r\n"+link+"\r\n")
}

func (n *SMTPPasswordResetNotifier) sendTextMail(ctx context.Context, email, subject, body string) error {
	return n.sendTemplateMail(ctx, email, subject, "", body)
}

func (n *SMTPPasswordResetNotifier) SendTestEmail(ctx context.Context, email string) error {
	return n.sendTemplateMail(ctx, email, "AI Token Gateway SMTP test", "<p>This is a test email from AI Token Gateway.</p>", "This is a test email from AI Token Gateway.\r\n")
}

func TestSMTPConnection(ctx context.Context, settings SMTPSettings) error {
	notifier, err := NewSMTPNotifier(settings)
	if err != nil {
		return err
	}
	connection, err := (&net.Dialer{}).DialContext(ctx, "tcp", notifier.address)
	if err != nil {
		return err
	}
	defer connection.Close()
	client, err := smtp.NewClient(connection, notifier.host)
	if err != nil {
		return err
	}
	defer client.Close()
	if notifier.tls {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("smtp server does not support STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{ServerName: notifier.host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	}
	if notifier.auth != nil {
		if err := client.Auth(notifier.auth); err != nil {
			return err
		}
	}
	return client.Quit()
}

func (n *SMTPPasswordResetNotifier) sendTemplateMail(ctx context.Context, email, subject, htmlBody, textBody string) error {
	if n == nil || strings.TrimSpace(email) == "" {
		return errors.New("smtp notifier is not configured")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	recipient, err := mail.ParseAddress(strings.TrimSpace(email))
	if err != nil {
		return err
	}
	fromHeader := n.from
	if n.fromName != "" {
		fromHeader = (&mail.Address{Name: n.fromName, Address: n.from}).String()
	}
	if strings.ContainsAny(subject, "\r\n") {
		return errors.New("smtp subject is invalid")
	}
	contentType := "text/plain; charset=UTF-8"
	body := textBody
	if htmlBody != "" {
		contentType = "text/html; charset=UTF-8"
		body = htmlBody
	}
	message := []byte("To: " + recipient.Address + "\r\nFrom: " + fromHeader + "\r\nSubject: " + subject + "\r\nMIME-Version: 1.0\r\nContent-Type: " + contentType + "\r\n\r\n" + body)
	connection, err := (&net.Dialer{}).DialContext(ctx, "tcp", n.address)
	if err != nil {
		return err
	}
	defer connection.Close()
	client, err := smtp.NewClient(connection, n.host)
	if err != nil {
		return err
	}
	defer client.Close()
	if n.tls {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("smtp server does not support STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{ServerName: n.host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	}
	if n.auth != nil {
		if err := client.Auth(n.auth); err != nil {
			return err
		}
	}
	if err := client.Mail(n.from); err != nil {
		return err
	}
	if err := client.Rcpt(recipient.Address); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(message); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func renderEmailTemplate(value string, values map[string]string) string {
	for key, item := range values {
		value = strings.ReplaceAll(value, "{{"+key+"}}", item)
	}
	return value
}

var _ PasswordResetNotifier = (*SMTPPasswordResetNotifier)(nil)
var _ EmailVerificationNotifier = (*SMTPPasswordResetNotifier)(nil)
var _ EmailVerificationNotifier = (*DynamicSMTPPasswordResetNotifier)(nil)
