package relay

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ai-token/internal/auth"
	"ai-token/internal/billing"
)

func TestServiceRoutesThroughSelectedChannel(t *testing.T) {
	router := &fakeChannelRouter{
		channel: Channel{
			ID:                "channel-1",
			Provider:          ProviderOpenAI,
			BaseURL:           "https://api.openai.com/v1",
			CredentialRef:     "env:OPENAI_API_KEY",
			UpstreamModelName: "gpt-5-mini",
		},
	}
	provider := &recordingProvider{
		response: ChatCompletionResponse{
			ID:      "upstream-id",
			Object:  "chat.completion",
			Created: 123,
			Model:   "gpt-5-mini",
			Choices: []ChatCompletionChoice{
				{
					Index: 0,
					Message: ChatCompletionReply{
						Role:    "assistant",
						Content: "ok",
					},
					FinishReason: "stop",
				},
			},
		},
	}
	service, err := NewService(
		router,
		EnvCredentialResolver{Lookup: func(name string) string {
			if name == "OPENAI_API_KEY" {
				return " sk-test "
			}
			return ""
		}},
		map[string]Provider{ProviderOpenAI: provider},
	)
	if err != nil {
		t.Fatal(err)
	}

	response, err := service.ChatCompletions(context.Background(), &auth.Principal{
		Audience:      auth.AudienceRelay,
		AllowedModels: map[string]struct{}{"gpt-5": {}},
	}, ChatCompletionRequest{
		Model: "gpt-5",
		Messages: []ChatMessage{
			{Role: "user", Content: "hello"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if router.selectedModel != "gpt-5" {
		t.Fatalf("unexpected selected model: %s", router.selectedModel)
	}
	if provider.received.APIKey != "sk-test" || provider.received.UpstreamModel != "gpt-5-mini" {
		t.Fatalf("unexpected upstream request: %#v", provider.received)
	}
	if response.Model != "gpt-5" || response.Choices[0].Message.Content != "ok" {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestChannelMutationKeepsMultipleModelsForOneProvider(t *testing.T) {
	request, err := (ChannelMutation{
		Name:     "OpenAI",
		Provider: ProviderOpenAI,
		BaseURL:  "https://api.openai.com/v1",
		APIKey:   "sk-test",
		Models: []ChannelModelMutation{
			{Model: "gpt-5", UpstreamModel: "gpt-5"},
			{Model: "gpt-5-mini", UpstreamModel: "gpt-5-mini"},
		},
	}).validate(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(request.Models) != 2 {
		t.Fatalf("expected two same-provider models to remain mapped, got %#v", request.Models)
	}
}

func TestChannelMutationAllowsEmptyModelMappings(t *testing.T) {
	request, err := (ChannelMutation{
		Name:     "Anthropic",
		Provider: ProviderAnthropic,
		BaseURL:  "https://api.anthropic.com",
		APIKey:   "sk-ant-test",
	}).validate(true)
	if err != nil {
		t.Fatalf("a channel may be saved before model discovery or manual mapping: %v", err)
	}
	if len(request.Models) != 0 {
		t.Fatalf("empty channel mappings must remain empty, got %#v", request.Models)
	}
}

func TestServiceRejectsModelOutsideTokenAllowlist(t *testing.T) {
	provider := &recordingProvider{}
	service, err := NewService(
		&fakeChannelRouter{channel: Channel{Provider: ProviderOpenAI, CredentialRef: "env:OPENAI_API_KEY"}},
		EnvCredentialResolver{Lookup: func(string) string { return "sk-test" }},
		map[string]Provider{ProviderOpenAI: provider},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.ChatCompletions(context.Background(), &auth.Principal{
		Audience:      auth.AudienceRelay,
		AllowedModels: map[string]struct{}{"gpt-5-mini": {}},
	}, ChatCompletionRequest{
		Model:    "gpt-5",
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	})
	if !errors.Is(err, ErrModelNotAllowed) {
		t.Fatalf("expected ErrModelNotAllowed, got %v", err)
	}
	if provider.called {
		t.Fatal("provider must not be called when model is outside the token allowlist")
	}
}

func TestServiceReservesBeforeUpstreamAndSettlesAfterSuccess(t *testing.T) {
	provider := &recordingProvider{
		response: ChatCompletionResponse{
			Usage: ChatUsage{
				PromptTokens:     12,
				CompletionTokens: 8,
				TotalTokens:      20,
			},
		},
	}
	biller := &fakeBillingService{
		reservation: billing.Reservation{ID: "reservation-1"},
	}
	service, err := NewService(
		&fakeChannelRouter{channel: Channel{
			ID:            "channel-1",
			Provider:      ProviderOpenAI,
			CredentialRef: "env:OPENAI_API_KEY",
		}},
		EnvCredentialResolver{Lookup: func(string) string { return "sk-test" }},
		map[string]Provider{ProviderOpenAI: provider},
		biller,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.ChatCompletions(context.Background(), &auth.Principal{
		ID:            "token-1",
		TokenID:       "token-1",
		Audience:      auth.AudienceRelay,
		TenantID:      "tenant-1",
		ProjectIDs:    map[string]struct{}{"project-1": {}},
		AllowedModels: map[string]struct{}{"gpt-5": {}},
	}, ChatCompletionRequest{
		Model:          "gpt-5",
		RequestID:      "request-1",
		IdempotencyKey: "idempotency-1",
		Messages:       []ChatMessage{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(biller.events) != 2 || biller.events[0] != "reserve" || biller.events[1] != "settle" {
		t.Fatalf("unexpected billing order: %#v", biller.events)
	}
	if !provider.called || biller.settledUsage.InputTokens != 12 || biller.settledUsage.OutputTokens != 8 {
		t.Fatalf("billing did not receive upstream usage: %#v", biller)
	}
}

func TestServiceDoesNotEstimatePaidChatUsageWhenUpstreamOmitsUsage(t *testing.T) {
	provider := &recordingProvider{
		response: ChatCompletionResponse{
			Choices: []ChatCompletionChoice{{Message: ChatCompletionReply{Role: "assistant", Content: "ok"}}},
		},
	}
	base := &fakeBillingService{reservation: billing.Reservation{ID: "reservation-1"}}
	biller := &pendingBillingService{fakeBillingService: base}
	service, err := NewService(
		&fakeChannelRouter{channel: Channel{
			ID:            "channel-1",
			Provider:      ProviderOpenAI,
			CredentialRef: "env:OPENAI_API_KEY",
		}},
		EnvCredentialResolver{Lookup: func(string) string { return "sk-test" }},
		map[string]Provider{ProviderOpenAI: provider},
		biller,
	)
	if err != nil {
		t.Fatal(err)
	}

	response, err := service.ChatCompletions(context.Background(), &auth.Principal{
		ID:            "token-1",
		TokenID:       "token-1",
		Audience:      auth.AudienceRelay,
		TenantID:      "tenant-1",
		AllowedModels: map[string]struct{}{"gpt-5": {}},
	}, ChatCompletionRequest{
		Model:          "gpt-5",
		RequestID:      "request-1",
		IdempotencyKey: "idempotency-1",
		Messages:       []ChatMessage{{Role: "user", Content: "hello"}},
	})
	if err != nil || response.Choices[0].Message.Content != "ok" {
		t.Fatalf("successful upstream response should be returned while billing is pending: %#v %v", response, err)
	}
	if len(base.events) != 2 || base.events[0] != "reserve" || base.events[1] != "pending" {
		t.Fatalf("paid request must retain its reservation for reconciliation: %#v", biller.events)
	}
}

func TestServiceDoesNotSettlePaidChatWithIncompleteUsage(t *testing.T) {
	provider := &recordingProvider{
		response: ChatCompletionResponse{
			Choices: []ChatCompletionChoice{{Message: ChatCompletionReply{Role: "assistant", Content: "ok"}}},
			Usage:   ChatUsage{PromptTokens: 8, UsageProvided: true},
		},
	}
	base := &fakeBillingService{reservation: billing.Reservation{ID: "reservation-1"}}
	biller := &pendingBillingService{fakeBillingService: base}
	service, err := NewService(
		&fakeChannelRouter{channel: Channel{
			ID: "channel-1", Provider: ProviderOpenAI, CredentialRef: "env:OPENAI_API_KEY",
		}},
		EnvCredentialResolver{Lookup: func(string) string { return "sk-test" }},
		map[string]Provider{ProviderOpenAI: provider},
		biller,
	)
	if err != nil {
		t.Fatal(err)
	}

	response, err := service.ChatCompletions(context.Background(), &auth.Principal{
		ID: "token-1", TokenID: "token-1", Audience: auth.AudienceRelay, TenantID: "tenant-1",
		AllowedModels: map[string]struct{}{"gpt-5": {}},
	}, ChatCompletionRequest{
		Model: "gpt-5", RequestID: "request-1", IdempotencyKey: "idempotency-1",
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	})
	if err != nil || response.Choices[0].Message.Content != "ok" {
		t.Fatalf("successful upstream response should be returned while billing is pending: %#v %v", response, err)
	}
	if len(base.events) != 2 || base.events[0] != "reserve" || base.events[1] != "pending_usage" {
		t.Fatalf("incomplete usage must remain pending: %#v", base.events)
	}
	if biller.pendingUsage.InputTokens != 8 || biller.pendingUsage.OutputTokens != 0 {
		t.Fatalf("pending usage snapshot is wrong: %#v", biller.pendingUsage)
	}
}

func TestScopedBillingIdempotencyKeySeparatesTokens(t *testing.T) {
	metadata := RequestMetadata{Endpoint: "/v1/chat/completions", RequestID: "request-1"}
	first := scopedBillingIdempotencyKey(&auth.Principal{TokenID: "token-a"}, metadata, "same-key")
	second := scopedBillingIdempotencyKey(&auth.Principal{TokenID: "token-b"}, metadata, "same-key")
	if first == second || first == "same-key" || len(first) <= len("relay:") {
		t.Fatalf("idempotency key must be scoped and opaque: %q %q", first, second)
	}
	if repeat := scopedBillingIdempotencyKey(&auth.Principal{TokenID: "token-a"}, metadata, "same-key"); repeat != first {
		t.Fatalf("scoped idempotency key must be deterministic: %q != %q", repeat, first)
	}
}

func TestServicePreservesUsageWhenSettlementFails(t *testing.T) {
	base := &fakeBillingService{
		reservation: billing.Reservation{ID: "reservation-1"},
		settleErr:   errors.New("temporary settlement failure"),
	}
	biller := &pendingBillingService{fakeBillingService: base}
	service := &Service{billing: biller}

	err := service.completeRelayBilling(context.Background(), base.reservation, "", ChatUsage{
		PromptTokens:     12,
		CompletionTokens: 8,
		TotalTokens:      20,
	}, "provider-request-1", "channel-1", "upstream")
	if err != nil {
		t.Fatalf("settlement failure should be preserved as pending without failing the successful relay: %v", err)
	}
	if len(base.events) != 2 || base.events[0] != "settle" || base.events[1] != "pending_usage" {
		t.Fatalf("usage settlement failure must be retained for reconciliation: %#v", base.events)
	}
	if biller.pendingUsage.InputTokens != 12 || biller.pendingUsage.OutputTokens != 8 {
		t.Fatalf("pending record lost upstream usage: %#v", biller.pendingUsage)
	}
}

func TestPendingUsageMarkerFallsBackToOrdinaryPending(t *testing.T) {
	base := &fakeBillingService{reservation: billing.Reservation{ID: "reservation-1"}}
	biller := &pendingBillingService{
		fakeBillingService: base,
		pendingErr:         errors.New("cannot persist usage snapshot"),
	}
	service := &Service{billing: biller}

	service.markRelayBillingPendingWithUsage(
		context.Background(),
		base.reservation,
		"",
		"billing_settlement_failed",
		billing.Usage{InputTokens: 3, OutputTokens: 2, Source: "upstream"},
		"provider-request-2",
	)
	if len(base.events) != 2 || base.events[0] != "pending_usage" || base.events[1] != "pending" {
		t.Fatalf("failed usage snapshot must fall back to pending state: %#v", base.events)
	}
}

func TestChatUsageProvidedFlagWithoutQuantitiesIsNotBillableUsage(t *testing.T) {
	if chatUsageHasValues(ChatUsage{UsageProvided: true}) {
		t.Fatal("an empty usage object must not count as reliable usage")
	}
}

func TestServiceReleasesReservationOnUpstreamFailure(t *testing.T) {
	provider := &recordingProvider{err: ErrUpstream}
	biller := &fakeBillingService{
		reservation: billing.Reservation{ID: "reservation-1"},
	}
	service, err := NewService(
		&fakeChannelRouter{channel: Channel{
			ID:            "channel-1",
			Provider:      ProviderOpenAI,
			CredentialRef: "env:OPENAI_API_KEY",
		}},
		EnvCredentialResolver{Lookup: func(string) string { return "sk-test" }},
		map[string]Provider{ProviderOpenAI: provider},
		biller,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.ChatCompletions(context.Background(), &auth.Principal{
		ID:            "token-1",
		TokenID:       "token-1",
		Audience:      auth.AudienceRelay,
		TenantID:      "tenant-1",
		ProjectIDs:    map[string]struct{}{"project-1": {}},
		AllowedModels: map[string]struct{}{"gpt-5": {}},
	}, ChatCompletionRequest{
		Model:          "gpt-5",
		RequestID:      "request-1",
		IdempotencyKey: "idempotency-1",
		Messages:       []ChatMessage{{Role: "user", Content: "hello"}},
	})
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("expected upstream error, got %v", err)
	}
	if len(biller.events) != 2 || biller.events[0] != "reserve" || biller.events[1] != "fail" {
		t.Fatalf("unexpected billing order: %#v", biller.events)
	}
}

func TestChatUsageBillingBreakdownClampsProviderDetails(t *testing.T) {
	inputTokens, cachedInputTokens, outputTokens, reasoningTokens := (ChatUsage{
		PromptTokens:     10,
		CompletionTokens: 6,
		PromptTokensDetails: &ChatPromptTokensDetails{
			CachedTokens: 20,
		},
		CompletionTokensDetails: &ChatCompletionTokensDetails{
			ReasoningTokens: 12,
		},
	}).billingBreakdown()

	if inputTokens != 10 || cachedInputTokens != 10 || outputTokens != 6 || reasoningTokens != 6 {
		t.Fatalf(
			"unexpected billing breakdown: input=%d cached=%d output=%d reasoning=%d",
			inputTokens,
			cachedInputTokens,
			outputTokens,
			reasoningTokens,
		)
	}
}

func TestEnvCredentialResolverOnlyAllowsEnvRefs(t *testing.T) {
	resolver := EnvCredentialResolver{Lookup: func(name string) string {
		if name == "ANTHROPIC_API_KEY" {
			return " anthropic-secret "
		}
		return ""
	}}

	secret, err := resolver.Resolve(context.Background(), "env:ANTHROPIC_API_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if secret != "anthropic-secret" {
		t.Fatalf("unexpected secret: %q", secret)
	}

	if _, err := resolver.Resolve(context.Background(), "plain:secret"); !errors.Is(err, ErrCredentialUnavailable) {
		t.Fatalf("expected plaintext refs to be rejected, got %v", err)
	}
	if _, err := resolver.Resolve(context.Background(), "env:"); !errors.Is(err, ErrCredentialUnavailable) {
		t.Fatalf("expected blank env refs to be rejected, got %v", err)
	}
}

func TestOpenAIProviderListsModelsWithoutExposingCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected model path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Fatalf("unexpected authorization header: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"gpt-z"},{"id":"gpt-a"},{"id":"gpt-a"}]}`))
	}))
	defer server.Close()

	models, err := (OpenAIProvider{}).ListModels(context.Background(), server.URL+"/v1", "sk-test")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].ID != "gpt-a" || models[1].ID != "gpt-z" {
		t.Fatalf("unexpected discovered models: %#v", models)
	}
	if models[0].Provider != ProviderOpenAI || models[0].DisplayName != "gpt-a" {
		t.Fatalf("unexpected model metadata: %#v", models[0])
	}
}

func TestGrokProviderListsModelsThroughXAICompatibleEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected model path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer xai-test" {
			t.Fatalf("unexpected authorization header: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"grok-4.6"}]}`))
	}))
	defer server.Close()

	models, err := (GrokProvider{}).ListModels(context.Background(), server.URL+"/v1", "xai-test")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "grok-4.6" || models[0].Provider != ProviderGrok {
		t.Fatalf("unexpected Grok models: %#v", models)
	}
}

func TestDefaultProvidersIncludeGrokAndGemini(t *testing.T) {
	providers := DefaultProviders()
	for _, name := range []string{ProviderOpenAI, ProviderAnthropic, ProviderGrok, ProviderGemini} {
		if providers[name] == nil {
			t.Fatalf("default providers must include %s", name)
		}
	}
}

func TestGeminiProviderTranslatesGenerateContentResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-test:generateContent" {
			t.Fatalf("unexpected Gemini path: %s", r.URL.Path)
		}
		if r.Header.Get("x-goog-api-key") != "gemini-test" {
			t.Fatalf("unexpected Gemini api key header: %q", r.Header.Get("x-goog-api-key"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"responseId":"gemini-response-1",
			"modelVersion":"gemini-test-001",
			"createTime":"2026-08-29T00:00:00Z",
			"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"hello from Gemini"}]},"finishReason":"STOP"}],
			"usageMetadata":{"promptTokenCount":5,"cachedContentTokenCount":2,"candidatesTokenCount":7,"thoughtsTokenCount":2,"totalTokenCount":14,"promptTokensDetails":[{"modality":"IMAGE","tokenCount":3}],"cacheTokensDetails":[{"modality":"IMAGE","tokenCount":2}]}
		}`))
	}))
	defer server.Close()

	response, err := (GeminiProvider{}).ChatCompletions(context.Background(), UpstreamChatCompletionRequest{
		Channel: Channel{ID: "gemini-channel", BaseURL: server.URL},
		APIKey:  "gemini-test",
		Request: ChatCompletionRequest{
			Model:    "gemini-test",
			Messages: []ChatMessage{{Role: "system", Content: "policy"}, {Role: "user", Content: "hello"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.ID != "gemini-response-1" || response.Model != "gemini-test-001" || response.Choices[0].Message.Content != "hello from Gemini" {
		t.Fatalf("unexpected Gemini response: %#v", response)
	}
	if response.Usage.PromptTokens != 5 || response.Usage.CompletionTokens != 9 || response.Usage.CompletionTokensDetails.ReasoningTokens != 2 {
		t.Fatalf("unexpected Gemini usage: %#v", response.Usage)
	}
	metrics := response.Usage.meteredUsage()
	if metrics["input_tokens"] != "5" || metrics["input_image_tokens"] != "5" || metrics["cached_input_tokens"] != "0" || metrics["cached_image_tokens"] != "2" {
		t.Fatalf("Gemini media/cache usage was not kept disjoint: %#v", metrics)
	}
}

func TestGeminiProviderListsModelsWithOfficialSDK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models" {
			t.Fatalf("unexpected Gemini model path: %s", r.URL.Path)
		}
		if r.Header.Get("x-goog-api-key") != "gemini-test" {
			t.Fatalf("unexpected Gemini api key header: %q", r.Header.Get("x-goog-api-key"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"models/gemini-test","displayName":"Gemini Test","supportedActions":["generateContent"]}]}`))
	}))
	defer server.Close()

	models, err := (GeminiProvider{}).ListModels(context.Background(), server.URL, "gemini-test")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "gemini-test" || models[0].DisplayName != "Gemini Test" || models[0].Provider != ProviderGemini {
		t.Fatalf("unexpected Gemini models: %#v", models)
	}
}

func TestOpenAIProviderPreservesHTTPStatusForFailover(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limited","type":"rate_limit_error"}}`))
	}))
	defer server.Close()

	_, err := (OpenAIProvider{}).ChatCompletions(context.Background(), UpstreamChatCompletionRequest{
		Channel: Channel{ID: "channel-1", BaseURL: server.URL + "/v1"},
		APIKey:  "sk-test",
		Request: ChatCompletionRequest{Model: "gpt-5", Messages: []ChatMessage{{Role: "user", Content: "hello"}}},
	})
	var upstreamErr *UpstreamError
	if !errors.As(err, &upstreamErr) || upstreamErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected status-preserving upstream error, got %T %v", err, err)
	}
}

func TestAnthropicProviderPreservesHTTPStatusForFailover(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"overloaded_error","message":"busy"}}`))
	}))
	defer server.Close()

	_, err := (AnthropicProvider{}).ChatCompletions(context.Background(), UpstreamChatCompletionRequest{
		Channel: Channel{ID: "channel-1", BaseURL: server.URL},
		APIKey:  "sk-ant-test",
		Request: ChatCompletionRequest{Model: "claude-sonnet-5", Messages: []ChatMessage{{Role: "user", Content: "hello"}}},
	})
	var upstreamErr *UpstreamError
	if !errors.As(err, &upstreamErr) || upstreamErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected status-preserving upstream error, got %T %v", err, err)
	}
}

func TestAnthropicProviderListsModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected model path: %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "sk-ant-test" {
			t.Fatalf("unexpected api key header: %q", r.Header.Get("x-api-key"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data":[
				{"id":"claude-z","display_name":"Claude Z","created_at":"2026-01-01T00:00:00Z","type":"model","max_input_tokens":1,"max_tokens":1,"capabilities":{}},
				{"id":"claude-a","display_name":"Claude A","created_at":"2026-01-01T00:00:00Z","type":"model","max_input_tokens":1,"max_tokens":1,"capabilities":{}}
			],
			"has_more":false
		}`))
	}))
	defer server.Close()

	models, err := (AnthropicProvider{}).ListModels(context.Background(), server.URL, "sk-ant-test")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].ID != "claude-a" || models[1].ID != "claude-z" {
		t.Fatalf("unexpected discovered models: %#v", models)
	}
	if models[0].DisplayName != "Claude A" || models[0].Provider != ProviderAnthropic {
		t.Fatalf("unexpected model metadata: %#v", models[0])
	}
}

func TestStopSequencesAcceptStringOrArray(t *testing.T) {
	var single StopSequences
	if err := json.Unmarshal([]byte(`"END"`), &single); err != nil {
		t.Fatal(err)
	}
	if len(single) != 1 || single[0] != "END" {
		t.Fatalf("unexpected single stop: %#v", single)
	}

	var many StopSequences
	if err := json.Unmarshal([]byte(`["A", " ", "B"]`), &many); err != nil {
		t.Fatal(err)
	}
	if len(many) != 2 || many[0] != "A" || many[1] != "B" {
		t.Fatalf("unexpected stop list: %#v", many)
	}
}

func TestAnthropicMessageParamsMoveSystemToTopLevel(t *testing.T) {
	maxTokens := int64(77)
	params, err := anthropicMessageParams(ChatCompletionRequest{
		Model:     "claude-sonnet-5",
		MaxTokens: &maxTokens,
		Stop:      StopSequences{"END"},
		Messages: []ChatMessage{
			{Role: "system", Content: "policy"},
			{Role: "developer", Content: "developer policy"},
			{Role: "user", Content: "hello"},
		},
	}, "claude-sonnet-5")
	if err != nil {
		t.Fatal(err)
	}
	if params.MaxTokens != maxTokens || len(params.StopSequences) != 1 {
		t.Fatalf("unexpected generation params: %#v", params)
	}
	if len(params.System) != 2 || params.System[0].Text != "policy" || params.System[1].Text != "developer policy" {
		t.Fatalf("unexpected system params: %#v", params.System)
	}
	if len(params.Messages) != 1 {
		t.Fatalf("unexpected message params: %#v", params.Messages)
	}
}

type fakeChannelRouter struct {
	channel         Channel
	candidates      []Channel
	err             error
	selectedModel   string
	discoveryConfig ChannelDiscoveryConfig
	credentialErr   error
	groupPolicy     GroupPolicy
	groupPolicyErr  error
	rpmAllowed      *bool
	rpmCalls        int
}

func (r *fakeChannelRouter) Select(_ context.Context, model string) (Channel, error) {
	r.selectedModel = model
	if r.err != nil {
		return Channel{}, r.err
	}
	return r.channel, nil
}

func (r *fakeChannelRouter) SelectCandidates(_ context.Context, model string) ([]Channel, error) {
	r.selectedModel = model
	if r.err != nil {
		return nil, r.err
	}
	if r.candidates != nil {
		return r.candidates, nil
	}
	return []Channel{r.channel}, nil
}

func (r *fakeChannelRouter) SelectCandidatesForGroup(ctx context.Context, model, _ string) ([]Channel, error) {
	return r.SelectCandidates(ctx, model)
}

func (r *fakeChannelRouter) ResolveGroupPolicy(_ context.Context, _ string) (GroupPolicy, error) {
	if r.groupPolicyErr != nil {
		return GroupPolicy{}, r.groupPolicyErr
	}
	return r.groupPolicy, nil
}

func (r *fakeChannelRouter) ConsumeGroupRPM(_ context.Context, _ string, _ int) (bool, error) {
	r.rpmCalls++
	if r.rpmAllowed == nil {
		return true, nil
	}
	return *r.rpmAllowed, nil
}

func TestServiceFailsOverAcrossPriorityTiers(t *testing.T) {
	provider := &recordingProvider{
		response: ChatCompletionResponse{
			Choices: []ChatCompletionChoice{{Message: ChatCompletionReply{Role: "assistant", Content: "from fallback"}}},
		},
		failByChannel: map[string]error{"channel-high": ErrUpstream},
	}
	router := &fakeChannelRouter{candidates: []Channel{
		{ID: "channel-high", Provider: ProviderOpenAI, CredentialRef: "env:HIGH", Priority: 100, Weight: 100},
		{ID: "channel-low", Provider: ProviderOpenAI, CredentialRef: "env:LOW", Priority: 10, Weight: 100},
	}}
	service, err := NewService(
		router,
		EnvCredentialResolver{Lookup: func(name string) string { return name }},
		map[string]Provider{ProviderOpenAI: provider},
	)
	if err != nil {
		t.Fatal(err)
	}

	response, err := service.ChatCompletions(context.Background(), &auth.Principal{
		Audience:      auth.AudienceRelay,
		AllowedModels: map[string]struct{}{"gpt-5": {}},
	}, ChatCompletionRequest{Model: "gpt-5", Messages: []ChatMessage{{Role: "user", Content: "hello"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.attempts) != 2 || provider.attempts[0] != "channel-high" || provider.attempts[1] != "channel-low" {
		t.Fatalf("unexpected failover attempts: %#v", provider.attempts)
	}
	if response.Choices[0].Message.Content != "from fallback" {
		t.Fatalf("unexpected fallback response: %#v", response)
	}
}

func TestServiceRetriesTransientStreamOpenWithoutDuplicateBilling(t *testing.T) {
	provider := &streamRecordingProvider{
		openErrors: []error{&UpstreamError{StatusCode: http.StatusServiceUnavailable, Err: ErrUpstream}},
		events: []ChatCompletionStreamEvent{
			{
				ID: "stream-success", Model: "gpt-5", Delta: "ok",
				HasUsage: true, Usage: ChatUsage{PromptTokens: 4, CompletionTokens: 2, TotalTokens: 6},
			},
		},
	}
	biller := &fakeBillingService{reservation: billing.Reservation{ID: "reservation-stream"}}
	service, err := NewService(
		&fakeChannelRouter{candidates: []Channel{{
			ID: "channel-stream", Provider: ProviderOpenAI, CredentialRef: "env:STREAM",
		}}},
		EnvCredentialResolver{Lookup: func(string) string { return "sk-stream" }},
		map[string]Provider{ProviderOpenAI: provider},
		biller,
	)
	if err != nil {
		t.Fatal(err)
	}

	var output strings.Builder
	err = service.StreamChatCompletions(context.Background(), &auth.Principal{
		ID: "token-stream", TokenID: "token-stream", TenantID: "tenant-stream",
		Audience: auth.AudienceRelay, AllowedModels: map[string]struct{}{"gpt-5": {}},
	}, ChatCompletionRequest{
		Model: "gpt-5", RequestID: "request-stream",
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	}, func(event ChatCompletionStreamEvent) error {
		output.WriteString(event.Delta)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider.openCalls != 2 {
		t.Fatalf("expected one transient stream retry, got %d opens", provider.openCalls)
	}
	if output.String() != "ok" {
		t.Fatalf("unexpected streamed output: %q", output.String())
	}
	if len(biller.events) != 2 || biller.events[0] != "reserve" || biller.events[1] != "settle" {
		t.Fatalf("stream retry must use one billing lifecycle: %#v", biller.events)
	}
	if biller.settledUsage.InputTokens != 4 || biller.settledUsage.OutputTokens != 2 {
		t.Fatalf("stream usage was not settled once: %#v", biller.settledUsage)
	}
}

func TestServiceStreamFailoverMovesToNextChannelForCredentialFailure(t *testing.T) {
	provider := &streamCandidateProvider{
		openErrors: map[string]error{
			"channel-invalid": &UpstreamError{StatusCode: http.StatusUnauthorized, Err: ErrUpstream},
		},
		events: map[string][]ChatCompletionStreamEvent{
			"channel-fallback": {{ID: "fallback-stream", Model: "gpt-5", Delta: "fallback"}},
		},
	}
	service, err := NewService(
		&fakeChannelRouter{candidates: []Channel{
			{ID: "channel-invalid", Provider: ProviderOpenAI, CredentialRef: "env:INVALID", Priority: 100, Weight: 100},
			{ID: "channel-fallback", Provider: ProviderOpenAI, CredentialRef: "env:FALLBACK", Priority: 10, Weight: 100},
		}},
		EnvCredentialResolver{Lookup: func(name string) string { return name }},
		map[string]Provider{ProviderOpenAI: provider},
	)
	if err != nil {
		t.Fatal(err)
	}

	var output strings.Builder
	err = service.StreamChatCompletions(context.Background(), &auth.Principal{
		Audience: auth.AudienceRelay, AllowedModels: map[string]struct{}{"gpt-5": {}},
	}, ChatCompletionRequest{
		Model: "gpt-5", Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	}, func(event ChatCompletionStreamEvent) error {
		output.WriteString(event.Delta)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if output.String() != "fallback" {
		t.Fatalf("unexpected fallback output: %q", output.String())
	}
	if len(provider.attempts) != 2 || provider.attempts[0] != "channel-invalid" || provider.attempts[1] != "channel-fallback" {
		t.Fatalf("credential failure must fail over without retrying the same channel: %#v", provider.attempts)
	}
}

func TestRetryableStreamErrorOnlyIncludesTransientFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "service unavailable", err: &UpstreamError{StatusCode: http.StatusServiceUnavailable, Err: ErrUpstream}, want: true},
		{name: "too many requests", err: &UpstreamError{StatusCode: http.StatusTooManyRequests, Err: ErrUpstream}, want: true},
		{name: "transport", err: &UpstreamError{Err: ErrUpstream}, want: true},
		{name: "authentication", err: &UpstreamError{StatusCode: http.StatusUnauthorized, Err: ErrUpstream}, want: false},
		{name: "not found", err: &UpstreamError{StatusCode: http.StatusNotFound, Err: ErrUpstream}, want: false},
		{name: "invalid request", err: ErrInvalidRequest, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := retryableStreamError(test.err); got != test.want {
				t.Fatalf("retryableStreamError() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestStreamFailureReason(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "http status",
			err:  &UpstreamError{StatusCode: http.StatusBadGateway, Err: ErrUpstream},
			want: "upstream_stream_failed_http_502",
		},
		{
			name: "transport",
			err:  &UpstreamError{Err: ErrUpstream},
			want: "upstream_stream_failed_transport",
		},
		{
			name: "timeout",
			err:  context.DeadlineExceeded,
			want: "upstream_stream_failed_timeout",
		},
		{
			name: "canceled",
			err:  context.Canceled,
			want: "upstream_stream_failed_canceled",
		},
		{
			name: "other",
			err:  ErrStreamingUnsupported,
			want: "upstream_stream_failed",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := streamFailureReason(test.err); got != test.want {
				t.Fatalf("streamFailureReason() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestChannelWideFailureClassification(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "model not found is mapping specific", err: &UpstreamError{StatusCode: http.StatusNotFound, Err: ErrUpstream}, want: false},
		{name: "rate limit is mapping specific", err: &UpstreamError{StatusCode: http.StatusTooManyRequests, Err: ErrUpstream}, want: false},
		{name: "authentication failure affects channel", err: &UpstreamError{StatusCode: http.StatusUnauthorized, Err: ErrUpstream}, want: true},
		{name: "transport failure affects channel", err: ErrUpstream, want: true},
		{name: "credential failure affects channel", err: ErrCredentialUnavailable, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := channelWideFailure(test.err); got != test.want {
				t.Fatalf("channelWideFailure() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestServiceFailoverUsesOneBillingReservation(t *testing.T) {
	provider := &recordingProvider{
		response:      ChatCompletionResponse{Usage: ChatUsage{PromptTokens: 3, CompletionTokens: 2}},
		failByChannel: map[string]error{"channel-high": ErrUpstream},
	}
	biller := &fakeBillingService{reservation: billing.Reservation{ID: "reservation-1"}}
	service, err := NewService(
		&fakeChannelRouter{candidates: []Channel{
			{ID: "channel-high", Provider: ProviderOpenAI, CredentialRef: "env:HIGH", Priority: 100, Weight: 100},
			{ID: "channel-low", Provider: ProviderOpenAI, CredentialRef: "env:LOW", Priority: 10, Weight: 100},
		}},
		EnvCredentialResolver{Lookup: func(name string) string { return name }},
		map[string]Provider{ProviderOpenAI: provider},
		biller,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ChatCompletions(context.Background(), &auth.Principal{
		ID:            "token-1",
		TokenID:       "token-1",
		TenantID:      "tenant-1",
		ProjectIDs:    map[string]struct{}{"project-1": {}},
		Audience:      auth.AudienceRelay,
		AllowedModels: map[string]struct{}{"gpt-5": {}},
	}, ChatCompletionRequest{Model: "gpt-5", RequestID: "request-1", IdempotencyKey: "request-1", Messages: []ChatMessage{{Role: "user", Content: "hello"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(biller.events) != 2 || biller.events[0] != "reserve" || biller.events[1] != "settle" {
		t.Fatalf("billing must reserve and settle once: %#v", biller.events)
	}
	if biller.reboundChannelID != "channel-low" {
		t.Fatalf("billing record must follow successful fallback channel: %q", biller.reboundChannelID)
	}
}

func TestServiceGroupRPMStopsBeforeUpstream(t *testing.T) {
	allowed := false
	provider := &recordingProvider{response: ChatCompletionResponse{}}
	router := &fakeChannelRouter{
		candidates:  []Channel{{ID: "channel-1", Provider: ProviderOpenAI, CredentialRef: "env:KEY", Priority: 100, Weight: 100}},
		groupPolicy: GroupPolicy{ID: "group-1", Status: "active", Multiplier: "1.000000", RPMLimit: 10, BillingType: "prepaid"},
		rpmAllowed:  &allowed,
	}
	service, err := NewService(router, EnvCredentialResolver{Lookup: func(string) string { return "sk-test" }}, map[string]Provider{ProviderOpenAI: provider})
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.ChatCompletions(context.Background(), &auth.Principal{
		GroupID:       "group-1",
		Audience:      auth.AudienceRelay,
		AllowedModels: map[string]struct{}{"gpt-5": {}},
	}, ChatCompletionRequest{Model: "gpt-5", Messages: []ChatMessage{{Role: "user", Content: "hello"}}})
	if !errors.Is(err, ErrGroupRateLimited) {
		t.Fatalf("expected group rate limit error, got %v", err)
	}
	if router.rpmCalls != 1 || provider.called {
		t.Fatalf("upstream must not be called after RPM rejection: calls=%d provider=%v", router.rpmCalls, provider.called)
	}
}

func TestServiceFreeGroupSkipsBilling(t *testing.T) {
	provider := &recordingProvider{response: ChatCompletionResponse{Choices: []ChatCompletionChoice{{Message: ChatCompletionReply{Content: "ok"}}}}}
	biller := &fakeBillingService{reservation: billing.Reservation{ID: "must-not-be-used"}}
	router := &fakeChannelRouter{
		candidates:  []Channel{{ID: "channel-1", Provider: ProviderOpenAI, CredentialRef: "env:KEY", Priority: 100, Weight: 100}},
		groupPolicy: GroupPolicy{ID: "group-free", Status: "active", Multiplier: "3.000000", BillingType: "free"},
	}
	service, err := NewService(router, EnvCredentialResolver{Lookup: func(string) string { return "sk-test" }}, map[string]Provider{ProviderOpenAI: provider}, biller)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ChatCompletions(context.Background(), &auth.Principal{
		GroupID:       "group-free",
		Audience:      auth.AudienceRelay,
		AllowedModels: map[string]struct{}{"gpt-5": {}},
	}, ChatCompletionRequest{Model: "gpt-5", Messages: []ChatMessage{{Role: "user", Content: "hello"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(biller.events) != 0 {
		t.Fatalf("free group must not create billing events: %#v", biller.events)
	}
}

func TestProbeModelUsesMinimalProviderCompatibleRequestWithoutBilling(t *testing.T) {
	tests := []struct {
		name                    string
		provider                string
		wantMaxTokens           bool
		wantMaxCompletionTokens bool
	}{
		{name: "openai", provider: ProviderOpenAI, wantMaxCompletionTokens: true},
		{name: "grok", provider: ProviderGrok, wantMaxTokens: true},
		{name: "anthropic", provider: ProviderAnthropic, wantMaxTokens: true},
		{name: "gemini", provider: ProviderGemini, wantMaxCompletionTokens: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := &recordingProvider{}
			biller := &fakeBillingService{reservation: billing.Reservation{ID: "must-not-be-used"}}
			service, err := NewService(
				&fakeChannelRouter{candidates: []Channel{{
					ID: "channel-1", Provider: test.provider, CredentialRef: "env:KEY", Priority: 100, Weight: 100,
				}}},
				EnvCredentialResolver{Lookup: func(string) string { return "sk-probe" }},
				map[string]Provider{test.provider: provider},
				biller,
			)
			if err != nil {
				t.Fatal(err)
			}

			if err := service.ProbeModel(context.Background(), "group-1", "gpt-5"); err != nil {
				t.Fatalf("ProbeModel() error = %v", err)
			}
			if len(biller.events) != 0 {
				t.Fatalf("probe must not create billing events: %#v", biller.events)
			}
			if len(provider.received.Request.Messages) != 1 || provider.received.Request.Messages[0].Content != "x" {
				t.Fatalf("probe request must use one-character input: %#v", provider.received.Request)
			}
			if provider.received.Request.Temperature != nil {
				t.Fatalf("probe must not set temperature: %#v", provider.received.Request)
			}
			if (provider.received.Request.MaxTokens != nil) != test.wantMaxTokens {
				t.Fatalf("max_tokens presence = %v, want %v", provider.received.Request.MaxTokens != nil, test.wantMaxTokens)
			}
			if (provider.received.Request.MaxCompletionTokens != nil) != test.wantMaxCompletionTokens {
				t.Fatalf("max_completion_tokens presence = %v, want %v", provider.received.Request.MaxCompletionTokens != nil, test.wantMaxCompletionTokens)
			}
		})
	}
}

func TestProbeModelSkipsUnsupportedMediaModel(t *testing.T) {
	provider := &recordingProvider{}
	service, err := NewService(
		&fakeChannelRouter{candidates: []Channel{{
			ID: "channel-1", Provider: ProviderOpenAI, CredentialRef: "env:KEY", Priority: 100, Weight: 100,
		}}},
		EnvCredentialResolver{Lookup: func(string) string { return "sk-probe" }},
		map[string]Provider{ProviderOpenAI: provider},
	)
	if err != nil {
		t.Fatal(err)
	}

	err = service.ProbeModel(context.Background(), "group-1", "dall-e-3")
	if !errors.Is(err, ErrUnsupportedFeature) {
		t.Fatalf("ProbeModel() error = %v, want ErrUnsupportedFeature", err)
	}
	if provider.called {
		t.Fatal("unsupported media model must not call the upstream provider")
	}
}

func (r *fakeChannelRouter) DiscoveryConfig(_ context.Context, _ string) (ChannelDiscoveryConfig, error) {
	if r.discoveryConfig.Provider == "" {
		return ChannelDiscoveryConfig{
			Provider:      ProviderOpenAI,
			BaseURL:       "https://api.openai.com/v1",
			CredentialRef: "secret:channel-1",
		}, nil
	}
	return r.discoveryConfig, nil
}

func TestServiceDiscoverModelsUsesStoredCredentialForExistingChannel(t *testing.T) {
	router := &fakeChannelRouter{
		discoveryConfig: ChannelDiscoveryConfig{
			Provider:      ProviderOpenAI,
			BaseURL:       "https://api.openai.com/v1",
			CredentialRef: "secret:channel-1",
		},
	}
	provider := &recordingProvider{
		models: []DiscoveredModel{{ID: "gpt-5", DisplayName: "gpt-5"}},
	}
	service, err := NewService(
		router,
		credentialResolverFunc(func(_ context.Context, ref string) (string, error) {
			if ref != "secret:channel-1" {
				t.Fatalf("unexpected credential ref: %q", ref)
			}
			return "sk-stored", nil
		}),
		map[string]Provider{ProviderOpenAI: provider},
	)
	if err != nil {
		t.Fatal(err)
	}

	models, err := service.DiscoverModels(context.Background(), ModelDiscoveryRequest{
		ChannelID: "channel-1",
		Provider:  ProviderOpenAI,
		BaseURL:   "https://api.openai.com/v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "gpt-5" {
		t.Fatalf("unexpected discovered models: %#v", models)
	}
	if provider.modelAPIKey != "sk-stored" {
		t.Fatalf("stored credential was not used: %q", provider.modelAPIKey)
	}
}

func TestServiceDiscoverModelsIgnoresExistingChannelEndpointFromRequest(t *testing.T) {
	router := &fakeChannelRouter{
		discoveryConfig: ChannelDiscoveryConfig{
			Provider:      ProviderOpenAI,
			BaseURL:       "https://api.openai.com/v1",
			CredentialRef: "secret:channel-1",
		},
	}
	provider := &recordingProvider{models: []DiscoveredModel{{ID: "gpt-5"}}}
	service, err := NewService(
		router,
		credentialResolverFunc(func(_ context.Context, ref string) (string, error) {
			if ref != "secret:channel-1" {
				t.Fatalf("unexpected credential ref: %q", ref)
			}
			return "sk-stored", nil
		}),
		map[string]Provider{ProviderOpenAI: provider},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.DiscoverModels(context.Background(), ModelDiscoveryRequest{
		ChannelID: "channel-1",
		Provider:  ProviderOpenAI,
		BaseURL:   "https://attacker.example.invalid/v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider.modelAPIKey != "sk-stored" {
		t.Fatalf("stored credential was not used: %q", provider.modelAPIKey)
	}
}

type recordingProvider struct {
	called        bool
	received      UpstreamChatCompletionRequest
	response      ChatCompletionResponse
	err           error
	failByChannel map[string]error
	attempts      []string
	models        []DiscoveredModel
	modelAPIKey   string
}

type streamRecordingProvider struct {
	openErrors []error
	events     []ChatCompletionStreamEvent
	openCalls  int
}

func (p *streamRecordingProvider) ChatCompletions(_ context.Context, _ UpstreamChatCompletionRequest) (ChatCompletionResponse, error) {
	return ChatCompletionResponse{}, ErrUnsupportedFeature
}

func (p *streamRecordingProvider) NewChatCompletionStream(_ context.Context, _ UpstreamChatCompletionRequest) (ChatCompletionStream, error) {
	p.openCalls++
	if p.openCalls <= len(p.openErrors) && p.openErrors[p.openCalls-1] != nil {
		return nil, p.openErrors[p.openCalls-1]
	}
	return &sliceChatCompletionStream{events: append([]ChatCompletionStreamEvent(nil), p.events...)}, nil
}

type sliceChatCompletionStream struct {
	events []ChatCompletionStreamEvent
	index  int
}

func (s *sliceChatCompletionStream) Recv() (ChatCompletionStreamEvent, error) {
	if s.index >= len(s.events) {
		return ChatCompletionStreamEvent{}, io.EOF
	}
	event := s.events[s.index]
	s.index++
	return event, nil
}

func (s *sliceChatCompletionStream) Close() error {
	return nil
}

type streamCandidateProvider struct {
	openErrors map[string]error
	events     map[string][]ChatCompletionStreamEvent
	attempts   []string
}

func (p *streamCandidateProvider) ChatCompletions(_ context.Context, _ UpstreamChatCompletionRequest) (ChatCompletionResponse, error) {
	return ChatCompletionResponse{}, ErrUnsupportedFeature
}

func (p *streamCandidateProvider) NewChatCompletionStream(_ context.Context, request UpstreamChatCompletionRequest) (ChatCompletionStream, error) {
	channelID := request.Channel.ID
	p.attempts = append(p.attempts, channelID)
	if err := p.openErrors[channelID]; err != nil {
		return nil, err
	}
	return &sliceChatCompletionStream{events: append([]ChatCompletionStreamEvent(nil), p.events[channelID]...)}, nil
}

type fakeBillingService struct {
	events           []string
	reservation      billing.Reservation
	reservedRequest  billing.Request
	settledUsage     billing.Usage
	reboundChannelID string
	reserveErr       error
	settleErr        error
	failErr          error
}

type pendingBillingService struct {
	*fakeBillingService
	pendingUsage billing.Usage
	pendingErr   error
}

func (b *pendingBillingService) MarkSettlementPending(_ context.Context, _ string, _ string) error {
	b.events = append(b.events, "pending")
	return nil
}

func (b *pendingBillingService) MarkSettlementPendingWithUsage(_ context.Context, _ string, _ string, usage billing.Usage, _ string) error {
	b.events = append(b.events, "pending_usage")
	b.pendingUsage = usage
	return b.pendingErr
}

func (b *fakeBillingService) Reserve(_ context.Context, request billing.Request) (billing.Reservation, error) {
	b.events = append(b.events, "reserve")
	b.reservedRequest = request
	return b.reservation, b.reserveErr
}

func (b *fakeBillingService) Settle(_ context.Context, _ string, usage billing.Usage, _ string) error {
	b.events = append(b.events, "settle")
	b.settledUsage = usage
	return b.settleErr
}

func (b *fakeBillingService) Fail(_ context.Context, _ string, _ string) error {
	b.events = append(b.events, "fail")
	return b.failErr
}

func (b *fakeBillingService) RebindReservationChannel(_ context.Context, _ string, channelID string) error {
	b.reboundChannelID = channelID
	return nil
}

func (p *recordingProvider) ChatCompletions(
	_ context.Context,
	request UpstreamChatCompletionRequest,
) (ChatCompletionResponse, error) {
	p.called = true
	p.received = request
	p.attempts = append(p.attempts, request.Channel.ID)
	if err, ok := p.failByChannel[request.Channel.ID]; ok {
		return ChatCompletionResponse{}, err
	}
	if p.err != nil {
		return ChatCompletionResponse{}, p.err
	}
	return p.response, nil
}

func (p *recordingProvider) ListModels(_ context.Context, _ string, apiKey string) ([]DiscoveredModel, error) {
	p.modelAPIKey = apiKey
	return p.models, p.err
}

type credentialResolverFunc func(context.Context, string) (string, error)

func (f credentialResolverFunc) Resolve(ctx context.Context, ref string) (string, error) {
	return f(ctx, ref)
}
