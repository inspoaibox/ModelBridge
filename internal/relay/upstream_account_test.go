package relay

import (
	"encoding/json"
	"testing"
)

func TestAccountEndpointURLRemovesRelayProtocolSuffix(t *testing.T) {
	tests := []struct {
		base string
		want string
	}{
		{base: "https://gateway.example.com/v1", want: "https://gateway.example.com/api/user/self"},
		{base: "https://gateway.example.com/api/v1/", want: "https://gateway.example.com/api/user/self"},
		{base: "https://gateway.example.com/proxy/v1", want: "https://gateway.example.com/proxy/api/user/self"},
		{base: "https://gateway.example.com", want: "https://gateway.example.com/api/user/self"},
	}
	for _, test := range tests {
		got, err := accountEndpointURL(test.base, "/api/user/self")
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", test.base, err)
		}
		if got != test.want {
			t.Fatalf("%s: got %q, want %q", test.base, got, test.want)
		}
	}
}

func TestParseNewAPIAccount(t *testing.T) {
	snapshot, err := parseNewAPIAccount(json.RawMessage(`{
		"success": true,
		"data": {
			"remain_quota": 123.450000,
			"group_ratio": "0.500000"
		}
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snapshot.balance != "123.45" || snapshot.balanceUnit != "quota" || snapshot.rateMultiplier != "0.5" {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
}

func TestParseSub2APIAccount(t *testing.T) {
	snapshot, err := parseSub2APIAccount(json.RawMessage(`{
		"data": {
			"available_balance": "12.3400",
			"multiplier": 1.500000
		}
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snapshot.balance != "12.34" || snapshot.balanceUnit != "USD" || snapshot.rateMultiplier != "1.5" {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
}

func TestSingleRateMultiplierRejectsDifferentGroupRates(t *testing.T) {
	if got := singleRateMultiplier(json.RawMessage(`{
		"data": [
			{"group": "default", "model_ratio": 0.5},
			{"group": "vip", "model_ratio": 1.5}
		]
	}`)); got != "" {
		t.Fatalf("got %q, want empty multiplier for different group rates", got)
	}
}

func TestSingleRateMultiplierAcceptsOneRate(t *testing.T) {
	if got := singleRateMultiplier(json.RawMessage(`{
		"data": {"default": {"model_ratio": "1.000000"}}
	}`)); got != "1" {
		t.Fatalf("got %q, want 1", got)
	}
}

func TestAccountNumberDoesNotIntroduceFloatingPointArtifacts(t *testing.T) {
	tests := map[string]string{
		"123.450000": "123.45",
		"12.3400":    "12.34",
		"0.000000":   "0",
		"1e-6":       "0.000001",
	}
	for raw, want := range tests {
		if got := accountNumber(json.Number(raw)); got != want {
			t.Fatalf("%s: got %q, want %q", raw, got, want)
		}
	}
}
