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

func TestGrokVoiceAndVideoUseXAIEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/stt":
			if r.Method != http.MethodPost || !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data;") {
				t.Fatalf("unexpected Grok STT request: %s %s", r.Method, r.URL.Path)
			}
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatal(err)
			}
			if _, _, err := r.FormFile("file"); err != nil {
				t.Fatal(err)
			}
			_, _ = io.WriteString(w, `{"text":"hello","usage":{"seconds":1.25}}`)
		case "/v1/tts":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected Grok TTS method: %s", r.Method)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["text"] != "hello" || body["voice_id"] != "eve" || body["model"] != nil {
				t.Fatalf("unexpected Grok TTS payload: %#v", body)
			}
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write([]byte("audio"))
		case "/v1/videos/generations":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected Grok video method: %s", r.Method)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["model"] != "grok-imagine-video-1.5" || body["prompt"] != "water" || body["duration"] != float64(12) || body["seconds"] != nil {
				t.Fatalf("unexpected Grok video payload: %#v", body)
			}
			_, _ = io.WriteString(w, `{"request_id":"req-1","status":"pending"}`)
		case "/v1/videos/req-1":
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected Grok video status method: %s", r.Method)
			}
			_, _ = io.WriteString(w, `{"request_id":"req-1","status":"done","video":{"url":"https://cdn.example.com/video.mp4"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	transcript, err := (GrokProvider{}).TranscribeAudio(context.Background(), UpstreamAudioRequest{Channel: Channel{BaseURL: server.URL + "/v1"}, APIKey: "xai-test", Request: AudioRequest{Model: "grok-stt", FileName: "sample.mp3", FileType: "audio/mpeg", File: []byte("audio")}})
	if err != nil || transcript.Usage.Metrics["input_audio_seconds"] != "1.250000" {
		t.Fatalf("unexpected Grok STT result: %#v %v", transcript, err)
	}
	speech, err := (GrokProvider{}).SynthesizeSpeech(context.Background(), UpstreamSpeechRequest{Channel: Channel{BaseURL: server.URL + "/v1"}, APIKey: "xai-test", Request: SpeechRequest{Model: "grok-tts", Input: "hello", Payload: json.RawMessage(`{"voice":"eve"}`)}})
	if err != nil || string(speech.Body) != "audio" {
		t.Fatalf("unexpected Grok TTS result: %#v %v", speech, err)
	}
	created, err := (GrokProvider{}).CreateVideo(context.Background(), UpstreamVideoRequest{Channel: Channel{BaseURL: server.URL + "/v1"}, APIKey: "xai-test", Request: VideoCreateRequest{Model: "grok-imagine-video-1.5", Prompt: "water", Duration: "12", Payload: json.RawMessage(`{"seconds":12}`)}})
	if err != nil || created.ID != "req-1" {
		t.Fatalf("unexpected Grok video create result: %#v %v", created, err)
	}
	status, err := (GrokProvider{}).GetVideo(context.Background(), UpstreamVideoRequest{Channel: Channel{BaseURL: server.URL + "/v1"}, APIKey: "xai-test"}, "req-1")
	if err != nil || status.Status != "done" {
		t.Fatalf("unexpected Grok video status result: %#v %v", status, err)
	}
}

func TestGrokDoesNotAdvertiseAnthropicStyleAudioTranslation(t *testing.T) {
	var provider any = GrokProvider{}
	if _, ok := provider.(AudioTranslationProvider); ok {
		t.Fatal("Grok must not advertise unsupported audio translation")
	}
}
