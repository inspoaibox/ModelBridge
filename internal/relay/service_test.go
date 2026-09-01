package relay

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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
	biller := &fakeBillingService{reservation: billing.Reservation{ID: "reservation-1"}}
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
		AllowedModels: map[string]struct{}{"gpt-5": {}},
	}, ChatCompletionRequest{
		Model:          "gpt-5",
		RequestID:      "request-1",
		IdempotencyKey: "idempotency-1",
		Messages:       []ChatMessage{{Role: "user", Content: "hello"}},
	})
	if !errors.Is(err, ErrUsageUnavailable) {
		t.Fatalf("paid request without upstream usage must require reconciliation, got %v", err)
	}
	if len(biller.events) != 1 || biller.events[0] != "reserve" {
		t.Fatalf("paid request must retain its reservation for reconciliation: %#v", biller.events)
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
