package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ai-token/internal/auth"
	"ai-token/internal/billing"
	"ai-token/internal/ids"
)

const (
	maxMediaFileSize    = 50 << 20
	maxMediaJSONSize    = 8 << 20
	mediaReservationTTL = 2 * time.Hour
)

// MediaJSONResponse carries the upstream JSON unchanged while keeping the
// usage information needed by the billing lifecycle separate from the wire
// representation returned to the caller.
type MediaJSONResponse struct {
	Body              json.RawMessage
	Usage             MediaUsage
	ProviderRequestID string
	ID                string
	Status            string
}

type MediaBinaryResponse struct {
	Body              []byte
	ContentType       string
	ProviderRequestID string
	Usage             MediaUsage
}

type MediaUsage struct {
	Metrics           billing.MeteredUsage
	InputTokens       int64
	OutputTokens      int64
	CachedInputTokens int64
	ReasoningTokens   int64
	Source            string
	Raw               json.RawMessage
}

type ImageGenerationRequest struct {
	Model          string
	Prompt         string
	Count          int64
	Payload        json.RawMessage
	RequestID      string
	IdempotencyKey string
}

type ImageEditRequest struct {
	Model          string
	Fields         map[string]string
	ImageName      string
	ImageType      string
	Image          []byte
	MaskName       string
	MaskType       string
	Mask           []byte
	RequestID      string
	IdempotencyKey string
}

type AudioRequest struct {
	Model          string
	Fields         map[string]string
	FileName       string
	FileType       string
	File           []byte
	RequestID      string
	IdempotencyKey string
}

type SpeechRequest struct {
	Model          string
	Input          string
	Payload        json.RawMessage
	RequestID      string
	IdempotencyKey string
}

type VideoCreateRequest struct {
	Model          string
	Prompt         string
	Duration       string
	Payload        json.RawMessage
	RequestID      string
	IdempotencyKey string
}

type MediaJob struct {
	ID                string
	TenantID          string
	ProjectID         string
	TokenID           string
	GroupID           string
	Model             string
	UpstreamModelName string
	Provider          string
	Channel           Channel
	UpstreamJobID     string
	Status            string
	ReservationID     string
	ModelRequestID    string
	ProviderRequestID string
	OutputURI         string
	Response          json.RawMessage
	EstimatedMetrics  billing.MeteredUsage
	FailureReason     string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	CompletedAt       *time.Time
}

type MediaJobStore interface {
	CreateMediaJob(context.Context, MediaJob) error
	GetMediaJob(context.Context, string, string, string) (MediaJob, error)
	UpdateMediaJob(context.Context, string, string, string, json.RawMessage, string) error
	ListPendingMediaJobs(context.Context, int) ([]MediaJob, error)
}

type ImageGenerationProvider interface {
	GenerateImages(context.Context, UpstreamImageRequest) (MediaJSONResponse, error)
	EditImage(context.Context, UpstreamImageEditRequest) (MediaJSONResponse, error)
}

type AudioProvider interface {
	TranscribeAudio(context.Context, UpstreamAudioRequest) (MediaJSONResponse, error)
	TranslateAudio(context.Context, UpstreamAudioRequest) (MediaJSONResponse, error)
	SynthesizeSpeech(context.Context, UpstreamSpeechRequest) (MediaBinaryResponse, error)
}

type AudioTranscriptionProvider interface {
	TranscribeAudio(context.Context, UpstreamAudioRequest) (MediaJSONResponse, error)
}

type AudioTranslationProvider interface {
	TranslateAudio(context.Context, UpstreamAudioRequest) (MediaJSONResponse, error)
}

type SpeechProvider interface {
	SynthesizeSpeech(context.Context, UpstreamSpeechRequest) (MediaBinaryResponse, error)
}

type VideoProvider interface {
	CreateVideo(context.Context, UpstreamVideoRequest) (MediaJSONResponse, error)
	GetVideo(context.Context, UpstreamVideoRequest, string) (MediaJSONResponse, error)
	DownloadVideo(context.Context, UpstreamVideoRequest, string) (MediaBinaryResponse, error)
}

type UpstreamImageRequest struct {
	Channel       Channel
	APIKey        string
	Request       ImageGenerationRequest
	UpstreamModel string
}

type UpstreamImageEditRequest struct {
	Channel       Channel
	APIKey        string
	Request       ImageEditRequest
	UpstreamModel string
}

type UpstreamAudioRequest struct {
	Channel       Channel
	APIKey        string
	Request       AudioRequest
	UpstreamModel string
}

type UpstreamSpeechRequest struct {
	Channel       Channel
	APIKey        string
	Request       SpeechRequest
	UpstreamModel string
}

type UpstreamVideoRequest struct {
	Channel       Channel
	APIKey        string
	Request       VideoCreateRequest
	UpstreamModel string
}

type MediaCompletionService interface {
	GenerateImages(context.Context, *auth.Principal, ImageGenerationRequest) (MediaJSONResponse, error)
	EditImage(context.Context, *auth.Principal, ImageEditRequest) (MediaJSONResponse, error)
	TranscribeAudio(context.Context, *auth.Principal, AudioRequest) (MediaJSONResponse, error)
	TranslateAudio(context.Context, *auth.Principal, AudioRequest) (MediaJSONResponse, error)
	SynthesizeSpeech(context.Context, *auth.Principal, SpeechRequest) (MediaBinaryResponse, error)
	CreateVideo(context.Context, *auth.Principal, VideoCreateRequest) (MediaJSONResponse, error)
	GetVideo(context.Context, *auth.Principal, string) (MediaJSONResponse, error)
	DownloadVideo(context.Context, *auth.Principal, string) (MediaBinaryResponse, error)
}

func (s *Service) GenerateImages(ctx context.Context, principal *auth.Principal, request ImageGenerationRequest) (MediaJSONResponse, error) {
	if err := validateImageGenerationRequest(&request); err != nil {
		return MediaJSONResponse{}, err
	}
	estimated := billing.MeteredUsage{"output_images": strconv.FormatInt(request.Count, 10)}
	return s.executeMediaJSON(ctx, principal, request.Model, "image_generation", estimated, request.RequestID, request.IdempotencyKey,
		func(channel Channel, key string) (MediaJSONResponse, error) {
			provider := s.providers[canonicalProvider(channel.Provider)]
			imageProvider, ok := provider.(ImageGenerationProvider)
			if !ok {
				return MediaJSONResponse{}, ErrProviderUnsupported
			}
			return imageProvider.GenerateImages(ctx, UpstreamImageRequest{Channel: channel, APIKey: key, Request: request, UpstreamModel: upstreamModelFor(channel, request.Model)})
		})
}

func (s *Service) EditImage(ctx context.Context, principal *auth.Principal, request ImageEditRequest) (MediaJSONResponse, error) {
	if err := validateImageEditRequest(&request); err != nil {
		return MediaJSONResponse{}, err
	}
	estimated := billing.MeteredUsage{"input_images": "1", "output_images": "1"}
	return s.executeMediaJSON(ctx, principal, request.Model, "image_edit", estimated, request.RequestID, request.IdempotencyKey,
		func(channel Channel, key string) (MediaJSONResponse, error) {
			provider := s.providers[canonicalProvider(channel.Provider)]
			imageProvider, ok := provider.(ImageGenerationProvider)
			if !ok {
				return MediaJSONResponse{}, ErrProviderUnsupported
			}
			return imageProvider.EditImage(ctx, UpstreamImageEditRequest{Channel: channel, APIKey: key, Request: request, UpstreamModel: upstreamModelFor(channel, request.Model)})
		})
}

func (s *Service) TranscribeAudio(ctx context.Context, principal *auth.Principal, request AudioRequest) (MediaJSONResponse, error) {
	return s.audioRequest(ctx, principal, request, false)
}

func (s *Service) TranslateAudio(ctx context.Context, principal *auth.Principal, request AudioRequest) (MediaJSONResponse, error) {
	return s.audioRequest(ctx, principal, request, true)
}

func (s *Service) audioRequest(ctx context.Context, principal *auth.Principal, request AudioRequest, translate bool) (MediaJSONResponse, error) {
	if err := validateAudioRequest(&request); err != nil {
		return MediaJSONResponse{}, err
	}
	estimated := billing.MeteredUsage{}
	if seconds := wavDurationSeconds(request.File); seconds != "" {
		estimated["input_audio_seconds"] = seconds
	}
	requestType := "audio_transcription"
	if translate {
		requestType = "audio_translation"
	}
	return s.executeMediaJSON(ctx, principal, request.Model, requestType, estimated, request.RequestID, request.IdempotencyKey,
		func(channel Channel, key string) (MediaJSONResponse, error) {
			provider := s.providers[canonicalProvider(channel.Provider)]
			upstream := UpstreamAudioRequest{Channel: channel, APIKey: key, Request: request, UpstreamModel: upstreamModelFor(channel, request.Model)}
			if translate {
				translator, ok := provider.(AudioTranslationProvider)
				if !ok {
					return MediaJSONResponse{}, ErrProviderUnsupported
				}
				return translator.TranslateAudio(ctx, upstream)
			}
			transcriber, ok := provider.(AudioTranscriptionProvider)
			if !ok {
				return MediaJSONResponse{}, ErrProviderUnsupported
			}
			return transcriber.TranscribeAudio(ctx, upstream)
		})
}

func (s *Service) SynthesizeSpeech(ctx context.Context, principal *auth.Principal, request SpeechRequest) (MediaBinaryResponse, error) {
	if err := validateSpeechRequest(&request); err != nil {
		return MediaBinaryResponse{}, err
	}
	estimated := billing.MeteredUsage{"input_characters": strconv.FormatInt(int64(len([]rune(request.Input))), 10)}
	return s.executeMediaBinary(ctx, principal, request.Model, "audio_speech", estimated, request.RequestID, request.IdempotencyKey,
		func(channel Channel, key string) (MediaBinaryResponse, error) {
			provider := s.providers[canonicalProvider(channel.Provider)]
			speechProvider, ok := provider.(SpeechProvider)
			if !ok {
				return MediaBinaryResponse{}, ErrProviderUnsupported
			}
			return speechProvider.SynthesizeSpeech(ctx, UpstreamSpeechRequest{Channel: channel, APIKey: key, Request: request, UpstreamModel: upstreamModelFor(channel, request.Model)})
		})
}

func (s *Service) CreateVideo(ctx context.Context, principal *auth.Principal, request VideoCreateRequest) (MediaJSONResponse, error) {
	if err := validateVideoCreateRequest(&request); err != nil {
		return MediaJSONResponse{}, err
	}
	policy, candidates, billingEnabled, _, err := s.prepareMedia(ctx, principal, request.Model, "video_generation", request.RequestID, request.IdempotencyKey, 1)
	if err != nil {
		return MediaJSONResponse{}, err
	}
	release, err := s.acquireMediaLimiter(ctx, principal)
	if err != nil {
		return MediaJSONResponse{}, err
	}
	defer release()
	store, ok := s.router.(MediaJobStore)
	if !ok {
		return MediaJSONResponse{}, ErrUnavailable
	}
	attempted := map[string]struct{}{}
	var lastErr error
	var reservation billing.Reservation
	var freeID string
	for len(attempted) < len(candidates) {
		channel, found := pickWeightedChannel(candidates, attempted)
		if !found {
			break
		}
		attempted[channelKey(channel)] = struct{}{}
		provider := s.providers[canonicalProvider(channel.Provider)]
		videoProvider, supported := provider.(VideoProvider)
		if !supported {
			lastErr = ErrProviderUnsupported
			continue
		}
		key, resolveErr := s.credentials.Resolve(ctx, channel.CredentialRef)
		if resolveErr != nil {
			lastErr = resolveErr
			s.recordChannelFailure(ctx, channel, resolveErr)
			continue
		}
		if reservation.ID == "" && freeID == "" {
			reservation, freeID, err = s.startMediaBilling(ctx, principal, request.Model, "video_generation", request.RequestID, request.IdempotencyKey, policy, channel, billingEnabled, videoEstimatedMetrics(request))
			if err != nil {
				return MediaJSONResponse{}, err
			}
		}
		response, callErr := videoProvider.CreateVideo(ctx, UpstreamVideoRequest{Channel: channel, APIKey: key, Request: request, UpstreamModel: upstreamModelFor(channel, request.Model)})
		if callErr != nil {
			lastErr = callErr
			s.recordChannelFailure(ctx, channel, callErr)
			if ctx.Err() != nil || !retryableUpstreamError(callErr) {
				break
			}
			continue
		}
		jobID := strings.TrimSpace(response.ID)
		if jobID == "" {
			jobID = extractJSONString(response.Body, "name")
		}
		if jobID == "" {
			s.failRelayBilling(ctx, reservation, freeID, "upstream_video_job_id_missing")
			return MediaJSONResponse{}, ErrUpstream
		}
		localID := newMediaJobID()
		modelRequestID := reservation.ModelRequestID
		if modelRequestID == "" {
			modelRequestID = freeID
		}
		job := MediaJob{ID: localID, TenantID: principal.TenantID, ProjectID: principalProjectID(principal), TokenID: principal.TokenID, GroupID: policy.ID, Model: request.Model, UpstreamModelName: upstreamModelFor(channel, request.Model), Provider: canonicalProvider(channel.Provider), Channel: channel, UpstreamJobID: jobID, Status: normalizeVideoStatus(response.Status), ReservationID: reservation.ID, ModelRequestID: modelRequestID, ProviderRequestID: response.ProviderRequestID, Response: response.Body, EstimatedMetrics: videoEstimatedMetrics(request)}
		if job.Status == "" {
			job.Status = "queued"
		}
		if err := store.CreateMediaJob(ctx, job); err != nil {
			s.failRelayBilling(ctx, reservation, freeID, "media_job_persist_failed")
			return MediaJSONResponse{}, err
		}
		if job.Status == "completed" || job.Status == "failed" || job.Status == "cancelled" {
			if err := s.finishVideoJob(ctx, job, job.Status, response); err != nil {
				return MediaJSONResponse{}, err
			}
		}
		s.recordChannelSuccess(ctx, channel)
		response.ID = localID
		response.Status = job.Status
		return response, nil
	}
	s.failRelayBilling(ctx, reservation, freeID, "upstream_video_create_failed")
	if lastErr == nil {
		lastErr = ErrProviderUnsupported
	}
	return MediaJSONResponse{}, lastErr
}

func (s *Service) GetVideo(ctx context.Context, principal *auth.Principal, localID string) (MediaJSONResponse, error) {
	job, provider, key, err := s.loadVideoJob(ctx, principal, localID)
	if err != nil {
		return MediaJSONResponse{}, err
	}
	upstreamModel := firstNonEmpty(job.UpstreamModelName, job.Channel.UpstreamModelName, job.Model)
	response, err := provider.GetVideo(ctx, UpstreamVideoRequest{Channel: job.Channel, APIKey: key, Request: VideoCreateRequest{Model: job.Model}, UpstreamModel: upstreamModel}, job.UpstreamJobID)
	if err != nil {
		return MediaJSONResponse{}, err
	}
	status := normalizeVideoStatus(response.Status)
	if status == "" {
		status = normalizeVideoStatus(extractJSONString(response.Body, "status"))
	}
	if status == "" {
		status = job.Status
	}
	store, _ := s.router.(MediaJobStore)
	if outputURI := extractVideoURI(response.Body); outputURI != "" {
		job.OutputURI = outputURI
	}
	if status == "completed" || status == "failed" || status == "cancelled" {
		if err := s.finishVideoJob(ctx, job, status, response); err != nil {
			return MediaJSONResponse{}, err
		}
	}
	if store != nil {
		if err := store.UpdateMediaJob(ctx, job.ID, status, job.OutputURI, response.Body, ""); err != nil {
			return MediaJSONResponse{}, err
		}
	}
	response.ID = job.ID
	response.Status = status
	return response, nil
}

// ReconcileMediaJobs settles terminal video operations even when a downstream
// client stops polling. The operation is idempotent at the billing layer, so a
// retry after a process restart cannot create a second charge.
func (s *Service) ReconcileMediaJobs(ctx context.Context) error {
	if s == nil || s.router == nil || s.credentials == nil {
		return ErrUnavailable
	}
	store, ok := s.router.(MediaJobStore)
	if !ok {
		return ErrUnavailable
	}
	jobs, err := store.ListPendingMediaJobs(ctx, 50)
	if err != nil {
		return err
	}
	failures := 0
	for _, job := range jobs {
		provider, ok := s.providers[canonicalProvider(job.Provider)].(VideoProvider)
		if !ok {
			continue
		}
		if extender, ok := s.billing.(billing.ReservationExtender); ok && strings.TrimSpace(job.ReservationID) != "" {
			if extendErr := extender.ExtendReservation(ctx, job.ReservationID, mediaReservationTTL); extendErr != nil {
				failures++
				continue
			}
		}
		key, resolveErr := s.credentials.Resolve(ctx, job.Channel.CredentialRef)
		if resolveErr != nil {
			failures++
			continue
		}
		upstreamModel := firstNonEmpty(job.UpstreamModelName, job.Channel.UpstreamModelName, job.Model)
		request := UpstreamVideoRequest{Channel: job.Channel, APIKey: key, Request: VideoCreateRequest{Model: job.Model}, UpstreamModel: upstreamModel}
		response, getErr := provider.GetVideo(ctx, request, job.UpstreamJobID)
		if getErr != nil {
			failures++
			continue
		}
		status := normalizeVideoStatus(response.Status)
		if status == "" {
			status = normalizeVideoStatus(extractJSONString(response.Body, "status"))
		}
		if status == "" || status == "queued" || status == "processing" {
			if status != "" {
				if updateErr := store.UpdateMediaJob(ctx, job.ID, status, job.OutputURI, response.Body, ""); updateErr != nil {
					failures++
				}
			}
			continue
		}
		outputURI := extractVideoURI(response.Body)
		if err := s.finishVideoJob(ctx, job, status, response); err != nil {
			failures++
			continue
		}
		if err := store.UpdateMediaJob(ctx, job.ID, status, outputURI, response.Body, ""); err != nil {
			failures++
		}
	}
	if failures > 0 {
		return fmt.Errorf("media job reconciliation failed for %d job(s)", failures)
	}
	return nil
}

func (s *Service) DownloadVideo(ctx context.Context, principal *auth.Principal, localID string) (MediaBinaryResponse, error) {
	job, provider, key, err := s.loadVideoJob(ctx, principal, localID)
	if err != nil {
		return MediaBinaryResponse{}, err
	}
	if job.Status != "completed" {
		return MediaBinaryResponse{}, ErrInvalidRequest
	}
	upstreamModel := firstNonEmpty(job.UpstreamModelName, job.Channel.UpstreamModelName, job.Model)
	return provider.DownloadVideo(ctx, UpstreamVideoRequest{Channel: job.Channel, APIKey: key, Request: VideoCreateRequest{Model: job.Model}, UpstreamModel: upstreamModel}, firstNonEmpty(job.OutputURI, job.UpstreamJobID))
}

func (s *Service) loadVideoJob(ctx context.Context, principal *auth.Principal, localID string) (MediaJob, VideoProvider, string, error) {
	if s == nil || s.router == nil || s.credentials == nil || principal == nil {
		return MediaJob{}, nil, "", ErrUnavailable
	}
	store, ok := s.router.(MediaJobStore)
	if !ok {
		return MediaJob{}, nil, "", ErrUnavailable
	}
	job, err := store.GetMediaJob(ctx, strings.TrimSpace(localID), principal.TenantID, principal.TokenID)
	if err != nil {
		return MediaJob{}, nil, "", err
	}
	provider, ok := s.providers[canonicalProvider(job.Provider)].(VideoProvider)
	if !ok {
		return MediaJob{}, nil, "", ErrProviderUnsupported
	}
	key, err := s.credentials.Resolve(ctx, job.Channel.CredentialRef)
	if err != nil {
		return MediaJob{}, nil, "", err
	}
	return job, provider, key, nil
}

func (s *Service) finishVideoJob(ctx context.Context, job MediaJob, status string, response MediaJSONResponse) error {
	if status == "failed" || status == "cancelled" {
		if strings.TrimSpace(job.ReservationID) != "" {
			s.failRelayBilling(ctx, billing.Reservation{ID: job.ReservationID}, "", "upstream_video_"+status)
		} else if strings.TrimSpace(job.ModelRequestID) != "" {
			s.failRelayBilling(ctx, billing.Reservation{}, job.ModelRequestID, "upstream_video_"+status)
		}
		return nil
	}
	if status != "completed" {
		return ErrInvalidRequest
	}
	response.Usage = mergeMediaUsage(response.Usage, job.EstimatedMetrics)
	if len(response.Usage.Metrics) == 0 && strings.TrimSpace(job.ReservationID) != "" {
		if marker, ok := s.billing.(billing.ReservationPendingMarker); ok {
			if err := marker.MarkSettlementPending(ctx, job.ReservationID, "upstream_video_usage_unavailable"); err == nil {
				return nil
			}
		}
		return ErrUsageUnavailable
	}
	if strings.TrimSpace(job.ReservationID) != "" {
		if rebinder, ok := s.billing.(billing.ReservationChannelRebinder); ok {
			if err := rebinder.RebindReservationChannel(ctx, job.ReservationID, job.Channel.ID); err != nil {
				return err
			}
		}
		return s.billing.Settle(ctx, job.ReservationID, mediaBillingUsage(response.Usage), response.ProviderRequestID)
	}
	if job.ModelRequestID != "" {
		if recorder, ok := s.billing.(billing.FreeRequestRecorder); ok {
			return recorder.CompleteFreeRequest(ctx, job.ModelRequestID, mediaBillingUsage(response.Usage), response.ProviderRequestID)
		}
	}
	return nil
}

func (s *Service) executeMediaJSON(ctx context.Context, principal *auth.Principal, model, requestType string, estimated billing.MeteredUsage, requestID, idempotencyKey string, invoke func(Channel, string) (MediaJSONResponse, error)) (MediaJSONResponse, error) {
	policy, candidates, billingEnabled, _, err := s.prepareMedia(ctx, principal, model, requestType, requestID, idempotencyKey, 1)
	if err != nil {
		return MediaJSONResponse{}, err
	}
	release, err := s.acquireMediaLimiter(ctx, principal)
	if err != nil {
		return MediaJSONResponse{}, err
	}
	defer release()
	attempted := map[string]struct{}{}
	var lastErr error
	var reservation billing.Reservation
	var freeID string
	for len(attempted) < len(candidates) {
		channel, found := pickWeightedChannel(candidates, attempted)
		if !found {
			break
		}
		attempted[channelKey(channel)] = struct{}{}
		key, resolveErr := s.credentials.Resolve(ctx, channel.CredentialRef)
		if resolveErr != nil {
			lastErr = resolveErr
			s.recordChannelFailure(ctx, channel, resolveErr)
			continue
		}
		// Do not reserve for a channel which cannot implement this operation.
		response, callErr := func() (MediaJSONResponse, error) {
			provider := s.providers[canonicalProvider(channel.Provider)]
			if provider == nil {
				return MediaJSONResponse{}, ErrProviderUnsupported
			}
			if _, ok := provider.(ImageGenerationProvider); requestType == "image_generation" || requestType == "image_edit" {
				if !ok {
					return MediaJSONResponse{}, ErrProviderUnsupported
				}
			}
			if requestType == "audio_transcription" {
				if _, ok := provider.(AudioTranscriptionProvider); !ok {
					return MediaJSONResponse{}, ErrProviderUnsupported
				}
			}
			if requestType == "audio_translation" {
				if _, ok := provider.(AudioTranslationProvider); !ok {
					return MediaJSONResponse{}, ErrProviderUnsupported
				}
			}
			if reservation.ID == "" && freeID == "" {
				reservation, freeID, err = s.startMediaBilling(ctx, principal, model, requestType, requestID, idempotencyKey, policy, channel, billingEnabled, estimated)
				if err != nil {
					return MediaJSONResponse{}, err
				}
			}
			return invoke(channel, key)
		}()
		if callErr != nil {
			lastErr = callErr
			if !errors.Is(callErr, ErrProviderUnsupported) {
				s.recordChannelFailure(ctx, channel, callErr)
			}
			if ctx.Err() != nil || (!errors.Is(callErr, ErrProviderUnsupported) && !retryableUpstreamError(callErr)) {
				break
			}
			continue
		}
		response.Usage = mergeMediaUsage(response.Usage, estimated)
		if len(response.Usage.Metrics) == 0 && response.Usage.InputTokens == 0 && response.Usage.OutputTokens == 0 {
			if billingEnabled {
				s.markRelayBillingPending(ctx, reservation, freeID, "upstream_media_usage_unavailable")
				lastErr = ErrUsageUnavailable
				break
			}
		}
		if err := s.completeMediaBilling(ctx, reservation, freeID, response.Usage, response.ProviderRequestID, channel.ID); err != nil {
			return MediaJSONResponse{}, err
		}
		s.recordChannelSuccess(ctx, channel)
		return response, nil
	}
	s.failRelayBilling(ctx, reservation, freeID, "upstream_media_request_failed")
	if errors.Is(lastErr, ErrUsageUnavailable) {
		return MediaJSONResponse{}, lastErr
	}
	if lastErr == nil {
		lastErr = ErrProviderUnsupported
	}
	return MediaJSONResponse{}, lastErr
}

func (s *Service) executeMediaBinary(ctx context.Context, principal *auth.Principal, model, requestType string, estimated billing.MeteredUsage, requestID, idempotencyKey string, invoke func(Channel, string) (MediaBinaryResponse, error)) (MediaBinaryResponse, error) {
	policy, candidates, billingEnabled, _, err := s.prepareMedia(ctx, principal, model, requestType, requestID, idempotencyKey, 1)
	if err != nil {
		return MediaBinaryResponse{}, err
	}
	release, err := s.acquireMediaLimiter(ctx, principal)
	if err != nil {
		return MediaBinaryResponse{}, err
	}
	defer release()
	attempted := map[string]struct{}{}
	var lastErr error
	var reservation billing.Reservation
	var freeID string
	for len(attempted) < len(candidates) {
		channel, found := pickWeightedChannel(candidates, attempted)
		if !found {
			break
		}
		attempted[channelKey(channel)] = struct{}{}
		provider := s.providers[canonicalProvider(channel.Provider)]
		if _, ok := provider.(SpeechProvider); !ok {
			lastErr = ErrProviderUnsupported
			continue
		}
		key, resolveErr := s.credentials.Resolve(ctx, channel.CredentialRef)
		if resolveErr != nil {
			lastErr = resolveErr
			s.recordChannelFailure(ctx, channel, resolveErr)
			continue
		}
		if reservation.ID == "" && freeID == "" {
			reservation, freeID, err = s.startMediaBilling(ctx, principal, model, requestType, requestID, idempotencyKey, policy, channel, billingEnabled, estimated)
			if err != nil {
				return MediaBinaryResponse{}, err
			}
		}
		response, callErr := invoke(channel, key)
		if callErr != nil {
			lastErr = callErr
			s.recordChannelFailure(ctx, channel, callErr)
			if ctx.Err() != nil || !retryableUpstreamError(callErr) {
				break
			}
			continue
		}
		response.Usage = mergeMediaUsage(response.Usage, estimated)
		if len(response.Usage.Metrics) == 0 && response.Usage.InputTokens == 0 && response.Usage.OutputTokens == 0 && billingEnabled {
			s.markRelayBillingPending(ctx, reservation, freeID, "upstream_media_usage_unavailable")
			lastErr = ErrUsageUnavailable
			break
		}
		if err := s.completeMediaBilling(ctx, reservation, freeID, response.Usage, response.ProviderRequestID, channel.ID); err != nil {
			return MediaBinaryResponse{}, err
		}
		s.recordChannelSuccess(ctx, channel)
		return response, nil
	}
	s.failRelayBilling(ctx, reservation, freeID, "upstream_media_request_failed")
	if errors.Is(lastErr, ErrUsageUnavailable) {
		return MediaBinaryResponse{}, lastErr
	}
	if lastErr == nil {
		lastErr = ErrProviderUnsupported
	}
	return MediaBinaryResponse{}, lastErr
}

func (s *Service) prepareMedia(ctx context.Context, principal *auth.Principal, model, requestType, requestID, idempotencyKey string, estimatedTokens int64) (GroupPolicy, []Channel, bool, RequestMetadata, error) {
	if s == nil || s.router == nil || s.credentials == nil || principal == nil {
		return GroupPolicy{}, nil, false, RequestMetadata{}, ErrUnavailable
	}
	if !modelAllowed(principal, model) {
		return GroupPolicy{}, nil, false, RequestMetadata{}, ErrModelNotAllowed
	}
	policy, err := s.resolveGroupPolicy(ctx, principal)
	if err != nil {
		return GroupPolicy{}, nil, false, RequestMetadata{}, err
	}
	if policy.Status != "" && policy.Status != "active" {
		return GroupPolicy{}, nil, false, RequestMetadata{}, ErrGroupUnavailable
	}
	if strings.TrimSpace(principal.TenantID) != "" && s.billing == nil {
		return GroupPolicy{}, nil, false, RequestMetadata{}, billing.ErrUnavailable
	}
	if err := s.consumeGroupRPM(ctx, policy); err != nil {
		return GroupPolicy{}, nil, false, RequestMetadata{}, err
	}
	_ = estimatedTokens
	candidates, err := s.channelCandidates(ctx, principal, model)
	if err != nil {
		return GroupPolicy{}, nil, false, RequestMetadata{}, err
	}
	if len(candidates) == 0 {
		return GroupPolicy{}, nil, false, RequestMetadata{}, ErrModelNotFound
	}
	metadata := RequestMetadataFromContext(ctx)
	metadata.RequestType = requestType
	if requestID == "" {
		requestID = metadata.RequestID
	}
	if idempotencyKey == "" {
		idempotencyKey = metadata.IdempotencyKey
	}
	return policy, candidates, s.billing != nil && policy.BillingType != "free", metadata, nil
}

func (s *Service) startMediaBilling(ctx context.Context, principal *auth.Principal, model, requestType, requestID, idempotencyKey string, policy GroupPolicy, channel Channel, enabled bool, metrics billing.MeteredUsage) (billing.Reservation, string, error) {
	metadata := RequestMetadataFromContext(ctx)
	if requestID == "" {
		requestID = metadata.RequestID
	}
	if idempotencyKey == "" {
		idempotencyKey = metadata.IdempotencyKey
	}
	request := billing.Request{RequestID: requestID, IdempotencyKey: idempotencyKey, TenantID: principal.TenantID, ProjectID: principalProjectID(principal), TokenID: principal.TokenID, Model: model, Provider: canonicalProvider(channel.Provider), ChannelID: channel.ID, GroupID: policy.ID, GroupMultiplier: policy.Multiplier, EstimatedMetrics: metrics, Endpoint: metadata.Endpoint, ClientIP: metadata.ClientIP, RequestType: requestType, BillingType: policy.BillingType}
	if enabled {
		value, err := s.billing.Reserve(ctx, request)
		return value, "", err
	}
	if recorder, ok := s.billing.(billing.FreeRequestRecorder); ok {
		freeID, err := recorder.StartFreeRequest(ctx, request)
		return billing.Reservation{}, freeID, err
	}
	return billing.Reservation{}, "", nil
}

func (s *Service) completeMediaBilling(ctx context.Context, reservation billing.Reservation, freeID string, usage MediaUsage, providerRequestID, channelID string) error {
	billingUsage := mediaBillingUsage(usage)
	if reservation.ID != "" {
		if rebinder, ok := s.billing.(billing.ReservationChannelRebinder); ok {
			if err := rebinder.RebindReservationChannel(ctx, reservation.ID, channelID); err != nil {
				return err
			}
		}
		return s.billing.Settle(ctx, reservation.ID, billingUsage, providerRequestID)
	}
	if freeID != "" {
		if recorder, ok := s.billing.(billing.FreeRequestRecorder); ok {
			if err := recorder.RebindRequestChannel(ctx, freeID, channelID); err != nil {
				return err
			}
			return recorder.CompleteFreeRequest(ctx, freeID, billingUsage, providerRequestID)
		}
	}
	return nil
}

func mediaBillingUsage(usage MediaUsage) billing.Usage {
	source := usage.Source
	if source == "" {
		source = "upstream"
	}
	return billing.Usage{InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens, CachedInputTokens: usage.CachedInputTokens, ReasoningTokens: usage.ReasoningTokens, Metrics: usage.Metrics, PricingTier: "", Source: source, Raw: usage.Raw}
}

func validateImageGenerationRequest(request *ImageGenerationRequest) error {
	if request == nil || strings.TrimSpace(request.Model) == "" || strings.TrimSpace(request.Prompt) == "" || len(request.Prompt) > 32000 {
		return ErrInvalidRequest
	}
	if request.Count == 0 {
		request.Count = 1
	}
	if request.Count < 1 || request.Count > 10 || len(request.Payload) > maxMediaJSONSize {
		return ErrInvalidRequest
	}
	return nil
}

func validateImageEditRequest(request *ImageEditRequest) error {
	if request == nil || strings.TrimSpace(request.Model) == "" || len(request.Image) == 0 || int64(len(request.Image)) > maxMediaFileSize {
		return ErrInvalidRequest
	}
	return nil
}

func validateAudioRequest(request *AudioRequest) error {
	if request == nil || strings.TrimSpace(request.Model) == "" || len(request.File) == 0 || int64(len(request.File)) > maxMediaFileSize || len(request.Fields) > 64 {
		return ErrInvalidRequest
	}
	return nil
}

func validateSpeechRequest(request *SpeechRequest) error {
	if request == nil || strings.TrimSpace(request.Model) == "" || strings.TrimSpace(request.Input) == "" || len([]rune(request.Input)) > 4096 || len(request.Payload) > maxMediaJSONSize {
		return ErrInvalidRequest
	}
	return nil
}

func validateVideoCreateRequest(request *VideoCreateRequest) error {
	if request == nil || strings.TrimSpace(request.Model) == "" || strings.TrimSpace(request.Prompt) == "" || len([]rune(request.Prompt)) > 10000 || len(request.Payload) > maxMediaJSONSize {
		return ErrInvalidRequest
	}
	if strings.TrimSpace(request.Duration) == "" {
		var payload map[string]any
		if json.Unmarshal(request.Payload, &payload) == nil {
			for _, key := range []string{"seconds", "duration"} {
				if value, ok := payload[key]; ok && strings.TrimSpace(fmt.Sprint(value)) != "" {
					request.Duration = strings.Trim(strings.TrimSpace(fmt.Sprint(value)), `"`)
					break
				}
			}
		}
	}
	if strings.TrimSpace(request.Duration) == "" || !validVideoDuration(request.Duration) {
		return ErrInvalidRequest
	}
	return nil
}

func validVideoDuration(value string) bool {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return err == nil && parsed > 0 && parsed <= 600
}

func upstreamModelFor(channel Channel, requested string) string {
	if value := strings.TrimSpace(channel.UpstreamModelName); value != "" {
		return value
	}
	return strings.TrimSpace(requested)
}

func mediaJSONRequest(ctx context.Context, baseURL, apiKey, path string, body []byte) ([]byte, http.Header, int, error) {
	return mediaJSONMethodRequest(ctx, http.MethodPost, baseURL, apiKey, path, body)
}

func mediaJSONMethodRequest(ctx context.Context, method, baseURL, apiKey, requestPath string, body []byte) ([]byte, http.Header, int, error) {
	request, err := newUpstreamMethodRequest(ctx, method, baseURL, apiKey, requestPath, body, "application/json")
	if err != nil {
		return nil, nil, 0, ErrInvalidRequest
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

func mediaMultipartRequest(ctx context.Context, baseURL, apiKey, path string, fields map[string]string, fileField, fileName, fileType string, file []byte, maskName, maskType string, mask []byte) ([]byte, http.Header, int, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			return nil, nil, 0, ErrInvalidRequest
		}
	}
	if len(file) > 0 {
		part, err := createMediaFilePart(writer, fileField, fileName, fileType)
		if err != nil {
			return nil, nil, 0, ErrInvalidRequest
		}
		if _, err := part.Write(file); err != nil {
			return nil, nil, 0, ErrInvalidRequest
		}
	}
	if len(mask) > 0 {
		part, err := createMediaFilePart(writer, "mask", maskName, maskType)
		if err != nil {
			return nil, nil, 0, ErrInvalidRequest
		}
		if _, err := part.Write(mask); err != nil {
			return nil, nil, 0, ErrInvalidRequest
		}
	}
	if err := writer.Close(); err != nil {
		return nil, nil, 0, ErrInvalidRequest
	}
	request, err := newUpstreamMethodRequest(ctx, http.MethodPost, baseURL, apiKey, path, body.Bytes(), writer.FormDataContentType())
	if err != nil {
		return nil, nil, 0, ErrInvalidRequest
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

func createMediaFilePart(writer *multipart.Writer, field, name, contentType string) (io.Writer, error) {
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if _, _, err := mime.ParseMediaType(contentType); err != nil {
		contentType = "application/octet-stream"
	}
	return writer.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": []string{`form-data; name="` + escapeFormValue(field) + `"; filename="` + escapeFormValue(name) + `"`},
		"Content-Type":        []string{contentType},
	})
}

func escapeFormValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `_`)
	value = strings.ReplaceAll(value, `"`, `_`)
	value = strings.ReplaceAll(value, "\r", "_")
	return strings.ReplaceAll(value, "\n", "_")
}

func mediaBinaryRequest(ctx context.Context, method, baseURL, apiKey, path string, body []byte, contentType string) (MediaBinaryResponse, error) {
	request, err := newUpstreamMethodRequest(ctx, method, baseURL, apiKey, path, body, contentType)
	if err != nil {
		return MediaBinaryResponse{}, ErrInvalidRequest
	}
	client, err := providerHTTPClient(baseURL)
	if err != nil {
		return MediaBinaryResponse{}, ErrInvalidRequest
	}
	response, err := client.Do(request)
	if err != nil {
		return MediaBinaryResponse{}, &UpstreamError{Err: ErrUpstream}
	}
	defer response.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(response.Body, maxMediaFileSize))
	if readErr != nil {
		return MediaBinaryResponse{}, &UpstreamError{StatusCode: response.StatusCode, Err: ErrUpstream}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return MediaBinaryResponse{}, &UpstreamError{StatusCode: response.StatusCode, Err: ErrUpstream}
	}
	return MediaBinaryResponse{Body: data, ContentType: response.Header.Get("Content-Type")}, nil
}

func newUpstreamMethodRequest(ctx context.Context, method, baseURL, apiKey, path string, body []byte, contentType string) (*http.Request, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, ErrInvalidRequest
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + path
	request, err := http.NewRequestWithContext(ctx, method, parsed.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	request.Header.Set("Content-Type", contentType)
	return request, nil
}

func decodeMediaJSON(data []byte, status int, header http.Header) (MediaJSONResponse, error) {
	if status < 200 || status >= 300 {
		return MediaJSONResponse{}, &UpstreamError{StatusCode: status, Err: ErrUpstream}
	}
	var value any
	if json.Unmarshal(data, &value) != nil {
		return MediaJSONResponse{}, ErrUpstream
	}
	return MediaJSONResponse{Body: append(json.RawMessage(nil), data...), ProviderRequestID: header.Get("x-request-id"), Usage: parseMediaUsage(data)}, nil
}

func parseMediaUsage(data []byte) MediaUsage {
	usage := MediaUsage{Metrics: billing.MeteredUsage{}, Source: "upstream", Raw: append(json.RawMessage(nil), data...)}
	var value struct {
		Usage json.RawMessage   `json:"usage"`
		Data  []json.RawMessage `json:"data"`
	}
	if json.Unmarshal(data, &value) != nil {
		return usage
	}
	var providerUsage struct {
		Type              string  `json:"type"`
		InputTokens       int64   `json:"input_tokens"`
		OutputTokens      int64   `json:"output_tokens"`
		TotalTokens       int64   `json:"total_tokens"`
		Seconds           float64 `json:"seconds"`
		InputTokenDetails struct {
			AudioTokens int64 `json:"audio_tokens"`
			ImageTokens int64 `json:"image_tokens"`
			VideoTokens int64 `json:"video_tokens"`
			TextTokens  int64 `json:"text_tokens"`
		} `json:"input_token_details"`
		InputTokensDetails struct {
			AudioTokens int64 `json:"audio_tokens"`
			ImageTokens int64 `json:"image_tokens"`
			VideoTokens int64 `json:"video_tokens"`
			TextTokens  int64 `json:"text_tokens"`
		} `json:"input_tokens_details"`
		OutputTokenDetails struct {
			AudioTokens int64 `json:"audio_tokens"`
			ImageTokens int64 `json:"image_tokens"`
			VideoTokens int64 `json:"video_tokens"`
		} `json:"output_token_details"`
		OutputTokensDetails struct {
			AudioTokens int64 `json:"audio_tokens"`
			ImageTokens int64 `json:"image_tokens"`
			VideoTokens int64 `json:"video_tokens"`
		} `json:"output_tokens_details"`
	}
	if len(value.Usage) > 0 {
		_ = json.Unmarshal(value.Usage, &providerUsage)
	}
	usage.InputTokens, usage.OutputTokens = providerUsage.InputTokens, providerUsage.OutputTokens
	if providerUsage.InputTokens > 0 {
		usage.Metrics["input_tokens"] = strconv.FormatInt(providerUsage.InputTokens, 10)
	}
	if providerUsage.OutputTokens > 0 {
		usage.Metrics["output_tokens"] = strconv.FormatInt(providerUsage.OutputTokens, 10)
	}
	inputAudio := providerUsage.InputTokenDetails.AudioTokens + providerUsage.InputTokensDetails.AudioTokens
	inputImage := providerUsage.InputTokenDetails.ImageTokens + providerUsage.InputTokensDetails.ImageTokens
	inputVideo := providerUsage.InputTokenDetails.VideoTokens + providerUsage.InputTokensDetails.VideoTokens
	outputAudio := providerUsage.OutputTokenDetails.AudioTokens + providerUsage.OutputTokensDetails.AudioTokens
	outputImage := providerUsage.OutputTokenDetails.ImageTokens + providerUsage.OutputTokensDetails.ImageTokens
	outputVideo := providerUsage.OutputTokenDetails.VideoTokens + providerUsage.OutputTokensDetails.VideoTokens
	if inputAudio > 0 {
		usage.Metrics["input_audio_tokens"] = strconv.FormatInt(inputAudio, 10)
	}
	if inputImage > 0 {
		usage.Metrics["input_image_tokens"] = strconv.FormatInt(inputImage, 10)
	}
	if inputVideo > 0 {
		usage.Metrics["input_video_tokens"] = strconv.FormatInt(inputVideo, 10)
	}
	if outputAudio > 0 {
		usage.Metrics["output_audio_tokens"] = strconv.FormatInt(outputAudio, 10)
	}
	if outputImage > 0 {
		usage.Metrics["output_image_tokens"] = strconv.FormatInt(outputImage, 10)
	}
	if outputVideo > 0 {
		usage.Metrics["output_video_tokens"] = strconv.FormatInt(outputVideo, 10)
	}
	if len(value.Data) > 0 {
		usage.Metrics["output_images"] = strconv.Itoa(len(value.Data))
	}
	if providerUsage.Type == "tokens" && providerUsage.TotalTokens > 0 && usage.InputTokens == 0 {
		usage.InputTokens = providerUsage.TotalTokens
		usage.Metrics["input_audio_tokens"] = strconv.FormatInt(providerUsage.TotalTokens, 10)
	}
	return usage
}

func parseImageResponseUsage(data []byte, requested int64) MediaUsage {
	usage := parseMediaUsage(data)
	if _, ok := usage.Metrics["output_images"]; !ok && requested > 0 {
		usage.Metrics["output_images"] = strconv.FormatInt(requested, 10)
	}
	return usage
}

func parseAudioResponseUsage(data []byte, file []byte) MediaUsage {
	usage := parseMediaUsage(data)
	if _, ok := usage.Metrics["input_audio_seconds"]; !ok {
		if seconds := wavDurationSeconds(file); seconds != "" {
			usage.Metrics["input_audio_seconds"] = seconds
		} else {
			var value struct {
				Seconds  float64 `json:"seconds"`
				Duration float64 `json:"duration"`
				Usage    struct {
					Seconds float64 `json:"seconds"`
				} `json:"usage"`
			}
			if json.Unmarshal(data, &value) == nil {
				seconds := value.Usage.Seconds
				if seconds == 0 {
					seconds = value.Seconds
				}
				if seconds == 0 {
					seconds = value.Duration
				}
				if seconds > 0 {
					usage.Metrics["input_audio_seconds"] = decimalFloat(seconds)
				}
			}
		}
	}
	return usage
}

func wavDurationSeconds(data []byte) string {
	if len(data) < 44 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return ""
	}
	channels := int(binaryLittleEndianUint16(data[22:24]))
	rate := int(binaryLittleEndianUint32(data[24:28]))
	bits := int(binaryLittleEndianUint16(data[34:36]))
	dataSize := 0
	for offset := 12; offset+8 <= len(data); {
		size := int(binaryLittleEndianUint32(data[offset+4 : offset+8]))
		if string(data[offset:offset+4]) == "data" {
			dataSize = size
			break
		}
		offset += 8 + size
	}
	if channels <= 0 || rate <= 0 || bits <= 0 || dataSize <= 0 {
		return ""
	}
	return decimalFloat(float64(dataSize) / float64(channels*rate*(bits/8)))
}

func binaryLittleEndianUint16(value []byte) uint16 { return uint16(value[0]) | uint16(value[1])<<8 }
func binaryLittleEndianUint32(value []byte) uint32 {
	return uint32(value[0]) | uint32(value[1])<<8 | uint32(value[2])<<16 | uint32(value[3])<<24
}

func decimalFloat(value float64) string { return strconv.FormatFloat(value, 'f', 6, 64) }

func normalizeVideoStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "succeeded", "success", "completed", "done":
		return "completed"
	case "failed", "error":
		return "failed"
	case "cancelled", "canceled":
		return "cancelled"
	case "running", "in_progress", "processing":
		return "processing"
	case "queued", "pending":
		return "queued"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func extractJSONString(data []byte, key string) string {
	var value map[string]any
	if json.Unmarshal(data, &value) != nil {
		return ""
	}
	if result, ok := value[key].(string); ok {
		return strings.TrimSpace(result)
	}
	return ""
}

func extractVideoURI(data []byte) string {
	var value map[string]any
	if json.Unmarshal(data, &value) != nil {
		return ""
	}
	if uri := findStringKey(value, "uri"); uri != "" {
		return uri
	}
	return findStringKey(value, "url")
}

func findStringKey(value map[string]any, key string) string {
	if result, ok := value[key].(string); ok && strings.TrimSpace(result) != "" {
		return strings.TrimSpace(result)
	}
	for _, child := range value {
		if nested, ok := child.(map[string]any); ok {
			if result := findStringKey(nested, key); result != "" {
				return result
			}
		}
		if list, ok := child.([]any); ok {
			for _, item := range list {
				if nested, ok := item.(map[string]any); ok {
					if result := findStringKey(nested, key); result != "" {
						return result
					}
				}
			}
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func newMediaJobID() string {
	value, err := ids.New()
	if err != nil {
		return "media_job_unavailable"
	}
	return value
}

func (s *Service) acquireMediaLimiter(ctx context.Context, principal *auth.Principal) (func(), error) {
	if s == nil || s.rateLimiter == nil || principal == nil || strings.TrimSpace(principal.TokenID) == "" {
		return func() {}, nil
	}
	return s.rateLimiter.AcquireToken(ctx, principal.TokenID, 1)
}

func mergeMediaUsage(usage MediaUsage, estimated billing.MeteredUsage) MediaUsage {
	if len(estimated) == 0 {
		return usage
	}
	if usage.Metrics == nil {
		usage.Metrics = make(billing.MeteredUsage, len(estimated))
	}
	added := false
	for key, value := range estimated {
		if strings.TrimSpace(usage.Metrics[key]) == "" {
			usage.Metrics[key] = value
			added = true
		}
	}
	if added && usage.Source == "" {
		usage.Source = "local_estimate"
	}
	return usage
}

func videoEstimatedMetrics(request VideoCreateRequest) billing.MeteredUsage {
	seconds := strings.TrimSpace(request.Duration)
	if seconds == "" {
		var payload map[string]any
		if json.Unmarshal(request.Payload, &payload) == nil {
			for _, key := range []string{"seconds", "duration"} {
				if value, ok := payload[key]; ok {
					seconds = fmt.Sprint(value)
					break
				}
			}
		}
	}
	if seconds == "" {
		return billing.MeteredUsage{}
	}
	return billing.MeteredUsage{"output_seconds": seconds}
}

func openAIImageRequestBody(request ImageGenerationRequest, model string) ([]byte, error) {
	if len(request.Payload) > 0 {
		var value map[string]any
		if json.Unmarshal(request.Payload, &value) == nil {
			value["model"] = model
			value["n"] = request.Count
			return json.Marshal(value)
		}
	}
	return json.Marshal(map[string]any{"model": model, "prompt": request.Prompt, "n": request.Count})
}

func openAISpeechRequestBody(request SpeechRequest, model string) ([]byte, error) {
	if len(request.Payload) > 0 {
		var value map[string]any
		if json.Unmarshal(request.Payload, &value) == nil {
			value["model"] = model
			value["input"] = request.Input
			return json.Marshal(value)
		}
	}
	return json.Marshal(map[string]any{"model": model, "input": request.Input})
}

func openAIVideoRequestBody(request VideoCreateRequest, model string) ([]byte, error) {
	if len(request.Payload) > 0 {
		var value map[string]any
		if json.Unmarshal(request.Payload, &value) == nil {
			value["model"] = model
			value["prompt"] = request.Prompt
			if _, ok := value["seconds"]; !ok && request.Duration != "" {
				value["seconds"] = request.Duration
				delete(value, "duration")
			}
			return json.Marshal(value)
		}
	}
	value := map[string]any{"model": model, "prompt": request.Prompt}
	if request.Duration != "" {
		value["seconds"] = request.Duration
	}
	return json.Marshal(value)
}

func imageResponseWithUsage(data []byte, status int, header http.Header, count int64) (MediaJSONResponse, error) {
	result, err := decodeMediaJSON(data, status, header)
	if err != nil {
		return result, err
	}
	var envelope struct {
		Data []json.RawMessage `json:"data"`
	}
	if json.Unmarshal(data, &envelope) != nil || len(envelope.Data) == 0 {
		return MediaJSONResponse{}, ErrUpstream
	}
	result.Usage = parseImageResponseUsage(data, count)
	result.ID = extractJSONString(data, "id")
	return result, nil
}
func audioResponseWithUsage(data []byte, status int, header http.Header, file []byte) (MediaJSONResponse, error) {
	result, err := decodeMediaJSON(data, status, header)
	if err != nil {
		return result, err
	}
	result.Usage = parseAudioResponseUsage(data, file)
	result.ID = extractJSONString(data, "id")
	return result, nil
}
