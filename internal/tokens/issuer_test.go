package tokens

import "testing"

func TestNormalizeRateLimit(t *testing.T) {
	result, err := normalizeRateLimit(map[string]any{
		"requests_per_minute": float64(12),
		"tokens_per_minute":   float64(5000),
		"max_concurrent":      float64(3),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result["rpm"] != 12 || result["tpm"] != 5000 || result["concurrency"] != 3 {
		t.Fatalf("unexpected normalized limits: %#v", result)
	}
	if _, err := normalizeRateLimit(map[string]any{"rpm": float64(-1)}); err == nil {
		t.Fatal("negative rate limit must be rejected")
	}
}

func TestNormalizeSpendLimit(t *testing.T) {
	tests := map[string]string{
		"":             "0",
		"0":            "0",
		"500":          "500",
		"000500.5000":  "500.5",
		".5":           "0.5",
		"0.0000000001": "0.0000000001",
	}
	for input, expected := range tests {
		actual, err := normalizeSpendLimit(input)
		if err != nil {
			t.Fatalf("normalizeSpendLimit(%q) returned error: %v", input, err)
		}
		if actual != expected {
			t.Fatalf("normalizeSpendLimit(%q) = %q, want %q", input, actual, expected)
		}
	}

	for _, input := range []string{
		"-1",
		"+1",
		"1e3",
		"12345678901",
		"0.1234567890123456789012345678901",
		"not-a-number",
		".",
	} {
		if _, err := normalizeSpendLimit(input); err != ErrTokenSpendLimitInvalid {
			t.Fatalf("normalizeSpendLimit(%q) error = %v, want ErrTokenSpendLimitInvalid", input, err)
		}
	}
}
