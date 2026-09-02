package billing

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	dbpkg "ai-token/internal/db"
	"ai-token/internal/ids"
)

func TestSQLServiceReserveSettleAndRelease(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not configured")
	}

	ctx := context.Background()
	conn, err := dbpkg.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := dbpkg.Migrate(ctx, conn, "../../migrations"); err != nil {
		t.Fatal(err)
	}

	var userID string
	if err := conn.QueryRowContext(ctx, `SELECT id::text FROM users ORDER BY created_at LIMIT 1`).Scan(&userID); err != nil {
		t.Skip("a bootstrap user is required for billing integration tests")
	}

	tenantID := newTestID(t)
	projectID := newTestID(t)
	tokenID := newTestID(t)
	modelID := newTestID(t)
	channelID := newTestID(t)
	slug := "billing-test-" + strings.ToLower(strings.ReplaceAll(tenantID[:8], "-", ""))
	modelName := "billing-test-model-" + strings.ToLower(strings.ReplaceAll(tenantID[:8], "-", ""))
	testSuffix := strings.ToLower(strings.ReplaceAll(tenantID[:8], "-", ""))

	_, err = conn.ExecContext(ctx, `
		INSERT INTO tenants (id, name, slug, status, currency)
		VALUES ($1, 'Billing Integration Tenant', $2, 'active', 'TST')
	`, tenantID, slug)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupBillingFixture(t, conn, tenantID, projectID, tokenID, modelID, channelID)
	})

	_, err = conn.ExecContext(ctx, `
		INSERT INTO projects (id, tenant_id, name, slug, status, created_by)
		VALUES ($1, $2, 'Billing Integration Project', 'default', 'active', $3)
	`, projectID, tenantID, userID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.ExecContext(ctx, `
		INSERT INTO api_tokens (
			id, tenant_id, project_id, created_by, name, token_prefix,
			token_hash, scopes_json, allowed_models_json, allowed_ips_json,
			rate_limit_json
		) VALUES ($1, $2, $3, $4, 'billing-test', 'test',
		          $5, '["model:use"]', '[]', '[]', '{}')
	`, tokenID, tenantID, projectID, userID, "billing-test-hash-"+tokenID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.ExecContext(ctx, `
		INSERT INTO models (
			id, provider, model_name, protocol_family, capabilities_json, status
		) VALUES ($1, 'openai', $2, 'openai_chat_completions',
		          '{"modalities":["text"]}', 'active')
	`, modelID, modelName)
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.ExecContext(ctx, `
		INSERT INTO channels (
			id, name, provider, base_url, credential_ref, status
		) VALUES ($1, 'Billing Test Channel', 'openai', 'https://example.invalid/v1',
		          'env:BILLING_TEST_KEY', 'active')
	`, channelID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.ExecContext(ctx, `
		INSERT INTO channel_models (channel_id, model_id, upstream_model_name, enabled)
		VALUES ($1, $2, $3, true)
	`, channelID, modelID, modelName)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewSQLService(conn)
	if err != nil {
		t.Fatal(err)
	}
	price, err := service.PublishPrice(ctx, userID, PublishPriceRequest{
		ScopeType:               "platform_default",
		ModelID:                 modelID,
		Currency:                "TST",
		InputPricePerUnit:       "0.01",
		OutputPricePerUnit:      "0.02",
		CachedInputPricePerUnit: "0.03",
		ReasoningPricePerUnit:   "0.04",
		MinimumCharge:           "0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if price.Version != 1 || price.ScopeID != "" || price.Status != "active" {
		t.Fatalf("unexpected published price: %#v", price)
	}
	secondPrice, err := service.PublishPrice(ctx, userID, PublishPriceRequest{
		ScopeType:               "platform_default",
		ModelID:                 modelID,
		Currency:                "TST",
		InputPricePerUnit:       "0.01",
		OutputPricePerUnit:      "0.02",
		CachedInputPricePerUnit: "0.03",
		ReasoningPricePerUnit:   "0.04",
		MinimumCharge:           "0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if secondPrice.Version != 2 || secondPrice.Status != "active" {
		t.Fatalf("unexpected second published price: %#v", secondPrice)
	}
	var previousStatus string
	if err := conn.QueryRowContext(ctx, `SELECT status FROM price_versions WHERE id = $1`, price.ID).Scan(&previousStatus); err != nil {
		t.Fatal(err)
	}
	if previousStatus != "retired" {
		t.Fatalf("previous price version must be retired, got %s", previousStatus)
	}

	account, err := service.Credit(ctx, userID, CreditRequest{
		TenantID:       tenantID,
		Currency:       "TST",
		Amount:         "100",
		IdempotencyKey: "billing-credit-1-" + testSuffix,
		Reason:         "integration test",
	})
	if err != nil {
		t.Fatal(err)
	}
	accountID := account.ID
	assertBalance(t, conn, accountID, "100")
	if _, err := service.Credit(ctx, userID, CreditRequest{
		TenantID:       tenantID,
		Currency:       "TST",
		Amount:         "100",
		IdempotencyKey: "billing-credit-1-" + testSuffix,
	}); !errors.Is(err, ErrDuplicateTransaction) {
		t.Fatalf("expected duplicate credit error, got %v", err)
	}

	reservation, err := service.Reserve(ctx, Request{
		RequestID:             "billing-test-request-1-" + testSuffix,
		IdempotencyKey:        "billing-test-idempotency-1-" + testSuffix,
		TenantID:              tenantID,
		ProjectID:             projectID,
		TokenID:               tokenID,
		Model:                 modelName,
		Provider:              "openai",
		ChannelID:             channelID,
		GroupMultiplier:       "2",
		EstimatedInputTokens:  2,
		EstimatedOutputTokens: 3,
		Endpoint:              "/v1/chat/completions",
		ClientIP:              "203.0.113.10",
		RequestType:           "sync",
		ReasoningEffort:       "high",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Reservation uses the request's ordinary input/output estimate. Cache and
	// reasoning prices are applied only when the upstream reports those meters.
	assertBalance(t, conn, accountID, "99.84")
	var groupMultiplier string
	if err := conn.QueryRowContext(ctx, `
		SELECT group_multiplier::text
		FROM model_requests
		WHERE idempotency_key = $1
	`, "billing-test-idempotency-1-"+testSuffix).Scan(&groupMultiplier); err != nil {
		t.Fatal(err)
	}
	if normalizeDecimal(groupMultiplier) != "2" {
		t.Fatalf("expected group multiplier snapshot 2, got %s", groupMultiplier)
	}
	var endpoint, clientIP, requestType, reasoningEffort string
	if err := conn.QueryRowContext(ctx, `
		SELECT endpoint, client_ip, request_type, reasoning_effort
		FROM model_requests WHERE id = $1
	`, reservation.ModelRequestID).Scan(&endpoint, &clientIP, &requestType, &reasoningEffort); err != nil {
		t.Fatal(err)
	}
	if endpoint != "/v1/chat/completions" || clientIP != "203.0.113.10" || requestType != "sync" || reasoningEffort != "high" {
		t.Fatalf("unexpected request metadata: %q %q %q %q", endpoint, clientIP, requestType, reasoningEffort)
	}
	if err := service.RecordRequestMetrics(ctx, reservation.ModelRequestID, 1234); err != nil {
		t.Fatal(err)
	}
	var latency int64
	if err := conn.QueryRowContext(ctx, `SELECT latency_ms FROM model_requests WHERE id = $1`, reservation.ModelRequestID).Scan(&latency); err != nil {
		t.Fatal(err)
	}
	if latency != 1234 {
		t.Fatalf("expected request latency 1234, got %d", latency)
	}

	_, err = service.Reserve(ctx, Request{
		RequestID:             "billing-test-request-duplicate-" + testSuffix,
		IdempotencyKey:        "billing-test-idempotency-1-" + testSuffix,
		TenantID:              tenantID,
		ProjectID:             projectID,
		TokenID:               tokenID,
		Model:                 modelName,
		Provider:              "openai",
		ChannelID:             channelID,
		GroupMultiplier:       "2",
		EstimatedInputTokens:  1,
		EstimatedOutputTokens: 1,
	})
	if !errors.Is(err, ErrDuplicateRequest) {
		t.Fatalf("expected duplicate request error, got %v", err)
	}

	if err := service.Settle(ctx, reservation.ID, Usage{
		InputTokens:       1,
		OutputTokens:      2,
		CachedInputTokens: 10,
		ReasoningTokens:   10,
	}, "provider-request-1"); err != nil {
		t.Fatal(err)
	}
	assertBalance(t, conn, accountID, "99.78")

	var settledStatus string
	if err := conn.QueryRowContext(ctx, `
		SELECT status FROM billing_reservations WHERE id = $1
	`, reservation.ID).Scan(&settledStatus); err != nil {
		t.Fatal(err)
	}
	if settledStatus != "settled" {
		t.Fatalf("expected settled reservation, got %s", settledStatus)
	}

	failedReservation, err := service.Reserve(ctx, Request{
		RequestID:             "billing-test-request-2-" + testSuffix,
		IdempotencyKey:        "billing-test-idempotency-2-" + testSuffix,
		TenantID:              tenantID,
		ProjectID:             projectID,
		TokenID:               tokenID,
		Model:                 modelName,
		Provider:              "openai",
		ChannelID:             channelID,
		GroupMultiplier:       "2",
		EstimatedInputTokens:  1,
		EstimatedOutputTokens: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Fail(ctx, failedReservation.ID, "test_failure"); err != nil {
		t.Fatal(err)
	}
	assertBalance(t, conn, accountID, "99.78")

	freeRequestID, err := service.StartFreeRequest(ctx, Request{
		RequestID:       "billing-test-free-request-" + testSuffix,
		IdempotencyKey:  "billing-test-free-idempotency-" + testSuffix,
		TenantID:        tenantID,
		ProjectID:       projectID,
		TokenID:         tokenID,
		Model:           modelName,
		Provider:        "openai",
		ChannelID:       channelID,
		GroupMultiplier: "1",
		Endpoint:        "/v1/chat/completions",
		ClientIP:        "203.0.113.11",
		RequestType:     "sync",
		BillingType:     "free",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.CompleteFreeRequest(ctx, freeRequestID, Usage{InputTokens: 2, OutputTokens: 3}, "free-response"); err != nil {
		t.Fatal(err)
	}
	var freeStatus string
	var freeAmount string
	if err := conn.QueryRowContext(ctx, `SELECT status, settled_amount::text FROM model_requests WHERE id = $1`, freeRequestID).Scan(&freeStatus, &freeAmount); err != nil {
		t.Fatal(err)
	}
	if freeStatus != "settled" || normalizeDecimal(freeAmount) != "0" {
		t.Fatalf("unexpected free request state: %s %s", freeStatus, freeAmount)
	}

	// Final usage can exceed the initial estimate. The successful request must
	// still settle atomically instead of remaining held until the reaper.
	overageReservation, err := service.Reserve(ctx, Request{
		RequestID:             "billing-test-request-overage-" + testSuffix,
		IdempotencyKey:        "billing-test-idempotency-overage-" + testSuffix,
		TenantID:              tenantID,
		ProjectID:             projectID,
		TokenID:               tokenID,
		Model:                 modelName,
		Provider:              "openai",
		ChannelID:             channelID,
		GroupMultiplier:       "2",
		EstimatedInputTokens:  1,
		EstimatedOutputTokens: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Settle(ctx, overageReservation.ID, Usage{
		InputTokens:  1,
		OutputTokens: 10000,
	}, "provider-request-overage"); err != nil {
		t.Fatalf("overage settlement should complete: %v", err)
	}
	var overageStatus string
	if err := conn.QueryRowContext(ctx, `SELECT status FROM billing_reservations WHERE id = $1`, overageReservation.ID).Scan(&overageStatus); err != nil {
		t.Fatal(err)
	}
	if overageStatus != "settled" {
		t.Fatalf("expected overage reservation to settle, got %s", overageStatus)
	}
	if _, err := service.Credit(ctx, userID, CreditRequest{
		TenantID:       tenantID,
		Currency:       "TST",
		Amount:         "1000",
		IdempotencyKey: "billing-credit-pending-" + testSuffix,
		Reason:         "integration pending reconciliation reserve",
	}); err != nil {
		t.Fatal(err)
	}

	pendingReservation, err := service.Reserve(ctx, Request{
		RequestID:             "billing-test-request-pending-" + testSuffix,
		IdempotencyKey:        "billing-test-idempotency-pending-" + testSuffix,
		TenantID:              tenantID,
		ProjectID:             projectID,
		TokenID:               tokenID,
		Model:                 modelName,
		Provider:              "openai",
		ChannelID:             channelID,
		GroupMultiplier:       "2",
		EstimatedInputTokens:  1,
		EstimatedOutputTokens: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.MarkSettlementPending(ctx, pendingReservation.ID, "integration_usage_missing"); err != nil {
		t.Fatal(err)
	}
	var pendingStatus, pendingRequestStatus string
	if err := conn.QueryRowContext(ctx, `
		SELECT br.status, mr.status
		FROM billing_reservations br
		JOIN model_requests mr ON mr.id = br.request_id
		WHERE br.id = $1
	`, pendingReservation.ID).Scan(&pendingStatus, &pendingRequestStatus); err != nil {
		t.Fatal(err)
	}
	if pendingStatus != "pending" || pendingRequestStatus != "settlement_pending" {
		t.Fatalf("unexpected pending state: reservation=%s request=%s", pendingStatus, pendingRequestStatus)
	}
	if err := service.SettleByModelRequestID(ctx, pendingReservation.ModelRequestID, Usage{InputTokens: 2, OutputTokens: 2}, "reconciled-provider-request"); err != nil {
		t.Fatalf("pending request should be reconciled: %v", err)
	}
	if err := service.SettleByModelRequestID(ctx, pendingReservation.ModelRequestID, Usage{InputTokens: 99, OutputTokens: 99}, "duplicate-reconciliation"); err != nil {
		t.Fatalf("reconciling an already settled request should be idempotent: %v", err)
	}
}

func newTestID(t *testing.T) string {
	t.Helper()
	value, err := ids.New()
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func assertBalance(t *testing.T, conn *sql.DB, accountID, expected string) {
	t.Helper()
	var actual string
	if err := conn.QueryRow(`SELECT balance::text FROM ledger_accounts WHERE id = $1`, accountID).Scan(&actual); err != nil {
		t.Fatal(err)
	}
	if actual != expected && normalizeDecimal(actual) != normalizeDecimal(expected) {
		t.Fatalf("expected balance %s, got %s", expected, actual)
	}
}

func normalizeDecimal(value string) string {
	if !strings.Contains(value, ".") {
		return value
	}
	return strings.TrimRight(strings.TrimRight(value, "0"), ".")
}

func cleanupBillingFixture(
	t *testing.T,
	conn *sql.DB,
	tenantID, projectID, tokenID, modelID, channelID string,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = conn.ExecContext(ctx, `
		DELETE FROM ledger_lines
		WHERE transaction_id IN (
			SELECT id FROM ledger_transactions
			WHERE reference_type = 'tenant' AND reference_id = $1
		)
	`, tenantID)
	_, _ = conn.ExecContext(ctx, `
		DELETE FROM ledger_transactions
		WHERE reference_type = 'tenant' AND reference_id = $1
	`, tenantID)
	_, _ = conn.ExecContext(ctx, `
		DELETE FROM ledger_lines
		WHERE transaction_id IN (
			SELECT id FROM ledger_transactions
			WHERE reference_type = 'model_request'
			  AND reference_id IN (SELECT id FROM model_requests WHERE tenant_id = $1)
		)
	`, tenantID)
	_, _ = conn.ExecContext(ctx, `
		DELETE FROM ledger_transactions
		WHERE reference_type = 'model_request'
		  AND reference_id IN (SELECT id FROM model_requests WHERE tenant_id = $1)
	`, tenantID)
	_, _ = conn.ExecContext(ctx, `DELETE FROM billing_reservations WHERE tenant_id = $1`, tenantID)
	_, _ = conn.ExecContext(ctx, `DELETE FROM usage_events WHERE request_id IN (SELECT id FROM model_requests WHERE tenant_id = $1)`, tenantID)
	_, _ = conn.ExecContext(ctx, `DELETE FROM model_requests WHERE tenant_id = $1`, tenantID)
	_, _ = conn.ExecContext(ctx, `DELETE FROM ledger_accounts WHERE tenant_id = $1 OR account_code IN ('system:revenue:TST', 'system:topup:TST')`, tenantID)
	_, _ = conn.ExecContext(ctx, `DELETE FROM channel_models WHERE channel_id = $1`, channelID)
	_, _ = conn.ExecContext(ctx, `DELETE FROM channels WHERE id = $1`, channelID)
	_, _ = conn.ExecContext(ctx, `DELETE FROM api_tokens WHERE id = $1`, tokenID)
	_, _ = conn.ExecContext(ctx, `DELETE FROM price_versions WHERE model_id = $1`, modelID)
	_, _ = conn.ExecContext(ctx, `DELETE FROM models WHERE id = $1`, modelID)
	_, _ = conn.ExecContext(ctx, `DELETE FROM projects WHERE id = $1`, projectID)
	_, _ = conn.ExecContext(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
}
