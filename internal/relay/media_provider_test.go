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

func TestOpenAIImageGenerationForwardsModelAndReturnsImageUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/images/generations" {
			t.Fatalf("unexpected image request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["model"] != "gpt-image-1" || body["prompt"] != "a lighthouse" || body["n"] != float64(2) {
			t.Fatalf("unexpected image payload: %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"created":1,"data":[{"b64_json":"one"},{"b64_json":"two"}],"usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7,"input_tokens_details":{"image_tokens":2},"output_tokens_details":{"image_tokens":4}}}`)
	}))
	defer server.Close()

	response, err := (OpenAIProvider{}).GenerateImages(context.Background(), UpstreamImageRequest{
		Channel: Channel{BaseURL: server.URL + "/v1"}, APIKey: "sk-test",
		Request: ImageGenerationRequest{Model: "gpt-image-1", Prompt: "a lighthouse", Count: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.ID != "" || response.Usage.Metrics["output_images"] != "2" || response.Usage.Metrics["input_image_tokens"] != "2" || response.Usage.Metrics["output_image_tokens"] != "4" {
		t.Fatalf("unexpected image response: %#v", response)
	}
}

func TestOpenAIAudioTranscriptionForwardsMultipartAndParsesDuration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/transcriptions" || !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data;") {
			t.Fatalf("unexpected audio request: %s %q", r.URL.Path, r.Header.Get("Content-Type"))
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		if r.FormValue("model") != "whisper-1" {
			t.Fatalf("model field was not forwarded: %q", r.FormValue("model"))
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		_ = file.Close()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"text":"hello","usage":{"type":"duration","seconds":2.5}}`)
	}))
	defer server.Close()

	response, err := (OpenAIProvider{}).TranscribeAudio(context.Background(), UpstreamAudioRequest{
		Channel: Channel{BaseURL: server.URL + "/v1"}, APIKey: "sk-test",
		Request: AudioRequest{Model: "whisper-1", FileName: "sample.wav", FileType: "audio/wav", File: []byte("audio")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(response.Body) != `{"text":"hello","usage":{"type":"duration","seconds":2.5}}` || response.Usage.Metrics["input_audio_seconds"] != "2.500000" {
		t.Fatalf("unexpected transcription result: %#v", response)
	}
}

func TestGeminiImageResponseRequiresGeneratedData(t *testing.T) {
	if _, err := geminiImageResponse([]byte(`{"predictions":[]}`), http.StatusOK, http.Header{}, 1); err == nil {
		t.Fatal("empty Gemini image response must not be treated as a successful free result")
	}
	response, err := geminiImageResponse([]byte(`{"predictions":[{"bytesBase64Encoded":"abc"}]}`), http.StatusOK, http.Header{}, 1)
	if err != nil || response.Usage.Metrics["output_images"] != "1" {
		t.Fatalf("unexpected Gemini image response: %#v %v", response, err)
	}
}

func TestParseMediaUsageSupportsOpenAIPluralTokenDetails(t *testing.T) {
	usage := parseMediaUsage([]byte(`{"usage":{"input_tokens":10,"output_tokens":8,"input_tokens_details":{"audio_tokens":3},"output_tokens_details":{"image_tokens":4}}}`))
	if usage.Metrics["input_audio_tokens"] != "3" || usage.Metrics["output_image_tokens"] != "4" {
		t.Fatalf("plural token details were not parsed: %#v", usage.Metrics)
	}
}

func TestParseGeminiMediaUsageMapsUsageMetadata(t *testing.T) {
	usage := parseGeminiMediaUsage([]byte(`{"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":6,"totalTokenCount":16,"promptTokensDetails":[{"modality":"AUDIO","tokenCount":7}],"candidatesTokensDetails":[{"modality":"IMAGE","tokenCount":4}]}}`))
	if usage.InputTokens != 10 || usage.OutputTokens != 6 || usage.Metrics["input_audio_tokens"] != "7" || usage.Metrics["output_image_tokens"] != "4" {
		t.Fatalf("Gemini usage metadata was not mapped: %#v", usage)
	}
}
