package modelprices

import (
	"encoding/json"
	"testing"
)

func TestFindPriceMatchesOnlyTheConfiguredProvider(t *testing.T) {
	source := map[string]sourceRecord{
		"gpt-5": {
			InputCostPerToken:  json.RawMessage("0.000001"),
			OutputCostPerToken: json.RawMessage("0.000008"),
			LiteLLMProvider:    "openai",
		},
		"openrouter/openai/gpt-5": {
			InputCostPerToken:  json.RawMessage("0.000002"),
			OutputCostPerToken: json.RawMessage("0.000009"),
			LiteLLMProvider:    "openrouter",
		},
	}

	price, ok := findPrice(source, "openai", "gpt-5")
	if !ok || price.SourceModelKey != "gpt-5" || price.Input != "0.000001" {
		t.Fatalf("unexpected matched price: %#v, %v", price, ok)
	}
	if _, ok := findPrice(source, "anthropic", "gpt-5"); ok {
		t.Fatal("price from another provider must not match")
	}
}

func TestFindPriceRequiresInputAndOutputCosts(t *testing.T) {
	source := map[string]sourceRecord{
		"gpt-incomplete": {
			InputCostPerToken: json.RawMessage("0.000001"),
			LiteLLMProvider:   "openai",
		},
	}
	if _, ok := findPrice(source, "openai", "gpt-incomplete"); ok {
		t.Fatal("incomplete price must not be imported")
	}
}

func TestTieredOnlyRecordGetsBasePricesFromFirstRange(t *testing.T) {
	record := sourceRecord{}
	if err := json.Unmarshal([]byte(`{
		"litellm_provider":"dashscope",
		"mode":"chat",
		"tiered_pricing":[
			{"input_cost_per_token":5e-8,"output_cost_per_token":4e-7,"range":[0,256000]},
			{"input_cost_per_token":2.5e-7,"output_cost_per_token":2e-6,"range":[256000,1000000]}
		]
	}`), &record); err != nil {
		t.Fatal(err)
	}
	price, ok := normalizeRecord("dashscope/qwen-flash", record)
	if !ok || price.Input != "0.00000005" || price.Output != "0.0000004" {
		t.Fatalf("tiered-only record was not normalized: %#v, %v", price, ok)
	}
	if len(price.Components) < 2 {
		t.Fatalf("expected input and output components: %#v", price.Components)
	}
}

func TestAnthropicOneHourCachePriceIsMapped(t *testing.T) {
	record := sourceRecord{}
	if err := json.Unmarshal([]byte(`{
		"litellm_provider":"anthropic",
		"mode":"chat",
		"input_cost_per_token":1e-6,
		"output_cost_per_token":5e-6,
		"cache_creation_input_token_cost":1.25e-6,
		"cache_creation_input_token_cost_above_1hr":2e-6
	}`), &record); err != nil {
		t.Fatal(err)
	}
	price, ok := normalizeRecord("claude-test", record)
	if !ok {
		t.Fatal("expected record to normalize")
	}
	for _, component := range price.Components {
		if component.Code == "cache_creation_1h_tokens" && component.Price == "0.000002" {
			return
		}
	}
	t.Fatalf("one-hour cache component was not mapped: %#v", price.Components)
}

func TestAnthropicOneHourCacheContextTierIsMapped(t *testing.T) {
	record := sourceRecord{}
	if err := json.Unmarshal([]byte(`{
		"litellm_provider":"anthropic",
		"mode":"chat",
		"input_cost_per_token":1e-6,
		"output_cost_per_token":5e-6,
		"cache_creation_input_token_cost":1.25e-6,
		"cache_creation_input_token_cost_above_1hr":2e-6,
		"cache_creation_input_token_cost_above_1hr_above_200k_tokens":4e-6
	}`), &record); err != nil {
		t.Fatal(err)
	}
	price, ok := normalizeRecord("claude-test", record)
	if !ok {
		t.Fatal("expected record to normalize")
	}
	for _, component := range price.Components {
		if component.Code != "cache_creation_1h_tokens" {
			continue
		}
		var tiers []map[string]string
		if err := json.Unmarshal(component.Tiers, &tiers); err != nil {
			t.Fatal(err)
		}
		if len(tiers) != 2 || tiers[0]["up_to"] != "200000" || tiers[1]["price_per_unit"] != "0.000004" {
			t.Fatalf("unexpected one-hour cache tiers: %#v", tiers)
		}
		return
	}
	t.Fatalf("one-hour cache component was not found: %#v", price.Components)
}

func TestTieredPricingKeepsProgressiveRanges(t *testing.T) {
	record := sourceRecord{}
	if err := json.Unmarshal([]byte(`{
		"litellm_provider":"dashscope","mode":"chat",
		"tiered_pricing":[
			{"input_cost_per_token":1e-6,"output_cost_per_token":2e-6,"range":[0,100000]},
			{"input_cost_per_token":2e-6,"output_cost_per_token":4e-6,"range":[100000,1000000]}
		]
	}`), &record); err != nil {
		t.Fatal(err)
	}
	price, ok := normalizeRecord("dashscope/test", record)
	if !ok {
		t.Fatal("expected tiered record to normalize")
	}
	for _, component := range price.Components {
		if component.Code != "input_tokens" {
			continue
		}
		var tiers []map[string]string
		if err := json.Unmarshal(component.Tiers, &tiers); err != nil {
			t.Fatal(err)
		}
		if len(tiers) == 2 && tiers[0]["up_to"] == "100000" && tiers[1]["price_per_unit"] == "0.000002" {
			var metadata map[string]string
			if err := json.Unmarshal(component.Metadata, &metadata); err != nil {
				t.Fatal(err)
			}
			if metadata["tier_basis"] == "input_tokens" && metadata["tier_mode"] == "context" {
				return
			}
		}
	}
	t.Fatalf("input tier component was not found: %#v", price.Components)
}
