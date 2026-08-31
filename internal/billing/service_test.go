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
