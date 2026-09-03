package relay

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"google.golang.org/genai"
)

type GeminiProvider struct{}

func newGeminiClient(ctx context.Context, baseURL, apiKey string) (*genai.Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	apiVersion := ""
	for _, version := range []string{"v1beta", "v1"} {
		suffix := "/" + version
		if strings.HasSuffix(baseURL, suffix) {
			baseURL = strings.TrimSuffix(baseURL, suffix)
			apiVersion = version
			break
		}
	}
	config := &genai.ClientConfig{
		APIKey:  strings.TrimSpace(apiKey),
		Backend: genai.BackendGeminiAPI,
	}
	httpClient, err := providerHTTPClient(baseURL)
	if err != nil {
		return nil, err
	}
	config.HTTPClient = httpClient
	if baseURL != "" || apiVersion != "" {
		config.HTTPOptions = genai.HTTPOptions{BaseURL: baseURL, APIVersion: apiVersion}
	}
	return genai.NewClient(ctx, config)
}

func (GeminiProvider) ChatCompletions(ctx context.Context, upstream UpstreamChatCompletionRequest) (ChatCompletionResponse, error) {
	client, err := newGeminiClient(ctx, upstream.Channel.BaseURL, upstream.APIKey)
	if err != nil {
		return ChatCompletionResponse{}, geminiUpstreamError(err)
	}
	contents, systemInstruction, err := geminiContents(upstream.Request.Messages)
	if err != nil {
		return ChatCompletionResponse{}, err
	}
	config, err := geminiGenerateConfig(upstream.Request, systemInstruction, upstream.model())
	if err != nil {
		return ChatCompletionResponse{}, err
	}
	response, err := client.Models.GenerateContent(ctx, strings.TrimPrefix(upstream.model(), "models/"), contents, config)
	if err != nil {
		return ChatCompletionResponse{}, geminiUpstreamError(err)
	}
	return geminiChatResponse(response, upstream.model())
}

func (GeminiProvider) NewChatCompletionStream(ctx context.Context, upstream UpstreamChatCompletionRequest) (ChatCompletionStream, error) {
	client, err := newGeminiClient(ctx, upstream.Channel.BaseURL, upstream.APIKey)
	if err != nil {
		return nil, geminiUpstreamError(err)
	}
	contents, systemInstruction, err := geminiContents(upstream.Request.Messages)
	if err != nil {
		return nil, err
	}
	config, err := geminiGenerateConfig(upstream.Request, systemInstruction, upstream.model())
	if err != nil {
		return nil, err
	}
	streamCtx, cancel := context.WithCancel(ctx)
	results := make(chan geminiStreamResult, 1)
	go func() {
		defer close(results)
		for response, streamErr := range client.Models.GenerateContentStream(streamCtx, strings.TrimPrefix(upstream.model(), "models/"), contents, config) {
			select {
			case results <- geminiStreamResult{response: response, err: streamErr}:
			case <-streamCtx.Done():
				return
			}
			if streamErr != nil {
				return
			}
		}
	}()
	return &geminiCompletionStream{results: results, cancel: cancel, requestedModel: upstream.model()}, nil
}

func geminiGenerateConfig(request ChatCompletionRequest, systemInstruction, model string) (*genai.GenerateContentConfig, error) {
	config := &genai.GenerateContentConfig{}
	if systemInstruction != "" {
		config.SystemInstruction = genai.NewContentFromText(systemInstruction, genai.RoleUser)
	}
	if request.Temperature != nil {
		value := float32(*request.Temperature)
		config.Temperature = &value
	}
	if request.TopP != nil {
		value := float32(*request.TopP)
		config.TopP = &value
	}
	if request.MaxCompletionTokens != nil {
		config.MaxOutputTokens = int32(*request.MaxCompletionTokens)
	} else if request.MaxTokens != nil {
		config.MaxOutputTokens = int32(*request.MaxTokens)
	}
	if request.ReasoningEffort == "none" && geminiThinkingProbeModel(model) {
		zero := int32(0)
		config.ThinkingConfig = &genai.ThinkingConfig{ThinkingBudget: &zero}
	}
	if len(request.Stop) > 0 {
		config.StopSequences = []string(request.Stop)
	}
	if rawJSONPresent(request.ResponseFormat) {
		var responseFormat map[string]any
		if err := json.Unmarshal(request.ResponseFormat, &responseFormat); err != nil {
			return nil, ErrInvalidRequest
		}
		format, _ := responseFormat["type"].(string)
		if format != "json_object" && format != "json_schema" {
			return nil, ErrUnsupportedFeature
		}
		config.ResponseMIMEType = "application/json"
		if schema, ok := responseFormat["json_schema"].(map[string]any); ok {
			config.ResponseJsonSchema = schema["schema"]
		}
	}
	if rawJSONPresent(request.Tools) {
		tools, err := geminiTools(request.Tools)
		if err != nil {
			return nil, err
		}
		config.Tools = tools
	}
	if rawJSONPresent(request.ToolChoice) {
		choice, err := geminiToolChoice(request.ToolChoice)
		if err != nil {
			return nil, err
		}
		config.ToolConfig = &genai.ToolConfig{FunctionCallingConfig: choice}
	}
	return config, nil
}

type geminiStreamResult struct {
	response *genai.GenerateContentResponse
	err      error
}

type geminiCompletionStream struct {
	results        <-chan geminiStreamResult
	cancel         context.CancelFunc
	requestedModel string
	closed         bool
}

func (s *geminiCompletionStream) Recv() (ChatCompletionStreamEvent, error) {
	if s == nil || s.closed {
		return ChatCompletionStreamEvent{}, io.EOF
	}
	result, ok := <-s.results
	if !ok {
		return ChatCompletionStreamEvent{}, io.EOF
	}
	if result.err != nil {
		return ChatCompletionStreamEvent{}, geminiUpstreamError(result.err)
	}
	response, err := geminiChatResponse(result.response, s.requestedModel)
	if err != nil {
		return ChatCompletionStreamEvent{}, err
	}
	event := ChatCompletionStreamEvent{ID: response.ID, Created: response.Created, Model: response.Model}
	if len(response.Choices) > 0 {
		event.Index = response.Choices[0].Index
		event.Role = "assistant"
		event.Delta = response.Choices[0].Message.Content
		event.FinishReason = response.Choices[0].FinishReason
	}
	if response.Usage.PromptTokens > 0 || response.Usage.CompletionTokens > 0 {
		event.HasUsage, event.Usage = true, response.Usage
	}
	return event, nil
}

func (s *geminiCompletionStream) Close() error {
	if s == nil || s.closed {
		return nil
	}
	s.closed = true
	s.cancel()
	return nil
}

func (GeminiProvider) CreateEmbeddings(ctx context.Context, upstream UpstreamEmbeddingRequest) (EmbeddingResponse, error) {
	client, err := newGeminiClient(ctx, upstream.Channel.BaseURL, upstream.APIKey)
	if err != nil {
		return EmbeddingResponse{}, geminiUpstreamError(err)
	}
	inputs, err := embeddingInputStrings(upstream.Request.Input)
	if err != nil || len(inputs) == 0 {
		return EmbeddingResponse{}, ErrInvalidRequest
	}
	contents := make([]*genai.Content, 0, len(inputs))
	for _, input := range inputs {
		contents = append(contents, genai.NewContentFromText(input, genai.RoleUser))
	}
	config := &genai.EmbedContentConfig{}
	if upstream.Request.Dimensions != nil {
		value := int32(*upstream.Request.Dimensions)
		config.OutputDimensionality = &value
	}
	response, err := client.Models.EmbedContent(ctx, strings.TrimPrefix(upstream.model(), "models/"), contents, config)
	if err != nil {
		return EmbeddingResponse{}, geminiUpstreamError(err)
	}
	if response == nil || len(response.Embeddings) == 0 {
		return EmbeddingResponse{}, ErrUpstream
	}
	data := make([]EmbeddingData, 0, len(response.Embeddings))
	var tokenCount int64
	for index, item := range response.Embeddings {
		if item == nil {
			continue
		}
		vector := make([]float64, len(item.Values))
		for i, value := range item.Values {
			vector[i] = float64(value)
		}
		if item.Statistics != nil {
			tokenCount += int64(item.Statistics.TokenCount)
		}
		data = append(data, EmbeddingData{Object: "embedding", Embedding: vector, Index: int64(index)})
	}
	if len(data) == 0 {
		return EmbeddingResponse{}, ErrUpstream
	}
	return EmbeddingResponse{Object: "list", Data: data, Model: upstream.model(), Usage: EmbeddingUsage{PromptTokens: tokenCount, TotalTokens: tokenCount}}, nil
}

func embeddingInputStrings(raw json.RawMessage) ([]string, error) {
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		if strings.TrimSpace(single) == "" {
			return nil, ErrInvalidRequest
		}
		return []string{single}, nil
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err != nil {
		return nil, ErrInvalidRequest
	}
	for _, value := range many {
		if strings.TrimSpace(value) == "" {
			return nil, ErrInvalidRequest
		}
	}
	return many, nil
}

func geminiContents(messages []ChatMessage) ([]*genai.Content, string, error) {
	contents := make([]*genai.Content, 0, len(messages))
	systemParts := make([]string, 0)
	for _, message := range messages {
		content := strings.TrimSpace(message.Content)
		switch normalizeRole(message.Role) {
		case "system", "developer":
			if len(message.ContentParts) > 0 {
				return nil, "", ErrUnsupportedFeature
			}
			if content == "" {
				return nil, "", ErrInvalidRequest
			}
			systemParts = append(systemParts, content)
		case "user":
			converted, err := geminiMessageContent(message, genai.RoleUser)
			if err != nil {
				return nil, "", err
			}
			contents = append(contents, converted)
		case "assistant":
			converted, err := geminiMessageContent(message, genai.RoleModel)
			if err != nil {
				return nil, "", err
			}
			contents = append(contents, converted)
		case "tool":
			if strings.TrimSpace(message.ToolCallID) == "" {
				return nil, "", ErrInvalidRequest
			}
			functionName := message.Name
			if functionName == "" {
				functionName = message.ToolCallID
			}
			contents = append(contents, genai.NewContentFromFunctionResponse(functionName, map[string]any{"content": content}, genai.RoleUser))
		default:
			return nil, "", ErrInvalidRequest
		}
	}
	if len(contents) == 0 {
		return nil, "", ErrInvalidRequest
	}
	return contents, strings.Join(systemParts, "\n\n"), nil
}

func geminiMessageContent(message ChatMessage, role genai.Role) (*genai.Content, error) {
	parts := make([]*genai.Part, 0)
	if len(message.ContentParts) > 0 && string(message.ContentParts) != "null" {
		var raw []map[string]any
		if err := json.Unmarshal(message.ContentParts, &raw); err != nil {
			return nil, ErrInvalidRequest
		}
		for _, item := range raw {
			typ, _ := item["type"].(string)
			switch typ {
			case "text", "input_text":
				text, _ := item["text"].(string)
				parts = append(parts, genai.NewPartFromText(text))
			case "image_url":
				image, _ := item["image_url"].(map[string]any)
				value, _ := image["url"].(string)
				part, err := geminiMediaPart(value, "image")
				if err != nil {
					return nil, err
				}
				parts = append(parts, part)
			case "input_image":
				value, _ := item["image_url"].(string)
				if image, ok := item["image_url"].(map[string]any); ok {
					value, _ = image["url"].(string)
				}
				part, err := geminiMediaPart(value, "image")
				if err != nil {
					return nil, err
				}
				parts = append(parts, part)
			case "image":
				source, _ := item["source"].(map[string]any)
				value, _ := source["url"].(string)
				if value == "" {
					value, _ = source["data"].(string)
				}
				part, err := geminiMediaPart(value, "image")
				if err != nil {
					return nil, err
				}
				parts = append(parts, part)
			default:
				return nil, ErrUnsupportedFeature
			}
		}
	}
	if content := strings.TrimSpace(message.Content); content != "" {
		parts = append(parts, genai.NewPartFromText(content))
	}
	if len(message.ToolCalls) > 0 && string(message.ToolCalls) != "null" {
		var calls []struct {
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		}
		if err := json.Unmarshal(message.ToolCalls, &calls); err != nil {
			return nil, ErrInvalidRequest
		}
		for _, call := range calls {
			var args map[string]any
			if json.Unmarshal([]byte(call.Function.Arguments), &args) != nil {
				return nil, ErrInvalidRequest
			}
			parts = append(parts, genai.NewPartFromFunctionCall(call.Function.Name, args))
		}
	}
	if len(parts) == 0 {
		return nil, ErrInvalidRequest
	}
	return genai.NewContentFromParts(parts, role), nil
}

func geminiMediaPart(value, defaultType string) (*genai.Part, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "data:") {
		comma := strings.IndexByte(value, ',')
		if comma <= 5 {
			return nil, ErrInvalidRequest
		}
		meta, encoded := value[5:comma], value[comma+1:]
		mimeType := strings.TrimSuffix(strings.Split(meta, ";")[0], ";base64")
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, ErrInvalidRequest
		}
		return genai.NewPartFromBytes(data, mimeType), nil
	}
	if value == "" {
		return nil, ErrInvalidRequest
	}
	return genai.NewPartFromURI(value, defaultType+"/*"), nil
}

func geminiTools(raw json.RawMessage) ([]*genai.Tool, error) {
	var values []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, ErrInvalidRequest
	}
	declarations := make([]*genai.FunctionDeclaration, 0, len(values))
	for _, value := range values {
		var function struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Parameters  json.RawMessage `json:"parameters"`
		}
		_ = json.Unmarshal(value["function"], &function)
		if function.Name == "" {
			_ = json.Unmarshal(value["name"], &function.Name)
			_ = json.Unmarshal(value["description"], &function.Description)
			_ = json.Unmarshal(value["input_schema"], &function.Parameters)
			_ = json.Unmarshal(value["parameters"], &function.Parameters)
		}
		if function.Name == "" {
			return nil, ErrInvalidRequest
		}
		var schema *genai.Schema
		if len(function.Parameters) > 0 && string(function.Parameters) != "null" {
			schema = &genai.Schema{}
			if err := json.Unmarshal(function.Parameters, schema); err != nil {
				return nil, ErrInvalidRequest
			}
		}
		declarations = append(declarations, &genai.FunctionDeclaration{Name: function.Name, Description: function.Description, Parameters: schema})
	}
	return []*genai.Tool{{FunctionDeclarations: declarations}}, nil
}

func geminiToolChoice(raw json.RawMessage) (*genai.FunctionCallingConfig, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, ErrInvalidRequest
	}
	if choice, ok := value.(string); ok {
		switch choice {
		case "auto":
			return &genai.FunctionCallingConfig{Mode: genai.FunctionCallingConfigModeAuto}, nil
		case "required":
			return &genai.FunctionCallingConfig{Mode: genai.FunctionCallingConfigModeAny}, nil
		case "none":
			return &genai.FunctionCallingConfig{Mode: genai.FunctionCallingConfigModeNone}, nil
		default:
			return nil, ErrInvalidRequest
		}
	}
	choice, ok := value.(map[string]any)
	if !ok {
		return nil, ErrInvalidRequest
	}
	typeValue, _ := choice["type"].(string)
	if typeValue == "auto" || typeValue == "none" || typeValue == "any" {
		mode := genai.FunctionCallingConfigModeAuto
		if typeValue == "none" {
			mode = genai.FunctionCallingConfigModeNone
		}
		if typeValue == "any" {
			mode = genai.FunctionCallingConfigModeAny
		}
		return &genai.FunctionCallingConfig{Mode: mode}, nil
	}
	if typeValue == "function" || typeValue == "tool" {
		name := ""
		if function, ok := choice["function"].(map[string]any); ok {
			name, _ = function["name"].(string)
		}
		if name == "" {
			name, _ = choice["name"].(string)
		}
		if name == "" {
			return nil, ErrInvalidRequest
		}
		return &genai.FunctionCallingConfig{Mode: genai.FunctionCallingConfigModeAny, AllowedFunctionNames: []string{name}}, nil
	}
	return nil, ErrInvalidRequest
}

func geminiChatResponse(response *genai.GenerateContentResponse, requestedModel string) (ChatCompletionResponse, error) {
	if response == nil || len(response.Candidates) == 0 {
		return ChatCompletionResponse{}, ErrUpstream
	}
	candidate := response.Candidates[0]
	if candidate == nil {
		return ChatCompletionResponse{}, ErrUpstream
	}
	model := strings.TrimSpace(response.ModelVersion)
	if model == "" {
		model = strings.TrimSpace(requestedModel)
	}
	id := strings.TrimSpace(response.ResponseID)
	if id == "" {
		id = fmt.Sprintf("gemini-%d", time.Now().UnixNano())
	}
	created := time.Now().Unix()
	if !response.CreateTime.IsZero() {
		created = response.CreateTime.Unix()
	}
	usage := ChatUsage{}
	if response.UsageMetadata != nil {
		usage.UsageProvided = true
		usage.PromptTokens = int64(response.UsageMetadata.PromptTokenCount + response.UsageMetadata.ToolUsePromptTokenCount)
		usage.CompletionTokens = int64(response.UsageMetadata.CandidatesTokenCount + response.UsageMetadata.ThoughtsTokenCount)
		usage.TotalTokens = int64(response.UsageMetadata.TotalTokenCount)
		if usage.TotalTokens == 0 {
			usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		}
		usage.PromptTokensDetails = &ChatPromptTokensDetails{CachedTokens: int64(response.UsageMetadata.CachedContentTokenCount)}
		usage.CompletionTokensDetails = &ChatCompletionTokensDetails{ReasoningTokens: int64(response.UsageMetadata.ThoughtsTokenCount)}
		for _, detail := range response.UsageMetadata.PromptTokensDetails {
			addGeminiPromptTokens(usage.PromptTokensDetails, detail, false)
		}
		for _, detail := range response.UsageMetadata.ToolUsePromptTokensDetails {
			addGeminiPromptTokens(usage.PromptTokensDetails, detail, false)
		}
		for _, detail := range response.UsageMetadata.CacheTokensDetails {
			addGeminiPromptTokens(usage.PromptTokensDetails, detail, true)
		}
		for _, detail := range response.UsageMetadata.CandidatesTokensDetails {
			addGeminiCompletionTokens(usage.CompletionTokensDetails, detail)
		}
	}
	var toolCalls json.RawMessage
	if calls := response.FunctionCalls(); len(calls) > 0 {
		encoded := make([]map[string]any, 0, len(calls))
		for index, call := range calls {
			if call == nil || strings.TrimSpace(call.Name) == "" {
				continue
			}
			arguments, _ := json.Marshal(call.Args)
			encoded = append(encoded, map[string]any{
				"id": fmt.Sprintf("call_%d", index), "type": "function",
				"function": map[string]any{"name": call.Name, "arguments": string(arguments)},
			})
		}
		if len(encoded) > 0 {
			toolCalls, _ = json.Marshal(encoded)
		}
	}
	return ChatCompletionResponse{
		ID:      id,
		Object:  "chat.completion",
		Created: created,
		Model:   model,
		Choices: []ChatCompletionChoice{{
			Index:        int64(candidate.Index),
			Message:      ChatCompletionReply{Role: "assistant", Content: response.Text(), ToolCalls: toolCalls},
			FinishReason: geminiFinishReason(candidate.FinishReason),
		}},
		Usage: usage,
	}, nil
}

func addGeminiPromptTokens(target *ChatPromptTokensDetails, detail *genai.ModalityTokenCount, cached bool) {
	if target == nil || detail == nil || detail.TokenCount <= 0 {
		return
	}
	tokens := int64(detail.TokenCount)
	switch detail.Modality {
	case genai.MediaModalityAudio:
		if cached {
			target.CachedAudioTokens += tokens
		} else {
			target.AudioTokens += tokens
		}
	case genai.MediaModalityImage:
		if cached {
			target.CachedImageTokens += tokens
		} else {
			target.ImageTokens += tokens
		}
	case genai.MediaModalityVideo:
		if cached {
			target.CachedVideoTokens += tokens
		} else {
			target.VideoTokens += tokens
		}
	}
}

func addGeminiCompletionTokens(target *ChatCompletionTokensDetails, detail *genai.ModalityTokenCount) {
	if target == nil || detail == nil || detail.TokenCount <= 0 {
		return
	}
	tokens := int64(detail.TokenCount)
	switch detail.Modality {
	case genai.MediaModalityAudio:
		target.AudioTokens += tokens
	case genai.MediaModalityImage:
		target.ImageTokens += tokens
	case genai.MediaModalityVideo:
		target.VideoTokens += tokens
	}
}

func geminiFinishReason(reason genai.FinishReason) string {
	switch reason {
	case genai.FinishReasonStop:
		return "stop"
	case genai.FinishReasonMaxTokens:
		return "length"
	case genai.FinishReasonSafety, genai.FinishReasonRecitation,
		genai.FinishReasonBlocklist, genai.FinishReasonProhibitedContent,
		genai.FinishReasonSPII, genai.FinishReasonImageSafety:
		return "content_filter"
	default:
		return strings.ToLower(string(reason))
	}
}

func geminiUpstreamError(err error) error {
	var apiErr *genai.APIError
	if errors.As(err, &apiErr) {
		return &UpstreamError{StatusCode: apiErr.Code, Err: ErrUpstream}
	}
	var valueErr genai.APIError
	if errors.As(err, &valueErr) {
		return &UpstreamError{StatusCode: valueErr.Code, Err: ErrUpstream}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return &UpstreamError{StatusCode: http.StatusRequestTimeout, Err: ErrUpstream}
	}
	return &UpstreamError{Err: ErrUpstream}
}
