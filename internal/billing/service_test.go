package billing

import (
	"errors"
	"strings"
	"testing"
)

func TestIsZeroAmount(t *testing.T) {
	for _, value := range []string{"0", "0.0", "0.000000000000"} {
		if !isZeroAmount(value) {
			t.Fatalf("expected zero amount for %q", value)
		}
	}
	if isZeroAmount("0.01") {
		t.Fatal("non-zero amount was treated as zero")
	}
}

func TestNormalizeUsageClampsProviderSubsets(t *testing.T) {
	usage := normalizeUsage(Usage{
		InputTokens:       10,
		OutputTokens:      6,
		CachedInputTokens: 20,
		ReasoningTokens:   12,
	})
	if usage.CachedInputTokens != 10 || usage.ReasoningTokens != 6 {
		t.Fatalf("unexpected normalized usage: %#v", usage)
	}
}

func TestPricePerMillionTokens(t *testing.T) {
	tests := map[string]string{
		"0.000001":       "1",
		"0.000002500000": "2.5",
		"0.01":           "10000",
		"0":              "0",
	}
	for input, expected := range tests {
		if actual := pricePerMillionTokens(input); actual != expected {
			t.Fatalf("pricePerMillionTokens(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestInsufficientBalanceErrorKeepsStableSentinel(t *testing.T) {
	err := &InsufficientBalanceError{Currency: "USD", Available: "0.01", Required: "0.01536"}
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Fatal("detailed insufficient balance error must unwrap to the stable sentinel")
	}
	if !strings.Contains(err.Error(), "available=0.01") || !strings.Contains(err.Error(), "required=0.01536") {
		t.Fatalf("detailed insufficient balance error lost reservation figures: %v", err)
	}
}

func TestOfficialPriceSnapshotCanBeSettledWithoutManualPriceVersion(t *testing.T) {
	original := Price{
		ID:                 "official-price-version",
		Source:             "litellm",
		ModelID:            "model-id",
		Currency:           "USD",
		InputPricePerUnit:  "0.000005",
		OutputPricePerUnit: "0.000025",
		MinimumCharge:      "0",
		Components: []PriceComponent{
			{ComponentCode: "input_tokens", Unit: "token", PricePerUnit: "0.000005"},
			{ComponentCode: "output_tokens", Unit: "token", PricePerUnit: "0.000025"},
		},
	}

	raw := marshalJSON(priceSnapshot(original), nil)
	decoded, err := priceFromSnapshot(raw, "USD")
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Source != "litellm" || decoded.PriceVersionID != "" || decoded.ID != original.ID {
		t.Fatalf("unexpected official price snapshot: %#v", decoded)
	}
	charge, err := calculateMeteredCharge(priceComponentsFor(decoded), MeteredUsage{
		"input_tokens": "1000000", "output_tokens": "1000000",
	}, decoded.MinimumCharge)
	if err != nil {
		t.Fatal(err)
	}
	if charge.Amount != "30" {
		t.Fatalf("unexpected charge from official price snapshot: %s", charge.Amount)
	}
}

func TestPriceAllowsZeroUsageRespectsPricingTier(t *testing.T) {
	price := Price{
		MinimumCharge: "0",
		Components: []PriceComponent{
			{ComponentCode: "requests_priority", Unit: "request", PricePerUnit: "1"},
		},
	}
	if priceAllowsZeroUsage(price, "") {
		t.Fatal("priority request pricing must not apply to the default tier")
	}
	if !priceAllowsZeroUsage(price, "priority") {
		t.Fatal("priority request pricing should allow zero-token settlement for priority tier")
	}
	if priceAllowsZeroUsage(price, "flex") {
		t.Fatal("priority request pricing must not apply to flex tier")
	}
}

func TestPriceAllowsZeroUsageHonorsMinimumCharge(t *testing.T) {
	price := Price{MinimumCharge: "0.01"}
	if !priceAllowsZeroUsage(price, "") {
		t.Fatal("a non-zero minimum charge should allow settlement without token usage")
	}
}

func TestMatrixComponentEstimatesApplyGroupMultiplierAndChannelDiscount(t *testing.T) {
	customer := []PriceComponent{
		{ComponentCode: "input_tokens", Unit: "token", PricePerUnit: "0.000005"},
		{ComponentCode: "output_tokens", Unit: "token", PricePerUnit: "0.00002"},
	}
	upstream := []PriceComponent{
		{ComponentCode: "input_tokens", Unit: "token", PricePerUnit: "0.000004"},
		{ComponentCode: "output_tokens", Unit: "token", PricePerUnit: "0.000015"},
	}

	estimates := matrixComponentEstimates(customer, upstream, "1.5", "prepaid", "0.5")
	if len(estimates) != 2 {
		t.Fatalf("expected two component estimates, got %#v", estimates)
	}
	if estimates[0].CustomerPricePerUnit != "0.0000075" ||
		estimates[0].EstimatedCostPerUnit != "0.000002" ||
		estimates[0].ProfitPerUnit != "0.0000055" {
		t.Fatalf("unexpected input estimate: %#v", estimates[0])
	}
	if estimates[0].ProfitMarginPercent != "73.333333333333333333333333333333" {
		t.Fatalf("unexpected input margin: %q", estimates[0].ProfitMarginPercent)
	}
	if estimates[1].CustomerPricePerUnit != "0.00003" ||
		estimates[1].EstimatedCostPerUnit != "0.0000075" ||
		estimates[1].ProfitPerUnit != "0.0000225" {
		t.Fatalf("unexpected output estimate: %#v", estimates[1])
	}
}

func TestMatrixComponentEstimatesMarkFreeGroupAsLossWithoutMargin(t *testing.T) {
	customer := []PriceComponent{
		{ComponentCode: "input_tokens", Unit: "token", PricePerUnit: "0.000005"},
	}
	upstream := []PriceComponent{
		{ComponentCode: "input_tokens", Unit: "token", PricePerUnit: "0.000004"},
	}

	estimates := matrixComponentEstimates(customer, upstream, "1", "free", "1")
	if len(estimates) != 1 {
		t.Fatalf("expected one component estimate, got %#v", estimates)
	}
	if estimates[0].CustomerPricePerUnit != "0" ||
		estimates[0].EstimatedCostPerUnit != "0.000004" ||
		estimates[0].ProfitPerUnit != "-0.000004" ||
		estimates[0].ProfitMarginPercent != "" {
		t.Fatalf("unexpected free group estimate: %#v", estimates[0])
	}
}

func TestSettlementPricingTierUsesRequestSnapshot(t *testing.T) {
	if got := settlementPricingTier("priority", "flex"); got != "priority" {
		t.Fatalf("request pricing tier must win over reported tier, got %q", got)
	}
	if got := settlementPricingTier("", "batch"); got != "batches" {
		t.Fatalf("reported batch tier should normalize when snapshot is absent, got %q", got)
	}
	if got := settlementPricingTier("unknown", "priority"); got != "priority" {
		t.Fatalf("unknown snapshot tier should not block a valid reported tier, got %q", got)
	}
}
