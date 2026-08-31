package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type testResolver map[string]*Principal

func (r testResolver) Resolve(_ context.Context, credential string) (*Principal, error) {
	return r[credential], nil
}

func TestProtectRejectsMissingCredential(t *testing.T) {
	middleware := NewMiddleware(testResolver{})
	handler := middleware.Protect(AudienceAdmin, "audit:read")(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/admin/v1/me", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestProtectRejectsWrongAudienceAndPermission(t *testing.T) {
	middleware := NewMiddleware(testResolver{
		"console-token": {
			ID:          "user-1",
			Type:        PrincipalTenantUser,
			Audience:    AudienceConsole,
			TenantID:    "tenant-a",
			Permissions: map[string]struct{}{"usage:read": {}},
		},
		"admin-token": {
			ID:          "admin-1",
			Type:        PrincipalPlatformUser,
			Audience:    AudienceAdmin,
			Permissions: map[string]struct{}{"user:freeze": {}},
		},
	})

	tests := []struct {
		name       string
		token      string
		audience   Audience
		permission string
		want       int
	}{
		{"wrong audience", "console-token", AudienceAdmin, "usage:read", http.StatusForbidden},
		{"missing permission", "admin-token", AudienceAdmin, "audit:read", http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := middleware.Protect(tt.audience, tt.permission)(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusNoContent)
				}),
			)
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			req.Header.Set("Authorization", "Bearer "+tt.token)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("expected %d, got %d", tt.want, rec.Code)
			}
		})
	}
}

func TestRequireTenantPathDoesNotLeakOtherTenant(t *testing.T) {
	middleware := NewMiddleware(testResolver{
		"tenant-token": {
			ID:          "user-1",
			Type:        PrincipalTenantUser,
			Audience:    AudienceConsole,
			TenantID:    "tenant-a",
			Permissions: map[string]struct{}{"usage:read": {}},
		},
	})
	handler := middleware.Protect(AudienceConsole, "usage:read")(
		RequireTenantPath("tenantID")(
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}),
		),
	)

	req := httptest.NewRequest(http.MethodGet, "/console/v1/tenants/tenant-b/usage", nil)
	req.SetPathValue("tenantID", "tenant-b")
	req.Header.Set("Authorization", "Bearer tenant-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestProtectAcceptsAudienceSpecificSessionCookie(t *testing.T) {
	middleware := NewCredentialMiddleware(
		nil,
		SessionResolverFunc(func(_ context.Context, sessionID string, audience Audience) (*Principal, error) {
			if sessionID != "session-1" || audience != AudienceAdmin {
				return nil, errors.New("unexpected session")
			}
			return &Principal{
				ID:          "admin-1",
				Type:        PrincipalPlatformUser,
				Audience:    AudienceAdmin,
				Permissions: map[string]struct{}{"audit:read": {}},
			}, nil
		}),
	)
	handler := middleware.Protect(AudienceAdmin, "audit:read")(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/admin/v1/me", nil)
	req.AddCookie(&http.Cookie{Name: "admin_session", Value: "session-1"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

func TestProtectDoesNotUseAdminCookieForConsole(t *testing.T) {
	middleware := NewCredentialMiddleware(
		nil,
		SessionResolverFunc(func(_ context.Context, _ string, _ Audience) (*Principal, error) {
			t.Fatal("admin cookie must not resolve for console route")
			return nil, nil
		}),
	)
	handler := middleware.Protect(AudienceConsole, "usage:read")(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/console/v1/usage", nil)
	req.AddCookie(&http.Cookie{Name: "admin_session", Value: "admin-session"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestAuthenticateLeavesPrincipalOnOriginalRequestContext(t *testing.T) {
	seen := false
	middleware := NewMiddleware(testResolver{
		"admin-token": {
			ID:       "admin-1",
			Type:     PrincipalPlatformUser,
			Audience: AudienceAdmin,
		},
	})
	handler := middleware.Authenticate(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, seen = PrincipalFromContext(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/admin/v1/me", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if !seen {
		t.Fatal("authenticated principal must be available on the request passed through middleware")
	}
}
