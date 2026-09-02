package billing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ai-token/internal/ids"
)

type UsageRecord struct {
	ID              string         `json:"id"`
	RequestID       string         `json:"request_id"`
	TokenID         string         `json:"token_id"`
	TokenName       string         `json:"token_name"`
	TokenPrefix     string         `json:"token_prefix"`
	TenantID        string         `json:"tenant_id"`
	TenantName      string         `json:"tenant_name"`
	ModelID         string         `json:"model_id"`
	Provider        string         `json:"provider"`
	Model           string         `json:"model"`
	ReasoningEffort string         `json:"reasoning_effort"`
	Endpoint        string         `json:"endpoint"`
	ClientIP        string         `json:"client_ip"`
	GroupID         string         `json:"group_id"`
	GroupCode       string         `json:"group_code"`
	GroupName       string         `json:"group_name"`
	RequestType     string         `json:"request_type"`
	BillingType     string         `json:"billing_type"`
	Status          string         `json:"status"`
	FailureReason   string         `json:"failure_reason,omitempty"`
	InputTokens     int64          `json:"input_tokens"`
	OutputTokens    int64          `json:"output_tokens"`
	CachedInput     int64          `json:"cached_input_tokens"`
	ReasoningTokens int64          `json:"reasoning_tokens"`
	TotalTokens     int64          `json:"total_tokens"`
	Cost            string         `json:"cost"`
	EstimatedCost   string         `json:"estimated_cost"`
	Currency        string         `json:"currency"`
	LatencyMS       int64          `json:"latency_ms"`
	StartedAt       time.Time      `json:"started_at"`
	FinishedAt      *time.Time     `json:"finished_at,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UsageMetrics    MeteredUsage   `json:"usage_metrics,omitempty"`
	ChargeBreakdown []ChargeLine   `json:"charge_breakdown,omitempty"`
	PriceSnapshot   map[string]any `json:"price_snapshot,omitempty"`
}

type UsageQuery struct {
	Limit      int
	Offset     int
	TenantID   string
	ProjectIDs []string
	Model      string
	GroupID    string
	Status     string
	Search     string
	From       *time.Time
	To         *time.Time
}

type UsageSummary struct {
	TotalRecords     int64        `json:"total_records"`
	InputTokens      int64        `json:"input_tokens"`
	OutputTokens     int64        `json:"output_tokens"`
	CachedInputToken int64        `json:"cached_input_tokens"`
	ReasoningTokens  int64        `json:"reasoning_tokens"`
	TotalTokens      int64        `json:"total_tokens"`
	TotalCost        string       `json:"total_cost"`
	UsageMetrics     MeteredUsage `json:"usage_metrics,omitempty"`
}

type UsageReport struct {
	Records []UsageRecord `json:"records"`
	Summary UsageSummary  `json:"summary"`
	Limit   int           `json:"limit"`
	Offset  int           `json:"offset"`
}

type FinanceCurrencySummary struct {
	Currency         string `json:"currency"`
	CustomerCount    int64  `json:"customer_count"`
	RemainingBalance string `json:"remaining_balance"`
	TotalConsumed    string `json:"total_consumed"`
	TotalTopups      string `json:"total_topups"`
	RequestCount     int64  `json:"request_count"`
}

type FinanceAccount struct {
	TenantID      string     `json:"tenant_id"`
	TenantName    string     `json:"tenant_name"`
	TenantSlug    string     `json:"tenant_slug"`
	Currency      string     `json:"currency"`
	Balance       string     `json:"balance"`
	TotalConsumed string     `json:"total_consumed"`
	TotalTopups   string     `json:"total_topups"`
	RequestCount  int64      `json:"request_count"`
	LastUsageAt   *time.Time `json:"last_usage_at,omitempty"`
}

type FinanceTransaction struct {
	ID              string         `json:"id"`
	Transaction     string         `json:"transaction_type"`
	Direction       string         `json:"direction"`
	Amount          string         `json:"amount"`
	Currency        string         `json:"currency"`
	TenantID        string         `json:"tenant_id"`
	TenantName      string         `json:"tenant_name"`
	Reference       string         `json:"reference_type"`
	ReferenceID     string         `json:"reference_id"`
	Model           string         `json:"model"`
	TokenName       string         `json:"token_name"`
	Description     string         `json:"description"`
	CreatedAt       time.Time      `json:"created_at"`
	UsageMetrics    MeteredUsage   `json:"usage_metrics,omitempty"`
	ChargeBreakdown []ChargeLine   `json:"charge_breakdown,omitempty"`
	PriceSnapshot   map[string]any `json:"price_snapshot,omitempty"`
}

type FinanceQuery struct {
	Limit    int
	Offset   int
	TenantID string
	Currency string
	Search   string
	From     *time.Time
	To       *time.Time
}

type FinanceReport struct {
	Summaries         []FinanceCurrencySummary `json:"summaries"`
	Accounts          []FinanceAccount         `json:"accounts"`
	Transactions      []FinanceTransaction     `json:"transactions"`
	TotalAccounts     int64                    `json:"total_accounts"`
	TotalTransactions int64                    `json:"total_transactions"`
	Limit             int                      `json:"limit"`
	Offset            int                      `json:"offset"`
}

type UsageReporter interface {
	ListUsageRecords(context.Context, UsageQuery) (UsageReport, error)
}

type FinanceReporter interface {
	ListFinanceReport(context.Context, FinanceQuery) (FinanceReport, error)
}

func (s *SQLService) ListUsageRecords(ctx context.Context, query UsageQuery) (UsageReport, error) {
	if s == nil || s.db == nil {
		return UsageReport{}, ErrUnavailable
	}
	query = normalizeUsageQuery(query)
	where, args := usageWhere(query)
	var summary UsageSummary
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)::bigint,
		       COALESCE(SUM(mr.input_tokens), 0)::bigint,
		       COALESCE(SUM(mr.output_tokens), 0)::bigint,
		       COALESCE(SUM(mr.cached_input_tokens), 0)::bigint,
		       COALESCE(SUM(mr.reasoning_tokens), 0)::bigint,
		       COALESCE(SUM(mr.input_tokens + mr.output_tokens), 0)::bigint,
		       COALESCE(SUM(mr.settled_amount), 0)::text
		FROM model_requests mr
		JOIN api_tokens tok ON tok.id = mr.token_id
		JOIN tenants ten ON ten.id = mr.tenant_id
		JOIN models mod ON mod.id = mr.model_id
		LEFT JOIN routing_groups grp ON grp.id = mr.group_id
		WHERE `+where, args...).Scan(
		&summary.TotalRecords,
		&summary.InputTokens,
		&summary.OutputTokens,
		&summary.CachedInputToken,
		&summary.ReasoningTokens,
		&summary.TotalTokens,
		&summary.TotalCost,
	); err != nil {
		return UsageReport{}, err
	}
	summary.TotalCost = normalizeDecimalText(summary.TotalCost)
	var summaryMetricsRaw []byte
	if err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(jsonb_object_agg(metric, total), '{}'::jsonb)
		FROM (
			SELECT metric, SUM(quantity::numeric)::text AS total
			FROM model_requests mr
			CROSS JOIN LATERAL jsonb_each_text(mr.usage_metrics_json) AS usage(metric, quantity)
			JOIN api_tokens tok ON tok.id = mr.token_id
			JOIN tenants ten ON ten.id = mr.tenant_id
			JOIN models mod ON mod.id = mr.model_id
			LEFT JOIN routing_groups grp ON grp.id = mr.group_id
			WHERE `+where+`
			GROUP BY metric
		) metrics
	`, args...).Scan(&summaryMetricsRaw); err != nil {
		return UsageReport{}, err
	}
	if len(summaryMetricsRaw) > 0 && string(summaryMetricsRaw) != "null" {
		_ = json.Unmarshal(summaryMetricsRaw, &summary.UsageMetrics)
	}

	listArgs := append([]any{}, args...)
	limitPosition := len(listArgs) + 1
	offsetPosition := len(listArgs) + 2
	listArgs = append(listArgs, query.Limit, query.Offset)
	rows, err := s.db.QueryContext(ctx, `
		SELECT mr.id::text, mr.request_id, mr.token_id::text, tok.name, tok.token_prefix,
		       mr.tenant_id::text, ten.name, mr.model_id::text, mod.provider, mod.model_name,
		       mr.reasoning_effort, mr.endpoint, mr.client_ip, COALESCE(mr.group_id::text, ''),
		       COALESCE(grp.code, ''), COALESCE(grp.name, ''), mr.request_type,
		       COALESCE(grp.billing_type, CASE WHEN COALESCE(mr.price_version_id, mr.official_price_version_id) IS NULL THEN 'free' ELSE 'prepaid' END),
		       mr.status, COALESCE(mr.failure_reason, ''), mr.input_tokens, mr.output_tokens, mr.cached_input_tokens,
			   mr.reasoning_tokens, mr.settled_amount::text, mr.estimated_amount::text,
			   mr.currency, mr.latency_ms, mr.started_at, mr.finished_at, mr.created_at,
			   mr.usage_metrics_json, mr.charge_breakdown_json, mr.price_snapshot_json
		FROM model_requests mr
		JOIN api_tokens tok ON tok.id = mr.token_id
		JOIN tenants ten ON ten.id = mr.tenant_id
		JOIN models mod ON mod.id = mr.model_id
		LEFT JOIN routing_groups grp ON grp.id = mr.group_id
		WHERE `+where+fmt.Sprintf(`
		ORDER BY mr.created_at DESC, mr.id DESC
		LIMIT $%d OFFSET $%d`, limitPosition, offsetPosition), listArgs...)
	if err != nil {
		return UsageReport{}, err
	}
	defer rows.Close()
	records := make([]UsageRecord, 0, query.Limit)
	for rows.Next() {
		var record UsageRecord
		var finishedAt sql.NullTime
		var usageMetricsRaw, chargeBreakdownRaw, priceSnapshotRaw []byte
		if err := rows.Scan(
			&record.ID, &record.RequestID, &record.TokenID, &record.TokenName, &record.TokenPrefix,
			&record.TenantID, &record.TenantName, &record.ModelID, &record.Provider, &record.Model,
			&record.ReasoningEffort, &record.Endpoint, &record.ClientIP, &record.GroupID,
			&record.GroupCode, &record.GroupName, &record.RequestType, &record.BillingType,
			&record.Status, &record.FailureReason, &record.InputTokens, &record.OutputTokens, &record.CachedInput,
			&record.ReasoningTokens, &record.Cost, &record.EstimatedCost, &record.Currency,
			&record.LatencyMS, &record.StartedAt, &finishedAt, &record.CreatedAt,
			&usageMetricsRaw, &chargeBreakdownRaw, &priceSnapshotRaw,
		); err != nil {
			return UsageReport{}, err
		}
		if len(usageMetricsRaw) > 0 && string(usageMetricsRaw) != "null" {
			_ = json.Unmarshal(usageMetricsRaw, &record.UsageMetrics)
		}
		if len(chargeBreakdownRaw) > 0 && string(chargeBreakdownRaw) != "null" {
			_ = json.Unmarshal(chargeBreakdownRaw, &record.ChargeBreakdown)
		}
		if len(priceSnapshotRaw) > 0 && string(priceSnapshotRaw) != "null" {
			_ = json.Unmarshal(priceSnapshotRaw, &record.PriceSnapshot)
		}
		record.TotalTokens = record.InputTokens + record.OutputTokens
		record.Provider = strings.ToLower(strings.TrimSpace(record.Provider))
		record.Currency = strings.ToUpper(strings.TrimSpace(record.Currency))
		record.Cost = normalizeDecimalText(record.Cost)
		record.EstimatedCost = normalizeDecimalText(record.EstimatedCost)
		if finishedAt.Valid {
			value := finishedAt.Time
			record.FinishedAt = &value
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return UsageReport{}, err
	}
	return UsageReport{Records: records, Summary: summary, Limit: query.Limit, Offset: query.Offset}, nil
}

// PostgreSQL numeric columns retain the configured scale when converted to
// text. Reports should keep the exact decimal value without rendering padding
// zeros that have no meaning to a customer.
func normalizeDecimalText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "0"
	}

	sign := ""
	if value[0] == '-' || value[0] == '+' {
		sign, value = value[:1], value[1:]
	}
	parts := strings.SplitN(value, ".", 2)
	integerPart := strings.TrimLeft(parts[0], "0")
	if integerPart == "" {
		integerPart = "0"
	}
	fractionPart := ""
	if len(parts) == 2 {
		fractionPart = strings.TrimRight(parts[1], "0")
	}
	if integerPart == "0" && fractionPart == "" {
		return "0"
	}
	if fractionPart == "" {
		return sign + integerPart
	}
	return sign + integerPart + "." + fractionPart
}

func (s *SQLService) ListFinanceReport(ctx context.Context, query FinanceQuery) (FinanceReport, error) {
	if s == nil || s.db == nil {
		return FinanceReport{}, ErrUnavailable
	}
	query = normalizeFinanceQuery(query)
	where, args := financeAccountWhere(query)
	summaryArgs := append([]any{}, args...)
	usageTime, topupTime := financeTimeClauses(&summaryArgs, query)
	rows, err := s.db.QueryContext(ctx, `
		SELECT COALESCE(a.currency, ten.currency), COUNT(*)::bigint,
		       COALESCE(SUM(COALESCE(a.balance, 0)), 0)::text,
		       COALESCE(SUM(usage.total_consumed), 0)::text,
		       COALESCE(SUM(topup.total_topups), 0)::text,
		       COALESCE(SUM(usage.request_count), 0)::bigint
		FROM tenants ten
		LEFT JOIN ledger_accounts a ON a.tenant_id = ten.id
		  AND a.account_type = 'prepaid_balance' AND a.status = 'active'
		LEFT JOIN LATERAL (
			SELECT COALESCE(SUM(mr.settled_amount), 0)::numeric AS total_consumed,
			       COUNT(*)::bigint AS request_count
			FROM model_requests mr WHERE mr.tenant_id = ten.id AND mr.status = 'settled'`+usageTime+`
		) usage ON true
		LEFT JOIN LATERAL (
			SELECT COALESCE(SUM(ll.amount) FILTER (WHERE ll.direction = 'credit'), 0)::numeric AS total_topups
			FROM ledger_lines ll WHERE ll.account_id = a.id`+topupTime+`
		) topup ON true
		WHERE `+where+`
		GROUP BY COALESCE(a.currency, ten.currency)
		ORDER BY COALESCE(a.currency, ten.currency)`, summaryArgs...)
	if err != nil {
		return FinanceReport{}, err
	}
	summaries := make([]FinanceCurrencySummary, 0)
	for rows.Next() {
		var item FinanceCurrencySummary
		if err := rows.Scan(&item.Currency, &item.CustomerCount, &item.RemainingBalance, &item.TotalConsumed, &item.TotalTopups, &item.RequestCount); err != nil {
			rows.Close()
			return FinanceReport{}, err
		}
		item.Currency = strings.ToUpper(strings.TrimSpace(item.Currency))
		summaries = append(summaries, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return FinanceReport{}, err
	}
	rows.Close()

	accountCount, err := s.countFinanceAccounts(ctx, query)
	if err != nil {
		return FinanceReport{}, err
	}
	transactionWhere, transactionArgs := financeTransactionWhere(query)
	var transactionCount int64
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)::bigint
		FROM ledger_lines ll
		JOIN ledger_accounts a ON a.id = ll.account_id AND a.account_type = 'prepaid_balance' AND a.status = 'active'
		JOIN tenants ten ON ten.id = a.tenant_id
		JOIN ledger_transactions lt ON lt.id = ll.transaction_id
		LEFT JOIN model_requests mr ON lt.reference_type = 'model_request' AND mr.id = lt.reference_id
		LEFT JOIN models mod ON mod.id = mr.model_id
		WHERE `+transactionWhere, transactionArgs...).Scan(&transactionCount); err != nil {
		return FinanceReport{}, err
	}

	listArgs := append([]any{}, transactionArgs...)
	limitPosition := len(listArgs) + 1
	offsetPosition := len(listArgs) + 2
	listArgs = append(listArgs, query.Limit, query.Offset)
	transactionRows, err := s.db.QueryContext(ctx, `
		SELECT lt.id::text, lt.transaction_type, ll.direction, ll.amount::text, ll.currency,
		       ten.id::text, ten.name, lt.reference_type, lt.reference_id::text,
		       COALESCE(mod.model_name, ''), COALESCE(tok.name, ''),
			   COALESCE(ll.metadata_json ->> 'reason', CASE WHEN lt.transaction_type = 'model_usage' THEN 'model usage' ELSE 'account credit' END),
			   lt.created_at, mr.usage_metrics_json, mr.charge_breakdown_json, mr.price_snapshot_json
		FROM ledger_lines ll
		JOIN ledger_accounts a ON a.id = ll.account_id AND a.account_type = 'prepaid_balance' AND a.status = 'active'
		JOIN tenants ten ON ten.id = a.tenant_id
		JOIN ledger_transactions lt ON lt.id = ll.transaction_id
		LEFT JOIN model_requests mr ON lt.reference_type = 'model_request' AND mr.id = lt.reference_id
		LEFT JOIN models mod ON mod.id = mr.model_id
		LEFT JOIN api_tokens tok ON tok.id = mr.token_id
		WHERE `+transactionWhere+fmt.Sprintf(`
		ORDER BY lt.created_at DESC, lt.id DESC
		LIMIT $%d OFFSET $%d`, limitPosition, offsetPosition), listArgs...)
	if err != nil {
		return FinanceReport{}, err
	}
	defer transactionRows.Close()
	transactions := make([]FinanceTransaction, 0, query.Limit)
	for transactionRows.Next() {
		var item FinanceTransaction
		var usageMetricsRaw, chargeBreakdownRaw, priceSnapshotRaw []byte
		if err := transactionRows.Scan(&item.ID, &item.Transaction, &item.Direction, &item.Amount, &item.Currency, &item.TenantID, &item.TenantName, &item.Reference, &item.ReferenceID, &item.Model, &item.TokenName, &item.Description, &item.CreatedAt, &usageMetricsRaw, &chargeBreakdownRaw, &priceSnapshotRaw); err != nil {
			return FinanceReport{}, err
		}
		if len(usageMetricsRaw) > 0 && string(usageMetricsRaw) != "null" {
			_ = json.Unmarshal(usageMetricsRaw, &item.UsageMetrics)
		}
		if len(chargeBreakdownRaw) > 0 && string(chargeBreakdownRaw) != "null" {
			_ = json.Unmarshal(chargeBreakdownRaw, &item.ChargeBreakdown)
		}
		if len(priceSnapshotRaw) > 0 && string(priceSnapshotRaw) != "null" {
			_ = json.Unmarshal(priceSnapshotRaw, &item.PriceSnapshot)
		}
		item.Currency = strings.ToUpper(strings.TrimSpace(item.Currency))
		transactions = append(transactions, item)
	}
	if err := transactionRows.Err(); err != nil {
		return FinanceReport{}, err
	}
	accounts, err := s.financeAccounts(ctx, query)
	if err != nil {
		return FinanceReport{}, err
	}
	return FinanceReport{Summaries: summaries, Accounts: accounts, Transactions: transactions, TotalAccounts: accountCount, TotalTransactions: transactionCount, Limit: query.Limit, Offset: query.Offset}, nil
}

func (s *SQLService) countFinanceAccounts(ctx context.Context, query FinanceQuery) (int64, error) {
	where, args := financeAccountWhere(query)
	var count int64
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)::bigint FROM tenants ten
		LEFT JOIN ledger_accounts a ON a.tenant_id = ten.id AND a.account_type = 'prepaid_balance' AND a.status = 'active'
		WHERE `+where, args...).Scan(&count)
	return count, err
}

func (s *SQLService) financeAccounts(ctx context.Context, query FinanceQuery) ([]FinanceAccount, error) {
	where, args := financeAccountWhere(query)
	listArgs := append([]any{}, args...)
	usageTime, topupTime := financeTimeClauses(&listArgs, query)
	listArgs = append(listArgs, query.Limit, query.Offset)
	limitPosition := len(listArgs) - 1
	offsetPosition := len(listArgs)
	rows, err := s.db.QueryContext(ctx, `
		SELECT ten.id::text, ten.name, ten.slug, COALESCE(a.currency, ten.currency),
		       COALESCE(a.balance, 0)::text, COALESCE(usage.total_consumed, 0)::text,
		       COALESCE(topup.total_topups, 0)::text, COALESCE(usage.request_count, 0), usage.last_usage_at
		FROM tenants ten
		LEFT JOIN ledger_accounts a ON a.tenant_id = ten.id AND a.account_type = 'prepaid_balance' AND a.status = 'active'
		LEFT JOIN LATERAL (
			SELECT COALESCE(SUM(mr.settled_amount), 0)::numeric AS total_consumed,
			       COUNT(*)::bigint AS request_count,
			       MAX(COALESCE(mr.finished_at, mr.created_at)) AS last_usage_at
			FROM model_requests mr WHERE mr.tenant_id = ten.id AND mr.status = 'settled'`+usageTime+`
		) usage ON true
		LEFT JOIN LATERAL (
			SELECT COALESCE(SUM(ll.amount) FILTER (WHERE ll.direction = 'credit'), 0)::numeric AS total_topups
			FROM ledger_lines ll WHERE ll.account_id = a.id`+topupTime+`
		) topup ON true
		WHERE `+where+fmt.Sprintf(`
		ORDER BY ten.name ASC, ten.id ASC
		LIMIT $%d OFFSET $%d`, limitPosition, offsetPosition), listArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	accounts := make([]FinanceAccount, 0, query.Limit)
	for rows.Next() {
		var item FinanceAccount
		var lastUsage sql.NullTime
		if err := rows.Scan(&item.TenantID, &item.TenantName, &item.TenantSlug, &item.Currency, &item.Balance, &item.TotalConsumed, &item.TotalTopups, &item.RequestCount, &lastUsage); err != nil {
			return nil, err
		}
		item.Currency = strings.ToUpper(strings.TrimSpace(item.Currency))
		if lastUsage.Valid {
			value := lastUsage.Time
			item.LastUsageAt = &value
		}
		accounts = append(accounts, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return accounts, nil
}

func financeTimeClauses(args *[]any, query FinanceQuery) (string, string) {
	clauses := make([]string, 0, 2)
	if query.From != nil {
		*args = append(*args, *query.From)
		clauses = append(clauses, fmt.Sprintf(" >= $%d", len(*args)))
	}
	if query.To != nil {
		*args = append(*args, *query.To)
		clauses = append(clauses, fmt.Sprintf(" < $%d", len(*args)))
	}
	if len(clauses) == 0 {
		return "", ""
	}
	var usage, topup strings.Builder
	for _, clause := range clauses {
		usage.WriteString(" AND mr.created_at")
		usage.WriteString(clause)
		topup.WriteString(" AND ll.created_at")
		topup.WriteString(clause)
	}
	return usage.String(), topup.String()
}

func normalizeUsageQuery(query UsageQuery) UsageQuery {
	if query.Limit <= 0 || query.Limit > 200 {
		query.Limit = 50
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	query.TenantID = strings.TrimSpace(query.TenantID)
	projectIDs := make([]string, 0, len(query.ProjectIDs))
	for _, projectID := range query.ProjectIDs {
		projectID = strings.TrimSpace(projectID)
		if projectID != "" {
			projectIDs = append(projectIDs, projectID)
		}
	}
	query.ProjectIDs = projectIDs
	query.Model = strings.TrimSpace(query.Model)
	query.GroupID = strings.TrimSpace(query.GroupID)
	query.Status = strings.ToLower(strings.TrimSpace(query.Status))
	query.Search = strings.TrimSpace(query.Search)
	return query
}

func normalizeFinanceQuery(query FinanceQuery) FinanceQuery {
	if query.Limit <= 0 || query.Limit > 200 {
		query.Limit = 50
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	query.TenantID = strings.TrimSpace(query.TenantID)
	query.Currency = strings.ToUpper(strings.TrimSpace(query.Currency))
	query.Search = strings.TrimSpace(query.Search)
	return query
}

func usageWhere(query UsageQuery) (string, []any) {
	clauses := []string{"1 = 1"}
	args := make([]any, 0)
	add := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}
	if query.TenantID != "" {
		add("mr.tenant_id = $%d::uuid", query.TenantID)
	}
	if len(query.ProjectIDs) > 0 {
		projectClauses := make([]string, 0, len(query.ProjectIDs))
		for _, projectID := range query.ProjectIDs {
			if !ids.Valid(projectID) {
				projectClauses = []string{"1 = 0"}
				break
			}
			args = append(args, projectID)
			projectClauses = append(projectClauses, fmt.Sprintf("mr.project_id = $%d::uuid", len(args)))
		}
		clauses = append(clauses, "("+strings.Join(projectClauses, " OR ")+")")
	}
	if query.Model != "" {
		add("mod.model_name = $%d", query.Model)
	}
	if query.GroupID != "" {
		add("mr.group_id = $%d::uuid", query.GroupID)
	}
	if query.Status != "" {
		add("mr.status = $%d", query.Status)
	}
	if query.Search != "" {
		pattern := "%" + query.Search + "%"
		start := len(args) + 1
		for i := 0; i < 5; i++ {
			args = append(args, pattern)
		}
		clauses = append(clauses, fmt.Sprintf("(tok.name ILIKE $%d OR tok.token_prefix ILIKE $%d OR ten.name ILIKE $%d OR mod.model_name ILIKE $%d OR mod.provider ILIKE $%d)", start, start+1, start+2, start+3, start+4))
	}
	if query.From != nil {
		add("mr.created_at >= $%d", *query.From)
	}
	if query.To != nil {
		add("mr.created_at < $%d", *query.To)
	}
	return strings.Join(clauses, " AND "), args
}

func financeAccountWhere(query FinanceQuery) (string, []any) {
	clauses := []string{"ten.status = 'active'", "ten.deleted_at IS NULL"}
	args := make([]any, 0)
	if query.TenantID != "" {
		args = append(args, query.TenantID)
		clauses = append(clauses, fmt.Sprintf("ten.id = $%d::uuid", len(args)))
	}
	if query.Currency != "" {
		args = append(args, query.Currency)
		clauses = append(clauses, fmt.Sprintf("COALESCE(a.currency, ten.currency) = $%d", len(args)))
	}
	if query.Search != "" {
		args = append(args, "%"+query.Search+"%")
		placeholder := fmt.Sprintf("$%d", len(args))
		clauses = append(clauses, "(ten.name ILIKE "+placeholder+" OR ten.slug ILIKE "+placeholder+")")
	}
	return strings.Join(clauses, " AND "), args
}

func financeTransactionWhere(query FinanceQuery) (string, []any) {
	clauses := []string{"ten.status = 'active'", "ten.deleted_at IS NULL"}
	args := make([]any, 0)
	if query.TenantID != "" {
		args = append(args, query.TenantID)
		clauses = append(clauses, fmt.Sprintf("ten.id = $%d::uuid", len(args)))
	}
	if query.Currency != "" {
		args = append(args, query.Currency)
		clauses = append(clauses, fmt.Sprintf("ll.currency = $%d", len(args)))
	}
	if query.Search != "" {
		args = append(args, "%"+query.Search+"%")
		placeholder := fmt.Sprintf("$%d", len(args))
		clauses = append(clauses, "(ten.name ILIKE "+placeholder+" OR COALESCE(mod.model_name, '') ILIKE "+placeholder+" OR lt.transaction_type ILIKE "+placeholder+")")
	}
	if query.From != nil {
		args = append(args, *query.From)
		clauses = append(clauses, fmt.Sprintf("lt.created_at >= $%d", len(args)))
	}
	if query.To != nil {
		args = append(args, *query.To)
		clauses = append(clauses, fmt.Sprintf("lt.created_at < $%d", len(args)))
	}
	return strings.Join(clauses, " AND "), args
}
