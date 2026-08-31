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
