package relay

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
            "quota": 500000,
            "used_quota": 250000,
            "group": "default",
            "group_ratio": 1.5
        }
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snapshot.balance != "1" || snapshot.balanceUsed != "0.5" || snapshot.balanceTotal != "1.5" ||
		snapshot.balanceUnit != "USD" || snapshot.planName != "default" || snapshot.rateMultiplier != "1.5" {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
}

func TestParseSub2APIAccount(t *testing.T) {
	snapshot, err := parseSub2APIAccount(json.RawMessage(`{
        "remaining": "12.3400",
        "unit": "USD",
        "total": "20",
        "used": "7.66",
        "planName": "API Key 额度",
        "multiplier": 1.500000
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snapshot.balance != "12.34" || snapshot.balanceUnit != "USD" || snapshot.balanceTotal != "20" ||
		snapshot.balanceUsed != "7.66" || snapshot.planName != "API Key 额度" || snapshot.rateMultiplier != "1.5" {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
}

func TestFetchNewAPIAccountUsesProtocolEndpointAndHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/user/self" {
			t.Errorf("path = %q, want /api/user/self", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer account-secret" {
			t.Errorf("authorization = %q, want bearer account-secret", got)
		}
		if got := r.Header.Get("New-Api-User"); got != "user-123" {
			t.Errorf("New-Api-User = %q, want user-123", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"quota":500000,"used_quota":250000,"group":"default","group_ratio":1.5}}`))
	}))
	defer server.Close()

	snapshot, err := fetchUpstreamAccount(context.Background(), UpstreamIntegrationNewAPI, ProviderOpenAI, server.URL, "account-secret", "user-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snapshot.balance != "1" || snapshot.balanceUsed != "0.5" || snapshot.balanceTotal != "1.5" {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
}

func TestFetchSub2APIAccountUsesUsageEndpointAndBearer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/usage" {
			t.Errorf("path = %q, want /v1/usage", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sub2api-key" {
			t.Errorf("authorization = %q, want bearer sub2api-key", got)
		}
		if got := r.Header.Get("New-Api-User"); got != "" {
			t.Errorf("New-Api-User = %q, want empty", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"remaining":12.34,"unit":"USD"}}`))
	}))
	defer server.Close()

	snapshot, err := fetchUpstreamAccount(context.Background(), UpstreamIntegrationSub2API, ProviderOpenAI, server.URL, "sub2api-key", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snapshot.balance != "12.34" || snapshot.balanceUnit != "USD" {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
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
