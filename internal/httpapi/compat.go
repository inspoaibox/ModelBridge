package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ai-token/internal/auth"
	"ai-token/internal/models"
	"ai-token/internal/relay"
)

func relayModelListHandler(catalog models.Catalog) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if catalog == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "MODELS_UNAVAILABLE"})
			return
		}
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		if !auth.NetworkAllowlistAllows(principal, clientIP(r), r.Header.Get("Origin"), r.Header.Get("Referer")) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "TOKEN_NETWORK_NOT_ALLOWED"})
			return
		}
		items, err := catalog.ListPublic(r.Context())
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "MODELS_UNAVAILABLE"})
			return
		}
		data := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if !item.Available {
				continue
			}
			if len(principal.AllowedModels) > 0 {
				if _, allowed := principal.AllowedModels[item.Name]; !allowed {
					continue
				}
			}
			if principal.GroupID != "" && !modelInGroup(item, principal.GroupID) {
				continue
			}
			data = append(data, map[string]any{
				"id": item.Name, "object": "model", "created": int64(0), "owned_by": item.Provider,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
	})
}

func modelInGroup(item models.Summary, groupID string) bool {
	for _, itemGroupID := range item.GroupIDs {
		if itemGroupID == groupID {
			return true
		}
	}
	return false
}

type responsesPayload struct {
	Model           string          `json:"model"`
	Input           json.RawMessage `json:"input"`
	Instructions    string          `json:"instructions,omitempty"`
	MaxOutputTokens *int64          `json:"max_output_tokens,omitempty"`
	Temperature     *float64        `json:"temperature,omitempty"`
	TopP            *float64        `json:"top_p,omitempty"`
	Tools           json.RawMessage `json:"tools,omitempty"`
	ToolChoice      json.RawMessage `json:"tool_choice,omitempty"`
	Stream          bool            `json:"stream,omitempty"`
}

func relayResponsesHandler(service relay.ChatCompletionService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "RELAY_NOT_IMPLEMENTED"})
			return
		}
		var payload responsesPayload
		if err := decodeRelayJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		messages, err := responseInputMessages(payload.Input, payload.Instructions)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		metadata := relay.RequestMetadataFromContext(r.Context())
		request := relay.ChatCompletionRequest{
			Model: payload.Model, Messages: messages, MaxCompletionTokens: payload.MaxOutputTokens,
			Temperature: payload.Temperature, TopP: payload.TopP, Tools: payload.Tools, ToolChoice: payload.ToolChoice, Stream: payload.Stream,
			RequestID: metadata.RequestID, IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key")),
		}
		if request.IdempotencyKey == "" {
			request.IdempotencyKey = metadata.RequestID
		}
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		if !auth.NetworkAllowlistAllows(principal, clientIP(r), r.Header.Get("Origin"), r.Header.Get("Referer")) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "TOKEN_NETWORK_NOT_ALLOWED"})
			return
		}
		if payload.Stream {
			streamer, ok := service.(relay.StreamingChatCompletionService)
			if !ok {
				writeRelayError(w, relay.ErrStreamingUnsupported)
				return
			}
			started := false
			responseID := ""
			model := payload.Model
			var output strings.Builder
			emit := func(event relay.ChatCompletionStreamEvent) error {
				if event.ID != "" {
					responseID = event.ID
				}
				if event.Model != "" {
					model = event.Model
				}
				if responseID == "" {
					responseID = "resp_" + fmt.Sprint(time.Now().UnixNano())
				}
				if !started {
					if err := writeSSE(w, "response.created", map[string]any{"type": "response.created", "response": map[string]any{"id": responseID, "object": "response", "status": "in_progress", "model": model}}); err != nil {
						return err
					}
					started = true
				}
				if event.Delta != "" {
					output.WriteString(event.Delta)
					return writeSSE(w, "response.output_text.delta", map[string]any{"type": "response.output_text.delta", "item_id": responseID + "_output_0", "output_index": 0, "content_index": 0, "delta": event.Delta})
				}
				return nil
			}
			if err := streamer.StreamChatCompletions(relayRequestContext(r, "stream"), principal, request, emit); err != nil {
				if !started {
					writeRelayError(w, err)
				}
				return
			}
			if !started {
				_ = emit(relay.ChatCompletionStreamEvent{Model: model})
			}
			completed := map[string]any{"type": "response.completed", "response": map[string]any{
				"id": responseID, "object": "response", "status": "completed", "model": model,
				"output": []any{map[string]any{"id": responseID + "_output_0", "type": "message", "status": "completed", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": output.String(), "annotations": []any{}}}}},
			}}
			_ = writeSSE(w, "response.completed", completed)
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			return
		}
		response, err := service.ChatCompletions(r.Context(), principal, request)
		if err != nil {
			writeRelayError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id": response.ID, "object": "response", "created_at": response.Created,
			"status": "completed", "model": response.Model,
			"output": responseOutput(response),
			"usage":  map[string]int64{"input_tokens": response.Usage.PromptTokens, "output_tokens": response.Usage.CompletionTokens, "total_tokens": response.Usage.TotalTokens},
		})
	})
}

func responseInputMessages(raw json.RawMessage, instructions string) ([]relay.ChatMessage, error) {
	messages := make([]relay.ChatMessage, 0, 4)
	if strings.TrimSpace(instructions) != "" {
		messages = append(messages, relay.ChatMessage{Role: "system", Content: instructions})
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil, relay.ErrInvalidRequest
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return append(messages, relay.ChatMessage{Role: "user", Content: text}), nil
	}
	var items []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &items); err != nil || len(items) == 0 {
		return nil, relay.ErrInvalidRequest
	}
	for _, item := range items {
		var content string
		if err := json.Unmarshal(item.Content, &content); err == nil {
			if strings.TrimSpace(content) == "" {
				return nil, relay.ErrInvalidRequest
			}
			messages = append(messages, relay.ChatMessage{Role: item.Role, Content: content})
			continue
		}
		var parts []any
		if err := json.Unmarshal(item.Content, &parts); err != nil || len(parts) == 0 {
			return nil, relay.ErrInvalidRequest
		}
		normalized, err := normalizeResponseParts(parts)
		if err != nil {
			return nil, relay.ErrInvalidRequest
		}
		messages = append(messages, relay.ChatMessage{Role: item.Role, ContentParts: normalized})
	}
	return messages, nil
}

func normalizeResponseParts(parts []any) (json.RawMessage, error) {
	normalized := make([]map[string]any, 0, len(parts))
	for _, raw := range parts {
		value, ok := raw.(map[string]any)
		if !ok {
			return nil, relay.ErrInvalidRequest
		}
		typ, _ := value["type"].(string)
		switch typ {
		case "input_text":
			normalized = append(normalized, map[string]any{"type": "text", "text": value["text"]})
		case "input_image":
			urlValue := value["image_url"]
			if urlString, ok := urlValue.(string); ok {
				urlValue = map[string]any{"url": urlString}
			}
			normalized = append(normalized, map[string]any{"type": "image_url", "image_url": urlValue})
		case "text", "image_url":
			normalized = append(normalized, value)
		default:
			return nil, relay.ErrUnsupportedFeature
		}
	}
	return json.Marshal(normalized)
}

func responseOutput(response relay.ChatCompletionResponse) []map[string]any {
	output := make([]map[string]any, 0, len(response.Choices))
	for index, choice := range response.Choices {
		content := []map[string]any{}
		if choice.Message.Content != "" {
			content = append(content, map[string]any{"type": "output_text", "text": choice.Message.Content, "annotations": []any{}})
		}
		if len(choice.Message.ToolCalls) > 0 && string(choice.Message.ToolCalls) != "null" {
			var calls []map[string]any
			if json.Unmarshal(choice.Message.ToolCalls, &calls) == nil {
				for _, call := range calls {
					function, _ := call["function"].(map[string]any)
					content = append(content, map[string]any{"type": "function_call", "id": call["id"], "name": function["name"], "arguments": function["arguments"]})
				}
			}
		}
		output = append(output, map[string]any{
			"id": response.ID + "_output_" + strconv.Itoa(index), "type": "message", "status": "completed", "role": "assistant",
			"content": content,
		})
	}
	return output
}

type anthropicMessagesPayload struct {
	Model         string                    `json:"model"`
	System        json.RawMessage           `json:"system,omitempty"`
	Messages      []anthropicMessagePayload `json:"messages"`
	MaxTokens     *int64                    `json:"max_tokens,omitempty"`
	Temperature   *float64                  `json:"temperature,omitempty"`
	TopP          *float64                  `json:"top_p,omitempty"`
	StopSequences relay.StopSequences       `json:"stop_sequences,omitempty"`
	Tools         json.RawMessage           `json:"tools,omitempty"`
	ToolChoice    json.RawMessage           `json:"tool_choice,omitempty"`
	Metadata      json.RawMessage           `json:"metadata,omitempty"`
	Thinking      json.RawMessage           `json:"thinking,omitempty"`
	ServiceTier   string                    `json:"service_tier,omitempty"`
	Stream        bool                      `json:"stream,omitempty"`
}

type anthropicMessagePayload struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

func rawAnthropicJSONPresent(raw json.RawMessage) bool {
	value := strings.TrimSpace(string(raw))
	return value != "" && value != "null"
}

func anthropicContentMessage(role string, raw json.RawMessage) (relay.ChatMessage, error) {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		return relay.ChatMessage{}, errors.New("anthropic content is required")
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		if strings.TrimSpace(text) == "" {
			return relay.ChatMessage{}, errors.New("anthropic text content is empty")
		}
		return relay.ChatMessage{Role: role, Content: text}, nil
	}

	var blocks []map[string]any
	if err := json.Unmarshal(raw, &blocks); err != nil || len(blocks) == 0 {
		return relay.ChatMessage{}, errors.New("anthropic content blocks are invalid")
	}
	for _, block := range blocks {
		if block == nil {
			return relay.ChatMessage{}, errors.New("anthropic content block is invalid")
		}
		blockType, _ := block["type"].(string)
		if strings.TrimSpace(blockType) == "" {
			return relay.ChatMessage{}, errors.New("anthropic content block type is required")
		}
	}
	encoded, err := json.Marshal(blocks)
	if err != nil {
		return relay.ChatMessage{}, err
	}
	return relay.ChatMessage{Role: role, ContentParts: encoded}, nil
}

func relayAnthropicMessagesHandler(service relay.ChatCompletionService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "RELAY_NOT_IMPLEMENTED"})
			return
		}
		var payload anthropicMessagesPayload
		if err := decodeRelayJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request_error"})
			return
		}
		messages := make([]relay.ChatMessage, 0, len(payload.Messages)+1)
		if rawAnthropicJSONPresent(payload.System) {
			system, err := anthropicContentMessage("system", payload.System)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request_error"})
				return
			}
			messages = append(messages, system)
		}
		for _, item := range payload.Messages {
			message, err := anthropicContentMessage(item.Role, item.Content)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request_error"})
				return
			}
			messages = append(messages, message)
		}
		metadata := relay.RequestMetadataFromContext(r.Context())
		idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if idempotencyKey == "" {
			idempotencyKey = metadata.RequestID
		}
		request := relay.ChatCompletionRequest{
			Model: payload.Model, Messages: messages, MaxCompletionTokens: payload.MaxTokens,
			Temperature: payload.Temperature, TopP: payload.TopP, Stop: payload.StopSequences,
			Tools: payload.Tools, ToolChoice: payload.ToolChoice, Metadata: payload.Metadata,
			Thinking: payload.Thinking, ServiceTier: payload.ServiceTier, Stream: payload.Stream,
			RequestID: metadata.RequestID, IdempotencyKey: idempotencyKey,
		}
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication_error"})
			return
		}
		if !auth.NetworkAllowlistAllows(principal, clientIP(r), r.Header.Get("Origin"), r.Header.Get("Referer")) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "permission_error"})
			return
		}
		if payload.Stream {
			streamer, ok := service.(relay.StreamingChatCompletionService)
			if !ok {
				writeRelayError(w, relay.ErrStreamingUnsupported)
				return
			}
			started := false
			blockStarted := false
			toolBlockStarted := false
			messageID := ""
			model := payload.Model
			var finalUsage relay.ChatUsage
			hasFinalUsage := false
			finishReason := ""
			emit := func(event relay.ChatCompletionStreamEvent) error {
				if event.ID != "" {
					messageID = event.ID
				}
				if event.Model != "" {
					model = event.Model
				}
				if event.HasUsage {
					finalUsage = event.Usage
					hasFinalUsage = true
				}
				if event.FinishReason != "" {
					finishReason = event.FinishReason
				}
				if messageID == "" {
					messageID = "msg_" + fmt.Sprint(time.Now().UnixNano())
				}
				if !started {
					if err := writeSSE(w, "message_start", map[string]any{"type": "message_start", "message": map[string]any{"id": messageID, "type": "message", "role": "assistant", "model": model, "content": []any{}, "stop_reason": nil, "stop_sequence": nil, "usage": anthropicWireUsage(event.Usage)}}); err != nil {
						return err
					}
					started = true
				}
				if event.Delta != "" && !blockStarted {
					if err := writeSSE(w, "content_block_start", map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": ""}}); err != nil {
						return err
					}
					blockStarted = true
				}
				if event.Delta != "" {
					if err := writeSSE(w, "content_block_delta", map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": event.Delta}}); err != nil {
						return err
					}
				}
				if len(event.ToolCalls) > 0 && !toolBlockStarted {
					var calls []map[string]any
					if json.Unmarshal(event.ToolCalls, &calls) == nil && len(calls) > 0 {
						function, _ := calls[0]["function"].(map[string]any)
						if err := writeSSE(w, "content_block_start", map[string]any{"type": "content_block_start", "index": 1, "content_block": map[string]any{"type": "tool_use", "id": calls[0]["id"], "name": function["name"], "input": map[string]any{}}}); err != nil {
							return err
						}
						toolBlockStarted = true
					}
				}
				if len(event.FunctionCall) > 0 && string(event.FunctionCall) != "null" {
					var functionCall map[string]any
					if json.Unmarshal(event.FunctionCall, &functionCall) == nil {
						if arguments, ok := functionCall["arguments"].(string); ok && arguments != "" {
							if err := writeSSE(w, "content_block_delta", map[string]any{"type": "content_block_delta", "index": 1, "delta": map[string]any{"type": "input_json_delta", "partial_json": arguments}}); err != nil {
								return err
							}
						}
					}
				}
				return nil
			}
			if err := streamer.StreamChatCompletions(relayRequestContext(r, "stream"), principal, request, emit); err != nil {
				if !started {
					writeRelayError(w, err)
				}
				return
			}
			if !started {
				_ = emit(relay.ChatCompletionStreamEvent{Model: model})
			}
			if blockStarted {
				_ = writeSSE(w, "content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
			}
			if toolBlockStarted {
				_ = writeSSE(w, "content_block_stop", map[string]any{"type": "content_block_stop", "index": 1})
			}
			if finishReason == "" {
				finishReason = "end_turn"
			}
			usage := relay.ChatUsage{}
			if hasFinalUsage {
				usage = finalUsage
			}
			_ = writeSSE(w, "message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": anthropicWireStopReason(finishReason), "stop_sequence": nil}, "usage": anthropicWireUsage(usage)})
			_ = writeSSE(w, "message_stop", map[string]any{"type": "message_stop"})
			return
		}
		response, err := service.ChatCompletions(r.Context(), principal, request)
		if err != nil {
			writeRelayError(w, err)
			return
		}
		stopReason := "end_turn"
		if len(response.Choices) > 0 && response.Choices[0].FinishReason != "" {
			stopReason = response.Choices[0].FinishReason
		}
		text := ""
		content := []map[string]any{}
		if len(response.Choices) > 0 {
			text = response.Choices[0].Message.Content
			if text != "" {
				content = append(content, map[string]any{"type": "text", "text": text})
			}
			if len(response.Choices[0].Message.ToolCalls) > 0 {
				var calls []map[string]any
				if json.Unmarshal(response.Choices[0].Message.ToolCalls, &calls) == nil {
					for _, call := range calls {
						function, _ := call["function"].(map[string]any)
						arguments := function["arguments"]
						var input any
						if value, ok := arguments.(string); ok && json.Unmarshal([]byte(value), &input) == nil {
							arguments = input
						}
						content = append(content, map[string]any{"type": "tool_use", "id": call["id"], "name": function["name"], "input": arguments})
					}
				}
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id": response.ID, "type": "message", "role": "assistant", "model": response.Model,
			"content": content, "stop_reason": anthropicWireStopReason(stopReason),
			"stop_sequence": nil, "usage": anthropicWireUsage(response.Usage),
		})
	})
}

func writeSSE(w http.ResponseWriter, event string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, encoded); err != nil {
		return err
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

func anthropicWireUsage(usage relay.ChatUsage) map[string]any {
	inputTokens := usage.PromptTokens - usage.CacheCreationInputTokens - usage.CacheReadInputTokens
	if inputTokens < 0 {
		inputTokens = 0
	}
	result := map[string]any{"input_tokens": inputTokens, "output_tokens": usage.CompletionTokens}
	if usage.CacheCreationInputTokens > 0 {
		fiveMinute := usage.CacheCreationInputTokens - usage.CacheCreation1HInputTokens
		if fiveMinute < 0 {
			fiveMinute = 0
		}
		result["cache_creation_input_tokens"] = usage.CacheCreationInputTokens
		result["cache_creation"] = map[string]int64{"ephemeral_5m_input_tokens": fiveMinute, "ephemeral_1h_input_tokens": usage.CacheCreation1HInputTokens}
	}
	if usage.CacheReadInputTokens > 0 {
		result["cache_read_input_tokens"] = usage.CacheReadInputTokens
	}
	return result
}

// The relay service uses mostly OpenAI-compatible finish-reason names for
// providers. Anthropic clients require Anthropic's wire values, including
// preserving stop_sequence instead of collapsing it into end_turn.
func anthropicWireStopReason(reason string) string {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "", "end_turn":
		return "end_turn"
	case "stop":
		return "end_turn"
	case "stop_sequence":
		return "stop_sequence"
	case "length", "max_tokens":
		return "max_tokens"
	case "tool_calls", "function_call", "tool_use":
		return "tool_use"
	case "content_filter", "refusal":
		return "refusal"
	case "pause_turn":
		return "pause_turn"
	case "model_context_window_exceeded", "context_window_exceeded", "context_length_exceeded":
		return "model_context_window_exceeded"
	default:
		// Never emit an internal or unknown finish reason on an Anthropic
		// response. Anthropic clients expect a finite wire-level enum.
		return "end_turn"
	}
}
