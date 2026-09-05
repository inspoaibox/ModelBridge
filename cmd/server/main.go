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
	"sync"
	"syscall"
	"time"

	"ai-token/internal/adminsettings"
	"ai-token/internal/announcements"
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
		announcementService announcements.Service
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
		announcementSQLService, err := announcements.NewSQLService(dbConn)
		if err != nil {
			log.Fatal(err)
		}
		announcementService = announcementSQLService
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
		oauthService, err := auth.NewSQLOAuthService(dbConn, sessions, settingsService)
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
				log.Printf("model monitor prober started")
				go runModelMonitorProber(ctx, monitorService, prober)
			} else {
				log.Printf("model monitor prober unavailable: relay service does not implement ModelProbeService")
			}
		} else {
			log.Printf("model monitor prober unavailable: group service does not implement ModelMonitorService")
		}
		if accountSyncService, ok := relayService.(upstreamAccountSyncService); ok {
			go runUpstreamAccountSync(ctx, accountSyncService)
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
			OAuth:                 oauthService,
		}
	}

	handler := httpapi.NewWithRelayAndGroupsAndTokensAndConsoleTokensAndModelCatalogAndUsersAndPriceSyncAndAuditAndCommercialAndAnnouncements(
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
		announcementService,
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

type upstreamAccountSyncService interface {
	ListChannels(context.Context) ([]relay.ChannelSummary, error)
	relay.ChannelAccountSyncer
}

func runUpstreamAccountSync(ctx context.Context, service upstreamAccountSyncService) {
	if service == nil {
		return
	}

	run := func() {
		runUpstreamAccountSyncOnce(ctx, service)
	}

	// Refresh immediately after the server starts, then keep the operational
	// snapshot current once per minute. Account sync is intentionally isolated
	// from relay traffic, billing, and channel health.
	run()
	ticker := time.NewTicker(time.Minute)
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

func runUpstreamAccountSyncOnce(ctx context.Context, service upstreamAccountSyncService) {
	if service == nil {
		return
	}
	listCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	channels, err := service.ListChannels(listCtx)
	cancel()
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			log.Printf("upstream account list failed: %v", err)
		}
		return
	}

	const workerCount = 4
	jobs := make(chan relay.ChannelSummary)
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go func() {
			defer workers.Done()
			for channel := range jobs {
				if !shouldSyncUpstreamAccount(channel) {
					continue
				}
				syncCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
				_, syncErr := service.SyncChannelAccount(syncCtx, "", channel.ID)
				cancel()
				if syncErr != nil && !errors.Is(syncErr, context.Canceled) {
					log.Printf("upstream account sync failed for channel %s: %v", channel.ID, syncErr)
				}
			}
		}()
	}

	for _, channel := range channels {
		if !shouldSyncUpstreamAccount(channel) {
			continue
		}
		select {
		case jobs <- channel:
		case <-ctx.Done():
			close(jobs)
			workers.Wait()
			return
		}
	}
	close(jobs)
	workers.Wait()
}

func shouldSyncUpstreamAccount(channel relay.ChannelSummary) bool {
	if !channel.HasUpstreamAccountCredential {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(channel.UpstreamIntegration)) {
	case relay.UpstreamIntegrationNewAPI, relay.UpstreamIntegrationSub2API:
		return true
	default:
		return false
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
	const (
		workerCount  = 4
		pollInterval = 5 * time.Second
	)
	run := func() {
		jobs := make(chan *groups.ModelMonitor)
		var workers sync.WaitGroup
		workers.Add(workerCount)
		for i := 0; i < workerCount; i++ {
			go func() {
				defer workers.Done()
				for monitor := range jobs {
					status, probeError := probeModelMonitor(ctx, prober, monitor)
					if err := monitors.CompleteActiveModelMonitor(ctx, monitor.ID, status, probeError); err != nil &&
						!errors.Is(err, context.Canceled) {
						log.Printf("model monitor completion failed for %s: %v", monitor.ID, err)
					}
				}
			}()
		}

		for {
			monitor, err := monitors.ClaimDueActiveModelMonitor(ctx)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					log.Printf("model monitor claim failed: %v", err)
				}
				break
			}
			if monitor == nil {
				break
			}
			select {
			case jobs <- monitor:
			case <-ctx.Done():
				close(jobs)
				workers.Wait()
				return
			}
		}
		close(jobs)
		workers.Wait()
	}

	run()
	ticker := time.NewTicker(pollInterval)
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

func probeModelMonitor(
	ctx context.Context,
	prober relay.ModelProbeService,
	monitor *groups.ModelMonitor,
) (string, string) {
	outcome := relay.ProbeModelCandidates(ctx, prober, monitor.GroupID, monitor.PrimaryModel, monitor.ModelNames)
	if outcome.Supported == 0 {
		return groups.MonitorProbeSkipped, ""
	}
	if outcome.Succeeded {
		return groups.MonitorProbeSuccess, ""
	}
	return groups.MonitorProbeFailed, strings.Join(outcome.Failures, "; ")
}
