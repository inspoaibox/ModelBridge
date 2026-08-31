package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	"ai-token/internal/ids"
	"ai-token/internal/tokens"
)

type SessionIssuer struct {
	db     *sql.DB
	hasher *tokens.Hasher
	ttl    time.Duration
	now    func() time.Time
	random func([]byte) error
}

type IssuedSession struct {
	ID        string
	Secret    string
	Audience  Audience
	ExpiresAt time.Time
}

func NewSessionIssuer(db *sql.DB, hasher *tokens.Hasher, ttl time.Duration) (*SessionIssuer, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	if hasher == nil {
		return nil, errors.New("session hasher is required")
	}
	if ttl <= 0 {
		return nil, errors.New("session ttl must be positive")
	}
	return &SessionIssuer{
		db:     db,
		hasher: hasher,
		ttl:    ttl,
		now:    time.Now,
		random: func(value []byte) error {
			_, err := rand.Read(value)
			return err
		},
	}, nil
}

func (i *SessionIssuer) Issue(
	ctx context.Context,
	userID string,
	tenantID string,
	audience Audience,
	authStrength string,
) (IssuedSession, error) {
	if i == nil || i.db == nil || i.hasher == nil {
		return IssuedSession{}, errors.New("session issuer is not configured")
	}
	if userID == "" || authStrength == "" {
		return IssuedSession{}, errors.New("user and auth strength are required")
	}
	if audience != AudienceAdmin && audience != AudienceConsole {
		return IssuedSession{}, errors.New("unsupported session audience")
	}
	if audience == AudienceConsole && tenantID == "" {
		return IssuedSession{}, errors.New("console session requires tenant")
	}
	if audience == AudienceAdmin && tenantID != "" {
		return IssuedSession{}, errors.New("admin session cannot be tenant scoped")
	}

	secretBytes := make([]byte, 32)
	if err := i.random(secretBytes); err != nil {
		return IssuedSession{}, err
	}
	secret := "sess_" + base64.RawURLEncoding.EncodeToString(secretBytes)
	sessionID, err := ids.New()
	if err != nil {
		return IssuedSession{}, err
	}
	expiresAt := i.now().Add(i.ttl)

	var tenantValue any
	if tenantID != "" {
		tenantValue = tenantID
	}
	_, err = i.db.ExecContext(ctx, `
		INSERT INTO web_sessions (
			id, user_id, tenant_id, audience, session_hash,
			auth_strength, expires_at, last_seen_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $7)
	`, sessionID, userID, tenantValue, string(audience), i.hasher.Digest(secret),
		authStrength, expiresAt)
	if err != nil {
		return IssuedSession{}, err
	}

	return IssuedSession{
		ID:        sessionID,
		Secret:    secret,
		Audience:  audience,
		ExpiresAt: expiresAt,
	}, nil
}

func (i *SessionIssuer) Revoke(ctx context.Context, secret string) error {
	if i == nil || i.db == nil || i.hasher == nil {
		return errors.New("session issuer is not configured")
	}
	if secret == "" {
		return errors.New("session secret is required")
	}
	_, err := i.db.ExecContext(ctx, `
		UPDATE web_sessions
		SET revoked_at = COALESCE(revoked_at, now())
		WHERE session_hash = $1 AND revoked_at IS NULL
	`, i.hasher.Digest(secret))
	return err
}

func SetSessionCookie(w http.ResponseWriter, session IssuedSession, secure bool) error {
	name := sessionCookieName(session.Audience)
	if name == "" || session.Secret == "" || session.ExpiresAt.IsZero() {
		return errors.New("invalid issued session")
	}
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    session.Secret,
		Path:     "/",
		Expires:  session.ExpiresAt,
		MaxAge:   maxAge(session.ExpiresAt),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func maxAge(expiresAt time.Time) int {
	seconds := int(time.Until(expiresAt).Seconds())
	if seconds < 1 {
		return 1
	}
	return seconds
}
