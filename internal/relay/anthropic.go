package relay

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	anthropicparam "github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
)

type AnthropicProvider struct{}

func (AnthropicProvider) ChatCompletions(
	ctx context.Context,
	upstream UpstreamChatCompletionRequest,
) (ChatCompletionResponse, error) {
	body, err := anthropicRequestBody(upstream, false)
	if err != nil {
		return ChatCompletionResponse{}, err
	}
	responseBody, status, err := doAnthropicRequest(ctx, upstream.Channel.BaseURL, upstream.APIKey, body)
	if err != nil {
		return ChatCompletionResponse{}, err
	}
	if status < 200 || status >= 300 {
		return ChatCompletionResponse{}, &UpstreamError{StatusCode: status, Err: ErrUpstream}
	}
	return decodeAnthropicCompletion(responseBody)
}

func (AnthropicProvider) NewChatCompletionStream(ctx context.Context, upstream UpstreamChatCompletionRequest) (ChatCompletionStream, error) {
	body, err := anthropicRequestBody(upstream, true)
	if err != nil {
		return nil, err
	}
	request, err := newAnthropicRequest(ctx, upstream.Channel.BaseURL, upstream.APIKey, body)
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
	return &anthropicWireStream{body: response.Body, scanner: newSSEScanner(response.Body)}, nil
}

func anthropicRequestBody(upstream UpstreamChatCompletionRequest, stream bool) ([]byte, error) {
	if rawJSONPresent(upstream.Request.ResponseFormat) {
		return nil, ErrUnsupportedFeature
	}
	messages := make([]map[string]any, 0, len(upstream.Request.Messages))
	system := make([]map[string]any, 0)
	for _, message := range upstream.Request.Messages {
		role := normalizeRole(message.Role)
		content, err := anthropicMessageContent(message)
		if err != nil {
			return nil, err
		}
		if role == "system" || role == "developer" {
			switch value := content.(type) {
			case string:
				system = append(system, map[string]any{"type": "text", "text": value})
			case []map[string]any:
				for _, block := range value {
					system = append(system, block)
				}
			}
			continue
		}
		if role == "tool" {
			role = "user"
		}
		if role != "user" && role != "assistant" {
			return nil, ErrInvalidRequest
		}
		messages = append(messages, map[string]any{"role": role, "content": content})
	}
	if len(messages) == 0 {
		return nil, ErrInvalidRequest
	}
	payload := map[string]any{
		"model": upstream.model(), "max_tokens": upstream.Request.anthropicMaxTokens(),
		"messages": messages, "stream": stream,
	}
	if len(system) > 0 {
		payload["system"] = system
	}
	if upstream.Request.Temperature != nil {
		payload["temperature"] = *upstream.Request.Temperature
	}
	if upstream.Request.TopP != nil {
		payload["top_p"] = *upstream.Request.TopP
	}
	if len(upstream.Request.Stop) > 0 {
		payload["stop_sequences"] = []string(upstream.Request.Stop)
	}
	if strings.TrimSpace(upstream.Request.ServiceTier) != "" {
		payload["service_tier"] = strings.TrimSpace(upstream.Request.ServiceTier)
	}
	if rawJSONPresent(upstream.Request.Metadata) {
		var metadata map[string]any
		if err := json.Unmarshal(upstream.Request.Metadata, &metadata); err != nil {
			return nil, ErrInvalidRequest
		}
		payload["metadata"] = metadata
	}
	if rawJSONPresent(upstream.Request.Thinking) {
		var thinking map[string]any
		if err := json.Unmarshal(upstream.Request.Thinking, &thinking); err != nil {
			return nil, ErrInvalidRequest
		}
		payload["thinking"] = thinking
	}
	if rawJSONPresent(upstream.Request.Tools) {
		tools, err := anthropicTools(upstream.Request.Tools)
		if err != nil {
			return nil, err
		}
		payload["tools"] = tools
	}
	if rawJSONPresent(upstream.Request.Functions) {
		tools, err := anthropicLegacyFunctions(upstream.Request.Functions)
		if err != nil {
			return nil, err
		}
		payload["tools"] = tools
	}
	if rawJSONPresent(upstream.Request.ToolChoice) {
		choice, err := anthropicToolChoice(upstream.Request.ToolChoice)
		if err != nil {
			return nil, err
		}
		payload["tool_choice"] = choice
	}
	if rawJSONPresent(upstream.Request.FunctionCall) {
		choice, err := anthropicToolChoice(upstream.Request.FunctionCall)
		if err != nil {
			return nil, err
		}
		payload["tool_choice"] = choice
	}
	return json.Marshal(payload)
}

func anthropicMessageContent(message ChatMessage) (any, error) {
	if normalizeRole(message.Role) == "tool" {
		return []map[string]any{{"type": "tool_result", "tool_use_id": message.ToolCallID, "content": message.Content}}, nil
	}
	blocks := make([]map[string]any, 0)
	if len(message.ContentParts) > 0 && string(message.ContentParts) != "null" {
		var raw []map[string]any
		if err := json.Unmarshal(message.ContentParts, &raw); err != nil {
			return nil, ErrInvalidRequest
		}
		for _, block := range raw {
			blockType, _ := block["type"].(string)
			switch blockType {
			case "input_text":
				block["type"] = "text"
			case "input_image":
				block["type"] = "image"
				if source, ok := block["image_url"].(map[string]any); ok {
					block["source"] = map[string]any{"type": "url", "url": source["url"]}
					delete(block, "image_url")
				}
			case "image_url":
				block["type"] = "image"
				if source, ok := block["image_url"].(map[string]any); ok {
					block["source"] = map[string]any{"type": "url", "url": source["url"]}
					delete(block, "image_url")
				}
			}
			blocks = append(blocks, block)
		}
	}
	if strings.TrimSpace(message.Content) != "" {
		blocks = append(blocks, map[string]any{"type": "text", "text": message.Content})
	}
	if len(message.ToolCalls) > 0 && string(message.ToolCalls) != "null" {
		var calls []struct {
			ID       string `json:"id"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		}
		if err := json.Unmarshal(message.ToolCalls, &calls); err != nil {
			return nil, ErrInvalidRequest
		}
		for _, call := range calls {
			var input any
			if err := json.Unmarshal([]byte(call.Function.Arguments), &input); err != nil {
				return nil, ErrInvalidRequest
			}
			blocks = append(blocks, map[string]any{"type": "tool_use", "id": call.ID, "name": call.Function.Name, "input": input})
		}
	}
	if len(message.FunctionCall) > 0 && string(message.FunctionCall) != "null" {
		var call struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}
		if err := json.Unmarshal(message.FunctionCall, &call); err != nil {
			return nil, ErrInvalidRequest
		}
		var input any
		if err := json.Unmarshal([]byte(call.Arguments), &input); err != nil {
			return nil, ErrInvalidRequest
		}
		blocks = append(blocks, map[string]any{"type": "tool_use", "id": "toolu_legacy", "name": call.Name, "input": input})
	}
	if len(blocks) == 0 {
		return nil, ErrInvalidRequest
	}
	if len(blocks) == 1 && blocks[0]["type"] == "text" {
		return blocks[0]["text"], nil
	}
	return blocks, nil
}

func anthropicTools(raw json.RawMessage) ([]map[string]any, error) {
	var values []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, ErrInvalidRequest
	}
	result := make([]map[string]any, 0, len(values))
	for _, item := range values {
		var function struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Parameters  json.RawMessage `json:"parameters"`
		}
		_ = json.Unmarshal(item["function"], &function)
		if function.Name == "" {
			_ = json.Unmarshal(item["name"], &function.Name)
			_ = json.Unmarshal(item["description"], &function.Description)
			_ = json.Unmarshal(item["input_schema"], &function.Parameters)
			_ = json.Unmarshal(item["parameters"], &function.Parameters)
		}
		if function.Name == "" {
			return nil, ErrInvalidRequest
		}
		var schema any
		if len(function.Parameters) > 0 && json.Unmarshal(function.Parameters, &schema) != nil {
			return nil, ErrInvalidRequest
		}
		result = append(result, map[string]any{"name": function.Name, "description": function.Description, "input_schema": schema})
	}
	return result, nil
}

func anthropicLegacyFunctions(raw json.RawMessage) ([]map[string]any, error) {
	var values []struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	}
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, ErrInvalidRequest
	}
	result := make([]map[string]any, 0, len(values))
	for _, item := range values {
		var schema any
		if len(item.Parameters) > 0 && json.Unmarshal(item.Parameters, &schema) != nil {
			return nil, ErrInvalidRequest
		}
		result = append(result, map[string]any{"name": item.Name, "description": item.Description, "input_schema": schema})
	}
	return result, nil
}

func anthropicToolChoice(raw json.RawMessage) (map[string]any, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, ErrInvalidRequest
	}
	switch choice := value.(type) {
	case string:
		switch choice {
		case "auto":
			return map[string]any{"type": "auto"}, nil
		case "required":
			return map[string]any{"type": "any"}, nil
		case "none":
			return map[string]any{"type": "auto"}, nil
		}
	case map[string]any:
		if choiceType, ok := choice["type"].(string); ok {
			switch choiceType {
			case "auto":
				return map[string]any{"type": "auto"}, nil
			case "any":
				return map[string]any{"type": "any"}, nil
			case "tool":
				if name, ok := choice["name"].(string); ok && name != "" {
					return map[string]any{"type": "tool", "name": name}, nil
				}
			}
		}
		if name, ok := choice["name"].(string); ok && name != "" {
			return map[string]any{"type": "tool", "name": name}, nil
		}
		if function, ok := choice["function"].(map[string]any); ok {
			if name, ok := function["name"].(string); ok && name != "" {
				return map[string]any{"type": "tool", "name": name}, nil
			}
		}
	}
	return nil, ErrInvalidRequest
}

func doAnthropicRequest(ctx context.Context, baseURL, apiKey string, body []byte) ([]byte, int, error) {
	request, err := newAnthropicRequest(ctx, baseURL, apiKey, body)
	if err != nil {
		return nil, 0, err
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
	data, err := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if err != nil {
		return nil, response.StatusCode, &UpstreamError{StatusCode: response.StatusCode, Err: ErrUpstream}
	}
	return data, response.StatusCode, nil
}

func newAnthropicRequest(ctx context.Context, baseURL, apiKey string, body []byte) (*http.Request, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, ErrInvalidRequest
	}
	path := strings.TrimRight(parsed.Path, "/")
	if !strings.HasSuffix(path, "/v1") {
		path += "/v1"
	}
	parsed.Path = path + "/messages"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("x-api-key", strings.TrimSpace(apiKey))
	request.Header.Set("anthropic-version", "2023-06-01")
	request.Header.Set("Content-Type", "application/json")
	return request, nil
}

type anthropicWireStream struct {
	body    io.ReadCloser
	scanner *bufio.Scanner
	usage   ChatUsage
	id      string
	model   string
	closed  bool
}

func (s *anthropicWireStream) Recv() (ChatCompletionStreamEvent, error) {
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
		var value struct {
			Type    string `json:"type"`
			Message struct {
				ID    string `json:"id"`
				Model string `json:"model"`
				Usage struct {
					InputTokens              int64 `json:"input_tokens"`
					CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
					CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
					CacheCreation            struct {
						Ephemeral1HInputTokens int64 `json:"ephemeral_1h_input_tokens"`
						Ephemeral5MInputTokens int64 `json:"ephemeral_5m_input_tokens"`
					} `json:"cache_creation"`
					OutputTokensDetails struct {
						ThinkingTokens int64 `json:"thinking_tokens"`
					} `json:"output_tokens_details"`
					ServiceTier string `json:"service_tier"`
				} `json:"usage"`
			} `json:"message"`
			Index int64 `json:"index"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"`
				StopReason  string `json:"stop_reason"`
			} `json:"delta"`
			ContentBlock struct {
				Type  string `json:"type"`
				ID    string `json:"id"`
				Name  string `json:"name"`
				Input any    `json:"input"`
			} `json:"content_block"`
			Usage struct {
				InputTokens              int64 `json:"input_tokens"`
				OutputTokens             int64 `json:"output_tokens"`
				CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
				CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
				CacheCreation            struct {
					Ephemeral1HInputTokens int64 `json:"ephemeral_1h_input_tokens"`
					Ephemeral5MInputTokens int64 `json:"ephemeral_5m_input_tokens"`
				} `json:"cache_creation"`
				OutputTokensDetails struct {
					ThinkingTokens int64 `json:"thinking_tokens"`
				} `json:"output_tokens_details"`
				ServiceTier string `json:"service_tier"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &value); err != nil {
			return ChatCompletionStreamEvent{}, ErrUpstream
		}
		if value.Type == "error" {
			return ChatCompletionStreamEvent{}, ErrUpstream
		}
		if value.Message.ID != "" {
			s.id, s.model = value.Message.ID, value.Message.Model
			cacheCreation := anthropicCacheCreationTotal(value.Message.Usage.CacheCreationInputTokens, value.Message.Usage.CacheCreation.Ephemeral5MInputTokens, value.Message.Usage.CacheCreation.Ephemeral1HInputTokens)
			s.usage.PromptTokens = value.Message.Usage.InputTokens + cacheCreation + value.Message.Usage.CacheReadInputTokens
			s.usage.CacheCreationInputTokens = cacheCreation
			s.usage.CacheCreation1HInputTokens = minUsagePart(value.Message.Usage.CacheCreation.Ephemeral1HInputTokens, cacheCreation)
			s.usage.CacheReadInputTokens = value.Message.Usage.CacheReadInputTokens
			s.usage.PromptTokensDetails = &ChatPromptTokensDetails{CachedTokens: value.Message.Usage.CacheReadInputTokens}
			s.usage.CompletionTokensDetails = &ChatCompletionTokensDetails{ReasoningTokens: value.Message.Usage.OutputTokensDetails.ThinkingTokens}
			s.usage.PricingTier = value.Message.Usage.ServiceTier
		}
		event := ChatCompletionStreamEvent{ID: s.id, Model: s.model, Index: value.Index}
		switch value.Type {
		case "message_start":
			event.Role = "assistant"
			event.Usage, event.HasUsage = s.usage, true
		case "content_block_start":
			if value.ContentBlock.Type == "tool_use" {
				arguments, _ := json.Marshal(value.ContentBlock.Input)
				calls, _ := json.Marshal([]map[string]any{{"id": value.ContentBlock.ID, "type": "function", "function": map[string]any{"name": value.ContentBlock.Name, "arguments": string(arguments)}}})
				event.ToolCalls = calls
			}
		case "content_block_delta":
			if value.Delta.Type == "text_delta" {
				event.Delta = value.Delta.Text
			} else if value.Delta.Type == "input_json_delta" {
				event.FunctionCall, _ = json.Marshal(map[string]any{"arguments": value.Delta.PartialJSON})
			}
		case "message_delta":
			event.FinishReason = anthropicFinishReason(value.Delta.StopReason)
			if value.Usage.CacheCreationInputTokens > s.usage.CacheCreationInputTokens {
				s.usage.CacheCreationInputTokens = value.Usage.CacheCreationInputTokens
			}
			if value.Usage.CacheReadInputTokens > s.usage.CacheReadInputTokens {
				s.usage.CacheReadInputTokens = value.Usage.CacheReadInputTokens
			}
			if value.Usage.InputTokens > 0 {
				s.usage.PromptTokens = value.Usage.InputTokens + s.usage.CacheCreationInputTokens + s.usage.CacheReadInputTokens
			}
			s.usage.CompletionTokens = value.Usage.OutputTokens
			if value.Usage.OutputTokensDetails.ThinkingTokens > 0 {
				s.usage.CompletionTokensDetails = &ChatCompletionTokensDetails{ReasoningTokens: value.Usage.OutputTokensDetails.ThinkingTokens}
			}
			s.usage.TotalTokens = s.usage.PromptTokens + s.usage.CompletionTokens
			event.Usage, event.HasUsage = s.usage, true
		case "message_stop":
			event.FinishReason = "stop"
		default:
			continue
		}
		return event, nil
	}
	if err := s.scanner.Err(); err != nil {
		return ChatCompletionStreamEvent{}, &UpstreamError{Err: ErrUpstream}
	}
	return ChatCompletionStreamEvent{}, io.EOF
}

func (s *anthropicWireStream) Close() error {
	if s == nil || s.closed {
		return nil
	}
	s.closed = true
	return s.body.Close()
}

func decodeAnthropicCompletion(data []byte) (ChatCompletionResponse, error) {
	var value struct {
		ID         string `json:"id"`
		Model      string `json:"model"`
		StopReason string `json:"stop_reason"`
		Content    []struct {
			Type  string `json:"type"`
			Text  string `json:"text"`
			ID    string `json:"id"`
			Name  string `json:"name"`
			Input any    `json:"input"`
		} `json:"content"`
		Usage struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
			CacheCreation            struct {
				Ephemeral1HInputTokens int64 `json:"ephemeral_1h_input_tokens"`
				Ephemeral5MInputTokens int64 `json:"ephemeral_5m_input_tokens"`
			} `json:"cache_creation"`
			OutputTokensDetails struct {
				ThinkingTokens int64 `json:"thinking_tokens"`
			} `json:"output_tokens_details"`
			ServiceTier string `json:"service_tier"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &value); err != nil || len(value.Content) == 0 {
		return ChatCompletionResponse{}, ErrUpstream
	}
	var envelope map[string]json.RawMessage
	_ = json.Unmarshal(data, &envelope)
	usageProvided := len(envelope["usage"]) > 0 && string(envelope["usage"]) != "null"
	text := strings.Builder{}
	toolCalls := make([]map[string]any, 0)
	for _, block := range value.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
		if block.Type == "tool_use" {
			arguments, _ := json.Marshal(block.Input)
			toolCalls = append(toolCalls, map[string]any{"id": block.ID, "type": "function", "function": map[string]any{"name": block.Name, "arguments": string(arguments)}})
		}
	}
	var calls json.RawMessage
	if len(toolCalls) > 0 {
		calls, _ = json.Marshal(toolCalls)
	}
	cacheCreation := anthropicCacheCreationTotal(value.Usage.CacheCreationInputTokens, value.Usage.CacheCreation.Ephemeral5MInputTokens, value.Usage.CacheCreation.Ephemeral1HInputTokens)
	promptTokens := value.Usage.InputTokens + cacheCreation + value.Usage.CacheReadInputTokens
	return ChatCompletionResponse{
		ID: value.ID, Object: "chat.completion", Created: time.Now().Unix(), Model: value.Model,
		Choices: []ChatCompletionChoice{{Index: 0, Message: ChatCompletionReply{Role: "assistant", Content: text.String(), ToolCalls: calls}, FinishReason: anthropicFinishReason(value.StopReason)}},
		Usage:   ChatUsage{PromptTokens: promptTokens, CompletionTokens: value.Usage.OutputTokens, TotalTokens: promptTokens + value.Usage.OutputTokens, CacheCreationInputTokens: cacheCreation, CacheCreation1HInputTokens: minUsagePart(value.Usage.CacheCreation.Ephemeral1HInputTokens, cacheCreation), CacheReadInputTokens: value.Usage.CacheReadInputTokens, PromptTokensDetails: &ChatPromptTokensDetails{CachedTokens: value.Usage.CacheReadInputTokens}, CompletionTokensDetails: &ChatCompletionTokensDetails{ReasoningTokens: value.Usage.OutputTokensDetails.ThinkingTokens}, PricingTier: value.Usage.ServiceTier, UsageProvided: usageProvided},
	}, nil
}

func newAnthropicClient(ctx context.Context, baseURL, apiKey string) (*anthropic.Client, error) {
	opts := []anthropicoption.RequestOption{anthropicoption.WithAPIKey(strings.TrimSpace(apiKey))}
	httpClient, err := providerHTTPClient(baseURL)
	if err != nil {
		return nil, err
	}
	opts = append(opts, anthropicoption.WithHTTPClient(httpClient))
	if baseURL = strings.TrimSpace(baseURL); baseURL != "" {
		opts = append(opts, anthropicoption.WithBaseURL(baseURL))
	}
	client := anthropic.NewClient(opts...)
	return &client, nil
}

type anthropicCompletionStream struct {
	stream *ssestream.Stream[anthropic.MessageStreamEventUnion]
	usage  ChatUsage
	closed bool
}

func (s *anthropicCompletionStream) Recv() (ChatCompletionStreamEvent, error) {
	if s == nil || s.closed {
		return ChatCompletionStreamEvent{}, io.EOF
	}
	for s.stream.Next() {
		current := s.stream.Current()
		event := ChatCompletionStreamEvent{Index: current.Index}
		switch current.Type {
		case "message_start":
			start := current.AsMessageStart()
			event.ID = start.Message.ID
			event.Model = string(start.Message.Model)
			event.Role = "assistant"
			cacheCreation := anthropicCacheCreationTotal(start.Message.Usage.CacheCreationInputTokens, start.Message.Usage.CacheCreation.Ephemeral5mInputTokens, start.Message.Usage.CacheCreation.Ephemeral1hInputTokens)
			s.usage = ChatUsage{
				PromptTokens:               start.Message.Usage.InputTokens + cacheCreation + start.Message.Usage.CacheReadInputTokens,
				CacheCreationInputTokens:   cacheCreation,
				CacheCreation1HInputTokens: minUsagePart(start.Message.Usage.CacheCreation.Ephemeral1hInputTokens, cacheCreation),
				CacheReadInputTokens:       start.Message.Usage.CacheReadInputTokens,
				PromptTokensDetails:        &ChatPromptTokensDetails{CachedTokens: start.Message.Usage.CacheReadInputTokens},
				CompletionTokensDetails:    &ChatCompletionTokensDetails{ReasoningTokens: start.Message.Usage.OutputTokensDetails.ThinkingTokens},
				PricingTier:                string(start.Message.Usage.ServiceTier),
				UsageProvided:              true,
			}
			event.Usage, event.HasUsage = s.usage, true
		case "content_block_delta":
			delta := current.AsContentBlockDelta()
			event.Delta = delta.Delta.Text
		case "message_delta":
			delta := current.AsMessageDelta()
			event.FinishReason = anthropicFinishReason(string(delta.Delta.StopReason))
			if delta.Usage.CacheCreationInputTokens > s.usage.CacheCreationInputTokens {
				s.usage.CacheCreationInputTokens = delta.Usage.CacheCreationInputTokens
			}
			if delta.Usage.CacheReadInputTokens > s.usage.CacheReadInputTokens {
				s.usage.CacheReadInputTokens = delta.Usage.CacheReadInputTokens
			}
			if delta.Usage.InputTokens > 0 {
				s.usage.PromptTokens = delta.Usage.InputTokens + s.usage.CacheCreationInputTokens + s.usage.CacheReadInputTokens
			}
			s.usage.CompletionTokens = delta.Usage.OutputTokens
			s.usage.CompletionTokensDetails = &ChatCompletionTokensDetails{ReasoningTokens: delta.Usage.OutputTokensDetails.ThinkingTokens}
			s.usage.TotalTokens = s.usage.PromptTokens + delta.Usage.OutputTokens
			event.Usage, event.HasUsage = s.usage, true
		case "message_stop":
			event.FinishReason = "stop"
		default:
			continue
		}
		return event, nil
	}
	if err := s.stream.Err(); err != nil {
		return ChatCompletionStreamEvent{}, anthropicUpstreamError(err)
	}
	return ChatCompletionStreamEvent{}, io.EOF
}

func (s *anthropicCompletionStream) Close() error {
	if s == nil || s.closed {
		return nil
	}
	s.closed = true
	return s.stream.Close()
}

func anthropicUpstreamError(err error) error {
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		return &UpstreamError{StatusCode: apiErr.StatusCode, Err: ErrUpstream}
	}
	return &UpstreamError{Err: ErrUpstream}
}

func anthropicMessageParams(request ChatCompletionRequest, model string) (anthropic.MessageNewParams, error) {
	params := anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: request.anthropicMaxTokens(),
		Messages:  []anthropic.MessageParam{},
	}
	if request.Temperature != nil {
		params.Temperature = anthropicparam.NewOpt(*request.Temperature)
	}
	if request.TopP != nil {
		params.TopP = anthropicparam.NewOpt(*request.TopP)
	}
	if len(request.Stop) > 0 {
		params.StopSequences = []string(request.Stop)
	}

	for _, message := range request.Messages {
		if len(message.ContentParts) > 0 || len(message.ToolCalls) > 0 || len(message.FunctionCall) > 0 || normalizeRole(message.Role) == "tool" {
			return anthropic.MessageNewParams{}, ErrUnsupportedFeature
		}
		block := anthropic.NewTextBlock(message.Content)
		switch normalizeRole(message.Role) {
		case "system", "developer":
			params.System = append(params.System, anthropic.TextBlockParam{Text: message.Content})
		case "user":
			params.Messages = append(params.Messages, anthropic.NewUserMessage(block))
		case "assistant":
			params.Messages = append(params.Messages, anthropic.NewAssistantMessage(block))
		default:
			return anthropic.MessageNewParams{}, ErrInvalidRequest
		}
	}
	if len(params.Messages) == 0 {
		return anthropic.MessageNewParams{}, ErrInvalidRequest
	}
	return params, nil
}

func anthropicChatResponse(message *anthropic.Message) (ChatCompletionResponse, error) {
	if message == nil {
		return ChatCompletionResponse{}, ErrUpstream
	}
	cacheCreation := anthropicCacheCreationTotal(message.Usage.CacheCreationInputTokens, message.Usage.CacheCreation.Ephemeral5mInputTokens, message.Usage.CacheCreation.Ephemeral1hInputTokens)
	promptTokens := message.Usage.InputTokens +
		cacheCreation +
		message.Usage.CacheReadInputTokens
	completionTokens := message.Usage.OutputTokens
	return ChatCompletionResponse{
		ID:      message.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   string(message.Model),
		Choices: []ChatCompletionChoice{
			{
				Index: 0,
				Message: ChatCompletionReply{
					Role:    "assistant",
					Content: anthropicText(message.Content),
				},
				FinishReason: anthropicFinishReason(string(message.StopReason)),
			},
		},
		Usage: ChatUsage{
			PromptTokens:               promptTokens,
			CompletionTokens:           completionTokens,
			TotalTokens:                promptTokens + completionTokens,
			CacheCreationInputTokens:   cacheCreation,
			CacheCreation1HInputTokens: minUsagePart(message.Usage.CacheCreation.Ephemeral1hInputTokens, cacheCreation),
			CacheReadInputTokens:       message.Usage.CacheReadInputTokens,
			PromptTokensDetails: &ChatPromptTokensDetails{
				// Anthropic reports cache creation and cache reads separately.
				// Only cache reads are the cached-input subset; creation is billed
				// by its dedicated cache_creation meter below.
				CachedTokens: message.Usage.CacheReadInputTokens,
			},
			CompletionTokensDetails: &ChatCompletionTokensDetails{ReasoningTokens: message.Usage.OutputTokensDetails.ThinkingTokens},
			PricingTier:             string(message.Usage.ServiceTier),
			UsageProvided:           true,
		},
	}, nil
}

func anthropicText(blocks []anthropic.ContentBlockUnion) string {
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == "text" && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "")
}

func anthropicFinishReason(reason string) string {
	switch reason {
	case "end_turn", "stop_sequence":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	case "refusal", "model_context_window_exceeded":
		return "content_filter"
	default:
		return reason
	}
}

func anthropicCacheCreationTotal(total, fiveMinute, oneHour int64) int64 {
	if total < 0 {
		total = 0
	}
	if fiveMinute < 0 {
		fiveMinute = 0
	}
	if oneHour < 0 {
		oneHour = 0
	}
	if oneHour > total {
		total = oneHour
	}
	if fiveMinute > total-oneHour {
		total = fiveMinute + oneHour
	}
	return total
}

func minUsagePart(value, total int64) int64 {
	if value < 0 {
		return 0
	}
	if total >= 0 && value > total {
		return total
	}
	return value
}
