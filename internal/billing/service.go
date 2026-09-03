package billing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"math/big"
	"strings"
	"time"

	"ai-token/internal/ids"
	"github.com/jackc/pgx/v5/pgconn"
)

const defaultReservationTTL = 10 * time.Minute

const (
	MeteringToken        = "token"
	MeteringImageCount   = "image_count"
	MeteringVideoSeconds = "video_seconds"
	MeteringVideoRequest = "video_request"
)

var (
	ErrUnavailable            = errors.New("billing service is unavailable")
	ErrInvalidRequest         = errors.New("invalid billing request")
	ErrPriceNotConfigured     = errors.New("price is not configured")
	ErrInsufficientBalance    = errors.New("insufficient balance")
	ErrAccountNotFound        = errors.New("billing account is not found")
	ErrDuplicateRequest       = errors.New("duplicate idempotency key")
	ErrDuplicateTransaction   = errors.New("duplicate billing transaction")
	ErrReservationNotFound    = errors.New("billing reservation is not found")
	ErrReservationClosed      = errors.New("billing reservation is already closed")
	ErrSettlementPending      = errors.New("billing settlement is pending reconciliation")
	ErrUsageUnavailable       = errors.New("billable usage is unavailable")
	ErrModelNotFound          = errors.New("billing model is not found")
	ErrInvalidPrice           = errors.New("invalid price")
	ErrTokenSpendLimitReached = errors.New("api token spend limit reached")
)

type Service interface {
	Reserve(context.Context, Request) (Reservation, error)
	Settle(context.Context, string, Usage, string) error
	Fail(context.Context, string, string) error
}

type ReservationChannelRebinder interface {
	RebindReservationChannel(context.Context, string, string) error
}

// ReservationExtender keeps long-running asynchronous jobs from being
// released by the reservation reaper before their provider operation ends.
type ReservationExtender interface {
	ExtendReservation(context.Context, string, time.Duration) error
}

// ReservationUsagePolicy tells relay handlers whether a zero-usage response
// can still be settled safely, for example when the published price is a
// fixed per-request fee or has a minimum charge.
type ReservationUsagePolicy interface {
	CanSettleWithoutUsage(context.Context, string) (bool, error)
}

type ReservationPendingMarker interface {
	MarkSettlementPending(context.Context, string, string) error
}

// ReservationPendingUsageMarker preserves the usage already received from an
// upstream provider when final pricing or settlement cannot be completed.
// The request remains pending until an operator reconciles it.
type ReservationPendingUsageMarker interface {
	MarkSettlementPendingWithUsage(context.Context, string, string, Usage, string) error
}

type SettlementReconciler interface {
	SettleByModelRequestID(context.Context, string, Usage, string) error
}

type Request struct {
	RequestID             string
	IdempotencyKey        string
	TenantID              string
	ProjectID             string
	TokenID               string
	Model                 string
	Provider              string
	ChannelID             string
	GroupID               string
	GroupMultiplier       string
	MeteringMode          string
	UpstreamCostDiscount  string
	EstimatedInputTokens  int64
	EstimatedOutputTokens int64
	EstimatedMetrics      MeteredUsage
	PricingTier           string
	Endpoint              string
	ClientIP              string
	RequestType           string
	ReasoningEffort       string
	BillingType           string
}

type Usage struct {
	InputTokens       int64
	OutputTokens      int64
	CachedInputTokens int64
	ReasoningTokens   int64
	Metrics           MeteredUsage
	PricingTier       string
	Source            string
	Raw               json.RawMessage
}

type Reservation struct {
	ID             string
	ModelRequestID string
	AccountID      string
	Currency       string
	ReservedAmount string
}

type RequestMetricsRecorder interface {
	RecordRequestMetrics(context.Context, string, int64) error
}

type FreeRequestRecorder interface {
	StartFreeRequest(context.Context, Request) (string, error)
	CompleteFreeRequest(context.Context, string, Usage, string) error
	FailFreeRequest(context.Context, string, string) error
	RebindRequestChannel(context.Context, string, string) error
}

type Price struct {
	// ID identifies the source price record. PriceVersionID is only populated
	// for internal manual prices because model_requests.price_version_id has a
	// foreign key to price_versions, not official_model_price_versions.
	ID                      string
	PriceVersionID          string
	Source                  string
	ModelID                 string
	Currency                string
	InputPricePerUnit       string
	OutputPricePerUnit      string
	CachedInputPricePerUnit string
	ReasoningPricePerUnit   string
	MinimumCharge           string
	Components              []PriceComponent
}

type priceSnapshotRecord struct {
	ID                      string           `json:"id"`
	PriceVersionID          string           `json:"price_version_id"`
	Source                  string           `json:"source"`
	ModelID                 string           `json:"model_id"`
	Currency                string           `json:"currency"`
	InputPricePerUnit       string           `json:"input_price_per_unit"`
	OutputPricePerUnit      string           `json:"output_price_per_unit"`
	CachedInputPricePerUnit string           `json:"cached_input_price_per_unit"`
	ReasoningPricePerUnit   string           `json:"reasoning_price_per_unit"`
	MinimumCharge           string           `json:"minimum_charge"`
	Components              []PriceComponent `json:"components"`
}

func priceFromSnapshot(raw []byte, fallbackCurrency string) (Price, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return Price{}, ErrPriceNotConfigured
	}
	var snapshot priceSnapshotRecord
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return Price{}, ErrPriceNotConfigured
	}
	if strings.TrimSpace(snapshot.ID) == "" || (len(snapshot.Components) == 0 &&
		strings.TrimSpace(snapshot.InputPricePerUnit) == "" &&
		strings.TrimSpace(snapshot.OutputPricePerUnit) == "") {
		return Price{}, ErrPriceNotConfigured
	}
	currency := strings.ToUpper(strings.TrimSpace(snapshot.Currency))
	if currency == "" {
		currency = strings.ToUpper(strings.TrimSpace(fallbackCurrency))
	}
	if currency == "" {
		return Price{}, ErrPriceNotConfigured
	}
	return Price{
		ID: snapshot.ID, PriceVersionID: strings.TrimSpace(snapshot.PriceVersionID),
		Source: snapshot.Source, ModelID: snapshot.ModelID, Currency: currency,
		InputPricePerUnit: snapshot.InputPricePerUnit, OutputPricePerUnit: snapshot.OutputPricePerUnit,
		CachedInputPricePerUnit: snapshot.CachedInputPricePerUnit,
		ReasoningPricePerUnit:   snapshot.ReasoningPricePerUnit,
		MinimumCharge:           snapshot.MinimumCharge, Components: snapshot.Components,
	}, nil
}

func officialPriceVersionID(price Price) string {
	if strings.EqualFold(strings.TrimSpace(price.Source), "litellm") {
		return strings.TrimSpace(price.ID)
	}
	return ""
}

type PriceVersionSummary struct {
	ID                      string           `json:"id"`
	ScopeType               string           `json:"scope_type"`
	ScopeID                 string           `json:"scope_id,omitempty"`
	ModelID                 string           `json:"model_id"`
	Provider                string           `json:"provider"`
	Model                   string           `json:"model"`
	Currency                string           `json:"currency"`
	InputPricePerUnit       string           `json:"input_price_per_unit"`
	OutputPricePerUnit      string           `json:"output_price_per_unit"`
	CachedInputPricePerUnit string           `json:"cached_input_price_per_unit"`
	ReasoningPricePerUnit   string           `json:"reasoning_price_per_unit"`
	MinimumCharge           string           `json:"minimum_charge"`
	Version                 int64            `json:"version"`
	EffectiveFrom           time.Time        `json:"effective_from"`
	EffectiveTo             *time.Time       `json:"effective_to,omitempty"`
	Status                  string           `json:"status"`
	CreatedBy               string           `json:"created_by"`
	CreatedAt               time.Time        `json:"created_at"`
	Components              []PriceComponent `json:"components,omitempty"`
}

// PriceMatrixSummary is the current reference price for one configured model.
// Historical price versions remain available in the ledger, but are intentionally
// excluded from the operational pricing screen.
type PriceMatrixSummary struct {
	ModelID                          string                    `json:"model_id"`
	Provider                         string                    `json:"provider"`
	Model                            string                    `json:"model"`
	Currency                         string                    `json:"currency"`
	InputPricePerMillionTokens       string                    `json:"input_price_per_million_tokens"`
	OutputPricePerMillionTokens      string                    `json:"output_price_per_million_tokens"`
	CachedInputPricePerMillionTokens string                    `json:"cached_input_price_per_million_tokens"`
	ReasoningPricePerMillionTokens   string                    `json:"reasoning_price_per_million_tokens"`
	Source                           string                    `json:"source"`
	SourceURL                        string                    `json:"source_url,omitempty"`
	UpdatedAt                        *time.Time                `json:"updated_at,omitempty"`
	Components                       []PriceComponent          `json:"components,omitempty"`
	CostEstimates                    []PriceMatrixCostEstimate `json:"cost_estimates,omitempty"`
}

type PriceMatrixComponentEstimate struct {
	ComponentCode        string `json:"component_code"`
	Unit                 string `json:"unit"`
	CustomerPricePerUnit string `json:"customer_price_per_unit,omitempty"`
	EstimatedCostPerUnit string `json:"estimated_cost_per_unit,omitempty"`
	ProfitPerUnit        string `json:"profit_per_unit,omitempty"`
	ProfitMarginPercent  string `json:"profit_margin_percent,omitempty"`
}

type PriceMatrixCostEstimate struct {
	GroupID              string                         `json:"group_id"`
	GroupCode            string                         `json:"group_code"`
	GroupName            string                         `json:"group_name"`
	GroupPriority        int                            `json:"group_priority"`
	Multiplier           string                         `json:"multiplier"`
	BillingType          string                         `json:"billing_type"`
	ChannelID            string                         `json:"channel_id"`
	ChannelName          string                         `json:"channel_name"`
	RouteCount           int                            `json:"route_count"`
	UpstreamCostDiscount string                         `json:"upstream_cost_discount"`
	Components           []PriceMatrixComponentEstimate `json:"components,omitempty"`
}

type PublishPriceRequest struct {
	ScopeType               string                `json:"scope_type"`
	ScopeID                 string                `json:"scope_id,omitempty"`
	ModelID                 string                `json:"model_id,omitempty"`
	Provider                string                `json:"provider,omitempty"`
	Model                   string                `json:"model,omitempty"`
	Currency                string                `json:"currency"`
	InputPricePerUnit       string                `json:"input_price_per_unit"`
	OutputPricePerUnit      string                `json:"output_price_per_unit"`
	CachedInputPricePerUnit string                `json:"cached_input_price_per_unit"`
	ReasoningPricePerUnit   string                `json:"reasoning_price_per_unit"`
	MinimumCharge           string                `json:"minimum_charge"`
	Components              []PriceComponentInput `json:"components,omitempty"`
	EffectiveFrom           *time.Time            `json:"effective_from,omitempty"`
}

type AccountSummary struct {
	ID       string `json:"id"`
	TenantID string `json:"tenant_id"`
	Currency string `json:"currency"`
	Balance  string `json:"balance"`
	Status   string `json:"status"`
}

type CreditRequest struct {
	TenantID       string `json:"-"`
	Currency       string `json:"currency"`
	Amount         string `json:"amount"`
	IdempotencyKey string `json:"idempotency_key"`
	Reason         string `json:"reason"`
}

type AdminService interface {
	ListPrices(context.Context) ([]PriceVersionSummary, error)
	PublishPrice(context.Context, string, PublishPriceRequest) (PriceVersionSummary, error)
	GetPrepaidAccount(context.Context, string, string) (AccountSummary, error)
	Credit(context.Context, string, CreditRequest) (AccountSummary, error)
}

type PriceMatrixReader interface {
	ListPriceMatrix(context.Context) ([]PriceMatrixSummary, error)
}

type SQLService struct {
	db  *sql.DB
	now func() time.Time
}

func NewSQLService(db *sql.DB) (*SQLService, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &SQLService{
		db:  db,
		now: time.Now,
	}, nil
}

func (s *SQLService) EnsurePrepaidAccount(ctx context.Context, tenantID, currency string) (string, error) {
	if s == nil || s.db == nil {
		return "", ErrUnavailable
	}
	tenantID = strings.TrimSpace(tenantID)
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if !ids.Valid(tenantID) || currency == "" {
		return "", ErrInvalidRequest
	}
	accountID, err := ids.New()
	if err != nil {
		return "", err
	}
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO ledger_accounts (
			id, tenant_id, account_type, currency, status
		) VALUES ($1, $2, 'prepaid_balance', $3, 'active')
		ON CONFLICT (tenant_id, account_type, currency)
		DO UPDATE SET status = 'active'
		RETURNING id::text
	`, accountID, tenantID, currency).Scan(&accountID)
	if err != nil {
		return "", err
	}
	return accountID, nil
}

func (s *SQLService) GetPrepaidAccount(ctx context.Context, tenantID, currency string) (AccountSummary, error) {
	if s == nil || s.db == nil {
		return AccountSummary{}, ErrUnavailable
	}
	tenantID = strings.TrimSpace(tenantID)
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if !ids.Valid(tenantID) {
		return AccountSummary{}, ErrInvalidRequest
	}

	if currency == "" {
		if err := s.db.QueryRowContext(ctx, `
			SELECT currency
			FROM tenants
			WHERE id = $1
			  AND status = 'active'
			  AND deleted_at IS NULL
		`, tenantID).Scan(&currency); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return AccountSummary{}, ErrAccountNotFound
			}
			return AccountSummary{}, err
		}
		currency = strings.ToUpper(strings.TrimSpace(currency))
	}
	var account AccountSummary
	err := s.db.QueryRowContext(ctx, `
		SELECT id::text, tenant_id::text, currency, balance::text, status
		FROM ledger_accounts
		WHERE tenant_id = $1
		  AND account_type = 'prepaid_balance'
		  AND currency = $2
		  AND status = 'active'
	`, tenantID, currency).Scan(
		&account.ID,
		&account.TenantID,
		&account.Currency,
		&account.Balance,
		&account.Status,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return AccountSummary{}, ErrAccountNotFound
	}
	if err != nil {
		return AccountSummary{}, err
	}
	account.Currency = strings.ToUpper(strings.TrimSpace(account.Currency))
	return account, nil
}

func (s *SQLService) Credit(ctx context.Context, actorID string, request CreditRequest) (AccountSummary, error) {
	if s == nil || s.db == nil {
		return AccountSummary{}, ErrUnavailable
	}
	request.TenantID = strings.TrimSpace(request.TenantID)
	request.Currency = strings.ToUpper(strings.TrimSpace(request.Currency))
	request.Amount = strings.TrimSpace(request.Amount)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	request.Reason = strings.TrimSpace(request.Reason)
	if !ids.Valid(request.TenantID) || request.IdempotencyKey == "" ||
		len(request.IdempotencyKey) > 256 || len(request.Reason) > 1024 ||
		!validPositiveDecimal(request.Amount) {
		return AccountSummary{}, ErrInvalidRequest
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AccountSummary{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var existingTransactionID string
	err = tx.QueryRowContext(ctx, `
		SELECT id::text
		FROM ledger_transactions
		WHERE idempotency_key = $1
	`, request.IdempotencyKey).Scan(&existingTransactionID)
	if err == nil {
		return AccountSummary{}, ErrDuplicateTransaction
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return AccountSummary{}, err
	}

	tenantCurrencyValue, err := tenantCurrency(ctx, tx, request.TenantID)
	if err != nil {
		return AccountSummary{}, err
	}
	if request.Currency == "" {
		request.Currency = tenantCurrencyValue
	}
	if request.Currency != tenantCurrencyValue {
		return AccountSummary{}, ErrInvalidRequest
	}

	var account AccountSummary
	err = tx.QueryRowContext(ctx, `
		SELECT id::text, tenant_id::text, currency, balance::text, status
		FROM ledger_accounts
		WHERE tenant_id = $1
		  AND account_type = 'prepaid_balance'
		  AND currency = $2
		  AND status = 'active'
		FOR UPDATE
	`, request.TenantID, request.Currency).Scan(
		&account.ID,
		&account.TenantID,
		&account.Currency,
		&account.Balance,
		&account.Status,
	)
	if errors.Is(err, sql.ErrNoRows) {
		account.ID, err = ids.New()
		if err != nil {
			return AccountSummary{}, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO ledger_accounts (
				id, tenant_id, account_type, currency, status
			) VALUES ($1, $2, 'prepaid_balance', $3, 'active')
			ON CONFLICT (tenant_id, account_type, currency) DO NOTHING
		`, account.ID, request.TenantID, request.Currency); err != nil {
			return AccountSummary{}, err
		}
		if err := tx.QueryRowContext(ctx, `
			SELECT id::text, tenant_id::text, currency, balance::text, status
			FROM ledger_accounts
			WHERE tenant_id = $1
			  AND account_type = 'prepaid_balance'
			  AND currency = $2
			  AND status = 'active'
			FOR UPDATE
		`, request.TenantID, request.Currency).Scan(
			&account.ID,
			&account.TenantID,
			&account.Currency,
			&account.Balance,
			&account.Status,
		); err != nil {
			return AccountSummary{}, err
		}
	} else if err != nil {
		return AccountSummary{}, err
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE ledger_accounts
		SET balance = balance + $2::numeric
		WHERE id = $1 AND status = 'active'
	`, account.ID, request.Amount)
	if err != nil {
		return AccountSummary{}, err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return AccountSummary{}, err
	} else if affected != 1 {
		return AccountSummary{}, ErrAccountNotFound
	}

	systemAccountID, err := ensureSystemAccountKind(ctx, tx, request.Currency, "topup", "credit")
	if err != nil {
		return AccountSummary{}, err
	}
	transactionID, err := ids.New()
	if err != nil {
		return AccountSummary{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ledger_transactions (
			id, idempotency_key, transaction_type, reference_type, reference_id
		) VALUES ($1, $2, 'account_credit', 'tenant', $3)
	`, transactionID, request.IdempotencyKey, request.TenantID); err != nil {
		if isUniqueViolation(err) {
			return AccountSummary{}, ErrDuplicateTransaction
		}
		return AccountSummary{}, err
	}
	metadata, _ := json.Marshal(map[string]string{
		"actor_id": strings.TrimSpace(actorID),
		"reason":   request.Reason,
	})
	userLineID, err := ids.New()
	if err != nil {
		return AccountSummary{}, err
	}
	systemLineID, err := ids.New()
	if err != nil {
		return AccountSummary{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ledger_lines (
			id, transaction_id, account_id, direction, amount, currency, metadata_json
		) VALUES
			($1, $2, $3, 'credit', $4, $5, $6),
			($7, $2, $8, 'debit', $4, $5, $6)
	`, userLineID, transactionID, account.ID, request.Amount, request.Currency, metadata,
		systemLineID, systemAccountID); err != nil {
		return AccountSummary{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE ledger_accounts
		SET balance = balance + $2::numeric
		WHERE id = $1 AND status = 'active'
	`, systemAccountID, request.Amount); err != nil {
		return AccountSummary{}, err
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT balance::text
		FROM ledger_accounts
		WHERE id = $1
	`, account.ID).Scan(&account.Balance); err != nil {
		return AccountSummary{}, err
	}

	if err := tx.Commit(); err != nil {
		return AccountSummary{}, err
	}
	account.Currency = strings.ToUpper(strings.TrimSpace(account.Currency))
	return account, nil
}

func (s *SQLService) ListPrices(ctx context.Context) ([]PriceVersionSummary, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT pv.id::text, pv.scope_type, COALESCE(pv.scope_id::text, ''),
		       pv.model_id::text, m.provider, m.model_name, pv.currency,
		       pv.input_price_per_unit::text, pv.output_price_per_unit::text,
		       pv.cached_input_price_per_unit::text,
		       pv.reasoning_price_per_unit::text, pv.minimum_charge::text,
		       pv.version, pv.effective_from, pv.effective_to, pv.status,
		       pv.created_by::text, pv.created_at
		FROM price_versions pv
		JOIN models m ON m.id = pv.model_id
		ORDER BY m.provider, m.model_name, pv.scope_type, pv.version DESC
		LIMIT 2000
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	prices := make([]PriceVersionSummary, 0)
	for rows.Next() {
		var (
			price     PriceVersionSummary
			effective sql.NullTime
		)
		if err := rows.Scan(
			&price.ID,
			&price.ScopeType,
			&price.ScopeID,
			&price.ModelID,
			&price.Provider,
			&price.Model,
			&price.Currency,
			&price.InputPricePerUnit,
			&price.OutputPricePerUnit,
			&price.CachedInputPricePerUnit,
			&price.ReasoningPricePerUnit,
			&price.MinimumCharge,
			&price.Version,
			&price.EffectiveFrom,
			&effective,
			&price.Status,
			&price.CreatedBy,
			&price.CreatedAt,
		); err != nil {
			return nil, err
		}
		if effective.Valid {
			value := effective.Time
			price.EffectiveTo = &value
		}
		price.Currency = strings.ToUpper(strings.TrimSpace(price.Currency))
		price.Components, err = loadPriceComponents(ctx, s.db, price.ID)
		if err != nil {
			return nil, err
		}
		prices = append(prices, price)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return prices, nil
}

func (s *SQLService) ListPriceMatrix(ctx context.Context) ([]PriceMatrixSummary, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx, `
		WITH configured_models AS (
			SELECT DISTINCT m.id, m.provider, m.model_name
			FROM models m
			JOIN channel_models cm ON cm.model_id = m.id AND cm.enabled = true
			JOIN channels c ON c.id = cm.channel_id AND c.deleted_at IS NULL
			WHERE m.status = 'active'
		)
		SELECT cm.id::text, cm.provider, cm.model_name,
		       COALESCE(manual.currency, official.currency, 'USD'),
		       COALESCE(manual.input_price_per_unit::text, official.input_price_per_unit::text, ''),
		       COALESCE(manual.output_price_per_unit::text, official.output_price_per_unit::text, ''),
		       COALESCE(manual.cached_input_price_per_unit::text, official.cached_input_price_per_unit::text, ''),
		       COALESCE(manual.reasoning_price_per_unit::text, official.reasoning_price_per_unit::text, ''),
		       CASE
			       WHEN manual.id IS NOT NULL THEN 'manual'
			       WHEN official.id IS NOT NULL THEN 'litellm'
			       ELSE 'unconfigured'
		       END,
		       CASE WHEN manual.id IS NOT NULL THEN '' ELSE COALESCE(official.source_url, '') END,
		       COALESCE(manual.effective_from, official.fetched_at)
		FROM configured_models cm
		LEFT JOIN LATERAL (
			SELECT pv.id, pv.currency, pv.input_price_per_unit,
			       pv.output_price_per_unit, pv.cached_input_price_per_unit,
			       pv.reasoning_price_per_unit, pv.effective_from
			FROM price_versions pv
			WHERE pv.model_id = cm.id
			  AND pv.scope_type = 'platform_default'
			  AND pv.scope_id IS NULL
			  AND pv.currency = 'USD'
			  AND pv.status = 'active'
			  AND pv.effective_from <= now()
			  AND (pv.effective_to IS NULL OR pv.effective_to > now())
			ORDER BY pv.effective_from DESC, pv.version DESC
			LIMIT 1
		) manual ON true
		LEFT JOIN LATERAL (
			SELECT omp.id, omp.currency, omp.input_price_per_unit,
			       omp.output_price_per_unit, omp.cached_input_price_per_unit,
			       omp.reasoning_price_per_unit, 0::numeric AS minimum_charge, omp.source_url, omp.fetched_at
			FROM official_model_price_versions omp
			WHERE omp.model_id = cm.id
			  AND omp.source = 'litellm'
			  AND omp.effective_to IS NULL
			ORDER BY omp.effective_from DESC
			LIMIT 1
		) official ON true
		ORDER BY cm.provider, cm.model_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]PriceMatrixSummary, 0)
	for rows.Next() {
		var (
			item      PriceMatrixSummary
			input     string
			output    string
			cached    string
			reasoning string
			updatedAt sql.NullTime
		)
		if err := rows.Scan(
			&item.ModelID,
			&item.Provider,
			&item.Model,
			&item.Currency,
			&input,
			&output,
			&cached,
			&reasoning,
			&item.Source,
			&item.SourceURL,
			&updatedAt,
		); err != nil {
			return nil, err
		}
		item.Provider = strings.ToLower(strings.TrimSpace(item.Provider))
		item.Currency = strings.ToUpper(strings.TrimSpace(item.Currency))
		item.InputPricePerMillionTokens = pricePerMillionTokens(input)
		item.OutputPricePerMillionTokens = pricePerMillionTokens(output)
		item.CachedInputPricePerMillionTokens = pricePerMillionTokens(cached)
		item.ReasoningPricePerMillionTokens = pricePerMillionTokens(reasoning)
		var componentPriceID string
		if item.Source == "manual" {
			_ = s.db.QueryRowContext(ctx, "SELECT id::text FROM price_versions WHERE model_id = $1 AND scope_type = 'platform_default' AND scope_id IS NULL AND currency = $2 AND status = 'active' ORDER BY effective_from DESC, version DESC LIMIT 1", item.ModelID, item.Currency).Scan(&componentPriceID)
			if componentPriceID != "" {
				item.Components, err = loadPriceComponents(ctx, s.db, componentPriceID)
			}
		} else if item.Source == "litellm" {
			_ = s.db.QueryRowContext(ctx, "SELECT id::text FROM official_model_price_versions WHERE model_id = $1 AND source = 'litellm' AND effective_to IS NULL ORDER BY effective_from DESC LIMIT 1", item.ModelID).Scan(&componentPriceID)
			if componentPriceID != "" {
				item.Components, err = loadOfficialPriceComponents(ctx, s.db, componentPriceID)
			}
		}
		if err != nil {
			return nil, err
		}
		item.CostEstimates, err = s.listPriceMatrixCostEstimates(
			ctx,
			item.ModelID,
			input,
			output,
			cached,
			reasoning,
			item.Components,
			item.Source,
		)
		if err != nil {
			return nil, err
		}
		if updatedAt.Valid {
			value := updatedAt.Time.UTC()
			item.UpdatedAt = &value
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *SQLService) listPriceMatrixCostEstimates(
	ctx context.Context,
	modelID string,
	customerInput string,
	customerOutput string,
	customerCached string,
	customerReasoning string,
	customerComponents []PriceComponent,
	pricingSource string,
) ([]PriceMatrixCostEstimate, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}

	customerComponents = matrixPriceComponents(
		customerInput,
		customerOutput,
		customerCached,
		customerReasoning,
		customerComponents,
	)
	upstreamInput := customerInput
	upstreamOutput := customerOutput
	upstreamCached := customerCached
	upstreamReasoning := customerReasoning
	upstreamComponents := customerComponents

	// A manual platform price can differ from the provider reference price.
	// Cost estimates use the latest official snapshot whenever it exists,
	// matching the settlement cost basis used by the relay.
	if strings.EqualFold(strings.TrimSpace(pricingSource), "manual") {
		var officialPriceID string
		err := s.db.QueryRowContext(ctx, `
			SELECT id::text, input_price_per_unit::text,
			       output_price_per_unit::text,
			       cached_input_price_per_unit::text,
			       reasoning_price_per_unit::text
			FROM official_model_price_versions
			WHERE model_id = $1
			  AND source = 'litellm'
			  AND effective_to IS NULL
			ORDER BY effective_from DESC
			LIMIT 1
		`, modelID).Scan(
			&officialPriceID,
			&upstreamInput,
			&upstreamOutput,
			&upstreamCached,
			&upstreamReasoning,
		)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if err == nil {
			upstreamComponents = matrixPriceComponents(
				upstreamInput,
				upstreamOutput,
				upstreamCached,
				upstreamReasoning,
				nil,
			)
			upstreamComponents, err = loadOfficialPriceComponents(ctx, s.db, officialPriceID)
			if err != nil {
				return nil, err
			}
			upstreamComponents = matrixPriceComponents(
				upstreamInput,
				upstreamOutput,
				upstreamCached,
				upstreamReasoning,
				upstreamComponents,
			)
		}
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT route.group_id, route.group_code, route.group_name,
		       route.group_priority, route.multiplier, route.billing_type,
		       route.channel_id, route.channel_name, route.route_count,
		       route.upstream_cost_discount
		FROM (
			SELECT DISTINCT ON (rg.id)
			       rg.id::text AS group_id,
			       rg.code AS group_code,
			       rg.name AS group_name,
			       rg.priority AS group_priority,
			       rg.multiplier::text AS multiplier,
			       rg.billing_type,
			       c.id::text AS channel_id,
			       c.name AS channel_name,
			       COUNT(*) OVER (PARTITION BY rg.id)::int AS route_count,
			       c.upstream_cost_discount::text AS upstream_cost_discount
			FROM routing_groups rg
			JOIN routing_group_channels rgc ON rgc.group_id = rg.id
			JOIN channels c ON c.id = rgc.channel_id
			JOIN channel_models cm
			  ON cm.channel_id = c.id
			 AND cm.model_id = $1::uuid
			 AND cm.enabled = true
			 AND (cm.auto_disabled_until IS NULL OR cm.auto_disabled_until <= now())
			WHERE rg.status = 'active'
			  AND rg.deleted_at IS NULL
			  AND c.status = 'active'
			  AND c.deleted_at IS NULL
			  AND (c.auto_disabled_until IS NULL OR c.auto_disabled_until <= now())
			ORDER BY rg.id, c.priority DESC, c.updated_at ASC, c.id ASC
		) route
		ORDER BY route.group_priority DESC, route.group_code ASC
	`, modelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	estimates := make([]PriceMatrixCostEstimate, 0)
	for rows.Next() {
		var estimate PriceMatrixCostEstimate
		if err := rows.Scan(
			&estimate.GroupID,
			&estimate.GroupCode,
			&estimate.GroupName,
			&estimate.GroupPriority,
			&estimate.Multiplier,
			&estimate.BillingType,
			&estimate.ChannelID,
			&estimate.ChannelName,
			&estimate.RouteCount,
			&estimate.UpstreamCostDiscount,
		); err != nil {
			return nil, err
		}
		estimate.Multiplier = normalizeDecimalText(estimate.Multiplier)
		estimate.UpstreamCostDiscount = normalizeDecimalText(estimate.UpstreamCostDiscount)
		estimate.Components = matrixComponentEstimates(
			customerComponents,
			upstreamComponents,
			estimate.Multiplier,
			estimate.BillingType,
			estimate.UpstreamCostDiscount,
		)
		estimates = append(estimates, estimate)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return estimates, nil
}

func matrixPriceComponents(input, output, cached, reasoning string, components []PriceComponent) []PriceComponent {
	result := append([]PriceComponent(nil), components...)
	existing := make(map[string]struct{}, len(result))
	for _, component := range result {
		code := strings.TrimSpace(component.ComponentCode)
		if code != "" {
			existing[code] = struct{}{}
		}
	}
	for _, item := range []struct {
		code  string
		value string
	}{
		{code: "input_tokens", value: input},
		{code: "output_tokens", value: output},
		{code: "cached_input_tokens", value: cached},
		{code: "reasoning_tokens", value: reasoning},
	} {
		if strings.TrimSpace(item.value) == "" {
			continue
		}
		if _, exists := existing[item.code]; exists {
			continue
		}
		result = append(result, PriceComponent{
			ComponentCode: item.code,
			Unit:          "token",
			PricePerUnit:  item.value,
		})
		existing[item.code] = struct{}{}
	}
	return result
}

func matrixComponentEstimates(
	customerComponents []PriceComponent,
	upstreamComponents []PriceComponent,
	multiplier string,
	billingType string,
	discount string,
) []PriceMatrixComponentEstimate {
	customerByCode := make(map[string]PriceComponent, len(customerComponents))
	upstreamByCode := make(map[string]PriceComponent, len(upstreamComponents))
	order := make([]string, 0, len(customerComponents)+len(upstreamComponents))
	seen := make(map[string]struct{}, len(customerComponents)+len(upstreamComponents))
	for _, component := range customerComponents {
		code := strings.TrimSpace(component.ComponentCode)
		if code == "" {
			continue
		}
		customerByCode[code] = component
		if _, exists := seen[code]; !exists {
			order = append(order, code)
			seen[code] = struct{}{}
		}
	}
	for _, component := range upstreamComponents {
		code := strings.TrimSpace(component.ComponentCode)
		if code == "" {
			continue
		}
		upstreamByCode[code] = component
		if _, exists := seen[code]; !exists {
			order = append(order, code)
			seen[code] = struct{}{}
		}
	}

	result := make([]PriceMatrixComponentEstimate, 0, len(order))
	for _, code := range order {
		customer, hasCustomer := customerByCode[code]
		upstream, hasUpstream := upstreamByCode[code]
		unit := customer.Unit
		if strings.TrimSpace(unit) == "" {
			unit = upstream.Unit
		}
		item := PriceMatrixComponentEstimate{
			ComponentCode: code,
			Unit:          unit,
		}
		if hasCustomer {
			customerFactor := multiplier
			if strings.EqualFold(strings.TrimSpace(billingType), "free") {
				customerFactor = "0"
			}
			item.CustomerPricePerUnit = multiplyMatrixDecimal(customer.PricePerUnit, customerFactor)
		} else if strings.EqualFold(strings.TrimSpace(billingType), "free") {
			item.CustomerPricePerUnit = "0"
		}
		if hasUpstream {
			item.EstimatedCostPerUnit = multiplyMatrixDecimal(upstream.PricePerUnit, discount)
		}
		if item.CustomerPricePerUnit != "" && item.EstimatedCostPerUnit != "" {
			item.ProfitPerUnit = subtractMatrixDecimal(item.CustomerPricePerUnit, item.EstimatedCostPerUnit)
			item.ProfitMarginPercent = matrixProfitMargin(item.CustomerPricePerUnit, item.ProfitPerUnit)
		}
		result = append(result, item)
	}
	return result
}

func multiplyMatrixDecimal(left, right string) string {
	leftValue, leftOK := new(big.Rat).SetString(strings.TrimSpace(left))
	rightValue, rightOK := new(big.Rat).SetString(strings.TrimSpace(right))
	if !leftOK || !rightOK {
		return ""
	}
	return ratDecimal(new(big.Rat).Mul(leftValue, rightValue))
}

func subtractMatrixDecimal(left, right string) string {
	leftValue, leftOK := new(big.Rat).SetString(strings.TrimSpace(left))
	rightValue, rightOK := new(big.Rat).SetString(strings.TrimSpace(right))
	if !leftOK || !rightOK {
		return ""
	}
	return ratDecimal(new(big.Rat).Sub(leftValue, rightValue))
}

func matrixProfitMargin(customerPrice, profit string) string {
	customerValue, customerOK := new(big.Rat).SetString(strings.TrimSpace(customerPrice))
	profitValue, profitOK := new(big.Rat).SetString(strings.TrimSpace(profit))
	if !customerOK || !profitOK || customerValue.Sign() == 0 {
		return ""
	}
	return ratDecimal(new(big.Rat).Mul(
		new(big.Rat).Quo(profitValue, customerValue),
		big.NewRat(100, 1),
	))
}

func (s *SQLService) PublishPrice(
	ctx context.Context,
	actorID string,
	request PublishPriceRequest,
) (PriceVersionSummary, error) {
	if s == nil || s.db == nil {
		return PriceVersionSummary{}, ErrUnavailable
	}
	request.ScopeType = strings.ToLower(strings.TrimSpace(request.ScopeType))
	request.ScopeID = strings.TrimSpace(request.ScopeID)
	request.ModelID = strings.TrimSpace(request.ModelID)
	request.Provider = strings.ToLower(strings.TrimSpace(request.Provider))
	request.Model = strings.TrimSpace(request.Model)
	request.Currency = strings.ToUpper(strings.TrimSpace(request.Currency))
	request.InputPricePerUnit = strings.TrimSpace(request.InputPricePerUnit)
	request.OutputPricePerUnit = strings.TrimSpace(request.OutputPricePerUnit)
	request.CachedInputPricePerUnit = strings.TrimSpace(request.CachedInputPricePerUnit)
	request.ReasoningPricePerUnit = strings.TrimSpace(request.ReasoningPricePerUnit)
	request.MinimumCharge = strings.TrimSpace(request.MinimumCharge)
	if request.InputPricePerUnit == "" {
		request.InputPricePerUnit = "0"
	}
	if request.OutputPricePerUnit == "" {
		request.OutputPricePerUnit = "0"
	}
	if request.CachedInputPricePerUnit == "" {
		request.CachedInputPricePerUnit = "0"
	}
	if request.ReasoningPricePerUnit == "" {
		request.ReasoningPricePerUnit = "0"
	}
	if request.MinimumCharge == "" {
		request.MinimumCharge = "0"
	}
	request.InputPricePerUnit = zeroPrice(request.InputPricePerUnit)
	request.OutputPricePerUnit = zeroPrice(request.OutputPricePerUnit)
	request.CachedInputPricePerUnit = zeroPrice(request.CachedInputPricePerUnit)
	request.ReasoningPricePerUnit = zeroPrice(request.ReasoningPricePerUnit)
	if request.ScopeType != "platform_default" &&
		request.ScopeType != "tenant" &&
		request.ScopeType != "project" &&
		request.ScopeType != "token" {
		return PriceVersionSummary{}, ErrInvalidPrice
	}
	if request.ScopeType == "platform_default" {
		request.ScopeID = ""
	} else if request.ScopeID == "" || !ids.Valid(request.ScopeID) {
		return PriceVersionSummary{}, ErrInvalidPrice
	}
	if request.ModelID != "" && !ids.Valid(request.ModelID) {
		return PriceVersionSummary{}, ErrInvalidPrice
	}
	if request.ModelID == "" && (request.Provider == "" || request.Model == "") {
		return PriceVersionSummary{}, ErrInvalidPrice
	}
	for _, value := range []string{
		request.InputPricePerUnit,
		request.OutputPricePerUnit,
		request.CachedInputPricePerUnit,
		request.ReasoningPricePerUnit,
		request.MinimumCharge,
	} {
		if _, ok := canonicalDecimal(value, 30, 30); !ok {
			return PriceVersionSummary{}, ErrInvalidPrice
		}
	}
	components := mergePriceComponentInputs(
		legacyPriceComponentInputs(request.InputPricePerUnit, request.OutputPricePerUnit, request.CachedInputPricePerUnit, request.ReasoningPricePerUnit),
		request.Components,
	)
	components, err := normalizePriceComponentInputs(components)
	if err != nil {
		return PriceVersionSummary{}, err
	}
	if !hasPricedComponent(components) {
		return PriceVersionSummary{}, ErrInvalidPrice
	}
	if len(request.Currency) != 3 {
		return PriceVersionSummary{}, ErrInvalidPrice
	}

	effectiveFrom := s.now()
	if request.EffectiveFrom != nil {
		effectiveFrom = request.EffectiveFrom.UTC()
	}
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return PriceVersionSummary{}, ErrInvalidRequest
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PriceVersionSummary{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()
	if request.ScopeID != "" {
		exists, err := priceScopeExists(ctx, tx, request.ScopeType, request.ScopeID)
		if err != nil {
			return PriceVersionSummary{}, ErrInvalidPrice
		}
		if !exists {
			return PriceVersionSummary{}, ErrInvalidPrice
		}
	}
	var modelID string
	if request.ModelID != "" {
		err = tx.QueryRowContext(ctx, `
			SELECT id::text
			FROM models
			WHERE id = $1 AND status = 'active'
		`, request.ModelID).Scan(&modelID)
	} else {
		err = tx.QueryRowContext(ctx, `
			SELECT id::text
			FROM models
			WHERE provider = $1 AND model_name = $2 AND status = 'active'
		`, request.Provider, request.Model).Scan(&modelID)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return PriceVersionSummary{}, ErrModelNotFound
	}
	if err != nil {
		return PriceVersionSummary{}, err
	}

	lockKey := request.ScopeType + ":" + request.ScopeID + ":" + modelID
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return PriceVersionSummary{}, err
	}

	var version int64
	if request.ScopeID == "" {
		err = tx.QueryRowContext(ctx, `
			SELECT COALESCE(MAX(version), 0) + 1
			FROM price_versions
			WHERE scope_type = $1 AND scope_id IS NULL AND model_id = $2
		`, request.ScopeType, modelID).Scan(&version)
	} else {
		err = tx.QueryRowContext(ctx, `
			SELECT COALESCE(MAX(version), 0) + 1
			FROM price_versions
			WHERE scope_type = $1 AND scope_id = $2::uuid AND model_id = $3
		`, request.ScopeType, request.ScopeID, modelID).Scan(&version)
	}
	if err != nil {
		return PriceVersionSummary{}, err
	}

	if request.ScopeID == "" {
		_, err = tx.ExecContext(ctx, `
			UPDATE price_versions
			SET effective_to = $4, status = 'retired'
			WHERE scope_type = $1
			  AND scope_id IS NULL
			  AND model_id = $2
			  AND currency = $3
			  AND status = 'active'
			  AND effective_from < $4
			  AND (effective_to IS NULL OR effective_to > $4)
		`, request.ScopeType, modelID, request.Currency, effectiveFrom)
	} else {
		_, err = tx.ExecContext(ctx, `
			UPDATE price_versions
			SET effective_to = $5, status = 'retired'
			WHERE scope_type = $1
			  AND scope_id = $2::uuid
			  AND model_id = $3
			  AND currency = $4
			  AND status = 'active'
			  AND effective_from < $5
			  AND (effective_to IS NULL OR effective_to > $5)
		`, request.ScopeType, request.ScopeID, modelID, request.Currency, effectiveFrom)
	}
	if err != nil {
		return PriceVersionSummary{}, err
	}

	priceID, err := ids.New()
	if err != nil {
		return PriceVersionSummary{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO price_versions (
			id, scope_type, scope_id, model_id, currency,
			input_price_per_unit, output_price_per_unit,
			cached_input_price_per_unit, reasoning_price_per_unit,
			minimum_charge, version, effective_from, status, created_by
		) VALUES (
			$1, $2, NULLIF($3, '')::uuid, $4, $5,
			$6::numeric, $7::numeric, $8::numeric, $9::numeric,
			$10::numeric, $11, $12, 'active', $13
		)
	`, priceID, request.ScopeType, request.ScopeID, modelID, request.Currency,
		request.InputPricePerUnit, request.OutputPricePerUnit,
		request.CachedInputPricePerUnit, request.ReasoningPricePerUnit,
		request.MinimumCharge, version, effectiveFrom, actorID); err != nil {
		return PriceVersionSummary{}, err
	}
	if err := insertPriceComponents(ctx, tx, priceID, components); err != nil {
		return PriceVersionSummary{}, err
	}
	if err := tx.Commit(); err != nil {
		return PriceVersionSummary{}, err
	}

	return s.getPriceVersion(ctx, priceID)
}

func (s *SQLService) getPriceVersion(ctx context.Context, priceID string) (PriceVersionSummary, error) {
	var (
		price     PriceVersionSummary
		effective sql.NullTime
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT pv.id::text, pv.scope_type, COALESCE(pv.scope_id::text, ''),
		       pv.model_id::text, m.provider, m.model_name, pv.currency,
		       pv.input_price_per_unit::text, pv.output_price_per_unit::text,
		       pv.cached_input_price_per_unit::text,
		       pv.reasoning_price_per_unit::text, pv.minimum_charge::text,
		       pv.version, pv.effective_from, pv.effective_to, pv.status,
		       pv.created_by::text, pv.created_at
		FROM price_versions pv
		JOIN models m ON m.id = pv.model_id
		WHERE pv.id = $1
	`, priceID).Scan(
		&price.ID,
		&price.ScopeType,
		&price.ScopeID,
		&price.ModelID,
		&price.Provider,
		&price.Model,
		&price.Currency,
		&price.InputPricePerUnit,
		&price.OutputPricePerUnit,
		&price.CachedInputPricePerUnit,
		&price.ReasoningPricePerUnit,
		&price.MinimumCharge,
		&price.Version,
		&price.EffectiveFrom,
		&effective,
		&price.Status,
		&price.CreatedBy,
		&price.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return PriceVersionSummary{}, ErrPriceNotConfigured
	}
	if err != nil {
		return PriceVersionSummary{}, err
	}
	if effective.Valid {
		value := effective.Time
		price.EffectiveTo = &value
	}
	price.Currency = strings.ToUpper(strings.TrimSpace(price.Currency))
	price.Components, err = loadPriceComponents(ctx, s.db, price.ID)
	if err != nil {
		return PriceVersionSummary{}, err
	}
	return price, nil
}

func (s *SQLService) Reserve(ctx context.Context, request Request) (Reservation, error) {
	if s == nil || s.db == nil {
		return Reservation{}, ErrUnavailable
	}
	groupMultiplier, err := normalizeGroupMultiplier(request.GroupMultiplier)
	if err != nil {
		return Reservation{}, err
	}
	request.GroupMultiplier = groupMultiplier
	upstreamCostDiscount, err := normalizeUpstreamCostDiscount(request.UpstreamCostDiscount)
	if err != nil {
		return Reservation{}, err
	}
	request.UpstreamCostDiscount = upstreamCostDiscount
	meteringMode, err := normalizeMeteringMode(request.MeteringMode)
	if err != nil {
		return Reservation{}, err
	}
	request.MeteringMode = meteringMode
	if err := validateRequest(request); err != nil {
		return Reservation{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Reservation{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var existingStatus string
	err = tx.QueryRowContext(ctx, `
		SELECT status
		FROM model_requests
		WHERE idempotency_key = $1
	`, request.IdempotencyKey).Scan(&existingStatus)
	if err == nil {
		return Reservation{}, ErrDuplicateRequest
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Reservation{}, err
	}

	// Lock the token before calculating the reservation so concurrent
	// requests cannot reserve more than its configured spend limit.
	var tokenSpendLimit, tokenSpentAmount string
	err = tx.QueryRowContext(ctx, `
		SELECT spend_limit::text, spent_amount::text
		FROM api_tokens
		WHERE id = $1::uuid
		  AND status = 'active'
		  AND deleted_at IS NULL
		  AND (expires_at IS NULL OR expires_at > now())
		FOR UPDATE
	`, request.TokenID).Scan(&tokenSpendLimit, &tokenSpentAmount)
	if errors.Is(err, sql.ErrNoRows) {
		return Reservation{}, ErrInvalidRequest
	}
	if err != nil {
		return Reservation{}, err
	}

	currency, err := tenantCurrency(ctx, tx, request.TenantID)
	if err != nil {
		return Reservation{}, err
	}
	modelID, err := modelID(ctx, tx, request.Provider, request.Model)
	if err != nil {
		return Reservation{}, ErrPriceNotConfigured
	}
	price, err := resolvePrice(ctx, tx, request, modelID, currency, s.now())
	if err != nil {
		return Reservation{}, err
	}
	estimatedMetrics, err := customerMeteringUsage(requestMetricsFor(request), request.MeteringMode)
	if err != nil {
		return Reservation{}, err
	}
	estimatedMetrics = addImplicitRequestMetric(priceComponentsFor(price), estimatedMetrics, request.PricingTier)
	estimatedMetrics, err = prepareReservationMetrics(priceComponentsFor(price), estimatedMetrics, request.PricingTier)
	if err != nil {
		return Reservation{}, err
	}
	reservedCharge, err := calculateMeteredChargeForTier(
		priceComponentsFor(price), estimatedMetrics, price.MinimumCharge, request.PricingTier,
	)
	if err != nil {
		return Reservation{}, err
	}
	reservedAmount, err := multiplyAmount(ctx, tx, reservedCharge.Amount, request.GroupMultiplier)
	if err != nil {
		return Reservation{}, err
	}
	if err := checkTokenSpendLimit(ctx, tx, request.TokenID, tokenSpendLimit, tokenSpentAmount, reservedAmount); err != nil {
		return Reservation{}, err
	}
	upstreamPrice, upstreamPriceBasis, err := resolveUpstreamPrice(
		ctx, tx, modelID, currency, s.now(), price,
	)
	if err != nil {
		return Reservation{}, err
	}
	upstreamEstimatedMetrics := addImplicitRequestMetric(
		priceComponentsFor(upstreamPrice),
		requestMetricsFor(request),
		request.PricingTier,
	)
	upstreamEstimatedMetrics, err = prepareReservationMetrics(
		priceComponentsFor(upstreamPrice),
		upstreamEstimatedMetrics,
		request.PricingTier,
	)
	if err != nil {
		return Reservation{}, err
	}
	estimatedUpstreamCost, err := calculateUpstreamCost(
		priceComponentsFor(upstreamPrice),
		upstreamEstimatedMetrics,
		upstreamPrice.MinimumCharge,
		request.UpstreamCostDiscount,
		request.PricingTier,
	)
	if err != nil && strings.EqualFold(strings.TrimSpace(upstreamPrice.Source), "litellm") {
		// Official data can lag a newly introduced meter. The customer price
		// remains the safe estimation fallback until the catalog is refreshed.
		upstreamPrice = price
		upstreamPriceBasis = "customer_price_fallback"
		upstreamEstimatedMetrics = estimatedMetrics
		estimatedUpstreamCost, err = calculateUpstreamCost(
			priceComponentsFor(upstreamPrice),
			upstreamEstimatedMetrics,
			upstreamPrice.MinimumCharge,
			request.UpstreamCostDiscount,
			request.PricingTier,
		)
	}
	if err != nil {
		return Reservation{}, err
	}
	estimatedMetricsJSON := marshalJSON(estimatedMetrics, []byte(`{}`))
	priceSnapshotValue := priceSnapshot(price)
	priceSnapshotValue["pricing_tier"] = request.PricingTier
	priceSnapshotValue["group_metering_mode"] = request.MeteringMode
	priceSnapshotValue["upstream_cost_discount"] = request.UpstreamCostDiscount
	priceSnapshotJSON := marshalJSON(priceSnapshotValue, []byte(`{}`))
	upstreamPriceSnapshotJSON := marshalJSON(
		priceSnapshotWithBasis(upstreamPrice, upstreamPriceBasis),
		[]byte(`{}`),
	)

	var accountID, accountCurrency string
	err = tx.QueryRowContext(ctx, `
		SELECT id::text, currency
		FROM ledger_accounts
		WHERE tenant_id = $1
		  AND account_type = 'prepaid_balance'
		  AND status = 'active'
		  AND currency = $2
		FOR UPDATE
	`, request.TenantID, currency).Scan(&accountID, &accountCurrency)
	if errors.Is(err, sql.ErrNoRows) {
		return Reservation{}, ErrAccountNotFound
	}
	if err != nil {
		return Reservation{}, err
	}
	if accountCurrency != currency {
		return Reservation{}, ErrAccountNotFound
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE ledger_accounts
		SET balance = balance - $2::numeric
		WHERE id = $1
		  AND balance >= $2::numeric
	`, accountID, reservedAmount)
	if err != nil {
		return Reservation{}, err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return Reservation{}, err
	} else if affected != 1 {
		return Reservation{}, ErrInsufficientBalance
	}

	modelRequestID, err := ids.New()
	if err != nil {
		return Reservation{}, err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO model_requests (
			id, request_id, idempotency_key, tenant_id, project_id, token_id,
			model_id, channel_id, price_version_id, official_price_version_id, group_id, group_multiplier,
			upstream_cost_discount, estimated_upstream_cost, upstream_cost,
			status, estimated_amount, currency, endpoint, client_ip,
			request_type, reasoning_effort, price_snapshot_json,
			upstream_price_snapshot_json, usage_metrics_json, group_metering_mode
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, NULLIF($8, '')::uuid,
			NULLIF($9, '')::uuid, NULLIF($10, '')::uuid, NULLIF($11, '')::uuid,
			$12::numeric, $13::numeric, $14::numeric,
			$15::numeric, 'started', $16, $17, $18, $19, $20, $21,
			$22::jsonb, $23::jsonb, $24::jsonb, $25
		)
	`, modelRequestID, request.RequestID, request.IdempotencyKey,
		request.TenantID, request.ProjectID, request.TokenID, modelID,
		request.ChannelID, price.PriceVersionID, officialPriceVersionID(price), request.GroupID, request.GroupMultiplier,
		request.UpstreamCostDiscount, estimatedUpstreamCost, "0",
		reservedAmount, currency, request.Endpoint, request.ClientIP,
		request.RequestType, request.ReasoningEffort, priceSnapshotJSON,
		upstreamPriceSnapshotJSON, []byte(`{}`), request.MeteringMode)
	if err != nil {
		if isUniqueViolation(err) {
			return Reservation{}, ErrDuplicateRequest
		}
		return Reservation{}, err
	}

	reservationID, err := ids.New()
	if err != nil {
		return Reservation{}, err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO billing_reservations (
			id, request_id, tenant_id, account_id, reserved_amount,
			currency, status, expires_at, estimated_metrics_json, estimated_upstream_metrics_json
		) VALUES ($1, $2, $3, $4, $5, $6, 'held', $7, $8::jsonb, $9::jsonb)
	`, reservationID, modelRequestID, request.TenantID, accountID,
		reservedAmount, currency, s.now().Add(defaultReservationTTL), estimatedMetricsJSON,
		marshalJSON(upstreamEstimatedMetrics, []byte(`{}`)))
	if err != nil {
		return Reservation{}, err
	}

	if err := tx.Commit(); err != nil {
		return Reservation{}, err
	}
	return Reservation{
		ID:             reservationID,
		ModelRequestID: modelRequestID,
		AccountID:      accountID,
		Currency:       currency,
		ReservedAmount: reservedAmount,
	}, nil
}

func checkTokenSpendLimit(
	ctx context.Context,
	tx *sql.Tx,
	tokenID, spendLimit, spentAmount, reservedAmount string,
) error {
	if isZeroAmount(spendLimit) {
		return nil
	}
	var pendingAmount string
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(br.reserved_amount), 0)::text
		FROM billing_reservations br
		JOIN model_requests mr ON mr.id = br.request_id
		WHERE mr.token_id = $1::uuid
		  AND br.status IN ('held', 'pending')
	`, tokenID).Scan(&pendingAmount); err != nil {
		return err
	}
	var allowed bool
	if err := tx.QueryRowContext(ctx, `
		SELECT $1::numeric + $2::numeric + $3::numeric <= $4::numeric
	`, spentAmount, pendingAmount, reservedAmount, spendLimit).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return ErrTokenSpendLimitReached
	}
	return nil
}

func (s *SQLService) StartFreeRequest(ctx context.Context, request Request) (string, error) {
	if s == nil || s.db == nil {
		return "", ErrUnavailable
	}
	groupMultiplier, err := normalizeGroupMultiplier(request.GroupMultiplier)
	if err != nil {
		return "", err
	}
	request.GroupMultiplier = groupMultiplier
	upstreamCostDiscount, err := normalizeUpstreamCostDiscount(request.UpstreamCostDiscount)
	if err != nil {
		return "", err
	}
	request.UpstreamCostDiscount = upstreamCostDiscount
	meteringMode, err := normalizeMeteringMode(request.MeteringMode)
	if err != nil {
		return "", err
	}
	request.MeteringMode = meteringMode
	if strings.TrimSpace(request.RequestType) == "" {
		request.RequestType = "sync"
	}
	if err := validateRequest(request); err != nil {
		return "", err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	var existing string
	if err := tx.QueryRowContext(ctx, `SELECT id::text FROM model_requests WHERE idempotency_key = $1`, request.IdempotencyKey).Scan(&existing); err == nil {
		return "", ErrDuplicateRequest
	} else if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	currency, err := tenantCurrency(ctx, tx, request.TenantID)
	if err != nil {
		return "", err
	}
	modelID, err := modelID(ctx, tx, request.Provider, request.Model)
	if err != nil {
		return "", ErrModelNotFound
	}
	priceSnapshotJSON := []byte(`{}`)
	upstreamPriceSnapshotJSON := []byte(`{}`)
	estimatedUpstreamCost := "0"
	if price, priceErr := resolvePrice(ctx, tx, request, modelID, currency, s.now()); priceErr == nil {
		estimatedMetrics := requestMetricsFor(request)
		estimatedMetrics, priceErr = prepareReservationMetrics(priceComponentsFor(price), estimatedMetrics, request.PricingTier)
		if priceErr != nil {
			return "", priceErr
		}
		upstreamPrice, upstreamPriceBasis, upstreamPriceErr := resolveUpstreamPrice(
			ctx, tx, modelID, currency, s.now(), price,
		)
		if upstreamPriceErr != nil {
			return "", upstreamPriceErr
		}
		upstreamEstimatedMetrics := addImplicitRequestMetric(
			priceComponentsFor(upstreamPrice),
			requestMetricsFor(request),
			request.PricingTier,
		)
		upstreamEstimatedMetrics, priceErr = prepareReservationMetrics(
			priceComponentsFor(upstreamPrice),
			upstreamEstimatedMetrics,
			request.PricingTier,
		)
		if priceErr != nil {
			return "", priceErr
		}
		estimatedUpstreamCost, priceErr = calculateUpstreamCost(
			priceComponentsFor(upstreamPrice),
			upstreamEstimatedMetrics,
			upstreamPrice.MinimumCharge,
			request.UpstreamCostDiscount,
			request.PricingTier,
		)
		if priceErr != nil && strings.EqualFold(strings.TrimSpace(upstreamPrice.Source), "litellm") {
			upstreamPrice = price
			upstreamPriceBasis = "customer_price_fallback"
			upstreamEstimatedMetrics = estimatedMetrics
			estimatedUpstreamCost, priceErr = calculateUpstreamCost(
				priceComponentsFor(upstreamPrice),
				upstreamEstimatedMetrics,
				upstreamPrice.MinimumCharge,
				request.UpstreamCostDiscount,
				request.PricingTier,
			)
		}
		if priceErr != nil {
			return "", priceErr
		}
		snapshot := priceSnapshot(price)
		snapshot["pricing_tier"] = request.PricingTier
		snapshot["group_metering_mode"] = request.MeteringMode
		snapshot["upstream_cost_discount"] = request.UpstreamCostDiscount
		priceSnapshotJSON = marshalJSON(snapshot, []byte(`{}`))
		upstreamPriceSnapshotJSON = marshalJSON(
			priceSnapshotWithBasis(upstreamPrice, upstreamPriceBasis),
			[]byte(`{}`),
		)
	} else if !errors.Is(priceErr, ErrPriceNotConfigured) {
		return "", priceErr
	}
	modelRequestID, err := ids.New()
	if err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO model_requests (
			id, request_id, idempotency_key, tenant_id, project_id, token_id,
			model_id, channel_id,
			group_id, group_multiplier, upstream_cost_discount,
			estimated_upstream_cost, upstream_cost, status, estimated_amount,
			currency, endpoint, client_ip, request_type, reasoning_effort,
			price_snapshot_json, upstream_price_snapshot_json, group_metering_mode
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, NULLIF($8, '')::uuid,
			NULLIF($9, '')::uuid, $10::numeric, $11::numeric,
			$12::numeric, $13::numeric, 'started', 0, $14,
			$15, $16, $17, $18, $19::jsonb, $20::jsonb, $21
		)
	`, modelRequestID, request.RequestID, request.IdempotencyKey,
		request.TenantID, request.ProjectID, request.TokenID, modelID,
		request.ChannelID, request.GroupID, request.GroupMultiplier,
		request.UpstreamCostDiscount, estimatedUpstreamCost, "0", currency,
		request.Endpoint, request.ClientIP, request.RequestType,
		request.ReasoningEffort, priceSnapshotJSON, upstreamPriceSnapshotJSON, request.MeteringMode)
	if err != nil {
		if isUniqueViolation(err) {
			return "", ErrDuplicateRequest
		}
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return modelRequestID, nil
}

func (s *SQLService) CompleteFreeRequest(ctx context.Context, modelRequestID string, usage Usage, providerRequestID string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	modelRequestID = strings.TrimSpace(modelRequestID)
	if !ids.Valid(modelRequestID) || usage.InputTokens < 0 || usage.OutputTokens < 0 || usage.CachedInputTokens < 0 || usage.ReasoningTokens < 0 || !validUsageSource(usage.Source) {
		return ErrInvalidRequest
	}
	usage = normalizeUsage(usage)
	metrics, err := normalizeMeteredUsage(usageMetricsFor(usage))
	if err != nil {
		return ErrInvalidRequest
	}
	usage.Metrics = metrics
	usageMetricsJSON := marshalJSON(usage.Metrics, []byte(`{}`))
	chargeBreakdownJSON := []byte(`[]`)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var status, currency, upstreamCostDiscount string
	var priceSnapshotRaw, upstreamPriceSnapshotRaw []byte
	if err := tx.QueryRowContext(ctx, `
		SELECT status, currency, upstream_cost_discount::text,
		       price_snapshot_json, upstream_price_snapshot_json
		FROM model_requests
		WHERE id = $1
		FOR UPDATE
	`, modelRequestID).Scan(
		&status, &currency, &upstreamCostDiscount,
		&priceSnapshotRaw, &upstreamPriceSnapshotRaw,
	); errors.Is(err, sql.ErrNoRows) {
		return ErrReservationNotFound
	} else if err != nil {
		return err
	}
	if status == "settled" {
		return nil
	}
	if status != "started" {
		return ErrReservationClosed
	}
	rawUsage := usage.Raw
	if len(rawUsage) == 0 {
		rawUsage, _ = json.Marshal(usage)
	}
	upstreamCost := "0"
	upstreamPriceSnapshotJSON := []byte(`{}`)
	if upstreamPrice, priceErr := priceFromSnapshot(upstreamPriceSnapshotRaw, currency); priceErr == nil {
		var snapshot struct {
			PricingTier string `json:"pricing_tier"`
		}
		_ = json.Unmarshal(priceSnapshotRaw, &snapshot)
		upstreamPriceSnapshotJSON = upstreamPriceSnapshotRaw
		upstreamCost, priceErr = calculateUpstreamCost(
			priceComponentsFor(upstreamPrice),
			usage.Metrics,
			upstreamPrice.MinimumCharge,
			upstreamCostDiscount,
			snapshot.PricingTier,
		)
		if priceErr != nil && strings.EqualFold(strings.TrimSpace(upstreamPrice.Source), "litellm") {
			if customerPrice, customerPriceErr := priceFromSnapshot(priceSnapshotRaw, currency); customerPriceErr == nil {
				upstreamPriceSnapshotJSON = marshalJSON(
					priceSnapshotWithBasis(customerPrice, "customer_price_fallback"),
					[]byte(`{}`),
				)
				upstreamCost, priceErr = calculateUpstreamCost(
					priceComponentsFor(customerPrice),
					usage.Metrics,
					customerPrice.MinimumCharge,
					upstreamCostDiscount,
					snapshot.PricingTier,
				)
			}
		}
		if priceErr != nil && !errors.Is(priceErr, ErrPriceNotConfigured) {
			return priceErr
		}
	} else if customerPrice, priceErr := priceFromSnapshot(priceSnapshotRaw, currency); priceErr == nil {
		// Requests created before the upstream price snapshot migration only
		// have the customer-facing price snapshot available.
		upstreamPriceSnapshotJSON = marshalJSON(
			priceSnapshotWithBasis(customerPrice, "customer_price_fallback"),
			[]byte(`{}`),
		)
		upstreamCost, priceErr = calculateUpstreamCost(
			priceComponentsFor(customerPrice),
			usage.Metrics,
			customerPrice.MinimumCharge,
			upstreamCostDiscount,
			"",
		)
		if priceErr != nil && !errors.Is(priceErr, ErrPriceNotConfigured) {
			return priceErr
		}
	}
	usageEventID, err := ids.New()
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO usage_events (
			id, request_id, source, input_tokens, output_tokens,
			cached_input_tokens, reasoning_tokens, raw_usage_json, event_version,
			usage_metrics_json, charge_breakdown_json
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 1, $9::jsonb, $10::jsonb)
		ON CONFLICT (request_id, source, event_version) DO NOTHING
	`, usageEventID, modelRequestID, usage.Source, usage.InputTokens, usage.OutputTokens, usage.CachedInputTokens, usage.ReasoningTokens, rawUsage, usageMetricsJSON, chargeBreakdownJSON); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE model_requests
		SET status = 'settled', provider_request_id = NULLIF($2, ''),
			input_tokens = $3, output_tokens = $4, cached_input_tokens = $5,
			reasoning_tokens = $6, settled_amount = 0, upstream_cost = $7::numeric,
			finished_at = now(), usage_metrics_json = $8::jsonb,
			charge_breakdown_json = $9::jsonb,
			upstream_price_snapshot_json = $10::jsonb
		WHERE id = $1 AND status = 'started'
	`, modelRequestID, providerRequestID, usage.InputTokens, usage.OutputTokens, usage.CachedInputTokens, usage.ReasoningTokens, upstreamCost, usageMetricsJSON, chargeBreakdownJSON, upstreamPriceSnapshotJSON); err != nil {
		return err
	}
	return tx.Commit()
}

// MarkSettlementPending retains the reservation when the upstream accepted
// the request but did not provide a trustworthy billable usage payload.
// Reconciliation must provide the actual usage before the reservation can be
// settled; the reaper only releases ordinary held reservations.
func (s *SQLService) MarkSettlementPending(ctx context.Context, reservationID, reason string) error {
	return s.markSettlementPending(ctx, reservationID, reason, nil, "")
}

// MarkSettlementPendingWithUsage keeps the provider usage in both the request
// row and the usage event before an operator performs reconciliation. This is
// important when the provider has accepted the request but the current price
// configuration is incomplete or temporarily unavailable.
func (s *SQLService) MarkSettlementPendingWithUsage(
	ctx context.Context,
	reservationID, reason string,
	usage Usage,
	providerRequestID string,
) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	reservationID = strings.TrimSpace(reservationID)
	reason = strings.TrimSpace(reason)
	if !ids.Valid(reservationID) || reason == "" || len(reason) > 1024 ||
		usage.InputTokens < 0 || usage.OutputTokens < 0 ||
		usage.CachedInputTokens < 0 || usage.ReasoningTokens < 0 ||
		!validUsageSource(usage.Source) {
		return ErrInvalidRequest
	}
	usage = normalizeUsage(usage)
	metrics, err := normalizeMeteredUsage(usageMetricsFor(usage))
	if err != nil {
		return ErrInvalidRequest
	}
	usage.Metrics = metrics
	return s.markSettlementPending(ctx, reservationID, reason, &usage, providerRequestID)
}

func (s *SQLService) markSettlementPending(
	ctx context.Context,
	reservationID, reason string,
	usage *Usage,
	providerRequestID string,
) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	reservationID = strings.TrimSpace(reservationID)
	reason = strings.TrimSpace(reason)
	if !ids.Valid(reservationID) || reason == "" || len(reason) > 1024 {
		return ErrInvalidRequest
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var modelRequestID, status string
	err = tx.QueryRowContext(ctx, `
		SELECT request_id::text, status
		FROM billing_reservations
		WHERE id = $1
		FOR UPDATE
	`, reservationID).Scan(&modelRequestID, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrReservationNotFound
	}
	if err != nil {
		return err
	}
	if status == "settled" {
		return nil
	}
	if status != "held" && status != "pending" {
		return ErrReservationClosed
	}
	if status == "held" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE billing_reservations
			SET status = 'pending', updated_at = now()
			WHERE id = $1 AND status = 'held'
		`, reservationID); err != nil {
			return err
		}
	}
	if usage != nil && (usageHasPositiveQuantity(*usage) || len(usage.Metrics) > 0) {
		rawUsage := usage.Raw
		if len(rawUsage) == 0 {
			rawUsage, _ = json.Marshal(*usage)
		}
		usageMetricsJSON := marshalJSON(usage.Metrics, []byte(`{}`))
		chargeBreakdownJSON := []byte(`[]`)
		usageEventID, err := ids.New()
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO usage_events (
				id, request_id, source, input_tokens, output_tokens,
				cached_input_tokens, reasoning_tokens, raw_usage_json, event_version,
				usage_metrics_json, charge_breakdown_json
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 1, $9::jsonb, $10::jsonb)
			ON CONFLICT (request_id, source, event_version) DO UPDATE SET
				input_tokens = EXCLUDED.input_tokens,
				output_tokens = EXCLUDED.output_tokens,
				cached_input_tokens = EXCLUDED.cached_input_tokens,
				reasoning_tokens = EXCLUDED.reasoning_tokens,
				raw_usage_json = EXCLUDED.raw_usage_json,
				usage_metrics_json = EXCLUDED.usage_metrics_json,
				charge_breakdown_json = EXCLUDED.charge_breakdown_json
		`, usageEventID, modelRequestID, usage.Source, usage.InputTokens, usage.OutputTokens,
			usage.CachedInputTokens, usage.ReasoningTokens, rawUsage, usageMetricsJSON, chargeBreakdownJSON); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE model_requests
			SET status = 'settlement_pending',
			    provider_request_id = COALESCE(NULLIF($2, ''), provider_request_id),
			    failure_reason = $3,
			    input_tokens = $4,
			    output_tokens = $5,
			    cached_input_tokens = $6,
			    reasoning_tokens = $7,
			    usage_metrics_json = $8::jsonb,
			    charge_breakdown_json = $9::jsonb
			WHERE id = $1 AND status IN ('started', 'settlement_pending')
		`, modelRequestID, strings.TrimSpace(providerRequestID), reason,
			usage.InputTokens, usage.OutputTokens, usage.CachedInputTokens,
			usage.ReasoningTokens, usageMetricsJSON, chargeBreakdownJSON); err != nil {
			return err
		}
	} else if _, err := tx.ExecContext(ctx, `
		UPDATE model_requests
		SET status = 'settlement_pending', failure_reason = $2
		WHERE id = $1 AND status IN ('started', 'settlement_pending')
	`, modelRequestID, reason); err != nil {
		return err
	}
	return tx.Commit()
}

// SettleByModelRequestID is the reconciliation entry point used by platform
// operators. Model request IDs are shown in usage records; reservation IDs are
// deliberately kept as internal billing identifiers.
func (s *SQLService) SettleByModelRequestID(ctx context.Context, modelRequestID string, usage Usage, providerRequestID string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	modelRequestID = strings.TrimSpace(modelRequestID)
	if !ids.Valid(modelRequestID) {
		return ErrInvalidRequest
	}
	var reservationID, status string
	err := s.db.QueryRowContext(ctx, `
		SELECT id::text, status
		FROM billing_reservations
		WHERE request_id = $1::uuid
	`, modelRequestID).Scan(&reservationID, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrReservationNotFound
	}
	if err != nil {
		return err
	}
	if status == "settled" {
		return nil
	}
	if status != "pending" {
		return ErrSettlementPending
	}
	if strings.TrimSpace(usage.Source) == "" {
		usage.Source = "reconciliation"
	}
	return s.Settle(ctx, reservationID, usage, providerRequestID)
}

func usageHasPositiveQuantity(usage Usage) bool {
	if usage.InputTokens > 0 || usage.OutputTokens > 0 || usage.CachedInputTokens > 0 || usage.ReasoningTokens > 0 {
		return true
	}
	return meteredUsageHasPositiveQuantity(usage.Metrics)
}

func meteredUsageHasPositiveQuantity(metrics MeteredUsage) bool {
	for _, value := range metrics {
		quantity, ok := new(big.Rat).SetString(strings.TrimSpace(value))
		if ok && quantity.Sign() > 0 {
			return true
		}
	}
	return false
}

func priceAllowsZeroUsage(price Price, pricingTier string, meteringModes ...string) bool {
	meteringMode := MeteringToken
	if len(meteringModes) > 0 {
		meteringMode = meteringModes[0]
	}
	normalizedMode, err := normalizeMeteringMode(meteringMode)
	if err != nil {
		return false
	}
	// Image and duration billing require an actual provider result (or the
	// media estimate captured before dispatch). A minimum charge must not turn
	// an unknown image count or video duration into a customer charge.
	if normalizedMode != MeteringToken {
		return false
	}
	if !isZeroAmount(price.MinimumCharge) {
		return true
	}
	return hasComponentForTier(priceComponentsFor(price), "requests", pricingTier)
}

func settlementPricingTier(snapshotTier, reportedTier string) string {
	if tier := normalizePricingTier(snapshotTier); tier != "" {
		return tier
	}
	return normalizePricingTier(reportedTier)
}

func (s *SQLService) FailFreeRequest(ctx context.Context, modelRequestID, reason string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	modelRequestID = strings.TrimSpace(modelRequestID)
	reason = strings.TrimSpace(reason)
	if !ids.Valid(modelRequestID) || len(reason) > 1024 {
		return ErrInvalidRequest
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE model_requests
		SET status = 'failed', failure_reason = NULLIF($2, ''), finished_at = now()
		WHERE id = $1 AND status = 'started'
	`, modelRequestID, reason)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected == 0 {
		return nil
	}
	return nil
}

func (s *SQLService) RebindRequestChannel(ctx context.Context, modelRequestID, channelID string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	modelRequestID = strings.TrimSpace(modelRequestID)
	channelID = strings.TrimSpace(channelID)
	if !ids.Valid(modelRequestID) || !ids.Valid(channelID) {
		return ErrInvalidRequest
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var (
		currentModelID, currentModel                            string
		targetModelID, targetProvider, targetCostDiscount       string
		tenantID, projectID, tokenID, currency, groupMultiplier string
		estimatedUpstreamCost                                   string
		estimatedMetricsRaw, priceSnapshotRaw                   []byte
		upstreamPriceSnapshotRaw                                []byte
	)
	err = tx.QueryRowContext(ctx, `
		SELECT mr.model_id::text, current_model.model_name,
		       target_model.id::text, target_model.provider,
		       c.upstream_cost_discount::text,
		       mr.tenant_id::text, mr.project_id::text, mr.token_id::text, mr.currency,
		       mr.group_multiplier::text,
		       mr.estimated_upstream_cost::text, mr.usage_metrics_json,
		       mr.price_snapshot_json, mr.upstream_price_snapshot_json
		FROM model_requests mr
		JOIN models current_model ON current_model.id = mr.model_id
		JOIN channels c
		  ON c.id = $2::uuid
		 AND c.status = 'active'
		 AND c.deleted_at IS NULL
		JOIN channel_models target_mapping
		  ON target_mapping.channel_id = c.id
		 AND target_mapping.enabled = true
		JOIN models target_model
		  ON target_model.id = target_mapping.model_id
		 AND target_model.status = 'active'
		 AND target_model.model_name = current_model.model_name
		WHERE mr.id = $1
		  AND mr.status = 'started'
		FOR UPDATE OF mr
	`, modelRequestID, channelID).Scan(
		&currentModelID, &currentModel,
		&targetModelID, &targetProvider,
		&targetCostDiscount,
		&tenantID, &projectID, &tokenID, &currency, &groupMultiplier,
		&estimatedUpstreamCost,
		&estimatedMetricsRaw, &priceSnapshotRaw, &upstreamPriceSnapshotRaw,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrModelNotFound
	}
	if err != nil {
		return err
	}

	priceSnapshotValue := map[string]any{}
	if len(priceSnapshotRaw) > 0 && string(priceSnapshotRaw) != "null" {
		_ = json.Unmarshal(priceSnapshotRaw, &priceSnapshotValue)
	}
	pricingTier := ""
	if value, ok := priceSnapshotValue["pricing_tier"].(string); ok {
		pricingTier = value
	}

	if targetModelID == currentModelID {
		estimatedUpstreamCost = "0"
		if customerPrice, priceErr := priceFromSnapshot(priceSnapshotRaw, ""); priceErr == nil {
			var estimatedMetrics MeteredUsage
			if len(estimatedMetricsRaw) > 0 && string(estimatedMetricsRaw) != "null" {
				_ = json.Unmarshal(estimatedMetricsRaw, &estimatedMetrics)
			}
			upstreamPrice, upstreamPriceBasis, upstreamPriceErr := resolveUpstreamPrice(
				ctx, tx, targetModelID, currency, s.now(), customerPrice,
			)
			if upstreamPriceErr != nil {
				return upstreamPriceErr
			}
			estimatedUpstreamCost, priceErr = calculateUpstreamCost(
				priceComponentsFor(upstreamPrice),
				estimatedMetrics,
				upstreamPrice.MinimumCharge,
				targetCostDiscount,
				pricingTier,
			)
			if priceErr != nil && strings.EqualFold(strings.TrimSpace(upstreamPrice.Source), "litellm") {
				upstreamPrice = customerPrice
				upstreamPriceBasis = "customer_price_fallback"
				estimatedUpstreamCost, priceErr = calculateUpstreamCost(
					priceComponentsFor(upstreamPrice),
					estimatedMetrics,
					upstreamPrice.MinimumCharge,
					targetCostDiscount,
					pricingTier,
				)
			}
			if priceErr != nil {
				return priceErr
			}
			priceSnapshotValue["upstream_cost_discount"] = targetCostDiscount
			upstreamPriceSnapshotRaw = marshalJSON(
				priceSnapshotWithBasis(upstreamPrice, upstreamPriceBasis),
				[]byte(`{}`),
			)
		} else {
			priceSnapshotValue = map[string]any{}
			upstreamPriceSnapshotRaw = []byte(`{}`)
		}
	} else {
		// A free request may start without a configured platform price. Do not
		// carry the previous provider's snapshot into a cross-provider fallback.
		priceSnapshotValue = map[string]any{}
		estimatedUpstreamCost = "0"
		price, priceErr := resolvePrice(ctx, tx, Request{
			TenantID: tenantID, ProjectID: projectID, TokenID: tokenID,
			Model: currentModel, Provider: targetProvider,
			GroupMultiplier: groupMultiplier,
		}, targetModelID, currency, s.now())
		if priceErr == nil {
			var estimatedMetrics MeteredUsage
			if len(estimatedMetricsRaw) > 0 && string(estimatedMetricsRaw) != "null" {
				if err := json.Unmarshal(estimatedMetricsRaw, &estimatedMetrics); err != nil {
					return ErrInvalidRequest
				}
			}
			upstreamPrice, upstreamPriceBasis, upstreamPriceErr := resolveUpstreamPrice(
				ctx, tx, targetModelID, currency, s.now(), price,
			)
			if upstreamPriceErr != nil {
				return upstreamPriceErr
			}
			estimatedMetrics, err = prepareReservationMetrics(priceComponentsFor(upstreamPrice), estimatedMetrics, pricingTier)
			if err != nil {
				return err
			}
			estimatedUpstreamCost, err = calculateUpstreamCost(
				priceComponentsFor(upstreamPrice),
				estimatedMetrics,
				upstreamPrice.MinimumCharge,
				targetCostDiscount,
				pricingTier,
			)
			if err != nil {
				return err
			}
			priceSnapshotValue = priceSnapshot(price)
			priceSnapshotValue["pricing_tier"] = pricingTier
			priceSnapshotValue["upstream_cost_discount"] = targetCostDiscount
			upstreamPriceSnapshotRaw = marshalJSON(
				priceSnapshotWithBasis(upstreamPrice, upstreamPriceBasis),
				[]byte(`{}`),
			)
		} else if !errors.Is(priceErr, ErrPriceNotConfigured) {
			return priceErr
		}
	}
	if len(priceSnapshotValue) == 0 {
		priceSnapshotValue["upstream_cost_discount"] = targetCostDiscount
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE model_requests
		SET channel_id = $2,
		    model_id = $3,
		    upstream_cost_discount = $4::numeric,
		    estimated_upstream_cost = $5::numeric,
		    price_snapshot_json = $6::jsonb,
		    upstream_price_snapshot_json = $7::jsonb
		WHERE id = $1 AND status = 'started'
	`, modelRequestID, channelID, targetModelID, targetCostDiscount,
		estimatedUpstreamCost, marshalJSON(priceSnapshotValue, []byte(`{}`)),
		upstreamPriceSnapshotRaw)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected != 1 {
		return ErrReservationClosed
	}
	return tx.Commit()
}

func (s *SQLService) RecordRequestMetrics(ctx context.Context, modelRequestID string, latencyMS int64) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	modelRequestID = strings.TrimSpace(modelRequestID)
	if !ids.Valid(modelRequestID) || latencyMS < 0 {
		return ErrInvalidRequest
	}
	_, err := s.db.ExecContext(ctx, `UPDATE model_requests SET latency_ms = $2 WHERE id = $1`, modelRequestID, latencyMS)
	return err
}

func (s *SQLService) Settle(
	ctx context.Context,
	reservationID string,
	usage Usage,
	providerRequestID string,
) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	reservationID = strings.TrimSpace(reservationID)
	if !ids.Valid(reservationID) || usage.InputTokens < 0 || usage.OutputTokens < 0 ||
		usage.CachedInputTokens < 0 || usage.ReasoningTokens < 0 || !validUsageSource(usage.Source) {
		return ErrInvalidRequest
	}
	usage = normalizeUsage(usage)
	metrics, err := normalizeMeteredUsage(usageMetricsFor(usage))
	if err != nil {
		return ErrInvalidRequest
	}
	usage.Metrics = metrics

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var (
		modelRequestID           string
		tokenID                  string
		accountID                string
		reservedAmount           string
		currency                 string
		status                   string
		priceID                  sql.NullString
		officialPriceID          sql.NullString
		groupMultiplier          string
		meteringMode             string
		upstreamCostDiscount     string
		estimatedUpstreamCost    string
		estimatedMetricsRaw      []byte
		estimatedUpstreamRaw     []byte
		priceSnapshotRaw         []byte
		upstreamPriceSnapshotRaw []byte
	)
	err = tx.QueryRowContext(ctx, `
		SELECT br.request_id::text, mr.token_id::text, br.account_id::text, br.reserved_amount::text,
		       br.currency, br.status, mr.price_version_id::text,
		       mr.official_price_version_id::text,
		       mr.group_multiplier::text, mr.group_metering_mode,
		       mr.upstream_cost_discount::text,
		       mr.estimated_upstream_cost::text, br.estimated_metrics_json,
		       br.estimated_upstream_metrics_json, mr.price_snapshot_json,
		       mr.upstream_price_snapshot_json
		FROM billing_reservations br
		JOIN model_requests mr ON mr.id = br.request_id
		WHERE br.id = $1
		FOR UPDATE
	`, reservationID).Scan(
		&modelRequestID, &tokenID, &accountID, &reservedAmount, &currency, &status, &priceID,
		&officialPriceID, &groupMultiplier, &meteringMode, &upstreamCostDiscount,
		&estimatedUpstreamCost, &estimatedMetricsRaw, &estimatedUpstreamRaw,
		&priceSnapshotRaw, &upstreamPriceSnapshotRaw,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrReservationNotFound
	}
	if err != nil {
		return err
	}
	if status == "settled" {
		return nil
	}
	if status != "held" && status != "pending" {
		return ErrReservationClosed
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT id::text
		FROM api_tokens
		WHERE id = $1::uuid
		FOR UPDATE
	`, tokenID).Scan(&tokenID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidRequest
		}
		return err
	}
	var requestPricingTier string
	if len(priceSnapshotRaw) > 0 && string(priceSnapshotRaw) != "null" {
		var snapshot struct {
			PricingTier string `json:"pricing_tier"`
		}
		_ = json.Unmarshal(priceSnapshotRaw, &snapshot)
		requestPricingTier = strings.TrimSpace(snapshot.PricingTier)
	}
	// Providers and manual reconciliation callers do not always echo the
	// service tier. The request snapshot is authoritative for the selected
	// priority, flex, or batch price; accepting a different reported tier
	// would allow the same request to settle against a different price rule.
	usage.PricingTier = settlementPricingTier(requestPricingTier, usage.PricingTier)

	var inputPrice, outputPrice, minimumCharge string
	var cachedInputPrice, reasoningPrice string
	var price Price
	if priceID.Valid && strings.TrimSpace(priceID.String) != "" {
		if err := tx.QueryRowContext(ctx, `
			SELECT input_price_per_unit::text, output_price_per_unit::text,
			       cached_input_price_per_unit::text,
			       reasoning_price_per_unit::text,
			       minimum_charge::text
			FROM price_versions
			WHERE id = $1
		`, priceID.String).Scan(
			&inputPrice,
			&outputPrice,
			&cachedInputPrice,
			&reasoningPrice,
			&minimumCharge,
		); err != nil {
			return ErrPriceNotConfigured
		}
		price = Price{
			ID: priceID.String, PriceVersionID: priceID.String, Source: "manual", Currency: currency,
			InputPricePerUnit: inputPrice, OutputPricePerUnit: outputPrice,
			CachedInputPricePerUnit: cachedInputPrice, ReasoningPricePerUnit: reasoningPrice,
			MinimumCharge: minimumCharge,
		}
		price.Components, err = loadPriceComponents(ctx, tx, priceID.String)
		if err != nil {
			return err
		}
	} else if officialPriceID.Valid && strings.TrimSpace(officialPriceID.String) != "" {
		if err := tx.QueryRowContext(ctx, `
			SELECT currency, input_price_per_unit::text,
			       output_price_per_unit::text,
			       cached_input_price_per_unit::text,
			       reasoning_price_per_unit::text,
			       '0'::text
			FROM official_model_price_versions
			WHERE id = $1
		`, officialPriceID.String).Scan(
			&currency, &inputPrice, &outputPrice, &cachedInputPrice,
			&reasoningPrice, &minimumCharge,
		); err != nil {
			return ErrPriceNotConfigured
		}
		price = Price{
			ID: officialPriceID.String, Source: "litellm", Currency: currency,
			InputPricePerUnit: inputPrice, OutputPricePerUnit: outputPrice,
			CachedInputPricePerUnit: cachedInputPrice, ReasoningPricePerUnit: reasoningPrice,
			MinimumCharge: minimumCharge,
		}
		price.Components, err = loadOfficialPriceComponents(ctx, tx, officialPriceID.String)
		if err != nil {
			return err
		}
	} else {
		price, err = priceFromSnapshot(priceSnapshotRaw, currency)
		if err != nil {
			return err
		}
	}
	var estimatedCustomerMetrics MeteredUsage
	if len(estimatedMetricsRaw) > 0 && string(estimatedMetricsRaw) != "null" {
		if err := json.Unmarshal(estimatedMetricsRaw, &estimatedCustomerMetrics); err != nil {
			return ErrInvalidRequest
		}
	}
	customerMetrics, err := customerMeteringUsageWithEstimate(usage.Metrics, estimatedCustomerMetrics, meteringMode)
	if err != nil {
		return err
	}
	if !meteredUsageHasPositiveQuantity(customerMetrics) && !priceAllowsZeroUsage(price, usage.PricingTier, meteringMode) {
		return ErrUsageUnavailable
	}
	customerMetrics = addImplicitRequestMetric(priceComponentsFor(price), customerMetrics, usage.PricingTier)
	charge, err := calculateMeteredChargeForTier(priceComponentsFor(price), customerMetrics, price.MinimumCharge, usage.PricingTier)
	if err != nil {
		return err
	}
	charge, err = multiplierCharge(charge, groupMultiplier)
	if err != nil {
		return err
	}
	actualAmount := charge.Amount
	upstreamPrice := price
	upstreamPriceSnapshotJSON := marshalJSON(
		priceSnapshotWithBasis(price, "customer_price_fallback"),
		[]byte(`{}`),
	)
	if snapshotPrice, snapshotErr := priceFromSnapshot(upstreamPriceSnapshotRaw, currency); snapshotErr == nil {
		upstreamPrice = snapshotPrice
		upstreamPriceSnapshotJSON = upstreamPriceSnapshotRaw
	}
	upstreamMetrics := usage.Metrics
	if len(upstreamMetrics) == 0 && len(estimatedUpstreamRaw) > 0 && string(estimatedUpstreamRaw) != "null" {
		if err := json.Unmarshal(estimatedUpstreamRaw, &upstreamMetrics); err != nil {
			return ErrInvalidRequest
		}
	}
	upstreamCost, err := calculateUpstreamCost(
		priceComponentsFor(upstreamPrice),
		upstreamMetrics,
		upstreamPrice.MinimumCharge,
		upstreamCostDiscount,
		usage.PricingTier,
	)
	if err != nil && strings.EqualFold(strings.TrimSpace(upstreamPrice.Source), "litellm") {
		// An official catalog can lag a newly introduced provider meter. Keep
		// customer settlement usable and mark the cost basis as a fallback
		// instead of blocking the successful upstream request.
		upstreamPrice = price
		upstreamPriceSnapshotJSON = marshalJSON(
			priceSnapshotWithBasis(price, "customer_price_fallback"),
			[]byte(`{}`),
		)
		upstreamCost, err = calculateUpstreamCost(
			priceComponentsFor(upstreamPrice),
			upstreamMetrics,
			upstreamPrice.MinimumCharge,
			upstreamCostDiscount,
			usage.PricingTier,
		)
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE api_tokens
		SET spent_amount = spent_amount + $2::numeric
		WHERE id = $1::uuid
	`, tokenID, actualAmount); err != nil {
		return err
	}
	usageMetricsJSON := marshalJSON(usage.Metrics, []byte(`{}`))
	chargeBreakdownJSON := marshalJSON(charge.Lines, []byte(`[]`))
	priceSnapshotValue := priceSnapshot(price)
	priceSnapshotValue["pricing_tier"] = usage.PricingTier
	priceSnapshotValue["group_multiplier"] = groupMultiplier
	priceSnapshotValue["group_metering_mode"] = meteringMode
	priceSnapshotValue["upstream_cost_discount"] = upstreamCostDiscount
	priceSnapshotJSON := marshalJSON(priceSnapshotValue, []byte(`{}`))

	// The reservation already reduced the available balance. Final usage may
	// still exceed the reservation when an upstream returns more output than
	// the estimate or a media job reports its final usage later. Settle the
	// complete amount so a successful upstream request is never left in `held`;
	// a negative prepaid balance blocks subsequent reservations until credit.
	result, err := tx.ExecContext(ctx, `
		UPDATE ledger_accounts
		SET balance = balance + $2::numeric - $3::numeric
		WHERE id = $1
	`, accountID, reservedAmount, actualAmount)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected != 1 {
		return ErrAccountNotFound
	}

	if !isZeroAmount(actualAmount) {
		systemAccountID, err := ensureSystemAccount(ctx, tx, currency)
		if err != nil {
			return err
		}
		transactionID, err := ids.New()
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO ledger_transactions (
				id, idempotency_key, transaction_type, reference_type, reference_id
			) VALUES ($1, $2, 'model_usage', 'model_request', $3)
		`, transactionID, "billing:charge:"+modelRequestID, modelRequestID); err != nil {
			return err
		}
		userLineID, err := ids.New()
		if err != nil {
			return err
		}
		systemLineID, err := ids.New()
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO ledger_lines (
				id, transaction_id, account_id, direction, amount, currency
			) VALUES
				($1, $2, $3, 'debit', $4, $5),
				($6, $2, $7, 'credit', $4, $5)
		`, userLineID, transactionID, accountID, actualAmount, currency,
			systemLineID, systemAccountID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE ledger_accounts
			SET balance = balance + $2::numeric
			WHERE id = $1
		`, systemAccountID, actualAmount); err != nil {
			return err
		}
	}

	rawUsage := usage.Raw
	if len(rawUsage) == 0 {
		rawUsage, _ = json.Marshal(usage)
	}
	usageEventID, err := ids.New()
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO usage_events (
			id, request_id, source, input_tokens, output_tokens,
			cached_input_tokens, reasoning_tokens, raw_usage_json, event_version,
			usage_metrics_json, charge_breakdown_json
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 1, $9::jsonb, $10::jsonb)
		ON CONFLICT (request_id, source, event_version) DO UPDATE SET
			input_tokens = EXCLUDED.input_tokens,
			output_tokens = EXCLUDED.output_tokens,
			cached_input_tokens = EXCLUDED.cached_input_tokens,
			reasoning_tokens = EXCLUDED.reasoning_tokens,
			raw_usage_json = EXCLUDED.raw_usage_json,
			usage_metrics_json = EXCLUDED.usage_metrics_json,
			charge_breakdown_json = EXCLUDED.charge_breakdown_json
	`, usageEventID, modelRequestID, usage.Source, usage.InputTokens, usage.OutputTokens,
		usage.CachedInputTokens, usage.ReasoningTokens, rawUsage, usageMetricsJSON, chargeBreakdownJSON)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE billing_reservations
		SET settled_amount = $2,
		    released_amount = GREATEST(reserved_amount - $2::numeric, 0),
		    status = 'settled',
		    updated_at = now()
		WHERE id = $1 AND status IN ('held', 'pending')
	`, reservationID, actualAmount); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE model_requests
		SET status = 'settled',
		    provider_request_id = NULLIF($2, ''),
		    input_tokens = $3,
		    output_tokens = $4,
		cached_input_tokens = $5,
		reasoning_tokens = $6,
		settled_amount = $7,
		usage_metrics_json = $8::jsonb,
		charge_breakdown_json = $9::jsonb,
		price_snapshot_json = $10::jsonb,
		upstream_cost_discount = $11::numeric,
		estimated_upstream_cost = $12::numeric,
		upstream_cost = $13::numeric,
		upstream_price_snapshot_json = $14::jsonb,
		finished_at = now()
		WHERE id = $1
	`, modelRequestID, providerRequestID, usage.InputTokens, usage.OutputTokens,
		usage.CachedInputTokens, usage.ReasoningTokens, actualAmount, usageMetricsJSON,
		chargeBreakdownJSON, priceSnapshotJSON, upstreamCostDiscount, estimatedUpstreamCost,
		upstreamCost, upstreamPriceSnapshotJSON); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLService) CanSettleWithoutUsage(ctx context.Context, reservationID string) (bool, error) {
	if s == nil || s.db == nil {
		return false, ErrUnavailable
	}
	reservationID = strings.TrimSpace(reservationID)
	if !ids.Valid(reservationID) {
		return false, ErrInvalidRequest
	}
	var status, currency, meteringMode string
	var snapshotRaw, estimatedMetricsRaw []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT br.status, br.currency, mr.price_snapshot_json, mr.group_metering_mode,
		       br.estimated_metrics_json
		FROM billing_reservations br
		JOIN model_requests mr ON mr.id = br.request_id
		WHERE br.id = $1
	`, reservationID).Scan(&status, &currency, &snapshotRaw, &meteringMode, &estimatedMetricsRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrReservationNotFound
	}
	if err != nil {
		return false, err
	}
	if status == "settled" {
		return true, nil
	}
	if status != "held" && status != "pending" {
		return false, ErrReservationClosed
	}
	price, err := priceFromSnapshot(snapshotRaw, currency)
	if err != nil {
		return false, err
	}
	var snapshot struct {
		PricingTier string `json:"pricing_tier"`
	}
	if len(snapshotRaw) > 0 && string(snapshotRaw) != "null" {
		_ = json.Unmarshal(snapshotRaw, &snapshot)
	}
	var estimatedMetrics MeteredUsage
	if len(estimatedMetricsRaw) > 0 && string(estimatedMetricsRaw) != "null" {
		if err := json.Unmarshal(estimatedMetricsRaw, &estimatedMetrics); err != nil {
			return false, ErrInvalidRequest
		}
	}
	customerMetrics, err := customerMeteringUsageWithEstimate(nil, estimatedMetrics, meteringMode)
	if err != nil {
		return false, err
	}
	if meteredUsageHasPositiveQuantity(customerMetrics) {
		return true, nil
	}
	return priceAllowsZeroUsage(price, snapshot.PricingTier, meteringMode), nil
}

func (s *SQLService) RebindReservationChannel(ctx context.Context, reservationID, channelID string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	reservationID = strings.TrimSpace(reservationID)
	channelID = strings.TrimSpace(channelID)
	if !ids.Valid(reservationID) || !ids.Valid(channelID) {
		return ErrInvalidRequest
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var (
		modelRequestID, accountID, oldReserved, currency, status   string
		tenantID, projectID, tokenID, currentModel, currentModelID string
		groupMultiplier                                            string
		startedAt                                                  time.Time
		estimatedMetricsRaw, estimatedUpstreamMetricsRaw           []byte
		priceSnapshotRaw                                           []byte
		upstreamPriceSnapshotRaw                                   []byte
		provider, targetModelID, targetCostDiscount                string
	)
	err = tx.QueryRowContext(ctx, `
		SELECT br.request_id::text, br.account_id::text, br.reserved_amount::text,
		       br.currency, br.status, mr.tenant_id::text, mr.project_id::text,
		       mr.token_id::text, mr.model_id::text, current_model.model_name,
		       mr.group_multiplier::text, br.estimated_metrics_json,
		       br.estimated_upstream_metrics_json, mr.price_snapshot_json,
		       mr.upstream_price_snapshot_json, mr.started_at,
		       c.provider, target_cm.model_id::text, c.upstream_cost_discount::text
		FROM billing_reservations br
		JOIN model_requests mr ON mr.id = br.request_id
		JOIN models current_model ON current_model.id = mr.model_id
		JOIN channels c ON c.id = $2::uuid
		 AND c.status = 'active'
		 AND c.deleted_at IS NULL
		JOIN channel_models target_cm ON target_cm.channel_id = c.id AND target_cm.enabled = true
		JOIN models target_model ON target_model.id = target_cm.model_id AND target_model.status = 'active'
		WHERE br.id = $1
		  AND target_model.model_name = current_model.model_name
		FOR UPDATE OF br, mr
	`, reservationID, channelID).Scan(
		&modelRequestID, &accountID, &oldReserved, &currency, &status,
		&tenantID, &projectID, &tokenID, &currentModelID, &currentModel,
		&groupMultiplier, &estimatedMetricsRaw, &estimatedUpstreamMetricsRaw,
		&priceSnapshotRaw, &upstreamPriceSnapshotRaw,
		&startedAt, &provider, &targetModelID,
		&targetCostDiscount,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrReservationNotFound
	}
	if err != nil {
		return err
	}
	if status == "settled" {
		return nil
	}
	if status != "held" && status != "pending" {
		return ErrReservationClosed
	}
	if targetModelID == currentModelID {
		estimatedUpstreamCost := "0"
		priceSnapshotValue := map[string]any{}
		if len(priceSnapshotRaw) > 0 && string(priceSnapshotRaw) != "null" {
			_ = json.Unmarshal(priceSnapshotRaw, &priceSnapshotValue)
		}
		if customerPrice, priceErr := priceFromSnapshot(priceSnapshotRaw, currency); priceErr == nil {
			var estimatedMetrics MeteredUsage
			if len(estimatedMetricsRaw) > 0 && string(estimatedMetricsRaw) != "null" {
				_ = json.Unmarshal(estimatedMetricsRaw, &estimatedMetrics)
			}
			var upstreamEstimatedMetrics MeteredUsage
			if len(estimatedUpstreamMetricsRaw) > 0 && string(estimatedUpstreamMetricsRaw) != "null" {
				_ = json.Unmarshal(estimatedUpstreamMetricsRaw, &upstreamEstimatedMetrics)
			}
			if len(upstreamEstimatedMetrics) == 0 {
				upstreamEstimatedMetrics = estimatedMetrics
			}
			upstreamPrice, upstreamPriceBasis, upstreamPriceErr := resolveUpstreamPrice(
				ctx, tx, targetModelID, currency, s.now(), customerPrice,
			)
			if upstreamPriceErr != nil {
				return upstreamPriceErr
			}
			pricingTier := ""
			if value, ok := priceSnapshotValue["pricing_tier"].(string); ok {
				pricingTier = value
			}
			estimatedUpstreamCost, priceErr = calculateUpstreamCost(
				priceComponentsFor(upstreamPrice),
				upstreamEstimatedMetrics,
				upstreamPrice.MinimumCharge,
				targetCostDiscount,
				pricingTier,
			)
			if priceErr != nil {
				return priceErr
			}
			priceSnapshotValue["upstream_cost_discount"] = targetCostDiscount
			upstreamPriceSnapshotRaw = marshalJSON(
				priceSnapshotWithBasis(upstreamPrice, upstreamPriceBasis),
				[]byte(`{}`),
			)
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE model_requests
			SET channel_id = $2,
			    upstream_cost_discount = $3::numeric,
			    estimated_upstream_cost = $4::numeric,
			    price_snapshot_json = $5::jsonb,
			    upstream_price_snapshot_json = $6::jsonb
			WHERE id = $1 AND status IN ('started', 'settlement_pending')
		`, modelRequestID, channelID, targetCostDiscount, estimatedUpstreamCost,
			marshalJSON(priceSnapshotValue, []byte(`{}`)), upstreamPriceSnapshotRaw)
		if err != nil {
			return err
		}
		if affected, err := result.RowsAffected(); err != nil {
			return err
		} else if affected != 1 {
			return ErrReservationClosed
		}
		return tx.Commit()
	}
	var snapshot struct {
		PricingTier string `json:"pricing_tier"`
	}
	if len(priceSnapshotRaw) > 0 {
		_ = json.Unmarshal(priceSnapshotRaw, &snapshot)
	}
	price, err := resolvePrice(ctx, tx, Request{
		TenantID: tenantID, ProjectID: projectID, TokenID: tokenID,
		Model: currentModel, Provider: provider, GroupMultiplier: groupMultiplier,
	}, targetModelID, currency, startedAt)
	if err != nil {
		return err
	}
	var estimatedMetrics MeteredUsage
	if len(estimatedMetricsRaw) > 0 && string(estimatedMetricsRaw) != "null" {
		if err := json.Unmarshal(estimatedMetricsRaw, &estimatedMetrics); err != nil {
			return ErrInvalidRequest
		}
	}
	var upstreamEstimatedMetrics MeteredUsage
	if len(estimatedUpstreamMetricsRaw) > 0 && string(estimatedUpstreamMetricsRaw) != "null" {
		if err := json.Unmarshal(estimatedUpstreamMetricsRaw, &upstreamEstimatedMetrics); err != nil {
			return ErrInvalidRequest
		}
	}
	if len(upstreamEstimatedMetrics) == 0 {
		upstreamEstimatedMetrics = estimatedMetrics
	}
	estimatedMetrics = addImplicitRequestMetric(priceComponentsFor(price), estimatedMetrics, snapshot.PricingTier)
	estimatedMetrics, err = prepareReservationMetrics(priceComponentsFor(price), estimatedMetrics, snapshot.PricingTier)
	if err != nil {
		return err
	}
	estimatedCharge, err := calculateMeteredChargeForTier(priceComponentsFor(price), estimatedMetrics, price.MinimumCharge, snapshot.PricingTier)
	if err != nil {
		return err
	}
	newReserved, err := multiplyAmount(ctx, tx, estimatedCharge.Amount, groupMultiplier)
	if err != nil {
		return err
	}
	upstreamPrice, upstreamPriceBasis, err := resolveUpstreamPrice(
		ctx, tx, targetModelID, currency, s.now(), price,
	)
	if err != nil {
		return err
	}
	upstreamEstimatedMetrics = addImplicitRequestMetric(
		priceComponentsFor(upstreamPrice), upstreamEstimatedMetrics, snapshot.PricingTier,
	)
	upstreamEstimatedMetrics, err = prepareReservationMetrics(
		priceComponentsFor(upstreamPrice), upstreamEstimatedMetrics, snapshot.PricingTier,
	)
	if err != nil {
		return err
	}
	estimatedUpstreamCost, err := calculateUpstreamCost(
		priceComponentsFor(upstreamPrice),
		upstreamEstimatedMetrics,
		upstreamPrice.MinimumCharge,
		targetCostDiscount,
		snapshot.PricingTier,
	)
	if err != nil && strings.EqualFold(strings.TrimSpace(upstreamPrice.Source), "litellm") {
		upstreamPrice = price
		upstreamPriceBasis = "customer_price_fallback"
		estimatedUpstreamCost, err = calculateUpstreamCost(
			priceComponentsFor(upstreamPrice),
			upstreamEstimatedMetrics,
			upstreamPrice.MinimumCharge,
			targetCostDiscount,
			snapshot.PricingTier,
		)
	}
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE ledger_accounts
		SET balance = balance + $2::numeric - $3::numeric
		WHERE id = $1
		  AND balance + $2::numeric >= $3::numeric
	`, accountID, oldReserved, newReserved)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected != 1 {
		return ErrInsufficientBalance
	}
	priceSnapshotValue := priceSnapshot(price)
	priceSnapshotValue["pricing_tier"] = snapshot.PricingTier
	priceSnapshotValue["group_multiplier"] = groupMultiplier
	priceSnapshotValue["upstream_cost_discount"] = targetCostDiscount
	priceSnapshotJSON := marshalJSON(priceSnapshotValue, []byte(`{}`))
	upstreamPriceSnapshotRaw = marshalJSON(
		priceSnapshotWithBasis(upstreamPrice, upstreamPriceBasis),
		[]byte(`{}`),
	)
	if _, err := tx.ExecContext(ctx, `
		UPDATE billing_reservations
		SET reserved_amount = $2::numeric,
		    estimated_metrics_json = $3::jsonb,
		    estimated_upstream_metrics_json = $4::jsonb,
		    updated_at = now()
		WHERE id = $1 AND status IN ('held', 'pending')
	`, reservationID, newReserved, marshalJSON(estimatedMetrics, []byte(`{}`)),
		marshalJSON(upstreamEstimatedMetrics, []byte(`{}`))); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE model_requests
		SET channel_id = $2, model_id = $3, price_version_id = NULLIF($4, '')::uuid,
		    official_price_version_id = NULLIF($5, '')::uuid,
		    estimated_amount = $6::numeric, price_snapshot_json = $7::jsonb,
		    upstream_cost_discount = $8::numeric,
		    estimated_upstream_cost = $9::numeric,
		    upstream_price_snapshot_json = $10::jsonb
		WHERE id = $1 AND status IN ('started', 'settlement_pending')
	`, modelRequestID, channelID, targetModelID, price.PriceVersionID, officialPriceVersionID(price), newReserved, priceSnapshotJSON, targetCostDiscount, estimatedUpstreamCost, upstreamPriceSnapshotRaw); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLService) ExtendReservation(ctx context.Context, reservationID string, extension time.Duration) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	reservationID = strings.TrimSpace(reservationID)
	if !ids.Valid(reservationID) || extension <= 0 || extension > 24*time.Hour {
		return ErrInvalidRequest
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE billing_reservations
		SET expires_at = GREATEST(expires_at, now() + ($2 * interval '1 second')),
		    updated_at = now()
		WHERE id = $1 AND status IN ('held', 'pending')
	`, reservationID, int64(extension/time.Second))
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected != 1 {
		return ErrReservationClosed
	}
	return nil
}

func (s *SQLService) Fail(ctx context.Context, reservationID, reason string) error {
	return s.releaseReservation(ctx, reservationID, reason, "released")
}

func (s *SQLService) releaseReservation(
	ctx context.Context,
	reservationID string,
	reason string,
	targetStatus string,
) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	reservationID = strings.TrimSpace(reservationID)
	reason = strings.TrimSpace(reason)
	if !ids.Valid(reservationID) || len(reason) > 1024 ||
		(targetStatus != "released" && targetStatus != "expired") {
		return ErrInvalidRequest
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var modelRequestID, accountID, reservedAmount, currentStatus string
	err = tx.QueryRowContext(ctx, `
		SELECT br.request_id::text, br.account_id::text, br.reserved_amount::text, br.status
		FROM billing_reservations br
		WHERE br.id = $1
		FOR UPDATE
	`, reservationID).Scan(&modelRequestID, &accountID, &reservedAmount, &currentStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrReservationNotFound
	}
	if err != nil {
		return err
	}
	if currentStatus == "released" || currentStatus == "expired" || currentStatus == "settled" {
		return nil
	}
	if currentStatus != "held" {
		return ErrReservationClosed
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE ledger_accounts
		SET balance = balance + $2::numeric
		WHERE id = $1
	`, accountID, reservedAmount)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected != 1 {
		return ErrAccountNotFound
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE billing_reservations
		SET released_amount = reserved_amount,
		    status = $2,
		    updated_at = now()
		WHERE id = $1 AND status = 'held'
	`, reservationID, targetStatus); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE model_requests
		SET status = 'failed',
		    failure_reason = NULLIF($2, ''),
		    finished_at = now()
		WHERE id = $1
	`, modelRequestID, reason); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLService) ExpireReservations(ctx context.Context) (int, error) {
	if s == nil || s.db == nil {
		return 0, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id::text
		FROM billing_reservations
		WHERE status = 'held'
		  AND expires_at <= now()
		ORDER BY expires_at
		LIMIT 100
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	idsToRelease := make([]string, 0, 100)
	for rows.Next() {
		var reservationID string
		if err := rows.Scan(&reservationID); err != nil {
			return 0, err
		}
		idsToRelease = append(idsToRelease, reservationID)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	released := 0
	for _, reservationID := range idsToRelease {
		if err := s.releaseReservation(ctx, reservationID, "reservation_expired", "expired"); err != nil {
			if errors.Is(err, ErrReservationClosed) || errors.Is(err, ErrReservationNotFound) {
				continue
			}
			return released, err
		}
		released++
	}
	return released, nil
}

func (s *SQLService) RunReaper(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := s.ExpireReservations(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("billing reservation reaper: %v", err)
			}
		}
	}
}

func validateRequest(request Request) error {
	if strings.TrimSpace(request.RequestID) == "" ||
		len(request.RequestID) > 256 ||
		strings.TrimSpace(request.IdempotencyKey) == "" ||
		len(request.IdempotencyKey) > 256 ||
		!ids.Valid(request.TenantID) ||
		!ids.Valid(request.ProjectID) ||
		!ids.Valid(request.TokenID) ||
		strings.TrimSpace(request.Model) == "" ||
		len(request.Model) > 256 ||
		strings.TrimSpace(request.Provider) == "" ||
		!ids.Valid(request.ChannelID) ||
		len(request.Endpoint) > 512 ||
		len(request.ClientIP) > 128 ||
		len(request.RequestType) > 64 ||
		len(request.ReasoningEffort) > 64 ||
		len(request.BillingType) > 32 ||
		len(request.MeteringMode) > 32 ||
		request.EstimatedInputTokens < 0 ||
		request.EstimatedOutputTokens < 0 {
		return ErrInvalidRequest
	}
	return nil
}

func tenantCurrency(ctx context.Context, tx *sql.Tx, tenantID string) (string, error) {
	var currency string
	err := tx.QueryRowContext(ctx, `
		SELECT currency
		FROM tenants
		WHERE id = $1
		  AND status = 'active'
		  AND deleted_at IS NULL
	`, tenantID).Scan(&currency)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrAccountNotFound
	}
	if err != nil {
		return "", err
	}
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" {
		return "", ErrAccountNotFound
	}
	return currency, nil
}

func modelID(ctx context.Context, tx *sql.Tx, provider, model string) (string, error) {
	var id string
	err := tx.QueryRowContext(ctx, `
		SELECT id::text
		FROM models
		WHERE provider = $1
		  AND model_name = $2
		  AND status = 'active'
	`, strings.ToLower(strings.TrimSpace(provider)), strings.TrimSpace(model)).Scan(&id)
	return id, err
}

func priceScopeExists(ctx context.Context, tx *sql.Tx, scopeType, scopeID string) (bool, error) {
	var exists bool
	var query string
	switch scopeType {
	case "tenant":
		query = `
			SELECT EXISTS (
				SELECT 1 FROM tenants
				WHERE id = $1::uuid AND deleted_at IS NULL
			)
		`
	case "project":
		query = `
			SELECT EXISTS (
				SELECT 1 FROM projects
				WHERE id = $1::uuid AND deleted_at IS NULL
			)
		`
	case "token":
		query = `
			SELECT EXISTS (
				SELECT 1 FROM api_tokens
				WHERE id = $1::uuid
			)
		`
	default:
		return false, ErrInvalidPrice
	}
	if err := tx.QueryRowContext(ctx, query, scopeID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func resolveOfficialPrice(
	ctx context.Context,
	tx *sql.Tx,
	modelID string,
	currency string,
	now time.Time,
) (Price, error) {
	var price Price
	err := tx.QueryRowContext(ctx, `
		SELECT id::text, currency, input_price_per_unit::text,
		       output_price_per_unit::text,
		       cached_input_price_per_unit::text,
		       reasoning_price_per_unit::text,
		       '0'::text
		FROM official_model_price_versions
		WHERE model_id = $1
		  AND source = 'litellm'
		  AND effective_from <= $2
		  AND (effective_to IS NULL OR effective_to > $2)
		ORDER BY effective_from DESC
		LIMIT 1
	`, modelID, now).Scan(
		&price.ID, &price.Currency, &price.InputPricePerUnit,
		&price.OutputPricePerUnit, &price.CachedInputPricePerUnit,
		&price.ReasoningPricePerUnit, &price.MinimumCharge,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Price{}, ErrPriceNotConfigured
	}
	if err != nil {
		return Price{}, err
	}
	price.ModelID = modelID
	price.Source = "litellm"
	price.Currency = strings.ToUpper(strings.TrimSpace(price.Currency))
	price.Components, err = loadOfficialPriceComponents(ctx, tx, price.ID)
	if err != nil {
		return Price{}, err
	}
	if price.Currency != strings.ToUpper(strings.TrimSpace(currency)) {
		return Price{}, ErrPriceNotConfigured
	}
	return price, nil
}

func resolveUpstreamPrice(
	ctx context.Context,
	tx *sql.Tx,
	modelID string,
	currency string,
	now time.Time,
	customerPrice Price,
) (Price, string, error) {
	officialPrice, err := resolveOfficialPrice(ctx, tx, modelID, currency, now)
	if err == nil {
		return officialPrice, "official_reference", nil
	}
	if !errors.Is(err, ErrPriceNotConfigured) {
		return Price{}, "", err
	}
	// An official reference price is preferred, but a manually configured
	// customer price remains a useful estimate until the official catalog has
	// been synchronized for this model.
	return customerPrice, "customer_price_fallback", nil
}

func resolvePrice(
	ctx context.Context,
	tx *sql.Tx,
	request Request,
	modelID string,
	currency string,
	now time.Time,
) (Price, error) {
	candidates := []struct {
		scope string
		id    string
	}{
		{"token", strings.TrimSpace(request.TokenID)},
		{"project", strings.TrimSpace(request.ProjectID)},
		{"tenant", strings.TrimSpace(request.TenantID)},
		{"platform_default", ""},
	}

	for _, candidate := range candidates {
		var price Price
		var err error
		if candidate.scope == "platform_default" {
			err = tx.QueryRowContext(ctx, `
			SELECT id::text, currency, input_price_per_unit::text,
				       output_price_per_unit::text,
				       cached_input_price_per_unit::text,
				       reasoning_price_per_unit::text,
				       minimum_charge::text
				FROM price_versions
				WHERE model_id = $1
				  AND scope_type = 'platform_default'
				  AND scope_id IS NULL
				  AND status = 'active'
				  AND effective_from <= $2
				  AND (effective_to IS NULL OR effective_to > $2)
				ORDER BY version DESC, effective_from DESC
				LIMIT 1
			`, modelID, now).Scan(
				&price.ID, &price.Currency, &price.InputPricePerUnit,
				&price.OutputPricePerUnit, &price.CachedInputPricePerUnit,
				&price.ReasoningPricePerUnit, &price.MinimumCharge,
			)
		} else {
			err = tx.QueryRowContext(ctx, `
				SELECT id::text, currency, input_price_per_unit::text,
				       output_price_per_unit::text,
				       cached_input_price_per_unit::text,
				       reasoning_price_per_unit::text,
				       minimum_charge::text
				FROM price_versions
				WHERE model_id = $1
				  AND scope_type = $2
				  AND scope_id = $3
				  AND status = 'active'
				  AND effective_from <= $4
				  AND (effective_to IS NULL OR effective_to > $4)
				ORDER BY version DESC, effective_from DESC
				LIMIT 1
			`, modelID, candidate.scope, candidate.id, now).Scan(
				&price.ID, &price.Currency, &price.InputPricePerUnit,
				&price.OutputPricePerUnit, &price.CachedInputPricePerUnit,
				&price.ReasoningPricePerUnit, &price.MinimumCharge,
			)
		}
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return Price{}, err
		}
		price.ModelID = modelID
		price.PriceVersionID = price.ID
		price.Source = "manual"
		price.Currency = strings.ToUpper(strings.TrimSpace(price.Currency))
		price.Components, err = loadPriceComponents(ctx, tx, price.ID)
		if err != nil {
			return Price{}, err
		}
		if price.Currency != currency {
			continue
		}
		return price, nil
	}

	// The public model catalog intentionally falls back to the latest synced
	// LiteLLM price when no manual platform price exists. Use the same source
	// for runtime billing so a model shown as priced in the catalog cannot fail
	// at reservation time with PRICE_NOT_CONFIGURED.
	return resolveOfficialPrice(ctx, tx, modelID, currency, now)
}

func calculateAmount(
	ctx context.Context,
	tx *sql.Tx,
	inputPrice string,
	inputTokens int64,
	cachedInputPrice string,
	cachedInputTokens int64,
	outputPrice string,
	outputTokens int64,
	reasoningPrice string,
	reasoningTokens int64,
	minimumCharge string,
) (string, error) {
	var amount string
	err := tx.QueryRowContext(ctx, `
		SELECT GREATEST(
			$1::numeric * GREATEST($2::bigint - $4::bigint, 0)
				+ CASE
					WHEN $3::numeric > 0 THEN $3::numeric
					ELSE $1::numeric
				  END * $4::bigint
				+ $5::numeric * GREATEST($6::bigint - $8::bigint, 0)
				+ CASE
					WHEN $7::numeric > 0 THEN $7::numeric
					ELSE $5::numeric
				  END * $8::bigint,
			$9::numeric
		)::text
	`, inputPrice, inputTokens, cachedInputPrice, cachedInputTokens,
		outputPrice, outputTokens, reasoningPrice, reasoningTokens,
		minimumCharge).Scan(&amount)
	return amount, err
}

func normalizeUsage(usage Usage) Usage {
	if strings.TrimSpace(usage.Source) == "" {
		usage.Source = "upstream"
	}
	// Keep the legacy columns and the extensible meters in one normalized view.
	// Adapters may provide either representation; a non-zero explicit meter
	// wins, while a missing/zero meter is completed from the legacy field.
	merged := make(MeteredUsage, len(usage.Metrics)+4)
	for code, value := range usage.Metrics {
		merged[code] = value
	}
	for _, item := range []struct {
		code  string
		value int64
	}{
		{"input_tokens", usage.InputTokens},
		{"output_tokens", usage.OutputTokens},
		{"cached_input_tokens", usage.CachedInputTokens},
		{"reasoning_tokens", usage.ReasoningTokens},
	} {
		if item.value <= 0 {
			continue
		}
		current, exists := merged[item.code]
		if !exists || isZeroAmount(current) {
			merged[item.code] = integerString(item.value)
		}
	}
	if normalized, err := normalizeMeteredUsage(merged); err == nil && len(normalized) > 0 {
		usage.Metrics = normalized
		if value := usageMetricInt(normalized, "input_tokens"); value > 0 {
			usage.InputTokens = value
		}
		if value := usageMetricInt(normalized, "output_tokens"); value > 0 {
			usage.OutputTokens = value
		}
		if value := usageMetricInt(normalized, "cached_input_tokens"); value > 0 {
			usage.CachedInputTokens = value
		}
		if value := usageMetricInt(normalized, "reasoning_tokens"); value > 0 {
			usage.ReasoningTokens = value
		}
	}
	if usage.CachedInputTokens > usage.InputTokens {
		usage.CachedInputTokens = usage.InputTokens
	}
	if usage.ReasoningTokens > usage.OutputTokens {
		usage.ReasoningTokens = usage.OutputTokens
	}
	return usage
}

func usageMetricInt(metrics MeteredUsage, code string) int64 {
	value, ok := new(big.Rat).SetString(strings.TrimSpace(metrics[code]))
	if !ok || value.Sign() < 0 || value.Denom().Cmp(big.NewInt(1)) != 0 || !value.Num().IsInt64() {
		return 0
	}
	return value.Num().Int64()
}

func validUsageSource(source string) bool {
	switch strings.TrimSpace(source) {
	case "", "upstream", "local_estimate", "reconciliation":
		return true
	default:
		return false
	}
}

func calculateReservationAmount(
	ctx context.Context,
	tx *sql.Tx,
	inputPrice string,
	inputTokens int64,
	cachedInputPrice string,
	outputPrice string,
	outputTokens int64,
	reasoningPrice string,
	minimumCharge string,
) (string, error) {
	var amount string
	err := tx.QueryRowContext(ctx, `
		SELECT GREATEST(
			GREATEST($1::numeric, $2::numeric) * $3::bigint
				+ GREATEST($4::numeric, $5::numeric) * $6::bigint,
			$7::numeric
		)::text
	`, inputPrice, cachedInputPrice, inputTokens,
		outputPrice, reasoningPrice, outputTokens, minimumCharge).Scan(&amount)
	return amount, err
}

func multiplyAmount(ctx context.Context, tx *sql.Tx, amount, multiplier string) (string, error) {
	if strings.TrimSpace(multiplier) == "" {
		multiplier = "1"
	}
	var result string
	err := tx.QueryRowContext(ctx, `SELECT ($1::numeric * $2::numeric)::text`, amount, multiplier).Scan(&result)
	return result, err
}

func ensureSystemAccount(ctx context.Context, tx *sql.Tx, currency string) (string, error) {
	return ensureSystemAccountKind(ctx, tx, currency, "revenue", "receivable")
}

func ensureSystemAccountKind(
	ctx context.Context,
	tx *sql.Tx,
	currency string,
	purpose string,
	accountType string,
) (string, error) {
	accountID, err := ids.New()
	if err != nil {
		return "", err
	}
	accountCode := "system:" + strings.TrimSpace(purpose) + ":" + strings.ToUpper(strings.TrimSpace(currency))
	err = tx.QueryRowContext(ctx, `
		INSERT INTO ledger_accounts (
			id, tenant_id, account_type, currency, status, account_code, is_system
		) VALUES ($1, NULL, $2, $3, 'active', $4, true)
		ON CONFLICT (account_code) WHERE account_code IS NOT NULL
		DO UPDATE SET status = 'active'
		RETURNING id::text
	`, accountID, accountType, currency, accountCode).Scan(&accountID)
	return accountID, err
}

func isZeroAmount(value string) bool {
	rational, ok := new(big.Rat).SetString(strings.TrimSpace(value))
	return ok && rational.Sign() == 0
}

func pricePerMillionTokens(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	amount, ok := new(big.Rat).SetString(value)
	if !ok {
		return ""
	}
	amount.Mul(amount, big.NewRat(1_000_000, 1))
	result := strings.TrimRight(strings.TrimRight(amount.FloatString(30), "0"), ".")
	if result == "" {
		return "0"
	}
	return result
}

func validNonNegativeDecimal(value string) bool {
	_, ok := canonicalDecimal(value, 30, 30)
	return ok
}

func zeroPrice(value string) string {
	if strings.TrimSpace(value) == "" {
		return "0"
	}
	return strings.TrimSpace(value)
}

func hasPricedComponent(components []PriceComponentInput) bool {
	for _, component := range components {
		if !isZeroAmount(component.PricePerUnit) {
			return true
		}
	}
	return false
}

func mergePriceComponentInputs(base, overrides []PriceComponentInput) []PriceComponentInput {
	result := make([]PriceComponentInput, 0, len(base)+len(overrides))
	indexByCode := make(map[string]int, len(base)+len(overrides))
	for _, component := range append(append([]PriceComponentInput{}, base...), overrides...) {
		code := strings.TrimSpace(component.ComponentCode)
		if code == "" {
			continue
		}
		if index, exists := indexByCode[code]; exists {
			result[index] = component
			continue
		}
		indexByCode[code] = len(result)
		result = append(result, component)
	}
	return result
}

func validPositiveDecimal(value string) bool {
	return validNonNegativeDecimal(value) && !isZeroAmount(value)
}

func normalizeGroupMultiplier(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "1.000000", nil
	}
	if !validNonNegativeDecimal(value) || isZeroAmount(value) {
		return "", ErrInvalidRequest
	}
	parts := strings.Split(value, ".")
	if len(parts) == 2 && len(parts[1]) > 6 {
		return "", ErrInvalidRequest
	}
	rational, ok := new(big.Rat).SetString(value)
	if !ok || rational.Cmp(new(big.Rat).SetInt64(1000)) > 0 {
		return "", ErrInvalidRequest
	}
	return rational.FloatString(6), nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
