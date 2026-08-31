package relay

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// The official SDK remains the source for model discovery. Completions use a
// typed HTTP adapter so newer OpenAI-compatible fields are preserved without
// silently dropping tools, response formats, or multimodal message content.
type OpenAIProvider struct{}

func (OpenAIProvider) ChatCompletions(ctx context.Context, upstream UpstreamChatCompletionRequest) (ChatCompletionResponse, error) {
	body, err := upstreamChatBody(upstream, false)
	if err != nil {
		return ChatCompletionResponse{}, err
	}
	responseBody, status, err := doOpenAIRequest(ctx, upstream.Channel.BaseURL, upstream.APIKey, "/chat/completions", body)
	if err != nil {
		return ChatCompletionResponse{}, err
	}
	if status < 200 || status >= 300 {
		return ChatCompletionResponse{}, &UpstreamError{StatusCode: status, Err: ErrUpstream}
	}
	return decodeOpenAICompletion(responseBody)
}

func (OpenAIProvider) NewChatCompletionStream(ctx context.Context, upstream UpstreamChatCompletionRequest) (ChatCompletionStream, error) {
	body, err := upstreamChatBody(upstream, true)
	if err != nil {
		return nil, err
	}
	request, err := newUpstreamRequest(ctx, upstream.Channel.BaseURL, upstream.APIKey, "/chat/completions", body)
	if err != nil {
		return nil, ErrInvalidRequest
	}
	request.Header.Set("Accept", "text/event-stream")
	client, err := providerHTTPClient(upstream.Channel.BaseURL)
	if err != nil {
		return nil, ErrInvalidRequest
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, &UpstreamError{Err: ErrUpstream}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return nil, &UpstreamError{StatusCode: response.StatusCode, Err: ErrUpstream}
	}
	return &openAICompletionStream{body: response.Body, scanner: newSSEScanner(response.Body)}, nil
}

func (OpenAIProvider) CreateEmbeddings(ctx context.Context, upstream UpstreamEmbeddingRequest) (EmbeddingResponse, error) {
	body, err := json.Marshal(struct {
		Model          string          `json:"model"`
		Input          json.RawMessage `json:"input"`
		EncodingFormat string          `json:"encoding_format,omitempty"`
		Dimensions     *int64          `json:"dimensions,omitempty"`
		User           string          `json:"user,omitempty"`
	}{
		Model: upstream.model(), Input: upstream.Request.Input,
		EncodingFormat: upstream.Request.EncodingFormat,
		Dimensions:     upstream.Request.Dimensions, User: upstream.Request.User,
	})
	if err != nil {
		return EmbeddingResponse{}, ErrInvalidRequest
	}
	responseBody, status, err := doOpenAIRequest(ctx, upstream.Channel.BaseURL, upstream.APIKey, "/embeddings", body)
	if err != nil {
		return EmbeddingResponse{}, err
	}
	if status < 200 || status >= 300 {
		return EmbeddingResponse{}, &UpstreamError{StatusCode: status, Err: ErrUpstream}
	}
	var value EmbeddingResponse
	if err := json.Unmarshal(responseBody, &value); err != nil || len(value.Data) == 0 {
		return EmbeddingResponse{}, ErrUpstream
	}
	if value.Object == "" {
		value.Object = "list"
	}
	return value, nil
}

func upstreamChatBody(upstream UpstreamChatCompletionRequest, stream bool) ([]byte, error) {
	request := upstream.Request
	request.Model = upstream.model()
	request.Stream = stream
	body, err := json.Marshal(request)
	if err != nil {
		return nil, ErrInvalidRequest
	}
	if !stream {
		return body, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, ErrInvalidRequest
	}
	streamOptions, _ := payload["stream_options"].(map[string]any)
	if streamOptions == nil {
		streamOptions = map[string]any{}
	}
	streamOptions["include_usage"] = true
	payload["stream_options"] = streamOptions
	return json.Marshal(payload)
}

func doOpenAIRequest(ctx context.Context, baseURL, apiKey, path string, body []byte) ([]byte, int, error) {
	request, err := newUpstreamRequest(ctx, baseURL, apiKey, path, body)
	if err != nil {
		return nil, 0, ErrInvalidRequest
	}
	client, err := providerHTTPClient(baseURL)
	if err != nil {
		return nil, 0, ErrInvalidRequest
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, 0, &UpstreamError{Err: ErrUpstream}
	}
	defer response.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if readErr != nil {
		return nil, response.StatusCode, &UpstreamError{StatusCode: response.StatusCode, Err: ErrUpstream}
	}
	return data, response.StatusCode, nil
}

func newUpstreamRequest(ctx context.Context, baseURL, apiKey, path string, body []byte) (*http.Request, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, ErrInvalidRequest
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + path
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	request.Header.Set("Content-Type", "application/json")
	return request, nil
}

type openAIWireChoice struct {
	Index   int64 `json:"index"`
	Message struct {
		Role         string          `json:"role"`
		Content      json.RawMessage `json:"content"`
		ToolCalls    json.RawMessage `json:"tool_calls"`
		FunctionCall json.RawMessage `json:"function_call"`
	} `json:"message"`
	FinishReason string `json:"finish_reason"`
}

type openAIWireUsage struct {
	PromptTokens        int64 `json:"prompt_tokens"`
	CompletionTokens    int64 `json:"completion_tokens"`
	TotalTokens         int64 `json:"total_tokens"`
	PromptTokensDetails struct {
		CachedTokens int64 `json:"cached_tokens"`
		AudioTokens  int64 `json:"audio_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionTokensDetails struct {
		ReasoningTokens int64 `json:"reasoning_tokens"`
		AudioTokens     int64 `json:"audio_tokens"`
	} `json:"completion_tokens_details"`
}

type openAIWireResponse struct {
	ID          string             `json:"id"`
	Object      string             `json:"object"`
	Created     int64              `json:"created"`
	Model       string             `json:"model"`
	Choices     []openAIWireChoice `json:"choices"`
	Usage       *openAIWireUsage   `json:"usage"`
	ServiceTier string             `json:"service_tier"`
}

func chatUsageFromWire(value openAIWireUsage) ChatUsage {
	return ChatUsage{
		PromptTokens: value.PromptTokens, CompletionTokens: value.CompletionTokens, TotalTokens: value.TotalTokens,
		PromptTokensDetails:     &ChatPromptTokensDetails{CachedTokens: value.PromptTokensDetails.CachedTokens, AudioTokens: value.PromptTokensDetails.AudioTokens},
		CompletionTokensDetails: &ChatCompletionTokensDetails{ReasoningTokens: value.CompletionTokensDetails.ReasoningTokens, AudioTokens: value.CompletionTokensDetails.AudioTokens},
		CacheReadInputTokens:    value.PromptTokensDetails.CachedTokens,
		UsageProvided:           true,
	}
}

func decodeOpenAICompletion(data []byte) (ChatCompletionResponse, error) {
	var value openAIWireResponse
	if err := json.Unmarshal(data, &value); err != nil || len(value.Choices) == 0 {
		return ChatCompletionResponse{}, ErrUpstream
	}
	choices := make([]ChatCompletionChoice, 0, len(value.Choices))
	for _, choice := range value.Choices {
		content, err := jsonText(choice.Message.Content)
		if err != nil {
			return ChatCompletionResponse{}, ErrUpstream
		}
		choices = append(choices, ChatCompletionChoice{
			Index:        choice.Index,
			Message:      ChatCompletionReply{Role: choice.Message.Role, Content: content, ToolCalls: choice.Message.ToolCalls, FunctionCall: choice.Message.FunctionCall},
			FinishReason: choice.FinishReason,
		})
	}
	if value.Object == "" {
		value.Object = "chat.completion"
	}
	usage := ChatUsage{}
	if value.Usage != nil {
		usage = chatUsageFromWire(*value.Usage)
	}
	usage.PricingTier = value.ServiceTier
	return ChatCompletionResponse{ID: value.ID, Object: value.Object, Created: value.Created, Model: value.Model, Choices: choices, Usage: usage}, nil
}

func jsonText(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text, nil
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", err
	}
	var result strings.Builder
	for _, part := range parts {
		if part.Type == "text" || part.Type == "output_text" || part.Type == "" {
			result.WriteString(part.Text)
		}
	}
	return result.String(), nil
}

type openAICompletionStream struct {
	body    io.ReadCloser
	scanner *bufio.Scanner
	closed  bool
}

func newSSEScanner(reader io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 2<<20)
	return scanner
}

func (s *openAICompletionStream) Recv() (ChatCompletionStreamEvent, error) {
	if s == nil || s.closed {
		return ChatCompletionStreamEvent{}, io.EOF
	}
	for s.scanner.Scan() {
		line := strings.TrimSpace(s.scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			return ChatCompletionStreamEvent{}, io.EOF
		}
		var value struct {
			ID          string `json:"id"`
			Object      string `json:"object"`
			Created     int64  `json:"created"`
			Model       string `json:"model"`
			ServiceTier string `json:"service_tier"`
			Choices     []struct {
				Index int64 `json:"index"`
				Delta struct {
					Role         string          `json:"role"`
					Content      json.RawMessage `json:"content"`
					ToolCalls    json.RawMessage `json:"tool_calls"`
					FunctionCall json.RawMessage `json:"function_call"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Usage *openAIWireUsage `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &value); err != nil {
			return ChatCompletionStreamEvent{}, ErrUpstream
		}
		event := ChatCompletionStreamEvent{ID: value.ID, Object: value.Object, Created: value.Created, Model: value.Model}
		if len(value.Choices) > 0 {
			choice := value.Choices[0]
			event.Index, event.Role = choice.Index, choice.Delta.Role
			text, err := jsonText(choice.Delta.Content)
			if err != nil {
				return ChatCompletionStreamEvent{}, ErrUpstream
			}
			event.Delta, event.ToolCalls, event.FunctionCall = text, choice.Delta.ToolCalls, choice.Delta.FunctionCall
			if choice.FinishReason != nil {
				event.FinishReason = *choice.FinishReason
			}
		}
		if value.Usage != nil {
			event.HasUsage, event.Usage = true, chatUsageFromWire(*value.Usage)
			event.Usage.PricingTier = value.ServiceTier
		}
		return event, nil
	}
	if err := s.scanner.Err(); err != nil {
		return ChatCompletionStreamEvent{}, &UpstreamError{Err: ErrUpstream}
	}
	return ChatCompletionStreamEvent{}, io.EOF
}

func (s *openAICompletionStream) Close() error {
	if s == nil || s.closed {
		return nil
	}
	s.closed = true
	return s.body.Close()
}
