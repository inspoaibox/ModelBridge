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

func TestParseNewAPIGroupRate(t *testing.T) {
	ratio := parseNewAPIGroupRate(json.RawMessage(`{
        "success": true,
        "data": {
            "default": {"ratio": 1.25, "desc": "Default"},
            "vip": {"ratio": 2}
        }
    }`), "default")
	if ratio != "1.25" {
		t.Fatalf("ratio = %q, want 1.25", ratio)
	}
}

func TestParseSub2APIBillingRate(t *testing.T) {
	ratio := parseSub2APIBillingRate(json.RawMessage(`{
        "object": "sub2api.key_billing",
        "resolved_rate_multiplier": 0.75,
        "effective_rate_multiplier": 0.9
    }`))
	if ratio != "0.9" {
		t.Fatalf("ratio = %q, want 0.9", ratio)
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
		if got := r.Header.Get("Authorization"); got != "Bearer account-secret" {
			t.Errorf("authorization = %q, want bearer account-secret", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/user/self":
			if got := r.Header.Get("New-Api-User"); got != "user-123" {
				t.Errorf("New-Api-User = %q, want user-123", got)
			}
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota":500000,"used_quota":250000,"group":"default"}}`))
		case "/api/user/self/groups":
			if got := r.Header.Get("New-Api-User"); got != "user-123" {
				t.Errorf("New-Api-User = %q, want user-123", got)
			}
			_, _ = w.Write([]byte(`{"success":true,"data":{"default":{"ratio":1.5,"desc":"Default"}}}`))
		default:
			t.Errorf("path = %q, want NewAPI account endpoint", r.URL.Path)
		}
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

func TestFetchNewAPIAccountDoesNotRequireUserHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/user/self" {
			if got := r.Header.Get("New-Api-User"); got != "" {
				t.Errorf("New-Api-User = %q, want empty", got)
			}
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota":100000,"used_quota":0,"group":"default"}}`))
			return
		}
		if r.URL.Path == "/api/user/self/groups" {
			_, _ = w.Write([]byte(`{"success":true,"data":{"default":{"ratio":1}}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	snapshot, err := fetchUpstreamAccount(context.Background(), UpstreamIntegrationNewAPI, ProviderOpenAI, server.URL, "account-secret", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snapshot.balance != "0.2" || snapshot.rateMultiplier != "1" {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
}

func TestFetchSub2APIAccountUsesUsageEndpointAndBearer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sub2api-key" {
			t.Errorf("authorization = %q, want bearer sub2api-key", got)
		}
		if got := r.Header.Get("New-Api-User"); got != "" {
			t.Errorf("New-Api-User = %q, want empty", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/usage":
			_, _ = w.Write([]byte(`{"data":{"remaining":12.34,"unit":"USD"}}`))
		case "/v1/sub2api/billing":
			_, _ = w.Write([]byte(`{"effective_rate_multiplier":0.75}`))
		default:
			t.Errorf("path = %q, want Sub2API account endpoint", r.URL.Path)
		}
	}))
	defer server.Close()

	snapshot, err := fetchUpstreamAccount(context.Background(), UpstreamIntegrationSub2API, ProviderOpenAI, server.URL, "sub2api-key", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snapshot.balance != "12.34" || snapshot.balanceUnit != "USD" || snapshot.rateMultiplier != "0.75" {
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
