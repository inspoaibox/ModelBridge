package relay

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVolcengineVideoLifecycleUsesArkContentGenerationAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ark-test" {
			t.Fatalf("unexpected authorization header: %q", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v3/contents/generations/tasks":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["model"] != "doubao-seedance-2-5-260628" {
				t.Fatalf("upstream model mapping was not applied: %#v", body)
			}
			if _, ok := body["prompt"]; ok {
				t.Fatalf("OpenAI prompt must be converted to Ark content: %#v", body)
			}
			if _, ok := body["seconds"]; ok {
				t.Fatalf("OpenAI seconds must be converted to Ark duration: %#v", body)
			}
			if body["duration"] != float64(8) {
				t.Fatalf("unexpected Ark duration: %#v", body["duration"])
			}
			content, ok := body["content"].([]any)
			if !ok || len(content) != 1 {
				t.Fatalf("unexpected Ark content: %#v", body["content"])
			}
			item, ok := content[0].(map[string]any)
			if !ok || item["type"] != "text" || item["text"] != "a city at night" {
				t.Fatalf("unexpected generated text content: %#v", content)
			}
			if body["resolution"] != "1080p" || body["generate_audio"] != true {
				t.Fatalf("Ark extension fields were not preserved: %#v", body)
			}
			w.Header().Set("x-request-id", "ark-create-request")
			_, _ = io.WriteString(w, `{"id":"task-1","status":"queued"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/contents/generations/tasks/task-1":
			w.Header().Set("x-request-id", "ark-get-request")
			_, _ = io.WriteString(w, `{"id":"task-1","status":"succeeded","content":{"video_url":"https://cdn.example.com/seedance.mp4"},"usage":{"completion_tokens":108900,"total_tokens":108900}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := VolcengineProvider{}
	created, err := provider.CreateVideo(context.Background(), UpstreamVideoRequest{
		Channel: Channel{
			BaseURL:           server.URL,
			UpstreamModelName: "doubao-seedance-2-5-260628",
		},
		APIKey: "ark-test",
		Request: VideoCreateRequest{
			Model:    "seedance-pro",
			Prompt:   "a city at night",
			Duration: "8",
			Payload:  json.RawMessage(`{"model":"ignored","prompt":"ignored","seconds":8,"resolution":"1080p","generate_audio":true}`),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != "task-1" || created.Status != "queued" || created.ProviderRequestID != "ark-create-request" {
		t.Fatalf("unexpected create response: %#v", created)
	}

	completed, err := provider.GetVideo(context.Background(), UpstreamVideoRequest{
		Channel: Channel{BaseURL: server.URL},
		APIKey:  "ark-test",
	}, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if completed.ID != "task-1" || completed.Status != "succeeded" || completed.ProviderRequestID != "ark-get-request" {
		t.Fatalf("unexpected get response: %#v", completed)
	}
	if extractVideoURI(completed.Body) != "https://cdn.example.com/seedance.mp4" {
		t.Fatalf("Seedance video_url was not extracted: %s", completed.Body)
	}
	if completed.Usage.Metrics["output_video_tokens"] != "108900" ||
		completed.Usage.OutputTokens != 108900 ||
		completed.Usage.Source != "upstream" {
		t.Fatalf("unexpected Seedance usage: %#v", completed.Usage)
	}
	if !volcengineUsageAuthoritative(completed.Usage) {
		t.Fatal("completion_tokens should be authoritative video usage")
	}
}

func TestVolcenginePreservesExistingArkContentAndDropsPlatformFields(t *testing.T) {
	body, err := volcengineVideoRequestBody(VideoCreateRequest{
		Model:    "seedance",
		Prompt:   "fallback prompt",
		Duration: "5",
		Payload: json.RawMessage(`{
			"model":"wrong-model",
			"prompt":"wrong-prompt",
			"seconds":5,
			"content":[
				{"type":"text","text":"keep this"},
				{"type":"image_url","image_url":{"url":"https://example.com/reference.png"},"role":"reference_image"},
				{"type":"video_url","video_url":{"url":"https://example.com/reference.mp4"},"role":"reference_video"},
				{"type":"audio_url","audio_url":{"url":"https://example.com/reference.wav"},"role":"reference_audio"}
			],
			"ratio":"16:9",
			"watermark":false
		}`),
	}, "doubao-seedance-2-0-260128")
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	if value["model"] != "doubao-seedance-2-0-260128" || value["duration"] != float64(5) {
		t.Fatalf("model or duration was not normalized: %#v", value)
	}
	if _, ok := value["prompt"]; ok {
		t.Fatalf("platform prompt leaked into Ark request: %#v", value)
	}
	if _, ok := value["seconds"]; ok {
		t.Fatalf("platform seconds leaked into Ark request: %#v", value)
	}
	content, ok := value["content"].([]any)
	if !ok || len(content) != 4 {
		t.Fatalf("Ark content references were not preserved: %#v", value["content"])
	}
	if value["ratio"] != "16:9" || value["watermark"] != false {
		t.Fatalf("Ark extension fields were changed: %#v", value)
	}
}

func TestSeedance20And25ApplyDifferentDurationResolutionAndReferenceRules(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		payload string
		wantErr error
	}{
		{
			name:    "2.0 accepts 4k within 15 seconds",
			model:   "doubao-seedance-2-0-260128",
			payload: `{"content":[{"type":"text","text":"city"}],"duration":15,"resolution":"4k"}`,
		},
		{
			name:    "2.0 rejects duration above 15 seconds",
			model:   "doubao-seedance-2-0-260128",
			payload: `{"content":[{"type":"text","text":"city"}],"duration":16}`,
			wantErr: ErrInvalidRequest,
		},
		{
			name:    "2.0 rejects output format extension",
			model:   "doubao-seedance-2-0-260128",
			payload: `{"content":[{"type":"text","text":"city"}],"duration":5,"output_format":"mp4"}`,
			wantErr: ErrUnsupportedFeature,
		},
		{
			name:    "2.0 rejects frames",
			model:   "doubao-seedance-2-0-260128",
			payload: `{"content":[{"type":"text","text":"city"}],"frames":121}`,
			wantErr: ErrUnsupportedFeature,
		},
		{
			name:    "2.0 rejects audio-only content",
			model:   "doubao-seedance-2-0-260128",
			payload: `{"content":[{"type":"audio_url","audio_url":{"url":"https://example.com/a.wav"},"role":"reference_audio"}],"duration":5}`,
			wantErr: ErrUnsupportedFeature,
		},
		{
			name:    "2.0 accepts automatic duration",
			model:   "doubao-seedance-2-0-260128",
			payload: `{"content":[{"type":"text","text":"city"}],"duration":-1}`,
		},
		{
			name:    "2.5 accepts mov and 30 seconds",
			model:   "doubao-seedance-2-5-260628",
			payload: `{"content":[{"type":"text","text":"city"}],"duration":30,"resolution":"1080p","output_format":"mov"}`,
		},
		{
			name:    "2.5 rejects duration above 30 seconds",
			model:   "doubao-seedance-2-5-260628",
			payload: `{"content":[{"type":"text","text":"city"}],"duration":31}`,
			wantErr: ErrInvalidRequest,
		},
		{
			name:    "2.5 accepts audio-only content",
			model:   "doubao-seedance-2-5-260628",
			payload: `{"content":[{"type":"audio_url","audio_url":{"url":"https://example.com/a.wav"},"role":"reference_audio"}],"duration":5}`,
		},
		{
			name:    "2.5 rejects 4k",
			model:   "doubao-seedance-2-5-260628",
			payload: `{"content":[{"type":"text","text":"city"}],"duration":5,"resolution":"4k"}`,
			wantErr: ErrInvalidRequest,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := volcengineVideoRequestBody(VideoCreateRequest{
				Model:   test.model,
				Prompt:  "fallback",
				Payload: json.RawMessage(test.payload),
			}, test.model)
			if test.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want errors.Is(..., %v)", err, test.wantErr)
			}
		})
	}
}

func TestSeedance20And25RejectLegacyOnlyParameters(t *testing.T) {
	for _, model := range []string{"doubao-seedance-2-0-260128", "doubao-seedance-2-5-260628"} {
		for _, field := range []string{"seed", "camera_fixed", "draft"} {
			t.Run(model+"/"+field, func(t *testing.T) {
				payload := `{"content":[{"type":"text","text":"city"}],"duration":5,"` + field + `":false}`
				_, err := volcengineVideoRequestBody(VideoCreateRequest{
					Model:   model,
					Prompt:  "fallback",
					Payload: json.RawMessage(payload),
				}, model)
				if !errors.Is(err, ErrUnsupportedFeature) {
					t.Fatalf("%s = %v, want ErrUnsupportedFeature", field, err)
				}
			})
		}
	}
}

func TestSeedance25TaskTypeAndAdvancedFields(t *testing.T) {
	body, err := volcengineVideoRequestBody(VideoCreateRequest{
		Model: "doubao-seedance-2-5-260628",
		Payload: json.RawMessage(`{
			"content":[
				{"type":"video_url","video_url":{"url":"https://example.com/source.mp4"},"role":"reference_video"}
			],
			"omni_reference_task_type":"edit",
			"duration":-1,
			"ratio":"adaptive",
			"return_last_frame":true,
			"execution_expires_after":3600,
			"priority":9,
			"tools":[{"type":"web_search"}],
			"generate_audio":true
		}`),
	}, "doubao-seedance-2-5-260628")
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	if value["duration"] != float64(-1) || value["omni_reference_task_type"] != "edit" ||
		value["output_format"] != nil {
		t.Fatalf("unexpected Seedance 2.5 edit payload: %#v", value)
	}

	_, err = volcengineVideoRequestBody(VideoCreateRequest{
		Model: "doubao-seedance-2-0-260128",
		Payload: json.RawMessage(`{
			"content":[{"type":"text","text":"city"}],
			"omni_reference_task_type":"reference",
			"duration":5
		}`),
	}, "doubao-seedance-2-0-260128")
	if !errors.Is(err, ErrUnsupportedFeature) {
		t.Fatalf("Seedance 2.0 task type = %v, want ErrUnsupportedFeature", err)
	}
}

func TestSeedanceReferenceRolesAndTaskModes(t *testing.T) {
	valid := func(model, payload string) {
		t.Helper()
		if _, err := volcengineVideoRequestBody(VideoCreateRequest{Model: model, Payload: json.RawMessage(payload)}, model); err != nil {
			t.Fatalf("valid payload rejected: %v", err)
		}
	}
	reject := func(model, payload string, want error) {
		t.Helper()
		_, err := volcengineVideoRequestBody(VideoCreateRequest{Model: model, Payload: json.RawMessage(payload)}, model)
		if !errors.Is(err, want) {
			t.Fatalf("error = %v, want errors.Is(..., %v)", err, want)
		}
	}

	valid("doubao-seedance-2-0-260128", `{"content":[{"type":"image_url","image_url":{"url":"https://example.com/first.png"}}],"duration":5}`)
	valid("doubao-seedance-2-0-260128", `{"content":[{"type":"image_url","image_url":{"url":"https://example.com/first.png"},"role":"first_frame"},{"type":"image_url","image_url":{"url":"https://example.com/last.png"},"role":"last_frame"}],"duration":5}`)
	valid("doubao-seedance-2-5-260628", `{"content":[{"type":"image_url","image_url":{"url":"https://example.com/reference.png"},"role":"reference_image"},{"type":"audio_url","audio_url":{"url":"https://example.com/reference.wav"},"role":"reference_audio"}],"duration":-1}`)

	reject("doubao-seedance-2-0-260128", `{"content":[{"type":"video_url","video_url":{"url":"https://example.com/source.mp4"}}],"duration":5}`, ErrInvalidRequest)
	reject("doubao-seedance-2-5-260628", `{"content":[{"type":"image_url","image_url":{"url":"https://example.com/first.png"},"role":"first_frame"},{"type":"video_url","video_url":{"url":"https://example.com/source.mp4"},"role":"reference_video"}],"duration":5}`, ErrInvalidRequest)
	reject("doubao-seedance-2-5-260628", `{"content":[{"type":"video_url","video_url":{"url":"https://example.com/source.mp4"},"role":"reference_video"}],"omni_reference_task_type":"edit","duration":5,"ratio":"adaptive"}`, ErrInvalidRequest)
}

func TestVideoValidationAllowsPromptFromContentPayload(t *testing.T) {
	request := VideoCreateRequest{
		Model:   "seedance",
		Payload: json.RawMessage(`{"content":[{"type":"text","text":"a red bridge at dawn"}],"seconds":5}`),
	}
	if err := validateVideoCreateRequest(&request); err != nil {
		t.Fatal(err)
	}
	if request.Prompt != "a red bridge at dawn" {
		t.Fatalf("content text was not used as validation prompt: %q", request.Prompt)
	}
}

func TestVideoValidationAllowsFramesInsteadOfDuration(t *testing.T) {
	request := VideoCreateRequest{
		Model:   "seedance",
		Prompt:  "a stop-motion paper city",
		Payload: json.RawMessage(`{"frames":120}`),
	}
	if err := validateVideoCreateRequest(&request); err != nil {
		t.Fatal(err)
	}
	if !validVideoFrames(request.Payload) {
		t.Fatal("positive frames should be accepted")
	}
}

func TestVolcengineModelDiscoveryUsesOfficialModelsEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v3/models" {
			t.Fatalf("unexpected model discovery request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer ark-test" {
			t.Fatalf("unexpected authorization header: %q", r.Header.Get("Authorization"))
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"doubao-seed-1-6-260615"},{"id":"doubao-seedance-2-0-260128"},{"id":"doubao-seedance-2-5-260628","display_name":"Seedance 2.5"}]}`)
	}))
	defer server.Close()

	models, err := (VolcengineProvider{}).ListModels(context.Background(), server.URL+"/api/v3/", "ark-test")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].Provider != ProviderVolcengine || models[1].DisplayName != "Seedance 2.5" {
		t.Fatalf("unexpected discovered models: %#v", models)
	}
}

func TestVolcengineBaseURLAndAliases(t *testing.T) {
	for _, test := range []struct {
		raw  string
		want string
	}{
		{"https://ark.cn-beijing.volces.com", "https://ark.cn-beijing.volces.com/api/v3"},
		{"https://ark.cn-beijing.volces.com/api/v3/", "https://ark.cn-beijing.volces.com/api/v3"},
		{"https://ark.cn-beijing.volces.com/api/v3", "https://ark.cn-beijing.volces.com/api/v3"},
	} {
		got, err := volcengineBaseURL(test.raw)
		if err != nil || got != test.want {
			t.Fatalf("volcengineBaseURL(%q) = %q, %v; want %q", test.raw, got, err, test.want)
		}
	}
	for _, alias := range []string{"ark", "byteplus", "bytedance", "volc", "volcengine-ark"} {
		if canonicalProvider(alias) != ProviderVolcengine {
			t.Fatalf("provider alias %q was not normalized", alias)
		}
	}
	if !supportedProvider(ProviderVolcengine) {
		t.Fatal("volcengine provider should be supported")
	}
	if _, ok := DefaultProviders()[ProviderVolcengine]; !ok {
		t.Fatal("default providers should include volcengine")
	}
	if capabilities := modelCapabilities(ProviderVolcengine, "doubao-seedance-2-5-260628"); !strings.Contains(capabilities, `"video_generation":true`) {
		t.Fatalf("Seedance must be registered as a video model: %s", capabilities)
	}
	if capabilities := modelCapabilities(ProviderVolcengine, "doubao-seedance-2-0-260128"); !strings.Contains(capabilities, `"seedance_version":"2.0"`) ||
		!strings.Contains(capabilities, `"supports_4k":true`) {
		t.Fatalf("Seedance 2.0 capabilities were not differentiated: %s", capabilities)
	}
	if capabilities := modelCapabilities(ProviderVolcengine, "doubao-seedance-2-5-260628"); !strings.Contains(capabilities, `"seedance_version":"2.5"`) ||
		!strings.Contains(capabilities, `"supports_output_format":true`) ||
		!strings.Contains(capabilities, `"audio_only_reference":true`) {
		t.Fatalf("Seedance 2.5 capabilities were not differentiated: %s", capabilities)
	}
}

func TestVolcengineUnsupportedChatAndInvalidDownload(t *testing.T) {
	if _, err := (VolcengineProvider{}).ChatCompletions(context.Background(), UpstreamChatCompletionRequest{}); !errors.Is(err, ErrUnsupportedFeature) {
		t.Fatalf("unexpected chat capability error: %v", err)
	}
	if _, err := (VolcengineProvider{}).DownloadVideo(context.Background(), UpstreamVideoRequest{}, "http://cdn.example.com/video.mp4"); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("insecure video URL must be rejected: %v", err)
	}
}

func TestNormalizeVideoStatusTreatsArkExpiredAsFailed(t *testing.T) {
	if got := normalizeVideoStatus("expired"); got != "failed" {
		t.Fatalf("normalizeVideoStatus(expired) = %q", got)
	}
}

func TestVolcengineVideoRequestBodyRejectsMalformedPayload(t *testing.T) {
	_, err := volcengineVideoRequestBody(VideoCreateRequest{
		Model:   "seedance",
		Prompt:  "prompt",
		Payload: json.RawMessage(`{"content":`),
	}, "doubao-seedance-2-0-260128")
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("malformed payload should be rejected: %v", err)
	}
	if strings.TrimSpace(stringValue(float64(5))) != "5" {
		t.Fatal("number conversion helper changed unexpectedly")
	}
}
