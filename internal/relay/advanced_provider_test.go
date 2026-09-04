package relay

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIProviderPreservesToolsAndParsesToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if _, ok := payload["tools"]; !ok {
			t.Fatal("tools were not forwarded")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-tool","model":"gpt-test","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call-1","type":"function","function":{"name":"lookup","arguments":"{\"q\":\"go\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14}}`))
	}))
	defer server.Close()

	response, err := (OpenAIProvider{}).ChatCompletions(context.Background(), UpstreamChatCompletionRequest{
		Channel: Channel{BaseURL: server.URL + "/v1"}, APIKey: "sk-test",
		Request: ChatCompletionRequest{
			Model: "gpt-test", Messages: []ChatMessage{{Role: "user", Content: "find"}},
			Tools: json.RawMessage(`[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}]`),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Choices) != 1 || response.Choices[0].FinishReason != "tool_calls" || !strings.Contains(string(response.Choices[0].Message.ToolCalls), "lookup") {
		t.Fatalf("unexpected tool response: %#v", response)
	}
}

func TestOpenAIProviderStreamsUsageAndDeltas(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-stream\",\"model\":\"gpt-test\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"hi\"},\"finish_reason\":null}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-stream\",\"model\":\"gpt-test\",\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2,\"total_tokens\":5}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	stream, err := (OpenAIProvider{}).NewChatCompletionStream(context.Background(), UpstreamChatCompletionRequest{
		Channel: Channel{BaseURL: server.URL + "/v1"}, APIKey: "sk-test",
		Request: ChatCompletionRequest{Model: "gpt-test", Messages: []ChatMessage{{Role: "user", Content: "hello"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	first, err := stream.Recv()
	if err != nil || first.Delta != "hi" || first.Role != "assistant" {
		t.Fatalf("unexpected first stream event: %#v %v", first, err)
	}
	second, err := stream.Recv()
	if err != nil || !second.HasUsage || second.Usage.TotalTokens != 5 {
		t.Fatalf("unexpected usage event: %#v %v", second, err)
	}
	if _, err := stream.Recv(); err != io.EOF {
		t.Fatalf("expected stream EOF, got %v", err)
	}
}

func TestOpenAIProviderPreservesResponseServiceTier(t *testing.T) {
	response, err := decodeOpenAICompletion([]byte(`{
		"id":"chatcmpl-tier","model":"gpt-test",
		"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
		"service_tier":"priority",
		"usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if response.Usage.PricingTier != "priority" {
		t.Fatalf("service tier was not preserved: %#v", response.Usage)
	}
}

func TestOpenAIProviderDoesNotInventUsageWhenFieldIsAbsent(t *testing.T) {
	response, err := decodeOpenAICompletion([]byte(`{
		"id":"chatcmpl-no-usage","model":"gpt-test",
		"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if response.Usage.UsageProvided || response.Usage.PromptTokens != 0 || response.Usage.CompletionTokens != 0 {
		t.Fatalf("absent OpenAI usage was treated as present: %#v", response.Usage)
	}
}

func TestAnthropicCacheCreationAndReadAreDisjoint(t *testing.T) {
	response, err := decodeAnthropicCompletion([]byte(`{
		"id":"msg-cache","model":"claude-test",
		"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn",
		"usage":{"input_tokens":10,"cache_creation_input_tokens":20,"cache_creation":{"ephemeral_5m_input_tokens":15,"ephemeral_1h_input_tokens":5},"cache_read_input_tokens":30,"output_tokens":4}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	usage := response.Usage
	if usage.PromptTokens != 60 || usage.CacheCreationInputTokens != 20 || usage.CacheCreation1HInputTokens != 5 || usage.CacheReadInputTokens != 30 {
		t.Fatalf("unexpected Anthropic cache usage: %#v", usage)
	}
	if usage.PromptTokensDetails == nil || usage.PromptTokensDetails.CachedTokens != 30 {
		t.Fatalf("cache read was not isolated: %#v", usage.PromptTokensDetails)
	}
}

func TestAnthropicProviderPreservesStopSequenceAndDoesNotOverwriteItOnMessageStop(t *testing.T) {
	response, err := decodeAnthropicCompletion([]byte(`{
		"id":"msg-stop-sequence","model":"claude-test",
		"content":[{"type":"text","text":"ok"}],"stop_reason":"stop_sequence",
		"usage":{"input_tokens":2,"output_tokens":1}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Choices) != 1 || response.Choices[0].FinishReason != "stop_sequence" {
		t.Fatalf("Anthropic stop_sequence was not preserved: %#v", response)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg-stream-stop-sequence\",\"model\":\"claude-test\",\"usage\":{\"input_tokens\":2}}}\n\n"))
		_, _ = w.Write([]byte("event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"stop_sequence\"},\"usage\":{\"output_tokens\":1}}\n\n"))
		_, _ = w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer server.Close()

	stream, err := (AnthropicProvider{}).NewChatCompletionStream(context.Background(), UpstreamChatCompletionRequest{
		Channel: Channel{BaseURL: server.URL}, APIKey: "sk-ant-test",
		Request: ChatCompletionRequest{Model: "claude-test", Messages: []ChatMessage{{Role: "user", Content: "hello"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	for index := 0; index < 2; index++ {
		if _, err := stream.Recv(); err != nil {
			t.Fatal(err)
		}
	}
	stopEvent, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if stopEvent.FinishReason != "stop_sequence" {
		t.Fatalf("message_stop overwrote Anthropic stop_sequence: %#v", stopEvent)
	}
}

func TestOpenAIProviderCreatesEmbeddings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","model":"text-embedding-test","data":[{"object":"embedding","index":0,"embedding":[0.1,0.2]}],"usage":{"prompt_tokens":4,"total_tokens":4}}`))
	}))
	defer server.Close()

	response, err := (OpenAIProvider{}).CreateEmbeddings(context.Background(), UpstreamEmbeddingRequest{
		Channel: Channel{BaseURL: server.URL + "/v1"}, APIKey: "sk-test",
		Request: EmbeddingRequest{Model: "text-embedding-test", Input: json.RawMessage(`"hello"`)},
	})
	if err != nil || len(response.Data) != 1 || len(response.Data[0].Embedding) != 2 || response.Usage.PromptTokens != 4 {
		t.Fatalf("unexpected embedding response: %#v %v", response, err)
	}
}

func TestAnthropicProviderConvertsToolsAndStreamsMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" || r.Header.Get("x-api-key") != "sk-ant-test" {
			t.Fatalf("unexpected Anthropic request: %s %q", r.URL.Path, r.Header.Get("x-api-key"))
		}
		if got := r.Header.Get("User-Agent"); got != relayUserAgent {
			t.Fatalf("unexpected Anthropic user-agent: %q", got)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if _, ok := payload["tools"]; !ok {
			t.Fatal("Anthropic tools were not forwarded")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg-tool","model":"claude-test","content":[{"type":"tool_use","id":"toolu-1","name":"lookup","input":{"q":"go"}}],"stop_reason":"tool_use","usage":{"input_tokens":5,"output_tokens":3}}`))
	}))
	defer server.Close()

	response, err := (AnthropicProvider{}).ChatCompletions(context.Background(), UpstreamChatCompletionRequest{
		Channel: Channel{BaseURL: server.URL}, APIKey: "sk-ant-test",
		Request: ChatCompletionRequest{
			Model: "claude-test", Messages: []ChatMessage{{Role: "user", Content: "find"}},
			Tools: json.RawMessage(`[{"name":"lookup","description":"find data","input_schema":{"type":"object"}}]`),
		},
	})
	if err != nil || len(response.Choices) != 1 || !strings.Contains(string(response.Choices[0].Message.ToolCalls), "lookup") {
		t.Fatalf("unexpected Anthropic tool response: %#v %v", response, err)
	}

	streamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg-stream\",\"model\":\"claude-test\",\"usage\":{\"input_tokens\":4}}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hi\"}}\n\n"))
		_, _ = w.Write([]byte("event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\n"))
		_, _ = w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer streamServer.Close()
	stream, err := (AnthropicProvider{}).NewChatCompletionStream(context.Background(), UpstreamChatCompletionRequest{
		Channel: Channel{BaseURL: streamServer.URL}, APIKey: "sk-ant-test",
		Request: ChatCompletionRequest{Model: "claude-test", Messages: []ChatMessage{{Role: "user", Content: "hello"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	_, err = stream.Recv()
	if err != nil {
		t.Fatal(err)
	}
	event, err := stream.Recv()
	if err != nil || event.Delta != "hi" {
		t.Fatalf("unexpected Anthropic stream event: %#v %v", event, err)
	}
}
