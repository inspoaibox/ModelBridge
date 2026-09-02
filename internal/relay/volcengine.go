package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"ai-token/internal/billing"
)

// VolcengineProvider implements the official Volcano Engine Ark content
// generation API used by Seedance. Ark is a video-only provider in this
// relay; it must not be treated as an OpenAI-compatible chat provider.
type VolcengineProvider struct{}

type seedanceModelSpec struct {
	version                 string
	defaultDurationSeconds  float64
	maxDurationSeconds      float64
	maxReferenceImages      int
	maxReferenceVideos      int
	maxReferenceAudios      int
	audioOnlyReference      bool
	resolutions             map[string]struct{}
	supportsOutputFormat    bool
	supportsOmniTaskType    bool
	supportsReturnLastFrame bool
}

var (
	_ Provider             = VolcengineProvider{}
	_ VideoProvider        = VolcengineProvider{}
	_ ModelCatalogProvider = VolcengineProvider{}
)

func (VolcengineProvider) ChatCompletions(context.Context, UpstreamChatCompletionRequest) (ChatCompletionResponse, error) {
	return ChatCompletionResponse{}, ErrUnsupportedFeature
}

func (VolcengineProvider) CreateVideo(ctx context.Context, upstream UpstreamVideoRequest) (MediaJSONResponse, error) {
	baseURL, err := volcengineBaseURL(upstream.Channel.BaseURL)
	if err != nil {
		return MediaJSONResponse{}, ErrInvalidRequest
	}
	body, err := volcengineVideoRequestBody(upstream.Request, upstreamModelFor(upstream.Channel, upstream.Request.Model))
	if err != nil {
		return MediaJSONResponse{}, err
	}
	data, header, status, err := mediaJSONRequest(ctx, baseURL, upstream.APIKey, "/contents/generations/tasks", body)
	if err != nil {
		return MediaJSONResponse{}, err
	}
	response, err := decodeMediaJSON(data, status, header)
	if err != nil {
		return MediaJSONResponse{}, err
	}
	response.Usage = parseVolcengineMediaUsage(data)
	response.ID = firstNonEmpty(extractJSONString(data, "id"), extractJSONString(data, "task_id"))
	response.Status = extractJSONString(data, "status")
	response.ProviderRequestID = firstNonEmpty(header.Get("x-request-id"), response.ID)
	return response, nil
}

func (VolcengineProvider) GetVideo(ctx context.Context, upstream UpstreamVideoRequest, jobID string) (MediaJSONResponse, error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return MediaJSONResponse{}, ErrInvalidRequest
	}
	baseURL, err := volcengineBaseURL(upstream.Channel.BaseURL)
	if err != nil {
		return MediaJSONResponse{}, ErrInvalidRequest
	}
	data, header, status, err := mediaJSONMethodRequest(
		ctx,
		http.MethodGet,
		baseURL,
		upstream.APIKey,
		"/contents/generations/tasks/"+url.PathEscape(jobID),
		nil,
	)
	if err != nil {
		return MediaJSONResponse{}, err
	}
	response, err := decodeMediaJSON(data, status, header)
	if err != nil {
		return MediaJSONResponse{}, err
	}
	response.Usage = parseVolcengineMediaUsage(data)
	response.ID = firstNonEmpty(extractJSONString(data, "id"), extractJSONString(data, "task_id"), jobID)
	response.Status = extractJSONString(data, "status")
	response.ProviderRequestID = firstNonEmpty(header.Get("x-request-id"), response.ID)
	return response, nil
}

func (VolcengineProvider) DownloadVideo(ctx context.Context, upstream UpstreamVideoRequest, value string) (MediaBinaryResponse, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return MediaBinaryResponse{}, ErrInvalidRequest
	}
	if strings.HasPrefix(value, "https://") {
		return downloadMediaURL(ctx, upstream.Channel.BaseURL, upstream.APIKey, value)
	}
	return MediaBinaryResponse{}, ErrInvalidRequest
}

// ListModels tries Ark's official model catalog endpoint. It returns the
// upstream response as discovered models; it does not silently inject the
// Seedance IDs when Ark does not expose them for the configured account.
func (VolcengineProvider) ListModels(ctx context.Context, baseURL, apiKey string) ([]DiscoveredModel, error) {
	baseURL, err := volcengineBaseURL(baseURL)
	if err != nil {
		return nil, ErrModelDiscoveryFailed
	}
	data, header, status, err := mediaJSONMethodRequest(ctx, http.MethodGet, baseURL, apiKey, "/models", nil)
	if err != nil {
		return nil, ErrModelDiscoveryFailed
	}
	response, err := decodeMediaJSON(data, status, header)
	if err != nil {
		return nil, ErrModelDiscoveryFailed
	}
	var envelope struct {
		Data []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	if json.Unmarshal(response.Body, &envelope) != nil {
		return nil, ErrModelDiscoveryFailed
	}
	models := make([]DiscoveredModel, 0, len(envelope.Data))
	for _, item := range envelope.Data {
		id := firstNonEmpty(item.ID, item.Name)
		if id == "" || !strings.Contains(strings.ToLower(id), "seedance") {
			continue
		}
		models = append(models, DiscoveredModel{
			ID:          id,
			DisplayName: firstNonEmpty(item.DisplayName, id),
			Provider:    ProviderVolcengine,
		})
	}
	return normalizeDiscoveredModels(ProviderVolcengine, models), nil
}

func volcengineBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", ErrInvalidRequest
	}
	basePath := strings.TrimRight(parsed.Path, "/")
	if basePath == "" {
		basePath = "/api/v3"
	} else if !strings.HasSuffix(basePath, "/api/v3") {
		basePath += "/api/v3"
	}
	parsed.Path = basePath
	return strings.TrimRight(parsed.String(), "/"), nil
}

func volcengineVideoRequestBody(request VideoCreateRequest, model string) ([]byte, error) {
	body := map[string]any{}
	if len(request.Payload) > 0 {
		if err := json.Unmarshal(request.Payload, &body); err != nil {
			return nil, ErrInvalidRequest
		}
	}
	if body == nil {
		body = map[string]any{}
	}

	spec, err := seedanceModelSpecFor(model)
	if err != nil {
		return nil, err
	}
	body["model"] = model
	if content, ok := body["content"]; !ok || emptyJSONList(content) {
		if strings.TrimSpace(request.Prompt) == "" {
			return nil, ErrInvalidRequest
		}
		body["content"] = []any{map[string]any{"type": "text", "text": request.Prompt}}
	}

	// These are platform/OpenAI-compatible fields. Ark uses content and
	// duration instead. Vendor-specific fields are retained only after the
	// model-specific validator confirms that this Seedance version supports
	// them.
	delete(body, "prompt")
	seconds := strings.TrimSpace(request.Duration)
	if seconds == "" {
		if value, ok := body["seconds"]; ok {
			seconds = strings.Trim(strings.TrimSpace(stringValue(value)), `"`)
		}
	}
	delete(body, "seconds")
	if seconds == "" {
		if value, ok := body["duration"]; ok {
			seconds = strings.Trim(strings.TrimSpace(stringValue(value)), `"`)
		}
	}
	if seconds != "" {
		duration, err := strconv.ParseFloat(seconds, 64)
		if err != nil || math.IsNaN(duration) || math.IsInf(duration, 0) || math.Trunc(duration) != duration {
			return nil, ErrInvalidRequest
		}
		if duration <= 0 && duration != -1 {
			return nil, ErrInvalidRequest
		}
		body["duration"] = duration
	} else if _, ok := body["duration"]; !ok {
		// Ark supplies version-specific defaults. Materialize them so the
		// validator and the upstream request have the same semantics.
		body["duration"] = spec.defaultDurationSeconds
	}

	if err := validateSeedanceVideoBody(body, spec); err != nil {
		return nil, err
	}
	return json.Marshal(body)
}

func seedanceModelSpecFor(model string) (seedanceModelSpec, error) {
	name := strings.ToLower(strings.TrimSpace(model))
	normalized := strings.NewReplacer("-", "", ".", "", "_", " ").Replace(name)
	normalized = strings.ReplaceAll(normalized, " ", "")
	switch {
	case strings.Contains(normalized, "seedance25"):
		return seedanceModelSpec{
			version:                 "2.5",
			defaultDurationSeconds:  -1,
			maxDurationSeconds:      30,
			maxReferenceImages:      30,
			maxReferenceVideos:      10,
			maxReferenceAudios:      10,
			audioOnlyReference:      true,
			resolutions:             stringSet("480p", "720p", "1080p"),
			supportsOutputFormat:    true,
			supportsOmniTaskType:    true,
			supportsReturnLastFrame: true,
		}, nil
	case strings.Contains(normalized, "seedance20"):
		return seedanceModelSpec{
			version:                 "2.0",
			defaultDurationSeconds:  5,
			maxDurationSeconds:      15,
			maxReferenceImages:      9,
			maxReferenceVideos:      3,
			maxReferenceAudios:      3,
			audioOnlyReference:      false,
			resolutions:             stringSet("480p", "720p", "1080p", "4k"),
			supportsOutputFormat:    false,
			supportsOmniTaskType:    false,
			supportsReturnLastFrame: true,
		}, nil
	default:
		return seedanceModelSpec{}, fmt.Errorf("%w: unsupported Seedance model %q", ErrUnsupportedFeature, model)
	}
}

func stringSet(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func validateSeedanceVideoBody(body map[string]any, spec seedanceModelSpec) error {
	content, ok := seedanceContentItems(body["content"])
	if !ok || len(content) == 0 {
		return ErrInvalidRequest
	}

	imageCount, videoCount, audioCount := 0, 0, 0
	firstFrameCount, implicitFirstFrameCount, lastFrameCount, referenceImageCount := 0, 0, 0, 0
	for _, item := range content {
		contentType, _ := item["type"].(string)
		role := seedanceContentRole(item)
		switch strings.ToLower(strings.TrimSpace(contentType)) {
		case "text":
			if text, ok := item["text"].(string); !ok || strings.TrimSpace(text) == "" {
				return ErrInvalidRequest
			}
			if role != "" {
				return ErrInvalidRequest
			}
		case "image_url":
			if !seedanceMediaObjectPresent(item, "image_url") {
				return ErrInvalidRequest
			}
			imageCount++
			switch role {
			case "", "first_frame":
				if role == "first_frame" {
					firstFrameCount++
				} else {
					implicitFirstFrameCount++
				}
			case "last_frame":
				lastFrameCount++
			case "reference_image":
				referenceImageCount++
			default:
				return ErrInvalidRequest
			}
		case "video_url":
			if !seedanceMediaObjectPresent(item, "video_url") {
				return ErrInvalidRequest
			}
			videoCount++
			if role != "reference_video" {
				return ErrInvalidRequest
			}
		case "audio_url":
			if !seedanceMediaObjectPresent(item, "audio_url") {
				return ErrInvalidRequest
			}
			audioCount++
			if role != "reference_audio" {
				return ErrInvalidRequest
			}
		default:
			return ErrInvalidRequest
		}
	}
	if imageCount > spec.maxReferenceImages || videoCount > spec.maxReferenceVideos || audioCount > spec.maxReferenceAudios {
		return ErrInvalidRequest
	}
	frameFirstCount := firstFrameCount + implicitFirstFrameCount
	if frameFirstCount > 1 || lastFrameCount > 1 || lastFrameCount > 0 && frameFirstCount == 0 {
		return ErrInvalidRequest
	}
	if implicitFirstFrameCount > 0 && (imageCount != 1 || videoCount > 0 || audioCount > 0 || referenceImageCount > 0 || lastFrameCount > 0) {
		return ErrInvalidRequest
	}
	if (frameFirstCount > 0 || lastFrameCount > 0) && (videoCount > 0 || audioCount > 0 || referenceImageCount > 0) {
		return ErrInvalidRequest
	}
	if audioCount > 0 && !spec.audioOnlyReference && imageCount == 0 && videoCount == 0 {
		return fmt.Errorf("%w: Seedance %s requires an image or video when audio is provided", ErrUnsupportedFeature, spec.version)
	}

	if _, ok := body["frames"]; ok {
		return fmt.Errorf("%w: Seedance %s does not support frames; use duration", ErrUnsupportedFeature, spec.version)
	}
	if _, ok := body["seed"]; ok {
		return fmt.Errorf("%w: Seedance %s does not support seed", ErrUnsupportedFeature, spec.version)
	}
	if _, ok := body["camera_fixed"]; ok {
		return fmt.Errorf("%w: Seedance %s does not support camera_fixed", ErrUnsupportedFeature, spec.version)
	}
	if _, ok := body["draft"]; ok {
		return fmt.Errorf("%w: Seedance %s does not support draft mode", ErrUnsupportedFeature, spec.version)
	}

	if value, ok := body["resolution"]; ok {
		resolution, valid := value.(string)
		if !valid {
			return ErrInvalidRequest
		}
		if _, supported := spec.resolutions[strings.ToLower(strings.TrimSpace(resolution))]; !supported {
			return ErrInvalidRequest
		}
	}
	if value, ok := body["ratio"]; ok {
		ratio, valid := value.(string)
		if !valid || !validSeedanceRatio(ratio) {
			return ErrInvalidRequest
		}
		taskType := seedanceTaskType(body)
		hasReferenceFrame := frameFirstCount > 0 || lastFrameCount > 0
		if spec.version == "2.5" && (taskType == "edit" || taskType == "extend" || hasReferenceFrame) &&
			strings.ToLower(strings.TrimSpace(ratio)) != "adaptive" {
			return fmt.Errorf("%w: Seedance 2.5 reference, edit and extend tasks require ratio=adaptive", ErrInvalidRequest)
		}
	}
	if value, ok := body["generate_audio"]; ok {
		if _, valid := value.(bool); !valid {
			return ErrInvalidRequest
		}
	}
	if value, ok := body["return_last_frame"]; ok {
		if _, valid := value.(bool); !valid {
			return ErrInvalidRequest
		}
		if !spec.supportsReturnLastFrame {
			return fmt.Errorf("%w: Seedance %s does not support return_last_frame", ErrUnsupportedFeature, spec.version)
		}
	}
	if value, ok := body["output_format"]; ok {
		if !spec.supportsOutputFormat {
			return fmt.Errorf("%w: Seedance %s supports only the default MP4 output", ErrUnsupportedFeature, spec.version)
		}
		format, valid := value.(string)
		if !valid || (strings.ToLower(strings.TrimSpace(format)) != "mp4" && strings.ToLower(strings.TrimSpace(format)) != "mov") {
			return ErrInvalidRequest
		}
	}
	if value, ok := body["omni_reference_task_type"]; ok {
		if !spec.supportsOmniTaskType {
			return fmt.Errorf("%w: Seedance %s does not support omni_reference_task_type", ErrUnsupportedFeature, spec.version)
		}
		taskType, valid := value.(string)
		if !valid || !validSeedanceTaskType(taskType) {
			return ErrInvalidRequest
		}
		taskType = strings.ToLower(strings.TrimSpace(taskType))
		if frameFirstCount > 0 || lastFrameCount > 0 {
			return ErrInvalidRequest
		}
		if (taskType == "edit" || taskType == "extend") && videoCount == 0 {
			return ErrInvalidRequest
		}
		if taskType == "reference" && imageCount+videoCount+audioCount == 0 {
			return ErrInvalidRequest
		}
		if (taskType == "edit" || taskType == "extend") && strings.TrimSpace(stringValue(body["duration"])) == "" {
			return ErrInvalidRequest
		}
		if spec.version == "2.5" && taskType == "edit" && stringValue(body["duration"]) != "-1" {
			return ErrInvalidRequest
		}
	}
	if value, ok := body["service_tier"]; ok {
		tier, valid := value.(string)
		if !valid {
			return ErrInvalidRequest
		}
		switch strings.ToLower(strings.TrimSpace(tier)) {
		case "", "default":
		case "flex":
			return fmt.Errorf("%w: Seedance %s does not support service_tier=flex", ErrUnsupportedFeature, spec.version)
		default:
			return ErrInvalidRequest
		}
	}
	if value, ok := body["execution_expires_after"]; ok {
		seconds, valid := jsonInteger(value)
		if !valid || seconds < 3600 || seconds > 259200 {
			return ErrInvalidRequest
		}
	}
	if value, ok := body["priority"]; ok {
		priority, valid := jsonInteger(value)
		if !valid || priority < 0 || priority > 9 {
			return ErrInvalidRequest
		}
	}
	if value, ok := body["tools"]; ok {
		tools, valid := seedanceToolItems(value)
		if !valid {
			return ErrInvalidRequest
		}
		for _, tool := range tools {
			if toolType, _ := tool["type"].(string); strings.TrimSpace(toolType) != "web_search" {
				return ErrInvalidRequest
			}
		}
	}

	duration, hasDuration := body["duration"]
	if hasDuration {
		seconds, valid := jsonFloat(duration)
		if !valid || (seconds != -1 && (seconds < 4 || seconds > spec.maxDurationSeconds)) {
			return ErrInvalidRequest
		}
		if seconds == -1 {
			taskType := seedanceTaskType(body)
			if taskType != "" && taskType != "auto" && taskType != "reference" && taskType != "edit" && taskType != "extend" {
				return ErrInvalidRequest
			}
		}
	} else if seedanceTaskType(body) != "extend" {
		return ErrInvalidRequest
	}
	return nil
}

func seedanceContentItems(value any) ([]map[string]any, bool) {
	switch items := value.(type) {
	case []any:
		result := make([]map[string]any, 0, len(items))
		for _, item := range items {
			object, ok := item.(map[string]any)
			if !ok {
				return nil, false
			}
			result = append(result, object)
		}
		return result, true
	case []map[string]any:
		return items, true
	default:
		return nil, false
	}
}

func seedanceToolItems(value any) ([]map[string]any, bool) {
	return seedanceContentItems(value)
}

func seedanceContentRole(item map[string]any) string {
	role, _ := item["role"].(string)
	return strings.ToLower(strings.TrimSpace(role))
}

func seedanceMediaObjectPresent(item map[string]any, key string) bool {
	value, ok := item[key].(map[string]any)
	if !ok || len(value) == 0 {
		return false
	}
	for _, candidate := range []string{"url", "uri", "file_id", "asset_id", "base64", "b64_json", "data"} {
		if value, ok := value[candidate].(string); ok && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func validSeedanceRatio(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "16:9", "4:3", "1:1", "3:4", "9:16", "21:9", "adaptive":
		return true
	default:
		return false
	}
}

func validSeedanceTaskType(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "auto", "reference", "edit", "extend":
		return true
	default:
		return false
	}
}

func seedanceTaskType(body map[string]any) string {
	value, _ := body["omni_reference_task_type"].(string)
	return strings.ToLower(strings.TrimSpace(value))
}

func jsonFloat(value any) (float64, bool) {
	switch item := value.(type) {
	case float64:
		return item, !math.IsNaN(item) && !math.IsInf(item, 0)
	case float32:
		return float64(item), !math.IsNaN(float64(item)) && !math.IsInf(float64(item), 0)
	case int:
		return float64(item), true
	case int64:
		return float64(item), true
	case json.Number:
		parsed, err := item.Float64()
		return parsed, err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(item), 64)
		return parsed, err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
	default:
		return 0, false
	}
}

func jsonInteger(value any) (int64, bool) {
	parsed, ok := jsonFloat(value)
	if !ok || math.Trunc(parsed) != parsed || parsed < math.MinInt64 || parsed > math.MaxInt64 {
		return 0, false
	}
	return int64(parsed), true
}

func emptyJSONList(value any) bool {
	if value == nil {
		return true
	}
	items, ok := value.([]any)
	return ok && len(items) == 0
}

func stringValue(value any) string {
	switch item := value.(type) {
	case string:
		return item
	case json.Number:
		return item.String()
	default:
		return strings.TrimSpace(strings.Trim(stringifyJSONValue(value), `"`))
	}
}

func stringifyJSONValue(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}

func parseVolcengineMediaUsage(data []byte) MediaUsage {
	usage := MediaUsage{
		Metrics: billing.MeteredUsage{},
		Source:  "upstream",
		Raw:     append(json.RawMessage(nil), data...),
	}
	var envelope struct {
		Usage struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			InputTokens      int64 `json:"input_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
			OutputTokens     int64 `json:"output_tokens"`
			TotalTokens      int64 `json:"total_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(data, &envelope) != nil {
		return usage
	}

	input := firstPositive(envelope.Usage.InputTokens, envelope.Usage.PromptTokens)
	output := firstPositive(envelope.Usage.CompletionTokens, envelope.Usage.OutputTokens)
	if output == 0 && envelope.Usage.TotalTokens > input {
		output = envelope.Usage.TotalTokens - input
	}
	usage.InputTokens = input
	usage.OutputTokens = output
	if input > 0 {
		usage.Metrics["input_tokens"] = strconv.FormatInt(input, 10)
	}
	if output > 0 {
		value := strconv.FormatInt(output, 10)
		usage.Metrics["output_tokens"] = value
		usage.Metrics["output_video_tokens"] = value
	}
	return usage
}

func firstPositive(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func volcengineUsageAuthoritative(usage MediaUsage) bool {
	value, ok := usage.Metrics["output_video_tokens"]
	if !ok {
		return false
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return err == nil && parsed > 0
}
