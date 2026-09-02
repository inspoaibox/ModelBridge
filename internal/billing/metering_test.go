package billing

import (
	"strings"
	"testing"
)

func TestMeteredChargeUsesMediaPriceAndDoesNotDoubleChargeText(t *testing.T) {
	charge, err := calculateMeteredCharge([]PriceComponent{
		{ComponentCode: "input_tokens", Unit: "token", PricePerUnit: "1"},
		{ComponentCode: "input_audio_tokens", Unit: "audio_token", PricePerUnit: "3"},
	}, MeteredUsage{"input_tokens": "100", "input_audio_tokens": "20"}, "0")
	if err != nil {
		t.Fatal(err)
	}
	if charge.Amount != "140" {
		t.Fatalf("unexpected charge: %s", charge.Amount)
	}
}

func TestMeteredUsageClampsOverlappingProviderSubsets(t *testing.T) {
	charge, err := calculateMeteredCharge([]PriceComponent{
		{ComponentCode: "input_tokens", Unit: "token", PricePerUnit: "1"},
		{ComponentCode: "cached_input_tokens", Unit: "token", PricePerUnit: "2"},
		{ComponentCode: "cache_creation_tokens", Unit: "token", PricePerUnit: "3"},
		{ComponentCode: "input_audio_tokens", Unit: "audio_token", PricePerUnit: "4"},
	}, MeteredUsage{
		"input_tokens": "100", "cached_input_tokens": "80",
		"cache_creation_tokens": "50", "input_audio_tokens": "40",
	}, "0")
	if err != nil {
		t.Fatal(err)
	}
	// The children are allocated in provider order: 80 cached, 20 cache
	// creation, and no ordinary audio remains inside the 100-token parent.
	if charge.Amount != "220" {
		t.Fatalf("unexpected normalized charge: %s", charge.Amount)
	}
}

func TestMissingLegacySubsetPriceFallsBackToParent(t *testing.T) {
	charge, err := calculateMeteredCharge(legacyPriceComponents(Price{
		InputPricePerUnit:       "1",
		CachedInputPricePerUnit: "0",
	}), MeteredUsage{"input_tokens": "100", "cached_input_tokens": "20"}, "0")
	if err != nil {
		t.Fatal(err)
	}
	if charge.Amount != "100" {
		t.Fatalf("missing legacy cache price did not fall back to parent: %s", charge.Amount)
	}
}

func TestExplicitZeroSubsetComponentRemainsFree(t *testing.T) {
	charge, err := calculateMeteredCharge([]PriceComponent{
		{ComponentCode: "input_tokens", Unit: "token", PricePerUnit: "1"},
		{ComponentCode: "cached_input_tokens", Unit: "token", PricePerUnit: "0"},
	}, MeteredUsage{"input_tokens": "100", "cached_input_tokens": "20"}, "0")
	if err != nil {
		t.Fatal(err)
	}
	if charge.Amount != "80" {
		t.Fatalf("explicit zero cache price was not honored: %s", charge.Amount)
	}
}

func TestLegacyPriceKeepsExplicitFreePrimaryMeter(t *testing.T) {
	components := legacyPriceComponents(Price{InputPricePerUnit: "0", OutputPricePerUnit: "1"})
	charge, err := calculateMeteredCharge(components, MeteredUsage{"input_tokens": "10", "output_tokens": "2"}, "0")
	if err != nil {
		t.Fatal(err)
	}
	if charge.Amount != "2" {
		t.Fatalf("free legacy input meter was treated as missing: %s", charge.Amount)
	}
}

func TestMeteredChargeRejectsUnknownPaidMeter(t *testing.T) {
	_, err := calculateMeteredCharge([]PriceComponent{
		{ComponentCode: "input_tokens", Unit: "token", PricePerUnit: "1"},
	}, MeteredUsage{"input_tokens": "100", "video_seconds": "2"}, "0")
	if err == nil || !strings.Contains(err.Error(), "video_seconds") {
		t.Fatalf("expected missing video price, got %v", err)
	}
}

func TestMeteredChargeAddsPerRequestFeeOnce(t *testing.T) {
	charge, err := calculateMeteredCharge([]PriceComponent{
		{ComponentCode: "input_tokens", Unit: "token", PricePerUnit: "1"},
		{ComponentCode: "requests", Unit: "request", PricePerUnit: "5"},
	}, MeteredUsage{"input_tokens": "2"}, "0")
	if err != nil {
		t.Fatal(err)
	}
	if charge.Amount != "7" {
		t.Fatalf("unexpected request charge: %s", charge.Amount)
	}
}

func TestMeteredChargeUsesPriorityComponent(t *testing.T) {
	charge, err := calculateMeteredChargeForTier([]PriceComponent{
		{ComponentCode: "input_tokens", Unit: "token", PricePerUnit: "1"},
		{ComponentCode: "input_tokens_priority", Unit: "token", PricePerUnit: "3"},
	}, MeteredUsage{"input_tokens": "10"}, "0", "priority")
	if err != nil {
		t.Fatal(err)
	}
	if charge.Amount != "30" || len(charge.Lines) != 1 || charge.Lines[0].PricePerUnit != "3" {
		t.Fatalf("unexpected priority charge: %#v", charge)
	}
}

func TestTieredMeteredChargeUsesProgressiveRanges(t *testing.T) {
	charge, err := calculateMeteredCharge([]PriceComponent{{
		ComponentCode: "input_tokens",
		Unit:          "token",
		PricePerUnit:  "1",
		Tiers:         []byte("[{\"up_to\":\"10\",\"price_per_unit\":\"1\"},{\"price_per_unit\":\"2\"}]"),
	}}, MeteredUsage{"input_tokens": "15"}, "0")
	if err != nil {
		t.Fatal(err)
	}
	if charge.Amount != "20" {
		t.Fatalf("unexpected tiered charge: %s", charge.Amount)
	}
}

func TestContextTierUsesProgressiveRanges(t *testing.T) {
	charge, err := calculateMeteredCharge([]PriceComponent{{
		ComponentCode: "input_tokens", Unit: "token", PricePerUnit: "1",
		Tiers:    []byte(`[{"up_to":"100","price_per_unit":"1"},{"price_per_unit":"2"}]`),
		Metadata: []byte(`{"tier_basis":"input_tokens","tier_mode":"context"}`),
	}}, MeteredUsage{"input_tokens": "150"}, "0")
	if err != nil {
		t.Fatal(err)
	}
	if charge.Amount != "300" {
		t.Fatalf("context tier did not apply one rate to the whole request: %s", charge.Amount)
	}
}

func TestCachedMediaIsNotSubtractedTwiceFromPrompt(t *testing.T) {
	charge, err := calculateMeteredCharge([]PriceComponent{
		{ComponentCode: "input_tokens", Unit: "token", PricePerUnit: "1"},
		{ComponentCode: "cached_input_tokens", Unit: "token", PricePerUnit: "0.5"},
		{ComponentCode: "input_audio_tokens", Unit: "audio_token", PricePerUnit: "2"},
		{ComponentCode: "cached_audio_tokens", Unit: "audio_token", PricePerUnit: "0.25"},
	}, MeteredUsage{
		"input_tokens": "100", "cached_input_tokens": "20",
		"input_audio_tokens": "20", "cached_audio_tokens": "10",
	}, "0")
	if err != nil {
		t.Fatal(err)
	}
	// 60 ordinary text, 10 non-cached audio, and 10 cached audio.
	if charge.Amount != "92.5" {
		t.Fatalf("cached media was double-subtracted or double-charged: %s", charge.Amount)
	}
}

func TestNestedMediaCacheCannotExceedParentPrompt(t *testing.T) {
	charge, err := calculateMeteredCharge([]PriceComponent{
		{ComponentCode: "input_tokens", Unit: "token", PricePerUnit: "1"},
		{ComponentCode: "cached_input_tokens", Unit: "token", PricePerUnit: "1"},
		{ComponentCode: "input_image_tokens", Unit: "image_token", PricePerUnit: "2"},
		{ComponentCode: "cached_image_tokens", Unit: "image_token", PricePerUnit: "3"},
	}, MeteredUsage{
		"input_tokens": "100", "cached_input_tokens": "80",
		"input_image_tokens": "40", "cached_image_tokens": "30",
	}, "0")
	if err != nil {
		t.Fatal(err)
	}
	// The parent can account for only 20 image tokens after generic cached
	// input. The nested cached image subset is clamped to that 20-token parent.
	if charge.Amount != "140" {
		t.Fatalf("nested media cache exceeded its parent: %s", charge.Amount)
	}
}

func TestNormalizeUsageProjectsMetricsIntoLegacyColumns(t *testing.T) {
	usage := normalizeUsage(Usage{Metrics: MeteredUsage{
		"input_tokens":        "100",
		"output_tokens":       "40",
		"cached_input_tokens": "12",
		"reasoning_tokens":    "8",
	}, Source: "reconciliation"})
	if usage.InputTokens != 100 || usage.OutputTokens != 40 || usage.CachedInputTokens != 12 || usage.ReasoningTokens != 8 {
		t.Fatalf("metrics were not projected into legacy usage columns: %#v", usage)
	}
}

func TestUpstreamCostUsesDiscountWithoutChangingCustomerMultiplier(t *testing.T) {
	components := []PriceComponent{
		{ComponentCode: "input_tokens", Unit: "token", PricePerUnit: "0.01"},
		{ComponentCode: "output_tokens", Unit: "token", PricePerUnit: "0.02"},
	}
	cost, err := calculateUpstreamCost(
		components,
		MeteredUsage{"input_tokens": "100", "output_tokens": "50"},
		"0",
		"0.5",
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	// The reference charge is 2.00; a 50% channel cost factor produces 1.00.
	if cost != "1" {
		t.Fatalf("unexpected discounted upstream cost: %s", cost)
	}
	customerCharge, err := multiplierCharge(MeteredCharge{Amount: "2"}, "3")
	if err != nil {
		t.Fatal(err)
	}
	if customerCharge.Amount != "6" {
		t.Fatalf("customer group multiplier was affected by upstream discount: %s", customerCharge.Amount)
	}
}

func TestUpstreamCostDefaultsToFullReferencePrice(t *testing.T) {
	cost, err := calculateUpstreamCost(
		[]PriceComponent{{ComponentCode: "requests", Unit: "request", PricePerUnit: "0.25"}},
		MeteredUsage{},
		"0",
		"",
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	if cost != "0.25" {
		t.Fatalf("empty upstream discount should default to 1: %s", cost)
	}
}
