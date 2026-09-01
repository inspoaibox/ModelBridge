package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"ai-token/internal/adminsettings"
	"ai-token/internal/audit"
	"ai-token/internal/auth"
	"ai-token/internal/billing"
	"ai-token/internal/groups"
	"ai-token/internal/ids"
	"ai-token/internal/mfa"
	"ai-token/internal/modelprices"
	"ai-token/internal/models"
	"ai-token/internal/relay"
	"ai-token/internal/tokens"
	"ai-token/internal/users"
)

func New(authMiddleware *auth.Middleware, services *auth.Services, secureCookies bool, webDir string) http.Handler {
	return NewWithRelay(authMiddleware, services, nil, secureCookies, webDir)
}

func NewWithRelay(
	authMiddleware *auth.Middleware,
	services *auth.Services,
	relayService relay.ChatCompletionService,
	secureCookies bool,
	webDir string,
	billers ...billing.AdminService,
) http.Handler {
	return NewWithRelayAndGroupsAndTokens(authMiddleware, services, relayService, secureCookies, webDir, nil, nil, billers...)
}

func NewWithRelayAndGroups(
	authMiddleware *auth.Middleware,
	services *auth.Services,
	relayService relay.ChatCompletionService,
	secureCookies bool,
	webDir string,
	groupService groups.Service,
	billers ...billing.AdminService,
) http.Handler {
	return NewWithRelayAndGroupsAndTokens(authMiddleware, services, relayService, secureCookies, webDir, nil, groupService, billers...)
}

func NewWithRelayAndGroupsAndTokens(
	authMiddleware *auth.Middleware,
	services *auth.Services,
	relayService relay.ChatCompletionService,
	secureCookies bool,
	webDir string,
	tokenService tokens.AdminService,
	groupService groups.Service,
	billers ...billing.AdminService,
) http.Handler {
	return NewWithRelayAndGroupsAndTokensAndConsoleTokens(
		authMiddleware,
		services,
		relayService,
		secureCookies,
		webDir,
		tokenService,
		groupService,
		nil,
		billers...,
	)
}

func NewWithRelayAndGroupsAndTokensAndConsoleTokens(
	authMiddleware *auth.Middleware,
	services *auth.Services,
	relayService relay.ChatCompletionService,
	secureCookies bool,
	webDir string,
	tokenService tokens.AdminService,
	groupService groups.Service,
	consoleTokenService tokens.ConsoleService,
	billers ...billing.AdminService,
) http.Handler {
	return NewWithRelayAndGroupsAndTokensAndConsoleTokensAndModelCatalog(
		authMiddleware,
		services,
		relayService,
		secureCookies,
		webDir,
		tokenService,
		groupService,
		consoleTokenService,
		nil,
		billers...,
	)
}

func NewWithRelayAndGroupsAndTokensAndConsoleTokensAndModelCatalog(
	authMiddleware *auth.Middleware,
	services *auth.Services,
	relayService relay.ChatCompletionService,
	secureCookies bool,
	webDir string,
	tokenService tokens.AdminService,
	groupService groups.Service,
	consoleTokenService tokens.ConsoleService,
	modelCatalog models.Catalog,
	billers ...billing.AdminService,
) http.Handler {
	return NewWithRelayAndGroupsAndTokensAndConsoleTokensAndModelCatalogAndUsers(
		authMiddleware, services, relayService, secureCookies, webDir,
		tokenService, groupService, consoleTokenService, modelCatalog, nil, billers...,
	)
}

func NewWithRelayAndGroupsAndTokensAndConsoleTokensAndModelCatalogAndUsers(
	authMiddleware *auth.Middleware,
	services *auth.Services,
	relayService relay.ChatCompletionService,
	secureCookies bool,
	webDir string,
	tokenService tokens.AdminService,
	groupService groups.Service,
	consoleTokenService tokens.ConsoleService,
	modelCatalog models.Catalog,
	userService users.AdminService,
	billers ...billing.AdminService,
) http.Handler {
	return NewWithRelayAndGroupsAndTokensAndConsoleTokensAndModelCatalogAndUsersAndPriceSync(
		authMiddleware, services, relayService, secureCookies, webDir,
		tokenService, groupService, consoleTokenService, modelCatalog, userService, nil, billers...,
	)
}

func NewWithRelayAndGroupsAndTokensAndConsoleTokensAndModelCatalogAndUsersAndPriceSync(
	authMiddleware *auth.Middleware,
	services *auth.Services,
	relayService relay.ChatCompletionService,
	secureCookies bool,
	webDir string,
	tokenService tokens.AdminService,
	groupService groups.Service,
	consoleTokenService tokens.ConsoleService,
	modelCatalog models.Catalog,
	userService users.AdminService,
	priceSyncService modelprices.SyncService,
	billers ...billing.AdminService,
) http.Handler {
	return newHandler(authMiddleware, services, relayService, secureCookies, webDir, tokenService, groupService, consoleTokenService, modelCatalog, userService, priceSyncService, nil, billers...)
}

func NewWithRelayAndGroupsAndTokensAndConsoleTokensAndModelCatalogAndUsersAndPriceSyncAndAudit(
	authMiddleware *auth.Middleware,
	services *auth.Services,
	relayService relay.ChatCompletionService,
	secureCookies bool,
	webDir string,
	tokenService tokens.AdminService,
	groupService groups.Service,
	consoleTokenService tokens.ConsoleService,
	modelCatalog models.Catalog,
	userService users.AdminService,
	priceSyncService modelprices.SyncService,
	auditReader audit.Reader,
	billers ...billing.AdminService,
) http.Handler {
	return newHandler(authMiddleware, services, relayService, secureCookies, webDir, tokenService, groupService, consoleTokenService, modelCatalog, userService, priceSyncService, auditReader, billers...)
}

func newHandler(
	authMiddleware *auth.Middleware,
	services *auth.Services,
	relayService relay.ChatCompletionService,
	secureCookies bool,
	webDir string,
	tokenService tokens.AdminService,
	groupService groups.Service,
	consoleTokenService tokens.ConsoleService,
	modelCatalog models.Catalog,
	userService users.AdminService,
	priceSyncService modelprices.SyncService,
	auditReader audit.Reader,
	billers ...billing.AdminService,
) http.Handler {
	if authMiddleware == nil {
		authMiddleware = auth.NewMiddleware(nil)
	}
	if services == nil {
		services = &auth.Services{}
	}
	if webDir == "" {
		webDir = "web"
	}
	webDir = resolveWebDir(webDir)
	mux := http.NewServeMux()
	var billingService billing.AdminService
	if len(billers) > 0 {
		billingService = billers[0]
	}
	protectStepUp := func(handler http.Handler, permissions ...string) http.Handler {
		// Sensitive routes fail closed when the application forgot to wire a
		// verifier. Tests and alternate embeddings must opt into the same
		// contract by supplying a verifier explicitly.
		return authMiddleware.Protect(auth.AudienceAdmin, permissions...)(auth.RequireStepUp(services.StepUpMFA)(handler))
	}

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.Handle("GET /public/v1/models", modelListHandler(modelCatalog))
	mux.HandleFunc("GET /public/v1/settings", publicSystemSettingsHandler(services.SecuritySettings))
	mux.HandleFunc("GET /public/v1/features", publicFeatureSettingsHandler(services.SecuritySettings))

	mux.HandleFunc("POST /admin/v1/auth/login", loginHandler(services.Login, auth.AudienceAdmin, secureCookies))
	mux.HandleFunc("POST /console/v1/auth/login", loginHandler(services.Login, auth.AudienceConsole, secureCookies))
	mux.HandleFunc("POST /console/v1/auth/register", registrationHandler(services.Registration))
	mux.HandleFunc("POST /console/v1/auth/email/verify", emailVerificationHandler(services.Registration))
	mux.HandleFunc("POST /console/v1/auth/email/resend", emailVerificationResendHandler(services.Registration))
	mux.HandleFunc("POST /admin/v1/auth/password-reset/request", passwordResetRequestHandler(
		services.PasswordReset,
		services.PasswordResetNotifier,
	))
	mux.HandleFunc("POST /console/v1/auth/password-reset/request", passwordResetRequestHandler(
		services.PasswordReset,
		services.PasswordResetNotifier,
	))
	mux.HandleFunc("POST /admin/v1/auth/password-reset/confirm", passwordResetConfirmHandler(services.PasswordReset))
	mux.HandleFunc("POST /console/v1/auth/password-reset/confirm", passwordResetConfirmHandler(services.PasswordReset))

	mux.Handle("POST /admin/v1/auth/logout", authMiddleware.Protect(
		auth.AudienceAdmin,
	)(logoutHandler(services.Login, auth.AudienceAdmin, secureCookies)))

	mux.Handle("POST /console/v1/auth/logout", authMiddleware.Protect(
		auth.AudienceConsole,
	)(logoutHandler(services.Login, auth.AudienceConsole, secureCookies)))

	mux.Handle("POST /admin/v1/auth/mfa/enroll", authMiddleware.Protect(
		auth.AudienceAdmin,
	)(mfaEnrollmentBeginHandler(services.MFA)))

	mux.Handle("POST /admin/v1/auth/mfa/enroll/{enrollmentID}/confirm", authMiddleware.Protect(
		auth.AudienceAdmin,
	)(mfaEnrollmentConfirmHandler(services.MFA)))

	mux.Handle("GET /admin/v1/me", authMiddleware.Protect(
		auth.AudienceAdmin,
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, _ := auth.PrincipalFromContext(r.Context())
		permissions := make([]string, 0, len(principal.Permissions))
		for permission := range principal.Permissions {
			permissions = append(permissions, permission)
		}
		sort.Strings(permissions)
		writeJSON(w, http.StatusOK, map[string]any{
			"id":          principal.ID,
			"type":        principal.Type,
			"audience":    principal.Audience,
			"roles":       principal.Roles,
			"permissions": permissions,
		})
	})))

	mux.Handle("GET /admin/v1/profile", authMiddleware.Protect(
		auth.AudienceAdmin,
	)(consoleProfileHandler(userService)))

	mux.Handle("PUT /admin/v1/profile", authMiddleware.Protect(
		auth.AudienceAdmin,
	)(consoleProfileUpdateHandler(userService)))

	mux.Handle("PUT /admin/v1/profile/email", authMiddleware.Protect(
		auth.AudienceAdmin,
	)(consoleProfileEmailHandler(userService)))

	mux.Handle("PUT /admin/v1/profile/password", authMiddleware.Protect(
		auth.AudienceAdmin,
	)(consoleProfilePasswordHandler(userService)))

	mux.Handle("GET /admin/v1/auth/mfa/status", authMiddleware.Protect(
		auth.AudienceAdmin,
	)(mfaStatusHandler(services.MFA)))

	mux.Handle("POST /admin/v1/auth/mfa/disable", authMiddleware.Protect(
		auth.AudienceAdmin,
	)(adminMFADisableHandler(services.MFA, services.SecuritySettings)))

	mux.Handle("GET /console/v1/me", authMiddleware.Protect(
		auth.AudienceConsole,
	)(consoleProfileHandler(userService)))

	mux.Handle("GET /console/v1/profile", authMiddleware.Protect(
		auth.AudienceConsole,
	)(consoleProfileHandler(userService)))

	mux.Handle("PUT /console/v1/profile", authMiddleware.Protect(
		auth.AudienceConsole,
	)(consoleProfileUpdateHandler(userService)))

	mux.Handle("PUT /console/v1/profile/email", authMiddleware.Protect(
		auth.AudienceConsole,
	)(consoleProfileEmailHandler(userService)))

	mux.Handle("PUT /console/v1/profile/password", authMiddleware.Protect(
		auth.AudienceConsole,
	)(consoleProfilePasswordHandler(userService)))

	mux.Handle("GET /console/v1/profile/mfa", authMiddleware.Protect(
		auth.AudienceConsole,
	)(mfaStatusHandler(services.MFA)))

	mux.Handle("POST /console/v1/profile/mfa/enroll", authMiddleware.Protect(
		auth.AudienceConsole,
	)(mfaEnrollmentBeginHandler(services.MFA)))

	mux.Handle("POST /console/v1/profile/mfa/enroll/{enrollmentID}/confirm", authMiddleware.Protect(
		auth.AudienceConsole,
	)(mfaEnrollmentConfirmHandler(services.MFA)))

	mux.Handle("POST /console/v1/profile/mfa/disable", authMiddleware.Protect(
		auth.AudienceConsole,
	)(mfaDisableHandler(services.MFA)))

	mux.Handle("GET /admin/v1/security/settings", authMiddleware.Protect(
		auth.AudienceAdmin,
		"security:read",
	)(securitySettingsReadHandler(services.SecuritySettings)))

	mux.Handle("PUT /admin/v1/security/settings", protectStepUp(securitySettingsUpdateHandler(services.SecuritySettings), "security:update"))

	mux.Handle("GET /admin/v1/settings", authMiddleware.Protect(
		auth.AudienceAdmin,
		"security:read",
	)(systemSettingsReadHandler(services.SecuritySettings)))

	mux.Handle("PUT /admin/v1/settings", protectStepUp(systemSettingsUpdateHandler(services.SecuritySettings), "security:update"))

	mux.Handle("PUT /admin/v1/settings/site", protectStepUp(siteSettingsUpdateHandler(services.SecuritySettings), "security:update"))
	mux.Handle("GET /admin/v1/settings/email", authMiddleware.Protect(
		auth.AudienceAdmin,
		"security:read",
	)(emailSettingsReadHandler(services.SecuritySettings)))
	mux.Handle("PUT /admin/v1/settings/email", protectStepUp(emailSettingsUpdateHandler(services.SecuritySettings), "security:update"))
	mux.Handle("POST /admin/v1/settings/email/test-connection", protectStepUp(emailSMTPConnectionTestHandler(services.SecuritySettings), "security:update"))
	mux.Handle("POST /admin/v1/settings/email/test-message", protectStepUp(emailTestMessageHandler(services.SecuritySettings), "security:update"))
	mux.Handle("GET /admin/v1/settings/email/templates", authMiddleware.Protect(
		auth.AudienceAdmin,
		"security:read",
	)(emailTemplateListHandler(services.SecuritySettings)))
	mux.Handle("POST /admin/v1/settings/email/templates", protectStepUp(emailTemplateCreateHandler(services.SecuritySettings), "security:update"))
	mux.Handle("PUT /admin/v1/settings/email/templates/{templateID}", protectStepUp(emailTemplateUpdateHandler(services.SecuritySettings), "security:update"))
	mux.Handle("DELETE /admin/v1/settings/email/templates/{templateID}", protectStepUp(emailTemplateDeleteHandler(services.SecuritySettings), "security:update"))
	mux.Handle("GET /admin/v1/settings/features", authMiddleware.Protect(
		auth.AudienceAdmin,
		"security:read",
	)(featureSettingsReadHandler(services.SecuritySettings)))
	mux.Handle("PUT /admin/v1/settings/features", protectStepUp(featureSettingsUpdateHandler(services.SecuritySettings), "security:update"))

	mux.Handle("GET /admin/v1/channels", authMiddleware.Protect(
		auth.AudienceAdmin,
		"channel:read",
	)(relayChannelsHandler(relayService)))

	mux.Handle("POST /admin/v1/channels/discover-models", protectStepUp(relayChannelModelDiscoveryHandler(relayService), "channel:update"))

	mux.Handle("POST /admin/v1/channels", protectStepUp(relayChannelCreateHandler(relayService), "channel:update"))

	mux.Handle("PUT /admin/v1/channels/{channelID}", protectStepUp(relayChannelUpdateHandler(relayService), "channel:update"))

	mux.Handle("POST /admin/v1/channels/{channelID}/pause", protectStepUp(relayChannelStatusHandler(relayService, "disabled"), "channel:update"))

	mux.Handle("POST /admin/v1/channels/{channelID}/enable", protectStepUp(relayChannelStatusHandler(relayService, "active"), "channel:update"))

	mux.Handle("DELETE /admin/v1/channels/{channelID}", protectStepUp(relayChannelDeleteHandler(relayService), "channel:update"))

	mux.Handle("GET /admin/v1/groups", authMiddleware.Protect(
		auth.AudienceAdmin,
		"group:read",
	)(groupListHandler(groupService)))

	mux.Handle("POST /admin/v1/groups", protectStepUp(groupCreateHandler(groupService), "group:update"))

	mux.Handle("PUT /admin/v1/groups/{groupID}", protectStepUp(groupUpdateHandler(groupService), "group:update"))

	mux.Handle("DELETE /admin/v1/groups/{groupID}", protectStepUp(groupDeleteHandler(groupService), "group:update"))

	mux.Handle("GET /admin/v1/tokens", authMiddleware.Protect(
		auth.AudienceAdmin,
		"token:read",
	)(tokenListHandler(tokenService)))

	mux.Handle("PUT /admin/v1/tokens/{tokenID}/group", protectStepUp(tokenGroupUpdateHandler(tokenService), "token:update"))

	mux.Handle("POST /admin/v1/tokens", authMiddleware.Protect(
		auth.AudienceAdmin,
		"token:create",
	)(adminTokenCreationDisabledHandler()))

	mux.Handle("DELETE /admin/v1/tokens/{tokenID}", protectStepUp(tokenRevokeHandler(tokenService), "token:revoke"))

	// Tenant members and projects are managed by the tenant owner/admin. The
	// tenant path guard prevents a valid console session from crossing tenants;
	// the service repeats the role and ownership checks for every mutation.
	if tenantService, ok := userService.(users.TenantService); ok && tenantService != nil {
		mux.Handle("GET /console/v1/tenants/{tenantID}/members", authMiddleware.Protect(
			auth.AudienceConsole, "member:invite",
		)(auth.RequireTenantPath("tenantID")(tenantMembersListHandler(tenantService))))
		mux.Handle("POST /console/v1/tenants/{tenantID}/members", authMiddleware.Protect(
			auth.AudienceConsole, "member:invite",
		)(auth.RequireTenantPath("tenantID")(tenantMemberAddHandler(tenantService))))
		mux.Handle("PUT /console/v1/tenants/{tenantID}/members/{userID}", authMiddleware.Protect(
			auth.AudienceConsole, "member:invite",
		)(auth.RequireTenantPath("tenantID")(tenantMemberUpdateHandler(tenantService))))
		mux.Handle("DELETE /console/v1/tenants/{tenantID}/members/{userID}", authMiddleware.Protect(
			auth.AudienceConsole, "member:remove",
		)(auth.RequireTenantPath("tenantID")(tenantMemberRemoveHandler(tenantService))))
		mux.Handle("GET /console/v1/tenants/{tenantID}/projects", authMiddleware.Protect(
			auth.AudienceConsole, "project:read",
		)(auth.RequireTenantPath("tenantID")(tenantProjectsListHandler(tenantService))))
		mux.Handle("POST /console/v1/tenants/{tenantID}/projects", authMiddleware.Protect(
			auth.AudienceConsole, "project:update",
		)(auth.RequireTenantPath("tenantID")(tenantProjectCreateHandler(tenantService))))
		mux.Handle("PUT /console/v1/tenants/{tenantID}/projects/{projectID}", authMiddleware.Protect(
			auth.AudienceConsole, "project:update",
		)(auth.RequireTenantPath("tenantID")(tenantProjectUpdateHandler(tenantService))))
		mux.Handle("DELETE /console/v1/tenants/{tenantID}/projects/{projectID}", authMiddleware.Protect(
			auth.AudienceConsole, "project:update",
		)(auth.RequireTenantPath("tenantID")(tenantProjectDeleteHandler(tenantService))))
		mux.Handle("GET /console/v1/tenants/{tenantID}/projects/{projectID}/members", authMiddleware.Protect(
			auth.AudienceConsole, "project:update",
		)(auth.RequireTenantPath("tenantID")(tenantProjectMembersListHandler(tenantService))))
		mux.Handle("POST /console/v1/tenants/{tenantID}/projects/{projectID}/members", authMiddleware.Protect(
			auth.AudienceConsole, "project:update",
		)(auth.RequireTenantPath("tenantID")(tenantProjectMemberAddHandler(tenantService))))
		mux.Handle("PUT /console/v1/tenants/{tenantID}/projects/{projectID}/members/{userID}", authMiddleware.Protect(
			auth.AudienceConsole, "project:update",
		)(auth.RequireTenantPath("tenantID")(tenantProjectMemberUpdateHandler(tenantService))))
		mux.Handle("DELETE /console/v1/tenants/{tenantID}/projects/{projectID}/members/{userID}", authMiddleware.Protect(
			auth.AudienceConsole, "project:update",
		)(auth.RequireTenantPath("tenantID")(tenantProjectMemberRemoveHandler(tenantService))))
	}

	mux.Handle("GET /admin/v1/users", authMiddleware.Protect(
		auth.AudienceAdmin,
		"user:read",
	)(userListHandler(userService)))

	mux.Handle("GET /admin/v1/tenants", authMiddleware.Protect(
		auth.AudienceAdmin,
		"user:read",
	)(userTenantListHandler(userService)))

	mux.Handle("POST /admin/v1/users", authMiddleware.Protect(
		auth.AudienceAdmin,
		"user:create",
	)(adminUserCreationDisabledHandler()))

	mux.Handle("PUT /admin/v1/users/{userID}", protectStepUp(userUpdateHandler(userService), "user:update"))

	mux.Handle("PUT /admin/v1/users/{userID}/status", protectStepUp(userStatusUpdateHandler(userService), "user:update"))

	// Platform role definitions and administrator bindings are separate from
	// customer registration. The service additionally requires platform_owner,
	// so a role permission alone cannot grant privilege-management authority.
	if roleService, ok := userService.(users.PlatformRoleService); ok && roleService != nil {
		mux.Handle("GET /admin/v1/roles", authMiddleware.Protect(
			auth.AudienceAdmin, "role:read",
		)(platformRolesHandler(roleService)))
		mux.Handle("GET /admin/v1/permissions", authMiddleware.Protect(
			auth.AudienceAdmin, "role:read",
		)(platformPermissionsHandler(roleService)))
		mux.Handle("POST /admin/v1/roles", protectStepUp(platformRoleCreateHandler(roleService), "role:update"))
		mux.Handle("PUT /admin/v1/roles/{roleID}", protectStepUp(platformRoleUpdateHandler(roleService), "role:update"))
		mux.Handle("POST /admin/v1/roles/{roleID}/disable", protectStepUp(platformRoleDisableHandler(roleService), "role:update"))
		mux.Handle("GET /admin/v1/users/{userID}/roles", authMiddleware.Protect(
			auth.AudienceAdmin, "role:read",
		)(platformUserRolesHandler(roleService)))
		mux.Handle("PUT /admin/v1/users/{userID}/roles", protectStepUp(platformUserRolesUpdateHandler(roleService), "role:update"))
	}

	mux.Handle("GET /admin/v1/prices", authMiddleware.Protect(
		auth.AudienceAdmin,
		"price:read",
	)(billingPriceListHandler(billingService)))

	mux.Handle("POST /admin/v1/prices/publish", protectStepUp(billingPricePublishHandler(billingService), "price:publish"))

	mux.Handle("POST /admin/v1/prices/sync-official", protectStepUp(officialPriceSyncHandler(priceSyncService), "price:publish"))

	mux.Handle("GET /admin/v1/usage", authMiddleware.Protect(
		auth.AudienceAdmin,
		"usage:read",
	)(usageRecordsHandler(billingService)))

	mux.Handle("GET /admin/v1/overview", authMiddleware.Protect(
		auth.AudienceAdmin,
		"operations:read",
	)(operationsSnapshotHandler(billingService)))
	mux.Handle("GET /admin/v1/model-status", authMiddleware.Protect(
		auth.AudienceAdmin,
		"operations:read",
	)(adminModelStatusHandler(groupService)))
	mux.Handle("GET /admin/v1/ops", authMiddleware.Protect(
		auth.AudienceAdmin,
		"operations:read",
	)(operationsSnapshotHandler(billingService)))

	mux.Handle("GET /admin/v1/audit", authMiddleware.Protect(
		auth.AudienceAdmin,
		"audit:read",
	)(auditRecordsHandler(auditReader)))

	mux.Handle("GET /admin/v1/finance", authMiddleware.Protect(
		auth.AudienceAdmin,
		"finance:read",
	)(financeReportHandler(billingService)))

	mux.Handle("GET /admin/v1/tenants/{tenantID}/billing/account", authMiddleware.Protect(
		auth.AudienceAdmin,
		"billing:read",
	)(billingAccountReadHandler(billingService)))

	mux.Handle("POST /admin/v1/tenants/{tenantID}/billing/credit", protectStepUp(billingAccountCreditHandler(billingService), "billing:update"))

	mux.Handle("POST /admin/v1/usage/{requestID}/settle", protectStepUp(billingSettlementHandler(billingService), "billing:update"))

	mux.Handle("GET /console/v1/tenants/{tenantID}/usage", authMiddleware.Protect(
		auth.AudienceConsole,
		"usage:read",
	)(
		auth.RequireTenantPath("tenantID")(
			consoleUsageHandler(billingService),
		),
	))

	mux.Handle("GET /console/v1/tenants/{tenantID}/billing/account", authMiddleware.Protect(
		auth.AudienceConsole,
		"billing:read",
	)(
		auth.RequireTenantPath("tenantID")(
			billingAccountReadHandler(billingService),
		),
	))

	mux.Handle("GET /console/v1/tenants/{tenantID}/tokens", authMiddleware.Protect(
		auth.AudienceConsole,
		"token:read",
	)(
		auth.RequireTenantPath("tenantID")(
			tokenConsoleListHandler(consoleTokenService),
		),
	))

	mux.Handle("GET /console/v1/tenants/{tenantID}/token-groups", authMiddleware.Protect(
		auth.AudienceConsole,
		"token:create",
	)(
		auth.RequireTenantPath("tenantID")(
			tokenGroupListHandler(groupService),
		),
	))

	mux.Handle("GET /console/v1/tenants/{tenantID}/model-status", authMiddleware.Protect(
		auth.AudienceConsole,
		"model:status:read",
	)(
		auth.RequireTenantPath("tenantID")(
			modelStatusHandler(groupService, services.SecuritySettings),
		),
	))

	mux.Handle("POST /console/v1/tenants/{tenantID}/tokens", authMiddleware.Protect(
		auth.AudienceConsole,
		"token:create",
	)(
		auth.RequireTenantPath("tenantID")(
			tokenConsoleCreateHandler(consoleTokenService),
		),
	))

	mux.Handle("DELETE /console/v1/tenants/{tenantID}/tokens/{tokenID}", authMiddleware.Protect(
		auth.AudienceConsole,
		"token:revoke",
	)(
		auth.RequireTenantPath("tenantID")(
			tokenConsoleRevokeHandler(consoleTokenService),
		),
	))

	mux.Handle("POST /v1/chat/completions", authMiddleware.ProtectScopes(
		auth.AudienceRelay,
		"model:use",
	)(relayChatCompletionsHandler(relayService)))
	mux.Handle("POST /v1/embeddings", authMiddleware.ProtectScopes(
		auth.AudienceRelay,
		"model:use",
	)(relayEmbeddingsHandler(relayService)))
	mux.Handle("GET /v1/models", authMiddleware.ProtectScopes(
		auth.AudienceRelay,
		"model:use",
	)(relayModelListHandler(modelCatalog)))
	mux.Handle("POST /v1/responses", authMiddleware.ProtectScopes(
		auth.AudienceRelay,
		"model:use",
	)(relayResponsesHandler(relayService)))
	mux.Handle("POST /v1/messages", authMiddleware.ProtectScopes(
		auth.AudienceRelay,
		"model:use",
	)(relayAnthropicMessagesHandler(relayService)))
	mux.Handle("POST /v1/images/generations", authMiddleware.ProtectScopes(
		auth.AudienceRelay,
		"model:use",
	)(relayImageGenerationHandler(relayService)))
	mux.Handle("POST /v1/images/edits", authMiddleware.ProtectScopes(
		auth.AudienceRelay,
		"model:use",
	)(relayImageEditHandler(relayService)))
	mux.Handle("POST /v1/audio/transcriptions", authMiddleware.ProtectScopes(
		auth.AudienceRelay,
		"model:use",
	)(relayAudioHandler(relayService, false)))
	mux.Handle("POST /v1/audio/translations", authMiddleware.ProtectScopes(
		auth.AudienceRelay,
		"model:use",
	)(relayAudioHandler(relayService, true)))
	mux.Handle("POST /v1/audio/speech", authMiddleware.ProtectScopes(
		auth.AudienceRelay,
		"model:use",
	)(relaySpeechHandler(relayService)))
	mux.Handle("POST /v1/videos", authMiddleware.ProtectScopes(
		auth.AudienceRelay,
		"model:use",
	)(relayVideoCreateHandler(relayService)))
	mux.Handle("GET /v1/videos/{videoID}", authMiddleware.ProtectScopes(
		auth.AudienceRelay,
		"model:use",
	)(relayVideoGetHandler(relayService)))
	mux.Handle("GET /v1/videos/{videoID}/content", authMiddleware.ProtectScopes(
		auth.AudienceRelay,
		"model:use",
	)(relayVideoContentHandler(relayService)))

	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || strings.HasPrefix(r.URL.Path, "/assets/") || r.URL.Path == "/favicon.ico" {
			frontendHandler(webDir).ServeHTTP(w, r)
			return
		}
		if isAPIPath(r.URL.Path) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "RESOURCE_NOT_FOUND"})
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "RESOURCE_NOT_FOUND"})
	}))
	return withRecovery(withSecurityHeaders(withRequestID(mux)))
}

func isAPIPath(path string) bool {
	return path == "/healthz" || strings.HasPrefix(path, "/admin/") || strings.HasPrefix(path, "/console/") || strings.HasPrefix(path, "/public/") || strings.HasPrefix(path, "/v1/")
}

func frontendHandler(webDir string) http.Handler {
	if strings.TrimSpace(webDir) == "" {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "FRONTEND_UNAVAILABLE"})
		})
	}
	files := http.FileServer(http.Dir(webDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "METHOD_NOT_ALLOWED"})
			return
		}
		relative := filepath.Clean(filepath.FromSlash(strings.TrimPrefix(r.URL.Path, "/")))
		if filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "RESOURCE_NOT_FOUND"})
			return
		}
		if relative != "." {
			if info, err := os.Stat(filepath.Join(webDir, relative)); err == nil && !info.IsDir() {
				files.ServeHTTP(w, r)
				return
			}
		}
		indexPath := filepath.Join(webDir, "index.html")
		if _, err := os.Stat(indexPath); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "RESOURCE_NOT_FOUND"})
			return
		}
		http.ServeFile(w, r, indexPath)
	})
}

func withRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("http panic request_id=%s path=%s", w.Header().Get("X-Request-ID"), r.URL.Path)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "INTERNAL_SERVER_ERROR"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func withSecurityHeaders(next http.Handler) http.Handler {
	allowedOrigins := configuredOrigins()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if isAPIPath(r.URL.Path) {
			// API responses may contain one-time token or TOTP enrollment
			// material. Never allow a browser or intermediary to cache them.
			w.Header().Set("Cache-Control", "no-store")
		}
		// Brand assets may be hosted on an administrator-approved HTTPS CDN;
		// keep every executable resource and connection same-origin.
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' https:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin != "" && originAllowed(r, origin, allowedOrigins) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, X-API-Key, X-MFA-Code")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			if origin == "" || !originAllowed(r, origin, allowedOrigins) {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "ORIGIN_NOT_ALLOWED"})
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if isCookieMutation(r) && !sameOriginRequest(r, allowedOrigins) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "CSRF_ORIGIN_INVALID"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isCookieMutation(r *http.Request) bool {
	if r == nil || (r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch && r.Method != http.MethodDelete) {
		return false
	}
	return hasCookie(r, auth.SessionCookieName(auth.AudienceAdmin)) || hasCookie(r, auth.SessionCookieName(auth.AudienceConsole))
}

func hasCookie(r *http.Request, name string) bool {
	cookie, err := r.Cookie(name)
	return err == nil && strings.TrimSpace(cookie.Value) != ""
}

func configuredOrigins() map[string]struct{} {
	result := map[string]struct{}{}
	for _, value := range strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), ",") {
		if trimmed := strings.TrimRight(strings.TrimSpace(value), "/"); trimmed != "" {
			result[strings.ToLower(trimmed)] = struct{}{}
		}
	}
	return result
}

func originAllowed(r *http.Request, origin string, configured map[string]struct{}) bool {
	origin = strings.TrimRight(strings.ToLower(strings.TrimSpace(origin)), "/")
	if origin == "" {
		return false
	}
	if len(configured) > 0 {
		_, ok := configured[origin]
		return ok
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return false
	}
	return strings.EqualFold(parsed.Host, r.Host)
}

func sameOriginRequest(r *http.Request, configured map[string]struct{}) bool {
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
		return originAllowed(r, origin, configured)
	}
	if referer := strings.TrimSpace(r.Header.Get("Referer")); referer != "" {
		parsed, err := url.Parse(referer)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return false
		}
		return originAllowed(r, parsed.Scheme+"://"+parsed.Host, configured)
	}
	// Same-site non-browser clients do not send Origin or Referer. SameSite=Lax
	// remains the browser fallback, while API-token requests never use cookies.
	return true
}

func resolveWebDir(webDir string) string {
	builtDir := filepath.Join(webDir, "dist")
	if info, err := os.Stat(filepath.Join(builtDir, "index.html")); err == nil && !info.IsDir() {
		return builtDir
	}
	return ""
}

type loginPayload struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	TenantID string `json:"tenant_id"`
	MFACode  string `json:"mfa_code"`
}

type registrationPayload struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	TenantName  string `json:"tenant_name"`
	TenantSlug  string `json:"tenant_slug"`
	ProjectName string `json:"project_name"`
}

type emailVerificationPayload struct {
	Token string `json:"token"`
	Email string `json:"email"`
}

func registrationHandler(service auth.RegistrationProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "REGISTRATION_UNAVAILABLE"})
			return
		}
		var payload registrationPayload
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		account, err := service.Register(r.Context(), auth.RegistrationRequest{
			Email:       payload.Email,
			Password:    payload.Password,
			DisplayName: payload.DisplayName,
			TenantName:  payload.TenantName,
			TenantSlug:  payload.TenantSlug,
			ProjectName: payload.ProjectName,
			ClientIP:    clientIP(r),
		})
		if err != nil {
			writeRegistrationError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, account)
	}
}

func writeRegistrationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrRegistrationInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REGISTRATION"})
	case errors.Is(err, auth.ErrEmailAlreadyExists):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "EMAIL_ALREADY_REGISTERED"})
	case errors.Is(err, auth.ErrTenantAlreadyExists):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "TENANT_SLUG_ALREADY_REGISTERED"})
	case errors.Is(err, auth.ErrRegistrationThrottled):
		w.Header().Set("Retry-After", "60")
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "REGISTRATION_THROTTLED"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "REGISTRATION_UNAVAILABLE"})
	}
}

func emailVerificationHandler(service auth.RegistrationProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		verifier, ok := service.(auth.EmailVerificationService)
		if !ok || verifier == nil {
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "EMAIL_VERIFICATION_NOT_REQUIRED"})
			return
		}
		var payload emailVerificationPayload
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		if err := verifier.ConfirmEmail(r.Context(), payload.Token); err != nil {
			if errors.Is(err, auth.ErrEmailVerificationRequired) {
				writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "EMAIL_VERIFICATION_NOT_REQUIRED"})
				return
			}
			if errors.Is(err, auth.ErrEmailVerificationInvalid) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "EMAIL_VERIFICATION_INVALID"})
				return
			}
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "EMAIL_VERIFICATION_UNAVAILABLE"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "email_verified"})
	}
}

func emailVerificationResendHandler(service auth.RegistrationProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		verifier, ok := service.(auth.EmailVerificationService)
		if !ok || verifier == nil {
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "EMAIL_VERIFICATION_NOT_REQUIRED"})
			return
		}
		var payload emailVerificationPayload
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		if err := verifier.RequestEmailVerification(r.Context(), payload.Email, clientIP(r)); err != nil {
			if errors.Is(err, auth.ErrEmailVerificationRequired) {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "EMAIL_VERIFICATION_UNAVAILABLE"})
				return
			}
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "EMAIL_VERIFICATION_UNAVAILABLE"})
			return
		}
		// Do not reveal whether an email exists or is already verified.
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
}

type passwordResetRequestPayload struct {
	Email string `json:"email"`
}

type passwordResetConfirmPayload struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

type mfaEnrollmentPayload struct {
	Issuer  string `json:"issuer"`
	Account string `json:"account"`
}

type mfaConfirmPayload struct {
	Code string `json:"code"`
}

type securitySettingsPayload struct {
	AdminMFAEnabled bool `json:"admin_mfa_enabled"`
}

func loginHandler(service auth.LoginProvider, audience auth.Audience, secureCookies bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "AUTH_UNAVAILABLE"})
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		defer r.Body.Close()
		var payload loginPayload
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		if err := ensureSingleJSONValue(decoder); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}

		session, err := service.Login(r.Context(), auth.LoginRequest{
			Email:    payload.Email,
			Password: payload.Password,
			TenantID: payload.TenantID,
			MFACode:  payload.MFACode,
			IP:       clientIP(r),
		}, audience)
		if err != nil {
			writeLoginError(w, err)
			return
		}
		if err := auth.SetSessionCookie(w, session, secureCookies); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "AUTH_UNAVAILABLE"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":     "authenticated",
			"audience":   audience,
			"expires_at": session.ExpiresAt,
		})
	}
}

func logoutHandler(service auth.LoginProvider, audience auth.Audience, secureCookies bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "AUTH_UNAVAILABLE"})
			return
		}
		cookie, err := r.Cookie(auth.SessionCookieName(audience))
		if err != nil || cookie.Value == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		if err := service.Logout(r.Context(), cookie.Value); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "AUTH_UNAVAILABLE"})
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     auth.SessionCookieName(audience),
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   secureCookies,
			SameSite: http.SameSiteLaxMode,
		})
		writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
	})
}

func passwordResetRequestHandler(
	service auth.PasswordResetProvider,
	notifier auth.PasswordResetNotifier,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil || notifier == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "AUTH_UNAVAILABLE"})
			return
		}
		var payload passwordResetRequestPayload
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		token, found, err := service.Request(r.Context(), payload.Email, clientIP(r))
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "AUTH_UNAVAILABLE"})
			return
		}
		if found {
			if err := notifier.SendPasswordReset(r.Context(), payload.Email, token); err != nil {
				// Keep the response indistinguishable for existing and unknown
				// accounts. The token and email must never enter the log.
				log.Printf("password reset notification failed request_id=%s", r.Header.Get("X-Request-ID"))
			}
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
}

func passwordResetConfirmHandler(service auth.PasswordResetProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "AUTH_UNAVAILABLE"})
			return
		}
		var payload passwordResetConfirmPayload
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		if err := service.Confirm(r.Context(), payload.Token, payload.NewPassword); err != nil {
			switch {
			case errors.Is(err, auth.ErrInvalidResetToken):
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "RESET_TOKEN_INVALID"})
			default:
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "password_reset"})
	}
}

func mfaEnrollmentBeginHandler(service auth.MFAEnrollmentProvider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "MFA_UNAVAILABLE"})
			return
		}
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		var payload mfaEnrollmentPayload
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		enrollment, err := service.Begin(r.Context(), principal.ID, payload.Issuer, payload.Account)
		if err != nil {
			switch {
			case errors.Is(err, mfa.ErrMFAAlreadyEnabled):
				writeJSON(w, http.StatusConflict, map[string]string{"error": "MFA_ALREADY_ENABLED"})
			default:
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "MFA_ENROLLMENT_INVALID"})
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"enrollment_id": enrollment.ID,
			"secret":        enrollment.Secret,
			"otpauth_url":   enrollment.OTPAuthURL,
			"expires_at":    enrollment.ExpiresAt,
		})
	})
}

func mfaEnrollmentConfirmHandler(service auth.MFAEnrollmentProvider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "MFA_UNAVAILABLE"})
			return
		}
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		var payload mfaConfirmPayload
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		if err := service.Confirm(r.Context(), principal.ID, r.PathValue("enrollmentID"), payload.Code); err != nil {
			switch {
			case errors.Is(err, mfa.ErrMFAInvalidCode):
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "MFA_CODE_INVALID"})
			case errors.Is(err, mfa.ErrMFAThrottled):
				w.Header().Set("Retry-After", "900")
				writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "MFA_STEP_UP_THROTTLED"})
			case errors.Is(err, mfa.ErrEnrollmentExpired):
				writeJSON(w, http.StatusGone, map[string]string{"error": "MFA_ENROLLMENT_EXPIRED"})
			default:
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "MFA_ENROLLMENT_INVALID"})
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "mfa_enabled"})
	})
}

func mfaStatusHandler(service auth.MFAEnrollmentProvider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		settings, ok := service.(auth.MFASettingsProvider)
		if !ok || settings == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "MFA_UNAVAILABLE"})
			return
		}
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		status, err := settings.Status(r.Context(), principal.ID)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "MFA_UNAVAILABLE"})
			return
		}
		writeJSON(w, http.StatusOK, status)
	})
}

func mfaDisableHandler(service auth.MFAEnrollmentProvider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		settings, ok := service.(auth.MFASettingsProvider)
		if !ok || settings == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "MFA_UNAVAILABLE"})
			return
		}
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		var payload mfaConfirmPayload
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		if err := settings.Disable(r.Context(), principal.ID, payload.Code); err != nil {
			switch {
			case errors.Is(err, mfa.ErrMFANotEnabled):
				writeJSON(w, http.StatusConflict, map[string]string{"error": "MFA_NOT_ENABLED"})
			case errors.Is(err, mfa.ErrMFAInvalidCode):
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "MFA_CODE_INVALID"})
			default:
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "MFA_UNAVAILABLE"})
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "mfa_disabled"})
	})
}

func adminMFADisableHandler(service auth.MFAEnrollmentProvider, security auth.SecuritySettingsProvider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if security != nil {
			settings, err := security.GetAdminSecuritySettings(r.Context())
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "MFA_UNAVAILABLE"})
				return
			}
			if settings.AdminMFAEnabled {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "ADMIN_MFA_ENFORCED"})
				return
			}
		}
		mfaDisableHandler(service).ServeHTTP(w, r)
	})
}

func publicSystemSettingsHandler(service auth.SecuritySettingsProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		settings := adminsettings.SystemSettings{SiteName: adminsettings.DefaultSiteName}
		if provider, ok := service.(adminsettings.SystemSettingsProvider); ok && provider != nil {
			loaded, err := provider.GetSystemSettings(r.Context())
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "SYSTEM_SETTINGS_UNAVAILABLE"})
				return
			}
			settings = loaded
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"site_name":        settings.SiteName,
			"site_logo_url":    settings.SiteLogoURL,
			"site_favicon_url": settings.SiteFaviconURL,
		})
	}
}

// publicFeatureSettingsHandler intentionally exposes only customer-facing
// feature flags. SMTP settings and internal operations switches stay private.
func publicFeatureSettingsHandler(service auth.SecuritySettingsProvider) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		settings := map[string]bool{"model_status_enabled": true}
		provider, ok := service.(adminsettings.FeatureSettingsProvider)
		if ok && provider != nil {
			features, err := provider.GetFeatureSettings(r.Context())
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "PUBLIC_FEATURES_UNAVAILABLE"})
				return
			}
			settings["model_status_enabled"] = features.ModelStatusEnabled
		}
		writeJSON(w, http.StatusOK, settings)
	})
}

type systemSettingsPayload struct {
	SiteName          string `json:"site_name"`
	SiteLogoURL       string `json:"site_logo_url"`
	SiteFaviconURL    string `json:"site_favicon_url"`
	SMTPAddress       string `json:"smtp_addr"`
	SMTPFrom          string `json:"smtp_from"`
	SMTPUsername      string `json:"smtp_username"`
	SMTPPassword      string `json:"smtp_password"`
	SMTPPasswordClear bool   `json:"smtp_password_clear"`
	PublicBaseURL     string `json:"public_base_url"`
}

func systemSettingsReadHandler(service auth.SecuritySettingsProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provider, ok := service.(adminsettings.SystemSettingsProvider)
		if !ok || provider == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "SYSTEM_SETTINGS_UNAVAILABLE"})
			return
		}
		settings, err := provider.GetSystemSettings(r.Context())
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "SYSTEM_SETTINGS_UNAVAILABLE"})
			return
		}
		writeJSON(w, http.StatusOK, settings)
	}
}

func systemSettingsUpdateHandler(service auth.SecuritySettingsProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provider, ok := service.(adminsettings.SystemSettingsProvider)
		if !ok || provider == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "SYSTEM_SETTINGS_UNAVAILABLE"})
			return
		}
		// Keep the legacy endpoint safe for older clients: with the real
		// settings service it updates only branding. SMTP must use the separate
		// email settings endpoint so a stale client cannot overwrite it.
		if siteProvider, siteOK := service.(adminsettings.SiteSettingsProvider); siteOK && siteProvider != nil {
			var sitePayload struct {
				SiteName       string `json:"site_name"`
				SiteLogoURL    string `json:"site_logo_url"`
				SiteFaviconURL string `json:"site_favicon_url"`
			}
			if err := decodeJSON(w, r, &sitePayload); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_SYSTEM_SETTINGS"})
				return
			}
			settings, err := siteProvider.UpdateSiteSettings(r.Context(), principalID(r), adminsettings.SiteSettingsUpdate{SiteName: sitePayload.SiteName, SiteLogoURL: sitePayload.SiteLogoURL, SiteFaviconURL: sitePayload.SiteFaviconURL})
			if err != nil {
				if errors.Is(err, adminsettings.ErrInvalidSystemSettings) {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_SYSTEM_SETTINGS"})
					return
				}
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "SYSTEM_SETTINGS_UNAVAILABLE"})
				return
			}
			writeJSON(w, http.StatusOK, settings)
			return
		}
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		var payload systemSettingsPayload
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_SYSTEM_SETTINGS"})
			return
		}
		settings, err := provider.UpdateSystemSettings(r.Context(), principal.ID, adminsettings.SystemSettingsUpdate{
			SiteName:          payload.SiteName,
			SiteLogoURL:       payload.SiteLogoURL,
			SiteFaviconURL:    payload.SiteFaviconURL,
			SMTPAddress:       payload.SMTPAddress,
			SMTPFrom:          payload.SMTPFrom,
			SMTPUsername:      payload.SMTPUsername,
			SMTPPassword:      payload.SMTPPassword,
			SMTPPasswordClear: payload.SMTPPasswordClear,
			PublicBaseURL:     payload.PublicBaseURL,
		})
		if err != nil {
			if errors.Is(err, adminsettings.ErrInvalidSystemSettings) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_SYSTEM_SETTINGS"})
				return
			}
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "SYSTEM_SETTINGS_UNAVAILABLE"})
			return
		}
		writeJSON(w, http.StatusOK, settings)
	}
}

func siteSettingsUpdateHandler(service auth.SecuritySettingsProvider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provider, ok := service.(adminsettings.SiteSettingsProvider)
		if !ok || provider == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "SYSTEM_SETTINGS_UNAVAILABLE"})
			return
		}
		var payload struct {
			SiteName       string `json:"site_name"`
			SiteLogoURL    string `json:"site_logo_url"`
			SiteFaviconURL string `json:"site_favicon_url"`
		}
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_SYSTEM_SETTINGS"})
			return
		}
		settings, err := provider.UpdateSiteSettings(r.Context(), principalID(r), adminsettings.SiteSettingsUpdate{
			SiteName: payload.SiteName, SiteLogoURL: payload.SiteLogoURL, SiteFaviconURL: payload.SiteFaviconURL,
		})
		if err != nil {
			if errors.Is(err, adminsettings.ErrInvalidSystemSettings) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_SYSTEM_SETTINGS"})
				return
			}
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "SYSTEM_SETTINGS_UNAVAILABLE"})
			return
		}
		writeJSON(w, http.StatusOK, settings)
	})
}

type emailSettingsPayload struct {
	SMTPHost          string `json:"smtp_host"`
	SMTPPort          int    `json:"smtp_port"`
	SMTPUsername      string `json:"smtp_username"`
	SMTPPassword      string `json:"smtp_password"`
	SMTPPasswordClear bool   `json:"smtp_password_clear"`
	SMTPFromEmail     string `json:"smtp_from_email"`
	SMTPFromName      string `json:"smtp_from_name"`
	SMTPTLS           bool   `json:"smtp_tls"`
	PublicBaseURL     string `json:"public_base_url"`
}

type emailTestMessagePayload struct {
	Recipient string `json:"recipient"`
}

func emailSettingsReadHandler(service auth.SecuritySettingsProvider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provider, ok := service.(adminsettings.EmailSettingsProvider)
		if !ok || provider == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "EMAIL_SETTINGS_UNAVAILABLE"})
			return
		}
		settings, err := provider.GetEmailSettings(r.Context())
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "EMAIL_SETTINGS_UNAVAILABLE"})
			return
		}
		writeJSON(w, http.StatusOK, settings)
	})
}

func emailSettingsUpdateHandler(service auth.SecuritySettingsProvider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provider, ok := service.(adminsettings.EmailSettingsProvider)
		if !ok || provider == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "EMAIL_SETTINGS_UNAVAILABLE"})
			return
		}
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		var payload emailSettingsPayload
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_EMAIL_SETTINGS"})
			return
		}
		settings, err := provider.UpdateEmailSettings(r.Context(), principal.ID, adminsettings.EmailSettingsUpdate{
			SMTPHost: payload.SMTPHost, SMTPPort: payload.SMTPPort, SMTPUsername: payload.SMTPUsername,
			SMTPPassword: payload.SMTPPassword, SMTPPasswordClear: payload.SMTPPasswordClear,
			SMTPFromEmail: payload.SMTPFromEmail, SMTPFromName: payload.SMTPFromName,
			SMTPTLS: payload.SMTPTLS, PublicBaseURL: payload.PublicBaseURL,
		})
		if err != nil {
			if errors.Is(err, adminsettings.ErrInvalidEmailSettings) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_EMAIL_SETTINGS"})
				return
			}
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "EMAIL_SETTINGS_UNAVAILABLE"})
			return
		}
		writeJSON(w, http.StatusOK, settings)
	})
}

func emailSMTPConnectionTestHandler(service auth.SecuritySettingsProvider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provider, ok := service.(adminsettings.EmailSettingsProvider)
		if !ok || provider == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "EMAIL_SETTINGS_UNAVAILABLE"})
			return
		}
		if err := provider.TestSMTPConnection(r.Context()); err != nil {
			if errors.Is(err, adminsettings.ErrInvalidEmailSettings) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_EMAIL_SETTINGS"})
				return
			}
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "SMTP_CONNECTION_FAILED"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "connected"})
	})
}

func emailTestMessageHandler(service auth.SecuritySettingsProvider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provider, ok := service.(adminsettings.EmailSettingsProvider)
		if !ok || provider == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "EMAIL_SETTINGS_UNAVAILABLE"})
			return
		}
		var payload emailTestMessagePayload
		if err := decodeJSON(w, r, &payload); err != nil || strings.TrimSpace(payload.Recipient) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_EMAIL_RECIPIENT"})
			return
		}
		if err := provider.SendTestEmail(r.Context(), strings.TrimSpace(payload.Recipient)); err != nil {
			if errors.Is(err, adminsettings.ErrInvalidEmailSettings) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_EMAIL_SETTINGS"})
				return
			}
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "TEST_EMAIL_FAILED"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
	})
}

func featureSettingsReadHandler(service auth.SecuritySettingsProvider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provider, ok := service.(adminsettings.FeatureSettingsProvider)
		if !ok || provider == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "FEATURE_SETTINGS_UNAVAILABLE"})
			return
		}
		settings, err := provider.GetFeatureSettings(r.Context())
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "FEATURE_SETTINGS_UNAVAILABLE"})
			return
		}
		writeJSON(w, http.StatusOK, settings)
	})
}

func featureSettingsUpdateHandler(service auth.SecuritySettingsProvider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provider, ok := service.(adminsettings.FeatureSettingsProvider)
		if !ok || provider == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "FEATURE_SETTINGS_UNAVAILABLE"})
			return
		}
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		var payload adminsettings.FeatureSettingsUpdate
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_FEATURE_SETTINGS"})
			return
		}
		settings, err := provider.UpdateFeatureSettings(r.Context(), principal.ID, payload)
		if err != nil {
			if errors.Is(err, adminsettings.ErrInvalidEmailSettings) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_FEATURE_SETTINGS"})
				return
			}
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "FEATURE_SETTINGS_UNAVAILABLE"})
			return
		}
		writeJSON(w, http.StatusOK, settings)
	})
}

func emailTemplateListHandler(service auth.SecuritySettingsProvider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provider, ok := service.(adminsettings.EmailTemplateService)
		if !ok || provider == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "EMAIL_TEMPLATES_UNAVAILABLE"})
			return
		}
		items, err := provider.ListEmailTemplates(r.Context())
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "EMAIL_TEMPLATES_UNAVAILABLE"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"templates": items})
	})
}

func emailTemplateCreateHandler(service auth.SecuritySettingsProvider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provider, ok := service.(adminsettings.EmailTemplateService)
		if !ok || provider == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "EMAIL_TEMPLATES_UNAVAILABLE"})
			return
		}
		var payload adminsettings.EmailTemplateMutation
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_EMAIL_TEMPLATE"})
			return
		}
		item, err := provider.CreateEmailTemplate(r.Context(), principalID(r), payload)
		if err != nil {
			writeEmailTemplateError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, item)
	})
}

func emailTemplateUpdateHandler(service auth.SecuritySettingsProvider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provider, ok := service.(adminsettings.EmailTemplateService)
		if !ok || provider == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "EMAIL_TEMPLATES_UNAVAILABLE"})
			return
		}
		var payload adminsettings.EmailTemplateMutation
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_EMAIL_TEMPLATE"})
			return
		}
		item, err := provider.UpdateEmailTemplate(r.Context(), principalID(r), r.PathValue("templateID"), payload)
		if err != nil {
			writeEmailTemplateError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	})
}

func emailTemplateDeleteHandler(service auth.SecuritySettingsProvider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provider, ok := service.(adminsettings.EmailTemplateService)
		if !ok || provider == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "EMAIL_TEMPLATES_UNAVAILABLE"})
			return
		}
		if err := provider.DeleteEmailTemplate(r.Context(), principalID(r), r.PathValue("templateID")); err != nil {
			writeEmailTemplateError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	})
}

func writeEmailTemplateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, adminsettings.ErrInvalidEmailSettings):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_EMAIL_TEMPLATE"})
	case errors.Is(err, adminsettings.ErrEmailTemplateNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "EMAIL_TEMPLATE_NOT_FOUND"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "EMAIL_TEMPLATES_UNAVAILABLE"})
	}
}

func securitySettingsReadHandler(service auth.SecuritySettingsProvider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "SECURITY_SETTINGS_UNAVAILABLE"})
			return
		}
		settings, err := service.GetAdminSecuritySettings(r.Context())
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "SECURITY_SETTINGS_UNAVAILABLE"})
			return
		}
		writeJSON(w, http.StatusOK, settings)
	})
}

func securitySettingsUpdateHandler(service auth.SecuritySettingsProvider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "SECURITY_SETTINGS_UNAVAILABLE"})
			return
		}
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		var payload securitySettingsPayload
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		settings, err := service.UpdateAdminMFAEnabled(r.Context(), payload.AdminMFAEnabled, principal.ID)
		if err != nil {
			switch {
			case errors.Is(err, adminsettings.ErrAdminMFAEnrollmentRequired):
				writeJSON(w, http.StatusConflict, map[string]string{"error": "ADMIN_MFA_ENROLLMENT_REQUIRED"})
			default:
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "SECURITY_SETTINGS_UNAVAILABLE"})
			}
			return
		}
		writeJSON(w, http.StatusOK, settings)
	})
}

func relayChatCompletionsHandler(service relay.ChatCompletionService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "RELAY_NOT_IMPLEMENTED"})
			return
		}
		var payload relay.ChatCompletionRequest
		if err := decodeRelayJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		metadata := relay.RequestMetadataFromContext(r.Context())
		payload.RequestID = metadata.RequestID
		payload.IdempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if payload.IdempotencyKey == "" {
			payload.IdempotencyKey = metadata.RequestID
		}
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		if !auth.NetworkAllowlistAllows(principal, clientIP(r), r.Header.Get("Origin"), r.Header.Get("Referer")) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "TOKEN_NETWORK_NOT_ALLOWED"})
			return
		}
		if payload.Stream {
			streamer, ok := service.(relay.StreamingChatCompletionService)
			if !ok {
				writeRelayError(w, relay.ErrStreamingUnsupported)
				return
			}
			streamStarted := false
			emit := func(event relay.ChatCompletionStreamEvent) error {
				if !streamStarted {
					w.Header().Set("Content-Type", "text/event-stream")
					w.Header().Set("Cache-Control", "no-cache")
					w.Header().Set("Connection", "keep-alive")
					w.WriteHeader(http.StatusOK)
					streamStarted = true
				}
				encoded, err := json.Marshal(openAIStreamChunk(event, payload.Model))
				if err != nil {
					return err
				}
				if _, err := w.Write([]byte("data: ")); err != nil {
					return err
				}
				if _, err := w.Write(encoded); err != nil {
					return err
				}
				if _, err := w.Write([]byte("\n\n")); err != nil {
					return err
				}
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
				return nil
			}
			err := streamer.StreamChatCompletions(relayRequestContext(r, "stream"), principal, payload, emit)
			if err != nil {
				if !streamStarted {
					writeRelayError(w, err)
				}
				return
			}
			if !streamStarted {
				_ = emit(relay.ChatCompletionStreamEvent{Model: payload.Model})
			}
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			return
		}
		response, err := service.ChatCompletions(r.Context(), principal, payload)
		if err != nil {
			writeRelayError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
	})
}

func openAIStreamChunk(event relay.ChatCompletionStreamEvent, requestedModel string) map[string]any {
	model := event.Model
	if model == "" {
		model = requestedModel
	}
	chunk := map[string]any{
		"id": event.ID, "object": "chat.completion.chunk", "created": event.Created, "model": model,
		"choices": []any{},
	}
	if event.ID == "" {
		chunk["id"] = "chatcmpl_stream"
	}
	if event.Created == 0 {
		chunk["created"] = time.Now().Unix()
	}
	if event.Delta != "" || event.Role != "" || len(event.ToolCalls) > 0 || len(event.FunctionCall) > 0 || event.FinishReason != "" {
		delta := map[string]any{}
		if event.Role != "" {
			delta["role"] = event.Role
		}
		if event.Delta != "" {
			delta["content"] = event.Delta
		}
		if len(event.ToolCalls) > 0 && string(event.ToolCalls) != "null" {
			var value any
			if json.Unmarshal(event.ToolCalls, &value) == nil {
				delta["tool_calls"] = value
			}
		}
		if len(event.FunctionCall) > 0 && string(event.FunctionCall) != "null" {
			var value any
			if json.Unmarshal(event.FunctionCall, &value) == nil {
				delta["function_call"] = value
			}
		}
		finishReason := any(nil)
		if event.FinishReason != "" {
			finishReason = event.FinishReason
		}
		chunk["choices"] = []any{map[string]any{"index": event.Index, "delta": delta, "finish_reason": finishReason}}
	}
	if event.HasUsage {
		chunk["usage"] = map[string]any{
			"prompt_tokens": event.Usage.PromptTokens, "completion_tokens": event.Usage.CompletionTokens,
			"total_tokens": event.Usage.TotalTokens,
		}
	}
	return chunk
}

func relayEmbeddingsHandler(service relay.ChatCompletionService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		embedder, ok := service.(relay.EmbeddingCompletionService)
		if !ok {
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "EMBEDDINGS_NOT_IMPLEMENTED"})
			return
		}
		var payload relay.EmbeddingRequest
		if err := decodeRelayJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		metadata := relay.RequestMetadataFromContext(r.Context())
		payload.RequestID = metadata.RequestID
		payload.IdempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if payload.IdempotencyKey == "" {
			payload.IdempotencyKey = metadata.RequestID
		}
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		if !auth.NetworkAllowlistAllows(principal, clientIP(r), r.Header.Get("Origin"), r.Header.Get("Referer")) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "TOKEN_NETWORK_NOT_ALLOWED"})
			return
		}
		response, err := embedder.CreateEmbeddings(relayRequestContext(r, "embedding"), principal, payload)
		if err != nil {
			writeRelayError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
	})
}

func relayChannelsHandler(service relay.ChatCompletionService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		manager, ok := channelManager(w, service)
		if !ok {
			return
		}
		channels, err := manager.ListChannels(r.Context())
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "CHANNELS_UNAVAILABLE"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"channels": channels})
	})
}

func relayChannelModelDiscoveryHandler(service relay.ChatCompletionService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		manager, ok := modelDiscoveryManager(w, service)
		if !ok {
			return
		}
		var payload relay.ModelDiscoveryRequest
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		models, err := manager.DiscoverModels(ctx, payload)
		if err != nil {
			writeModelDiscoveryError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"models": models})
	})
}

func relayChannelCreateHandler(service relay.ChatCompletionService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		manager, ok := channelManager(w, service)
		if !ok {
			return
		}
		var payload relay.ChannelMutation
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		channel, err := manager.CreateChannel(r.Context(), principalID(r), payload)
		if err != nil {
			writeChannelError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, channel)
	})
}

func relayChannelUpdateHandler(service relay.ChatCompletionService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		manager, ok := channelManager(w, service)
		if !ok {
			return
		}
		var payload relay.ChannelMutation
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		channel, err := manager.UpdateChannel(r.Context(), principalID(r), r.PathValue("channelID"), payload)
		if err != nil {
			writeChannelError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, channel)
	})
}

func relayChannelStatusHandler(service relay.ChatCompletionService, status string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		manager, ok := channelManager(w, service)
		if !ok {
			return
		}
		channel, err := manager.SetChannelStatus(r.Context(), principalID(r), r.PathValue("channelID"), status)
		if err != nil {
			writeChannelError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, channel)
	})
}

func relayChannelDeleteHandler(service relay.ChatCompletionService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		manager, ok := channelManager(w, service)
		if !ok {
			return
		}
		if err := manager.DeleteChannel(r.Context(), principalID(r), r.PathValue("channelID")); err != nil {
			writeChannelError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	})
}

func groupListHandler(service groups.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "GROUPS_UNAVAILABLE"})
			return
		}
		items, err := service.List(r.Context())
		if err != nil {
			writeGroupError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"groups": items})
	})
}

func groupCreateHandler(service groups.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "GROUPS_UNAVAILABLE"})
			return
		}
		var payload groups.Mutation
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		item, err := service.Create(r.Context(), principalID(r), payload)
		if err != nil {
			writeGroupError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, item)
	})
}

func groupUpdateHandler(service groups.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "GROUPS_UNAVAILABLE"})
			return
		}
		var payload groups.Mutation
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		item, err := service.Update(r.Context(), principalID(r), r.PathValue("groupID"), payload)
		if err != nil {
			writeGroupError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	})
}

func groupDeleteHandler(service groups.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "GROUPS_UNAVAILABLE"})
			return
		}
		if err := service.Delete(r.Context(), principalID(r), r.PathValue("groupID")); err != nil {
			writeGroupError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	})
}

func writeGroupError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, groups.ErrInvalidRequest):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_GROUP_REQUEST"})
	case errors.Is(err, groups.ErrDefaultGroupProtected):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "DEFAULT_GROUP_PROTECTED"})
	case errors.Is(err, groups.ErrGroupInUse):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "GROUP_IN_USE"})
	case errors.Is(err, groups.ErrGroupNotFound), errors.Is(err, groups.ErrChannelNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "GROUP_RESOURCE_NOT_FOUND"})
	case errors.Is(err, groups.ErrUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "GROUPS_UNAVAILABLE"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "GROUPS_UNAVAILABLE"})
	}
}

func tokenListHandler(service tokens.AdminService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "TOKENS_UNAVAILABLE"})
			return
		}
		items, err := service.List(r.Context())
		if err != nil {
			writeTokenAdminError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"tokens": items})
	})
}

func modelListHandler(service models.Catalog) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "MODELS_UNAVAILABLE"})
			return
		}
		items, err := service.ListPublic(r.Context())
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "MODELS_UNAVAILABLE"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"models": items})
	})
}

func officialPriceSyncHandler(service modelprices.SyncService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "OFFICIAL_PRICES_UNAVAILABLE"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()
		result, err := service.Sync(ctx)
		if err != nil {
			writeOfficialPriceSyncError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
}

func writeOfficialPriceSyncError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, modelprices.ErrFetchFailed), errors.Is(err, modelprices.ErrInvalidData):
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "OFFICIAL_PRICES_SOURCE_FAILED"})
	case errors.Is(err, modelprices.ErrUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "OFFICIAL_PRICES_UNAVAILABLE"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "OFFICIAL_PRICES_UNAVAILABLE"})
	}
}

type userStatusPayload struct {
	Status string `json:"status"`
}

type platformRolePayload struct {
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Status      string   `json:"status"`
	Permissions []string `json:"permissions"`
}

type platformUserRolesPayload struct {
	RoleIDs []string `json:"role_ids"`
}

type userUpdatePayload struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}

func userListHandler(service users.AdminService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "USERS_UNAVAILABLE"})
			return
		}
		items, err := service.List(r.Context())
		if err != nil {
			writeUserAdminError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"users": items})
	})
}

func userTenantListHandler(service users.AdminService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "USERS_UNAVAILABLE"})
			return
		}
		items, err := service.ListTenants(r.Context())
		if err != nil {
			writeUserAdminError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"tenants": items})
	})
}

func adminUserCreationDisabledHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "ADMIN_USER_CREATION_DISABLED"})
	})
}

func userUpdateHandler(service users.AdminService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "USERS_UNAVAILABLE"})
			return
		}
		var payload userUpdatePayload
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		item, err := service.Update(r.Context(), principal.ID, r.PathValue("userID"), users.UpdateRequest{
			Email:       payload.Email,
			DisplayName: payload.DisplayName,
			Password:    payload.Password,
		})
		if err != nil {
			writeUserAdminError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	})
}

func userStatusUpdateHandler(service users.AdminService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "USERS_UNAVAILABLE"})
			return
		}
		var payload userStatusPayload
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		item, err := service.SetStatus(r.Context(), principal.ID, r.PathValue("userID"), payload.Status)
		if err != nil {
			writeUserAdminError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	})
}

func writeUserAdminError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, users.ErrInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_USER_REQUEST"})
	case errors.Is(err, users.ErrEmailExists):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "EMAIL_ALREADY_EXISTS"})
	case errors.Is(err, users.ErrLastPlatformAdmin):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "LAST_PLATFORM_ADMIN_PROTECTED"})
	case errors.Is(err, users.ErrPlatformOwnerProtected):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "PLATFORM_OWNER_PROTECTED"})
	case errors.Is(err, users.ErrEmailVerificationRequired):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "USER_EMAIL_NOT_VERIFIED"})
	case errors.Is(err, users.ErrSelfUpdate):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "SELF_ACCOUNT_CHANGE_NOT_ALLOWED"})
	case errors.Is(err, users.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "USER_NOT_FOUND"})
	case errors.Is(err, users.ErrUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "USERS_UNAVAILABLE"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "USERS_UNAVAILABLE"})
	}
}

func platformRolesHandler(service users.PlatformRoleService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		roles, err := service.ListPlatformRoles(r.Context())
		if err != nil {
			writePlatformRoleError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"roles": roles})
	})
}

func platformPermissionsHandler(service users.PlatformRoleService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		permissions, err := service.ListPlatformPermissions(r.Context())
		if err != nil {
			writePlatformRoleError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"permissions": permissions})
	})
}

func platformRoleCreateHandler(service users.PlatformRoleService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload platformRolePayload
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_PLATFORM_ROLE_REQUEST"})
			return
		}
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		role, err := service.CreatePlatformRole(r.Context(), principal.ID, users.PlatformRoleMutation{
			Code: payload.Code, Name: payload.Name, Status: payload.Status, Permissions: payload.Permissions,
		})
		if err != nil {
			writePlatformRoleError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, role)
	})
}

func platformRoleUpdateHandler(service users.PlatformRoleService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload platformRolePayload
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_PLATFORM_ROLE_REQUEST"})
			return
		}
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		role, err := service.UpdatePlatformRole(r.Context(), principal.ID, r.PathValue("roleID"), users.PlatformRoleMutation{
			Code: payload.Code, Name: payload.Name, Status: payload.Status, Permissions: payload.Permissions,
		})
		if err != nil {
			writePlatformRoleError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, role)
	})
}

func platformRoleDisableHandler(service users.PlatformRoleService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		if err := service.DisablePlatformRole(r.Context(), principal.ID, r.PathValue("roleID")); err != nil {
			writePlatformRoleError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "disabled"})
	})
}

func platformUserRolesHandler(service users.PlatformRoleService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		roles, err := service.GetPlatformUserRoles(r.Context(), r.PathValue("userID"))
		if err != nil {
			writePlatformRoleError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"roles": roles})
	})
}

func platformUserRolesUpdateHandler(service users.PlatformRoleService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload platformUserRolesPayload
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_PLATFORM_ROLE_REQUEST"})
			return
		}
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		roles, err := service.SetPlatformUserRoles(r.Context(), principal.ID, r.PathValue("userID"), payload.RoleIDs)
		if err != nil {
			writePlatformRoleError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"roles": roles})
	})
}

func writePlatformRoleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, users.ErrPlatformRoleAccessDenied):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "PLATFORM_ROLE_ACCESS_DENIED"})
	case errors.Is(err, users.ErrPlatformRoleInvalid), errors.Is(err, users.ErrPlatformPermissionUnknown):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_PLATFORM_ROLE_REQUEST"})
	case errors.Is(err, users.ErrPlatformRoleExists):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "PLATFORM_ROLE_EXISTS"})
	case errors.Is(err, users.ErrPlatformRoleProtected):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "PLATFORM_OWNER_ROLE_PROTECTED"})
	case errors.Is(err, users.ErrLastPlatformRoleAdmin):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "LAST_PLATFORM_ADMIN_PROTECTED"})
	case errors.Is(err, users.ErrPlatformMFARequired):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "PLATFORM_ADMIN_MFA_REQUIRED"})
	case errors.Is(err, users.ErrPlatformRoleNotFound), errors.Is(err, users.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "PLATFORM_ROLE_RESOURCE_NOT_FOUND"})
	case errors.Is(err, users.ErrUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "PLATFORM_ROLE_UNAVAILABLE"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "PLATFORM_ROLE_UNAVAILABLE"})
	}
}

type tenantMemberPayload struct {
	Email  string `json:"email"`
	Role   string `json:"role"`
	Status string `json:"status"`
}

type projectPayload struct {
	Name   string `json:"name"`
	Slug   string `json:"slug"`
	Status string `json:"status"`
}

type projectMemberPayload struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
}

func tenantServiceFromRequest(w http.ResponseWriter, r *http.Request, service users.TenantService) (*auth.Principal, bool) {
	if service == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "TENANT_MANAGEMENT_UNAVAILABLE"})
		return nil, false
	}
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
		return nil, false
	}
	return principal, true
}

func tenantMembersListHandler(service users.TenantService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := tenantServiceFromRequest(w, r, service)
		if !ok {
			return
		}
		items, err := service.ListMembers(r.Context(), r.PathValue("tenantID"), principal.ID)
		if err != nil {
			writeTenantManagementError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"members": items})
	})
}

func tenantMemberAddHandler(service users.TenantService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := tenantServiceFromRequest(w, r, service)
		if !ok {
			return
		}
		var payload tenantMemberPayload
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		item, err := service.AddMember(r.Context(), principal.ID, r.PathValue("tenantID"), users.MemberMutation{Email: payload.Email, Role: payload.Role})
		if err != nil {
			writeTenantManagementError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, item)
	})
}

func tenantMemberUpdateHandler(service users.TenantService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := tenantServiceFromRequest(w, r, service)
		if !ok {
			return
		}
		var payload tenantMemberPayload
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		item, err := service.UpdateMember(r.Context(), principal.ID, r.PathValue("tenantID"), r.PathValue("userID"), users.MemberMutation{Role: payload.Role, Status: payload.Status})
		if err != nil {
			writeTenantManagementError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	})
}

func tenantMemberRemoveHandler(service users.TenantService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := tenantServiceFromRequest(w, r, service)
		if !ok {
			return
		}
		if err := service.RemoveMember(r.Context(), principal.ID, r.PathValue("tenantID"), r.PathValue("userID")); err != nil {
			writeTenantManagementError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
	})
}

func tenantProjectsListHandler(service users.TenantService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := tenantServiceFromRequest(w, r, service)
		if !ok {
			return
		}
		items, err := service.ListProjects(r.Context(), r.PathValue("tenantID"), principal.ID)
		if err != nil {
			writeTenantManagementError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"projects": items})
	})
}

func tenantProjectCreateHandler(service users.TenantService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := tenantServiceFromRequest(w, r, service)
		if !ok {
			return
		}
		var payload projectPayload
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		item, err := service.CreateProject(r.Context(), principal.ID, r.PathValue("tenantID"), users.ProjectMutation{Name: payload.Name, Slug: payload.Slug, Status: payload.Status})
		if err != nil {
			writeTenantManagementError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, item)
	})
}

func tenantProjectUpdateHandler(service users.TenantService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := tenantServiceFromRequest(w, r, service)
		if !ok {
			return
		}
		var payload projectPayload
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		item, err := service.UpdateProject(r.Context(), principal.ID, r.PathValue("tenantID"), r.PathValue("projectID"), users.ProjectMutation{Name: payload.Name, Slug: payload.Slug, Status: payload.Status})
		if err != nil {
			writeTenantManagementError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	})
}

func tenantProjectDeleteHandler(service users.TenantService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := tenantServiceFromRequest(w, r, service)
		if !ok {
			return
		}
		if err := service.DeleteProject(r.Context(), principal.ID, r.PathValue("tenantID"), r.PathValue("projectID")); err != nil {
			writeTenantManagementError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	})
}

func tenantProjectMembersListHandler(service users.TenantService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := tenantServiceFromRequest(w, r, service)
		if !ok {
			return
		}
		items, err := service.ListProjectMembers(r.Context(), principal.ID, r.PathValue("tenantID"), r.PathValue("projectID"))
		if err != nil {
			writeTenantManagementError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"members": items})
	})
}

func tenantProjectMemberAddHandler(service users.TenantService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := tenantServiceFromRequest(w, r, service)
		if !ok {
			return
		}
		var payload projectMemberPayload
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		item, err := service.AddProjectMember(r.Context(), principal.ID, r.PathValue("tenantID"), r.PathValue("projectID"), users.ProjectMemberMutation{UserID: payload.UserID, Email: payload.Email, Role: payload.Role})
		if err != nil {
			writeTenantManagementError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, item)
	})
}

func tenantProjectMemberUpdateHandler(service users.TenantService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := tenantServiceFromRequest(w, r, service)
		if !ok {
			return
		}
		var payload projectMemberPayload
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		item, err := service.UpdateProjectMember(r.Context(), principal.ID, r.PathValue("tenantID"), r.PathValue("projectID"), r.PathValue("userID"), users.ProjectMemberMutation{Role: payload.Role})
		if err != nil {
			writeTenantManagementError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	})
}

func tenantProjectMemberRemoveHandler(service users.TenantService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := tenantServiceFromRequest(w, r, service)
		if !ok {
			return
		}
		if err := service.RemoveProjectMember(r.Context(), principal.ID, r.PathValue("tenantID"), r.PathValue("projectID"), r.PathValue("userID")); err != nil {
			writeTenantManagementError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
	})
}

func writeTenantManagementError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, users.ErrTenantAccessDenied):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "TENANT_MANAGEMENT_FORBIDDEN"})
	case errors.Is(err, users.ErrMemberNotFound), errors.Is(err, users.ErrProjectNotFound), errors.Is(err, users.ErrProjectMemberNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "TENANT_RESOURCE_NOT_FOUND"})
	case errors.Is(err, users.ErrMemberExists), errors.Is(err, users.ErrProjectExists), errors.Is(err, users.ErrProjectMemberExists):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "TENANT_RESOURCE_EXISTS"})
	case errors.Is(err, users.ErrLastTenantOwner):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "LAST_TENANT_OWNER_PROTECTED"})
	case errors.Is(err, users.ErrLastProjectAdmin):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "LAST_PROJECT_ADMIN_PROTECTED"})
	case errors.Is(err, users.ErrMemberInvalid), errors.Is(err, users.ErrProjectInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_TENANT_REQUEST"})
	case errors.Is(err, users.ErrUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "TENANT_MANAGEMENT_UNAVAILABLE"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "TENANT_MANAGEMENT_UNAVAILABLE"})
	}
}

func writeProfileError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, users.ErrCurrentPasswordInvalid):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "CURRENT_PASSWORD_INVALID"})
	case errors.Is(err, users.ErrPasswordInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_PASSWORD"})
	case errors.Is(err, users.ErrEmailExists):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "EMAIL_ALREADY_EXISTS"})
	case errors.Is(err, users.ErrInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_PROFILE_REQUEST"})
	case errors.Is(err, users.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "PROFILE_NOT_FOUND"})
	case errors.Is(err, users.ErrUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "PROFILE_UNAVAILABLE"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "PROFILE_UNAVAILABLE"})
	}
}

type tokenCreatePayload struct {
	TenantID       string         `json:"tenant_id"`
	ProjectID      string         `json:"project_id"`
	Name           string         `json:"name"`
	AllowedModels  []string       `json:"allowed_models"`
	AllowedIPs     []string       `json:"allowed_ips"`
	AllowedDomains []string       `json:"allowed_domains"`
	RateLimit      map[string]any `json:"rate_limit"`
	ExpiresAt      *time.Time     `json:"expires_at"`
	GroupID        string         `json:"group_id"`
}

func consoleProfileHandler(service users.AdminService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		profileService, ok := service.(users.ProfileService)
		if !ok || profileService == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "PROFILE_UNAVAILABLE"})
			return
		}
		profile, err := profileService.GetProfile(r.Context(), principal.ID)
		if err != nil {
			writeProfileError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, consoleProfileResponse(principal, profile))
	}
}

func consoleProfileResponse(principal *auth.Principal, profile users.Profile) map[string]any {
	projectIDs := make([]string, 0, len(principal.ProjectIDs))
	for projectID := range principal.ProjectIDs {
		projectIDs = append(projectIDs, projectID)
	}
	sort.Strings(projectIDs)
	permissions := make([]string, 0, len(principal.Permissions))
	for permission := range principal.Permissions {
		permissions = append(permissions, permission)
	}
	sort.Strings(permissions)
	return map[string]any{
		"id":            profile.ID,
		"type":          principal.Type,
		"audience":      principal.Audience,
		"email":         profile.Email,
		"display_name":  profile.DisplayName,
		"status":        profile.Status,
		"created_at":    profile.CreatedAt,
		"last_login_at": profile.LastLoginAt,
		"tenant_id":     principal.TenantID,
		"roles":         principal.Roles,
		"permissions":   permissions,
		"project_roles": principal.ProjectRoles,
		"project_ids":   projectIDs,
	}
}

type consoleProfilePayload struct {
	DisplayName string `json:"display_name"`
}

type consoleEmailPayload struct {
	Email           string `json:"email"`
	CurrentPassword string `json:"current_password"`
}

type consolePasswordPayload struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func consoleProfileUpdateHandler(service users.AdminService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		profileService, ok := service.(users.ProfileService)
		if !ok || profileService == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "PROFILE_UNAVAILABLE"})
			return
		}
		var payload consoleProfilePayload
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_PROFILE_REQUEST"})
			return
		}
		profile, err := profileService.UpdateProfile(r.Context(), principal.ID, payload.DisplayName)
		if err != nil {
			writeProfileError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, consoleProfileResponse(principal, profile))
	}
}

func consoleProfileEmailHandler(service users.AdminService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		profileService, ok := service.(users.ProfileService)
		if !ok || profileService == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "PROFILE_UNAVAILABLE"})
			return
		}
		var payload consoleEmailPayload
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_PROFILE_REQUEST"})
			return
		}
		profile, err := profileService.ChangeEmail(r.Context(), principal.ID, payload.CurrentPassword, payload.Email)
		if err != nil {
			writeProfileError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, consoleProfileResponse(principal, profile))
	}
}

func consoleProfilePasswordHandler(service users.AdminService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		profileService, ok := service.(users.ProfileService)
		if !ok || profileService == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "PROFILE_UNAVAILABLE"})
			return
		}
		var payload consolePasswordPayload
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_PROFILE_REQUEST"})
			return
		}
		if err := profileService.ChangePassword(r.Context(), principal.ID, payload.CurrentPassword, payload.NewPassword); err != nil {
			writeProfileError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "password_changed"})
	}
}

func adminTokenCreationDisabledHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "ADMIN_TOKEN_CREATION_DISABLED"})
	})
}

func tokenConsoleListHandler(service tokens.ConsoleService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "TOKENS_UNAVAILABLE"})
			return
		}
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		items, err := service.ListOwned(r.Context(), principal.TenantID, principal.ID)
		if err != nil {
			writeTokenConsoleError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"tokens": items})
	})
}

func tokenGroupListHandler(service groups.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lister, ok := service.(groups.TokenGroupLister)
		if !ok || lister == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "TOKEN_GROUPS_UNAVAILABLE"})
			return
		}
		items, err := lister.ListTokenGroups(r.Context())
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "TOKEN_GROUPS_UNAVAILABLE"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"groups": items})
	})
}

func modelStatusFeatureEnabled(ctx context.Context, service auth.SecuritySettingsProvider) (bool, error) {
	provider, ok := service.(adminsettings.FeatureSettingsProvider)
	if !ok || provider == nil {
		// Keep lightweight embeddings and unit-test handlers compatible. The
		// production server always wires the database-backed settings service.
		return true, nil
	}
	settings, err := provider.GetFeatureSettings(ctx)
	if err != nil {
		return false, err
	}
	return settings.ModelStatusEnabled, nil
}

func modelStatusHandler(service groups.Service, featureService auth.SecuritySettingsProvider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enabled, err := modelStatusFeatureEnabled(r.Context(), featureService)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "MODEL_STATUS_UNAVAILABLE"})
			return
		}
		if !enabled {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "MODEL_STATUS_DISABLED"})
			return
		}
		lister, ok := service.(groups.ModelStatusLister)
		if !ok || lister == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "MODEL_STATUS_UNAVAILABLE"})
			return
		}
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok || strings.TrimSpace(principal.TenantID) == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		report, err := lister.ListModelStatuses(r.Context(), principal.TenantID)
		if err != nil {
			if errors.Is(err, groups.ErrUnavailable) {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "MODEL_STATUS_UNAVAILABLE"})
				return
			}
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "MODEL_STATUS_UNAVAILABLE"})
			return
		}
		writeJSON(w, http.StatusOK, report)
	})
}

func adminModelStatusHandler(service groups.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lister, ok := service.(groups.AdminModelStatusLister)
		if !ok || lister == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "MODEL_STATUS_UNAVAILABLE"})
			return
		}
		report, err := lister.ListAdminModelStatuses(r.Context())
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "MODEL_STATUS_UNAVAILABLE"})
			return
		}
		writeJSON(w, http.StatusOK, report)
	})
}

func tokenConsoleCreateHandler(service tokens.ConsoleService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "TOKENS_UNAVAILABLE"})
			return
		}
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		var payload tokenCreatePayload
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		projectID := strings.TrimSpace(payload.ProjectID)
		if projectID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_TOKEN_REQUEST"})
			return
		}
		if _, allowed := principal.ProjectIDs[projectID]; !allowed {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "TOKEN_RESOURCE_NOT_FOUND"})
			return
		}
		issued, err := service.Create(r.Context(), tokens.CreateRequest{
			TenantID:       principal.TenantID,
			ProjectID:      projectID,
			CreatedBy:      principal.ID,
			Name:           payload.Name,
			AllowedModels:  payload.AllowedModels,
			AllowedIPs:     payload.AllowedIPs,
			AllowedDomains: payload.AllowedDomains,
			RateLimit:      payload.RateLimit,
			ExpiresAt:      payload.ExpiresAt,
			GroupID:        payload.GroupID,
		})
		if err != nil {
			writeTokenConsoleError(w, err)
			return
		}
		writeIssuedToken(w, issued)
	})
}

func tokenConsoleRevokeHandler(service tokens.ConsoleService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "TOKENS_UNAVAILABLE"})
			return
		}
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		if err := service.RevokeOwned(r.Context(), r.PathValue("tokenID"), principal.TenantID, principal.ID); err != nil {
			writeTokenConsoleError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
	})
}

func tokenRevokeHandler(service tokens.AdminService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		revoker, ok := service.(tokens.AdminRevoker)
		if !ok || revoker == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "TOKENS_UNAVAILABLE"})
			return
		}
		if err := revoker.Revoke(r.Context(), r.PathValue("tokenID")); err != nil {
			writeTokenAdminError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
	})
}

func writeIssuedToken(w http.ResponseWriter, issued tokens.IssuedToken) {
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":           issued.ID,
		"token":        issued.Plaintext,
		"token_prefix": issued.Prefix,
		"expires_at":   issued.ExpiresAt,
		"warning":      "The token secret is shown only once. Store it securely.",
	})
}

func writeTokenConsoleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, tokens.ErrConsoleInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_TOKEN_REQUEST"})
	case errors.Is(err, tokens.ErrNetworkAllowlistInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_TOKEN_NETWORK_ALLOWLIST"})
	case errors.Is(err, tokens.ErrTokenNotFound), errors.Is(err, tokens.ErrGroupNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "TOKEN_RESOURCE_NOT_FOUND"})
	case errors.Is(err, tokens.ErrConsoleUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "TOKENS_UNAVAILABLE"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "TOKENS_UNAVAILABLE"})
	}
}

type tokenGroupUpdatePayload struct {
	GroupID string `json:"group_id"`
}

func tokenGroupUpdateHandler(service tokens.AdminService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "TOKENS_UNAVAILABLE"})
			return
		}
		var payload tokenGroupUpdatePayload
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		item, err := service.SetGroup(r.Context(), r.PathValue("tokenID"), payload.GroupID)
		if err != nil {
			writeTokenAdminError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	})
}

func writeTokenAdminError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, tokens.ErrAdminInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_TOKEN_REQUEST"})
	case errors.Is(err, tokens.ErrNetworkAllowlistInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_TOKEN_NETWORK_ALLOWLIST"})
	case errors.Is(err, tokens.ErrTokenNotFound), errors.Is(err, tokens.ErrGroupNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "TOKEN_RESOURCE_NOT_FOUND"})
	case errors.Is(err, tokens.ErrAdminUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "TOKENS_UNAVAILABLE"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "TOKENS_UNAVAILABLE"})
	}
}

func billingPriceListHandler(service billing.AdminService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "BILLING_UNAVAILABLE"})
			return
		}
		if matrix, ok := service.(billing.PriceMatrixReader); ok {
			prices, err := matrix.ListPriceMatrix(r.Context())
			if err != nil {
				writeBillingManagementError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"prices": prices})
			return
		}
		prices, err := service.ListPrices(r.Context())
		if err != nil {
			writeBillingManagementError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"prices": prices})
	})
}

func billingPricePublishHandler(service billing.AdminService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "BILLING_UNAVAILABLE"})
			return
		}
		var payload billing.PublishPriceRequest
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		price, err := service.PublishPrice(r.Context(), principal.ID, payload)
		if err != nil {
			writeBillingManagementError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, price)
	})
}

func usageRecordsHandler(service billing.AdminService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reporter, ok := service.(billing.UsageReporter)
		if !ok || reporter == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "USAGE_REPORT_UNAVAILABLE"})
			return
		}
		query, err := usageReportQuery(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_USAGE_QUERY"})
			return
		}
		report, err := reporter.ListUsageRecords(r.Context(), query)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "USAGE_REPORT_UNAVAILABLE"})
			return
		}
		writeJSON(w, http.StatusOK, report)
	})
}

func consoleUsageHandler(service billing.AdminService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reporter, ok := service.(billing.UsageReporter)
		if !ok || reporter == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "USAGE_REPORT_UNAVAILABLE"})
			return
		}
		query, err := usageReportQuery(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_USAGE_QUERY"})
			return
		}
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok || principal.TenantID == "" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "RESOURCE_NOT_FOUND"})
			return
		}
		// The path and session, never a client query parameter, define scope.
		if r.PathValue("tenantID") != principal.TenantID {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "RESOURCE_NOT_FOUND"})
			return
		}
		query.TenantID = principal.TenantID
		query.ProjectIDs = make([]string, 0, len(principal.ProjectIDs))
		for projectID := range principal.ProjectIDs {
			query.ProjectIDs = append(query.ProjectIDs, projectID)
		}
		sort.Strings(query.ProjectIDs)
		if len(query.ProjectIDs) == 0 {
			// An authenticated tenant user with no project grant must see no
			// usage rows, never the whole tenant's data.
			query.ProjectIDs = []string{"no-project-access"}
		}
		report, err := reporter.ListUsageRecords(r.Context(), query)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "USAGE_REPORT_UNAVAILABLE"})
			return
		}
		writeJSON(w, http.StatusOK, report)
	})
}

func auditRecordsHandler(service audit.Reader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "AUDIT_UNAVAILABLE"})
			return
		}
		limit, offset, err := reportPagination(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_AUDIT_QUERY"})
			return
		}
		from, to, err := reportTimeRange(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_AUDIT_QUERY"})
			return
		}
		report, err := service.List(r.Context(), audit.Query{
			Limit: limit, Offset: offset, Action: r.URL.Query().Get("action"),
			ResourceType: r.URL.Query().Get("resource_type"), Result: r.URL.Query().Get("result"),
			Search: r.URL.Query().Get("search"), From: from, To: to,
		})
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "AUDIT_UNAVAILABLE"})
			return
		}
		writeJSON(w, http.StatusOK, report)
	})
}

func operationsSnapshotHandler(service billing.AdminService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reporter, ok := service.(billing.OperationsReporter)
		if !ok || reporter == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "OPERATIONS_UNAVAILABLE"})
			return
		}
		snapshot, err := reporter.GetOperationsSnapshot(r.Context())
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "OPERATIONS_UNAVAILABLE"})
			return
		}
		writeJSON(w, http.StatusOK, snapshot)
	})
}

func financeReportHandler(service billing.AdminService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reporter, ok := service.(billing.FinanceReporter)
		if !ok || reporter == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "FINANCE_REPORT_UNAVAILABLE"})
			return
		}
		query, err := financeReportQuery(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_FINANCE_QUERY"})
			return
		}
		report, err := reporter.ListFinanceReport(r.Context(), query)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "FINANCE_REPORT_UNAVAILABLE"})
			return
		}
		writeJSON(w, http.StatusOK, report)
	})
}

func usageReportQuery(r *http.Request) (billing.UsageQuery, error) {
	limit, offset, err := reportPagination(r)
	if err != nil {
		return billing.UsageQuery{}, err
	}
	from, to, err := reportTimeRange(r)
	if err != nil {
		return billing.UsageQuery{}, err
	}
	tenantID := strings.TrimSpace(r.URL.Query().Get("tenant_id"))
	groupID := strings.TrimSpace(r.URL.Query().Get("group_id"))
	if (tenantID != "" && !ids.Valid(tenantID)) || (groupID != "" && !ids.Valid(groupID)) {
		return billing.UsageQuery{}, errors.New("invalid report identifier")
	}
	return billing.UsageQuery{
		Limit:    limit,
		Offset:   offset,
		TenantID: tenantID,
		Model:    r.URL.Query().Get("model"),
		GroupID:  groupID,
		Status:   r.URL.Query().Get("status"),
		Search:   r.URL.Query().Get("search"),
		From:     from,
		To:       to,
	}, nil
}

func financeReportQuery(r *http.Request) (billing.FinanceQuery, error) {
	limit, offset, err := reportPagination(r)
	if err != nil {
		return billing.FinanceQuery{}, err
	}
	from, to, err := reportTimeRange(r)
	if err != nil {
		return billing.FinanceQuery{}, err
	}
	tenantID := strings.TrimSpace(r.URL.Query().Get("tenant_id"))
	if tenantID != "" && !ids.Valid(tenantID) {
		return billing.FinanceQuery{}, errors.New("invalid finance tenant identifier")
	}
	return billing.FinanceQuery{
		Limit:    limit,
		Offset:   offset,
		TenantID: tenantID,
		Currency: r.URL.Query().Get("currency"),
		Search:   r.URL.Query().Get("search"),
		From:     from,
		To:       to,
	}, nil
}

func reportPagination(r *http.Request) (int, int, error) {
	limit := 50
	offset := 0
	var err error
	if value := strings.TrimSpace(r.URL.Query().Get("limit")); value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil || limit < 1 || limit > 200 {
			return 0, 0, errors.New("invalid report limit")
		}
	}
	if value := strings.TrimSpace(r.URL.Query().Get("offset")); value != "" {
		offset, err = strconv.Atoi(value)
		if err != nil || offset < 0 {
			return 0, 0, errors.New("invalid report offset")
		}
	}
	return limit, offset, nil
}

func reportTimeRange(r *http.Request) (*time.Time, *time.Time, error) {
	var from, to *time.Time
	var err error
	if value := strings.TrimSpace(r.URL.Query().Get("from")); value != "" {
		parsed, parseErr := time.Parse(time.RFC3339, value)
		if parseErr != nil {
			return nil, nil, errors.New("invalid report from time")
		}
		from = &parsed
	}
	if value := strings.TrimSpace(r.URL.Query().Get("to")); value != "" {
		parsed, parseErr := time.Parse(time.RFC3339, value)
		if parseErr != nil {
			return nil, nil, errors.New("invalid report to time")
		}
		to = &parsed
	}
	if from != nil && to != nil && !from.Before(*to) {
		return nil, nil, errors.New("invalid report time range")
	}
	return from, to, err
}

func billingAccountReadHandler(service billing.AdminService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "BILLING_UNAVAILABLE"})
			return
		}
		account, err := service.GetPrepaidAccount(
			r.Context(),
			r.PathValue("tenantID"),
			r.URL.Query().Get("currency"),
		)
		if err != nil {
			writeBillingManagementError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, account)
	})
}

func billingAccountCreditHandler(service billing.AdminService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "BILLING_UNAVAILABLE"})
			return
		}
		var payload billing.CreditRequest
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		payload.TenantID = strings.TrimSpace(r.PathValue("tenantID"))
		if strings.TrimSpace(payload.IdempotencyKey) == "" {
			payload.IdempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		}
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		account, err := service.Credit(r.Context(), principal.ID, payload)
		if err != nil {
			writeBillingManagementError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, account)
	})
}

type billingSettlementPayload struct {
	InputTokens       int64                `json:"input_tokens"`
	OutputTokens      int64                `json:"output_tokens"`
	CachedInputTokens int64                `json:"cached_input_tokens"`
	ReasoningTokens   int64                `json:"reasoning_tokens"`
	Metrics           billing.MeteredUsage `json:"metrics"`
	PricingTier       string               `json:"pricing_tier"`
	Raw               json.RawMessage      `json:"raw"`
	ProviderRequestID string               `json:"provider_request_id"`
}

func billingSettlementHandler(service billing.AdminService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		settler, ok := service.(billing.SettlementReconciler)
		if !ok || settler == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "BILLING_UNAVAILABLE"})
			return
		}
		var payload billingSettlementPayload
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		err := settler.SettleByModelRequestID(r.Context(), r.PathValue("requestID"), billing.Usage{
			InputTokens: payload.InputTokens, OutputTokens: payload.OutputTokens,
			CachedInputTokens: payload.CachedInputTokens, ReasoningTokens: payload.ReasoningTokens,
			Metrics: payload.Metrics, PricingTier: payload.PricingTier, Source: "reconciliation", Raw: payload.Raw,
		}, payload.ProviderRequestID)
		if err != nil {
			writeBillingManagementError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "settled"})
	})
}

func writeBillingManagementError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, billing.ErrInvalidRequest), errors.Is(err, billing.ErrInvalidPrice):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_BILLING_REQUEST"})
	case errors.Is(err, billing.ErrModelNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "MODEL_NOT_FOUND"})
	case errors.Is(err, billing.ErrAccountNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "BILLING_ACCOUNT_NOT_FOUND"})
	case errors.Is(err, billing.ErrDuplicateTransaction):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "DUPLICATE_BILLING_TRANSACTION"})
	case errors.Is(err, billing.ErrReservationClosed), errors.Is(err, billing.ErrReservationNotFound), errors.Is(err, billing.ErrSettlementPending):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "BILLING_SETTLEMENT_INVALID"})
	case errors.Is(err, billing.ErrUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "BILLING_UNAVAILABLE"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "BILLING_UNAVAILABLE"})
	}
}

type channelManagementService interface {
	ListChannels(context.Context) ([]relay.ChannelSummary, error)
	CreateChannel(context.Context, string, relay.ChannelMutation) (relay.ChannelSummary, error)
	UpdateChannel(context.Context, string, string, relay.ChannelMutation) (relay.ChannelSummary, error)
	SetChannelStatus(context.Context, string, string, string) (relay.ChannelSummary, error)
	DeleteChannel(context.Context, string, string) error
}

type modelDiscoveryService interface {
	DiscoverModels(context.Context, relay.ModelDiscoveryRequest) ([]relay.DiscoveredModel, error)
}

func channelManager(w http.ResponseWriter, service relay.ChatCompletionService) (channelManagementService, bool) {
	if service == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "CHANNELS_UNAVAILABLE"})
		return nil, false
	}
	manager, ok := service.(channelManagementService)
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "CHANNELS_UNAVAILABLE"})
		return nil, false
	}
	return manager, true
}

func modelDiscoveryManager(w http.ResponseWriter, service relay.ChatCompletionService) (modelDiscoveryService, bool) {
	if service == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "CHANNELS_UNAVAILABLE"})
		return nil, false
	}
	manager, ok := service.(modelDiscoveryService)
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "CHANNELS_UNAVAILABLE"})
		return nil, false
	}
	return manager, true
}

func principalID(r *http.Request) string {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		return ""
	}
	return principal.ID
}

func writeChannelError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, relay.ErrInvalidRequest):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
	case errors.Is(err, relay.ErrCredentialRequired):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "CHANNEL_CREDENTIAL_REQUIRED"})
	case errors.Is(err, relay.ErrCredentialUnavailable):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "CHANNEL_CREDENTIAL_INVALID"})
	case errors.Is(err, relay.ErrChannelNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "CHANNEL_NOT_FOUND"})
	case errors.Is(err, relay.ErrUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "CHANNELS_UNAVAILABLE"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "CHANNELS_UNAVAILABLE"})
	}
}

func writeModelDiscoveryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, relay.ErrInvalidRequest):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
	case errors.Is(err, relay.ErrCredentialRequired):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "CHANNEL_CREDENTIAL_REQUIRED"})
	case errors.Is(err, relay.ErrCredentialUnavailable):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "CHANNEL_CREDENTIAL_INVALID"})
	case errors.Is(err, relay.ErrChannelNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "CHANNEL_NOT_FOUND"})
	case errors.Is(err, relay.ErrProviderUnsupported):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "PROVIDER_UNSUPPORTED"})
	case errors.Is(err, relay.ErrModelDiscoveryFailed):
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "MODEL_DISCOVERY_FAILED"})
	case errors.Is(err, relay.ErrUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "CHANNELS_UNAVAILABLE"})
	default:
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "MODEL_DISCOVERY_FAILED"})
	}
}

func writeRelayError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, billing.ErrInsufficientBalance):
		writeJSON(w, http.StatusPaymentRequired, map[string]string{"error": "INSUFFICIENT_BALANCE"})
	case errors.Is(err, billing.ErrAccountNotFound):
		writeJSON(w, http.StatusPaymentRequired, map[string]string{"error": "BILLING_ACCOUNT_NOT_FOUND"})
	case errors.Is(err, billing.ErrPriceNotConfigured):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "PRICE_NOT_CONFIGURED"})
	case errors.Is(err, billing.ErrDuplicateRequest):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "DUPLICATE_REQUEST"})
	case errors.Is(err, tokens.ErrTokenRateLimited):
		w.Header().Set("Retry-After", "60")
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "TOKEN_RATE_LIMITED"})
	case errors.Is(err, billing.ErrReservationClosed), errors.Is(err, billing.ErrReservationNotFound):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "BILLING_RESERVATION_INVALID"})
	case errors.Is(err, billing.ErrUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "BILLING_UNAVAILABLE"})
	case errors.Is(err, relay.ErrInvalidRequest):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
	case errors.Is(err, relay.ErrUnsupportedFeature):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "RELAY_FEATURE_UNSUPPORTED"})
	case errors.Is(err, relay.ErrUsageUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "UPSTREAM_USAGE_UNAVAILABLE"})
	case errors.Is(err, relay.ErrStreamingUnsupported):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "STREAMING_UNSUPPORTED"})
	case errors.Is(err, relay.ErrModelNotAllowed):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "MODEL_NOT_ALLOWED"})
	case errors.Is(err, relay.ErrModelNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "MODEL_NOT_AVAILABLE"})
	case errors.Is(err, relay.ErrGroupRateLimited):
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "GROUP_RATE_LIMITED"})
	case errors.Is(err, relay.ErrGroupUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "GROUP_UNAVAILABLE"})
	case errors.Is(err, relay.ErrCredentialUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "UPSTREAM_CREDENTIAL_UNAVAILABLE"})
	case errors.Is(err, relay.ErrProviderUnsupported):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "UPSTREAM_PROVIDER_UNSUPPORTED"})
	case errors.Is(err, relay.ErrUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "RELAY_UNAVAILABLE"})
	case errors.Is(err, relay.ErrUpstream):
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "UPSTREAM_REQUEST_FAILED"})
	default:
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "UPSTREAM_REQUEST_FAILED"})
	}
}

func writeLoginError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials), errors.Is(err, auth.ErrMFAInvalid):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_INVALID"})
	case errors.Is(err, auth.ErrLoginThrottled):
		w.Header().Set("Retry-After", "60")
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "AUTH_THROTTLED"})
	case errors.Is(err, auth.ErrMFARequired):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "MFA_REQUIRED"})
	case errors.Is(err, auth.ErrEmailVerificationRequired):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "EMAIL_VERIFICATION_REQUIRED"})
	case errors.Is(err, auth.ErrMFAUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "MFA_UNAVAILABLE"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "AUTH_UNAVAILABLE"})
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	return ensureSingleJSONValue(decoder)
}

func decodeRelayJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	return ensureSingleJSONValue(decoder)
}

func ensureSingleJSONValue(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("request contains multiple JSON values")
	}
	return nil
}

func clientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	peer := strings.TrimSpace(r.RemoteAddr)
	if host, _, err := net.SplitHostPort(peer); err == nil {
		peer = host
	}
	peerAddress, err := netip.ParseAddr(peer)
	if err != nil || !isTrustedProxy(peerAddress) {
		return peer
	}

	forwardedFor := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for index := len(forwardedFor) - 1; index >= 0; index-- {
		candidate, parseErr := netip.ParseAddr(strings.TrimSpace(forwardedFor[index]))
		if parseErr != nil {
			return peer
		}
		if !isTrustedProxy(candidate) {
			return candidate.String()
		}
	}
	return peer
}

func isTrustedProxy(address netip.Addr) bool {
	for _, prefix := range trustedProxyCIDRs() {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func trustedProxyCIDRs() []netip.Prefix {
	raw := strings.TrimSpace(os.Getenv("TRUSTED_PROXY_CIDRS"))
	if raw == "" {
		return nil
	}
	prefixes := make([]netip.Prefix, 0, 2)
	for _, item := range strings.Split(raw, ",") {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(item))
		if err == nil {
			prefixes = append(prefixes, prefix)
		}
	}
	return prefixes
}

func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := newRequestID()
		w.Header().Set("X-Request-ID", requestID)
		*r = *r.WithContext(relay.WithRequestMetadata(
			r.Context(),
			relay.RequestMetadata{
				RequestID:      requestID,
				IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key")),
				Endpoint:       r.URL.Path,
				ClientIP:       clientIP(r),
				RequestType:    "sync",
			},
		))
		next.ServeHTTP(w, r)
	})
}

func relayRequestContext(r *http.Request, requestType string) context.Context {
	metadata := relay.RequestMetadataFromContext(r.Context())
	metadata.RequestType = requestType
	return relay.WithRequestMetadata(r.Context(), metadata)
}

func newRequestID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "request-id-unavailable"
	}
	return hex.EncodeToString(raw[:])
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
