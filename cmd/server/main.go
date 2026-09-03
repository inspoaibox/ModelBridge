package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"ai-token/internal/adminsettings"
	"ai-token/internal/audit"
	"ai-token/internal/auth"
	"ai-token/internal/billing"
	"ai-token/internal/config"
	"ai-token/internal/db"
	"ai-token/internal/enterprise"
	"ai-token/internal/groups"
	"ai-token/internal/httpapi"
	"ai-token/internal/mfa"
	"ai-token/internal/modelprices"
	"ai-token/internal/models"
	"ai-token/internal/payments"
	"ai-token/internal/relay"
	"ai-token/internal/tokens"
	"ai-token/internal/users"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var (
		dbConn              *sql.DB
		tokenResolver       auth.Resolver
		sessionResolver     auth.SessionResolver
		services            auth.Services
		relayService        relay.ChatCompletionService
		billingService      *billing.SQLService
		groupService        groups.Service
		tokenAdminService   tokens.AdminService
		consoleTokenService tokens.ConsoleService
		modelCatalog        models.Catalog
		userAdminService    users.AdminService
		priceSyncService    modelprices.SyncService
		auditReader         audit.Reader
		auditWriter         audit.Writer
		enterpriseService   enterprise.AdminService
		paymentService      payments.Service
	)
	if cfg.DatabaseURL != "" {
		if len(cfg.MFAEncryptionKey) == 0 {
			log.Fatal("MFA_ENCRYPTION_KEY is required when DATABASE_URL is configured")
		}
		tokenHasher, err := tokens.NewHasher(cfg.TokenPepper)
		if err != nil {
			log.Fatal(err)
		}
		sessionHasher, err := tokens.NewHasher(cfg.SessionPepper)
		if err != nil {
			log.Fatal(err)
		}
		mfaBox, err := mfa.NewSecretBox(cfg.MFAEncryptionKey)
		if err != nil {
			log.Fatal(err)
		}
		openCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		dbConn, err = db.Open(openCtx, cfg.DatabaseURL)
		cancel()
		if err != nil {
			log.Fatal(err)
		}
		defer dbConn.Close()

		migrateCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err = db.Migrate(migrateCtx, dbConn, cfg.MigrationsDir)
		cancel()
		if err != nil {
			log.Fatal(err)
		}
		writer, err := audit.NewSQLWriter(dbConn)
		if err != nil {
			log.Fatal(err)
		}
		auditWriter = writer
		auditReader = writer

		resolver, err := auth.NewSQLResolver(dbConn, tokenHasher, sessionHasher)
		if err != nil {
			log.Fatal(err)
		}
		sessions, err := auth.NewSessionIssuer(dbConn, sessionHasher, cfg.SessionTTL)
		if err != nil {
			log.Fatal(err)
		}
		settingsService, err := adminsettings.New(dbConn, mfaBox)
		if err != nil {
			log.Fatal(err)
		}
		if err := settingsService.EnsureFeatureDefaults(ctx, cfg.RegistrationEnabled); err != nil {
			log.Fatal(err)
		}
		login, err := auth.NewSQLLoginService(
			dbConn,
			sessions,
			tokenHasher,
			settingsService,
			mfaBox,
			cfg.LoginMaxFailures,
			cfg.LoginWindow,
			cfg.LoginLockDuration,
		)
		if err != nil {
			log.Fatal(err)
		}
		passwordReset, err := auth.NewSQLPasswordResetService(
			dbConn,
			tokenHasher,
			30*time.Minute,
			cfg.LoginMaxFailures,
			cfg.LoginWindow,
			cfg.LoginLockDuration,
		)
		if err != nil {
			log.Fatal(err)
		}
		smtpNotifier := auth.NewDynamicSMTPPasswordResetNotifier(
			settingsService,
			auth.SMTPSettings{Address: cfg.SMTPAddress, From: cfg.SMTPFrom, Username: cfg.SMTPUsername, Password: cfg.SMTPPassword, BaseURL: cfg.PublicBaseURL},
		)
		passwordResetNotifier := auth.PasswordResetNotifier(smtpNotifier)
		mfaEnrollment, err := mfa.NewEnrollmentService(dbConn, mfaBox, 10*time.Minute)
		if err != nil {
			log.Fatal(err)
		}
		channelRouter, err := relay.NewSQLChannelRouter(dbConn, mfaBox)
		if err != nil {
			log.Fatal(err)
		}
		credentialResolver, err := relay.NewSQLCredentialResolver(
			dbConn,
			mfaBox,
			relay.EnvCredentialResolver{},
		)
		if err != nil {
			log.Fatal(err)
		}
		billingService, err = billing.NewSQLService(dbConn)
		if err != nil {
			log.Fatal(err)
		}
		groupService, err = groups.NewSQLService(dbConn)
		if err != nil {
			log.Fatal(err)
		}
		tokenIssuer, err := tokens.NewIssuer(dbConn, tokenHasher)
		if err != nil {
			log.Fatal(err)
		}
		tokenAdminService, err = tokens.NewAdminService(dbConn, tokenIssuer)
		if err != nil {
			log.Fatal(err)
		}
		consoleTokenService, err = tokens.NewConsoleService(dbConn, tokenIssuer)
		if err != nil {
			log.Fatal(err)
		}
		modelCatalog, err = models.NewCatalog(dbConn)
		if err != nil {
			log.Fatal(err)
		}
		userAdminService, err = users.NewAdminService(dbConn)
		if err != nil {
			log.Fatal(err)
		}
		priceSyncService, err = modelprices.NewSyncService(dbConn)
		if err != nil {
			log.Fatal(err)
		}
		enterpriseSQLService, err := enterprise.NewSQLService(dbConn, mfaBox)
		if err != nil {
			log.Fatal(err)
		}
		enterpriseService = enterpriseSQLService
		paymentSQLService, err := payments.NewSQLService(dbConn, mfaBox, billingService)
		if err != nil {
			log.Fatal(err)
		}
		paymentService = paymentSQLService
		// Registration is initialized regardless of its current availability.
		// The database-backed feature switch is checked for each request, so
		// administrators can open or close registration without a restart.
		registrationService, err := auth.NewSQLRegistrationServiceWithNotifier(
			dbConn,
			tokenHasher,
			cfg.LoginMaxFailures,
			cfg.LoginWindow,
			cfg.LoginLockDuration,
			smtpNotifier,
		)
		if err != nil {
			log.Fatal(err)
		}
		relayService, err = relay.NewServiceWithTokenLimiter(
			channelRouter,
			credentialResolver,
			relay.DefaultProviders(),
			billingService,
			tokenIssuer,
		)
		if err != nil {
			log.Fatal(err)
		}
		go billingService.RunReaper(ctx, time.Minute)
		if reconciler, ok := relayService.(interface {
			ReconcileMediaJobs(context.Context) error
		}); ok {
			go runMediaJobReconciler(ctx, reconciler)
		}
		if monitorService, ok := groupService.(groups.ModelMonitorService); ok {
			if prober, ok := relayService.(relay.ModelProbeService); ok {
				go runModelMonitorProber(ctx, monitorService, prober)
			}
		}
		tokenResolver = resolver
		sessionResolver = resolver
		services = auth.Services{
			Login:                 login,
			Registration:          registrationService,
			PasswordReset:         passwordReset,
			PasswordResetNotifier: passwordResetNotifier,
			MFA:                   mfaEnrollment,
			StepUpMFA:             mfaEnrollment,
			SecuritySettings:      settingsService,
		}
	}

	handler := httpapi.NewWithRelayAndGroupsAndTokensAndConsoleTokensAndModelCatalogAndUsersAndPriceSyncAndAuditAndCommercial(
		auth.NewCredentialMiddleware(tokenResolver, sessionResolver),
		&services,
		relayService,
		cfg.CookieSecure,
		cfg.WebDir,
		tokenAdminService,
		groupService,
		consoleTokenService,
		modelCatalog,
		userAdminService,
		priceSyncService,
		auditReader,
		enterpriseService,
		paymentService,
		billingService,
	)
	if auditWriter != nil {
		handler = audit.HTTPMiddleware(handler, auditWriter)
	}
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       cfg.HTTPReadTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		log.Printf("server listening on %s", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server stopped: %v", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}

func runMediaJobReconciler(ctx context.Context, reconciler interface {
	ReconcileMediaJobs(context.Context) error
}) {
	if err := reconciler.ReconcileMediaJobs(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("media job reconciler: %v", err)
	}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := reconciler.ReconcileMediaJobs(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("media job reconciler: %v", err)
			}
		}
	}
}

func runModelMonitorProber(
	ctx context.Context,
	monitors groups.ModelMonitorService,
	prober relay.ModelProbeService,
) {
	run := func() {
		for {
			monitor, err := monitors.ClaimDueActiveModelMonitor(ctx)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					log.Printf("model monitor claim failed: %v", err)
				}
				return
			}
			if monitor == nil {
				return
			}
			status := groups.MonitorProbeSuccess
			var failures []string
			supported := 0
			for _, model := range monitor.ModelNames {
				probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
				err := prober.ProbeModel(probeCtx, monitor.GroupID, model)
				cancel()
				if errors.Is(err, relay.ErrUnsupportedFeature) {
					continue
				}
				supported++
				if err != nil {
					failures = append(failures, model+": "+err.Error())
				}
			}
			if supported == 0 {
				status = groups.MonitorProbeSkipped
			} else if len(failures) > 0 {
				status = groups.MonitorProbeFailed
			}
			probeError := strings.Join(failures, "; ")
			if err := monitors.CompleteActiveModelMonitor(ctx, monitor.ID, status, probeError); err != nil &&
				!errors.Is(err, context.Canceled) {
				log.Printf("model monitor completion failed: %v", err)
			}
		}
	}

	run()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
