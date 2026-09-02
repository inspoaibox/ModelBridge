package adminsettings

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode"

	"ai-token/internal/ids"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrAPIEndpointNotFound = errors.New("api endpoint is not found")
	ErrAPIEndpointExists   = errors.New("api endpoint already exists")
	ErrInvalidAPIEndpoint  = errors.New("invalid api endpoint")
)

type APIEndpoint struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	BaseURL          string    `json:"base_url"`
	OpenAIBaseURL    string    `json:"openai_base_url"`
	AnthropicBaseURL string    `json:"anthropic_base_url"`
	Enabled          bool      `json:"enabled"`
	SortOrder        int       `json:"sort_order"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type PublicAPIEndpoint struct {
	Name             string `json:"name"`
	BaseURL          string `json:"base_url"`
	OpenAIBaseURL    string `json:"openai_base_url"`
	AnthropicBaseURL string `json:"anthropic_base_url"`
}

type APIEndpointMutation struct {
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	Enabled *bool  `json:"enabled,omitempty"`
}

type APIEndpointProvider interface {
	ListAPIEndpoints(context.Context, bool) ([]APIEndpoint, error)
	CreateAPIEndpoint(context.Context, string, APIEndpointMutation) (APIEndpoint, error)
	UpdateAPIEndpoint(context.Context, string, string, APIEndpointMutation) (APIEndpoint, error)
	DeleteAPIEndpoint(context.Context, string, string) error
}

func (s *Service) ListAPIEndpoints(ctx context.Context, enabledOnly bool) ([]APIEndpoint, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("api endpoint service is not configured")
	}
	query := `
		SELECT id::text, name, base_url, enabled, sort_order, created_at, updated_at
		FROM api_endpoints
	`
	if enabledOnly {
		query += ` WHERE enabled = true`
	}
	query += ` ORDER BY sort_order ASC, name ASC, id ASC`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]APIEndpoint, 0)
	for rows.Next() {
		var item APIEndpoint
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.BaseURL,
			&item.Enabled,
			&item.SortOrder,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item = hydrateAPIEndpoint(item)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Service) CreateAPIEndpoint(ctx context.Context, actorID string, request APIEndpointMutation) (APIEndpoint, error) {
	name, baseURL, enabled, err := normalizeAPIEndpointMutation(request, true)
	if err != nil || strings.TrimSpace(actorID) == "" {
		return APIEndpoint{}, ErrInvalidAPIEndpoint
	}
	if s == nil || s.db == nil {
		return APIEndpoint{}, errors.New("api endpoint service is not configured")
	}
	id, err := ids.New()
	if err != nil {
		return APIEndpoint{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return APIEndpoint{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var sortOrder int
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(sort_order), -1) + 1
		FROM api_endpoints
	`).Scan(&sortOrder); err != nil {
		return APIEndpoint{}, err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO api_endpoints (id, name, base_url, enabled, sort_order, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, '')::uuid, NULLIF($6, '')::uuid)
	`, id, name, baseURL, enabled, sortOrder, actorID)
	if isUniqueViolation(err) {
		return APIEndpoint{}, ErrAPIEndpointExists
	}
	if err != nil {
		return APIEndpoint{}, err
	}
	if err := tx.Commit(); err != nil {
		return APIEndpoint{}, err
	}
	return s.getAPIEndpoint(ctx, id)
}

func (s *Service) UpdateAPIEndpoint(ctx context.Context, actorID, endpointID string, request APIEndpointMutation) (APIEndpoint, error) {
	if strings.TrimSpace(actorID) == "" || !ids.Valid(strings.TrimSpace(endpointID)) {
		return APIEndpoint{}, ErrInvalidAPIEndpoint
	}
	name, baseURL, enabled, err := normalizeAPIEndpointMutation(request, false)
	if err != nil {
		return APIEndpoint{}, err
	}
	if s == nil || s.db == nil {
		return APIEndpoint{}, errors.New("api endpoint service is not configured")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return APIEndpoint{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var currentEnabled bool
	if err := tx.QueryRowContext(ctx, `
		SELECT enabled FROM api_endpoints WHERE id = $1 FOR UPDATE
	`, endpointID).Scan(&currentEnabled); errors.Is(err, sql.ErrNoRows) {
		return APIEndpoint{}, ErrAPIEndpointNotFound
	} else if err != nil {
		return APIEndpoint{}, err
	}
	if request.Enabled == nil {
		enabled = currentEnabled
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE api_endpoints
		SET name = $2, base_url = $3, enabled = $4, updated_by = NULLIF($5, '')::uuid, updated_at = now()
		WHERE id = $1
	`, endpointID, name, baseURL, enabled, actorID)
	if isUniqueViolation(err) {
		return APIEndpoint{}, ErrAPIEndpointExists
	}
	if err != nil {
		return APIEndpoint{}, err
	}
	if err := tx.Commit(); err != nil {
		return APIEndpoint{}, err
	}
	return s.getAPIEndpoint(ctx, endpointID)
}

func (s *Service) DeleteAPIEndpoint(ctx context.Context, actorID, endpointID string) error {
	if s == nil || s.db == nil {
		return errors.New("api endpoint service is not configured")
	}
	if strings.TrimSpace(actorID) == "" || !ids.Valid(strings.TrimSpace(endpointID)) {
		return ErrInvalidAPIEndpoint
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM api_endpoints WHERE id = $1`, endpointID)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected == 0 {
		return ErrAPIEndpointNotFound
	}
	return nil
}

func (s *Service) getAPIEndpoint(ctx context.Context, endpointID string) (APIEndpoint, error) {
	var item APIEndpoint
	err := s.db.QueryRowContext(ctx, `
		SELECT id::text, name, base_url, enabled, sort_order, created_at, updated_at
		FROM api_endpoints
		WHERE id = $1
	`, endpointID).Scan(
		&item.ID,
		&item.Name,
		&item.BaseURL,
		&item.Enabled,
		&item.SortOrder,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return APIEndpoint{}, ErrAPIEndpointNotFound
	}
	if err != nil {
		return APIEndpoint{}, err
	}
	item = hydrateAPIEndpoint(item)
	return item, nil
}

func normalizeAPIEndpointMutation(request APIEndpointMutation, creating bool) (string, string, bool, error) {
	name := strings.TrimSpace(request.Name)
	baseURL, err := normalizeAPIEndpointBaseURL(request.BaseURL)
	if name == "" || len(name) > 100 || baseURL == "" || len(baseURL) > 2048 || err != nil {
		return "", "", false, ErrInvalidAPIEndpoint
	}
	enabled := true
	if request.Enabled != nil {
		enabled = *request.Enabled
	} else if !creating {
		enabled = true
	}
	return name, baseURL, enabled, nil
}

// normalizeAPIEndpointBaseURL stores the gateway root, while accepting the
// OpenAI-compatible "/v1" form administrators commonly paste from SDK docs.
// A path prefix is preserved, so https://example.com/gateway/v1 becomes
// https://example.com/gateway.
func normalizeAPIEndpointBaseURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.IndexFunc(value, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}) >= 0 {
		return "", ErrInvalidAPIEndpoint
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Hostname() == "" {
		return "", ErrInvalidAPIEndpoint
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", ErrInvalidAPIEndpoint
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
		return "", ErrInvalidAPIEndpoint
	}

	path := strings.TrimRight(parsed.Path, "/")
	if path != "" {
		segments := strings.Split(path, "/")
		if len(segments) > 1 && strings.EqualFold(segments[len(segments)-1], "v1") {
			path = strings.TrimSuffix(path, "/"+segments[len(segments)-1])
		}
	}
	parsed.Path = path
	parsed.RawPath = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func hydrateAPIEndpoint(item APIEndpoint) APIEndpoint {
	root, err := normalizeAPIEndpointBaseURL(item.BaseURL)
	if err != nil {
		root = strings.TrimRight(strings.TrimSpace(item.BaseURL), "/")
	}
	item.BaseURL = root
	item.OpenAIBaseURL, item.AnthropicBaseURL = protocolBaseURLs(root)
	return item
}

func protocolBaseURLs(root string) (string, string) {
	root = strings.TrimRight(strings.TrimSpace(root), "/")
	if root == "" {
		return "", ""
	}
	return root + "/v1", root
}

// PublicAPIEndpointFrom converts an administrator endpoint to the safe,
// customer-facing representation without exposing internal identifiers.
func PublicAPIEndpointFrom(item APIEndpoint) PublicAPIEndpoint {
	item = hydrateAPIEndpoint(item)
	return PublicAPIEndpoint{
		Name:             item.Name,
		BaseURL:          item.BaseURL,
		OpenAIBaseURL:    item.OpenAIBaseURL,
		AnthropicBaseURL: item.AnthropicBaseURL,
	}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
