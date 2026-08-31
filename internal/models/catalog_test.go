package models

import "testing"

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
