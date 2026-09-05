package models

import (
	"encoding/json"
	"testing"
)

func TestModelCategory(t *testing.T) {
	tests := []struct {
		name         string
		provider     string
		protocol     string
		capabilities map[string]any
		want         string
	}{
		{name: "gpt-5", provider: "openai", protocol: "openai_chat_completions", want: "text"},
		{name: "gpt-image-1", provider: "openai", protocol: "openai_images", want: "image"},
		{name: "veo-3", provider: "gemini", protocol: "gemini_generate_content", want: "video"},
		{name: "text-embedding-3-large", provider: "openai", protocol: "openai_embeddings", want: "embedding"},
		{name: "whisper-1", provider: "openai", protocol: "openai_audio", want: "audio"},
		{name: "custom-image", provider: "openai", protocol: "custom", want: "image", capabilities: map[string]any{"image_generation": true}},
	}
	for _, test := range tests {
		capabilities := test.capabilities
		if capabilities == nil {
			capabilities = map[string]any{}
		}
		if got := modelCategory(test.provider, test.name, test.protocol, capabilities); got != test.want {
			t.Fatalf("modelCategory(%q) = %q, want %q", test.name, got, test.want)
		}
	}
}

func TestMultiplyDecimal(t *testing.T) {
	if got := multiplyDecimal("1.25", "0.5"); got != "0.625" {
		t.Fatalf("multiplyDecimal() = %q, want 0.625", got)
	}
	if got := multiplyDecimal("10", "1.000000"); got != "10" {
		t.Fatalf("multiplyDecimal() = %q, want 10", got)
	}
}

func TestPlatformPricesExposeNonTokenMetering(t *testing.T) {
	raw, err := json.Marshal([]groupPriceBase{{GroupID: "group-1", GroupCode: "images", GroupName: "Images", Multiplier: "1.2", BillingType: "prepaid", MeteringMode: "image_count", MeteringPrice: "0.1"}})
	if err != nil {
		t.Fatal(err)
	}
	prices := platformPrices(raw, &Pricing{Currency: "USD", InputPricePerMillionTokens: "1", Components: []PriceComponent{{ComponentCode: "input_tokens", Unit: "token", PricePerUnit: "0.000001"}}})
	if len(prices) != 1 {
		t.Fatalf("platform price count = %d", len(prices))
	}
	if prices[0].MeteringMode != "image_count" || prices[0].MeteringUnit != "image" || prices[0].MeteringPrice != "0.1" || prices[0].MeteringPricePerUnit != "0.12" {
		t.Fatalf("unexpected non-token pricing: %#v", prices[0])
	}
}
