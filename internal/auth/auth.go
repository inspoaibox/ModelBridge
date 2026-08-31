package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type Audience string

const (
	AudienceAdmin    Audience = "admin"
	AudienceConsole  Audience = "console"
	AudienceRelay    Audience = "relay"
	AudienceInternal Audience = "internal"
)

type PrincipalType string

const (
	PrincipalPlatformUser PrincipalType = "platform_user"
	PrincipalTenantUser   PrincipalType = "tenant_user"
	PrincipalAPIToken     PrincipalType = "api_token"
	PrincipalService      PrincipalType = "service"
)

type Principal struct {
	ID             string
	Type           PrincipalType
	Audience       Audience
	TenantID       string
	GroupID        string
	ProjectIDs     map[string]struct{}
	Roles          []string
	Permissions    map[string]struct{}
	Scopes         map[string]struct{}
	AllowedModels  map[string]struct{}
	AllowedIPs     map[string]struct{}
	AllowedDomains map[string]struct{}
	SessionID      string
	TokenID        string
	AuthStrength   string
}

func (p *Principal) HasPermission(permission string) bool {
	if p == nil {
		return false
	}
	_, ok := p.Permissions[permission]
	return ok
}

func (p *Principal) HasScope(scope string) bool {
	if p == nil {
		return false
	}
	_, ok := p.Scopes[scope]
	return ok
}

type Resolver interface {
	Resolve(ctx context.Context, credential string) (*Principal, error)
}

type SessionResolver interface {
	ResolveSession(ctx context.Context, sessionID string, audience Audience) (*Principal, error)
}

type ResolverFunc func(context.Context, string) (*Principal, error)

func (f ResolverFunc) Resolve(ctx context.Context, credential string) (*Principal, error) {
	return f(ctx, credential)
}

type SessionResolverFunc func(context.Context, string, Audience) (*Principal, error)

func (f SessionResolverFunc) ResolveSession(ctx context.Context, sessionID string, audience Audience) (*Principal, error) {
	return f(ctx, sessionID, audience)
}

type contextKey struct{}

func withPrincipal(ctx context.Context, principal *Principal) context.Context {
	return context.WithValue(ctx, contextKey{}, principal)
}

func PrincipalFromContext(ctx context.Context) (*Principal, bool) {
	principal, ok := ctx.Value(contextKey{}).(*Principal)
	return principal, ok && principal != nil
}

type Middleware struct {
	resolver        Resolver
	sessionResolver SessionResolver
}

func NewMiddleware(resolver Resolver) *Middleware {
	return NewCredentialMiddleware(resolver, nil)
}

func NewCredentialMiddleware(tokenResolver Resolver, sessionResolver SessionResolver) *Middleware {
	return &Middleware{
		resolver:        tokenResolver,
		sessionResolver: sessionResolver,
	}
}

func (m *Middleware) Authenticate(next http.Handler) http.Handler {
	return m.authenticateFor("", next)
}

func (m *Middleware) authenticateFor(audience Audience, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization := r.Header.Get("Authorization")
		if strings.TrimSpace(authorization) == "" && audience == AudienceRelay {
			apiKey := strings.TrimSpace(r.Header.Get("X-API-Key"))
			if apiKey == "" {
				apiKey = strings.TrimSpace(r.Header.Get("x-api-key"))
			}
			if apiKey != "" {
				authorization = "Bearer " + apiKey
			}
		}
		credential, present, err := bearerCredential(authorization)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "AUTH_INVALID")
			return
		}
		if present {
			if m == nil || m.resolver == nil {
				writeError(w, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE")
				return
			}
			principal, err := m.resolver.Resolve(r.Context(), credential)
			if err != nil || principal == nil {
				writeError(w, http.StatusUnauthorized, "AUTH_INVALID")
				return
			}
			*r = *r.WithContext(withPrincipal(r.Context(), principal))
			next.ServeHTTP(w, r)
			return
		}

		if audience != "" {
			cookieName := sessionCookieName(audience)
			if cookie, cookieErr := r.Cookie(cookieName); cookieErr == nil && cookie.Value != "" {
				if m == nil || m.sessionResolver == nil {
					writeError(w, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE")
					return
				}
				principal, resolveErr := m.sessionResolver.ResolveSession(r.Context(), cookie.Value, audience)
				if resolveErr != nil || principal == nil {
					writeError(w, http.StatusUnauthorized, "AUTH_INVALID")
					return
				}
				*r = *r.WithContext(withPrincipal(r.Context(), principal))
				next.ServeHTTP(w, r)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func (m *Middleware) Protect(audience Audience, permissions ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		handler := next
		for i := len(permissions) - 1; i >= 0; i-- {
			handler = RequirePermission(permissions[i])(handler)
		}
		handler = RequireAudience(audience)(handler)
		handler = RequireAuth(handler)
		return m.authenticateFor(audience, handler)
	}
}

func (m *Middleware) ProtectScopes(audience Audience, scopes ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		handler := next
		for i := len(scopes) - 1; i >= 0; i-- {
			handler = RequireScope(scopes[i])(handler)
		}
		handler = RequireAudience(audience)(handler)
		handler = RequireAuth(handler)
		return m.authenticateFor(audience, handler)
	}
}

func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := PrincipalFromContext(r.Context()); !ok {
			writeError(w, http.StatusUnauthorized, "AUTH_REQUIRED")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func RequireAudience(expected Audience) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := PrincipalFromContext(r.Context())
			if !ok || principal.Audience != expected {
				writeError(w, http.StatusForbidden, "PERMISSION_DENIED")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func RequirePermission(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := PrincipalFromContext(r.Context())
			if !ok || !principal.HasPermission(permission) {
				writeError(w, http.StatusForbidden, "PERMISSION_DENIED")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := PrincipalFromContext(r.Context())
			if !ok || !principal.HasScope(scope) {
				writeError(w, http.StatusForbidden, "PERMISSION_DENIED")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func RequireTenantPath(pathValueName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := PrincipalFromContext(r.Context())
			if !ok || principal.TenantID == "" {
				writeError(w, http.StatusNotFound, "RESOURCE_NOT_FOUND")
				return
			}
			if target := r.PathValue(pathValueName); target == "" || target != principal.TenantID {
				writeError(w, http.StatusNotFound, "RESOURCE_NOT_FOUND")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func bearerCredential(header string) (credential string, present bool, err error) {
	if strings.TrimSpace(header) == "" {
		return "", false, nil
	}
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", true, errors.New("invalid bearer authorization")
	}
	return parts[1], true, nil
}

func sessionCookieName(audience Audience) string {
	switch audience {
	case AudienceAdmin:
		return "admin_session"
	case AudienceConsole:
		return "console_session"
	default:
		return ""
	}
}

func SessionCookieName(audience Audience) string {
	return sessionCookieName(audience)
}

func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}
