package adminsettings

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"ai-token/internal/auth"
)

type LoginProviderSettings struct {
	Provider               string `json:"provider"`
	Enabled                bool   `json:"enabled"`
	ClientID               string `json:"client_id"`
	ClientSecretConfigured bool   `json:"client_secret_configured"`
	AuthorizationURL       string `json:"authorization_url"`
	TokenURL               string `json:"token_url"`
	UserInfoURL            string `json:"userinfo_url"`
	Scopes                 string `json:"scopes"`
}

type LoginSettings struct {
	Providers []LoginProviderSettings `json:"providers"`
}

type LoginProviderUpdate struct {
	Provider          string `json:"provider"`
	Enabled           bool   `json:"enabled"`
	ClientID          string `json:"client_id"`
	ClientSecret      string `json:"client_secret"`
	ClearClientSecret bool   `json:"clear_client_secret"`
	AuthorizationURL  string `json:"authorization_url"`
	TokenURL          string `json:"token_url"`
	UserInfoURL       string `json:"userinfo_url"`
	Scopes            string `json:"scopes"`
}

type LoginSettingsUpdate struct {
	Providers []LoginProviderUpdate `json:"providers"`
}

type LoginSettingsProvider interface {
	GetLoginSettings(context.Context) (LoginSettings, error)
	UpdateLoginSettings(context.Context, string, LoginSettingsUpdate) (LoginSettings, error)
}

type oauthProviderRecord struct {
	Provider         string `json:"provider"`
	Enabled          bool   `json:"enabled"`
	ClientID         string `json:"client_id"`
	ClientSecret     string `json:"client_secret"`
	AuthorizationURL string `json:"authorization_url"`
	TokenURL         string `json:"token_url"`
	UserInfoURL      string `json:"userinfo_url"`
	Scopes           string `json:"scopes"`
}

var defaultOAuthProviders = []oauthProviderRecord{
	{Provider: "google", AuthorizationURL: "https://accounts.google.com/o/oauth2/v2/auth", TokenURL: "https://oauth2.googleapis.com/token", UserInfoURL: "https://openidconnect.googleapis.com/v1/userinfo", Scopes: "openid email profile"},
	{Provider: "github", AuthorizationURL: "https://github.com/login/oauth/authorize", TokenURL: "https://github.com/login/oauth/access_token", UserInfoURL: "https://api.github.com/user", Scopes: "read:user user:email"},
	{Provider: "linuxdo", AuthorizationURL: "https://connect.linux.do/oauth2/authorize", TokenURL: "https://connect.linux.do/oauth2/token", UserInfoURL: "https://connect.linux.do/api/user", Scopes: "openid profile email"},
}

func (s *Service) GetLoginSettings(ctx context.Context) (LoginSettings, error) {
	records, err := s.loadOAuthRecords(ctx)
	if err != nil {
		return LoginSettings{}, err
	}
	items := make([]LoginProviderSettings, 0, len(records))
	for _, record := range records {
		items = append(items, LoginProviderSettings{
			Provider: record.Provider, Enabled: record.Enabled, ClientID: record.ClientID,
			ClientSecretConfigured: strings.TrimSpace(record.ClientSecret) != "",
			AuthorizationURL:       record.AuthorizationURL, TokenURL: record.TokenURL,
			UserInfoURL: record.UserInfoURL, Scopes: record.Scopes,
		})
	}
	return LoginSettings{Providers: items}, nil
}

func (s *Service) UpdateLoginSettings(ctx context.Context, actorID string, request LoginSettingsUpdate) (LoginSettings, error) {
	if s == nil || s.db == nil || s.box == nil || strings.TrimSpace(actorID) == "" || len(request.Providers) != len(defaultOAuthProviders) {
		return LoginSettings{}, ErrInvalidSystemSettings
	}
	current, err := s.loadOAuthRecords(ctx)
	if err != nil {
		return LoginSettings{}, err
	}
	byProvider := make(map[string]oauthProviderRecord, len(current))
	for _, item := range current {
		byProvider[item.Provider] = item
	}
	seen := make(map[string]struct{}, len(request.Providers))
	updated := make([]oauthProviderRecord, 0, len(request.Providers))
	for _, input := range request.Providers {
		provider := strings.ToLower(strings.TrimSpace(input.Provider))
		if !oauthProviderSupported(provider) {
			return LoginSettings{}, ErrInvalidSystemSettings
		}
		if _, ok := seen[provider]; ok {
			return LoginSettings{}, ErrInvalidSystemSettings
		}
		seen[provider] = struct{}{}
		base := byProvider[provider]
		if base.Provider == "" {
			base = defaultOAuthProvider(provider)
		}
		base.Enabled = input.Enabled
		base.ClientID = strings.TrimSpace(input.ClientID)
		base.AuthorizationURL = strings.TrimSpace(input.AuthorizationURL)
		base.TokenURL = strings.TrimSpace(input.TokenURL)
		base.UserInfoURL = strings.TrimSpace(input.UserInfoURL)
		base.Scopes = strings.TrimSpace(input.Scopes)
		if base.AuthorizationURL == "" {
			base.AuthorizationURL = defaultOAuthProvider(provider).AuthorizationURL
		}
		if base.TokenURL == "" {
			base.TokenURL = defaultOAuthProvider(provider).TokenURL
		}
		if base.UserInfoURL == "" {
			base.UserInfoURL = defaultOAuthProvider(provider).UserInfoURL
		}
		if base.Scopes == "" {
			base.Scopes = defaultOAuthProvider(provider).Scopes
		}
		if input.ClearClientSecret {
			base.ClientSecret = ""
		} else if strings.TrimSpace(input.ClientSecret) != "" {
			base.ClientSecret = strings.TrimSpace(input.ClientSecret)
		}
		if base.Enabled && (base.ClientID == "" || base.ClientSecret == "") || !validOAuthURL(base.AuthorizationURL) || !validOAuthURL(base.TokenURL) || !validOAuthURL(base.UserInfoURL) || len(base.ClientID) > 512 || len(base.ClientSecret) > 4096 || len(base.Scopes) > 512 {
			return LoginSettings{}, ErrInvalidSystemSettings
		}
		updated = append(updated, base)
	}
	for _, provider := range []string{"google", "github", "linuxdo"} {
		if _, ok := seen[provider]; !ok {
			return LoginSettings{}, ErrInvalidSystemSettings
		}
	}
	encoded, err := json.Marshal(updated)
	if err != nil {
		return LoginSettings{}, err
	}
	sealed, err := s.box.Seal(encoded)
	if err != nil {
		return LoginSettings{}, err
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO platform_settings (key, value, updated_by, updated_at) VALUES ('login_providers', $1, $2, now())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = now()
	`, sealed, actorID); err != nil {
		return LoginSettings{}, err
	}
	return s.GetLoginSettings(ctx)
}

func (s *Service) GetOAuthProvider(ctx context.Context, provider string) (auth.OAuthProviderConfig, error) {
	records, err := s.loadOAuthRecords(ctx)
	if err != nil {
		return auth.OAuthProviderConfig{}, err
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	for _, item := range records {
		if item.Provider != provider {
			continue
		}
		return auth.OAuthProviderConfig{Provider: item.Provider, Enabled: item.Enabled, ClientID: item.ClientID, ClientSecret: item.ClientSecret, AuthorizationURL: item.AuthorizationURL, TokenURL: item.TokenURL, UserInfoURL: item.UserInfoURL, Scopes: strings.Fields(item.Scopes)}, nil
	}
	return auth.OAuthProviderConfig{}, auth.ErrOAuthProviderNotConfigured
}

func (s *Service) ListOAuthProviders(ctx context.Context) ([]auth.OAuthProviderInfo, error) {
	settings, err := s.GetLoginSettings(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]auth.OAuthProviderInfo, 0, len(settings.Providers))
	for _, item := range settings.Providers {
		result = append(result, auth.OAuthProviderInfo{Provider: item.Provider, Enabled: item.Enabled && item.ClientID != "" && item.ClientSecretConfigured})
	}
	return result, nil
}

func (s *Service) loadOAuthRecords(ctx context.Context) ([]oauthProviderRecord, error) {
	if s == nil || s.db == nil {
		return nil, ErrInvalidSystemSettings
	}
	var encoded string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM platform_settings WHERE key = 'login_providers'`).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		result := make([]oauthProviderRecord, len(defaultOAuthProviders))
		copy(result, defaultOAuthProviders)
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	if s.box == nil {
		return nil, ErrInvalidSystemSettings
	}
	plain, err := s.box.Open(encoded)
	if err != nil {
		return nil, ErrInvalidSystemSettings
	}
	var records []oauthProviderRecord
	if err := json.Unmarshal(plain, &records); err != nil {
		return nil, ErrInvalidSystemSettings
	}
	for index := range records {
		defaults := defaultOAuthProvider(records[index].Provider)
		if defaults.Provider == "" {
			return nil, ErrInvalidSystemSettings
		}
		if records[index].AuthorizationURL == "" {
			records[index].AuthorizationURL = defaults.AuthorizationURL
		}
		if records[index].TokenURL == "" {
			records[index].TokenURL = defaults.TokenURL
		}
		if records[index].UserInfoURL == "" {
			records[index].UserInfoURL = defaults.UserInfoURL
		}
		if records[index].Scopes == "" {
			records[index].Scopes = defaults.Scopes
		}
	}
	return records, nil
}

func defaultOAuthProvider(provider string) oauthProviderRecord {
	for _, item := range defaultOAuthProviders {
		if item.Provider == provider {
			return item
		}
	}
	return oauthProviderRecord{}
}

func oauthProviderSupported(provider string) bool {
	return provider == "google" || provider == "github" || provider == "linuxdo"
}

func (s *Service) RegistrationEnabled(ctx context.Context) (bool, error) {
	features, err := s.GetFeatureSettings(ctx)
	if err != nil {
		return false, err
	}
	return features.RegistrationEnabled, nil
}

func validOAuthURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == ""
}

var _ LoginSettingsProvider = (*Service)(nil)
