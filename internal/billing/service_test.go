package billing

import "testing"

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
