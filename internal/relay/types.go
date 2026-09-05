package relay

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	mathrand "math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ai-token/internal/auth"
	"ai-token/internal/billing"
)

const (
	ProviderOpenAI     = "openai"
	ProviderAnthropic  = "anthropic"
	ProviderGrok       = "grok"
	ProviderGemini     = "gemini"
	ProviderVolcengine = "volcengine"

	defaultAnthropicMaxTokens = 1024
	maxRequestedTokens        = 1_000_000
)

const (
	UpstreamIntegrationOfficial = "official"
	UpstreamIntegrationNewAPI   = "newapi"
	UpstreamIntegrationSub2API  = "sub2api"
	UpstreamIntegrationOther    = "other"
)

var (
	ErrUnavailable           = errors.New("relay service is unavailable")
	ErrInvalidRequest        = errors.New("invalid relay request")
	ErrUnsupportedFeature    = errors.New("relay feature is unsupported")
	ErrStreamingUnsupported  = errors.New("streaming relay is unsupported")
	ErrModelNotAllowed       = errors.New("model is not allowed for this token")
	ErrModelNotFound         = errors.New("model has no active channel")
	ErrProviderUnsupported   = errors.New("channel provider is unsupported")
	ErrCredentialUnavailable = errors.New("channel credential is unavailable")
	ErrCredentialRequired    = errors.New("channel credential is required")
	ErrChannelNotFound       = errors.New("channel is not found")
	ErrModelDiscoveryFailed  = errors.New("upstream model discovery failed")
	ErrUpstream              = errors.New("upstream request failed")
	ErrGroupUnavailable      = errors.New("routing group is unavailable")
	ErrGroupRateLimited      = errors.New("routing group rate limit exceeded")
	ErrUsageUnavailable      = errors.New("upstream usage is unavailable for this request")
)

type UpstreamError struct {
	StatusCode int
	Err        error
}

func (e *UpstreamError) Error() string {
	if e == nil || e.StatusCode == 0 {
		return ErrUpstream.Error()
	}
	return fmt.Sprintf("upstream request failed with status %d", e.StatusCode)
}

func (e *UpstreamError) Unwrap() error {
	if e == nil || e.Err == nil {
		return ErrUpstream
	}
	return e.Err
}

type ChatCompletionService interface {
	ChatCompletions(context.Context, *auth.Principal, ChatCompletionRequest) (ChatCompletionResponse, error)
}

type RequestMetadata struct {
	RequestID      string
	IdempotencyKey string
	Endpoint       string
	ClientIP       string
	RequestType    string
}

type requestMetadataKey struct{}

func WithRequestMetadata(ctx context.Context, metadata RequestMetadata) context.Context {
	return context.WithValue(ctx, requestMetadataKey{}, metadata)
}

func RequestMetadataFromContext(ctx context.Context) RequestMetadata {
	metadata, _ := ctx.Value(requestMetadataKey{}).(RequestMetadata)
	return metadata
}

type ChannelRouter interface {
	Select(context.Context, string) (Channel, error)
}

type ChannelCandidateRouter interface {
	SelectCandidates(context.Context, string) ([]Channel, error)
}

type GroupChannelCandidateRouter interface {
	SelectCandidatesForGroup(context.Context, string, string) ([]Channel, error)
}

type GroupPolicy struct {
	ID           string
	Status       string
	Multiplier   string
	RPMLimit     int
	BillingType  string
	MeteringMode string
}

type GroupPolicyResolver interface {
	ResolveGroupPolicy(context.Context, string) (GroupPolicy, error)
}

type GroupRPMConsumer interface {
	ConsumeGroupRPM(context.Context, string, int) (bool, error)
}

type TokenRateLimiter interface {
	AcquireToken(context.Context, string, int64) (func(), error)
}

type ChannelHealthRecorder interface {
	RecordChannelFailure(context.Context, string, int) error
	RecordChannelSuccess(context.Context, string) error
}

// ChannelModelHealthRecorder records health for the concrete public-model to
// channel mapping. A channel can expose several models, so channel-level
// health alone is not sufficient for routing or status reporting.
type ChannelModelHealthRecorder interface {
	RecordChannelModelFailure(context.Context, string, string, int) error
	RecordChannelModelSuccess(context.Context, string, string) error
}

// ChannelModelProbeHealthRecorder stores active probe observations separately
// from real customer traffic. Probe results are diagnostics only and never
// affect routing eligibility or customer-facing health state.
type ChannelModelProbeHealthRecorder interface {
	RecordChannelModelProbeFailure(context.Context, string, string, int) error
	RecordChannelModelProbeSuccess(context.Context, string, string) error
}

// ModelProbeService performs an explicit, non-tenant probe against the
// configured upstream route. It must never create a billing reservation or a
// customer usage record.
type ModelProbeService interface {
	ProbeModel(context.Context, string, string) error
}

// ChannelDiscoveryConfigReader returns the persisted provider, endpoint, and
// credential reference for an existing channel. A stored credential may only
// be reused for the same endpoint; changing the endpoint requires a new key.
type ChannelDiscoveryConfigReader interface {
	DiscoveryConfig(context.Context, string) (ChannelDiscoveryConfig, error)
}

type ChannelDiscoveryConfig struct {
	Provider      string
	BaseURL       string
	CredentialRef string
}

type CredentialResolver interface {
	Resolve(context.Context, string) (string, error)
}

type ModelCatalogProvider interface {
	ListModels(context.Context, string, string) ([]DiscoveredModel, error)
}

type SecretBox interface {
	Seal([]byte) (string, error)
	Open(string) ([]byte, error)
}

type Provider interface {
	ChatCompletions(context.Context, UpstreamChatCompletionRequest) (ChatCompletionResponse, error)
}

// StreamingProvider is optional so providers that do not expose a streaming
// protocol can fail explicitly instead of returning a misleading buffered
// response.
type StreamingProvider interface {
	NewChatCompletionStream(context.Context, UpstreamChatCompletionRequest) (ChatCompletionStream, error)
}

type ChatCompletionStream interface {
	Recv() (ChatCompletionStreamEvent, error)
	Close() error
}

type ChatCompletionStreamEvent struct {
	ID           string
	Object       string
	Created      int64
	Model        string
	Index        int64
	Role         string
	Delta        string
	ToolCalls    json.RawMessage
	FunctionCall json.RawMessage
	FinishReason string
	Usage        ChatUsage
	HasUsage     bool
}

type EmbeddingProvider interface {
	CreateEmbeddings(context.Context, UpstreamEmbeddingRequest) (EmbeddingResponse, error)
}

type Service struct {
	router      ChannelRouter
	credentials CredentialResolver
	providers   map[string]Provider
	billing     billing.Service
	rateLimiter TokenRateLimiter
}

type Channel struct {
	ID                   string
	Name                 string
	Provider             string
	BaseURL              string
	CredentialRef        string
	ModelName            string
	UpstreamModelName    string
	UpstreamCostDiscount string
	Priority             int
	Weight               int
}

type ChannelSummary struct {
	ID                           string                `json:"id"`
	Name                         string                `json:"name"`
	Provider                     string                `json:"provider"`
	BaseURL                      string                `json:"base_url"`
	CredentialRef                string                `json:"credential_ref"`
	CredentialMode               string                `json:"credential_mode"`
	CredentialPreview            string                `json:"credential_preview"`
	HasCredential                bool                  `json:"has_credential"`
	Status                       string                `json:"status"`
	UpstreamCostDiscount         string                `json:"upstream_cost_discount"`
	UpstreamIntegration          string                `json:"upstream_integration"`
	HasUpstreamAccountCredential bool                  `json:"has_upstream_account_credential"`
	UpstreamAccountUserID        string                `json:"upstream_account_user_id"`
	UpstreamBalance              *string               `json:"upstream_balance,omitempty"`
	UpstreamBalanceUnit          string                `json:"upstream_balance_unit,omitempty"`
	UpstreamBalanceTotal         *string               `json:"upstream_balance_total,omitempty"`
	UpstreamBalanceUsed          *string               `json:"upstream_balance_used,omitempty"`
	UpstreamAccountPlanName      string                `json:"upstream_account_plan_name,omitempty"`
	UpstreamRateMultiplier       *string               `json:"upstream_rate_multiplier,omitempty"`
	UpstreamAccountSyncStatus    string                `json:"upstream_account_sync_status"`
	UpstreamAccountSyncError     string                `json:"upstream_account_sync_error,omitempty"`
	UpstreamAccountSyncedAt      *time.Time            `json:"upstream_account_synced_at,omitempty"`
	UpstreamAccountLastAttemptAt *time.Time            `json:"upstream_account_last_attempt_at,omitempty"`
	Priority                     int                   `json:"priority"`
	Weight                       int                   `json:"weight"`
	ConsecutiveFailures          int                   `json:"consecutive_failures"`
	AutoDisabledUntil            *time.Time            `json:"auto_disabled_until,omitempty"`
	LastFailureStatus            *int                  `json:"last_failure_status,omitempty"`
	Models                       []ChannelModelSummary `json:"models"`
	CreatedAt                    time.Time             `json:"created_at"`
	UpdatedAt                    time.Time             `json:"updated_at"`
}

type ChannelModelSummary struct {
	Model         string `json:"model"`
	Provider      string `json:"provider"`
	UpstreamModel string `json:"upstream_model"`
	Enabled       bool   `json:"enabled"`
	HealthStatus  string `json:"health_status"`
	ProbeHealth   string `json:"probe_health,omitempty"`
}

type ChannelLister interface {
	List(context.Context) ([]ChannelSummary, error)
}

type ChannelMutator interface {
	CreateChannel(context.Context, string, ChannelMutation) (ChannelSummary, error)
	UpdateChannel(context.Context, string, string, ChannelMutation) (ChannelSummary, error)
	SetChannelStatus(context.Context, string, string, string) (ChannelSummary, error)
	DeleteChannel(context.Context, string, string) error
}

// ChannelAccountSyncer refreshes an optional upstream account-information
// snapshot. The snapshot is operational metadata only; it must never be used
// by relay routing, health circuits, customer billing, or cost estimation.
type ChannelAccountSyncer interface {
	SyncChannelAccount(context.Context, string, string) (ChannelSummary, error)
}

type ModelDiscoveryRequest struct {
	ChannelID string `json:"channel_id,omitempty"`
	Provider  string `json:"provider"`
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"api_key"`
}

type DiscoveredModel struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Provider    string `json:"provider"`
}

type ChannelMutation struct {
	Name                           string                 `json:"name"`
	Provider                       string                 `json:"provider"`
	BaseURL                        string                 `json:"base_url"`
	APIKey                         string                 `json:"api_key,omitempty"`
	Status                         string                 `json:"status,omitempty"`
	UpstreamCostDiscount           string                 `json:"upstream_cost_discount,omitempty"`
	UpstreamIntegration            string                 `json:"upstream_integration,omitempty"`
	UpstreamAccountCredential      string                 `json:"upstream_account_credential,omitempty"`
	UpstreamAccountUserID          string                 `json:"upstream_account_user_id,omitempty"`
	ClearUpstreamAccountCredential bool                   `json:"clear_upstream_account_credential,omitempty"`
	Priority                       int                    `json:"priority"`
	Weight                         int                    `json:"weight"`
	Models                         []ChannelModelMutation `json:"models"`
}

type ChannelModelMutation struct {
	Model         string `json:"model"`
	UpstreamModel string `json:"upstream_model"`
	Enabled       *bool  `json:"enabled,omitempty"`
}

type UpstreamChatCompletionRequest struct {
	Channel       Channel
	APIKey        string
	Request       ChatCompletionRequest
	UpstreamModel string
}

type UpstreamEmbeddingRequest struct {
	Channel       Channel
	APIKey        string
	Request       EmbeddingRequest
	UpstreamModel string
}

func (r UpstreamEmbeddingRequest) model() string {
	if strings.TrimSpace(r.UpstreamModel) != "" {
		return strings.TrimSpace(r.UpstreamModel)
	}
	return strings.TrimSpace(r.Request.Model)
}

type EmbeddingRequest struct {
	Model          string          `json:"model"`
	Input          json.RawMessage `json:"input"`
	EncodingFormat string          `json:"encoding_format,omitempty"`
	Dimensions     *int64          `json:"dimensions,omitempty"`
	User           string          `json:"user,omitempty"`
	RequestID      string          `json:"-"`
	IdempotencyKey string          `json:"-"`
}

type EmbeddingResponse struct {
	Object string          `json:"object"`
	Data   []EmbeddingData `json:"data"`
	Model  string          `json:"model"`
	Usage  EmbeddingUsage  `json:"usage"`
}

type EmbeddingData struct {
	Object    string    `json:"object"`
	Embedding []float64 `json:"embedding"`
	Index     int64     `json:"index"`
}

type EmbeddingUsage struct {
	PromptTokens int64 `json:"prompt_tokens"`
	TotalTokens  int64 `json:"total_tokens"`
}

func NewService(
	router ChannelRouter,
	credentials CredentialResolver,
	providers map[string]Provider,
	billers ...billing.Service,
) (*Service, error) {
	return newService(router, credentials, providers, billers, nil)
}

func NewServiceWithTokenLimiter(
	router ChannelRouter,
	credentials CredentialResolver,
	providers map[string]Provider,
	billingService billing.Service,
	rateLimiter TokenRateLimiter,
) (*Service, error) {
	return newService(router, credentials, providers, []billing.Service{billingService}, rateLimiter)
}

func newService(
	router ChannelRouter,
	credentials CredentialResolver,
	providers map[string]Provider,
	billers []billing.Service,
	rateLimiter TokenRateLimiter,
) (*Service, error) {
	if router == nil {
		return nil, errors.New("channel router is required")
	}
	if credentials == nil {
		return nil, errors.New("credential resolver is required")
	}
	normalizedProviders := map[string]Provider{}
	for name, provider := range providers {
		if provider != nil {
			normalizedProviders[canonicalProvider(name)] = provider
		}
	}
	if len(normalizedProviders) == 0 {
		return nil, errors.New("at least one upstream provider is required")
	}
	var billingService billing.Service
	if len(billers) > 0 {
		billingService = billers[0]
	}
	return &Service{
		router:      router,
		credentials: credentials,
		providers:   normalizedProviders,
		billing:     billingService,
		rateLimiter: rateLimiter,
	}, nil
}

func (s *Service) ListChannels(ctx context.Context) ([]ChannelSummary, error) {
	if s == nil || s.router == nil {
		return nil, ErrUnavailable
	}
	lister, ok := s.router.(ChannelLister)
	if !ok {
		return nil, ErrUnavailable
	}
	return lister.List(ctx)
}

func (s *Service) SyncChannelAccount(ctx context.Context, actorID, channelID string) (ChannelSummary, error) {
	if s == nil || s.router == nil {
		return ChannelSummary{}, ErrUnavailable
	}
	syncer, ok := s.router.(ChannelAccountSyncer)
	if !ok {
		return ChannelSummary{}, ErrUnavailable
	}
	return syncer.SyncChannelAccount(ctx, actorID, channelID)
}

func (s *Service) DiscoverModels(ctx context.Context, request ModelDiscoveryRequest) ([]DiscoveredModel, error) {
	if s == nil || s.providers == nil {
		return nil, ErrUnavailable
	}
	request.ChannelID = strings.TrimSpace(request.ChannelID)
	request.Provider = canonicalProvider(request.Provider)
	request.BaseURL = strings.TrimSpace(request.BaseURL)
	request.APIKey = strings.TrimSpace(request.APIKey)
	if request.ChannelID != "" {
		reader, ok := s.router.(ChannelDiscoveryConfigReader)
		if !ok {
			return nil, ErrCredentialUnavailable
		}
		config, err := reader.DiscoveryConfig(ctx, request.ChannelID)
		if err != nil {
			return nil, err
		}
		configuredProvider := canonicalProvider(config.Provider)
		if configuredProvider == "" || !supportedProvider(configuredProvider) {
			return nil, ErrProviderUnsupported
		}
		if request.Provider != "" && request.Provider != configuredProvider {
			return nil, ErrInvalidRequest
		}
		request.Provider = configuredProvider
		// Keep the persisted endpoint only for older clients that omit it.
		if request.BaseURL == "" {
			request.BaseURL = strings.TrimSpace(config.BaseURL)
		}
		if request.APIKey == "" {
			request.APIKey, err = s.credentials.Resolve(ctx, strings.TrimSpace(config.CredentialRef))
			if err != nil {
				return nil, err
			}
		}
	}
	if !supportedProvider(request.Provider) {
		return nil, ErrProviderUnsupported
	}
	if !validBaseURL(request.BaseURL) {
		return nil, ErrInvalidRequest
	}
	if request.APIKey == "" {
		return nil, ErrCredentialRequired
	}
	if len(request.APIKey) > 4096 {
		return nil, ErrInvalidRequest
	}
	provider := s.providers[request.Provider]
	catalog, ok := provider.(ModelCatalogProvider)
	if !ok {
		return nil, ErrProviderUnsupported
	}
	models, err := catalog.ListModels(ctx, request.BaseURL, request.APIKey)
	if err != nil {
		return nil, err
	}
	return normalizeDiscoveredModels(request.Provider, models), nil
}

func (s *Service) CreateChannel(
	ctx context.Context,
	actorID string,
	request ChannelMutation,
) (ChannelSummary, error) {
	mutator, err := s.channelMutator()
	if err != nil {
		return ChannelSummary{}, err
	}
	return mutator.CreateChannel(ctx, actorID, request)
}

func (s *Service) UpdateChannel(
	ctx context.Context,
	actorID string,
	channelID string,
	request ChannelMutation,
) (ChannelSummary, error) {
	mutator, err := s.channelMutator()
	if err != nil {
		return ChannelSummary{}, err
	}
	return mutator.UpdateChannel(ctx, actorID, channelID, request)
}

func (s *Service) SetChannelStatus(
	ctx context.Context,
	actorID string,
	channelID string,
	status string,
) (ChannelSummary, error) {
	mutator, err := s.channelMutator()
	if err != nil {
		return ChannelSummary{}, err
	}
	return mutator.SetChannelStatus(ctx, actorID, channelID, status)
}

func (s *Service) DeleteChannel(ctx context.Context, actorID string, channelID string) error {
	mutator, err := s.channelMutator()
	if err != nil {
		return err
	}
	return mutator.DeleteChannel(ctx, actorID, channelID)
}

func (s *Service) channelMutator() (ChannelMutator, error) {
	if s == nil || s.router == nil {
		return nil, ErrUnavailable
	}
	mutator, ok := s.router.(ChannelMutator)
	if !ok {
		return nil, ErrUnavailable
	}
	return mutator, nil
}

func DefaultProviders() map[string]Provider {
	return map[string]Provider{
		ProviderOpenAI:     OpenAIProvider{},
		ProviderAnthropic:  AnthropicProvider{},
		ProviderGrok:       GrokProvider{},
		ProviderGemini:     GeminiProvider{},
		ProviderVolcengine: VolcengineProvider{},
	}
}

func (m ChannelMutation) validate(requireAPIKey bool) (ChannelMutation, error) {
	m.Name = strings.TrimSpace(m.Name)
	m.Provider = canonicalProvider(m.Provider)
	m.BaseURL = strings.TrimSpace(m.BaseURL)
	m.APIKey = strings.TrimSpace(m.APIKey)
	m.Status = normalizeChannelStatus(m.Status)
	m.UpstreamIntegration = normalizeUpstreamIntegration(m.UpstreamIntegration)
	m.UpstreamAccountCredential = strings.TrimSpace(m.UpstreamAccountCredential)
	m.UpstreamAccountUserID = strings.TrimSpace(m.UpstreamAccountUserID)
	if normalized, ok := normalizeUpstreamCostDiscount(m.UpstreamCostDiscount); ok {
		m.UpstreamCostDiscount = normalized
	} else {
		return ChannelMutation{}, ErrInvalidRequest
	}
	m.Models = cleanChannelModels(m.Models)

	if m.Name == "" || !supportedProvider(m.Provider) || m.BaseURL == "" || m.UpstreamIntegration == "" {
		return ChannelMutation{}, ErrInvalidRequest
	}
	if requireAPIKey && m.APIKey == "" {
		return ChannelMutation{}, ErrCredentialRequired
	}
	if !validBaseURL(m.BaseURL) || !validChannelStatus(m.Status) {
		return ChannelMutation{}, ErrInvalidRequest
	}
	if m.Priority < 0 || m.Priority > 10000 || m.Weight < 0 || m.Weight > 10000 {
		return ChannelMutation{}, ErrInvalidRequest
	}
	if len(m.UpstreamAccountCredential) > 4096 || len(m.UpstreamAccountUserID) > 256 {
		return ChannelMutation{}, ErrInvalidRequest
	}
	return m, nil
}

func normalizeUpstreamIntegration(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", UpstreamIntegrationOfficial:
		return UpstreamIntegrationOfficial
	case UpstreamIntegrationNewAPI:
		return UpstreamIntegrationNewAPI
	case UpstreamIntegrationSub2API:
		return UpstreamIntegrationSub2API
	case UpstreamIntegrationOther:
		return UpstreamIntegrationOther
	default:
		return ""
	}
}

func normalizeUpstreamCostDiscount(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "1"
	}
	for _, char := range value {
		if (char < '0' || char > '9') && char != '.' {
			return "", false
		}
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 || (len(parts) == 2 && len(parts[1]) > 18) {
		return "", false
	}
	rat, ok := new(big.Rat).SetString(value)
	if !ok || rat.Sign() < 0 || rat.Cmp(new(big.Rat).SetInt64(1000)) > 0 {
		return "", false
	}
	return trimDecimalZeros(rat.FloatString(18)), true
}

func trimDecimalZeros(value string) string {
	value = strings.TrimSpace(value)
	if !strings.Contains(value, ".") {
		return value
	}
	value = strings.TrimRight(value, "0")
	value = strings.TrimRight(value, ".")
	if value == "" || value == "-0" {
		return "0"
	}
	return value
}

func (s *Service) ChatCompletions(
	ctx context.Context,
	principal *auth.Principal,
	request ChatCompletionRequest,
) (ChatCompletionResponse, error) {
	if s == nil || s.router == nil || s.credentials == nil {
		return ChatCompletionResponse{}, ErrUnavailable
	}
	if request.Stream {
		return ChatCompletionResponse{}, ErrStreamingUnsupported
	}
	if err := request.validate(); err != nil {
		return ChatCompletionResponse{}, err
	}
	startedAt := time.Now()
	metadata := RequestMetadataFromContext(ctx)
	requestType := strings.TrimSpace(metadata.RequestType)
	if requestType == "" {
		requestType = "sync"
	}
	if !modelAllowed(principal, request.Model) {
		return ChatCompletionResponse{}, ErrModelNotAllowed
	}
	if s.rateLimiter != nil && principal != nil && principal.TokenID != "" {
		release, limitErr := s.rateLimiter.AcquireToken(ctx, principal.TokenID, estimateInputTokens(request)+estimateOutputTokens(request))
		if limitErr != nil {
			return ChatCompletionResponse{}, limitErr
		}
		defer release()
	}
	groupPolicy, err := s.resolveGroupPolicy(ctx, principal)
	if err != nil {
		return ChatCompletionResponse{}, err
	}
	if strings.TrimSpace(groupPolicy.Status) != "" && groupPolicy.Status != "active" {
		return ChatCompletionResponse{}, ErrGroupUnavailable
	}
	if strings.TrimSpace(principal.TenantID) != "" && s.billing == nil {
		return ChatCompletionResponse{}, billing.ErrUnavailable
	}
	if groupPolicy.RPMLimit > 0 {
		limiter, ok := s.router.(GroupRPMConsumer)
		if !ok {
			return ChatCompletionResponse{}, ErrUnavailable
		}
		allowed, err := limiter.ConsumeGroupRPM(ctx, groupPolicy.ID, groupPolicy.RPMLimit)
		if err != nil {
			return ChatCompletionResponse{}, err
		}
		if !allowed {
			return ChatCompletionResponse{}, ErrGroupRateLimited
		}
	}

	candidates, err := s.channelCandidates(ctx, principal, request.Model)
	if err != nil {
		return ChatCompletionResponse{}, err
	}
	if len(candidates) == 0 {
		return ChatCompletionResponse{}, ErrModelNotFound
	}

	var reservation billing.Reservation
	var freeRequestID string
	defer func() {
		s.recordRelayRequestMetrics(ctx, reservation, freeRequestID, startedAt)
	}()
	var lastErr error
	billingEnabled := s.billing != nil && groupPolicy.BillingType != "free"
	attempted := map[string]struct{}{}
	for len(attempted) < len(candidates) {
		channel, ok := pickWeightedChannel(candidates, attempted)
		if !ok {
			break
		}
		attempted[channelKey(channel)] = struct{}{}

		provider := s.providers[canonicalProvider(channel.Provider)]
		if provider == nil {
			lastErr = ErrProviderUnsupported
			continue
		}
		apiKey, resolveErr := s.credentials.Resolve(ctx, channel.CredentialRef)
		if resolveErr != nil {
			lastErr = resolveErr
			s.recordChannelFailure(ctx, channel, resolveErr)
			continue
		}

		if billingEnabled && reservation.ID == "" {
			reservation, err = s.billing.Reserve(ctx, billing.Request{
				RequestID:             request.RequestID,
				IdempotencyKey:        scopedBillingIdempotencyKey(principal, metadata, request.IdempotencyKey),
				TenantID:              principal.TenantID,
				ProjectID:             principalProjectID(principal),
				TokenID:               principal.TokenID,
				Model:                 request.Model,
				Provider:              canonicalProvider(channel.Provider),
				ChannelID:             channel.ID,
				GroupID:               groupPolicy.ID,
				GroupMultiplier:       groupPolicy.Multiplier,
				MeteringMode:          groupPolicy.MeteringMode,
				UpstreamCostDiscount:  channel.UpstreamCostDiscount,
				EstimatedInputTokens:  estimateInputTokens(request),
				EstimatedOutputTokens: estimateOutputTokens(request),
				Endpoint:              metadata.Endpoint,
				ClientIP:              metadata.ClientIP,
				RequestType:           requestType,
				ReasoningEffort:       request.ReasoningEffort,
				PricingTier:           request.ServiceTier,
				BillingType:           groupPolicy.BillingType,
			})
			if err != nil {
				if errors.Is(err, billing.ErrPriceNotConfigured) {
					lastErr = err
					continue
				}
				return ChatCompletionResponse{}, err
			}
		}
		if !billingEnabled && freeRequestID == "" {
			if recorder, ok := s.billing.(billing.FreeRequestRecorder); ok {
				freeRequestID, err = recorder.StartFreeRequest(ctx, billing.Request{
					RequestID:            request.RequestID,
					IdempotencyKey:       scopedBillingIdempotencyKey(principal, metadata, request.IdempotencyKey),
					TenantID:             principal.TenantID,
					ProjectID:            principalProjectID(principal),
					TokenID:              principal.TokenID,
					Model:                request.Model,
					Provider:             canonicalProvider(channel.Provider),
					ChannelID:            channel.ID,
					GroupID:              groupPolicy.ID,
					GroupMultiplier:      groupPolicy.Multiplier,
					MeteringMode:         groupPolicy.MeteringMode,
					UpstreamCostDiscount: channel.UpstreamCostDiscount,
					Endpoint:             metadata.Endpoint,
					ClientIP:             metadata.ClientIP,
					RequestType:          requestType,
					ReasoningEffort:      request.ReasoningEffort,
					PricingTier:          request.ServiceTier,
					BillingType:          groupPolicy.BillingType,
				})
				if err != nil {
					return ChatCompletionResponse{}, err
				}
			}
		}
		if err := s.bindReservationToCandidate(ctx, reservation, channel.ID); err != nil {
			lastErr = err
			if errors.Is(err, billing.ErrPriceNotConfigured) {
				continue
			}
			s.failRelayBilling(ctx, reservation, freeRequestID, "billing_channel_bind_failed")
			return ChatCompletionResponse{}, err
		}

		upstreamModel := strings.TrimSpace(channel.UpstreamModelName)
		if upstreamModel == "" {
			upstreamModel = request.Model
		}
		response, callErr := provider.ChatCompletions(ctx, UpstreamChatCompletionRequest{
			Channel:       channel,
			APIKey:        apiKey,
			Request:       request,
			UpstreamModel: upstreamModel,
		})
		if callErr != nil {
			lastErr = callErr
			s.recordChannelFailure(ctx, channel, callErr)
			if ctx.Err() != nil || !retryableUpstreamError(callErr) {
				break
			}
			continue
		}
		s.recordChannelSuccess(ctx, channel)

		if response.ID == "" {
			response.ID = newCompletionID("chatcmpl_")
		}
		if response.Object == "" {
			response.Object = "chat.completion"
		}
		if response.Created == 0 {
			response.Created = time.Now().Unix()
		}
		response.Model = request.Model
		if strings.TrimSpace(response.Usage.PricingTier) == "" {
			response.Usage.PricingTier = request.ServiceTier
		}
		source := "upstream"
		hadUpstreamUsage := chatUsageHasValues(response.Usage)
		if !hadUpstreamUsage {
			if billingEnabled {
				if canSettle, policyErr := s.canSettleWithoutUsage(ctx, reservation); policyErr != nil {
					return ChatCompletionResponse{}, policyErr
				} else if canSettle {
					if err := s.completeRelayBilling(ctx, reservation, freeRequestID, ChatUsage{}, response.ID, channel.ID, "upstream"); err != nil {
						return ChatCompletionResponse{}, err
					}
					s.recordChannelSuccess(ctx, channel)
					return response, nil
				}
				if err := s.markRelayBillingPending(ctx, reservation, freeRequestID, "upstream_usage_unavailable"); err != nil {
					return ChatCompletionResponse{}, err
				}
				return response, nil
			}
			if !canEstimateChatUsage(request) {
				s.failRelayBilling(ctx, reservation, freeRequestID, "upstream_usage_unavailable")
				return ChatCompletionResponse{}, ErrUsageUnavailable
			}
		}
		if hadUpstreamUsage && !chatUsageIsComplete(response.Usage, chatResponseHasOutput(response)) {
			if billingEnabled {
				if err := s.markRelayBillingPendingWithUsage(
					ctx,
					reservation,
					freeRequestID,
					"upstream_usage_incomplete",
					chatBillingUsage(response.Usage, "upstream"),
					response.ID,
				); err != nil {
					return ChatCompletionResponse{}, err
				}
				return response, nil
			}
			if !canEstimateChatUsage(request) {
				s.failRelayBilling(ctx, reservation, freeRequestID, "upstream_usage_incomplete")
				return ChatCompletionResponse{}, ErrUsageUnavailable
			}
			if input, _, output, _ := response.Usage.billingBreakdown(); input <= 0 {
				response.Usage.PromptTokens = estimateInputTokens(request)
			} else if output <= 0 && chatResponseHasOutput(response) {
				response.Usage.CompletionTokens = estimateResponseTokens(response)
			}
			source = "local_estimate"
		}
		if !hadUpstreamUsage {
			response.Usage.PromptTokens = estimateInputTokens(request)
			response.Usage.CompletionTokens = estimateResponseTokens(response)
			source = "local_estimate"
		}
		response.Usage.TotalTokens = response.Usage.PromptTokens + response.Usage.CompletionTokens
		if billingEnabled || freeRequestID != "" {
			if err := s.completeRelayBilling(ctx, reservation, freeRequestID, response.Usage, response.ID, channel.ID, source); err != nil {
				// A successful upstream response must never become an untracked
				// or free request when the accounting write fails. Paid requests
				// remain pending with the Usage snapshot for reconciliation.
				return ChatCompletionResponse{}, err
			}
		}
		return response, nil
	}

	if reservation.ID != "" {
		if releaseErr := s.billing.Fail(ctx, reservation.ID, "upstream_request_failed"); releaseErr != nil {
			return ChatCompletionResponse{}, releaseErr
		}
	}
	if freeRequestID != "" {
		if recorder, ok := s.billing.(billing.FreeRequestRecorder); ok {
			if releaseErr := recorder.FailFreeRequest(ctx, freeRequestID, "upstream_request_failed"); releaseErr != nil {
				return ChatCompletionResponse{}, releaseErr
			}
		}
	}
	requestRecordID := reservation.ModelRequestID
	if requestRecordID == "" {
		requestRecordID = freeRequestID
	}
	if recorder, ok := s.billing.(billing.RequestMetricsRecorder); ok && requestRecordID != "" {
		_ = recorder.RecordRequestMetrics(ctx, requestRecordID, time.Since(startedAt).Milliseconds())
	}
	if lastErr == nil {
		lastErr = ErrUpstream
	}
	return ChatCompletionResponse{}, lastErr
}

func (s *Service) resolveGroupPolicy(ctx context.Context, principal *auth.Principal) (GroupPolicy, error) {
	policy := GroupPolicy{Multiplier: "1.000000", BillingType: "prepaid", MeteringMode: billing.MeteringToken}
	if principal == nil || strings.TrimSpace(principal.GroupID) == "" {
		return policy, nil
	}
	resolver, ok := s.router.(GroupPolicyResolver)
	if !ok {
		return GroupPolicy{}, ErrUnavailable
	}
	policy, err := resolver.ResolveGroupPolicy(ctx, strings.TrimSpace(principal.GroupID))
	if err != nil {
		return GroupPolicy{}, err
	}
	if policy.Status != "active" || policy.Multiplier == "" {
		return GroupPolicy{}, ErrGroupUnavailable
	}
	return policy, nil
}

func (s *Service) recordChannelFailure(ctx context.Context, channel Channel, err error) {
	if !retryableUpstreamError(err) {
		return
	}
	statusCode := upstreamStatusCode(err)
	if recorder, ok := s.router.(ChannelModelHealthRecorder); ok && strings.TrimSpace(channel.ModelName) != "" {
		_ = recorder.RecordChannelModelFailure(ctx, channel.ID, channel.ModelName, statusCode)
	}
	// Per-model failures must not disable unrelated models on the same channel.
	// Channel-wide health is reserved for credential and connection failures,
	// which make every mapping on the channel unavailable.
	if recorder, ok := s.router.(ChannelHealthRecorder); ok && (strings.TrimSpace(channel.ModelName) == "" || channelWideFailure(err)) {
		_ = recorder.RecordChannelFailure(ctx, channel.ID, statusCode)
	}
}

func (s *Service) recordChannelProbeFailure(ctx context.Context, channel Channel, err error) {
	if recorder, ok := s.router.(ChannelModelProbeHealthRecorder); ok && strings.TrimSpace(channel.ModelName) != "" {
		_ = recorder.RecordChannelModelProbeFailure(ctx, channel.ID, channel.ModelName, upstreamStatusCode(err))
	}
}

func (s *Service) recordChannelProbeSuccess(ctx context.Context, channel Channel) {
	if recorder, ok := s.router.(ChannelModelProbeHealthRecorder); ok && strings.TrimSpace(channel.ModelName) != "" {
		_ = recorder.RecordChannelModelProbeSuccess(ctx, channel.ID, channel.ModelName)
	}
}

func (s *Service) recordChannelSuccess(ctx context.Context, channel Channel) {
	if recorder, ok := s.router.(ChannelHealthRecorder); ok {
		_ = recorder.RecordChannelSuccess(ctx, channel.ID)
	}
	if recorder, ok := s.router.(ChannelModelHealthRecorder); ok && strings.TrimSpace(channel.ModelName) != "" {
		_ = recorder.RecordChannelModelSuccess(ctx, channel.ID, channel.ModelName)
	}
}

func (s *Service) channelCandidates(ctx context.Context, principal *auth.Principal, model string) ([]Channel, error) {
	groupID := ""
	if principal != nil {
		groupID = strings.TrimSpace(principal.GroupID)
	}
	if groupID != "" {
		groupRouter, ok := s.router.(GroupChannelCandidateRouter)
		if !ok {
			return nil, ErrUnavailable
		}
		return groupRouter.SelectCandidatesForGroup(ctx, model, groupID)
	}
	if candidateRouter, ok := s.router.(ChannelCandidateRouter); ok {
		return candidateRouter.SelectCandidates(ctx, model)
	}
	channel, err := s.router.Select(ctx, model)
	if err != nil {
		return nil, err
	}
	return []Channel{channel}, nil
}

// ProbeModel sends the smallest supported text request through the same
// group-aware candidate order used by customer traffic. The request is
// intentionally outside ChatCompletions so it cannot consume tenant quota or
// write a billing record.
func (s *Service) ProbeModel(ctx context.Context, groupID, model string) error {
	if s == nil || s.router == nil || s.credentials == nil {
		return ErrUnavailable
	}
	groupID = strings.TrimSpace(groupID)
	model = strings.TrimSpace(model)
	if groupID == "" || model == "" {
		return ErrInvalidRequest
	}
	if !probeableTextModel(model) {
		return ErrUnsupportedFeature
	}
	groupRouter, ok := s.router.(GroupChannelCandidateRouter)
	if !ok {
		return ErrUnavailable
	}
	candidates, err := groupRouter.SelectCandidatesForGroup(ctx, model, groupID)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return ErrModelNotFound
	}

	attempted := map[string]struct{}{}
	var lastErr error
	for len(attempted) < len(candidates) {
		channel, found := pickWeightedChannel(candidates, attempted)
		if !found {
			break
		}
		attempted[channelKey(channel)] = struct{}{}
		provider := s.providers[canonicalProvider(channel.Provider)]
		if provider == nil {
			lastErr = ErrProviderUnsupported
			continue
		}
		apiKey, resolveErr := s.credentials.Resolve(ctx, channel.CredentialRef)
		if resolveErr != nil {
			lastErr = resolveErr
			s.recordChannelProbeFailure(ctx, channel, resolveErr)
			continue
		}
		upstreamModel := strings.TrimSpace(channel.UpstreamModelName)
		if upstreamModel == "" {
			upstreamModel = model
		}
		probeRequest := minimalProbeRequest(canonicalProvider(channel.Provider), upstreamModel, model)
		_, callErr := provider.ChatCompletions(ctx, UpstreamChatCompletionRequest{
			Channel: channel, APIKey: apiKey, Request: probeRequest, UpstreamModel: upstreamModel,
		})
		if callErr == nil {
			s.recordChannelProbeSuccess(ctx, channel)
			return nil
		}
		lastErr = callErr
		s.recordChannelProbeFailure(ctx, channel, callErr)
		if ctx.Err() != nil || !retryableUpstreamError(callErr) {
			break
		}
	}
	if lastErr == nil {
		return ErrUpstream
	}
	return lastErr
}

func minimalProbeRequest(provider, upstreamModel, requestedModel string) ChatCompletionRequest {
	one := int64(1)
	request := ChatCompletionRequest{
		Model:    requestedModel,
		Messages: []ChatMessage{{Role: "user", Content: "x"}},
	}
	// OpenAI's newer reasoning models use max_completion_tokens, while
	// OpenAI-compatible Grok endpoints commonly expect max_tokens.
	// Anthropic and Gemini adapters accept either field, so select the
	// spelling per upstream provider for broad compatibility.
	if provider == ProviderOpenAI || provider == ProviderGemini {
		request.MaxCompletionTokens = &one
	} else {
		request.MaxTokens = &one
	}
	if provider == ProviderOpenAI && reasoningProbeModel(upstreamModel) {
		request.ReasoningEffort = "none"
	}
	if provider == ProviderGemini && geminiThinkingProbeModel(upstreamModel) {
		request.ReasoningEffort = "none"
	}
	return request
}

func reasoningProbeModel(model string) bool {
	name := strings.ToLower(strings.TrimSpace(model))
	return strings.Contains(name, "gpt-5") ||
		strings.HasPrefix(name, "o1") ||
		strings.HasPrefix(name, "o3") ||
		strings.HasPrefix(name, "o4") ||
		strings.Contains(name, "reason")
}

func geminiThinkingProbeModel(model string) bool {
	name := strings.ToLower(strings.TrimSpace(model))
	return strings.Contains(name, "2.5") ||
		strings.Contains(name, "2-5") ||
		strings.Contains(name, "thinking")
}

// ModelProbeOutcome describes a group-level probe that uses the configured
// primary model first and only falls back to the remaining models when it
// fails. A single model failure therefore does not make the whole group
// unavailable when another configured model succeeds.
type ModelProbeOutcome struct {
	Supported int
	Succeeded bool
	Failures  []string
}

func ProbeModelCandidates(
	ctx context.Context,
	prober ModelProbeService,
	groupID, primary string,
	models []string,
) ModelProbeOutcome {
	ordered := orderedProbeModels(primary, models)
	outcome := ModelProbeOutcome{}
	for _, model := range ordered {
		if ctx.Err() != nil {
			outcome.Failures = append(outcome.Failures, model+": "+ctx.Err().Error())
			break
		}
		probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		err := prober.ProbeModel(probeCtx, groupID, model)
		cancel()
		if errors.Is(err, ErrUnsupportedFeature) {
			continue
		}
		outcome.Supported++
		if err == nil {
			outcome.Succeeded = true
			break
		}
		outcome.Failures = append(outcome.Failures, model+": "+err.Error())
	}
	return outcome
}

func orderedProbeModels(primary string, models []string) []string {
	ordered := make([]string, 0, len(models)+1)
	seen := make(map[string]struct{}, len(models)+1)
	appendModel := func(model string) {
		model = strings.TrimSpace(model)
		if model == "" {
			return
		}
		if _, ok := seen[model]; ok {
			return
		}
		seen[model] = struct{}{}
		ordered = append(ordered, model)
	}
	appendModel(primary)
	for _, model := range models {
		appendModel(model)
	}
	return ordered
}

func probeableTextModel(model string) bool {
	name := strings.ToLower(strings.TrimSpace(model))
	for _, marker := range []string{
		"embedding", "embed-", "image", "dall-e", "imagen", "video",
		"veo", "sora", "audio", "whisper", "transcri", "speech", "tts",
	} {
		if strings.Contains(name, marker) {
			return false
		}
	}
	return true
}

func pickWeightedChannel(candidates []Channel, attempted map[string]struct{}) (Channel, bool) {
	maxPriority := 0
	found := false
	for _, candidate := range candidates {
		if _, ok := attempted[channelKey(candidate)]; ok {
			continue
		}
		if !found || candidate.Priority > maxPriority {
			maxPriority = candidate.Priority
			found = true
		}
	}
	if !found {
		return Channel{}, false
	}

	pool := make([]Channel, 0)
	totalWeight := 0
	for _, candidate := range candidates {
		if candidate.Priority != maxPriority {
			continue
		}
		if _, ok := attempted[channelKey(candidate)]; ok {
			continue
		}
		pool = append(pool, candidate)
		if candidate.Weight > 0 {
			totalWeight += candidate.Weight
		}
	}
	if len(pool) == 0 {
		return Channel{}, false
	}
	if totalWeight == 0 {
		return pool[mathrand.Intn(len(pool))], true
	}
	target := mathrand.Intn(totalWeight)
	for _, candidate := range pool {
		if candidate.Weight <= 0 {
			continue
		}
		target -= candidate.Weight
		if target < 0 {
			return candidate, true
		}
	}
	return pool[len(pool)-1], true
}

func channelKey(channel Channel) string {
	if strings.TrimSpace(channel.ID) != "" {
		return channel.ID
	}
	return channel.Provider + "|" + channel.BaseURL + "|" + channel.UpstreamModelName
}

func retryableUpstreamError(err error) bool {
	if err == nil || errors.Is(err, ErrInvalidRequest) || errors.Is(err, ErrUnsupportedFeature) ||
		errors.Is(err, ErrStreamingUnsupported) || errors.Is(err, ErrModelNotAllowed) ||
		errors.Is(err, ErrModelNotFound) {
		return false
	}
	var upstreamErr *UpstreamError
	if errors.As(err, &upstreamErr) {
		switch upstreamErr.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound,
			http.StatusRequestTimeout, http.StatusConflict, http.StatusTooEarly,
			http.StatusTooManyRequests:
			return true
		default:
			return upstreamErr.StatusCode >= http.StatusInternalServerError && upstreamErr.StatusCode <= 599
		}
	}
	return errors.Is(err, ErrUpstream) || errors.Is(err, ErrCredentialUnavailable) || errors.Is(err, ErrProviderUnsupported)
}

func retryableStreamError(err error) bool {
	if err == nil || errors.Is(err, ErrInvalidRequest) || errors.Is(err, ErrUnsupportedFeature) ||
		errors.Is(err, ErrStreamingUnsupported) || errors.Is(err, ErrModelNotAllowed) ||
		errors.Is(err, ErrModelNotFound) {
		return false
	}
	var upstreamErr *UpstreamError
	if errors.As(err, &upstreamErr) {
		if upstreamErr.StatusCode == 0 {
			return errors.Is(upstreamErr, ErrUpstream)
		}
		switch upstreamErr.StatusCode {
		case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests:
			return true
		default:
			return upstreamErr.StatusCode >= http.StatusInternalServerError && upstreamErr.StatusCode <= 599
		}
	}
	// Provider adapters use an empty-status UpstreamError for transport
	// failures, which are safe to retry before any bytes reach the client.
	return errors.Is(err, ErrUpstream)
}

func upstreamStatusCode(err error) int {
	var upstreamErr *UpstreamError
	if errors.As(err, &upstreamErr) {
		return upstreamErr.StatusCode
	}
	return 0
}

func channelWideFailure(err error) bool {
	if errors.Is(err, ErrCredentialUnavailable) {
		return true
	}
	var upstreamErr *UpstreamError
	if !errors.As(err, &upstreamErr) {
		// ErrUpstream without a response status denotes a transport-level
		// failure, so retrying another model on the same endpoint is unlikely
		// to help.
		return errors.Is(err, ErrUpstream)
	}
	return upstreamErr.StatusCode == http.StatusUnauthorized || upstreamErr.StatusCode == http.StatusForbidden
}

type ChatCompletionRequest struct {
	Model               string          `json:"model"`
	Messages            []ChatMessage   `json:"messages"`
	FrequencyPenalty    *float64        `json:"frequency_penalty,omitempty"`
	PresencePenalty     *float64        `json:"presence_penalty,omitempty"`
	N                   *int64          `json:"n,omitempty"`
	Seed                *int64          `json:"seed,omitempty"`
	Logprobs            *bool           `json:"logprobs,omitempty"`
	TopLogprobs         *int64          `json:"top_logprobs,omitempty"`
	ParallelToolCalls   *bool           `json:"parallel_tool_calls,omitempty"`
	ServiceTier         string          `json:"service_tier,omitempty"`
	PromptCacheKey      string          `json:"prompt_cache_key,omitempty"`
	SafetyIdentifier    string          `json:"safety_identifier,omitempty"`
	Verbosity           string          `json:"verbosity,omitempty"`
	Modalities          []string        `json:"modalities,omitempty"`
	Audio               json.RawMessage `json:"audio,omitempty"`
	Metadata            json.RawMessage `json:"metadata,omitempty"`
	Thinking            json.RawMessage `json:"-"`
	Temperature         *float64        `json:"temperature,omitempty"`
	TopP                *float64        `json:"top_p,omitempty"`
	MaxTokens           *int64          `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int64          `json:"max_completion_tokens,omitempty"`
	Stop                StopSequences   `json:"stop,omitempty"`
	Stream              bool            `json:"stream,omitempty"`
	User                string          `json:"user,omitempty"`
	Tools               json.RawMessage `json:"tools,omitempty"`
	ToolChoice          json.RawMessage `json:"tool_choice,omitempty"`
	Functions           json.RawMessage `json:"functions,omitempty"`
	FunctionCall        json.RawMessage `json:"function_call,omitempty"`
	ResponseFormat      json.RawMessage `json:"response_format,omitempty"`
	ReasoningEffort     string          `json:"reasoning_effort,omitempty"`
	RequestID           string          `json:"-"`
	IdempotencyKey      string          `json:"-"`
}

type ChatMessage struct {
	Role         string          `json:"role"`
	Content      string          `json:"-"`
	ContentParts json.RawMessage `json:"-"`
	Name         string          `json:"name,omitempty"`
	ToolCallID   string          `json:"tool_call_id,omitempty"`
	ToolCalls    json.RawMessage `json:"tool_calls,omitempty"`
	FunctionCall json.RawMessage `json:"function_call,omitempty"`
}

func (m ChatMessage) MarshalJSON() ([]byte, error) {
	var content any = m.Content
	if len(m.ContentParts) > 0 && string(m.ContentParts) != "null" {
		if err := json.Unmarshal(m.ContentParts, &content); err != nil {
			return nil, err
		}
	}
	return json.Marshal(struct {
		Role         string          `json:"role"`
		Content      any             `json:"content"`
		Name         string          `json:"name,omitempty"`
		ToolCallID   string          `json:"tool_call_id,omitempty"`
		ToolCalls    json.RawMessage `json:"tool_calls,omitempty"`
		FunctionCall json.RawMessage `json:"function_call,omitempty"`
	}{m.Role, content, m.Name, m.ToolCallID, m.ToolCalls, m.FunctionCall})
}

func (m *ChatMessage) UnmarshalJSON(data []byte) error {
	var value struct {
		Role         string          `json:"role"`
		Content      json.RawMessage `json:"content"`
		Name         string          `json:"name"`
		ToolCallID   string          `json:"tool_call_id"`
		ToolCalls    json.RawMessage `json:"tool_calls"`
		FunctionCall json.RawMessage `json:"function_call"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	m.Role, m.Name, m.ToolCallID = value.Role, value.Name, value.ToolCallID
	m.ToolCalls, m.FunctionCall = value.ToolCalls, value.FunctionCall
	m.Content, m.ContentParts = "", nil
	if len(value.Content) == 0 || string(value.Content) == "null" {
		return nil
	}
	if err := json.Unmarshal(value.Content, &m.Content); err == nil {
		return nil
	}
	m.ContentParts = append(json.RawMessage(nil), value.Content...)
	return nil
}

type ChatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []ChatCompletionChoice `json:"choices"`
	Usage   ChatUsage              `json:"usage,omitempty"`
}

type ChatCompletionChoice struct {
	Index        int64               `json:"index"`
	Message      ChatCompletionReply `json:"message"`
	FinishReason string              `json:"finish_reason,omitempty"`
}

type ChatCompletionReply struct {
	Role         string          `json:"role"`
	Content      string          `json:"content"`
	ToolCalls    json.RawMessage `json:"tool_calls,omitempty"`
	FunctionCall json.RawMessage `json:"function_call,omitempty"`
}

type ChatUsage struct {
	PromptTokens               int64                        `json:"prompt_tokens,omitempty"`
	CompletionTokens           int64                        `json:"completion_tokens,omitempty"`
	TotalTokens                int64                        `json:"total_tokens,omitempty"`
	PromptTokensDetails        *ChatPromptTokensDetails     `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails    *ChatCompletionTokensDetails `json:"completion_tokens_details,omitempty"`
	CacheCreationInputTokens   int64                        `json:"-"`
	CacheCreation1HInputTokens int64                        `json:"-"`
	CacheReadInputTokens       int64                        `json:"-"`
	PricingTier                string                       `json:"-"`
	Metrics                    billing.MeteredUsage         `json:"-"`
	UsageProvided              bool                         `json:"-"`
}

type ChatPromptTokensDetails struct {
	CachedTokens      int64 `json:"cached_tokens,omitempty"`
	AudioTokens       int64 `json:"audio_tokens,omitempty"`
	ImageTokens       int64 `json:"image_tokens,omitempty"`
	VideoTokens       int64 `json:"video_tokens,omitempty"`
	CachedAudioTokens int64 `json:"-"`
	CachedImageTokens int64 `json:"-"`
	CachedVideoTokens int64 `json:"-"`
}

type ChatCompletionTokensDetails struct {
	ReasoningTokens int64 `json:"reasoning_tokens,omitempty"`
	AudioTokens     int64 `json:"audio_tokens,omitempty"`
	ImageTokens     int64 `json:"image_tokens,omitempty"`
	VideoTokens     int64 `json:"video_tokens,omitempty"`
}

func (u ChatUsage) meteredUsage() billing.MeteredUsage {
	if len(u.Metrics) > 0 {
		return u.Metrics
	}
	input, cached, output, reasoning := u.billingBreakdown()
	inputAudio, inputImage, inputVideo := int64(0), int64(0), int64(0)
	cachedAudio, cachedImage, cachedVideo := int64(0), int64(0), int64(0)
	if u.PromptTokensDetails != nil {
		inputAudio = u.PromptTokensDetails.AudioTokens
		inputImage = u.PromptTokensDetails.ImageTokens
		inputVideo = u.PromptTokensDetails.VideoTokens
		cachedAudio = u.PromptTokensDetails.CachedAudioTokens
		cachedImage = u.PromptTokensDetails.CachedImageTokens
		cachedVideo = u.PromptTokensDetails.CachedVideoTokens
	}
	outputAudio, outputImage, outputVideo := int64(0), int64(0), int64(0)
	if u.CompletionTokensDetails != nil {
		outputAudio = u.CompletionTokensDetails.AudioTokens
		outputImage = u.CompletionTokensDetails.ImageTokens
		outputVideo = u.CompletionTokensDetails.VideoTokens
	}
	// Detail modalities are subsets of the provider's prompt/completion
	// totals. Cache-read tokens are a further subset of prompt modalities.
	cachedAudio, cachedImage, cachedVideo = clampUsageParts(cached, cachedAudio, cachedImage, cachedVideo)
	inputAudio, inputImage, inputVideo = clampUsageParts(input, inputAudio+cachedAudio, inputImage+cachedImage, inputVideo+cachedVideo)
	outputAudio, outputImage, outputVideo = clampUsageParts(output, outputAudio, outputImage, outputVideo)
	if cached > input {
		cached = input
	}
	genericCached := cached - cachedAudio - cachedImage - cachedVideo
	if genericCached < 0 {
		genericCached = 0
	}
	metrics := billing.MeteredUsage{
		"input_tokens":        strconv.FormatInt(input, 10),
		"output_tokens":       strconv.FormatInt(output, 10),
		"cached_input_tokens": strconv.FormatInt(genericCached, 10),
		"reasoning_tokens":    strconv.FormatInt(reasoning, 10),
	}
	if inputAudio > 0 {
		metrics["input_audio_tokens"] = strconv.FormatInt(inputAudio, 10)
	}
	if outputAudio > 0 {
		metrics["output_audio_tokens"] = strconv.FormatInt(outputAudio, 10)
	}
	if inputImage > 0 {
		metrics["input_image_tokens"] = strconv.FormatInt(inputImage, 10)
	}
	if outputImage > 0 {
		metrics["output_image_tokens"] = strconv.FormatInt(outputImage, 10)
	}
	if inputVideo > 0 {
		metrics["input_video_tokens"] = strconv.FormatInt(inputVideo, 10)
	}
	if outputVideo > 0 {
		metrics["output_video_tokens"] = strconv.FormatInt(outputVideo, 10)
	}
	if u.CacheCreationInputTokens > 0 {
		cacheCreation := u.CacheCreationInputTokens - u.CacheCreation1HInputTokens
		if cacheCreation < 0 {
			cacheCreation = 0
		}
		if cacheCreation > 0 {
			metrics["cache_creation_tokens"] = strconv.FormatInt(cacheCreation, 10)
		}
	}
	if u.CacheCreation1HInputTokens > 0 {
		metrics["cache_creation_1h_tokens"] = strconv.FormatInt(u.CacheCreation1HInputTokens, 10)
	}
	if cachedAudio > 0 {
		metrics["cached_audio_tokens"] = strconv.FormatInt(cachedAudio, 10)
	}
	if cachedImage > 0 {
		metrics["cached_image_tokens"] = strconv.FormatInt(cachedImage, 10)
	}
	if cachedVideo > 0 {
		metrics["cached_video_tokens"] = strconv.FormatInt(cachedVideo, 10)
	}
	return metrics
}

func chatUsageHasValues(usage ChatUsage) bool {
	if usage.PromptTokens > 0 || usage.CompletionTokens > 0 || usage.CacheCreationInputTokens > 0 || usage.CacheCreation1HInputTokens > 0 || usage.CacheReadInputTokens > 0 {
		return true
	}
	if usage.PromptTokensDetails != nil && (usage.PromptTokensDetails.CachedTokens > 0 || usage.PromptTokensDetails.AudioTokens > 0 || usage.PromptTokensDetails.ImageTokens > 0 || usage.PromptTokensDetails.VideoTokens > 0) {
		return true
	}
	return usage.CompletionTokensDetails != nil && (usage.CompletionTokensDetails.ReasoningTokens > 0 || usage.CompletionTokensDetails.AudioTokens > 0 || usage.CompletionTokensDetails.ImageTokens > 0 || usage.CompletionTokensDetails.VideoTokens > 0)
}

func marshalUsageForBilling(usage ChatUsage) []byte {
	value := map[string]any{
		"prompt_tokens": usage.PromptTokens, "completion_tokens": usage.CompletionTokens,
		"total_tokens":                   usage.TotalTokens,
		"cache_creation_input_tokens":    usage.CacheCreationInputTokens,
		"cache_creation_1h_input_tokens": usage.CacheCreation1HInputTokens,
		"cache_read_input_tokens":        usage.CacheReadInputTokens,
	}
	if usage.PromptTokensDetails != nil {
		value["prompt_tokens_details"] = usage.PromptTokensDetails
	}
	if usage.CompletionTokensDetails != nil {
		value["completion_tokens_details"] = usage.CompletionTokensDetails
	}
	if usage.PricingTier != "" {
		value["service_tier"] = usage.PricingTier
	}
	encoded, _ := json.Marshal(value)
	return encoded
}

func canEstimateChatUsage(request ChatCompletionRequest) bool {
	if (len(request.Tools) > 0 && string(request.Tools) != "null") || (len(request.Functions) > 0 && string(request.Functions) != "null") || (len(request.Audio) > 0 && string(request.Audio) != "null") {
		return false
	}
	for _, modality := range request.Modalities {
		if strings.ToLower(strings.TrimSpace(modality)) != "text" {
			return false
		}
	}
	for _, message := range request.Messages {
		if (len(message.ContentParts) > 0 && string(message.ContentParts) != "null") || (len(message.ToolCalls) > 0 && string(message.ToolCalls) != "null") || (len(message.FunctionCall) > 0 && string(message.FunctionCall) != "null") {
			return false
		}
	}
	return true
}

func clampUsageParts(total int64, values ...int64) (int64, int64, int64) {
	if total < 0 {
		total = 0
	}
	result := [3]int64{}
	for index, value := range values {
		if index >= len(result) || value <= 0 || total == 0 {
			continue
		}
		if value > total {
			value = total
		}
		result[index] = value
		total -= value
	}
	return result[0], result[1], result[2]
}

func (u ChatUsage) billingBreakdown() (inputTokens, cachedInputTokens, outputTokens, reasoningTokens int64) {
	inputTokens = u.PromptTokens
	if inputTokens <= 0 && u.PromptTokensDetails != nil {
		inputTokens = u.PromptTokensDetails.CachedTokens + u.PromptTokensDetails.AudioTokens + u.PromptTokensDetails.ImageTokens + u.PromptTokensDetails.VideoTokens
	}
	if u.PromptTokensDetails != nil && u.PromptTokensDetails.CachedTokens > 0 {
		cachedInputTokens = u.PromptTokensDetails.CachedTokens
		if cachedInputTokens > inputTokens {
			cachedInputTokens = inputTokens
		}
	}
	if u.CacheReadInputTokens > cachedInputTokens {
		cachedInputTokens = u.CacheReadInputTokens
		if cachedInputTokens > inputTokens {
			cachedInputTokens = inputTokens
		}
	}
	outputTokens = u.CompletionTokens
	if outputTokens <= 0 && u.CompletionTokensDetails != nil {
		outputTokens = u.CompletionTokensDetails.ReasoningTokens + u.CompletionTokensDetails.AudioTokens + u.CompletionTokensDetails.ImageTokens + u.CompletionTokensDetails.VideoTokens
	}
	if u.CompletionTokensDetails != nil && u.CompletionTokensDetails.ReasoningTokens > 0 {
		reasoningTokens = u.CompletionTokensDetails.ReasoningTokens
		if reasoningTokens > outputTokens {
			reasoningTokens = outputTokens
		}
	}
	return
}

type StopSequences []string

func (s *StopSequences) UnmarshalJSON(data []byte) error {
	value := strings.TrimSpace(string(data))
	if value == "" || value == "null" {
		*s = nil
		return nil
	}

	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*s = cleanStrings([]string{single})
		return nil
	}

	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return err
	}
	*s = cleanStrings(many)
	return nil
}

func (r ChatCompletionRequest) validate() error {
	if strings.TrimSpace(r.Model) == "" || len(r.Messages) == 0 {
		return ErrInvalidRequest
	}
	if r.MaxTokens != nil && (*r.MaxTokens <= 0 || *r.MaxTokens > maxRequestedTokens) {
		return ErrInvalidRequest
	}
	if r.MaxCompletionTokens != nil && (*r.MaxCompletionTokens <= 0 || *r.MaxCompletionTokens > maxRequestedTokens) {
		return ErrInvalidRequest
	}
	for _, message := range r.Messages {
		role := normalizeRole(message.Role)
		switch role {
		case "system", "developer", "user", "assistant", "tool":
		default:
			return ErrInvalidRequest
		}
		if role == "tool" && strings.TrimSpace(message.ToolCallID) == "" {
			return ErrInvalidRequest
		}
		if strings.TrimSpace(message.Content) == "" && len(message.ContentParts) == 0 && len(message.ToolCalls) == 0 && len(message.FunctionCall) == 0 {
			return ErrInvalidRequest
		}
	}
	return nil
}

func (r EmbeddingRequest) validate() error {
	if strings.TrimSpace(r.Model) == "" || len(r.Input) == 0 || string(r.Input) == "null" {
		return ErrInvalidRequest
	}
	if r.Dimensions != nil && (*r.Dimensions <= 0 || *r.Dimensions > 32768) {
		return ErrInvalidRequest
	}
	if r.EncodingFormat != "" && r.EncodingFormat != "float" && r.EncodingFormat != "base64" {
		return ErrInvalidRequest
	}
	return nil
}

func (r ChatCompletionRequest) anthropicMaxTokens() int64 {
	if r.MaxCompletionTokens != nil && *r.MaxCompletionTokens > 0 {
		return *r.MaxCompletionTokens
	}
	if r.MaxTokens != nil && *r.MaxTokens > 0 {
		return *r.MaxTokens
	}
	return defaultAnthropicMaxTokens
}

func (r UpstreamChatCompletionRequest) model() string {
	if strings.TrimSpace(r.UpstreamModel) != "" {
		return strings.TrimSpace(r.UpstreamModel)
	}
	return strings.TrimSpace(r.Request.Model)
}

func modelAllowed(principal *auth.Principal, model string) bool {
	if principal == nil || principal.Audience != auth.AudienceRelay {
		return false
	}
	if len(principal.AllowedModels) == 0 {
		return true
	}
	_, ok := principal.AllowedModels[model]
	return ok
}

func principalProjectID(principal *auth.Principal) string {
	if principal == nil || len(principal.ProjectIDs) != 1 {
		return ""
	}
	for projectID := range principal.ProjectIDs {
		return projectID
	}
	return ""
}

func estimateInputTokens(request ChatCompletionRequest) int64 {
	var characters int64
	for _, message := range request.Messages {
		characters += int64(len([]rune(message.Content))) + 4
	}
	if characters < 1 {
		return 1
	}
	// Without a provider tokenizer, use a conservative estimate so a request
	// cannot routinely under-reserve for CJK or other non-ASCII content.
	return characters
}

func estimateEmbeddingTokens(request EmbeddingRequest) int64 {
	var value any
	if json.Unmarshal(request.Input, &value) != nil {
		return 1
	}
	var characters int64
	var add func(any)
	add = func(item any) {
		switch current := item.(type) {
		case string:
			characters += int64(len([]rune(current)))
		case []any:
			for _, nested := range current {
				add(nested)
			}
		}
	}
	add(value)
	if characters < 1 {
		return 1
	}
	return characters
}

func estimateResponseTokens(response ChatCompletionResponse) int64 {
	var characters int64
	for _, choice := range response.Choices {
		characters += int64(len([]rune(choice.Message.Content)))
	}
	return characters
}

func estimateOutputTokens(request ChatCompletionRequest) int64 {
	if request.MaxCompletionTokens != nil {
		return *request.MaxCompletionTokens
	}
	if request.MaxTokens != nil {
		return *request.MaxTokens
	}
	return defaultAnthropicMaxTokens
}

// scopedBillingIdempotencyKey prevents one tenant's user-supplied
// Idempotency-Key from colliding with another tenant or endpoint. The database
// stores only the opaque digest, never the caller-provided key.
func scopedBillingIdempotencyKey(principal *auth.Principal, metadata RequestMetadata, key string) string {
	tokenID := ""
	if principal != nil {
		tokenID = strings.TrimSpace(principal.TokenID)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		key = strings.TrimSpace(metadata.RequestID)
	}
	sum := sha256.Sum256([]byte(tokenID + "\x00" + strings.TrimSpace(metadata.Endpoint) + "\x00" + key))
	return "relay:" + hex.EncodeToString(sum[:])
}

func normalizeRole(role string) string {
	return strings.ToLower(strings.TrimSpace(role))
}

func canonicalProvider(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	switch provider {
	case "xai", "x-ai":
		return ProviderGrok
	case "google", "google-gemini", "google_genai":
		return ProviderGemini
	case "ark", "byteplus", "bytedance", "volc", "volcengine-ark":
		return ProviderVolcengine
	default:
		return provider
	}
}

func supportedProvider(provider string) bool {
	switch canonicalProvider(provider) {
	case ProviderOpenAI, ProviderAnthropic, ProviderGrok, ProviderGemini, ProviderVolcengine:
		return true
	default:
		return false
	}
}

func normalizeChannelStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		return "active"
	}
	return status
}

func validChannelStatus(status string) bool {
	switch status {
	case "active", "draining", "disabled":
		return true
	default:
		return false
	}
}

func validBaseURL(value string) bool {
	return validUpstreamBaseURL(value)
}

func cleanChannelModels(models []ChannelModelMutation) []ChannelModelMutation {
	cleaned := make([]ChannelModelMutation, 0, len(models))
	seen := map[string]struct{}{}
	for _, model := range models {
		model.Model = strings.TrimSpace(model.Model)
		model.UpstreamModel = strings.TrimSpace(model.UpstreamModel)
		if model.Model == "" {
			continue
		}
		if model.UpstreamModel == "" {
			model.UpstreamModel = model.Model
		}
		// Model aliases are the downstream contract. De-duplicate aliases, not
		// providers, so one channel can expose multiple models from the same
		// upstream account.
		key := strings.ToLower(model.Model)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		cleaned = append(cleaned, model)
	}
	return cleaned
}

func channelModelEnabled(model ChannelModelMutation) bool {
	if model.Enabled == nil {
		return true
	}
	return *model.Enabled
}

func rawJSONPresent(raw json.RawMessage) bool {
	value := strings.TrimSpace(string(raw))
	return value != "" && value != "null"
}

func cleanStrings(values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return cleaned
}

func newCompletionID(prefix string) string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return prefix + "unavailable"
	}
	return prefix + hex.EncodeToString(raw[:])
}
