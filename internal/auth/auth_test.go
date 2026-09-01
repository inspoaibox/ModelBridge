package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"ai-token/internal/mfa"
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

type stepUpTestVerifier struct{ err error }

func (v stepUpTestVerifier) Verify(context.Context, string, string) error { return v.err }

func TestRequireStepUpEnforcesCodeAndVerifier(t *testing.T) {
	principal := &Principal{ID: "admin-1", Type: PrincipalPlatformUser, Audience: AudienceAdmin}
	called := 0
	endpoint := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { called++; w.WriteHeader(http.StatusNoContent) })

	missing := httptest.NewRequest(http.MethodPost, "/sensitive", nil).WithContext(withPrincipal(context.Background(), principal))
	missingRec := httptest.NewRecorder()
	RequireStepUp(stepUpTestVerifier{})(endpoint).ServeHTTP(missingRec, missing)
	if missingRec.Code != http.StatusForbidden || called != 0 {
		t.Fatalf("missing step-up code must be rejected: %d %s", missingRec.Code, missingRec.Body.String())
	}

	invalid := httptest.NewRequest(http.MethodPost, "/sensitive", nil).WithContext(withPrincipal(context.Background(), principal))
	invalid.Header.Set("X-MFA-Code", "123456")
	invalidRec := httptest.NewRecorder()
	RequireStepUp(stepUpTestVerifier{err: mfa.ErrMFAInvalidCode})(endpoint).ServeHTTP(invalidRec, invalid)
	if invalidRec.Code != http.StatusUnauthorized || called != 0 {
		t.Fatalf("invalid step-up code must be rejected: %d %s", invalidRec.Code, invalidRec.Body.String())
	}

	ok := httptest.NewRequest(http.MethodPost, "/sensitive", nil).WithContext(withPrincipal(context.Background(), principal))
	ok.Header.Set("X-MFA-Code", "123456")
	okRec := httptest.NewRecorder()
	RequireStepUp(stepUpTestVerifier{})(endpoint).ServeHTTP(okRec, ok)
	if okRec.Code != http.StatusNoContent || called != 1 {
		t.Fatalf("valid step-up code must reach endpoint: %d %s", okRec.Code, okRec.Body.String())
	}
}

func TestRequireStepUpReturnsRetryableThrottle(t *testing.T) {
	principal := &Principal{ID: "admin-1", Type: PrincipalPlatformUser, Audience: AudienceAdmin}
	req := httptest.NewRequest(http.MethodPost, "/sensitive", nil).WithContext(withPrincipal(context.Background(), principal))
	req.Header.Set("X-MFA-Code", "123456")
	rec := httptest.NewRecorder()
	RequireStepUp(stepUpTestVerifier{err: mfa.ErrMFAThrottled})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "900" {
		t.Fatalf("expected throttled step-up response, got %d %q", rec.Code, rec.Header().Get("Retry-After"))
	}
}
