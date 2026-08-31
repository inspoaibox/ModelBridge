package relay

import (
	"context"
	"errors"
	"io"
	"strings"

	"ai-token/internal/auth"
	"ai-token/internal/billing"
)

type StreamingChatCompletionService interface {
	StreamChatCompletions(context.Context, *auth.Principal, ChatCompletionRequest, func(ChatCompletionStreamEvent) error) error
}

type EmbeddingCompletionService interface {
	CreateEmbeddings(context.Context, *auth.Principal, EmbeddingRequest) (EmbeddingResponse, error)
}

// StreamChatCompletions keeps the same authorization, routing, rate limits,
// failover and billing lifecycle as buffered completions. Failover is only
// attempted before any bytes have been delivered to the caller.
func (s *Service) StreamChatCompletions(
	ctx context.Context,
	principal *auth.Principal,
	request ChatCompletionRequest,
	emit func(ChatCompletionStreamEvent) error,
) error {
	if s == nil || s.router == nil || s.credentials == nil || emit == nil {
		return ErrUnavailable
	}
	request.Stream = true
	if err := validateChatRequest(request); err != nil {
		return err
	}
	if !modelAllowed(principal, request.Model) {
		return ErrModelNotAllowed
	}
	metadata := RequestMetadataFromContext(ctx)
	if metadata.RequestType == "" {
		metadata.RequestType = "stream"
	}
	if s.rateLimiter != nil && principal != nil && principal.TokenID != "" {
		release, err := s.rateLimiter.AcquireToken(ctx, principal.TokenID, estimateInputTokens(request)+estimateOutputTokens(request))
		if err != nil {
			return err
		}
		defer release()
	}
	groupPolicy, err := s.resolveGroupPolicy(ctx, principal)
	if err != nil {
		return err
	}
	if groupPolicy.BillingType != "free" && strings.TrimSpace(principal.TenantID) != "" && s.billing == nil {
		return billing.ErrUnavailable
	}
	if err := s.consumeGroupRPM(ctx, groupPolicy); err != nil {
		return err
	}
	candidates, err := s.channelCandidates(ctx, principal, request.Model)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return ErrModelNotFound
	}

	billingEnabled := s.billing != nil && groupPolicy.BillingType != "free"
	attempted := map[string]struct{}{}
	var lastErr error
	var reservation billing.Reservation
	var freeRequestID string
	for len(attempted) < len(candidates) {
		channel, ok := pickWeightedChannel(candidates, attempted)
		if !ok {
			break
		}
		attempted[channelKey(channel)] = struct{}{}
		provider := s.providers[canonicalProvider(channel.Provider)]
		streamProvider, supported := provider.(StreamingProvider)
		if !supported {
			lastErr = ErrStreamingUnsupported
			continue
		}
		apiKey, resolveErr := s.credentials.Resolve(ctx, channel.CredentialRef)
		if resolveErr != nil {
			lastErr = resolveErr
			s.recordChannelFailure(ctx, channel, resolveErr)
			continue
		}

		if reservation.ID == "" && freeRequestID == "" {
			reservation, freeRequestID, err = s.startRelayBilling(ctx, principal, request, metadata, groupPolicy, channel, billingEnabled)
			if err != nil {
				return err
			}
		}
		upstreamModel := channel.UpstreamModelName
		if strings.TrimSpace(upstreamModel) == "" {
			upstreamModel = request.Model
		}
		stream, streamErr := streamProvider.NewChatCompletionStream(ctx, UpstreamChatCompletionRequest{
			Channel: channel, APIKey: apiKey, Request: request, UpstreamModel: upstreamModel,
		})
		if streamErr != nil {
			lastErr = streamErr
			s.recordChannelFailure(ctx, channel, streamErr)
			if ctx.Err() != nil || !retryableUpstreamError(streamErr) {
				s.failRelayBilling(ctx, reservation, freeRequestID, "upstream_stream_open_failed")
				return streamErr
			}
			continue
		}

		var usage ChatUsage
		gotUsage := false
		delivered := false
		var output strings.Builder
		var providerRequestID string
		streamErr = func() error {
			defer stream.Close()
			for {
				event, recvErr := stream.Recv()
				if errors.Is(recvErr, io.EOF) {
					break
				}
				if recvErr != nil {
					return recvErr
				}
				if event.ID != "" {
					providerRequestID = event.ID
				}
				if event.Delta != "" {
					output.WriteString(event.Delta)
				}
				if event.HasUsage {
					usage = mergeChatUsage(usage, event.Usage)
					gotUsage = gotUsage || chatUsageHasValues(event.Usage)
				}
				// Mark before invoking the sink: an HTTP writer may have committed
				// headers before returning an error, so failover would corrupt the
				// stream even when this callback reports failure.
				delivered = true
				if emitErr := emit(event); emitErr != nil {
					return emitErr
				}
			}
			return nil
		}()
		if streamErr != nil {
			lastErr = streamErr
			s.recordChannelFailure(ctx, channel, streamErr)
			if delivered {
				if gotUsage {
					_ = s.completeRelayBilling(ctx, reservation, freeRequestID, usage, providerRequestID, channel.ID, "upstream")
				} else if canEstimateChatUsage(request) {
					estimatedInput := estimateInputTokens(request)
					usage = ChatUsage{PromptTokens: estimatedInput, CompletionTokens: int64(len([]rune(output.String()))), TotalTokens: estimatedInput + int64(len([]rune(output.String())))}
					_ = s.completeRelayBilling(ctx, reservation, freeRequestID, usage, providerRequestID, channel.ID, "local_estimate")
				} else if billingEnabled {
					s.markRelayBillingPending(ctx, reservation, freeRequestID, "upstream_stream_usage_unavailable")
				} else {
					s.failRelayBilling(ctx, reservation, freeRequestID, "upstream_usage_unavailable")
				}
			}
			if delivered || ctx.Err() != nil || !retryableUpstreamError(streamErr) {
				if !delivered {
					s.failRelayBilling(ctx, reservation, freeRequestID, "upstream_stream_failed")
				}
				return streamErr
			}
			continue
		}
		source := "upstream"
		if !gotUsage {
			if !canEstimateChatUsage(request) {
				if billingEnabled {
					s.markRelayBillingPending(ctx, reservation, freeRequestID, "upstream_stream_usage_unavailable")
				} else {
					s.failRelayBilling(ctx, reservation, freeRequestID, "upstream_usage_unavailable")
				}
				return ErrUsageUnavailable
			}
			source = "local_estimate"
			usage = ChatUsage{PromptTokens: estimateInputTokens(request), CompletionTokens: int64(len([]rune(output.String()))), TotalTokens: estimateInputTokens(request) + int64(len([]rune(output.String())))}
		} else {
			inputTokens, _, outputTokens, _ := usage.billingBreakdown()
			if inputTokens <= 0 || (outputTokens <= 0 && output.Len() > 0) {
				if !canEstimateChatUsage(request) {
					if billingEnabled {
						s.markRelayBillingPending(ctx, reservation, freeRequestID, "upstream_stream_usage_unavailable")
					} else {
						s.failRelayBilling(ctx, reservation, freeRequestID, "upstream_usage_unavailable")
					}
					return ErrUsageUnavailable
				}
				if inputTokens <= 0 {
					usage.PromptTokens = estimateInputTokens(request)
				}
				if outputTokens <= 0 && output.Len() > 0 {
					usage.CompletionTokens = int64(len([]rune(output.String())))
				}
				source = "local_estimate"
			}
		}
		if providerRequestID == "" {
			providerRequestID = newCompletionID("chatcmpl_")
		}
		if strings.TrimSpace(usage.PricingTier) == "" {
			usage.PricingTier = request.ServiceTier
		}
		if err := s.completeRelayBilling(ctx, reservation, freeRequestID, usage, providerRequestID, channel.ID, source); err != nil {
			return err
		}
		s.recordChannelSuccess(ctx, channel)
		return nil
	}
	if lastErr == nil {
		lastErr = ErrStreamingUnsupported
	}
	s.failRelayBilling(ctx, reservation, freeRequestID, "upstream_stream_failed")
	return lastErr
}

func mergeChatUsage(current, next ChatUsage) ChatUsage {
	current.UsageProvided = current.UsageProvided || next.UsageProvided
	if next.PromptTokens > current.PromptTokens {
		current.PromptTokens = next.PromptTokens
	}
	if next.CacheCreationInputTokens > current.CacheCreationInputTokens {
		current.CacheCreationInputTokens = next.CacheCreationInputTokens
	}
	if next.CacheCreation1HInputTokens > current.CacheCreation1HInputTokens {
		current.CacheCreation1HInputTokens = next.CacheCreation1HInputTokens
	}
	if next.CacheReadInputTokens > current.CacheReadInputTokens {
		current.CacheReadInputTokens = next.CacheReadInputTokens
	}
	if next.CompletionTokens > current.CompletionTokens {
		current.CompletionTokens = next.CompletionTokens
	}
	if next.TotalTokens > current.TotalTokens {
		current.TotalTokens = next.TotalTokens
	}
	if strings.TrimSpace(current.PricingTier) == "" {
		current.PricingTier = next.PricingTier
	}
	if current.PromptTokensDetails == nil {
		current.PromptTokensDetails = &ChatPromptTokensDetails{}
	}
	if next.PromptTokensDetails != nil && next.PromptTokensDetails.CachedTokens > current.PromptTokensDetails.CachedTokens {
		current.PromptTokensDetails.CachedTokens = next.PromptTokensDetails.CachedTokens
	}
	if next.PromptTokensDetails != nil {
		if next.PromptTokensDetails.AudioTokens > current.PromptTokensDetails.AudioTokens {
			current.PromptTokensDetails.AudioTokens = next.PromptTokensDetails.AudioTokens
		}
		if next.PromptTokensDetails.ImageTokens > current.PromptTokensDetails.ImageTokens {
			current.PromptTokensDetails.ImageTokens = next.PromptTokensDetails.ImageTokens
		}
		if next.PromptTokensDetails.VideoTokens > current.PromptTokensDetails.VideoTokens {
			current.PromptTokensDetails.VideoTokens = next.PromptTokensDetails.VideoTokens
		}
		if next.PromptTokensDetails.CachedAudioTokens > current.PromptTokensDetails.CachedAudioTokens {
			current.PromptTokensDetails.CachedAudioTokens = next.PromptTokensDetails.CachedAudioTokens
		}
		if next.PromptTokensDetails.CachedImageTokens > current.PromptTokensDetails.CachedImageTokens {
			current.PromptTokensDetails.CachedImageTokens = next.PromptTokensDetails.CachedImageTokens
		}
		if next.PromptTokensDetails.CachedVideoTokens > current.PromptTokensDetails.CachedVideoTokens {
			current.PromptTokensDetails.CachedVideoTokens = next.PromptTokensDetails.CachedVideoTokens
		}
	}
	if current.CompletionTokensDetails == nil {
		current.CompletionTokensDetails = &ChatCompletionTokensDetails{}
	}
	if next.CompletionTokensDetails != nil && next.CompletionTokensDetails.ReasoningTokens > current.CompletionTokensDetails.ReasoningTokens {
		current.CompletionTokensDetails.ReasoningTokens = next.CompletionTokensDetails.ReasoningTokens
	}
	if next.CompletionTokensDetails != nil && next.CompletionTokensDetails.AudioTokens > current.CompletionTokensDetails.AudioTokens {
		current.CompletionTokensDetails.AudioTokens = next.CompletionTokensDetails.AudioTokens
	}
	if next.CompletionTokensDetails != nil {
		if next.CompletionTokensDetails.ImageTokens > current.CompletionTokensDetails.ImageTokens {
			current.CompletionTokensDetails.ImageTokens = next.CompletionTokensDetails.ImageTokens
		}
		if next.CompletionTokensDetails.VideoTokens > current.CompletionTokensDetails.VideoTokens {
			current.CompletionTokensDetails.VideoTokens = next.CompletionTokensDetails.VideoTokens
		}
	}
	return current
}

func validateChatRequest(request ChatCompletionRequest) error {
	request.Stream = false
	return request.validate()
}

func (s *Service) consumeGroupRPM(ctx context.Context, policy GroupPolicy) error {
	if policy.RPMLimit <= 0 {
		return nil
	}
	limiter, ok := s.router.(GroupRPMConsumer)
	if !ok {
		return ErrUnavailable
	}
	allowed, err := limiter.ConsumeGroupRPM(ctx, policy.ID, policy.RPMLimit)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrGroupRateLimited
	}
	return nil
}

func (s *Service) startRelayBilling(
	ctx context.Context,
	principal *auth.Principal,
	request ChatCompletionRequest,
	metadata RequestMetadata,
	policy GroupPolicy,
	channel Channel,
	billingEnabled bool,
) (billing.Reservation, string, error) {
	if principal == nil {
		return billing.Reservation{}, "", ErrUnavailable
	}
	requestType := metadata.RequestType
	if requestType == "" {
		requestType = "sync"
	}
	billingRequest := billing.Request{
		RequestID: request.RequestID, IdempotencyKey: request.IdempotencyKey,
		TenantID: principal.TenantID, ProjectID: principalProjectID(principal), TokenID: principal.TokenID,
		Model: request.Model, Provider: canonicalProvider(channel.Provider), ChannelID: channel.ID,
		GroupID: policy.ID, GroupMultiplier: policy.Multiplier,
		EstimatedInputTokens: estimateInputTokens(request), EstimatedOutputTokens: estimateOutputTokens(request),
		Endpoint: metadata.Endpoint, ClientIP: metadata.ClientIP, RequestType: requestType,
		ReasoningEffort: request.ReasoningEffort, PricingTier: request.ServiceTier, BillingType: policy.BillingType,
	}
	if billingEnabled {
		reservation, err := s.billing.Reserve(ctx, billingRequest)
		return reservation, "", err
	}
	if recorder, ok := s.billing.(billing.FreeRequestRecorder); ok {
		freeID, err := recorder.StartFreeRequest(ctx, billingRequest)
		return billing.Reservation{}, freeID, err
	}
	return billing.Reservation{}, "", nil
}

func (s *Service) completeRelayBilling(ctx context.Context, reservation billing.Reservation, freeID string, usage ChatUsage, providerRequestID, channelID, source string) error {
	input, cached, output, reasoning := usage.billingBreakdown()
	raw := marshalUsageForBilling(usage)
	if reservation.ID != "" {
		if rebinder, ok := s.billing.(billing.ReservationChannelRebinder); ok {
			if err := rebinder.RebindReservationChannel(ctx, reservation.ID, channelID); err != nil {
				return err
			}
		}
		return s.billing.Settle(ctx, reservation.ID, billing.Usage{InputTokens: input, OutputTokens: output, CachedInputTokens: cached, ReasoningTokens: reasoning, Metrics: usage.meteredUsage(), PricingTier: usage.PricingTier, Source: source, Raw: raw}, providerRequestID)
	}
	if freeID != "" {
		if recorder, ok := s.billing.(billing.FreeRequestRecorder); ok {
			if err := recorder.RebindRequestChannel(ctx, freeID, channelID); err != nil {
				return err
			}
			return recorder.CompleteFreeRequest(ctx, freeID, billing.Usage{InputTokens: input, OutputTokens: output, CachedInputTokens: cached, ReasoningTokens: reasoning, Metrics: usage.meteredUsage(), PricingTier: usage.PricingTier, Source: source, Raw: raw}, providerRequestID)
		}
	}
	return nil
}

func (s *Service) failRelayBilling(ctx context.Context, reservation billing.Reservation, freeID, reason string) {
	if reservation.ID != "" && s.billing != nil {
		_ = s.billing.Fail(ctx, reservation.ID, reason)
	}
	if freeID != "" {
		if recorder, ok := s.billing.(billing.FreeRequestRecorder); ok {
			_ = recorder.FailFreeRequest(ctx, freeID, reason)
		}
	}
}

func (s *Service) markRelayBillingPending(ctx context.Context, reservation billing.Reservation, freeID, reason string) {
	if reservation.ID != "" && s.billing != nil {
		if marker, ok := s.billing.(billing.ReservationPendingMarker); ok {
			_ = marker.MarkSettlementPending(ctx, reservation.ID, reason)
		}
		return
	}
	if freeID != "" && s.billing != nil {
		if recorder, ok := s.billing.(billing.FreeRequestRecorder); ok {
			_ = recorder.FailFreeRequest(ctx, freeID, reason)
		}
	}
}

func (s *Service) CreateEmbeddings(ctx context.Context, principal *auth.Principal, request EmbeddingRequest) (EmbeddingResponse, error) {
	if s == nil || s.router == nil || s.credentials == nil {
		return EmbeddingResponse{}, ErrUnavailable
	}
	if err := request.validate(); err != nil {
		return EmbeddingResponse{}, err
	}
	if !modelAllowed(principal, request.Model) {
		return EmbeddingResponse{}, ErrModelNotAllowed
	}
	metadata := RequestMetadataFromContext(ctx)
	if metadata.RequestType == "" {
		metadata.RequestType = "embedding"
	}
	if s.rateLimiter != nil && principal != nil && principal.TokenID != "" {
		release, err := s.rateLimiter.AcquireToken(ctx, principal.TokenID, estimateEmbeddingTokens(request))
		if err != nil {
			return EmbeddingResponse{}, err
		}
		defer release()
	}
	policy, err := s.resolveGroupPolicy(ctx, principal)
	if err != nil {
		return EmbeddingResponse{}, err
	}
	if strings.TrimSpace(principal.TenantID) != "" && s.billing == nil {
		return EmbeddingResponse{}, billing.ErrUnavailable
	}
	if err := s.consumeGroupRPM(ctx, policy); err != nil {
		return EmbeddingResponse{}, err
	}
	candidates, err := s.channelCandidates(ctx, principal, request.Model)
	if err != nil {
		return EmbeddingResponse{}, err
	}
	if len(candidates) == 0 {
		return EmbeddingResponse{}, ErrModelNotFound
	}
	billingEnabled := s.billing != nil && policy.BillingType != "free"
	attempted := map[string]struct{}{}
	var lastErr error
	var reservation billing.Reservation
	var freeID string
	for len(attempted) < len(candidates) {
		channel, ok := pickWeightedChannel(candidates, attempted)
		if !ok {
			break
		}
		attempted[channelKey(channel)] = struct{}{}
		provider := s.providers[canonicalProvider(channel.Provider)]
		embedder, ok := provider.(EmbeddingProvider)
		if !ok {
			lastErr = ErrProviderUnsupported
			continue
		}
		apiKey, resolveErr := s.credentials.Resolve(ctx, channel.CredentialRef)
		if resolveErr != nil {
			lastErr = resolveErr
			s.recordChannelFailure(ctx, channel, resolveErr)
			continue
		}
		billingRequest := billing.Request{
			RequestID: request.RequestID, IdempotencyKey: request.IdempotencyKey, TenantID: principal.TenantID,
			ProjectID: principalProjectID(principal), TokenID: principal.TokenID, Model: request.Model,
			Provider: canonicalProvider(channel.Provider), ChannelID: channel.ID, GroupID: policy.ID,
			GroupMultiplier: policy.Multiplier, EstimatedInputTokens: estimateEmbeddingTokens(request),
			Endpoint: metadata.Endpoint, ClientIP: metadata.ClientIP, RequestType: metadata.RequestType,
			BillingType: policy.BillingType,
		}
		if reservation.ID == "" && freeID == "" {
			if billingEnabled {
				reservation, err = s.billing.Reserve(ctx, billingRequest)
			} else if recorder, recorderOK := s.billing.(billing.FreeRequestRecorder); recorderOK {
				freeID, err = recorder.StartFreeRequest(ctx, billingRequest)
			}
		}
		if err != nil {
			return EmbeddingResponse{}, err
		}
		upstreamModel := channel.UpstreamModelName
		if strings.TrimSpace(upstreamModel) == "" {
			upstreamModel = request.Model
		}
		response, callErr := embedder.CreateEmbeddings(ctx, UpstreamEmbeddingRequest{Channel: channel, APIKey: apiKey, Request: request, UpstreamModel: upstreamModel})
		if callErr != nil {
			lastErr = callErr
			s.recordChannelFailure(ctx, channel, callErr)
			if ctx.Err() != nil || !retryableUpstreamError(callErr) {
				break
			}
			continue
		}
		if response.Model == "" {
			response.Model = request.Model
		}
		if response.Object == "" {
			response.Object = "list"
		}
		source := "upstream"
		if response.Usage.PromptTokens <= 0 {
			source = "local_estimate"
			response.Usage.PromptTokens = estimateEmbeddingTokens(request)
		}
		if response.Usage.TotalTokens <= 0 {
			response.Usage.TotalTokens = response.Usage.PromptTokens
		}
		usage := ChatUsage{PromptTokens: response.Usage.PromptTokens, TotalTokens: response.Usage.TotalTokens}
		if err := s.completeRelayBilling(ctx, reservation, freeID, usage, newCompletionID("embedding-"), channel.ID, source); err != nil {
			return EmbeddingResponse{}, err
		}
		s.recordChannelSuccess(ctx, channel)
		return response, nil
	}
	if lastErr == nil {
		lastErr = ErrProviderUnsupported
	}
	s.failRelayBilling(ctx, reservation, freeID, "upstream_embedding_failed")
	return EmbeddingResponse{}, lastErr
}
