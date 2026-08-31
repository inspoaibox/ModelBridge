package relay

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"regexp"
	"strings"
)

var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type EnvCredentialResolver struct {
	Lookup func(string) string
}

func (r EnvCredentialResolver) Resolve(_ context.Context, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if !strings.HasPrefix(strings.ToLower(ref), "env:") {
		return "", ErrCredentialUnavailable
	}
	name := strings.TrimSpace(ref[4:])
	if !envNamePattern.MatchString(name) {
		return "", ErrCredentialUnavailable
	}
	lookup := r.Lookup
	if lookup == nil {
		lookup = os.Getenv
	}
	secret := strings.TrimSpace(lookup(name))
	if secret == "" {
		return "", ErrCredentialUnavailable
	}
	return secret, nil
}

type SQLCredentialResolver struct {
	db       *sql.DB
	box      SecretBox
	fallback CredentialResolver
}

func NewSQLCredentialResolver(
	db *sql.DB,
	box SecretBox,
	fallback CredentialResolver,
) (*SQLCredentialResolver, error) {
	if db == nil || box == nil {
		return nil, errors.New("database and secret box are required")
	}
	return &SQLCredentialResolver{
		db:       db,
		box:      box,
		fallback: fallback,
	}, nil
}

func (r *SQLCredentialResolver) Resolve(ctx context.Context, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(strings.ToLower(ref), "secret:") {
		return r.resolveStoredSecret(ctx, strings.TrimSpace(ref[7:]))
	}
	if r.fallback != nil {
		return r.fallback.Resolve(ctx, ref)
	}
	return "", ErrCredentialUnavailable
}

func (r *SQLCredentialResolver) resolveStoredSecret(ctx context.Context, secretID string) (string, error) {
	if r == nil || r.db == nil || r.box == nil || secretID == "" {
		return "", ErrCredentialUnavailable
	}
	var encrypted []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT encrypted_secret
		FROM channel_secrets
		WHERE id = $1
		  AND revoked_at IS NULL
	`, secretID).Scan(&encrypted)
	if err != nil {
		return "", ErrCredentialUnavailable
	}
	secret, err := r.box.Open(string(encrypted))
	if err != nil {
		return "", ErrCredentialUnavailable
	}
	if strings.TrimSpace(string(secret)) == "" {
		return "", ErrCredentialUnavailable
	}
	return strings.TrimSpace(string(secret)), nil
}
