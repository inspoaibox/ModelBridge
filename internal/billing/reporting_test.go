package billing

import (
	"strings"
	"testing"
)

func TestUsageWhereUsesDistinctSearchParameters(t *testing.T) {
	where, args := usageWhere(UsageQuery{Search: "gpt-5", Status: "settled"})
	if len(args) != 6 {
		t.Fatalf("expected status plus five search args, got %d", len(args))
	}
	if !strings.Contains(where, "tok.name ILIKE $2") || !strings.Contains(where, "mod.provider ILIKE $6") {
		t.Fatalf("search predicates were not parameterized independently: %s", where)
	}
	for _, value := range args[1:] {
		if value != "%gpt-5%" {
			t.Fatalf("unexpected search parameter: %#v", value)
		}
	}
}

func TestReportQueriesNormalizePagination(t *testing.T) {
	usage := normalizeUsageQuery(UsageQuery{Limit: 999, Offset: -1})
	if usage.Limit != 50 || usage.Offset != 0 {
		t.Fatalf("unexpected usage pagination normalization: %#v", usage)
	}
	finance := normalizeFinanceQuery(FinanceQuery{Limit: 0, Offset: -4, Currency: " usd "})
	if finance.Limit != 50 || finance.Offset != 0 || finance.Currency != "USD" {
		t.Fatalf("unexpected finance pagination normalization: %#v", finance)
	}
}

func TestNormalizeDecimalTextRemovesScalePadding(t *testing.T) {
	tests := map[string]string{
		"0.000003000000000000000000000000":  "0.000003",
		"12.340000000000000000000000000000": "12.34",
		"00042.000":                         "42",
		"0.000000000000000000000000000000":  "0",
		"-0.000000":                         "0",
	}
	for input, expected := range tests {
		if actual := normalizeDecimalText(input); actual != expected {
			t.Fatalf("normalizeDecimalText(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestUsageWhereScopesToAuthorizedProjects(t *testing.T) {
	where, args := usageWhere(UsageQuery{TenantID: "11111111-1111-4111-8111-111111111111", ProjectIDs: []string{
		"22222222-2222-4222-8222-222222222222",
		"33333333-3333-4333-8333-333333333333",
	}})
	if !strings.Contains(where, "mr.project_id = $2::uuid OR mr.project_id = $3::uuid") || len(args) != 3 {
		t.Fatalf("expected project scope predicates, where=%s args=%#v", where, args)
	}
	where, _ = usageWhere(UsageQuery{ProjectIDs: []string{"not-a-uuid"}})
	if !strings.Contains(where, "1 = 0") {
		t.Fatalf("invalid project scope must fail closed: %s", where)
	}
}

func TestUsageWhereSupportsConsoleFilters(t *testing.T) {
	query := normalizeUsageQuery(UsageQuery{
		TokenName: "  production  ",
		Model:     " claude ",
		GroupID:   "11111111-1111-4111-8111-111111111111",
	})
	where, args := usageWhere(query)
	if query.TokenName != "production" || query.Model != "claude" {
		t.Fatalf("console filter values were not normalized: %#v", query)
	}
	if !strings.Contains(where, "mod.model_name ILIKE $1") ||
		!strings.Contains(where, "tok.name ILIKE $2") ||
		!strings.Contains(where, "mr.group_id = $3::uuid") {
		t.Fatalf("console filter predicates are incomplete: %s", where)
	}
	if len(args) != 3 || args[0] != "%claude%" || args[1] != "%production%" || args[2] != query.GroupID {
		t.Fatalf("unexpected console filter arguments: %#v", args)
	}
}
