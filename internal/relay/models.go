package relay

import (
	"context"
	"sort"
	"strings"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	openai "github.com/openai/openai-go/v2"
	openaioption "github.com/openai/openai-go/v2/option"
)

const (
	maxDiscoveredModels = 500
	modelDiscoveryAgent = "ai-token-relay"
)

func (OpenAIProvider) ListModels(ctx context.Context, baseURL, apiKey string) ([]DiscoveredModel, error) {
	return listOpenAICompatibleModels(ctx, baseURL, apiKey, ProviderOpenAI)
}

func (GrokProvider) ListModels(ctx context.Context, baseURL, apiKey string) ([]DiscoveredModel, error) {
	return listOpenAICompatibleModels(ctx, baseURL, apiKey, ProviderGrok)
}

func listOpenAICompatibleModels(ctx context.Context, baseURL, apiKey, provider string) ([]DiscoveredModel, error) {
	opts := []openaioption.RequestOption{
		openaioption.WithAPIKey(strings.TrimSpace(apiKey)),
	}
	httpClient, err := providerHTTPClient(baseURL)
	if err != nil {
		return nil, ErrModelDiscoveryFailed
	}
	opts = append(opts, openaioption.WithHTTPClient(httpClient))
	if baseURL = strings.TrimSpace(baseURL); baseURL != "" {
		opts = append(opts, openaioption.WithBaseURL(baseURL))
	}
	// Some upstream WAF rules block the OpenAI SDK's generated User-Agent.
	// Keep the discovery request identifiable without exposing the SDK brand.
	opts = append(opts, openaioption.WithHeader("User-Agent", modelDiscoveryAgent))

	client := openai.NewClient(opts...)
	page, err := client.Models.List(ctx)
	if err != nil {
		return nil, ErrModelDiscoveryFailed
	}
	models := make([]DiscoveredModel, 0, len(page.Data))
	for _, model := range page.Data {
		id := strings.TrimSpace(model.ID)
		if id == "" {
			continue
		}
		models = append(models, DiscoveredModel{
			ID:          id,
			DisplayName: id,
			Provider:    provider,
		})
	}
	return normalizeDiscoveredModels(provider, models), nil
}

func (AnthropicProvider) ListModels(ctx context.Context, baseURL, apiKey string) ([]DiscoveredModel, error) {
	opts := []anthropicoption.RequestOption{
		anthropicoption.WithAPIKey(strings.TrimSpace(apiKey)),
	}
	httpClient, err := providerHTTPClient(baseURL)
	if err != nil {
		return nil, ErrModelDiscoveryFailed
	}
	opts = append(opts, anthropicoption.WithHTTPClient(httpClient))
	if baseURL = strings.TrimSpace(baseURL); baseURL != "" {
		opts = append(opts, anthropicoption.WithBaseURL(baseURL))
	}

	client := anthropic.NewClient(opts...)
	page, err := client.Models.List(ctx, anthropic.ModelListParams{})
	if err != nil {
		return nil, ErrModelDiscoveryFailed
	}
	models := make([]DiscoveredModel, 0, len(page.Data))
	for _, model := range page.Data {
		id := strings.TrimSpace(model.ID)
		if id == "" {
			continue
		}
		displayName := strings.TrimSpace(model.DisplayName)
		if displayName == "" {
			displayName = id
		}
		models = append(models, DiscoveredModel{
			ID:          id,
			DisplayName: displayName,
			Provider:    ProviderAnthropic,
		})
	}
	return normalizeDiscoveredModels(ProviderAnthropic, models), nil
}

func (GeminiProvider) ListModels(ctx context.Context, baseURL, apiKey string) ([]DiscoveredModel, error) {
	client, err := newGeminiClient(ctx, baseURL, apiKey)
	if err != nil {
		return nil, ErrModelDiscoveryFailed
	}
	page, err := client.Models.List(ctx, nil)
	if err != nil {
		return nil, geminiUpstreamError(err)
	}
	models := make([]DiscoveredModel, 0)
	for {
		for _, model := range page.Items {
			if model == nil {
				continue
			}
			id := strings.TrimSpace(strings.TrimPrefix(model.Name, "models/"))
			if id == "" {
				continue
			}
			displayName := strings.TrimSpace(model.DisplayName)
			if displayName == "" {
				displayName = id
			}
			models = append(models, DiscoveredModel{
				ID:          id,
				DisplayName: displayName,
				Provider:    ProviderGemini,
			})
			if len(models) >= maxDiscoveredModels {
				break
			}
		}
		if len(models) >= maxDiscoveredModels || page.NextPageToken == "" {
			break
		}
		page, err = page.Next(ctx)
		if err != nil {
			return nil, geminiUpstreamError(err)
		}
	}
	return normalizeDiscoveredModels(ProviderGemini, models), nil
}

func normalizeDiscoveredModels(provider string, models []DiscoveredModel) []DiscoveredModel {
	normalized := make([]DiscoveredModel, 0, len(models))
	seen := map[string]struct{}{}
	for _, model := range models {
		model.ID = strings.TrimSpace(model.ID)
		if model.ID == "" {
			continue
		}
		model.Provider = canonicalProvider(provider)
		model.DisplayName = strings.TrimSpace(model.DisplayName)
		if model.DisplayName == "" {
			model.DisplayName = model.ID
		}
		key := strings.ToLower(model.ID)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, model)
		if len(normalized) >= maxDiscoveredModels {
			break
		}
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		return strings.ToLower(normalized[i].ID) < strings.ToLower(normalized[j].ID)
	})
	return normalized
}
