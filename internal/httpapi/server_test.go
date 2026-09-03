package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"ai-token/internal/adminsettings"
	"ai-token/internal/auth"
	"ai-token/internal/billing"
	"ai-token/internal/groups"
	"ai-token/internal/mfa"
	"ai-token/internal/modelprices"
	"ai-token/internal/models"
	"ai-token/internal/relay"
	"ai-token/internal/tokens"
	"ai-token/internal/users"
)

type testResolver map[string]*auth.Principal

func (r testResolver) Resolve(_ context.Context, credential string) (*auth.Principal, error) {
	return r[credential], nil
}

func TestHealthIsPublicAndProtectedRoutesAreNot(t *testing.T) {
	handler := New(auth.NewMiddleware(testResolver{}), nil, false, "../../web")

	healthReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthRec := httptest.NewRecorder()
	handler.ServeHTTP(healthRec, healthReq)
	if healthRec.Code != http.StatusOK {
		t.Fatalf("expected health 200, got %d", healthRec.Code)
	}
	if healthRec.Header().Get("X-Request-ID") == "" {
		t.Fatal("expected request id")
	}

	adminReq := httptest.NewRequest(http.MethodGet, "/admin/v1/me", nil)
	adminRec := httptest.NewRecorder()
	handler.ServeHTTP(adminRec, adminReq)
	if adminRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected admin 401, got %d", adminRec.Code)
	}
}

func TestClientIPOnlyTrustsForwardingHeadersFromConfiguredProxies(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "127.0.0.1/32,::1/128")

	proxied := httptest.NewRequest(http.MethodGet, "/", nil)
	proxied.RemoteAddr = "127.0.0.1:4567"
	proxied.Header.Set("X-Forwarded-For", "203.0.113.25")
	if actual := clientIP(proxied); actual != "203.0.113.25" {
		t.Fatalf("trusted proxy client IP = %q", actual)
	}

	direct := httptest.NewRequest(http.MethodGet, "/", nil)
	direct.RemoteAddr = "198.51.100.25:4567"
	direct.Header.Set("X-Forwarded-For", "203.0.113.25")
	if actual := clientIP(direct); actual != "198.51.100.25" {
		t.Fatalf("untrusted peer must not control forwarded IP, got %q", actual)
	}
}

func TestCookieMutationAcceptsSameOriginRefererPath(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://gateway.example.com")
	handler := withSecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	allowed := httptest.NewRequest(http.MethodPost, "/console/v1/profile", nil)
	allowed.Host = "gateway.example.com"
	allowed.Header.Set("Referer", "https://gateway.example.com/console/profile")
	allowed.AddCookie(&http.Cookie{Name: auth.SessionCookieName(auth.AudienceConsole), Value: "session"})
	allowedRec := httptest.NewRecorder()
	handler.ServeHTTP(allowedRec, allowed)
	if allowedRec.Code != http.StatusNoContent {
		t.Fatalf("same-origin referer path should be accepted, got %d: %s", allowedRec.Code, allowedRec.Body.String())
	}

	denied := httptest.NewRequest(http.MethodPost, "/console/v1/profile", nil)
	denied.Host = "gateway.example.com"
	denied.Header.Set("Referer", "https://attacker.example/console/profile")
	denied.AddCookie(&http.Cookie{Name: auth.SessionCookieName(auth.AudienceConsole), Value: "session"})
	deniedRec := httptest.NewRecorder()
	handler.ServeHTTP(deniedRec, denied)
	if deniedRec.Code != http.StatusForbidden {
		t.Fatalf("cross-origin referer should be rejected, got %d", deniedRec.Code)
	}
}

func TestCORSAllowsOfficialProviderHeaders(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://app.example.com")
	handler := withSecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodOptions, "/messages", nil)
	req.Host = "gateway.example.com"
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Headers", "x-api-key, anthropic-version")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("allowed provider preflight should succeed, got %d: %s", rec.Code, rec.Body.String())
	}
	allowedHeaders := strings.ToLower(rec.Header().Get("Access-Control-Allow-Headers"))
	for _, header := range []string{"x-api-key", "anthropic-version", "anthropic-beta", "openai-beta"} {
		if !strings.Contains(allowedHeaders, header) {
			t.Fatalf("CORS headers must include %q, got %q", header, allowedHeaders)
		}
	}
}

func TestSecurityHeadersAllowConfiguredHTTPSBrandAssets(t *testing.T) {
	handler := withSecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/console/v1/profile", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	policy := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(policy, "img-src 'self' https:") || !strings.Contains(policy, "frame-ancestors 'none'") {
		t.Fatalf("unexpected content security policy: %q", policy)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("API responses must not be cacheable, got %q", rec.Header().Get("Cache-Control"))
	}
}

func TestPaymentReturnURLIsRestrictedToKnownOrigin(t *testing.T) {
	t.Setenv("PUBLIC_BASE_URL", "https://gateway.example.com")
	request := httptest.NewRequest(http.MethodPost, "/console/v1/tenants/tenant/billing/recharge", nil)
	request.Host = "gateway.example.com"
	request.Header.Set("Origin", "https://gateway.example.com")

	for _, test := range []struct {
		name  string
		value string
		want  bool
	}{
		{name: "same origin", value: "https://gateway.example.com/console/billing", want: true},
		{name: "same origin with console hash route", value: "https://gateway.example.com/#console/billing", want: true},
		{name: "configured origin", value: "https://gateway.example.com", want: true},
		{name: "external origin", value: "https://attacker.example/collect", want: false},
		{name: "credentials", value: "https://user:password@gateway.example.com", want: false},
		{name: "query", value: "https://gateway.example.com/?next=https://attacker.example", want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := safeReturnURL(test.value, request); got != test.want {
				t.Fatalf("safeReturnURL(%q) = %t, want %t", test.value, got, test.want)
			}
		})
	}
}

func TestUnknownPathsDoNotExposeFrontendSources(t *testing.T) {
	handler := New(auth.NewMiddleware(testResolver{}), nil, false, "../../web")
	for _, target := range []string{"/admin/v1/unknown", "/src/App.tsx", "/.env.local"} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected %s to return 404, got %d: %s", target, rec.Code, rec.Body.String())
		}
	}
}

func TestRecoveryReturnsGenericInternalError(t *testing.T) {
	handler := withRecovery(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("sensitive failure") }))
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "INTERNAL_SERVER_ERROR") || strings.Contains(rec.Body.String(), "sensitive") {
		t.Fatalf("unexpected recovery response: %d %s", rec.Code, rec.Body.String())
	}
}

func TestPublicModelCatalogReturnsSafeModelSummaries(t *testing.T) {
	handler := NewWithRelayAndGroupsAndTokensAndConsoleTokensAndModelCatalog(
		auth.NewMiddleware(testResolver{}),
		nil,
		nil,
		false,
		"../../web",
		nil,
		nil,
		nil,
		fakeModelCatalog{},
	)
	req := httptest.NewRequest(http.MethodGet, "/public/v1/models", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected public model catalog 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"name":"gpt-5"`) || strings.Contains(rec.Body.String(), "api_key") {
		t.Fatalf("unexpected public model response: %s", rec.Body.String())
	}
}

func TestOfficialPriceSyncRouteRequiresPricePublishPermission(t *testing.T) {
	service := &fakePriceSyncService{}
	handler := NewWithRelayAndGroupsAndTokensAndConsoleTokensAndModelCatalogAndUsersAndPriceSync(auth.NewMiddleware(testResolver{
		"price-publish": {
			ID:       "11111111-1111-4111-8111-111111111111",
			Type:     auth.PrincipalPlatformUser,
			Audience: auth.AudienceAdmin,
			Permissions: map[string]struct{}{
				"price:publish": {},
			},
		},
	}), &auth.Services{StepUpMFA: &fakeStepUpVerifier{}}, nil, false, "../../web", nil, nil, nil, nil, nil, service)

	req := httptest.NewRequest(http.MethodPost, "/admin/v1/prices/sync-official", nil)
	req.Header.Set("Authorization", "Bearer price-publish")
	req.Header.Set("X-MFA-Code", "123456")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || service.called != 1 {
		t.Fatalf("expected price sync 200, got %d: %s", rec.Code, rec.Body.String())
	}

	denied := httptest.NewRequest(http.MethodPost, "/admin/v1/prices/sync-official", nil)
	deniedRec := httptest.NewRecorder()
	handler.ServeHTTP(deniedRec, denied)
	if deniedRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated price sync 401, got %d", deniedRec.Code)
	}
}

func TestPlatformRoleManagementRoutesRequireStepUp(t *testing.T) {
	service := &fakeUserAdminService{}
	verifier := &fakeStepUpVerifier{}
	ownerID := "11111111-1111-4111-8111-111111111111"
	targetID := "22222222-2222-4222-8222-222222222222"
	roleID := "33333333-3333-4333-8333-333333333333"
	handler := NewWithRelayAndGroupsAndTokensAndConsoleTokensAndModelCatalogAndUsers(
		auth.NewMiddleware(testResolver{
			"owner": {ID: ownerID, Type: auth.PrincipalPlatformUser, Audience: auth.AudienceAdmin, Permissions: map[string]struct{}{
				"role:read": {}, "role:update": {},
			}},
		}),
		&auth.Services{StepUpMFA: verifier}, nil, false, "../../web", nil, nil, nil, nil, service,
	)

	read := httptest.NewRequest(http.MethodGet, "/admin/v1/roles", nil)
	read.Header.Set("Authorization", "Bearer owner")
	readRec := httptest.NewRecorder()
	handler.ServeHTTP(readRec, read)
	if readRec.Code != http.StatusOK {
		t.Fatalf("expected role list 200, got %d: %s", readRec.Code, readRec.Body.String())
	}

	missing := httptest.NewRequest(http.MethodPost, "/admin/v1/roles", strings.NewReader(`{"code":"ops_admin","name":"Operations Admin","permissions":["channel:read"]}`))
	missing.Header.Set("Authorization", "Bearer owner")
	missingRec := httptest.NewRecorder()
	handler.ServeHTTP(missingRec, missing)
	if missingRec.Code != http.StatusForbidden || !strings.Contains(missingRec.Body.String(), `"error":"STEP_UP_REQUIRED"`) {
		t.Fatalf("missing role step-up must be rejected: %d %s", missingRec.Code, missingRec.Body.String())
	}

	create := httptest.NewRequest(http.MethodPost, "/admin/v1/roles", strings.NewReader(`{"code":"ops_admin","name":"Operations Admin","permissions":["channel:read"]}`))
	create.Header.Set("Authorization", "Bearer owner")
	create.Header.Set("X-MFA-Code", "123456")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, create)
	if createRec.Code != http.StatusCreated || service.platformRole.Code != "ops_admin" || verifier.calls != 1 {
		t.Fatalf("expected role create 201, got %d: %s", createRec.Code, createRec.Body.String())
	}

	bind := httptest.NewRequest(http.MethodPut, "/admin/v1/users/"+targetID+"/roles", strings.NewReader(`{"role_ids":["`+roleID+`"]}`))
	bind.Header.Set("Authorization", "Bearer owner")
	bind.Header.Set("X-MFA-Code", "123456")
	bindRec := httptest.NewRecorder()
	handler.ServeHTTP(bindRec, bind)
	if bindRec.Code != http.StatusOK || service.boundUser != targetID || len(service.boundRoleIDs) != 1 {
		t.Fatalf("expected role binding 200, got %d: %s", bindRec.Code, bindRec.Body.String())
	}
}

func TestAdminReportingRoutesRequireDedicatedPermissions(t *testing.T) {
	service := &fakeBillingAdminService{}
	handler := NewWithRelayAndGroupsAndTokensAndConsoleTokensAndModelCatalogAndUsersAndPriceSync(
		auth.NewMiddleware(testResolver{
			"usage-reader": {
				ID:       "11111111-1111-4111-8111-111111111111",
				Type:     auth.PrincipalPlatformUser,
				Audience: auth.AudienceAdmin,
				Permissions: map[string]struct{}{
					"usage:read": {},
				},
			},
			"finance-reader": {
				ID:       "22222222-2222-4222-8222-222222222222",
				Type:     auth.PrincipalPlatformUser,
				Audience: auth.AudienceAdmin,
				Permissions: map[string]struct{}{
					"finance:read": {},
				},
			},
		}),
		nil, nil, false, "../../web", nil, nil, nil, nil, nil, nil, service)

	usage := httptest.NewRequest(http.MethodGet, "/admin/v1/usage", nil)
	usage.Header.Set("Authorization", "Bearer usage-reader")
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usage)
	if usageRec.Code != http.StatusOK || !strings.Contains(usageRec.Body.String(), `"total_records"`) {
		t.Fatalf("expected usage report 200, got %d: %s", usageRec.Code, usageRec.Body.String())
	}

	finance := httptest.NewRequest(http.MethodGet, "/admin/v1/finance", nil)
	finance.Header.Set("Authorization", "Bearer finance-reader")
	financeRec := httptest.NewRecorder()
	handler.ServeHTTP(financeRec, finance)
	if financeRec.Code != http.StatusOK || !strings.Contains(financeRec.Body.String(), `"summaries"`) {
		t.Fatalf("expected finance report 200, got %d: %s", financeRec.Code, financeRec.Body.String())
	}

	denied := httptest.NewRequest(http.MethodGet, "/admin/v1/finance", nil)
	denied.Header.Set("Authorization", "Bearer usage-reader")
	deniedRec := httptest.NewRecorder()
	handler.ServeHTTP(deniedRec, denied)
	if deniedRec.Code != http.StatusForbidden {
		t.Fatalf("expected finance permission denial, got %d", deniedRec.Code)
	}
}

func TestSensitiveAdminRoutesRequireStepUpMFA(t *testing.T) {
	verifier := &fakeStepUpVerifier{}
	handler := NewWithRelayAndGroupsAndTokensAndConsoleTokensAndModelCatalogAndUsersAndPriceSync(
		auth.NewMiddleware(testResolver{
			"admin-sensitive": {
				ID:       "11111111-1111-4111-8111-111111111111",
				Type:     auth.PrincipalPlatformUser,
				Audience: auth.AudienceAdmin,
				Permissions: map[string]struct{}{
					"price:publish": {},
				},
			},
		}),
		&auth.Services{StepUpMFA: verifier}, nil, false, "../../web", nil, nil, nil, nil, nil, nil,
		fakeBillingAdminService{},
	)

	missing := httptest.NewRequest(http.MethodPost, "/admin/v1/prices/publish", strings.NewReader(`{}`))
	missing.Header.Set("Authorization", "Bearer admin-sensitive")
	missingRec := httptest.NewRecorder()
	handler.ServeHTTP(missingRec, missing)
	if missingRec.Code != http.StatusForbidden || !strings.Contains(missingRec.Body.String(), `"error":"STEP_UP_REQUIRED"`) {
		t.Fatalf("missing step-up must be rejected: %d %s", missingRec.Code, missingRec.Body.String())
	}

	valid := httptest.NewRequest(http.MethodPost, "/admin/v1/prices/publish", strings.NewReader(`{"scope_type":"platform_default"}`))
	valid.Header.Set("Authorization", "Bearer admin-sensitive")
	valid.Header.Set("X-MFA-Code", "123456")
	validRec := httptest.NewRecorder()
	handler.ServeHTTP(validRec, valid)
	if validRec.Code != http.StatusCreated || verifier.calls != 1 {
		t.Fatalf("valid step-up must reach sensitive handler: %d %s calls=%d", validRec.Code, validRec.Body.String(), verifier.calls)
	}
}

func TestAdminStepUpUsesOperationPolicy(t *testing.T) {
	verifier := &fakeStepUpVerifier{}
	settings := &fakeSystemSettingsService{
		features: adminsettings.FeatureSettings{
			TOTPEnabled:          true,
			StepUpBillingEnabled: false,
		},
	}
	handler := NewWithRelayAndGroupsAndTokensAndConsoleTokensAndModelCatalogAndUsersAndPriceSync(
		auth.NewMiddleware(testResolver{
			"billing-admin": {
				ID:       "11111111-1111-4111-8111-111111111111",
				Type:     auth.PrincipalPlatformUser,
				Audience: auth.AudienceAdmin,
				Permissions: map[string]struct{}{
					"price:publish": {},
				},
			},
		}),
		&auth.Services{StepUpMFA: verifier, SecuritySettings: settings},
		nil,
		false,
		"../../web",
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		fakeBillingAdminService{},
	)

	withoutPolicy := httptest.NewRequest(http.MethodPost, "/admin/v1/prices/publish", strings.NewReader(`{"scope_type":"platform_default"}`))
	withoutPolicy.Header.Set("Authorization", "Bearer billing-admin")
	withoutPolicyRec := httptest.NewRecorder()
	handler.ServeHTTP(withoutPolicyRec, withoutPolicy)
	if withoutPolicyRec.Code != http.StatusCreated || verifier.calls != 0 {
		t.Fatalf("disabled billing policy must not require step-up, got %d: %s calls=%d", withoutPolicyRec.Code, withoutPolicyRec.Body.String(), verifier.calls)
	}

	settings.features.StepUpBillingEnabled = true
	missingCode := httptest.NewRequest(http.MethodPost, "/admin/v1/prices/publish", strings.NewReader(`{"scope_type":"platform_default"}`))
	missingCode.Header.Set("Authorization", "Bearer billing-admin")
	missingCodeRec := httptest.NewRecorder()
	handler.ServeHTTP(missingCodeRec, missingCode)
	if missingCodeRec.Code != http.StatusForbidden || !strings.Contains(missingCodeRec.Body.String(), `"error":"STEP_UP_REQUIRED"`) {
		t.Fatalf("enabled billing policy must require step-up, got %d: %s", missingCodeRec.Code, missingCodeRec.Body.String())
	}

	withCode := httptest.NewRequest(http.MethodPost, "/admin/v1/prices/publish", strings.NewReader(`{"scope_type":"platform_default"}`))
	withCode.Header.Set("Authorization", "Bearer billing-admin")
	withCode.Header.Set("X-MFA-Code", "123456")
	withCodeRec := httptest.NewRecorder()
	handler.ServeHTTP(withCodeRec, withCode)
	if withCodeRec.Code != http.StatusCreated || verifier.calls != 1 {
		t.Fatalf("enabled billing policy with a code must reach handler, got %d: %s calls=%d", withCodeRec.Code, withCodeRec.Body.String(), verifier.calls)
	}
}

func TestLoginRouteDoesNotAcceptUnknownFields(t *testing.T) {
	t.Setenv("ADMIN_ENTRY_PATH", "")
	handler := New(auth.NewMiddleware(testResolver{}), &auth.Services{Login: fakeLoginService{}}, false, "../../web")
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/auth/login", strings.NewReader(
		`{"email":"a@example.com","password":"password","unexpected":true}`,
	))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestRegistrationRouteCreatesTenantConsoleAccount(t *testing.T) {
	service := &fakeRegistrationService{}
	handler := NewWithRelayAndGroupsAndTokensAndConsoleTokensAndModelCatalogAndUsers(
		auth.NewMiddleware(testResolver{}),
		&auth.Services{Registration: service},
		nil,
		false,
		"../../web",
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	req := httptest.NewRequest(http.MethodPost, "/console/v1/auth/register", strings.NewReader(`{"email":"user@example.com","password":"a-very-long-password","display_name":"User","tenant_name":"Acme","tenant_slug":"acme","project_name":"Production"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated || service.request.Email != "user@example.com" || service.request.TenantSlug != "acme" {
		t.Fatalf("expected registration 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "password") {
		t.Fatalf("registration response must not contain password: %s", rec.Body.String())
	}
}

func TestRegistrationFeatureSwitchClosesEndpointAndPublicFlag(t *testing.T) {
	service := &fakeRegistrationService{}
	settings := &fakeSystemSettingsService{
		features: adminsettings.FeatureSettings{RegistrationEnabled: false},
	}
	handler := New(
		auth.NewMiddleware(testResolver{}),
		&auth.Services{Registration: service, SecuritySettings: settings},
		false,
		"../../web",
	)

	request := httptest.NewRequest(http.MethodPost, "/console/v1/auth/register", strings.NewReader(`{"email":"user@example.com","password":"a-very-long-password","display_name":"User","tenant_name":"Acme","tenant_slug":"acme","project_name":"Production"}`))
	record := httptest.NewRecorder()
	handler.ServeHTTP(record, request)
	if record.Code != http.StatusForbidden || !strings.Contains(record.Body.String(), "REGISTRATION_DISABLED") {
		t.Fatalf("expected registration feature to reject new accounts, got %d: %s", record.Code, record.Body.String())
	}
	if service.request.Email != "" {
		t.Fatalf("registration service must not be called while the feature is disabled: %#v", service.request)
	}

	publicRequest := httptest.NewRequest(http.MethodGet, "/public/v1/features", nil)
	publicRecord := httptest.NewRecorder()
	handler.ServeHTTP(publicRecord, publicRequest)
	if publicRecord.Code != http.StatusOK || !strings.Contains(publicRecord.Body.String(), `"registration_enabled":false`) {
		t.Fatalf("expected public registration flag false, got %d: %s", publicRecord.Code, publicRecord.Body.String())
	}
}

func TestLoginSetsHttpOnlySessionCookieWithoutReturningSecret(t *testing.T) {
	t.Setenv("ADMIN_ENTRY_PATH", "")
	handler := New(auth.NewMiddleware(testResolver{}), &auth.Services{Login: successfulLoginService{}}, false, "../../web")
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/auth/login", strings.NewReader(
		`{"email":"admin@example.com","password":"correct password"}`,
	))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	cookie := rec.Result().Cookies()
	if len(cookie) != 1 || cookie[0].Name != "admin_session" ||
		cookie[0].Value != "session-secret" || !cookie[0].HttpOnly {
		t.Fatalf("unexpected session cookie: %#v", cookie)
	}
	if strings.Contains(rec.Body.String(), "session-secret") {
		t.Fatal("session secret must not be returned in response body")
	}
}

func TestAdminLoginRequiresHiddenEntryTicket(t *testing.T) {
	t.Setenv("ADMIN_ENTRY_PATH", "/admin-0123456789abcdef")
	t.Setenv("SESSION_PEPPER", "session-pepper-with-at-least-thirty-two-characters")
	handler := New(auth.NewMiddleware(testResolver{}), &auth.Services{Login: fakeLoginService{}}, false, "../../web")

	direct := httptest.NewRequest(http.MethodPost, "/admin/v1/auth/login", strings.NewReader(
		`{"email":"admin@example.com","password":"correct password"}`,
	))
	directRec := httptest.NewRecorder()
	handler.ServeHTTP(directRec, direct)
	if directRec.Code != http.StatusNotFound || !strings.Contains(directRec.Body.String(), "ADMIN_ENTRY_REQUIRED") {
		t.Fatalf("direct admin login must be hidden, got %d: %s", directRec.Code, directRec.Body.String())
	}

	entry := httptest.NewRequest(http.MethodGet, "/admin-0123456789abcdef", nil)
	entryRec := httptest.NewRecorder()
	handler.ServeHTTP(entryRec, entry)
	if entryRec.Code != http.StatusOK {
		t.Fatalf("expected hidden entry to serve the frontend, got %d: %s", entryRec.Code, entryRec.Body.String())
	}
	cookies := entryRec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != adminEntryCookieName || !cookies[0].HttpOnly {
		t.Fatalf("expected short-lived HttpOnly entry cookie, got %#v", cookies)
	}

	allowed := httptest.NewRequest(http.MethodPost, "/admin/v1/auth/login", strings.NewReader(
		`{"email":"admin@example.com","password":"correct password"}`,
	))
	allowed.AddCookie(cookies[0])
	allowedRec := httptest.NewRecorder()
	handler.ServeHTTP(allowedRec, allowed)
	if allowedRec.Code != http.StatusUnauthorized {
		t.Fatalf("entry ticket should reach the normal login handler, got %d: %s", allowedRec.Code, allowedRec.Body.String())
	}
}

func TestPasswordResetRequiresNotifier(t *testing.T) {
	handler := New(auth.NewMiddleware(testResolver{}), &auth.Services{
		PasswordReset: fakePasswordResetService{},
	}, false, "../../web")
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/auth/password-reset/request", strings.NewReader(
		`{"email":"user@example.com"}`,
	))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "reset-secret") {
		t.Fatal("reset secret must not be exposed")
	}
}

func TestPasswordResetDoesNotRevealNotifierFailureForKnownAccount(t *testing.T) {
	handler := New(auth.NewMiddleware(testResolver{}), &auth.Services{
		PasswordReset:         fakePasswordResetService{},
		PasswordResetNotifier: failingPasswordResetNotifier{},
	}, false, "../../web")
	req := httptest.NewRequest(http.MethodPost, "/console/v1/auth/password-reset/request", strings.NewReader(
		`{"email":"user@example.com"}`,
	))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted || strings.Contains(rec.Body.String(), "reset-secret") {
		t.Fatalf("notifier failure must still return indistinguishable accepted response: %d %s", rec.Code, rec.Body.String())
	}
}

func TestRelayRequiresTokenScope(t *testing.T) {
	handler := New(auth.NewMiddleware(testResolver{
		"relay-ok": {
			ID:       "token-1",
			Type:     auth.PrincipalAPIToken,
			Audience: auth.AudienceRelay,
			Scopes:   map[string]struct{}{"model:use": {}},
			TenantID: "tenant-1",
		},
		"relay-no-scope": {
			ID:       "token-2",
			Type:     auth.PrincipalAPIToken,
			Audience: auth.AudienceRelay,
			Scopes:   map[string]struct{}{},
			TenantID: "tenant-1",
		},
	}), nil, false, "../../web")

	okReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	okReq.Header.Set("Authorization", "Bearer relay-ok")
	okRec := httptest.NewRecorder()
	handler.ServeHTTP(okRec, okReq)
	if okRec.Code != http.StatusNotImplemented {
		t.Fatalf("expected scoped token to reach relay, got %d", okRec.Code)
	}

	deniedReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	deniedReq.Header.Set("Authorization", "Bearer relay-no-scope")
	deniedRec := httptest.NewRecorder()
	handler.ServeHTTP(deniedRec, deniedReq)
	if deniedRec.Code != http.StatusForbidden {
		t.Fatalf("expected missing scope 403, got %d", deniedRec.Code)
	}
}

func TestRelayRouteAliasesRequireTheSameTokenScope(t *testing.T) {
	handler := New(auth.NewMiddleware(testResolver{
		"relay-no-scope": {
			ID:       "token-2",
			Type:     auth.PrincipalAPIToken,
			Audience: auth.AudienceRelay,
			Scopes:   map[string]struct{}{},
			TenantID: "tenant-1",
		},
	}), nil, false, "../../web")

	tests := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/v1/chat/completions"},
		{http.MethodPost, "/chat/completions"},
		{http.MethodPost, "/v1/v1/chat/completions"},
		{http.MethodPost, "/v1/embeddings"},
		{http.MethodPost, "/embeddings"},
		{http.MethodGet, "/v1/models"},
		{http.MethodGet, "/models"},
		{http.MethodPost, "/v1/responses"},
		{http.MethodPost, "/responses"},
		{http.MethodPost, "/v1/messages"},
		{http.MethodPost, "/messages"},
		{http.MethodPost, "/v1/v1/messages"},
		{http.MethodPost, "/v1/images/generations"},
		{http.MethodPost, "/images/generations"},
		{http.MethodPost, "/v1/audio/transcriptions"},
		{http.MethodPost, "/audio/transcriptions"},
		{http.MethodPost, "/v1/videos"},
		{http.MethodPost, "/videos"},
	}
	for _, test := range tests {
		t.Run(test.method+"_"+strings.ReplaceAll(test.path, "/", "_"), func(t *testing.T) {
			req := httptest.NewRequest(test.method, test.path, nil)
			req.Header.Set("Authorization", "Bearer relay-no-scope")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("alias %s %s must enforce model:use, got %d: %s", test.method, test.path, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestRelayChatRouteAliasesInvokeTheSameService(t *testing.T) {
	for _, path := range []string{"/v1/chat/completions", "/chat/completions", "/v1/v1/chat/completions"} {
		t.Run(strings.ReplaceAll(path, "/", "_"), func(t *testing.T) {
			service := &fakeRelayService{}
			handler := NewWithRelay(auth.NewMiddleware(testResolver{
				"relay-ok": {
					ID:       "token-1",
					Type:     auth.PrincipalAPIToken,
					Audience: auth.AudienceRelay,
					Scopes:   map[string]struct{}{"model:use": {}},
					TenantID: "tenant-1",
				},
			}), &auth.Services{StepUpMFA: &fakeStepUpVerifier{}}, service, false, "../../web")
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(
				`{"model":"gpt-5","messages":[{"role":"user","content":"hello"}]}`,
			))
			req.Header.Set("Authorization", "Bearer relay-ok")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK || service.model != "gpt-5" {
				t.Fatalf("alias %s must invoke relay, got %d: %s", path, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestRelayInvokesConfiguredService(t *testing.T) {
	service := &fakeRelayService{}
	handler := NewWithRelay(auth.NewMiddleware(testResolver{
		"relay-ok": {
			ID:       "token-1",
			Type:     auth.PrincipalAPIToken,
			Audience: auth.AudienceRelay,
			Scopes:   map[string]struct{}{"model:use": {}},
			TenantID: "tenant-1",
		},
	}), &auth.Services{StepUpMFA: &fakeStepUpVerifier{}}, service, false, "../../web")

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(
		`{"model":"gpt-5","messages":[{"role":"user","content":"hello"}]}`,
	))
	req.Header.Set("Authorization", "Bearer relay-ok")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected relay 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if service.model != "gpt-5" || service.principalID != "token-1" {
		t.Fatalf("service did not receive expected request: %#v", service)
	}
	if !strings.Contains(rec.Body.String(), `"content":"pong"`) {
		t.Fatalf("unexpected response body: %s", rec.Body.String())
	}
}

func TestAnthropicMessagesAcceptsClaudeContentBlockRequests(t *testing.T) {
	service := &fakeRelayService{}
	handler := NewWithRelay(auth.NewMiddleware(testResolver{
		"relay-ok": {
			ID:       "token-1",
			Type:     auth.PrincipalAPIToken,
			Audience: auth.AudienceRelay,
			Scopes:   map[string]struct{}{"model:use": {}},
			TenantID: "tenant-1",
		},
	}), nil, service, false, "../../web")

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
		"model":"claude-sonnet-5",
		"system":[{"type":"text","text":"Be concise.","cache_control":{"type":"ephemeral"}}],
		"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}],
		"max_tokens":256,
		"stop_sequences":["END"],
		"metadata":{"user_id":"user-1"},
		"thinking":{"type":"enabled","budget_tokens":128}
	}`))
	req.Header.Set("Authorization", "Bearer relay-ok")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Claude-style Anthropic request should be accepted, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"stop_reason":"end_turn"`) {
		t.Fatalf("non-stream Anthropic response must use end_turn, got: %s", rec.Body.String())
	}
	if len(service.request.Messages) != 2 || service.request.Messages[0].Role != "system" || service.request.Messages[1].Role != "user" {
		t.Fatalf("unexpected normalized Anthropic messages: %#v", service.request.Messages)
	}
	if len(service.request.Messages[0].ContentParts) == 0 || len(service.request.Messages[1].ContentParts) == 0 {
		t.Fatalf("Anthropic content block arrays must be preserved: %#v", service.request.Messages)
	}
	if len(service.request.Stop) != 1 || service.request.Stop[0] != "END" {
		t.Fatalf("stop_sequences were not preserved: %#v", service.request.Stop)
	}
	if string(service.request.Metadata) != `{"user_id":"user-1"}` || string(service.request.Thinking) != `{"type":"enabled","budget_tokens":128}` {
		t.Fatalf("Anthropic metadata/thinking were not preserved: metadata=%s thinking=%s", service.request.Metadata, service.request.Thinking)
	}
}

func TestAnthropicMessagesStreamingAcceptsClaudeAppPayload(t *testing.T) {
	service := &fakeStreamingRelayService{}
	handler := NewWithRelay(auth.NewMiddleware(testResolver{
		"relay-ok": {
			ID:       "token-1",
			Type:     auth.PrincipalAPIToken,
			Audience: auth.AudienceRelay,
			Scopes:   map[string]struct{}{"model:use": {}},
			TenantID: "tenant-1",
		},
	}), nil, service, false, "../../web")

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
		"model":"claude-sonnet-5",
		"system":[{"type":"text","text":"Be concise."}],
		"messages":[
			{"role":"user","content":[{"type":"text","text":"hello"}]},
			{"role":"assistant","content":[{"type":"text","text":"Sure."}]},
			{"role":"user","content":[{"type":"text","text":"continue"}]}
		],
		"max_tokens":256,
		"stream":true
	}`))
	req.Header.Set("Authorization", "Bearer relay-ok")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Claude App streaming request should be accepted, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: message_start") || !strings.Contains(body, "event: content_block_delta") || !strings.Contains(body, "event: message_stop") {
		t.Fatalf("unexpected Anthropic SSE response: %s", body)
	}
	if strings.Count(body, "event: message_delta") != 1 {
		t.Fatalf("Anthropic SSE response must contain one message_delta, got %d: %s", strings.Count(body, "event: message_delta"), body)
	}
	if !strings.Contains(body, `"stop_reason":"end_turn"`) {
		t.Fatalf("Anthropic SSE response must use end_turn, got: %s", body)
	}
	if service.request.Model != "claude-sonnet-5" || len(service.request.Messages) != 4 {
		t.Fatalf("stream service received unexpected normalized request: %#v", service.request)
	}
}

func TestAnthropicWireStopReasonUsesAnthropicValues(t *testing.T) {
	tests := map[string]string{
		"":                              "end_turn",
		"stop":                          "end_turn",
		"end_turn":                      "end_turn",
		"stop_sequence":                 "stop_sequence",
		"length":                        "max_tokens",
		"tool_calls":                    "tool_use",
		"function_call":                 "tool_use",
		"content_filter":                "refusal",
		"max_tokens":                    "max_tokens",
		"tool_use":                      "tool_use",
		"pause_turn":                    "pause_turn",
		"model_context_window_exceeded": "model_context_window_exceeded",
		"custom_future":                 "end_turn",
	}
	for input, expected := range tests {
		if actual := anthropicWireStopReason(input); actual != expected {
			t.Errorf("anthropicWireStopReason(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestRelayEnforcesTokenNetworkAllowlistBeforeUpstream(t *testing.T) {
	service := &fakeRelayService{}
	handler := NewWithRelay(auth.NewMiddleware(testResolver{
		"ip-allowed": {
			ID:       "token-ip",
			Type:     auth.PrincipalAPIToken,
			Audience: auth.AudienceRelay,
			Scopes:   map[string]struct{}{"model:use": {}},
			AllowedIPs: map[string]struct{}{
				"203.0.113.0/24": {},
			},
		},
		"domain-allowed": {
			ID:       "token-domain",
			Type:     auth.PrincipalAPIToken,
			Audience: auth.AudienceRelay,
			Scopes:   map[string]struct{}{"model:use": {}},
			AllowedIPs: map[string]struct{}{
				"198.51.100.0/24": {},
			},
			AllowedDomains: map[string]struct{}{
				"*.example.com": {},
			},
		},
	}), nil, service, false, "../../web")

	body := `{"model":"gpt-5","messages":[{"role":"user","content":"hello"}]}`
	allowedIP := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	allowedIP.RemoteAddr = "203.0.113.22:1234"
	allowedIP.Header.Set("Authorization", "Bearer ip-allowed")
	allowedIPRec := httptest.NewRecorder()
	handler.ServeHTTP(allowedIPRec, allowedIP)
	if allowedIPRec.Code != http.StatusOK {
		t.Fatalf("expected allowed IP request 200, got %d: %s", allowedIPRec.Code, allowedIPRec.Body.String())
	}

	deniedIP := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	deniedIP.RemoteAddr = "198.51.100.22:1234"
	deniedIP.Header.Set("Authorization", "Bearer ip-allowed")
	deniedIPRec := httptest.NewRecorder()
	handler.ServeHTTP(deniedIPRec, deniedIP)
	if deniedIPRec.Code != http.StatusForbidden || !strings.Contains(deniedIPRec.Body.String(), "TOKEN_NETWORK_NOT_ALLOWED") {
		t.Fatalf("expected denied IP request 403, got %d: %s", deniedIPRec.Code, deniedIPRec.Body.String())
	}

	allowedDomain := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	allowedDomain.RemoteAddr = "198.51.100.22:1234"
	allowedDomain.Header.Set("Origin", "https://app.example.com")
	allowedDomain.Header.Set("Authorization", "Bearer domain-allowed")
	allowedDomainRec := httptest.NewRecorder()
	handler.ServeHTTP(allowedDomainRec, allowedDomain)
	if allowedDomainRec.Code != http.StatusOK {
		t.Fatalf("expected allowed domain request 200, got %d: %s", allowedDomainRec.Code, allowedDomainRec.Body.String())
	}
}

func TestAdminChannelsListsSafeCredentialRefs(t *testing.T) {
	service := &fakeRelayService{
		channels: []relay.ChannelSummary{
			{
				ID:            "channel-1",
				Name:          "OpenAI Official",
				Provider:      "openai",
				BaseURL:       "https://api.openai.com/v1",
				CredentialRef: "env:OPENAI_API_KEY",
				Status:        "active",
				Priority:      100,
				Weight:        100,
				Models: []relay.ChannelModelSummary{
					{Model: "gpt-5", Provider: "openai", UpstreamModel: "gpt-5", Enabled: true, HealthStatus: "unknown"},
				},
			},
		},
	}
	handler := NewWithRelay(auth.NewMiddleware(testResolver{
		"admin-ok": {
			ID:       "user-1",
			Type:     auth.PrincipalPlatformUser,
			Audience: auth.AudienceAdmin,
			Permissions: map[string]struct{}{
				"channel:read": {},
			},
		},
		"admin-no-channel": {
			ID:          "user-2",
			Type:        auth.PrincipalPlatformUser,
			Audience:    auth.AudienceAdmin,
			Permissions: map[string]struct{}{},
		},
	}), nil, service, false, "../../web")

	req := httptest.NewRequest(http.MethodGet, "/admin/v1/channels", nil)
	req.Header.Set("Authorization", "Bearer admin-ok")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected channels 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"credential_ref":"env:OPENAI_API_KEY"`) ||
		strings.Contains(rec.Body.String(), "sk-live") {
		t.Fatalf("unexpected channels body: %s", rec.Body.String())
	}

	deniedReq := httptest.NewRequest(http.MethodGet, "/admin/v1/channels", nil)
	deniedReq.Header.Set("Authorization", "Bearer admin-no-channel")
	deniedRec := httptest.NewRecorder()
	handler.ServeHTTP(deniedRec, deniedReq)
	if deniedRec.Code != http.StatusForbidden {
		t.Fatalf("expected missing permission 403, got %d", deniedRec.Code)
	}
}

func TestAdminBillingRoutesEnforceBillingPermissions(t *testing.T) {
	service := &fakeBillingAdminService{}
	handler := NewWithRelay(auth.NewMiddleware(testResolver{
		"billing-read": {
			ID:          "finance-1",
			Type:        auth.PrincipalPlatformUser,
			Audience:    auth.AudienceAdmin,
			Permissions: map[string]struct{}{"price:read": {}},
		},
		"billing-denied": {
			ID:          "ops-1",
			Type:        auth.PrincipalPlatformUser,
			Audience:    auth.AudienceAdmin,
			Permissions: map[string]struct{}{},
		},
	}), &auth.Services{StepUpMFA: &fakeStepUpVerifier{}}, nil, false, "../../web", service)

	okRequest := httptest.NewRequest(http.MethodGet, "/admin/v1/prices", nil)
	okRequest.Header.Set("Authorization", "Bearer billing-read")
	okResponse := httptest.NewRecorder()
	handler.ServeHTTP(okResponse, okRequest)
	if okResponse.Code != http.StatusOK || !strings.Contains(okResponse.Body.String(), `"input_price_per_million_tokens":"1.25"`) {
		t.Fatalf("expected billing price list, got %d: %s", okResponse.Code, okResponse.Body.String())
	}

	deniedRequest := httptest.NewRequest(http.MethodGet, "/admin/v1/prices", nil)
	deniedRequest.Header.Set("Authorization", "Bearer billing-denied")
	deniedResponse := httptest.NewRecorder()
	handler.ServeHTTP(deniedResponse, deniedRequest)
	if deniedResponse.Code != http.StatusForbidden {
		t.Fatalf("expected billing permission denial, got %d", deniedResponse.Code)
	}
}

func TestAdminChannelMutationRoutesRequireUpdatePermission(t *testing.T) {
	service := &fakeRelayService{}
	handler := NewWithRelay(auth.NewMiddleware(testResolver{
		"admin-update": {
			ID:       "user-1",
			Type:     auth.PrincipalPlatformUser,
			Audience: auth.AudienceAdmin,
			Permissions: map[string]struct{}{
				"channel:update": {},
			},
		},
		"admin-readonly": {
			ID:       "user-2",
			Type:     auth.PrincipalPlatformUser,
			Audience: auth.AudienceAdmin,
			Permissions: map[string]struct{}{
				"channel:read": {},
			},
		},
	}), &auth.Services{StepUpMFA: &fakeStepUpVerifier{}}, service, false, "../../web")

	body := `{
		"name":"OpenAI Production",
		"provider":"openai",
		"base_url":"https://api.openai.com/v1",
		"api_key":"sk-live-secret",
		"status":"active",
		"priority":100,
		"weight":100,
		"models":[{"model":"gpt-5","upstream_model":"gpt-5","enabled":true}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/channels", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer admin-update")
	req.Header.Set("X-MFA-Code", "123456")
	req.Header.Set("X-MFA-Code", "123456")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected create 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if service.createdBy != "user-1" || service.created.APIKey != "sk-live-secret" {
		t.Fatalf("service did not receive expected create payload: %#v", service)
	}
	if strings.Contains(rec.Body.String(), "sk-live-secret") {
		t.Fatal("channel secret must not be returned in response")
	}

	deniedReq := httptest.NewRequest(http.MethodPost, "/admin/v1/channels", strings.NewReader(body))
	deniedReq.Header.Set("Authorization", "Bearer admin-readonly")
	deniedRec := httptest.NewRecorder()
	handler.ServeHTTP(deniedRec, deniedReq)
	if deniedRec.Code != http.StatusForbidden {
		t.Fatalf("expected readonly mutation 403, got %d", deniedRec.Code)
	}
}

func TestAdminChannelStatusAndDeleteRoutes(t *testing.T) {
	service := &fakeRelayService{}
	handler := NewWithRelay(auth.NewMiddleware(testResolver{
		"admin-update": {
			ID:       "user-1",
			Type:     auth.PrincipalPlatformUser,
			Audience: auth.AudienceAdmin,
			Permissions: map[string]struct{}{
				"channel:update": {},
			},
		},
	}), &auth.Services{StepUpMFA: &fakeStepUpVerifier{}}, service, false, "../../web")

	for _, route := range []struct {
		method string
		path   string
		status string
	}{
		{http.MethodPost, "/admin/v1/channels/channel-1/pause", "disabled"},
		{http.MethodPost, "/admin/v1/channels/channel-1/enable", "active"},
	} {
		req := httptest.NewRequest(route.method, route.path, nil)
		req.Header.Set("Authorization", "Bearer admin-update")
		req.Header.Set("X-MFA-Code", "123456")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected status route 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if service.statusChannelID != "channel-1" || service.status != route.status {
			t.Fatalf("unexpected status update: %#v", service)
		}
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/admin/v1/channels/channel-1", nil)
	deleteReq.Header.Set("Authorization", "Bearer admin-update")
	deleteReq.Header.Set("X-MFA-Code", "123456")
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected delete 200, got %d: %s", deleteRec.Code, deleteRec.Body.String())
	}
	if service.deletedChannelID != "channel-1" || service.deletedBy != "user-1" {
		t.Fatalf("unexpected delete call: %#v", service)
	}
}

func TestAdminChannelModelDiscoveryRequiresUpdatePermission(t *testing.T) {
	service := &fakeRelayService{
		discoveredModels: []relay.DiscoveredModel{
			{ID: "gpt-5", DisplayName: "gpt-5", Provider: "openai"},
		},
	}
	handler := NewWithRelay(auth.NewMiddleware(testResolver{
		"admin-update": {
			ID:       "user-1",
			Type:     auth.PrincipalPlatformUser,
			Audience: auth.AudienceAdmin,
			Permissions: map[string]struct{}{
				"channel:update": {},
			},
		},
		"admin-readonly": {
			ID:       "user-2",
			Type:     auth.PrincipalPlatformUser,
			Audience: auth.AudienceAdmin,
			Permissions: map[string]struct{}{
				"channel:read": {},
			},
		},
	}), &auth.Services{StepUpMFA: &fakeStepUpVerifier{}}, service, false, "../../web")

	body := `{"channel_id":"channel-1","provider":"openai","base_url":"https://api.openai.com/v1","api_key":"sk-discovery-secret"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/channels/discover-models", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer admin-update")
	req.Header.Set("X-MFA-Code", "123456")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected discovery 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if service.discovered.ChannelID != "channel-1" || service.discovered.Provider != "openai" || service.discovered.APIKey != "sk-discovery-secret" {
		t.Fatalf("service did not receive discovery payload: %#v", service.discovered)
	}
	if strings.Contains(rec.Body.String(), "sk-discovery-secret") {
		t.Fatal("discovery response must not contain api key")
	}

	deniedReq := httptest.NewRequest(http.MethodPost, "/admin/v1/channels/discover-models", strings.NewReader(body))
	deniedReq.Header.Set("Authorization", "Bearer admin-readonly")
	deniedRec := httptest.NewRecorder()
	handler.ServeHTTP(deniedRec, deniedReq)
	if deniedRec.Code != http.StatusForbidden {
		t.Fatalf("expected readonly discovery 403, got %d", deniedRec.Code)
	}
}

func TestAdminGroupRoutesEnforceGroupPermissions(t *testing.T) {
	service := &fakeGroupService{}
	handler := NewWithRelayAndGroups(auth.NewMiddleware(testResolver{
		"group-read": {
			ID:       "user-1",
			Type:     auth.PrincipalPlatformUser,
			Audience: auth.AudienceAdmin,
			Permissions: map[string]struct{}{
				"group:read": {},
			},
		},
		"group-update": {
			ID:       "user-2",
			Type:     auth.PrincipalPlatformUser,
			Audience: auth.AudienceAdmin,
			Permissions: map[string]struct{}{
				"group:update": {},
			},
		},
	}), &auth.Services{StepUpMFA: &fakeStepUpVerifier{}}, nil, false, "../../web", service)

	readReq := httptest.NewRequest(http.MethodGet, "/admin/v1/groups", nil)
	readReq.Header.Set("Authorization", "Bearer group-read")
	readRec := httptest.NewRecorder()
	handler.ServeHTTP(readRec, readReq)
	if readRec.Code != http.StatusOK {
		t.Fatalf("expected group read 200, got %d: %s", readRec.Code, readRec.Body.String())
	}

	createReq := httptest.NewRequest(http.MethodPost, "/admin/v1/groups", strings.NewReader(`{"code":"standard","name":"Standard","multiplier":"1","rpm_limit":0,"billing_type":"prepaid","priority":100,"channel_ids":[]}`))
	createReq.Header.Set("Authorization", "Bearer group-update")
	createReq.Header.Set("X-MFA-Code", "123456")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated || service.createdBy != "user-2" {
		t.Fatalf("expected group create 201, got %d: %s", createRec.Code, createRec.Body.String())
	}

	deniedReq := httptest.NewRequest(http.MethodPost, "/admin/v1/groups", strings.NewReader(`{"code":"blocked","name":"Blocked","multiplier":"1"}`))
	deniedReq.Header.Set("Authorization", "Bearer group-read")
	deniedRec := httptest.NewRecorder()
	handler.ServeHTTP(deniedRec, deniedReq)
	if deniedRec.Code != http.StatusForbidden {
		t.Fatalf("expected group write 403, got %d", deniedRec.Code)
	}
}

func TestAdminTokenGroupRoutesEnforceTokenPermissions(t *testing.T) {
	service := &fakeTokenAdminService{}
	handler := NewWithRelayAndGroupsAndTokens(auth.NewMiddleware(testResolver{
		"token-read": {
			ID:       "user-1",
			Type:     auth.PrincipalPlatformUser,
			Audience: auth.AudienceAdmin,
			Permissions: map[string]struct{}{
				"token:read": {},
			},
		},
		"token-update": {
			ID:       "user-2",
			Type:     auth.PrincipalPlatformUser,
			Audience: auth.AudienceAdmin,
			Permissions: map[string]struct{}{
				"token:update": {},
			},
		},
		"token-create": {
			ID:       "user-3",
			Type:     auth.PrincipalPlatformUser,
			Audience: auth.AudienceAdmin,
			Permissions: map[string]struct{}{
				"token:create": {},
			},
		},
	}), &auth.Services{StepUpMFA: &fakeStepUpVerifier{}}, nil, false, "../../web", service, nil)

	readReq := httptest.NewRequest(http.MethodGet, "/admin/v1/tokens", nil)
	readReq.Header.Set("Authorization", "Bearer token-read")
	readRec := httptest.NewRecorder()
	handler.ServeHTTP(readRec, readReq)
	if readRec.Code != http.StatusOK {
		t.Fatalf("expected token read 200, got %d: %s", readRec.Code, readRec.Body.String())
	}

	updateReq := httptest.NewRequest(http.MethodPut, "/admin/v1/tokens/token-1/group", strings.NewReader(`{"group_id":"group-1"}`))
	updateReq.Header.Set("Authorization", "Bearer token-update")
	updateReq.Header.Set("X-MFA-Code", "123456")
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK || service.updatedBy != "group-1" {
		t.Fatalf("expected token group update 200, got %d: %s", updateRec.Code, updateRec.Body.String())
	}

	deniedReq := httptest.NewRequest(http.MethodPut, "/admin/v1/tokens/token-1/group", strings.NewReader(`{"group_id":"group-1"}`))
	deniedReq.Header.Set("Authorization", "Bearer token-read")
	deniedRec := httptest.NewRecorder()
	handler.ServeHTTP(deniedRec, deniedReq)
	if deniedRec.Code != http.StatusForbidden {
		t.Fatalf("expected token group write 403, got %d", deniedRec.Code)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/admin/v1/tokens", strings.NewReader(`{"tenant_id":"tenant-1","project_id":"project-1","name":"admin-token","group_id":"group-1"}`))
	createReq.Header.Set("Authorization", "Bearer token-create")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusForbidden || !strings.Contains(createRec.Body.String(), `"error":"ADMIN_TOKEN_CREATION_DISABLED"`) {
		t.Fatalf("expected admin token creation to be disabled, got %d: %s", createRec.Code, createRec.Body.String())
	}
}

func TestAdminUserRoutesEnforceUserPermissions(t *testing.T) {
	service := &fakeUserAdminService{}
	handler := NewWithRelayAndGroupsAndTokensAndConsoleTokensAndModelCatalogAndUsers(auth.NewMiddleware(testResolver{
		"user-read": {
			ID:       "11111111-1111-4111-8111-111111111111",
			Type:     auth.PrincipalPlatformUser,
			Audience: auth.AudienceAdmin,
			Permissions: map[string]struct{}{
				"user:read": {},
			},
		},
		"user-update": {
			ID:       "11111111-1111-4111-8111-111111111111",
			Type:     auth.PrincipalPlatformUser,
			Audience: auth.AudienceAdmin,
			Permissions: map[string]struct{}{
				"user:update": {},
			},
		},
		"user-create": {
			ID:       "11111111-1111-4111-8111-111111111111",
			Type:     auth.PrincipalPlatformUser,
			Audience: auth.AudienceAdmin,
			Permissions: map[string]struct{}{
				"user:create": {},
			},
		},
	}), &auth.Services{StepUpMFA: &fakeStepUpVerifier{}}, nil, false, "../../web", nil, nil, nil, nil, service)

	readReq := httptest.NewRequest(http.MethodGet, "/admin/v1/users", nil)
	readReq.Header.Set("Authorization", "Bearer user-read")
	readRec := httptest.NewRecorder()
	handler.ServeHTTP(readRec, readReq)
	if readRec.Code != http.StatusOK {
		t.Fatalf("expected user read 200, got %d: %s", readRec.Code, readRec.Body.String())
	}

	tenantReq := httptest.NewRequest(http.MethodGet, "/admin/v1/tenants", nil)
	tenantReq.Header.Set("Authorization", "Bearer user-read")
	tenantRec := httptest.NewRecorder()
	handler.ServeHTTP(tenantRec, tenantReq)
	if tenantRec.Code != http.StatusOK || !strings.Contains(tenantRec.Body.String(), `"slug":"acme"`) {
		t.Fatalf("expected tenant options 200, got %d: %s", tenantRec.Code, tenantRec.Body.String())
	}

	createReq := httptest.NewRequest(http.MethodPost, "/admin/v1/users", strings.NewReader(`{"email":"new@example.com","display_name":"New User","password":"a-very-long-password","tenant_id":"33333333-3333-4333-8333-333333333333","tenant_role":"developer"}`))
	createReq.Header.Set("Authorization", "Bearer user-create")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusForbidden || !strings.Contains(createRec.Body.String(), `"error":"ADMIN_USER_CREATION_DISABLED"`) {
		t.Fatalf("expected admin user creation to be disabled, got %d: %s", createRec.Code, createRec.Body.String())
	}

	updateReq := httptest.NewRequest(http.MethodPut, "/admin/v1/users/22222222-2222-4222-8222-222222222222/status", strings.NewReader(`{"status":"disabled"}`))
	updateReq.Header.Set("Authorization", "Bearer user-update")
	updateReq.Header.Set("X-MFA-Code", "123456")
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK || service.status != "disabled" {
		t.Fatalf("expected user status update 200, got %d: %s", updateRec.Code, updateRec.Body.String())
	}

	editReq := httptest.NewRequest(http.MethodPut, "/admin/v1/users/22222222-2222-4222-8222-222222222222", strings.NewReader(`{"email":"edited@example.com","display_name":"Edited User","password":"another-long-password"}`))
	editReq.Header.Set("Authorization", "Bearer user-update")
	editReq.Header.Set("X-MFA-Code", "123456")
	editRec := httptest.NewRecorder()
	handler.ServeHTTP(editRec, editReq)
	if editRec.Code != http.StatusOK || service.updatedID != "22222222-2222-4222-8222-222222222222" || service.updated.Email != "edited@example.com" {
		t.Fatalf("expected user edit 200, got %d: %s", editRec.Code, editRec.Body.String())
	}

	deniedReq := httptest.NewRequest(http.MethodGet, "/admin/v1/users", nil)
	deniedReq.Header.Set("Authorization", "Bearer user-update")
	deniedRec := httptest.NewRecorder()
	handler.ServeHTTP(deniedRec, deniedReq)
	if deniedRec.Code != http.StatusForbidden {
		t.Fatalf("expected user read permission denied, got %d", deniedRec.Code)
	}
}

func TestConsoleTokenRoutesAreTenantAndOwnerScoped(t *testing.T) {
	service := &fakeTokenConsoleService{}
	groupService := &fakeGroupService{}
	handler := NewWithRelayAndGroupsAndTokensAndConsoleTokens(auth.NewMiddleware(testResolver{
		"console-user": {
			ID:       "user-1",
			Type:     auth.PrincipalTenantUser,
			Audience: auth.AudienceConsole,
			TenantID: "tenant-1",
			ProjectIDs: map[string]struct{}{
				"project-1": {},
			},
			Permissions: map[string]struct{}{
				"token:read":   {},
				"token:create": {},
				"token:update": {},
				"token:revoke": {},
			},
		},
		"console-editor": {
			ID:       "user-2",
			Type:     auth.PrincipalTenantUser,
			Audience: auth.AudienceConsole,
			TenantID: "tenant-1",
			ProjectIDs: map[string]struct{}{
				"project-1": {},
			},
			Permissions: map[string]struct{}{
				"token:read":   {},
				"token:update": {},
			},
		},
		"admin-user": {
			ID:       "admin-1",
			Type:     auth.PrincipalPlatformUser,
			Audience: auth.AudienceAdmin,
			Permissions: map[string]struct{}{
				"token:read": {},
			},
		},
	}), nil, nil, false, "../../web", nil, groupService, service)

	groupReq := httptest.NewRequest(http.MethodGet, "/console/v1/tenants/tenant-1/token-groups", nil)
	groupReq.Header.Set("Authorization", "Bearer console-user")
	groupRec := httptest.NewRecorder()
	handler.ServeHTTP(groupRec, groupReq)
	if groupRec.Code != http.StatusOK || !strings.Contains(groupRec.Body.String(), `"code":"standard"`) {
		t.Fatalf("expected token group options 200, got %d: %s", groupRec.Code, groupRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/console/v1/tenants/tenant-1/tokens", nil)
	listReq.Header.Set("Authorization", "Bearer console-user")
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK || service.listTenant != "tenant-1" || service.listOwner != "user-1" {
		t.Fatalf("expected owner-scoped token list, got %d: %s", listRec.Code, listRec.Body.String())
	}

	createReq := httptest.NewRequest(http.MethodPost, "/console/v1/tenants/tenant-1/tokens", strings.NewReader(`{"project_id":"project-1","name":"local-dev","group_id":"group-1","spend_limit":"500","allowed_ips":["203.0.113.10"],"allowed_domains":["app.example.com"]}`))
	createReq.Header.Set("Authorization", "Bearer console-user")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated || service.createdBy != "user-1" || service.createdProject != "project-1" || service.createdGroup != "group-1" || service.createdSpendLimit != "500" || len(service.createdIPs) != 1 || service.createdIPs[0] != "203.0.113.10" || len(service.createdDomains) != 1 || service.createdDomains[0] != "app.example.com" {
		t.Fatalf("expected console token create 201, got %d: %s", createRec.Code, createRec.Body.String())
	}
	if !strings.Contains(createRec.Body.String(), `"token":"sk-test-once"`) {
		t.Fatalf("expected one-time token in create response: %s", createRec.Body.String())
	}

	updateReq := httptest.NewRequest(http.MethodPut, "/console/v1/tenants/tenant-1/tokens/token-1", strings.NewReader(`{"project_id":"project-1","name":"edited-token","group_id":"group-2","spend_limit":"250.5","allowed_ips":["198.51.100.10"],"allowed_domains":["client.example.com"],"expires_at":"2030-01-01T00:00:00Z"}`))
	updateReq.Header.Set("Authorization", "Bearer console-user")
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK || service.updated.TokenID != "token-1" || service.updated.TenantID != "tenant-1" || service.updated.CreatedBy != "user-1" || service.updated.Name != "edited-token" || service.updated.GroupID != "group-2" || service.updated.SpendLimit != "250.5" || len(service.updated.AllowedIPs) != 1 || service.updated.AllowedIPs[0] != "198.51.100.10" {
		t.Fatalf("expected owner-scoped token update 200, got %d: %s (%#v)", updateRec.Code, updateRec.Body.String(), service.updated)
	}

	pauseReq := httptest.NewRequest(http.MethodPost, "/console/v1/tenants/tenant-1/tokens/token-1/pause", nil)
	pauseReq.Header.Set("Authorization", "Bearer console-user")
	pauseRec := httptest.NewRecorder()
	handler.ServeHTTP(pauseRec, pauseReq)
	if pauseRec.Code != http.StatusOK || service.status != "disabled" || service.statusTokenID != "token-1" || service.statusTenantID != "tenant-1" || service.statusOwnerID != "user-1" {
		t.Fatalf("expected owner-scoped token pause 200, got %d: %s", pauseRec.Code, pauseRec.Body.String())
	}

	resumeReq := httptest.NewRequest(http.MethodPost, "/console/v1/tenants/tenant-1/tokens/token-1/resume", nil)
	resumeReq.Header.Set("Authorization", "Bearer console-user")
	resumeRec := httptest.NewRecorder()
	handler.ServeHTTP(resumeRec, resumeReq)
	if resumeRec.Code != http.StatusOK || service.status != "active" {
		t.Fatalf("expected owner-scoped token resume 200, got %d: %s", resumeRec.Code, resumeRec.Body.String())
	}

	editorDeleteReq := httptest.NewRequest(http.MethodDelete, "/console/v1/tenants/tenant-1/tokens/token-1", nil)
	editorDeleteReq.Header.Set("Authorization", "Bearer console-editor")
	editorDeleteRec := httptest.NewRecorder()
	handler.ServeHTTP(editorDeleteRec, editorDeleteReq)
	if editorDeleteRec.Code != http.StatusForbidden || service.deleted {
		t.Fatalf("token delete must require token:revoke, got %d: %s", editorDeleteRec.Code, editorDeleteRec.Body.String())
	}

	terminateReq := httptest.NewRequest(http.MethodPost, "/console/v1/tenants/tenant-1/tokens/token-1/terminate", nil)
	terminateReq.Header.Set("Authorization", "Bearer console-user")
	terminateRec := httptest.NewRecorder()
	handler.ServeHTTP(terminateRec, terminateReq)
	if terminateRec.Code != http.StatusOK || service.status != "revoked" {
		t.Fatalf("expected owner-scoped token terminate 200, got %d: %s", terminateRec.Code, terminateRec.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/console/v1/tenants/tenant-1/tokens/token-1", nil)
	deleteReq.Header.Set("Authorization", "Bearer console-user")
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK || !service.deleted || service.deletedTokenID != "token-1" || service.deletedTenantID != "tenant-1" || service.deletedOwnerID != "user-1" {
		t.Fatalf("expected owner-scoped token delete 200, got %d: %s", deleteRec.Code, deleteRec.Body.String())
	}

	foreignProjectReq := httptest.NewRequest(http.MethodPost, "/console/v1/tenants/tenant-1/tokens", strings.NewReader(`{"project_id":"project-2","name":"not-allowed"}`))
	foreignProjectReq.Header.Set("Authorization", "Bearer console-user")
	foreignProjectRec := httptest.NewRecorder()
	handler.ServeHTTP(foreignProjectRec, foreignProjectReq)
	if foreignProjectRec.Code != http.StatusNotFound {
		t.Fatalf("expected inaccessible project 404, got %d", foreignProjectRec.Code)
	}

	adminReq := httptest.NewRequest(http.MethodGet, "/console/v1/tenants/tenant-1/tokens", nil)
	adminReq.Header.Set("Authorization", "Bearer admin-user")
	adminRec := httptest.NewRecorder()
	handler.ServeHTTP(adminRec, adminReq)
	if adminRec.Code != http.StatusForbidden {
		t.Fatalf("expected admin audience rejected by console route, got %d", adminRec.Code)
	}
}

func TestConsoleModelStatusRouteIsTenantAndPermissionBound(t *testing.T) {
	groupService := &fakeGroupService{}
	handler := NewWithRelayAndGroups(auth.NewMiddleware(testResolver{
		"console-status": {
			ID:       "user-1",
			Type:     auth.PrincipalTenantUser,
			Audience: auth.AudienceConsole,
			TenantID: "tenant-1",
			Permissions: map[string]struct{}{
				"model:status:read": {},
			},
		},
		"console-no-status": {
			ID:       "user-2",
			Type:     auth.PrincipalTenantUser,
			Audience: auth.AudienceConsole,
			TenantID: "tenant-1",
		},
	}), nil, nil, false, "../../web", groupService)

	request := httptest.NewRequest(http.MethodGet, "/console/v1/tenants/tenant-1/model-status", nil)
	request.Header.Set("Authorization", "Bearer console-status")
	record := httptest.NewRecorder()
	handler.ServeHTTP(record, request)
	if record.Code != http.StatusOK || !strings.Contains(record.Body.String(), `"group_code":"standard"`) {
		t.Fatalf("expected model status report 200, got %d: %s", record.Code, record.Body.String())
	}

	foreign := httptest.NewRequest(http.MethodGet, "/console/v1/tenants/tenant-2/model-status", nil)
	foreign.Header.Set("Authorization", "Bearer console-status")
	foreignRecord := httptest.NewRecorder()
	handler.ServeHTTP(foreignRecord, foreign)
	if foreignRecord.Code != http.StatusNotFound {
		t.Fatalf("expected foreign tenant model status 404, got %d", foreignRecord.Code)
	}

	denied := httptest.NewRequest(http.MethodGet, "/console/v1/tenants/tenant-1/model-status", nil)
	denied.Header.Set("Authorization", "Bearer console-no-status")
	deniedRecord := httptest.NewRecorder()
	handler.ServeHTTP(deniedRecord, denied)
	if deniedRecord.Code != http.StatusForbidden {
		t.Fatalf("expected missing model status permission 403, got %d", deniedRecord.Code)
	}
}

func TestModelStatusFeatureSwitchClosesConsoleRouteAndPublicFlag(t *testing.T) {
	settings := &fakeSystemSettingsService{
		features: adminsettings.FeatureSettings{ModelStatusEnabled: false},
	}
	handler := NewWithRelayAndGroups(auth.NewMiddleware(testResolver{
		"console-status": {
			ID:       "user-1",
			Type:     auth.PrincipalTenantUser,
			Audience: auth.AudienceConsole,
			TenantID: "tenant-1",
			Permissions: map[string]struct{}{
				"model:status:read": {},
			},
		},
	}), &auth.Services{SecuritySettings: settings}, nil, false, "../../web", &fakeGroupService{})

	request := httptest.NewRequest(http.MethodGet, "/console/v1/tenants/tenant-1/model-status", nil)
	request.Header.Set("Authorization", "Bearer console-status")
	record := httptest.NewRecorder()
	handler.ServeHTTP(record, request)
	if record.Code != http.StatusNotFound || !strings.Contains(record.Body.String(), "MODEL_STATUS_DISABLED") {
		t.Fatalf("expected disabled model status route, got %d: %s", record.Code, record.Body.String())
	}

	publicRequest := httptest.NewRequest(http.MethodGet, "/public/v1/features", nil)
	publicRecord := httptest.NewRecorder()
	handler.ServeHTTP(publicRecord, publicRequest)
	if publicRecord.Code != http.StatusOK || !strings.Contains(publicRecord.Body.String(), `"model_status_enabled":false`) {
		t.Fatalf("expected public model status flag false, got %d: %s", publicRecord.Code, publicRecord.Body.String())
	}
}

func TestAdminModelStatusRouteRequiresOperationsPermission(t *testing.T) {
	handler := NewWithRelayAndGroups(auth.NewMiddleware(testResolver{
		"admin-ops": {
			ID:       "admin-1",
			Type:     auth.PrincipalPlatformUser,
			Audience: auth.AudienceAdmin,
			Permissions: map[string]struct{}{
				"operations:read": {},
			},
		},
		"admin-no-ops": {
			ID:       "admin-2",
			Type:     auth.PrincipalPlatformUser,
			Audience: auth.AudienceAdmin,
		},
	}), nil, nil, false, "../../web", &fakeGroupService{})

	allowed := httptest.NewRequest(http.MethodGet, "/admin/v1/model-status", nil)
	allowed.Header.Set("Authorization", "Bearer admin-ops")
	allowedRecord := httptest.NewRecorder()
	handler.ServeHTTP(allowedRecord, allowed)
	if allowedRecord.Code != http.StatusOK || !strings.Contains(allowedRecord.Body.String(), `"group_code":"standard"`) {
		t.Fatalf("expected admin model status report, got %d: %s", allowedRecord.Code, allowedRecord.Body.String())
	}

	denied := httptest.NewRequest(http.MethodGet, "/admin/v1/model-status", nil)
	denied.Header.Set("Authorization", "Bearer admin-no-ops")
	deniedRecord := httptest.NewRecorder()
	handler.ServeHTTP(deniedRecord, denied)
	if deniedRecord.Code != http.StatusForbidden {
		t.Fatalf("expected operations permission rejection, got %d", deniedRecord.Code)
	}
}

func TestAdminModelMonitorRoutesRequirePermissionAndConfiguredStepUp(t *testing.T) {
	settings := &fakeSystemSettingsService{
		features: adminsettings.FeatureSettings{
			TOTPEnabled:               true,
			StepUpChannelModelEnabled: true,
		},
	}
	stepUp := &fakeStepUpVerifier{}
	groupService := &fakeModelMonitorGroupService{}
	handler := NewWithRelayAndGroups(
		auth.NewMiddleware(testResolver{
			"admin-monitor-read": {
				ID:       "admin-1",
				Type:     auth.PrincipalPlatformUser,
				Audience: auth.AudienceAdmin,
				Permissions: map[string]struct{}{
					"operations:read": {},
				},
			},
			"admin-monitor-write": {
				ID:       "admin-2",
				Type:     auth.PrincipalPlatformUser,
				Audience: auth.AudienceAdmin,
				Permissions: map[string]struct{}{
					"operations:read":   {},
					"operations:update": {},
				},
			},
		}),
		&auth.Services{SecuritySettings: settings, StepUpMFA: stepUp},
		nil,
		false,
		"../../web",
		groupService,
	)

	read := httptest.NewRequest(http.MethodGet, "/admin/v1/model-monitors", nil)
	read.Header.Set("Authorization", "Bearer admin-monitor-read")
	readRecord := httptest.NewRecorder()
	handler.ServeHTTP(readRecord, read)
	if readRecord.Code != http.StatusOK {
		t.Fatalf("monitor read should be allowed, got %d: %s", readRecord.Code, readRecord.Body.String())
	}

	body := `{"group_id":"11111111-1111-4111-8111-111111111111","name":"Primary","selection_mode":"all","mode":"passive","enabled":true}`
	denied := httptest.NewRequest(http.MethodPost, "/admin/v1/model-monitors", strings.NewReader(body))
	denied.Header.Set("Authorization", "Bearer admin-monitor-read")
	deniedRecord := httptest.NewRecorder()
	handler.ServeHTTP(deniedRecord, denied)
	if deniedRecord.Code != http.StatusForbidden {
		t.Fatalf("monitor write without permission should be rejected, got %d", deniedRecord.Code)
	}

	missingStepUp := httptest.NewRequest(http.MethodPost, "/admin/v1/model-monitors", strings.NewReader(body))
	missingStepUp.Header.Set("Authorization", "Bearer admin-monitor-write")
	missingStepUpRecord := httptest.NewRecorder()
	handler.ServeHTTP(missingStepUpRecord, missingStepUp)
	if missingStepUpRecord.Code != http.StatusForbidden || !strings.Contains(missingStepUpRecord.Body.String(), `"error":"STEP_UP_REQUIRED"`) {
		t.Fatalf("monitor write without step-up should be rejected, got %d: %s", missingStepUpRecord.Code, missingStepUpRecord.Body.String())
	}

	allowed := httptest.NewRequest(http.MethodPost, "/admin/v1/model-monitors", strings.NewReader(body))
	allowed.Header.Set("Authorization", "Bearer admin-monitor-write")
	allowed.Header.Set("X-MFA-Code", "123456")
	allowedRecord := httptest.NewRecorder()
	handler.ServeHTTP(allowedRecord, allowed)
	if allowedRecord.Code != http.StatusCreated || groupService.createdBy != "admin-2" || stepUp.calls != 1 {
		t.Fatalf("monitor write with step-up should succeed, got %d: %s calls=%d", allowedRecord.Code, allowedRecord.Body.String(), stepUp.calls)
	}
}

func TestAdminSecuritySettingsRoutes(t *testing.T) {
	settings := &fakeSecuritySettingsService{enabled: false}
	handler := New(auth.NewMiddleware(testResolver{
		"admin-ok": {
			ID:       "user-1",
			Type:     auth.PrincipalPlatformUser,
			Audience: auth.AudienceAdmin,
			Roles:    []string{"platform_owner"},
			Permissions: map[string]struct{}{
				"security:read":   {},
				"security:update": {},
				"audit:read":      {},
			},
		},
	}), &auth.Services{StepUpMFA: &fakeStepUpVerifier{}, SecuritySettings: settings}, false, "../../web")

	getReq := httptest.NewRequest(http.MethodGet, "/admin/v1/security/settings", nil)
	getReq.Header.Set("Authorization", "Bearer admin-ok")
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected read 200, got %d", getRec.Code)
	}
	if !strings.Contains(getRec.Body.String(), `"admin_mfa_enabled":false`) {
		t.Fatalf("unexpected settings body: %s", getRec.Body.String())
	}

	putReq := httptest.NewRequest(http.MethodPut, "/admin/v1/security/settings", strings.NewReader(
		`{"admin_mfa_enabled":true}`,
	))
	putReq.Header.Set("Authorization", "Bearer admin-ok")
	putReq.Header.Set("X-MFA-Code", "123456")
	putRec := httptest.NewRecorder()
	handler.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("expected update 200, got %d", putRec.Code)
	}
	if !strings.Contains(putRec.Body.String(), `"admin_mfa_enabled":true`) {
		t.Fatalf("unexpected update body: %s", putRec.Body.String())
	}
}

func TestSystemSettingsRouteRequiresAdminPermission(t *testing.T) {
	settings := &fakeSystemSettingsService{}
	handler := New(auth.NewMiddleware(testResolver{
		"admin-settings": {
			ID:       "11111111-1111-4111-8111-111111111111",
			Type:     auth.PrincipalPlatformUser,
			Audience: auth.AudienceAdmin,
			Permissions: map[string]struct{}{
				"security:read":   {},
				"security:update": {},
			},
		},
		"admin-no-settings": {
			ID:       "22222222-2222-4222-8222-222222222222",
			Type:     auth.PrincipalPlatformUser,
			Audience: auth.AudienceAdmin,
		},
	}), &auth.Services{StepUpMFA: &fakeStepUpVerifier{}, SecuritySettings: settings}, false, "../../web")

	read := httptest.NewRequest(http.MethodGet, "/admin/v1/settings", nil)
	read.Header.Set("Authorization", "Bearer admin-settings")
	readRec := httptest.NewRecorder()
	handler.ServeHTTP(readRec, read)
	if readRec.Code != http.StatusOK || !strings.Contains(readRec.Body.String(), `"site_name":"AI Token Gateway"`) {
		t.Fatalf("expected system settings 200, got %d: %s", readRec.Code, readRec.Body.String())
	}

	update := httptest.NewRequest(http.MethodPut, "/admin/v1/settings", strings.NewReader(`{"site_name":"Acme Gateway","site_logo_url":"/assets/acme.png","site_favicon_url":"https://cdn.example.com/acme.ico"}`))
	update.Header.Set("Authorization", "Bearer admin-settings")
	update.Header.Set("X-MFA-Code", "123456")
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, update)
	if updateRec.Code != http.StatusOK || settings.updated.SiteName != "Acme Gateway" {
		t.Fatalf("expected system settings update 200, got %d: %s", updateRec.Code, updateRec.Body.String())
	}

	denied := httptest.NewRequest(http.MethodGet, "/admin/v1/settings", nil)
	denied.Header.Set("Authorization", "Bearer admin-no-settings")
	deniedRec := httptest.NewRecorder()
	handler.ServeHTTP(deniedRec, denied)
	if deniedRec.Code != http.StatusForbidden {
		t.Fatalf("expected settings permission denial, got %d", deniedRec.Code)
	}
}

func TestAPIEndpointManagementIsAdminOnlyAndPublicListHidesDisabled(t *testing.T) {
	settings := &fakeSystemSettingsService{
		endpoints: []adminsettings.APIEndpoint{
			{ID: "endpoint-1", Name: "Primary", BaseURL: "https://gateway.example.com/v1", Enabled: true, SortOrder: 0},
			{ID: "endpoint-2", Name: "Legacy", BaseURL: "https://legacy.example.com/v1", Enabled: false, SortOrder: 1},
		},
	}
	handler := New(auth.NewMiddleware(testResolver{
		"endpoint-admin": {
			ID:       "11111111-1111-4111-8111-111111111111",
			Type:     auth.PrincipalPlatformUser,
			Audience: auth.AudienceAdmin,
			Permissions: map[string]struct{}{
				"security:read":   {},
				"security:update": {},
			},
		},
		"endpoint-readonly": {
			ID:       "22222222-2222-4222-8222-222222222222",
			Type:     auth.PrincipalPlatformUser,
			Audience: auth.AudienceAdmin,
			Permissions: map[string]struct{}{
				"security:read": {},
			},
		},
	}), &auth.Services{StepUpMFA: &fakeStepUpVerifier{}, SecuritySettings: settings}, false, "../../web")

	publicRequest := httptest.NewRequest(http.MethodGet, "/public/v1/settings", nil)
	publicRecord := httptest.NewRecorder()
	handler.ServeHTTP(publicRecord, publicRequest)
	if publicRecord.Code != http.StatusOK ||
		!strings.Contains(publicRecord.Body.String(), `"name":"Primary"`) ||
		strings.Contains(publicRecord.Body.String(), "Legacy") ||
		strings.Contains(publicRecord.Body.String(), `"id":"endpoint-1"`) ||
		publicRecord.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("public settings must expose enabled endpoint names and URLs only, got %d: %s", publicRecord.Code, publicRecord.Body.String())
	}

	listRequest := httptest.NewRequest(http.MethodGet, "/admin/v1/settings/api-endpoints", nil)
	listRequest.Header.Set("Authorization", "Bearer endpoint-admin")
	listRecord := httptest.NewRecorder()
	handler.ServeHTTP(listRecord, listRequest)
	if listRecord.Code != http.StatusOK ||
		!strings.Contains(listRecord.Body.String(), "Legacy") ||
		!strings.Contains(listRecord.Body.String(), `"enabled":false`) {
		t.Fatalf("admin list must include disabled endpoint, got %d: %s", listRecord.Code, listRecord.Body.String())
	}

	createRequest := httptest.NewRequest(http.MethodPost, "/admin/v1/settings/api-endpoints", strings.NewReader(`{"name":"Backup","base_url":"https://backup.example.com/v1","enabled":true}`))
	createRequest.Header.Set("Authorization", "Bearer endpoint-admin")
	createRequest.Header.Set("X-MFA-Code", "123456")
	createRecord := httptest.NewRecorder()
	handler.ServeHTTP(createRecord, createRequest)
	if createRecord.Code != http.StatusCreated || !strings.Contains(createRecord.Body.String(), `"name":"Backup"`) {
		t.Fatalf("admin endpoint create must succeed, got %d: %s", createRecord.Code, createRecord.Body.String())
	}

	deniedRequest := httptest.NewRequest(http.MethodPost, "/admin/v1/settings/api-endpoints", strings.NewReader(`{"name":"Nope","base_url":"https://nope.example.com"}`))
	deniedRequest.Header.Set("Authorization", "Bearer endpoint-readonly")
	deniedRecord := httptest.NewRecorder()
	handler.ServeHTTP(deniedRecord, deniedRequest)
	if deniedRecord.Code != http.StatusForbidden {
		t.Fatalf("console user must not manage API endpoints, got %d: %s", deniedRecord.Code, deniedRecord.Body.String())
	}
}

func TestAdminEmailSettingsRoutesAreProtectedAndSeparated(t *testing.T) {
	settings := &fakeSystemSettingsService{}
	stepUp := &fakeStepUpVerifier{}
	handler := New(auth.NewMiddleware(testResolver{
		"email-admin": {
			ID:       "11111111-1111-4111-8111-111111111111",
			Type:     auth.PrincipalPlatformUser,
			Audience: auth.AudienceAdmin,
			Permissions: map[string]struct{}{
				"security:read":   {},
				"security:update": {},
			},
		},
	}), &auth.Services{StepUpMFA: stepUp, SecuritySettings: settings}, false, "../../web")

	read := httptest.NewRequest(http.MethodGet, "/admin/v1/settings/email", nil)
	read.Header.Set("Authorization", "Bearer email-admin")
	readRec := httptest.NewRecorder()
	handler.ServeHTTP(readRec, read)
	if readRec.Code != http.StatusOK || !strings.Contains(readRec.Body.String(), `"email_enabled":false`) || strings.Contains(readRec.Body.String(), `"smtp_password":"`) {
		t.Fatalf("expected separated email settings without password, got %d: %s", readRec.Code, readRec.Body.String())
	}

	update := httptest.NewRequest(http.MethodPut, "/admin/v1/settings/features", strings.NewReader(`{"email_enabled":false,"email_verification_enabled":true,"email_password_reset_enabled":true,"email_subscription_enabled":true,"email_low_balance_alert_enabled":false,"email_recharge_success_enabled":false,"email_usage_limit_alert_enabled":false,"email_content_audit_enabled":false,"email_account_disabled_enabled":false,"email_cyber_policy_enabled":false,"email_operations_enabled":false,"balance_threshold":"0","recharge_url":""}`))
	update.Header.Set("Authorization", "Bearer email-admin")
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, update)
	if updateRec.Code != http.StatusOK || stepUp.calls != 0 {
		t.Fatalf("disabled MFA policy must not require step-up, got %d: %s calls=%d", updateRec.Code, updateRec.Body.String(), stepUp.calls)
	}

	templates := httptest.NewRequest(http.MethodGet, "/admin/v1/settings/email/templates", nil)
	templates.Header.Set("Authorization", "Bearer email-admin")
	templatesRec := httptest.NewRecorder()
	handler.ServeHTTP(templatesRec, templates)
	if templatesRec.Code != http.StatusOK {
		t.Fatalf("expected template list 200, got %d: %s", templatesRec.Code, templatesRec.Body.String())
	}
}

func TestFeatureSettingsValidationReturnsSpecificErrors(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		code string
	}{
		{name: "balance threshold", err: adminsettings.ErrInvalidBalanceThreshold, code: "INVALID_BALANCE_THRESHOLD"},
		{name: "recharge URL", err: adminsettings.ErrInvalidRechargeURL, code: "INVALID_RECHARGE_URL"},
		{name: "SMTP required", err: adminsettings.ErrEmailSMTPRequired, code: "EMAIL_SMTP_REQUIRED"},
		{name: "SMTP invalid", err: adminsettings.ErrInvalidEmailSMTP, code: "INVALID_EMAIL_SMTP"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			settings := &fakeSystemSettingsService{featureUpdateErr: testCase.err}
			handler := New(auth.NewMiddleware(testResolver{
				"feature-admin": {
					ID:       "11111111-1111-4111-8111-111111111111",
					Type:     auth.PrincipalPlatformUser,
					Audience: auth.AudienceAdmin,
					Permissions: map[string]struct{}{
						"security:update": {},
					},
				},
			}), &auth.Services{SecuritySettings: settings}, false, "../../web")
			request := httptest.NewRequest(http.MethodPut, "/admin/v1/settings/features", strings.NewReader(`{"balance_threshold":"0","recharge_url":""}`))
			request.Header.Set("Authorization", "Bearer feature-admin")
			record := httptest.NewRecorder()
			handler.ServeHTTP(record, request)
			if record.Code != http.StatusBadRequest || !strings.Contains(record.Body.String(), `{"error":"`+testCase.code+`"}`) {
				t.Fatalf("expected %s, got %d: %s", testCase.code, record.Code, record.Body.String())
			}
		})
	}
}

func TestConsoleProfileRoutesAreAudienceBoundAndSelfService(t *testing.T) {
	t.Setenv("ADMIN_ENTRY_PATH", "/admin-0123456789abcdef")
	userService := &fakeProfileUserService{}
	mfaService := &fakeMFASettingsService{}
	resolver := testResolver{
		"console-user": {
			ID:       "22222222-2222-4222-8222-222222222222",
			Type:     auth.PrincipalTenantUser,
			Audience: auth.AudienceConsole,
			TenantID: "33333333-3333-4333-8333-333333333333",
			Roles:    []string{"tenant_owner"},
			Permissions: map[string]struct{}{
				"token:read":   {},
				"token:create": {},
				"usage:read":   {},
				"billing:read": {},
			},
			ProjectIDs: map[string]struct{}{
				"44444444-4444-4444-8444-444444444444": {},
			},
		},
		"admin-user": {
			ID:       "11111111-1111-4111-8111-111111111111",
			Type:     auth.PrincipalPlatformUser,
			Audience: auth.AudienceAdmin,
		},
	}
	handler := NewWithRelayAndGroupsAndTokensAndConsoleTokensAndModelCatalogAndUsers(
		auth.NewMiddleware(resolver),
		&auth.Services{MFA: mfaService},
		nil,
		false,
		"../../web",
		nil,
		nil,
		nil,
		nil,
		userService,
	)

	unauthenticated := httptest.NewRequest(http.MethodGet, "/console/v1/profile", nil)
	unauthenticatedRec := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticatedRec, unauthenticated)
	if unauthenticatedRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated profile 401, got %d", unauthenticatedRec.Code)
	}

	adminRequest := httptest.NewRequest(http.MethodGet, "/console/v1/profile", nil)
	adminRequest.Header.Set("Authorization", "Bearer admin-user")
	adminRec := httptest.NewRecorder()
	handler.ServeHTTP(adminRec, adminRequest)
	if adminRec.Code != http.StatusForbidden {
		t.Fatalf("expected admin audience rejected by profile route, got %d", adminRec.Code)
	}

	profileRequest := httptest.NewRequest(http.MethodGet, "/console/v1/profile", nil)
	profileRequest.Header.Set("Authorization", "Bearer console-user")
	profileRec := httptest.NewRecorder()
	handler.ServeHTTP(profileRec, profileRequest)
	if profileRec.Code != http.StatusOK || !strings.Contains(profileRec.Body.String(), `"email":"user@example.com"`) || !strings.Contains(profileRec.Body.String(), `"permissions":["billing:read","token:create","token:read","usage:read"]`) {
		t.Fatalf("expected profile 200 with email, got %d: %s", profileRec.Code, profileRec.Body.String())
	}

	meRequest := httptest.NewRequest(http.MethodGet, "/console/v1/me", nil)
	meRequest.Header.Set("Authorization", "Bearer console-user")
	meRec := httptest.NewRecorder()
	handler.ServeHTTP(meRec, meRequest)
	if meRec.Code != http.StatusOK || !strings.Contains(meRec.Body.String(), `"permissions":["billing:read","token:create","token:read","usage:read"]`) {
		t.Fatalf("console me response must include permissions, got %d: %s", meRec.Code, meRec.Body.String())
	}
	if strings.Contains(meRec.Body.String(), `"admin_entry_path"`) {
		t.Fatalf("console me response must not expose the administrator entry path: %s", meRec.Body.String())
	}

	adminMeRequest := httptest.NewRequest(http.MethodGet, "/admin/v1/me", nil)
	adminMeRequest.Header.Set("Authorization", "Bearer admin-user")
	adminMeRec := httptest.NewRecorder()
	handler.ServeHTTP(adminMeRec, adminMeRequest)
	if adminMeRec.Code != http.StatusOK || !strings.Contains(adminMeRec.Body.String(), `"admin_entry_path":"/admin-0123456789abcdef"`) {
		t.Fatalf("admin me response must return the configured administrator entry path, got %d: %s", adminMeRec.Code, adminMeRec.Body.String())
	}

	adminProfileRequest := httptest.NewRequest(http.MethodGet, "/admin/v1/profile", nil)
	adminProfileRequest.Header.Set("Authorization", "Bearer admin-user")
	adminProfileRec := httptest.NewRecorder()
	handler.ServeHTTP(adminProfileRec, adminProfileRequest)
	if adminProfileRec.Code != http.StatusOK || !strings.Contains(adminProfileRec.Body.String(), `"admin_entry_path":"/admin-0123456789abcdef"`) {
		t.Fatalf("admin profile response must return the configured administrator entry path, got %d: %s", adminProfileRec.Code, adminProfileRec.Body.String())
	}

	updateRequest := httptest.NewRequest(http.MethodPut, "/console/v1/profile", strings.NewReader(`{"display_name":"Updated User"}`))
	updateRequest.Header.Set("Authorization", "Bearer console-user")
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, updateRequest)
	if updateRec.Code != http.StatusOK || userService.profile.displayName != "Updated User" {
		t.Fatalf("expected profile update 200, got %d: %s", updateRec.Code, updateRec.Body.String())
	}

	mfaRequest := httptest.NewRequest(http.MethodGet, "/console/v1/profile/mfa", nil)
	mfaRequest.Header.Set("Authorization", "Bearer console-user")
	mfaRec := httptest.NewRecorder()
	handler.ServeHTTP(mfaRec, mfaRequest)
	if mfaRec.Code != http.StatusOK || !strings.Contains(mfaRec.Body.String(), `"enabled":false`) {
		t.Fatalf("expected mfa status 200, got %d: %s", mfaRec.Code, mfaRec.Body.String())
	}

	disableRequest := httptest.NewRequest(http.MethodPost, "/console/v1/profile/mfa/disable", strings.NewReader(`{"code":"123456"}`))
	disableRequest.Header.Set("Authorization", "Bearer console-user")
	disableRec := httptest.NewRecorder()
	handler.ServeHTTP(disableRec, disableRequest)
	if disableRec.Code != http.StatusConflict || !strings.Contains(disableRec.Body.String(), `"error":"MFA_NOT_ENABLED"`) {
		t.Fatalf("expected disabled mfa conflict, got %d: %s", disableRec.Code, disableRec.Body.String())
	}
}

func TestTOTPFeatureGateHidesEnrollmentWhenDisabled(t *testing.T) {
	mfaService := &fakeMFASettingsService{}
	settings := &fakeSystemSettingsService{}
	handler := New(auth.NewMiddleware(testResolver{
		"console-user": {
			ID:       "22222222-2222-4222-8222-222222222222",
			Type:     auth.PrincipalTenantUser,
			Audience: auth.AudienceConsole,
			TenantID: "33333333-3333-4333-8333-333333333333",
		},
	}), &auth.Services{MFA: mfaService, SecuritySettings: settings}, false, "../../web")

	status := httptest.NewRequest(http.MethodGet, "/console/v1/profile/mfa", nil)
	status.Header.Set("Authorization", "Bearer console-user")
	statusRec := httptest.NewRecorder()
	handler.ServeHTTP(statusRec, status)
	if statusRec.Code != http.StatusOK || !strings.Contains(statusRec.Body.String(), `"enabled":false`) {
		t.Fatalf("disabled TOTP feature must hide active status, got %d: %s", statusRec.Code, statusRec.Body.String())
	}

	enroll := httptest.NewRequest(http.MethodPost, "/console/v1/profile/mfa/enroll", strings.NewReader(`{"issuer":"AI Token","account":"user@example.com"}`))
	enroll.Header.Set("Authorization", "Bearer console-user")
	enrollRec := httptest.NewRecorder()
	handler.ServeHTTP(enrollRec, enroll)
	if enrollRec.Code != http.StatusNotFound || !strings.Contains(enrollRec.Body.String(), `"error":"TOTP_DISABLED"`) {
		t.Fatalf("disabled TOTP feature must close enrollment, got %d: %s", enrollRec.Code, enrollRec.Body.String())
	}
}

type fakeLoginService struct{}

func (fakeLoginService) Login(_ context.Context, _ auth.LoginRequest, _ auth.Audience) (auth.IssuedSession, error) {
	return auth.IssuedSession{}, auth.ErrInvalidCredentials
}

func (fakeLoginService) Logout(_ context.Context, _ string) error {
	return nil
}

type successfulLoginService struct{}

func (successfulLoginService) Login(_ context.Context, _ auth.LoginRequest, _ auth.Audience) (auth.IssuedSession, error) {
	return auth.IssuedSession{
		Secret:    "session-secret",
		Audience:  auth.AudienceAdmin,
		ExpiresAt: time.Now().Add(time.Hour),
	}, nil
}

func (successfulLoginService) Logout(_ context.Context, _ string) error {
	return nil
}

type fakePasswordResetService struct{}

func (fakePasswordResetService) Request(_ context.Context, _ string, _ string) (string, bool, error) {
	return "reset-secret", true, nil
}

func (fakePasswordResetService) Confirm(_ context.Context, _ string, _ string) error {
	return nil
}

type failingPasswordResetNotifier struct{}

func (failingPasswordResetNotifier) SendPasswordReset(context.Context, string, string) error {
	return errors.New("smtp unavailable")
}

type fakeSecuritySettingsService struct {
	enabled bool
}

func (s *fakeSecuritySettingsService) AdminMFAEnabled(_ context.Context) (bool, error) {
	return s.enabled, nil
}

func (s *fakeSecuritySettingsService) GetAdminSecuritySettings(_ context.Context) (auth.SecuritySettings, error) {
	return auth.SecuritySettings{
		AdminMFAEnabled: s.enabled,
	}, nil
}

func (s *fakeSecuritySettingsService) UpdateAdminMFAEnabled(_ context.Context, enabled bool, actorID string) (auth.SecuritySettings, error) {
	s.enabled = enabled
	return auth.SecuritySettings{
		AdminMFAEnabled: enabled,
		UpdatedBy:       actorID,
	}, nil
}

type fakeSystemSettingsService struct {
	fakeSecuritySettingsService
	updated          adminsettings.SystemSettingsUpdate
	emailSettings    adminsettings.EmailSettings
	features         adminsettings.FeatureSettings
	featureUpdateErr error
	templates        []adminsettings.EmailTemplate
	endpoints        []adminsettings.APIEndpoint
	endpointSequence int
}

func (s *fakeSystemSettingsService) GetSystemSettings(_ context.Context) (adminsettings.SystemSettings, error) {
	return adminsettings.SystemSettings{
		AdminMFAEnabled: s.enabled,
		SiteName:        "AI Token Gateway",
	}, nil
}

func (s *fakeSystemSettingsService) UpdateSystemSettings(_ context.Context, _ string, update adminsettings.SystemSettingsUpdate) (adminsettings.SystemSettings, error) {
	s.updated = update
	return adminsettings.SystemSettings{
		AdminMFAEnabled: s.enabled,
		SiteName:        update.SiteName,
		SiteLogoURL:     update.SiteLogoURL,
		SiteFaviconURL:  update.SiteFaviconURL,
	}, nil
}

func (s *fakeSystemSettingsService) GetEmailSettings(context.Context) (adminsettings.EmailSettings, error) {
	return s.emailSettings, nil
}

func (s *fakeSystemSettingsService) UpdateEmailSettings(_ context.Context, _ string, _ adminsettings.EmailSettingsUpdate) (adminsettings.EmailSettings, error) {
	return s.emailSettings, nil
}

func (s *fakeSystemSettingsService) TestSMTPConnection(context.Context) error { return nil }

func (s *fakeSystemSettingsService) SendTestEmail(context.Context, string) error { return nil }

func (s *fakeSystemSettingsService) GetFeatureSettings(context.Context) (adminsettings.FeatureSettings, error) {
	return s.features, nil
}

func (s *fakeSystemSettingsService) UpdateFeatureSettings(_ context.Context, _ string, _ adminsettings.FeatureSettingsUpdate) (adminsettings.FeatureSettings, error) {
	if s.featureUpdateErr != nil {
		return adminsettings.FeatureSettings{}, s.featureUpdateErr
	}
	return s.features, nil
}

func (s *fakeSystemSettingsService) ListEmailTemplates(context.Context) ([]adminsettings.EmailTemplate, error) {
	return s.templates, nil
}

func (s *fakeSystemSettingsService) CreateEmailTemplate(context.Context, string, adminsettings.EmailTemplateMutation) (adminsettings.EmailTemplate, error) {
	return adminsettings.EmailTemplate{}, nil
}

func (s *fakeSystemSettingsService) UpdateEmailTemplate(context.Context, string, string, adminsettings.EmailTemplateMutation) (adminsettings.EmailTemplate, error) {
	return adminsettings.EmailTemplate{}, nil
}

func (s *fakeSystemSettingsService) DeleteEmailTemplate(context.Context, string, string) error {
	return nil
}

func (s *fakeSystemSettingsService) ListAPIEndpoints(_ context.Context, enabledOnly bool) ([]adminsettings.APIEndpoint, error) {
	result := make([]adminsettings.APIEndpoint, 0, len(s.endpoints))
	for _, item := range s.endpoints {
		if enabledOnly && !item.Enabled {
			continue
		}
		result = append(result, item)
	}
	return result, nil
}

func (s *fakeSystemSettingsService) CreateAPIEndpoint(_ context.Context, _ string, mutation adminsettings.APIEndpointMutation) (adminsettings.APIEndpoint, error) {
	if strings.TrimSpace(mutation.Name) == "" || strings.TrimSpace(mutation.BaseURL) == "" {
		return adminsettings.APIEndpoint{}, adminsettings.ErrInvalidAPIEndpoint
	}
	for _, item := range s.endpoints {
		if item.BaseURL == mutation.BaseURL {
			return adminsettings.APIEndpoint{}, adminsettings.ErrAPIEndpointExists
		}
	}
	s.endpointSequence++
	enabled := true
	if mutation.Enabled != nil {
		enabled = *mutation.Enabled
	}
	item := adminsettings.APIEndpoint{
		ID:        "endpoint-" + strconv.Itoa(s.endpointSequence),
		Name:      mutation.Name,
		BaseURL:   mutation.BaseURL,
		Enabled:   enabled,
		SortOrder: len(s.endpoints),
	}
	s.endpoints = append(s.endpoints, item)
	return item, nil
}

func (s *fakeSystemSettingsService) UpdateAPIEndpoint(_ context.Context, _ string, endpointID string, mutation adminsettings.APIEndpointMutation) (adminsettings.APIEndpoint, error) {
	for index, item := range s.endpoints {
		if item.ID != endpointID {
			continue
		}
		if strings.TrimSpace(mutation.Name) == "" || strings.TrimSpace(mutation.BaseURL) == "" {
			return adminsettings.APIEndpoint{}, adminsettings.ErrInvalidAPIEndpoint
		}
		if mutation.Enabled == nil {
			mutation.Enabled = &item.Enabled
		}
		s.endpoints[index] = adminsettings.APIEndpoint{
			ID: item.ID, Name: mutation.Name, BaseURL: mutation.BaseURL,
			Enabled: *mutation.Enabled, SortOrder: item.SortOrder,
		}
		return s.endpoints[index], nil
	}
	return adminsettings.APIEndpoint{}, adminsettings.ErrAPIEndpointNotFound
}

func (s *fakeSystemSettingsService) DeleteAPIEndpoint(_ context.Context, _ string, endpointID string) error {
	for index, item := range s.endpoints {
		if item.ID == endpointID {
			s.endpoints = append(s.endpoints[:index], s.endpoints[index+1:]...)
			return nil
		}
	}
	return adminsettings.ErrAPIEndpointNotFound
}

type fakeRelayService struct {
	principalID      string
	model            string
	request          relay.ChatCompletionRequest
	channels         []relay.ChannelSummary
	createdBy        string
	created          relay.ChannelMutation
	updatedBy        string
	updatedChannelID string
	updated          relay.ChannelMutation
	statusBy         string
	statusChannelID  string
	status           string
	deletedBy        string
	deletedChannelID string
	discoveredModels []relay.DiscoveredModel
	discovered       relay.ModelDiscoveryRequest
}

type fakeStreamingRelayService struct {
	fakeRelayService
}

func (s *fakeStreamingRelayService) StreamChatCompletions(
	_ context.Context,
	_ *auth.Principal,
	request relay.ChatCompletionRequest,
	emit func(relay.ChatCompletionStreamEvent) error,
) error {
	s.request = request
	if err := emit(relay.ChatCompletionStreamEvent{
		ID: "msg-stream", Model: request.Model, Delta: "hello",
	}); err != nil {
		return err
	}
	return emit(relay.ChatCompletionStreamEvent{
		FinishReason: "end_turn",
		HasUsage:     true,
		Usage:        relay.ChatUsage{PromptTokens: 5, CompletionTokens: 2, TotalTokens: 7},
	})
}

func (s *fakeRelayService) ChatCompletions(
	_ context.Context,
	principal *auth.Principal,
	request relay.ChatCompletionRequest,
) (relay.ChatCompletionResponse, error) {
	s.principalID = principal.ID
	s.model = request.Model
	s.request = request
	return relay.ChatCompletionResponse{
		ID:      "chatcmpl-test",
		Object:  "chat.completion",
		Created: 123,
		Model:   request.Model,
		Choices: []relay.ChatCompletionChoice{
			{
				Index: 0,
				Message: relay.ChatCompletionReply{
					Role:    "assistant",
					Content: "pong",
				},
				FinishReason: "stop",
			},
		},
	}, nil
}

func (s *fakeRelayService) ListChannels(_ context.Context) ([]relay.ChannelSummary, error) {
	return s.channels, nil
}

func (s *fakeRelayService) CreateChannel(
	_ context.Context,
	actorID string,
	request relay.ChannelMutation,
) (relay.ChannelSummary, error) {
	s.createdBy = actorID
	s.created = request
	return safeFakeChannel(request), nil
}

func (s *fakeRelayService) UpdateChannel(
	_ context.Context,
	actorID string,
	channelID string,
	request relay.ChannelMutation,
) (relay.ChannelSummary, error) {
	s.updatedBy = actorID
	s.updatedChannelID = channelID
	s.updated = request
	channel := safeFakeChannel(request)
	channel.ID = channelID
	return channel, nil
}

func (s *fakeRelayService) SetChannelStatus(
	_ context.Context,
	actorID string,
	channelID string,
	status string,
) (relay.ChannelSummary, error) {
	s.statusBy = actorID
	s.statusChannelID = channelID
	s.status = status
	return relay.ChannelSummary{
		ID:                channelID,
		Name:              "channel",
		Provider:          "openai",
		Status:            status,
		CredentialRef:     "secret:stored",
		CredentialMode:    "secret",
		CredentialPreview: "sk-liv...cret",
		HasCredential:     true,
	}, nil
}

func (s *fakeRelayService) DeleteChannel(_ context.Context, actorID string, channelID string) error {
	s.deletedBy = actorID
	s.deletedChannelID = channelID
	return nil
}

func (s *fakeRelayService) DiscoverModels(_ context.Context, request relay.ModelDiscoveryRequest) ([]relay.DiscoveredModel, error) {
	s.discovered = request
	return s.discoveredModels, nil
}

func safeFakeChannel(request relay.ChannelMutation) relay.ChannelSummary {
	return relay.ChannelSummary{
		ID:                "channel-1",
		Name:              request.Name,
		Provider:          request.Provider,
		BaseURL:           request.BaseURL,
		CredentialRef:     "secret:stored",
		CredentialMode:    "secret",
		CredentialPreview: "sk-liv...cret",
		HasCredential:     true,
		Status:            request.Status,
		Priority:          request.Priority,
		Weight:            request.Weight,
	}
}

type fakeBillingAdminService struct{}

func (fakeBillingAdminService) ListUsageRecords(context.Context, billing.UsageQuery) (billing.UsageReport, error) {
	return billing.UsageReport{Records: []billing.UsageRecord{}, Summary: billing.UsageSummary{}, Limit: 50}, nil
}

func (fakeBillingAdminService) ListFinanceReport(context.Context, billing.FinanceQuery) (billing.FinanceReport, error) {
	return billing.FinanceReport{Summaries: []billing.FinanceCurrencySummary{}, Accounts: []billing.FinanceAccount{}, Transactions: []billing.FinanceTransaction{}, Limit: 50}, nil
}

func (fakeBillingAdminService) ListPriceMatrix(context.Context) ([]billing.PriceMatrixSummary, error) {
	return []billing.PriceMatrixSummary{{
		ModelID:                     "model-1",
		Provider:                    "openai",
		Model:                       "gpt-5",
		Currency:                    "USD",
		InputPricePerMillionTokens:  "1.25",
		OutputPricePerMillionTokens: "10",
		Source:                      "litellm",
	}}, nil
}

func (fakeBillingAdminService) ListPrices(context.Context) ([]billing.PriceVersionSummary, error) {
	return []billing.PriceVersionSummary{}, nil
}

func (fakeBillingAdminService) PublishPrice(context.Context, string, billing.PublishPriceRequest) (billing.PriceVersionSummary, error) {
	return billing.PriceVersionSummary{}, nil
}

func (fakeBillingAdminService) GetPrepaidAccount(context.Context, string, string) (billing.AccountSummary, error) {
	return billing.AccountSummary{}, nil
}

func (fakeBillingAdminService) Credit(context.Context, string, billing.CreditRequest) (billing.AccountSummary, error) {
	return billing.AccountSummary{}, nil
}

type fakeRegistrationService struct {
	request auth.RegistrationRequest
}

func (s *fakeRegistrationService) Register(_ context.Context, request auth.RegistrationRequest) (auth.RegisteredAccount, error) {
	s.request = request
	return auth.RegisteredAccount{UserID: "user-1", TenantID: "tenant-1", ProjectID: "project-1"}, nil
}

type fakeUserAdminService struct {
	status       string
	updated      users.UpdateRequest
	updatedID    string
	platformRole users.PlatformRole
	boundUser    string
	boundRoleIDs []string
}

func (s *fakeUserAdminService) List(context.Context) ([]users.Summary, error) {
	return []users.Summary{{ID: "22222222-2222-4222-8222-222222222222", Email: "user@example.com", Status: "active"}}, nil
}

func (s *fakeUserAdminService) ListTenants(context.Context) ([]users.TenantSummary, error) {
	return []users.TenantSummary{{ID: "33333333-3333-4333-8333-333333333333", Name: "Acme", Slug: "acme"}}, nil
}

func (s *fakeUserAdminService) Update(_ context.Context, _ string, userID string, request users.UpdateRequest) (users.Summary, error) {
	s.updatedID = userID
	s.updated = request
	return users.Summary{ID: userID, Email: request.Email, DisplayName: request.DisplayName, Status: "active"}, nil
}

func (s *fakeUserAdminService) SetStatus(_ context.Context, _ string, userID, status string) (users.Summary, error) {
	s.status = status
	return users.Summary{ID: userID, Email: "user@example.com", Status: status}, nil
}

func (s *fakeUserAdminService) ListPlatformPermissions(context.Context) ([]users.PlatformPermission, error) {
	return []users.PlatformPermission{{ID: "44444444-4444-4444-8444-444444444444", Resource: "channel", Action: "read", Name: "channel:read"}}, nil
}

func (s *fakeUserAdminService) ListPlatformRoles(context.Context) ([]users.PlatformRole, error) {
	if s.platformRole.ID == "" {
		s.platformRole = users.PlatformRole{ID: "33333333-3333-4333-8333-333333333333", Code: "platform_owner", Name: "Platform Owner", Status: "active", Permissions: []string{"channel:read"}, CreatedAt: time.Now()}
	}
	return []users.PlatformRole{s.platformRole}, nil
}

func (s *fakeUserAdminService) CreatePlatformRole(_ context.Context, _ string, request users.PlatformRoleMutation) (users.PlatformRole, error) {
	s.platformRole = users.PlatformRole{ID: "55555555-5555-4555-8555-555555555555", Code: request.Code, Name: request.Name, Status: request.Status, Permissions: request.Permissions, CreatedAt: time.Now()}
	return s.platformRole, nil
}

func (s *fakeUserAdminService) UpdatePlatformRole(_ context.Context, _ string, roleID string, request users.PlatformRoleMutation) (users.PlatformRole, error) {
	s.platformRole.ID = roleID
	s.platformRole.Code, s.platformRole.Name, s.platformRole.Status, s.platformRole.Permissions = request.Code, request.Name, request.Status, request.Permissions
	return s.platformRole, nil
}

func (s *fakeUserAdminService) DisablePlatformRole(context.Context, string, string) error { return nil }

func (s *fakeUserAdminService) GetPlatformUserRoles(context.Context, string) ([]users.PlatformRole, error) {
	return []users.PlatformRole{}, nil
}

func (s *fakeUserAdminService) SetPlatformUserRoles(_ context.Context, _ string, userID string, roleIDs []string) ([]users.PlatformRole, error) {
	s.boundUser, s.boundRoleIDs = userID, roleIDs
	return []users.PlatformRole{{ID: roleIDs[0], Code: "ops_admin", Name: "Operations Admin", Status: "active", CreatedAt: time.Now()}}, nil
}

type fakeProfileUserService struct {
	fakeUserAdminService
	profile struct {
		displayName string
		email       string
	}
	passwordChanged bool
}

func (s *fakeProfileUserService) GetProfile(_ context.Context, userID string) (users.Profile, error) {
	if s.profile.email == "" {
		s.profile.email = "user@example.com"
	}
	return users.Profile{
		ID:          userID,
		Email:       s.profile.email,
		DisplayName: s.profile.displayName,
		Status:      "active",
		CreatedAt:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}, nil
}

func (s *fakeProfileUserService) UpdateProfile(_ context.Context, userID, displayName string) (users.Profile, error) {
	s.profile.displayName = displayName
	return s.GetProfile(context.Background(), userID)
}

func (s *fakeProfileUserService) ChangeEmail(_ context.Context, userID, _, email string) (users.Profile, error) {
	s.profile.email = email
	return s.GetProfile(context.Background(), userID)
}

func (s *fakeProfileUserService) ChangePassword(context.Context, string, string, string) error {
	s.passwordChanged = true
	return nil
}

type fakeMFASettingsService struct {
	enabled bool
}

func (s *fakeMFASettingsService) Begin(context.Context, string, string, string) (mfa.Enrollment, error) {
	return mfa.Enrollment{}, nil
}

func (s *fakeMFASettingsService) Confirm(context.Context, string, string, string) error {
	s.enabled = true
	return nil
}

func (s *fakeMFASettingsService) Status(context.Context, string) (mfa.Status, error) {
	return mfa.Status{Enabled: s.enabled}, nil
}

func (s *fakeMFASettingsService) Disable(context.Context, string, string) error {
	if !s.enabled {
		return mfa.ErrMFANotEnabled
	}
	s.enabled = false
	return nil
}

type fakeTokenAdminService struct {
	updatedBy    string
	createdGroup string
}

type fakeTokenConsoleService struct {
	listTenant        string
	listOwner         string
	createdBy         string
	createdProject    string
	createdGroup      string
	createdSpendLimit string
	createdIPs        []string
	createdDomains    []string
	updated           tokens.UpdateRequest
	status            string
	statusTokenID     string
	statusTenantID    string
	statusOwnerID     string
	deleted           bool
	deletedTokenID    string
	deletedTenantID   string
	deletedOwnerID    string
}

type fakeModelCatalog struct{}

func (fakeModelCatalog) ListPublic(context.Context) ([]models.Summary, error) {
	return []models.Summary{{ID: "model-1", Name: "gpt-5", DisplayName: "gpt-5", Provider: "openai", Available: true}}, nil
}

type fakePriceSyncService struct {
	called int
}

type fakeStepUpVerifier struct {
	calls int
	err   error
}

func (s *fakeStepUpVerifier) Verify(context.Context, string, string) error {
	s.calls++
	return s.err
}

func (s *fakePriceSyncService) Sync(context.Context) (modelprices.SyncResult, error) {
	s.called++
	return modelprices.SyncResult{ModelsSeen: 2, ModelsMatched: 1, ModelsUpdated: 1}, nil
}

func (s *fakeTokenConsoleService) ListOwned(_ context.Context, tenantID, createdBy string) ([]tokens.Summary, error) {
	s.listTenant = tenantID
	s.listOwner = createdBy
	return []tokens.Summary{{ID: "token-1", TenantID: tenantID, CreatedBy: createdBy, Status: "active"}}, nil
}

func (s *fakeTokenConsoleService) Create(_ context.Context, request tokens.CreateRequest) (tokens.IssuedToken, error) {
	s.createdBy = request.CreatedBy
	s.createdProject = request.ProjectID
	s.createdGroup = request.GroupID
	s.createdSpendLimit = request.SpendLimit
	s.createdIPs = request.AllowedIPs
	s.createdDomains = request.AllowedDomains
	return tokens.IssuedToken{ID: "token-1", Plaintext: "sk-test-once", Prefix: "sk-test"}, nil
}

func (s *fakeTokenConsoleService) UpdateOwned(_ context.Context, request tokens.UpdateRequest) (tokens.Summary, error) {
	s.updated = request
	return tokens.Summary{ID: "token-1", Status: "active"}, nil
}

func (s *fakeTokenConsoleService) SetStatusOwned(_ context.Context, tokenID, tenantID, ownerID, status string) error {
	s.status = status
	s.statusTokenID = tokenID
	s.statusTenantID = tenantID
	s.statusOwnerID = ownerID
	return nil
}

func (s *fakeTokenConsoleService) DeleteOwned(_ context.Context, tokenID, tenantID, ownerID string) error {
	s.deleted = true
	s.deletedTokenID = tokenID
	s.deletedTenantID = tenantID
	s.deletedOwnerID = ownerID
	return nil
}

func (s *fakeTokenAdminService) List(context.Context) ([]tokens.Summary, error) {
	return []tokens.Summary{}, nil
}

func (s *fakeTokenAdminService) SetGroup(_ context.Context, _ string, groupID string) (tokens.Summary, error) {
	s.updatedBy = groupID
	return tokens.Summary{ID: "token-1", GroupID: groupID, GroupCode: "standard", Status: "active"}, nil
}

func (s *fakeTokenAdminService) Create(_ context.Context, request tokens.CreateRequest) (tokens.IssuedToken, error) {
	s.createdGroup = request.GroupID
	return tokens.IssuedToken{ID: "token-1", Plaintext: "sk-test-once", Prefix: "sk-test"}, nil
}

type fakeGroupService struct {
	createdBy string
}

type fakeModelMonitorGroupService struct {
	fakeGroupService
	createdBy string
	item      groups.ModelMonitor
}

func (s *fakeGroupService) List(context.Context) ([]groups.Summary, error) {
	return []groups.Summary{}, nil
}

func (s *fakeModelMonitorGroupService) ListAdminModelMonitors(context.Context) ([]groups.ModelMonitor, error) {
	if s.item.ID == "" {
		s.item = groups.ModelMonitor{
			ID: "monitor-1", GroupID: "group-1", GroupCode: "standard", GroupName: "Standard",
			Name: "Primary", SelectionMode: groups.MonitorSelectionAll, Mode: groups.MonitorModePassive,
			ProbeIntervalSeconds: 300, Enabled: true, ModelNames: []string{"gpt-5"},
		}
	}
	return []groups.ModelMonitor{s.item}, nil
}

func (s *fakeModelMonitorGroupService) CreateAdminModelMonitor(_ context.Context, actorID string, request groups.ModelMonitorMutation) (groups.ModelMonitor, error) {
	s.createdBy = actorID
	s.item = groups.ModelMonitor{
		ID: "monitor-1", GroupID: request.GroupID, GroupCode: "standard", GroupName: "Standard",
		Name: request.Name, SelectionMode: request.SelectionMode, Mode: request.Mode,
		ProbeIntervalSeconds: request.ProbeIntervalSeconds, Enabled: request.Enabled,
		ModelNames: request.ModelNames,
	}
	return s.item, nil
}

func (s *fakeModelMonitorGroupService) UpdateAdminModelMonitor(context.Context, string, string, groups.ModelMonitorMutation) (groups.ModelMonitor, error) {
	return s.item, nil
}

func (*fakeModelMonitorGroupService) DeleteAdminModelMonitor(context.Context, string, string) error {
	return nil
}

func (*fakeModelMonitorGroupService) ClaimDueActiveModelMonitor(context.Context) (*groups.ModelMonitor, error) {
	return nil, nil
}

func (s *fakeModelMonitorGroupService) ClaimActiveModelMonitor(context.Context, string) (*groups.ModelMonitor, error) {
	if s.item.ID == "" {
		return nil, groups.ErrMonitorNotFound
	}
	return &s.item, nil
}

func (*fakeModelMonitorGroupService) CompleteActiveModelMonitor(context.Context, string, string, string) error {
	return nil
}

func (*fakeGroupService) ListModelStatuses(context.Context, string) (groups.ModelStatusReport, error) {
	return groups.ModelStatusReport{
		UpdatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Groups: []groups.ModelStatusGroup{{
			ID: "group-1", Code: "standard", Name: "Standard", Status: "normal", GroupStatus: "active",
			Multiplier: "1.000000", BillingType: "prepaid", Models: []groups.ModelStatus{{
				Model: "gpt-5", Provider: "openai", Status: "normal", TotalRoutes: 1, AvailableRoutes: 1,
			}},
		}},
	}, nil
}

func (*fakeGroupService) ListAdminModelStatuses(context.Context) (groups.ModelStatusReport, error) {
	return groups.ModelStatusReport{
		UpdatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Groups: []groups.ModelStatusGroup{{
			ID: "group-1", Code: "standard", Name: "Standard", Status: "normal", GroupStatus: "active",
			Multiplier: "1.000000", BillingType: "prepaid", Models: []groups.ModelStatus{{
				Model: "gpt-5", Provider: "openai", Status: "normal", TotalRoutes: 1, AvailableRoutes: 1,
			}},
		}},
	}, nil
}

func (*fakeGroupService) ListTokenGroups(context.Context) ([]groups.TokenGroupSummary, error) {
	return []groups.TokenGroupSummary{{ID: "group-1", Code: "standard", Name: "Standard", Multiplier: "1.000000", BillingType: "prepaid", Status: "active", Models: []string{"gpt-5"}}}, nil
}

func (s *fakeGroupService) Create(_ context.Context, actorID string, request groups.Mutation) (groups.Summary, error) {
	s.createdBy = actorID
	return groups.Summary{
		ID:          "group-1",
		Code:        request.Code,
		Name:        request.Name,
		Multiplier:  request.Multiplier,
		BillingType: request.BillingType,
		Status:      request.Status,
	}, nil
}

func (*fakeGroupService) Update(context.Context, string, string, groups.Mutation) (groups.Summary, error) {
	return groups.Summary{}, nil
}

func (*fakeGroupService) Delete(context.Context, string, string) error {
	return nil
}
