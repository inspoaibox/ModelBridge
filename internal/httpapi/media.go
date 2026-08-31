package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"ai-token/internal/auth"
	"ai-token/internal/relay"
)

const maxRelayUploadSize = 50 << 20

func relayMediaService(service relay.ChatCompletionService, w http.ResponseWriter) (relay.MediaCompletionService, bool) {
	media, ok := service.(relay.MediaCompletionService)
	if !ok || media == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "MEDIA_NOT_IMPLEMENTED"})
		return nil, false
	}
	return media, true
}

func relayMediaPrincipal(w http.ResponseWriter, r *http.Request) (*auth.Principal, bool) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
		return nil, false
	}
	if !auth.NetworkAllowlistAllows(principal, clientIP(r), r.Header.Get("Origin"), r.Header.Get("Referer")) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "TOKEN_NETWORK_NOT_ALLOWED"})
		return nil, false
	}
	return principal, true
}

func relayImageGenerationHandler(service relay.ChatCompletionService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		media, ok := relayMediaService(service, w)
		if !ok {
			return
		}
		principal, ok := relayMediaPrincipal(w, r)
		if !ok {
			return
		}
		payload, err := readMediaJSON(r, w)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		var value struct {
			Model  string `json:"model"`
			Prompt string `json:"prompt"`
			N      int64  `json:"n"`
		}
		if json.Unmarshal(payload, &value) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		request := relay.ImageGenerationRequest{Model: value.Model, Prompt: value.Prompt, Count: value.N, Payload: payload}
		setMediaRequestIDs(r, &request.RequestID, &request.IdempotencyKey)
		response, err := media.GenerateImages(relayRequestContext(r, "image_generation"), principal, request)
		if err != nil {
			writeRelayError(w, err)
			return
		}
		writeRawJSON(w, http.StatusOK, response.Body)
	})
}

func relayImageEditHandler(service relay.ChatCompletionService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		media, ok := relayMediaService(service, w)
		if !ok {
			return
		}
		principal, ok := relayMediaPrincipal(w, r)
		if !ok {
			return
		}
		fields, image, imageName, imageType, mask, maskName, maskType, err := readMediaMultipart(w, r, "image", true)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		request := relay.ImageEditRequest{Model: fields["model"], Fields: fields, Image: image, ImageName: imageName, ImageType: imageType, Mask: mask, MaskName: maskName, MaskType: maskType}
		setMediaRequestIDs(r, &request.RequestID, &request.IdempotencyKey)
		response, err := media.EditImage(relayRequestContext(r, "image_edit"), principal, request)
		if err != nil {
			writeRelayError(w, err)
			return
		}
		writeRawJSON(w, http.StatusOK, response.Body)
	})
}

func relayAudioHandler(service relay.ChatCompletionService, translate bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		media, ok := relayMediaService(service, w)
		if !ok {
			return
		}
		principal, ok := relayMediaPrincipal(w, r)
		if !ok {
			return
		}
		fields, file, fileName, fileType, _, _, _, err := readMediaMultipart(w, r, "file", false)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		request := relay.AudioRequest{Model: fields["model"], Fields: fields, File: file, FileName: fileName, FileType: fileType}
		setMediaRequestIDs(r, &request.RequestID, &request.IdempotencyKey)
		var response relay.MediaJSONResponse
		if translate {
			response, err = media.TranslateAudio(relayRequestContext(r, "audio_translation"), principal, request)
		} else {
			response, err = media.TranscribeAudio(relayRequestContext(r, "audio_transcription"), principal, request)
		}
		if err != nil {
			writeRelayError(w, err)
			return
		}
		writeRawJSON(w, http.StatusOK, response.Body)
	})
}

func relaySpeechHandler(service relay.ChatCompletionService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		media, ok := relayMediaService(service, w)
		if !ok {
			return
		}
		principal, ok := relayMediaPrincipal(w, r)
		if !ok {
			return
		}
		payload, err := readMediaJSON(r, w)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		var value struct {
			Model string `json:"model"`
			Input string `json:"input"`
		}
		if json.Unmarshal(payload, &value) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		request := relay.SpeechRequest{Model: value.Model, Input: value.Input, Payload: payload}
		setMediaRequestIDs(r, &request.RequestID, &request.IdempotencyKey)
		response, err := media.SynthesizeSpeech(relayRequestContext(r, "audio_speech"), principal, request)
		if err != nil {
			writeRelayError(w, err)
			return
		}
		contentType := response.ContentType
		if contentType == "" {
			contentType = "audio/mpeg"
		}
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(response.Body)
	})
}

func relayVideoCreateHandler(service relay.ChatCompletionService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		media, ok := relayMediaService(service, w)
		if !ok {
			return
		}
		principal, ok := relayMediaPrincipal(w, r)
		if !ok {
			return
		}
		payload, err := readMediaJSON(r, w)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		var value struct {
			Model   string          `json:"model"`
			Prompt  string          `json:"prompt"`
			Seconds json.RawMessage `json:"seconds"`
		}
		if json.Unmarshal(payload, &value) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_REQUEST"})
			return
		}
		request := relay.VideoCreateRequest{Model: value.Model, Prompt: value.Prompt, Duration: strings.Trim(string(value.Seconds), `"`), Payload: payload}
		setMediaRequestIDs(r, &request.RequestID, &request.IdempotencyKey)
		response, err := media.CreateVideo(relayRequestContext(r, "video_generation"), principal, request)
		if err != nil {
			writeRelayError(w, err)
			return
		}
		writeVideoJSON(w, response)
	})
}

func relayVideoGetHandler(service relay.ChatCompletionService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		media, ok := relayMediaService(service, w)
		if !ok {
			return
		}
		principal, ok := relayMediaPrincipal(w, r)
		if !ok {
			return
		}
		response, err := media.GetVideo(relayRequestContext(r, "video_status"), principal, r.PathValue("videoID"))
		if err != nil {
			writeRelayError(w, err)
			return
		}
		writeVideoJSON(w, response)
	})
}

func relayVideoContentHandler(service relay.ChatCompletionService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		media, ok := relayMediaService(service, w)
		if !ok {
			return
		}
		principal, ok := relayMediaPrincipal(w, r)
		if !ok {
			return
		}
		response, err := media.DownloadVideo(relayRequestContext(r, "video_content"), principal, r.PathValue("videoID"))
		if err != nil {
			writeRelayError(w, err)
			return
		}
		contentType := response.ContentType
		if contentType == "" {
			contentType = "video/mp4"
		}
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(response.Body)
	})
}

func readMediaJSON(r *http.Request, w http.ResponseWriter) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	defer r.Body.Close()
	data, err := io.ReadAll(r.Body)
	if err != nil || len(data) == 0 || !json.Valid(data) {
		return nil, errors.New("invalid media json")
	}
	return data, nil
}

func readMediaMultipart(w http.ResponseWriter, r *http.Request, requiredField string, withMask bool) (map[string]string, []byte, string, string, []byte, string, string, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRelayUploadSize)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return nil, nil, "", "", nil, "", "", err
	}
	fields := make(map[string]string, len(r.MultipartForm.Value))
	for key, values := range r.MultipartForm.Value {
		if len(key) > 64 || len(values) == 0 || len(values[0]) > 1<<20 {
			return nil, nil, "", "", nil, "", "", errors.New("invalid multipart field")
		}
		fields[key] = values[0]
	}
	file, name, contentType, err := readMediaPart(r, requiredField)
	if err != nil {
		return nil, nil, "", "", nil, "", "", err
	}
	var mask []byte
	var maskName, maskType string
	if withMask {
		mask, maskName, maskType, err = readMediaPartOptional(r, "mask")
		if err != nil {
			return nil, nil, "", "", nil, "", "", err
		}
	}
	return fields, file, name, contentType, mask, maskName, maskType, nil
}

func readMediaPart(r *http.Request, field string) ([]byte, string, string, error) {
	file, header, err := r.FormFile(field)
	if err != nil {
		return nil, "", "", err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxRelayUploadSize+1))
	if err != nil || len(data) > maxRelayUploadSize {
		return nil, "", "", errors.New("media file is too large")
	}
	if len(data) == 0 {
		return nil, "", "", errors.New("media file is empty")
	}
	return data, header.Filename, header.Header.Get("Content-Type"), nil
}

func readMediaPartOptional(r *http.Request, field string) ([]byte, string, string, error) {
	if _, ok := r.MultipartForm.File[field]; !ok {
		return nil, "", "", nil
	}
	return readMediaPart(r, field)
}

func setMediaRequestIDs(r *http.Request, requestID, idempotencyKey *string) {
	metadata := relay.RequestMetadataFromContext(r.Context())
	*requestID = metadata.RequestID
	*idempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if *idempotencyKey == "" {
		*idempotencyKey = metadata.RequestID
	}
}

func writeRawJSON(w http.ResponseWriter, status int, body []byte) {
	if len(body) == 0 || !json.Valid(body) {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "UPSTREAM_RESPONSE_INVALID"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func writeVideoJSON(w http.ResponseWriter, response relay.MediaJSONResponse) {
	var value map[string]any
	if json.Unmarshal(response.Body, &value) != nil {
		value = map[string]any{}
	}
	// Keep video status responses platform-shaped. Vendor operation names,
	// nested operation payloads and signed output URLs stay server-side.
	safe := map[string]any{}
	for _, key := range []string{"created_at", "completed_at", "expires_at", "progress"} {
		if item, ok := value[key]; ok {
			safe[key] = item
		}
	}
	safe["id"] = response.ID
	safe["object"] = "video"
	safe["status"] = response.Status
	writeJSON(w, http.StatusOK, safe)
}
