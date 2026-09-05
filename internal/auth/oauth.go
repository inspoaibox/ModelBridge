package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ai-token/internal/ids"
	"ai-token/internal/passwords"
)

var (
	ErrOAuthProviderNotConfigured = errors.New("oauth provider is not configured")
	ErrOAuthProviderDisabled      = errors.New("oauth provider is disabled")
	ErrOAuthInvalidState          = errors.New("oauth state is invalid")
	ErrOAuthCallbackFailed        = errors.New("oauth callback failed")
	ErrOAuthRegistrationDisabled  = errors.New("oauth registration is disabled")
)

type OAuthProviderConfig struct {
	Provider         string
	Enabled          bool
	ClientID         string
	ClientSecret     string
	AuthorizationURL string
	TokenURL         string
	UserInfoURL      string
	Scopes           []string
}

type OAuthProviderInfo struct {
	Provider string `json:"provider"`
	Enabled  bool   `json:"enabled"`
}

type OAuthSettingsProvider interface {
	GetOAuthProvider(context.Context, string) (OAuthProviderConfig, error)
	ListOAuthProviders(context.Context) ([]OAuthProviderInfo, error)
}

type OAuthRegistrationGate interface {
	RegistrationEnabled(context.Context) (bool, error)
}

type OAuthLoginProvider interface {
	Start(context.Context, string, string) (string, error)
	Callback(context.Context, string, string, string, string) (IssuedSession, error)
	Providers(context.Context) ([]OAuthProviderInfo, error)
}

type SQLOAuthService struct {
	db       *sql.DB
	sessions *SessionIssuer
	settings OAuthSettingsProvider
	client   *http.Client
}

func NewSQLOAuthService(db *sql.DB, sessions *SessionIssuer, settings OAuthSettingsProvider) (*SQLOAuthService, error) {
	if db == nil || sessions == nil || settings == nil {
		return nil, errors.New("database, session issuer and oauth settings are required")
	}
	return &SQLOAuthService{db: db, sessions: sessions, settings: settings, client: &http.Client{Timeout: 20 * time.Second}}, nil
}

func (s *SQLOAuthService) Providers(ctx context.Context) ([]OAuthProviderInfo, error) {
	if s == nil || s.settings == nil {
		return nil, ErrOAuthProviderNotConfigured
	}
	return s.settings.ListOAuthProviders(ctx)
}

func (s *SQLOAuthService) Start(ctx context.Context, provider, redirectURI string) (string, error) {
	cfg, err := s.settings.GetOAuthProvider(ctx, provider)
	if err != nil {
		return "", err
	}
	if !cfg.Enabled {
		return "", ErrOAuthProviderDisabled
	}
	if cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.AuthorizationURL == "" {
		return "", ErrOAuthProviderNotConfigured
	}
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", err
	}
	state := "oauth_" + base64.RawURLEncoding.EncodeToString(stateBytes)
	if _, err := s.db.ExecContext(ctx, `INSERT INTO oauth_states (state, provider, redirect_uri, expires_at) VALUES ($1, $2, $3, $4)`, state, strings.ToLower(strings.TrimSpace(provider)), redirectURI, time.Now().UTC().Add(10*time.Minute)); err != nil {
		return "", err
	}
	parsed, err := url.Parse(cfg.AuthorizationURL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("client_id", cfg.ClientID)
	query.Set("redirect_uri", redirectURI)
	query.Set("response_type", "code")
	query.Set("scope", strings.Join(cfg.Scopes, " "))
	query.Set("state", state)
	if cfg.Provider == "google" {
		query.Set("access_type", "online")
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func (s *SQLOAuthService) Callback(ctx context.Context, provider, code, state, redirectURI string) (IssuedSession, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	code, state, redirectURI = strings.TrimSpace(code), strings.TrimSpace(state), strings.TrimSpace(redirectURI)
	if code == "" || state == "" || redirectURI == "" {
		return IssuedSession{}, ErrOAuthInvalidState
	}
	var storedProvider, storedRedirect string
	var expiresAt time.Time
	err := s.db.QueryRowContext(ctx, `DELETE FROM oauth_states WHERE state = $1 AND expires_at > now() RETURNING provider, redirect_uri, expires_at`, state).Scan(&storedProvider, &storedRedirect, &expiresAt)
	if err != nil || storedProvider != provider || storedRedirect != redirectURI || expiresAt.Before(time.Now().UTC()) {
		return IssuedSession{}, ErrOAuthInvalidState
	}
	cfg, err := s.settings.GetOAuthProvider(ctx, provider)
	if err != nil || !cfg.Enabled || cfg.ClientID == "" || cfg.ClientSecret == "" {
		return IssuedSession{}, ErrOAuthProviderNotConfigured
	}
	token, err := s.exchangeCode(ctx, cfg, code, redirectURI)
	if err != nil {
		return IssuedSession{}, err
	}
	identity, err := s.fetchIdentity(ctx, cfg, token)
	if err != nil {
		return IssuedSession{}, err
	}
	userID, tenantID, err := s.resolveIdentity(ctx, provider, identity)
	if err != nil {
		return IssuedSession{}, err
	}
	return s.sessions.Issue(ctx, userID, tenantID, AudienceConsole, "oauth_"+provider)
}

func (s *SQLOAuthService) SetHTTPClient(client *http.Client) {
	if client != nil {
		s.client = client
	}
}

type oauthIdentity struct{ Subject, Email, Name string }

func (s *SQLOAuthService) exchangeCode(ctx context.Context, cfg OAuthProviderConfig, code, redirectURI string) (string, error) {
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirectURI}, "client_id": {cfg.ClientID}, "client_secret": {cfg.ClientSecret}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return "", ErrOAuthCallbackFailed
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", ErrOAuthCallbackFailed
	}
	var payload struct {
		AccessToken string `json:"access_token"`
	}
	if json.Unmarshal(raw, &payload) != nil || payload.AccessToken == "" {
		values, parseErr := url.ParseQuery(string(raw))
		if parseErr == nil {
			payload.AccessToken = values.Get("access_token")
		}
	}
	if payload.AccessToken == "" {
		return "", ErrOAuthCallbackFailed
	}
	return payload.AccessToken, nil
}

func (s *SQLOAuthService) fetchIdentity(ctx context.Context, cfg OAuthProviderConfig, token string) (oauthIdentity, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.UserInfoURL, nil)
	if err != nil {
		return oauthIdentity{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "AI-Token-Gateway")
	resp, err := s.client.Do(req)
	if err != nil {
		return oauthIdentity{}, ErrOAuthCallbackFailed
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return oauthIdentity{}, ErrOAuthCallbackFailed
	}
	var payload map[string]any
	if json.Unmarshal(raw, &payload) != nil {
		return oauthIdentity{}, ErrOAuthCallbackFailed
	}
	identity := oauthIdentity{Subject: firstString(payload, "sub", "id", "uid", "user_id"), Email: normalizeEmail(firstString(payload, "email", "mail")), Name: firstString(payload, "name", "preferred_username", "username", "login")}
	if identity.Email == "" && cfg.Provider == "github" {
		identity.Email = normalizeEmail(s.fetchGitHubPrimaryEmail(ctx, cfg, token))
	}
	if identity.Subject == "" || !validRegistrationEmail(identity.Email) {
		return oauthIdentity{}, ErrOAuthCallbackFailed
	}
	return identity, nil
}

func (s *SQLOAuthService) fetchGitHubPrimaryEmail(ctx context.Context, cfg OAuthProviderConfig, token string) string {
	endpoint := strings.TrimRight(cfg.UserInfoURL, "/") + "/emails"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	response, err := s.client.Do(req)
	if err != nil {
		return ""
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ""
	}
	raw, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	var items []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if json.Unmarshal(raw, &items) != nil {
		return ""
	}
	for _, item := range items {
		if item.Primary && item.Verified {
			return item.Email
		}
	}
	for _, item := range items {
		if item.Verified {
			return item.Email
		}
	}
	return ""
}

func firstString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
		if value, ok := payload[key].(float64); ok {
			return fmt.Sprintf("%.0f", value)
		}
	}
	return ""
}

func (s *SQLOAuthService) resolveIdentity(ctx context.Context, provider string, identity oauthIdentity) (string, string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = tx.Rollback() }()
	var userID string
	err = tx.QueryRowContext(ctx, `SELECT user_id::text FROM oauth_identities WHERE provider = $1 AND subject = $2`, provider, identity.Subject).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, `SELECT id::text FROM users WHERE lower(email) = lower($1) AND deleted_at IS NULL`, identity.Email).Scan(&userID)
		if errors.Is(err, sql.ErrNoRows) {
			if gate, ok := s.settings.(OAuthRegistrationGate); ok {
				enabled, gateErr := gate.RegistrationEnabled(ctx)
				if gateErr != nil {
					return "", "", gateErr
				}
				if !enabled {
					return "", "", ErrOAuthRegistrationDisabled
				}
			}
			userID, err = createOAuthAccount(ctx, tx, provider, identity)
		}
		if err != nil {
			return "", "", err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE users SET status = 'active', email_verified_at = COALESCE(email_verified_at, now()), updated_at = now() WHERE id = $1::uuid AND status = 'pending'`, userID); err != nil {
			return "", "", err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO oauth_identities (provider, subject, user_id, email) VALUES ($1, $2, $3::uuid, $4) ON CONFLICT (provider, subject) DO UPDATE SET user_id = EXCLUDED.user_id, email = EXCLUDED.email`, provider, identity.Subject, userID, identity.Email); err != nil {
			return "", "", err
		}
	} else if err != nil {
		return "", "", err
	}
	var tenantID string
	err = tx.QueryRowContext(ctx, `SELECT tm.tenant_id::text FROM tenant_members tm JOIN tenants t ON t.id = tm.tenant_id WHERE tm.user_id = $1::uuid AND tm.status = 'active' AND t.status = 'active' AND t.deleted_at IS NULL ORDER BY tm.created_at LIMIT 1`, userID).Scan(&tenantID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrOAuthCallbackFailed
	}
	if err != nil {
		return "", "", err
	}
	if err := tx.Commit(); err != nil {
		return "", "", err
	}
	return userID, tenantID, nil
}

func createOAuthAccount(ctx context.Context, tx *sql.Tx, provider string, identity oauthIdentity) (string, error) {
	userID, err := ids.New()
	if err != nil {
		return "", err
	}
	tenantID, err := ids.New()
	if err != nil {
		return "", err
	}
	projectID, err := ids.New()
	if err != nil {
		return "", err
	}
	memberID, err := ids.New()
	if err != nil {
		return "", err
	}
	accountID, err := ids.New()
	if err != nil {
		return "", err
	}
	passwordBytes := make([]byte, 32)
	if _, err := rand.Read(passwordBytes); err != nil {
		return "", err
	}
	passwordHash, err := passwords.Hash("oauth_" + base64.RawURLEncoding.EncodeToString(passwordBytes))
	if err != nil {
		return "", err
	}
	name := identity.Name
	if name == "" {
		name = identity.Email
	}
	localPart := strings.ToLower(strings.ReplaceAll(strings.Split(identity.Email, "@")[0], ".", "-"))
	if len(localPart) > 24 {
		localPart = localPart[:24]
	}
	slug := "oauth-" + provider + "-" + localPart + "-" + strings.ToLower(base64.RawURLEncoding.EncodeToString(passwordBytes[:6]))
	slug = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, slug)
	if _, err := tx.ExecContext(ctx, `INSERT INTO users (id, email, password_hash, display_name, status, password_changed_at) VALUES ($1, $2, $3, $4, 'active', now())`, userID, identity.Email, passwordHash, name); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO tenants (id, name, slug, status) VALUES ($1, $2, $3, 'active')`, tenantID, name+" workspace", slug); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO projects (id, tenant_id, name, slug, status, created_by) VALUES ($1, $2, 'Default Project', 'default', 'active', $3)`, projectID, tenantID, userID); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO tenant_members (id, tenant_id, user_id, role_code, status, created_by) VALUES ($1, $2, $3, 'tenant_owner', 'active', $3)`, memberID, tenantID, userID); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO project_members (project_id, user_id, role_code) VALUES ($1, $2, 'project_admin')`, projectID, userID); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO ledger_accounts (id, tenant_id, account_type, currency, status) VALUES ($1, $2, 'prepaid_balance', 'USD', 'active')`, accountID, tenantID); err != nil {
		return "", err
	}
	return userID, nil
}

var _ OAuthLoginProvider = (*SQLOAuthService)(nil)
