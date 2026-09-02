package relay

import (
	"context"
	"errors"
	"io"
	"log"
	"strings"
	"time"

	"ai-token/internal/auth"
	"ai-token/internal/billing"
)

type StreamingChatCompletionService interface {
	StreamChatCompletions(context.Context, *auth.Principal, ChatCompletionRequest, func(ChatCompletionStreamEvent) error) error
}

type EmbeddingCompletionService interface {
	CreateEmbeddings(context.Context, *auth.Principal, EmbeddingRequest) (EmbeddingResponse, error)
}

const (
	maxStreamAttemptsPerChannel = 2
	streamRetryDelay            = 100 * time.Millisecond
)

type streamedChatAttempt struct {
	usage             ChatUsage
	gotUsage          bool
	delivered         bool
	output            string
	providerRequestID string
}

func openAndReadChatStream(
	ctx context.Context,
	provider StreamingProvider,
	upstream UpstreamChatCompletionRequest,
	emit func(ChatCompletionStreamEvent) error,
) (streamedChatAttempt, error) {
	var result streamedChatAttempt
	stream, err := provider.NewChatCompletionStream(ctx, upstream)
	if err != nil {
		return result, err
	}
	defer stream.Close()

	var output strings.Builder
	for {
		event, recvErr := stream.Recv()
		if errors.Is(recvErr, io.EOF) {
			result.output = output.String()
			return result, nil
		}
		if recvErr != nil {
			result.output = output.String()
			return result, recvErr
		}
		if event.ID != "" {
			result.providerRequestID = event.ID
		}
		if event.Delta != "" {
			output.WriteString(event.Delta)
		}
		if event.HasUsage {
			result.usage = mergeChatUsage(result.usage, event.Usage)
			result.gotUsage = result.gotUsage || chatUsageHasValues(event.Usage)
		}
		// Mark before invoking the sink: an HTTP writer may have committed
		// headers before returning an error, so failover would corrupt the
		// stream even when this callback reports failure.
		result.delivered = true
		if emitErr := emit(event); emitErr != nil {
			result.output = output.String()
			return result, emitErr
		}
	}
}

func waitForStreamRetry(ctx context.Context, attempt int) error {
	delay := streamRetryDelay * time.Duration(attempt+1)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
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
	startedAt := time.Now()
	request.Stream = true
	var reservation billing.Reservation
	var freeRequestID string
	defer func() {
		s.recordRelayRequestMetrics(ctx, reservation, freeRequestID, startedAt)
	}()
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
				if billingEnabled && errors.Is(err, billing.ErrPriceNotConfigured) {
					lastErr = err
					continue
				}
				return err
			}
		}
		if err := s.bindReservationToCandidate(ctx, reservation, channel.ID); err != nil {
			lastErr = err
			if errors.Is(err, billing.ErrPriceNotConfigured) {
				continue
			}
			s.failRelayBilling(ctx, reservation, freeRequestID, "billing_channel_bind_failed")
			return err
		}
		upstreamModel := channel.UpstreamModelName
		if strings.TrimSpace(upstreamModel) == "" {
			upstreamModel = request.Model
		}
		for streamAttempt := 0; streamAttempt < maxStreamAttemptsPerChannel; streamAttempt++ {
			attempt, streamErr := openAndReadChatStream(ctx, streamProvider, UpstreamChatCompletionRequest{
				Channel: channel, APIKey: apiKey, Request: request, UpstreamModel: upstreamModel,
			}, emit)
			if streamErr != nil {
				lastErr = streamErr
				s.recordChannelFailure(ctx, channel, streamErr)
				if attempt.delivered {
					if billingEnabled {
						if attempt.gotUsage {
							if pendingErr := s.markRelayBillingPendingWithUsage(ctx, reservation, freeRequestID, "upstream_stream_interrupted", chatBillingUsage(attempt.usage, "upstream"), attempt.providerRequestID); pendingErr != nil {
								log.Printf("relay billing pending write failed after streamed response: reservation=%s request=%s reason=%s err=%v", reservation.ID, freeRequestID, "upstream_stream_interrupted", pendingErr)
							}
						} else {
							if pendingErr := s.markRelayBillingPending(ctx, reservation, freeRequestID, "upstream_stream_usage_unavailable"); pendingErr != nil {
								log.Printf("relay billing pending write failed after streamed response: reservation=%s request=%s reason=%s err=%v", reservation.ID, freeRequestID, "upstream_stream_usage_unavailable", pendingErr)
							}
						}
					} else {
						s.failRelayBilling(ctx, reservation, freeRequestID, "upstream_stream_interrupted")
					}
					return streamErr
				}
				if ctx.Err() != nil {
					s.failRelayBilling(ctx, reservation, freeRequestID, "upstream_stream_open_failed")
					return ctx.Err()
				}
				// A transient failure may be retried on the same channel. A
				// credential, endpoint, or model-specific failure must instead
				// move directly to the next candidate channel when failover is
				// possible.
				if !retryableUpstreamError(streamErr) {
					log.Printf(
						"stream upstream request rejected before response: channel=%s provider=%s model=%s attempt=%d status=%d err=%v",
						channel.ID, canonicalProvider(channel.Provider), upstreamModel, streamAttempt+1, upstreamStatusCode(streamErr), streamErr,
					)
					s.failRelayBilling(ctx, reservation, freeRequestID, "upstream_stream_open_failed")
					return streamErr
				}
				if retryableStreamError(streamErr) && streamAttempt+1 < maxStreamAttemptsPerChannel {
					log.Printf(
						"stream upstream transient failure; retrying candidate: channel=%s provider=%s model=%s attempt=%d status=%d err=%v",
						channel.ID, canonicalProvider(channel.Provider), upstreamModel, streamAttempt+1, upstreamStatusCode(streamErr), streamErr,
					)
					if retryErr := waitForStreamRetry(ctx, streamAttempt); retryErr != nil {
						s.failRelayBilling(ctx, reservation, freeRequestID, "upstream_stream_open_failed")
						return retryErr
					}
					continue
				}
				log.Printf(
					"stream upstream candidate exhausted before response: channel=%s provider=%s model=%s attempt=%d status=%d err=%v",
					channel.ID, canonicalProvider(channel.Provider), upstreamModel, streamAttempt+1, upstreamStatusCode(streamErr), streamErr,
				)
				break
			}

			usage := attempt.usage
			gotUsage := attempt.gotUsage
			output := attempt.output
			providerRequestID := attempt.providerRequestID
			source := "upstream"
			if !gotUsage {
				if billingEnabled {
					if canSettle, policyErr := s.canSettleWithoutUsage(ctx, reservation); policyErr != nil {
						return policyErr
					} else if canSettle {
						if err := s.completeRelayBilling(ctx, reservation, freeRequestID, ChatUsage{}, providerRequestID, channel.ID, source); err != nil {
							return err
						}
						s.recordChannelSuccess(ctx, channel)
						return nil
					}
					if err := s.markRelayBillingPending(ctx, reservation, freeRequestID, "upstream_stream_usage_unavailable"); err != nil {
						return err
					}
					return nil
				}
				if !canEstimateChatUsage(request) {
					if billingEnabled {
						s.markRelayBillingPending(ctx, reservation, freeRequestID, "upstream_stream_usage_unavailable")
					} else {
						s.failRelayBilling(ctx, reservation, freeRequestID, "upstream_usage_unavailable")
					}
					return ErrUsageUnavailable
				}
				source = "local_estimate"
				usage = ChatUsage{PromptTokens: estimateInputTokens(request), CompletionTokens: int64(len([]rune(output))), TotalTokens: estimateInputTokens(request) + int64(len([]rune(output)))}
			} else {
				inputTokens, _, outputTokens, _ := usage.billingBreakdown()
				if inputTokens <= 0 || (outputTokens <= 0 && len(output) > 0) {
					if billingEnabled {
						if err := s.markRelayBillingPendingWithUsage(ctx, reservation, freeRequestID, "upstream_stream_usage_incomplete", chatBillingUsage(usage, "upstream"), providerRequestID); err != nil {
							return err
						}
						return nil
					}
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
					if outputTokens <= 0 && len(output) > 0 {
						usage.CompletionTokens = int64(len([]rune(output)))
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
		RequestID: request.RequestID, IdempotencyKey: scopedBillingIdempotencyKey(principal, metadata, request.IdempotencyKey),
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
	billingUsage := chatBillingUsage(usage, source)
	if reservation.ID != "" {
		if err := s.bindReservationToCandidate(ctx, reservation, channelID); err != nil {
			if pendingErr := s.markRelayBillingPendingWithUsage(ctx, reservation, freeID, "billing_rebind_failed", billingUsage, providerRequestID); pendingErr == nil {
				return nil
			} else {
				log.Printf("relay billing rebind pending write failed: reservation=%s request=%s reason=%s err=%v", reservation.ID, freeID, "billing_rebind_failed", pendingErr)
			}
			return err
		}
		if err := s.billing.Settle(ctx, reservation.ID, billingUsage, providerRequestID); err != nil {
			if pendingErr := s.markRelayBillingPendingWithUsage(ctx, reservation, freeID, "billing_settlement_failed", billingUsage, providerRequestID); pendingErr == nil {
				return nil
			} else {
				log.Printf("relay billing settlement pending write failed: reservation=%s request=%s reason=%s err=%v", reservation.ID, freeID, "billing_settlement_failed", pendingErr)
			}
			return err
		}
		return nil
	}
	if freeID != "" {
		if recorder, ok := s.billing.(billing.FreeRequestRecorder); ok {
			if err := recorder.RebindRequestChannel(ctx, freeID, channelID); err != nil {
				_ = recorder.FailFreeRequest(ctx, freeID, "billing_rebind_failed")
				return err
			}
			if err := recorder.CompleteFreeRequest(ctx, freeID, billingUsage, providerRequestID); err != nil {
				_ = recorder.FailFreeRequest(ctx, freeID, "billing_completion_failed")
				return err
			}
		}
	}
	return nil
}

func (s *Service) bindReservationToCandidate(ctx context.Context, reservation billing.Reservation, channelID string) error {
	if reservation.ID == "" || s == nil || s.billing == nil {
		return nil
	}
	rebinder, ok := s.billing.(billing.ReservationChannelRebinder)
	if !ok {
		return nil
	}
	return rebinder.RebindReservationChannel(ctx, reservation.ID, channelID)
}

func (s *Service) canSettleWithoutUsage(ctx context.Context, reservation billing.Reservation) (bool, error) {
	if reservation.ID == "" || s == nil || s.billing == nil {
		return false, nil
	}
	policy, ok := s.billing.(billing.ReservationUsagePolicy)
	if !ok {
		return false, nil
	}
	return policy.CanSettleWithoutUsage(ctx, reservation.ID)
}

func chatBillingUsage(usage ChatUsage, source string) billing.Usage {
	input, cached, output, reasoning := usage.billingBreakdown()
	return billing.Usage{
		InputTokens: input, OutputTokens: output, CachedInputTokens: cached, ReasoningTokens: reasoning,
		Metrics: usage.meteredUsage(), PricingTier: usage.PricingTier, Source: source, Raw: marshalUsageForBilling(usage),
	}
}

func chatUsageIsComplete(usage ChatUsage, hasOutput bool) bool {
	inputTokens, _, outputTokens, _ := usage.billingBreakdown()
	return inputTokens > 0 && (!hasOutput || outputTokens > 0)
}

func chatResponseHasOutput(response ChatCompletionResponse) bool {
	for _, choice := range response.Choices {
		if strings.TrimSpace(choice.Message.Content) != "" ||
			(len(choice.Message.ToolCalls) > 0 && string(choice.Message.ToolCalls) != "null") ||
			(len(choice.Message.FunctionCall) > 0 && string(choice.Message.FunctionCall) != "null") {
			return true
		}
	}
	return false
}

func (s *Service) recordRelayRequestMetrics(ctx context.Context, reservation billing.Reservation, freeID string, startedAt time.Time) {
	if s == nil || s.billing == nil {
		return
	}
	requestID := reservation.ModelRequestID
	if requestID == "" {
		requestID = freeID
	}
	if recorder, ok := s.billing.(billing.RequestMetricsRecorder); ok && requestID != "" {
		_ = recorder.RecordRequestMetrics(ctx, requestID, time.Since(startedAt).Milliseconds())
	}
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

func (s *Service) markRelayBillingPending(ctx context.Context, reservation billing.Reservation, freeID, reason string) error {
	if reservation.ID != "" && s.billing != nil {
		if marker, ok := s.billing.(billing.ReservationPendingMarker); ok {
			return marker.MarkSettlementPending(ctx, reservation.ID, reason)
		}
		return billing.ErrUnavailable
	}
	if freeID != "" && s.billing != nil {
		if recorder, ok := s.billing.(billing.FreeRequestRecorder); ok {
			return recorder.FailFreeRequest(ctx, freeID, reason)
		}
	}
	return nil
}

func (s *Service) markRelayBillingPendingWithUsage(
	ctx context.Context,
	reservation billing.Reservation,
	freeID, reason string,
	usage billing.Usage,
	providerRequestID string,
) error {
	if reservation.ID != "" && s.billing != nil {
		var markerErr error
		if marker, ok := s.billing.(billing.ReservationPendingUsageMarker); ok {
			if err := marker.MarkSettlementPendingWithUsage(ctx, reservation.ID, reason, usage, providerRequestID); err == nil {
				return nil
			} else {
				markerErr = err
			}
		}
		if marker, ok := s.billing.(billing.ReservationPendingMarker); ok {
			if err := marker.MarkSettlementPending(ctx, reservation.ID, reason); err == nil {
				return nil
			} else if markerErr == nil {
				markerErr = err
			}
		}
		if markerErr != nil {
			return markerErr
		}
		return billing.ErrUnavailable
	}
	if freeID != "" {
		if recorder, ok := s.billing.(billing.FreeRequestRecorder); ok {
			return recorder.FailFreeRequest(ctx, freeID, reason)
		}
	}
	return nil
}

func (s *Service) CreateEmbeddings(ctx context.Context, principal *auth.Principal, request EmbeddingRequest) (EmbeddingResponse, error) {
	if s == nil || s.router == nil || s.credentials == nil {
		return EmbeddingResponse{}, ErrUnavailable
	}
	startedAt := time.Now()
	var reservation billing.Reservation
	var freeID string
	defer func() {
		s.recordRelayRequestMetrics(ctx, reservation, freeID, startedAt)
	}()
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
			RequestID: request.RequestID, IdempotencyKey: scopedBillingIdempotencyKey(principal, metadata, request.IdempotencyKey), TenantID: principal.TenantID,
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
			if billingEnabled && errors.Is(err, billing.ErrPriceNotConfigured) {
				lastErr = err
				continue
			}
			return EmbeddingResponse{}, err
		}
		if err := s.bindReservationToCandidate(ctx, reservation, channel.ID); err != nil {
			lastErr = err
			if errors.Is(err, billing.ErrPriceNotConfigured) {
				continue
			}
			s.failRelayBilling(ctx, reservation, freeID, "billing_channel_bind_failed")
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
			if billingEnabled {
				if canSettle, policyErr := s.canSettleWithoutUsage(ctx, reservation); policyErr != nil {
					return EmbeddingResponse{}, policyErr
				} else if canSettle {
					if err := s.completeRelayBilling(ctx, reservation, freeID, ChatUsage{}, newCompletionID("embedding-"), channel.ID, "upstream"); err != nil {
						return EmbeddingResponse{}, err
					}
					s.recordChannelSuccess(ctx, channel)
					return response, nil
				}
				if err := s.markRelayBillingPending(ctx, reservation, freeID, "upstream_embedding_usage_unavailable"); err != nil {
					return EmbeddingResponse{}, err
				}
				s.recordChannelSuccess(ctx, channel)
				return response, nil
			}
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
