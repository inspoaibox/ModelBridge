package relay

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"ai-token/internal/billing"
)

// OpenAIProvider also implements the media endpoints exposed by OpenAI and by
// OpenAI-compatible gateways such as xAI. The request bodies are kept as JSON
// or multipart form data so provider-specific fields survive the relay.
func (OpenAIProvider) GenerateImages(ctx context.Context, upstream UpstreamImageRequest) (MediaJSONResponse, error) {
	body, err := openAIImageRequestBody(upstream.Request, upstreamModelFor(upstream.Channel, upstream.Request.Model))
	if err != nil {
		return MediaJSONResponse{}, ErrInvalidRequest
	}
	data, header, status, err := mediaJSONRequest(ctx, upstream.Channel.BaseURL, upstream.APIKey, "/images/generations", body)
	if err != nil {
		return MediaJSONResponse{}, err
	}
	return imageResponseWithUsage(data, status, header, upstream.Request.Count)
}

func (OpenAIProvider) EditImage(ctx context.Context, upstream UpstreamImageEditRequest) (MediaJSONResponse, error) {
	fields := cloneStringMap(upstream.Request.Fields)
	fields["model"] = upstreamModelFor(upstream.Channel, upstream.Request.Model)
	data, header, status, err := mediaMultipartRequest(ctx, upstream.Channel.BaseURL, upstream.APIKey, "/images/edits", fields,
		"image", upstream.Request.ImageName, upstream.Request.ImageType, upstream.Request.Image,
		upstream.Request.MaskName, upstream.Request.MaskType, upstream.Request.Mask)
	if err != nil {
		return MediaJSONResponse{}, err
	}
	return imageResponseWithUsage(data, status, header, 1)
}

func (OpenAIProvider) TranscribeAudio(ctx context.Context, upstream UpstreamAudioRequest) (MediaJSONResponse, error) {
	return openAIAudioJSON(ctx, upstream, "/audio/transcriptions")
}

func (OpenAIProvider) TranslateAudio(ctx context.Context, upstream UpstreamAudioRequest) (MediaJSONResponse, error) {
	return openAIAudioJSON(ctx, upstream, "/audio/translations")
}

func openAIAudioJSON(ctx context.Context, upstream UpstreamAudioRequest, endpoint string) (MediaJSONResponse, error) {
	fields := cloneStringMap(upstream.Request.Fields)
	fields["model"] = upstreamModelFor(upstream.Channel, upstream.Request.Model)
	data, header, status, err := mediaMultipartRequest(ctx, upstream.Channel.BaseURL, upstream.APIKey, endpoint, fields,
		"file", upstream.Request.FileName, upstream.Request.FileType, upstream.Request.File, "", "", nil)
	if err != nil {
		return MediaJSONResponse{}, err
	}
	return audioResponseWithUsage(data, status, header, upstream.Request.File)
}

func (OpenAIProvider) SynthesizeSpeech(ctx context.Context, upstream UpstreamSpeechRequest) (MediaBinaryResponse, error) {
	body, err := openAISpeechRequestBody(upstream.Request, upstreamModelFor(upstream.Channel, upstream.Request.Model))
	if err != nil {
		return MediaBinaryResponse{}, ErrInvalidRequest
	}
	response, err := mediaBinaryRequest(ctx, http.MethodPost, upstream.Channel.BaseURL, upstream.APIKey, "/audio/speech", body, "application/json")
	if err != nil {
		return MediaBinaryResponse{}, err
	}
	response.Usage = MediaUsage{
		Metrics: billing.MeteredUsage{"input_characters": strconv.FormatInt(int64(len([]rune(upstream.Request.Input))), 10)},
		Source:  "local_estimate",
	}
	return response, nil
}

func (OpenAIProvider) CreateVideo(ctx context.Context, upstream UpstreamVideoRequest) (MediaJSONResponse, error) {
	body, err := openAIVideoRequestBody(upstream.Request, upstreamModelFor(upstream.Channel, upstream.Request.Model))
	if err != nil {
		return MediaJSONResponse{}, ErrInvalidRequest
	}
	data, header, status, err := mediaJSONRequest(ctx, upstream.Channel.BaseURL, upstream.APIKey, "/videos", body)
	if err != nil {
		return MediaJSONResponse{}, err
	}
	response, err := decodeMediaJSON(data, status, header)
	if err != nil {
		return MediaJSONResponse{}, err
	}
	response.ID = extractJSONString(data, "id")
	response.Status = extractJSONString(data, "status")
	return response, nil
}

func (OpenAIProvider) GetVideo(ctx context.Context, upstream UpstreamVideoRequest, jobID string) (MediaJSONResponse, error) {
	data, header, status, err := mediaJSONMethodRequest(ctx, http.MethodGet, upstream.Channel.BaseURL, upstream.APIKey, "/videos/"+url.PathEscape(strings.TrimSpace(jobID)), nil)
	if err != nil {
		return MediaJSONResponse{}, err
	}
	response, err := decodeMediaJSON(data, status, header)
	if err != nil {
		return MediaJSONResponse{}, err
	}
	response.ID = extractJSONString(data, "id")
	response.Status = extractJSONString(data, "status")
	return response, nil
}

func (OpenAIProvider) DownloadVideo(ctx context.Context, upstream UpstreamVideoRequest, value string) (MediaBinaryResponse, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return MediaBinaryResponse{}, ErrInvalidRequest
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return downloadMediaURL(ctx, upstream.Channel.BaseURL, upstream.APIKey, value)
	}
	return mediaBinaryRequest(ctx, http.MethodGet, upstream.Channel.BaseURL, upstream.APIKey, path.Join("/videos", url.PathEscape(value), "content"), nil, "")
}

func (GrokProvider) GenerateImages(ctx context.Context, upstream UpstreamImageRequest) (MediaJSONResponse, error) {
	return (OpenAIProvider{}).GenerateImages(ctx, upstream)
}

func (GrokProvider) EditImage(ctx context.Context, upstream UpstreamImageEditRequest) (MediaJSONResponse, error) {
	if len(upstream.Request.Mask) > 0 {
		// xAI's current image edit API does not expose OpenAI's mask contract.
		// Reject it rather than silently changing the requested edit semantics.
		return MediaJSONResponse{}, ErrUnsupportedFeature
	}
	return grokEditImage(ctx, upstream)
}

func (GrokProvider) TranscribeAudio(ctx context.Context, upstream UpstreamAudioRequest) (MediaJSONResponse, error) {
	return grokTranscribeAudio(ctx, upstream)
}

func (GrokProvider) SynthesizeSpeech(ctx context.Context, upstream UpstreamSpeechRequest) (MediaBinaryResponse, error) {
	return grokSynthesizeSpeech(ctx, upstream)
}

func (GrokProvider) CreateVideo(ctx context.Context, upstream UpstreamVideoRequest) (MediaJSONResponse, error) {
	body, err := grokVideoRequestBody(upstream.Request, upstreamModelFor(upstream.Channel, upstream.Request.Model))
	if err != nil {
		return MediaJSONResponse{}, ErrInvalidRequest
	}
	data, header, status, err := mediaJSONRequest(ctx, upstream.Channel.BaseURL, upstream.APIKey, "/videos/generations", body)
	if err != nil {
		return MediaJSONResponse{}, err
	}
	response, err := decodeMediaJSON(data, status, header)
	if err != nil {
		return MediaJSONResponse{}, err
	}
	response.ID = firstNonEmpty(extractJSONString(data, "request_id"), extractJSONString(data, "id"))
	response.Status = extractJSONString(data, "status")
	return response, nil
}

func (GrokProvider) GetVideo(ctx context.Context, upstream UpstreamVideoRequest, jobID string) (MediaJSONResponse, error) {
	data, header, status, err := mediaJSONMethodRequest(ctx, http.MethodGet, upstream.Channel.BaseURL, upstream.APIKey, "/videos/"+url.PathEscape(strings.TrimSpace(jobID)), nil)
	if err != nil {
		return MediaJSONResponse{}, err
	}
	response, err := decodeMediaJSON(data, status, header)
	if err != nil {
		return MediaJSONResponse{}, err
	}
	response.ID = firstNonEmpty(extractJSONString(data, "request_id"), extractJSONString(data, "id"), jobID)
	response.Status = extractJSONString(data, "status")
	return response, nil
}

func (GrokProvider) DownloadVideo(ctx context.Context, upstream UpstreamVideoRequest, value string) (MediaBinaryResponse, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return MediaBinaryResponse{}, ErrInvalidRequest
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return downloadMediaURL(ctx, upstream.Channel.BaseURL, upstream.APIKey, value)
	}
	return MediaBinaryResponse{}, ErrInvalidRequest
}

func grokEditImage(ctx context.Context, upstream UpstreamImageEditRequest) (MediaJSONResponse, error) {
	prompt := strings.TrimSpace(upstream.Request.Fields["prompt"])
	if prompt == "" || len(upstream.Request.Image) == 0 {
		return MediaJSONResponse{}, ErrInvalidRequest
	}
	imageType := firstNonEmpty(upstream.Request.ImageType, "image/png")
	body := map[string]any{
		"model":  upstreamModelFor(upstream.Channel, upstream.Request.Model),
		"prompt": prompt,
		"image": map[string]string{
			"url":  "data:" + imageType + ";base64," + base64.StdEncoding.EncodeToString(upstream.Request.Image),
			"type": "image_url",
		},
	}
	if quality := strings.TrimSpace(upstream.Request.Fields["quality"]); quality != "" {
		body["quality"] = quality
	}
	if aspectRatio := strings.TrimSpace(upstream.Request.Fields["aspect_ratio"]); aspectRatio != "" {
		body["aspect_ratio"] = aspectRatio
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return MediaJSONResponse{}, ErrInvalidRequest
	}
	data, header, status, err := mediaJSONRequest(ctx, upstream.Channel.BaseURL, upstream.APIKey, "/images/edits", encoded)
	if err != nil {
		return MediaJSONResponse{}, err
	}
	return imageResponseWithUsage(data, status, header, 1)
}

func grokVideoRequestBody(request VideoCreateRequest, model string) ([]byte, error) {
	var body map[string]any
	if len(request.Payload) > 0 {
		if err := json.Unmarshal(request.Payload, &body); err != nil {
			return nil, ErrInvalidRequest
		}
	}
	if body == nil {
		body = map[string]any{}
	}
	body["model"] = model
	body["prompt"] = request.Prompt
	if seconds, ok := body["seconds"]; ok {
		body["duration"] = seconds
		delete(body, "seconds")
	} else if duration := strings.TrimSpace(request.Duration); duration != "" {
		if parsed, err := strconv.ParseFloat(duration, 64); err == nil {
			body["duration"] = parsed
		} else {
			return nil, ErrInvalidRequest
		}
	}
	return json.Marshal(body)
}

func grokTranscribeAudio(ctx context.Context, upstream UpstreamAudioRequest) (MediaJSONResponse, error) {
	fields := cloneStringMap(upstream.Request.Fields)
	delete(fields, "model")
	data, header, status, err := mediaMultipartRequest(ctx, upstream.Channel.BaseURL, upstream.APIKey, "/stt", fields,
		"file", upstream.Request.FileName, upstream.Request.FileType, upstream.Request.File, "", "", nil)
	if err != nil {
		return MediaJSONResponse{}, err
	}
	return audioResponseWithUsage(data, status, header, upstream.Request.File)
}

func grokSynthesizeSpeech(ctx context.Context, upstream UpstreamSpeechRequest) (MediaBinaryResponse, error) {
	var body map[string]any
	if len(upstream.Request.Payload) > 0 {
		if err := json.Unmarshal(upstream.Request.Payload, &body); err != nil {
			return MediaBinaryResponse{}, ErrInvalidRequest
		}
	}
	if body == nil {
		body = map[string]any{}
	}
	body["text"] = upstream.Request.Input
	delete(body, "input")
	if voice, ok := body["voice"].(string); ok {
		if _, exists := body["voice_id"]; !exists {
			body["voice_id"] = voice
		}
		delete(body, "voice")
	}
	delete(body, "model")
	encoded, err := json.Marshal(body)
	if err != nil {
		return MediaBinaryResponse{}, ErrInvalidRequest
	}
	response, err := mediaBinaryRequest(ctx, http.MethodPost, upstream.Channel.BaseURL, upstream.APIKey, "/tts", encoded, "application/json")
	if err != nil {
		return MediaBinaryResponse{}, err
	}
	response.Usage = MediaUsage{Metrics: billing.MeteredUsage{"input_characters": strconv.FormatInt(int64(len([]rune(upstream.Request.Input))), 10)}, Source: "local_estimate"}
	return response, nil
}

// Gemini exposes image and video generation through the native REST methods.
// This keeps Imagen/Veo available without forcing an OpenAI-shaped request on
// the provider, whose operation and media response formats are different.
func (GeminiProvider) GenerateImages(ctx context.Context, upstream UpstreamImageRequest) (MediaJSONResponse, error) {
	model := geminiModelName(upstreamModelFor(upstream.Channel, upstream.Request.Model))
	if strings.Contains(strings.ToLower(model), "imagen") {
		body := map[string]any{"instances": []map[string]any{{"prompt": upstream.Request.Prompt}}, "parameters": map[string]any{"sampleCount": upstream.Request.Count}}
		encoded, err := json.Marshal(body)
		if err != nil {
			return MediaJSONResponse{}, ErrInvalidRequest
		}
		data, header, status, err := geminiRESTRequest(ctx, upstream.Channel.BaseURL, upstream.APIKey, http.MethodPost, "/models/"+url.PathEscape(model)+":predict", encoded)
		if err != nil {
			return MediaJSONResponse{}, err
		}
		return geminiImageResponse(data, status, header, upstream.Request.Count)
	}
	body := map[string]any{"contents": []map[string]any{{"role": "user", "parts": []map[string]any{{"text": upstream.Request.Prompt}}}}, "generationConfig": map[string]any{"responseModalities": []string{"TEXT", "IMAGE"}}}
	encoded, err := json.Marshal(body)
	if err != nil {
		return MediaJSONResponse{}, ErrInvalidRequest
	}
	data, header, status, err := geminiRESTRequest(ctx, upstream.Channel.BaseURL, upstream.APIKey, http.MethodPost, "/models/"+url.PathEscape(model)+":generateContent", encoded)
	if err != nil {
		return MediaJSONResponse{}, err
	}
	return geminiImageResponse(data, status, header, upstream.Request.Count)
}

func (GeminiProvider) EditImage(ctx context.Context, upstream UpstreamImageEditRequest) (MediaJSONResponse, error) {
	if len(upstream.Request.Image) == 0 {
		return MediaJSONResponse{}, ErrInvalidRequest
	}
	if len(upstream.Request.Mask) > 0 {
		return MediaJSONResponse{}, ErrUnsupportedFeature
	}
	imageType := firstNonEmpty(upstream.Request.ImageType, "image/png")
	prompt := strings.TrimSpace(upstream.Request.Fields["prompt"])
	if prompt == "" {
		return MediaJSONResponse{}, ErrInvalidRequest
	}
	body := map[string]any{
		"contents": []map[string]any{{"role": "user", "parts": []map[string]any{
			{"inlineData": map[string]string{"mimeType": imageType, "data": base64.StdEncoding.EncodeToString(upstream.Request.Image)}},
			{"text": prompt},
		}}},
		"generationConfig": map[string]any{"responseModalities": []string{"IMAGE"}},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return MediaJSONResponse{}, ErrInvalidRequest
	}
	model := geminiModelName(upstreamModelFor(upstream.Channel, upstream.Request.Model))
	data, header, status, err := geminiRESTRequest(ctx, upstream.Channel.BaseURL, upstream.APIKey, http.MethodPost, "/models/"+url.PathEscape(model)+":generateContent", encoded)
	if err != nil {
		return MediaJSONResponse{}, err
	}
	return geminiImageResponse(data, status, header, 1)
}

func (GeminiProvider) TranscribeAudio(ctx context.Context, upstream UpstreamAudioRequest) (MediaJSONResponse, error) {
	return geminiTranscribeAudio(ctx, upstream, false)
}

func (GeminiProvider) TranslateAudio(ctx context.Context, upstream UpstreamAudioRequest) (MediaJSONResponse, error) {
	return geminiTranscribeAudio(ctx, upstream, true)
}

func (GeminiProvider) SynthesizeSpeech(ctx context.Context, upstream UpstreamSpeechRequest) (MediaBinaryResponse, error) {
	return MediaJSONBinarySpeech(ctx, upstream)
}

func (GeminiProvider) CreateVideo(ctx context.Context, upstream UpstreamVideoRequest) (MediaJSONResponse, error) {
	model := geminiModelName(upstreamModelFor(upstream.Channel, upstream.Request.Model))
	body := map[string]any{"instances": []map[string]any{{"prompt": upstream.Request.Prompt}}, "parameters": map[string]any{"sampleCount": 1}}
	if requestBody, err := json.Marshal(upstream.Request.Payload); err == nil && len(upstream.Request.Payload) > 0 && string(requestBody) != "null" {
		var payload map[string]any
		if json.Unmarshal(upstream.Request.Payload, &payload) == nil {
			if instances, ok := payload["instances"]; ok {
				body["instances"] = instances
			}
			if parameters, ok := payload["parameters"]; ok {
				body["parameters"] = parameters
			}
		}
	}
	parameters, _ := body["parameters"].(map[string]any)
	if parameters == nil {
		parameters = map[string]any{}
		body["parameters"] = parameters
	}
	if _, ok := parameters["durationSeconds"]; !ok {
		if seconds, err := strconv.ParseFloat(strings.TrimSpace(upstream.Request.Duration), 64); err == nil && seconds > 0 {
			parameters["durationSeconds"] = seconds
		}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return MediaJSONResponse{}, ErrInvalidRequest
	}
	data, header, status, err := geminiRESTRequest(ctx, upstream.Channel.BaseURL, upstream.APIKey, http.MethodPost, "/models/"+url.PathEscape(model)+":predictLongRunning", encoded)
	if err != nil {
		return MediaJSONResponse{}, err
	}
	response, err := decodeMediaJSON(data, status, header)
	if err != nil {
		return MediaJSONResponse{}, err
	}
	response.Usage = parseGeminiMediaUsage(data)
	response.ID = extractJSONString(data, "name")
	response.Status = "queued"
	return response, nil
}

func (GeminiProvider) GetVideo(ctx context.Context, upstream UpstreamVideoRequest, jobID string) (MediaJSONResponse, error) {
	name := strings.TrimPrefix(strings.TrimSpace(jobID), "/")
	requestPath := "/" + strings.TrimPrefix(name, "v1beta/")
	if !strings.HasPrefix(requestPath, "/v1") {
		requestPath = "/v1beta" + requestPath
	}
	data, header, status, err := geminiRESTRequest(ctx, upstream.Channel.BaseURL, upstream.APIKey, http.MethodGet, requestPath, nil)
	if err != nil {
		return MediaJSONResponse{}, err
	}
	response, err := decodeMediaJSON(data, status, header)
	if err != nil {
		return MediaJSONResponse{}, err
	}
	response.Usage = parseGeminiMediaUsage(data)
	response.ID = name
	response.Status = geminiOperationStatus(data)
	return response, nil
}

func (GeminiProvider) DownloadVideo(ctx context.Context, upstream UpstreamVideoRequest, value string) (MediaBinaryResponse, error) {
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return downloadMediaURLWithHeader(ctx, upstream.Channel.BaseURL, upstream.APIKey, value, "x-goog-api-key", upstream.APIKey)
	}
	return MediaBinaryResponse{}, ErrInvalidRequest
}

func geminiTranscribeAudio(ctx context.Context, upstream UpstreamAudioRequest, translate bool) (MediaJSONResponse, error) {
	model := geminiModelName(upstreamModelFor(upstream.Channel, upstream.Request.Model))
	encoded := base64.StdEncoding.EncodeToString(upstream.Request.File)
	prompt := "Transcribe the attached audio exactly. Return only the transcript text."
	if translate {
		prompt = "Translate the attached audio into English. Return only the translated text."
	}
	body := map[string]any{
		"contents": []map[string]any{{"role": "user", "parts": []map[string]any{
			{"inlineData": map[string]string{"mimeType": firstNonEmpty(upstream.Request.FileType, "audio/mpeg"), "data": encoded}},
			{"text": prompt},
		}}},
		"generationConfig": map[string]any{"responseMimeType": "text/plain"},
	}
	requestBody, err := json.Marshal(body)
	if err != nil {
		return MediaJSONResponse{}, ErrInvalidRequest
	}
	data, header, status, err := geminiRESTRequest(ctx, upstream.Channel.BaseURL, upstream.APIKey, http.MethodPost, "/models/"+url.PathEscape(model)+":generateContent", requestBody)
	if err != nil {
		return MediaJSONResponse{}, err
	}
	if status < 200 || status >= 300 {
		return MediaJSONResponse{}, &UpstreamError{StatusCode: status, Err: ErrUpstream}
	}
	text := extractGeminiText(data)
	if text == "" {
		return MediaJSONResponse{}, ErrUpstream
	}
	responseBody, _ := json.Marshal(map[string]any{"text": text})
	usage := parseGeminiMediaUsage(data)
	if !usage.UsageProvided {
		usage = parseMediaUsage(data)
	}
	if _, ok := usage.Metrics["input_audio_seconds"]; !ok {
		if seconds := wavDurationSeconds(upstream.Request.File); seconds != "" {
			usage.Metrics["input_audio_seconds"] = seconds
			if !usage.UsageProvided {
				usage.Source = "local_estimate"
			}
		}
	}
	usage.Raw = append(json.RawMessage(nil), data...)
	return MediaJSONResponse{Body: responseBody, ProviderRequestID: header.Get("x-request-id"), Usage: usage}, nil
}

func geminiRESTRequest(ctx context.Context, baseURL, apiKey, method, requestPath string, body []byte) ([]byte, http.Header, int, error) {
	parsed, err := parseGeminiBaseURL(baseURL)
	if err != nil {
		return nil, nil, 0, ErrInvalidRequest
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + requestPath
	request, err := http.NewRequestWithContext(ctx, method, parsed.String(), strings.NewReader(string(body)))
	if err != nil {
		return nil, nil, 0, ErrInvalidRequest
	}
	request.Header.Set("x-goog-api-key", strings.TrimSpace(apiKey))
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	client, err := providerHTTPClient(baseURL)
	if err != nil {
		return nil, nil, 0, ErrInvalidRequest
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, nil, 0, &UpstreamError{Err: ErrUpstream}
	}
	defer response.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(response.Body, maxMediaJSONSize))
	if readErr != nil {
		return nil, response.Header, response.StatusCode, &UpstreamError{StatusCode: response.StatusCode, Err: ErrUpstream}
	}
	return data, response.Header, response.StatusCode, nil
}

func parseGeminiBaseURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(value), "/"))
	if err != nil || parsed.Host == "" {
		return nil, ErrInvalidRequest
	}
	parsed.Path = strings.TrimSuffix(parsed.Path, "/v1beta")
	parsed.Path = strings.TrimSuffix(parsed.Path, "/v1")
	parsed.Path += "/v1beta"
	return parsed, nil
}

func geminiModelName(value string) string {
	return strings.TrimPrefix(strings.TrimSpace(value), "models/")
}

func geminiImageResponse(data []byte, status int, header http.Header, count int64) (MediaJSONResponse, error) {
	if status < 200 || status >= 300 {
		return MediaJSONResponse{}, &UpstreamError{StatusCode: status, Err: ErrUpstream}
	}
	var value map[string]any
	if json.Unmarshal(data, &value) != nil {
		return MediaJSONResponse{}, ErrUpstream
	}
	items := []map[string]any{}
	if predictions, ok := value["predictions"].([]any); ok {
		for _, prediction := range predictions {
			if item, ok := prediction.(map[string]any); ok {
				if encoded, ok := item["bytesBase64Encoded"].(string); ok {
					items = append(items, map[string]any{"b64_json": encoded})
				} else if uri, ok := item["uri"].(string); ok {
					items = append(items, map[string]any{"url": uri})
				}
			}
		}
	}
	if len(items) == 0 {
		if parts := findGeminiImageParts(value); len(parts) > 0 {
			items = parts
		}
	}
	if len(items) == 0 {
		return MediaJSONResponse{}, ErrUpstream
	}
	responseBody, _ := json.Marshal(map[string]any{"created": time.Now().Unix(), "data": items})
	usage := parseGeminiMediaUsage(data)
	if !usage.UsageProvided {
		usage = MediaUsage{Metrics: billing.MeteredUsage{}, Source: "upstream"}
	}
	usage.Metrics["output_images"] = strconv.Itoa(len(items))
	usage.Raw = append(json.RawMessage(nil), data...)
	response := MediaJSONResponse{Body: responseBody, ProviderRequestID: header.Get("x-request-id"), Usage: usage}
	return response, nil
}

func findGeminiImageParts(value map[string]any) []map[string]any {
	parts := []map[string]any{}
	var visit func(any)
	visit = func(current any) {
		switch item := current.(type) {
		case map[string]any:
			if inline, ok := item["inlineData"].(map[string]any); ok {
				if encoded, ok := inline["data"].(string); ok {
					parts = append(parts, map[string]any{"b64_json": encoded})
				}
			}
			for _, child := range item {
				visit(child)
			}
		case []any:
			for _, child := range item {
				visit(child)
			}
		}
	}
	visit(value)
	return parts
}

func geminiOperationStatus(data []byte) string {
	var value map[string]any
	if json.Unmarshal(data, &value) != nil {
		return "processing"
	}
	if done, ok := value["done"].(bool); ok && done {
		if _, failed := value["error"]; failed {
			return "failed"
		}
		return "completed"
	}
	return "processing"
}

func MediaJSONBinarySpeech(ctx context.Context, upstream UpstreamSpeechRequest) (MediaBinaryResponse, error) {
	model := geminiModelName(upstreamModelFor(upstream.Channel, upstream.Request.Model))
	body := map[string]any{"contents": []map[string]any{{"parts": []map[string]any{{"text": upstream.Request.Input}}}}, "generationConfig": map[string]any{"responseModalities": []string{"AUDIO"}}}
	var payload map[string]any
	if len(upstream.Request.Payload) > 0 && json.Unmarshal(upstream.Request.Payload, &payload) == nil {
		if contents, ok := payload["contents"]; ok {
			body["contents"] = contents
		}
		generationConfig, _ := body["generationConfig"].(map[string]any)
		if configured, ok := payload["generationConfig"].(map[string]any); ok {
			generationConfig = configured
		}
		if configured, ok := payload["speechConfig"]; ok {
			generationConfig["speechConfig"] = configured
		}
		if _, ok := generationConfig["responseModalities"]; !ok {
			generationConfig["responseModalities"] = []string{"AUDIO"}
		}
		body["generationConfig"] = generationConfig
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return MediaBinaryResponse{}, ErrInvalidRequest
	}
	data, header, status, err := geminiRESTRequest(ctx, upstream.Channel.BaseURL, upstream.APIKey, http.MethodPost, "/models/"+url.PathEscape(model)+":generateContent", encoded)
	if err != nil || status < 200 || status >= 300 {
		if err != nil {
			return MediaBinaryResponse{}, err
		}
		return MediaBinaryResponse{}, &UpstreamError{StatusCode: status, Err: ErrUpstream}
	}
	var value map[string]any
	if json.Unmarshal(data, &value) != nil {
		return MediaBinaryResponse{}, ErrUpstream
	}
	encodedAudio, mimeType := findGeminiAudio(value)
	if encodedAudio == "" {
		return MediaBinaryResponse{}, ErrUpstream
	}
	decoded, err := base64.StdEncoding.DecodeString(encodedAudio)
	if err != nil {
		return MediaBinaryResponse{}, ErrUpstream
	}
	usage := parseGeminiMediaUsage(data)
	if !usage.UsageProvided {
		usage = MediaUsage{Metrics: billing.MeteredUsage{"input_characters": strconv.Itoa(len([]rune(upstream.Request.Input)))}, Source: "local_estimate", Raw: data}
	} else {
		usage.Raw = append(json.RawMessage(nil), data...)
	}
	return MediaBinaryResponse{Body: decoded, ContentType: firstNonEmpty(mimeType, "audio/wav"), ProviderRequestID: header.Get("x-request-id"), Usage: usage}, nil
}

func findGeminiAudio(value map[string]any) (string, string) {
	var encoded, mime string
	var visit func(any)
	visit = func(current any) {
		switch item := current.(type) {
		case map[string]any:
			if inline, ok := item["inlineData"].(map[string]any); ok {
				if encoded == "" {
					encoded, _ = inline["data"].(string)
					mime, _ = inline["mimeType"].(string)
				}
			}
			for _, child := range item {
				visit(child)
			}
		case []any:
			for _, child := range item {
				visit(child)
			}
		}
	}
	visit(value)
	return encoded, mime
}

func extractGeminiText(data []byte) string {
	var value map[string]any
	if json.Unmarshal(data, &value) != nil {
		return ""
	}
	var result strings.Builder
	var visit func(any)
	visit = func(current any) {
		switch item := current.(type) {
		case map[string]any:
			if text, ok := item["text"].(string); ok {
				result.WriteString(text)
			}
			for key, child := range item {
				if key != "text" {
					visit(child)
				}
			}
		case []any:
			for _, child := range item {
				visit(child)
			}
		}
	}
	visit(value)
	return strings.TrimSpace(result.String())
}

func parseGeminiMediaUsage(data []byte) MediaUsage {
	usage := MediaUsage{Metrics: billing.MeteredUsage{}, Source: "upstream", Raw: append(json.RawMessage(nil), data...)}
	var envelope any
	if json.Unmarshal(data, &envelope) != nil {
		return usage
	}
	rawUsage, ok := findNestedJSONValue(envelope, "usageMetadata")
	if !ok {
		return usage
	}
	var value struct {
		PromptTokenCount        int64 `json:"promptTokenCount"`
		ToolUsePromptTokenCount int64 `json:"toolUsePromptTokenCount"`
		CandidatesTokenCount    int64 `json:"candidatesTokenCount"`
		TotalTokenCount         int64 `json:"totalTokenCount"`
		CachedContentTokenCount int64 `json:"cachedContentTokenCount"`
		PromptTokensDetails     []struct {
			Modality   string `json:"modality"`
			TokenCount int64  `json:"tokenCount"`
		} `json:"promptTokensDetails"`
		ToolUsePromptTokensDetails []struct {
			Modality   string `json:"modality"`
			TokenCount int64  `json:"tokenCount"`
		} `json:"toolUsePromptTokensDetails"`
		CacheTokensDetails []struct {
			Modality   string `json:"modality"`
			TokenCount int64  `json:"tokenCount"`
		} `json:"cacheTokensDetails"`
		CandidatesTokensDetails []struct {
			Modality   string `json:"modality"`
			TokenCount int64  `json:"tokenCount"`
		} `json:"candidatesTokensDetails"`
	}
	raw, _ := json.Marshal(rawUsage)
	if json.Unmarshal(raw, &value) != nil ||
		value.PromptTokenCount <= 0 && value.ToolUsePromptTokenCount <= 0 &&
			value.CandidatesTokenCount <= 0 && value.TotalTokenCount <= 0 {
		return usage
	}
	usage.UsageProvided = true
	usage.InputTokens = value.PromptTokenCount + value.ToolUsePromptTokenCount
	usage.OutputTokens = value.CandidatesTokenCount
	if usage.InputTokens == 0 && usage.OutputTokens > 0 && value.TotalTokenCount > usage.OutputTokens {
		usage.InputTokens = value.TotalTokenCount - usage.OutputTokens
	}
	if usage.OutputTokens == 0 && usage.InputTokens > 0 && value.TotalTokenCount > usage.InputTokens {
		usage.OutputTokens = value.TotalTokenCount - usage.InputTokens
	}
	if usage.InputTokens > 0 {
		usage.Metrics["input_tokens"] = strconv.FormatInt(usage.InputTokens, 10)
	}
	if usage.OutputTokens > 0 {
		usage.Metrics["output_tokens"] = strconv.FormatInt(usage.OutputTokens, 10)
	}
	for _, detail := range value.PromptTokensDetails {
		setGeminiMediaUsageMetric(usage.Metrics, "input", detail.Modality, detail.TokenCount)
	}
	for _, detail := range value.ToolUsePromptTokensDetails {
		setGeminiMediaUsageMetric(usage.Metrics, "input", detail.Modality, detail.TokenCount)
	}
	for _, detail := range value.CacheTokensDetails {
		setGeminiMediaUsageMetric(usage.Metrics, "cached", detail.Modality, detail.TokenCount)
	}
	for _, detail := range value.CandidatesTokensDetails {
		setGeminiMediaUsageMetric(usage.Metrics, "output", detail.Modality, detail.TokenCount)
	}
	genericCached := value.CachedContentTokenCount
	for _, code := range []string{"cached_audio_tokens", "cached_image_tokens", "cached_video_tokens"} {
		if parsed, err := strconv.ParseInt(strings.TrimSpace(usage.Metrics[code]), 10, 64); err == nil {
			genericCached -= parsed
		}
	}
	if genericCached < 0 {
		genericCached = 0
	}
	usage.CachedInputTokens = genericCached
	if genericCached > 0 {
		usage.Metrics["cached_input_tokens"] = strconv.FormatInt(genericCached, 10)
	}
	return usage
}

func findNestedJSONValue(value any, key string) (any, bool) {
	switch current := value.(type) {
	case map[string]any:
		if child, ok := current[key]; ok {
			return child, true
		}
		for _, child := range current {
			if found, ok := findNestedJSONValue(child, key); ok {
				return found, true
			}
		}
	case []any:
		for _, child := range current {
			if found, ok := findNestedJSONValue(child, key); ok {
				return found, true
			}
		}
	}
	return nil, false
}

func setGeminiMediaUsageMetric(metrics billing.MeteredUsage, direction, modality string, value int64) {
	if value <= 0 {
		return
	}
	modality = strings.ToLower(strings.TrimSpace(modality))
	code := ""
	switch modality {
	case "audio":
		if direction == "cached" {
			code = "cached_audio_tokens"
		} else {
			code = direction + "_audio_tokens"
		}
	case "image":
		if direction == "cached" {
			code = "cached_image_tokens"
		} else {
			code = direction + "_image_tokens"
		}
	case "video":
		if direction == "cached" {
			code = "cached_video_tokens"
		} else {
			code = direction + "_video_tokens"
		}
	}
	if code != "" {
		existing, _ := strconv.ParseInt(strings.TrimSpace(metrics[code]), 10, 64)
		metrics[code] = strconv.FormatInt(existing+value, 10)
	}
}

func cloneStringMap(value map[string]string) map[string]string {
	result := make(map[string]string, len(value)+1)
	for key, item := range value {
		result[key] = item
	}
	return result
}

func downloadMediaURL(ctx context.Context, baseURL, apiKey, rawURL string) (MediaBinaryResponse, error) {
	return downloadMediaURLWithHeader(ctx, baseURL, apiKey, rawURL, "Authorization", "Bearer "+strings.TrimSpace(apiKey))
}

func downloadMediaURLWithHeader(ctx context.Context, baseURL, apiKey, rawURL, headerName, headerValue string) (MediaBinaryResponse, error) {
	target, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" || target.User != nil || target.Fragment != "" {
		return MediaBinaryResponse{}, ErrInvalidRequest
	}
	base, err := parseAndValidateBaseURL(baseURL)
	if err != nil || target.Scheme != "https" {
		return MediaBinaryResponse{}, ErrInvalidRequest
	}
	sameOrigin := strings.EqualFold(base.Hostname(), target.Hostname()) && effectiveURLPort(base) == effectiveURLPort(target)
	clientBase := baseURL
	if !sameOrigin {
		if err := validateHost(target.Hostname()); err != nil {
			return MediaBinaryResponse{}, ErrInvalidRequest
		}
		clientBase = target.Scheme + "://" + target.Host
	}
	client, err := providerHTTPClient(clientBase)
	if err != nil {
		return MediaBinaryResponse{}, ErrInvalidRequest
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return MediaBinaryResponse{}, ErrInvalidRequest
	}
	// Signed vendor URLs are intentionally fetched without forwarding the
	// provider API key; same-origin content endpoints still need it.
	if sameOrigin {
		request.Header.Set(headerName, headerValue)
	}
	response, err := client.Do(request)
	if err != nil {
		return MediaBinaryResponse{}, &UpstreamError{Err: ErrUpstream}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return MediaBinaryResponse{}, &UpstreamError{StatusCode: response.StatusCode, Err: ErrUpstream}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxMediaFileSize))
	if err != nil {
		return MediaBinaryResponse{}, &UpstreamError{StatusCode: response.StatusCode, Err: ErrUpstream}
	}
	return MediaBinaryResponse{Body: body, ContentType: response.Header.Get("Content-Type")}, nil
}

func effectiveURLPort(value *url.URL) string {
	if port := value.Port(); port != "" {
		return port
	}
	if value.Scheme == "https" {
		return "443"
	}
	return "80"
}
