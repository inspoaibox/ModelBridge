package modelprices

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"ai-token/internal/ids"
)

const DefaultSourceURL = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"

var (
	ErrUnavailable = errors.New("official model price sync is unavailable")
	ErrFetchFailed = errors.New("official model price source could not be fetched")
	ErrInvalidData = errors.New("official model price source is invalid")
)

type SyncResult struct {
	SourceURL       string    `json:"source_url"`
	FetchedAt       time.Time `json:"fetched_at"`
	ModelsSeen      int       `json:"models_seen"`
	ModelsMatched   int       `json:"models_matched"`
	ModelsUpdated   int       `json:"models_updated"`
	ModelsUnchanged int       `json:"models_unchanged"`
	Unmatched       []string  `json:"unmatched,omitempty"`
}

type SyncService interface {
	Sync(context.Context) (SyncResult, error)
}

type SQLSyncService struct {
	db        *sql.DB
	client    *http.Client
	sourceURL string
	now       func() time.Time
}

func NewSyncService(db *sql.DB) (*SQLSyncService, error) {
	return NewSyncServiceWithURL(db, DefaultSourceURL)
}

func NewSyncServiceWithURL(db *sql.DB, sourceURL string) (*SQLSyncService, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	if strings.TrimSpace(sourceURL) == "" {
		return nil, errors.New("source URL is required")
	}
	return &SQLSyncService{
		db:        db,
		client:    &http.Client{Timeout: 30 * time.Second},
		sourceURL: strings.TrimSpace(sourceURL),
		now:       time.Now,
	}, nil
}

// sourceRecord keeps the known fields available for old callers/tests while
// raw retains the complete LiteLLM record for new pricing fields and audit.
type sourceRecord struct {
	InputCostPerToken           json.RawMessage `json:"input_cost_per_token"`
	OutputCostPerToken          json.RawMessage `json:"output_cost_per_token"`
	CacheReadInputTokenCost     json.RawMessage `json:"cache_read_input_token_cost"`
	OutputCostPerReasoningToken json.RawMessage `json:"output_cost_per_reasoning_token"`
	LiteLLMProvider             string          `json:"litellm_provider"`
	Mode                        string          `json:"mode"`
	Source                      string          `json:"source"`
	raw                         map[string]json.RawMessage
}

func (record *sourceRecord) UnmarshalJSON(data []byte) error {
	type plain sourceRecord
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	decoded.raw = raw
	*record = sourceRecord(decoded)
	return nil
}

type configuredModel struct {
	ID       string
	Provider string
	Name     string
}

type normalizedComponent struct {
	Code     string
	Unit     string
	Price    string
	Tiers    json.RawMessage
	Metadata json.RawMessage
}

type normalizedPrice struct {
	SourceModelKey string
	Input          string
	Output         string
	CachedInput    string
	Reasoning      string
	Components     []normalizedComponent
	Metadata       map[string]any
	Fingerprint    string
}

type componentField struct {
	Field string
	Code  string
	Unit  string
}

var componentFields = []componentField{
	{"input_cost_per_token", "input_tokens", "token"},
	{"output_cost_per_token", "output_tokens", "token"},
	{"cache_read_input_token_cost", "cached_input_tokens", "token"},
	{"input_cost_per_token_cache_hit", "cached_input_tokens", "token"},
	{"output_cost_per_reasoning_token", "reasoning_tokens", "token"},
	{"citation_cost_per_token", "citation_tokens", "token"},
	{"cache_creation_input_token_cost", "cache_creation_tokens", "token"},
	{"cache_creation_input_token_cost_above_1hr", "cache_creation_1h_tokens", "token"},
	{"cache_creation_input_token_cost_priority", "cache_creation_tokens_priority", "token"},
	{"cache_creation_input_token_cost_flex", "cache_creation_tokens_flex", "token"},
	{"cache_read_input_token_cost_priority", "cached_input_tokens_priority", "token"},
	{"cache_read_input_token_cost_flex", "cached_input_tokens_flex", "token"},
	{"input_cost_per_audio_token", "input_audio_tokens", "audio_token"},
	{"output_cost_per_audio_token", "output_audio_tokens", "audio_token"},
	{"input_cost_per_audio_token_priority", "input_audio_tokens_priority", "audio_token"},
	{"output_cost_per_audio_token_priority", "output_audio_tokens_priority", "audio_token"},
	{"cache_creation_input_audio_token_cost", "cache_creation_audio_tokens", "audio_token"},
	{"cache_read_input_audio_token_cost", "cached_audio_tokens", "audio_token"},
	{"input_cost_per_image_token", "input_image_tokens", "image_token"},
	{"output_cost_per_image_token", "output_image_tokens", "image_token"},
	{"input_cost_per_image", "input_images", "image"},
	{"output_cost_per_image", "output_images", "image"},
	{"input_cost_per_pixel", "input_pixels", "pixel"},
	{"output_cost_per_pixel", "output_pixels", "pixel"},
	{"input_cost_per_character", "input_characters", "character"},
	{"output_cost_per_character", "output_characters", "character"},
	{"input_cost_per_audio_per_second", "input_audio_seconds", "second"},
	{"output_cost_per_audio_per_second", "output_audio_seconds", "second"},
	{"input_cost_per_video_per_second", "input_video_seconds", "second"},
	{"output_cost_per_video_per_second", "output_video_seconds", "second"},
	{"input_cost_per_video_token", "input_video_tokens", "video_token"},
	{"output_cost_per_video_token", "output_video_tokens", "video_token"},
	{"input_cost_per_second", "input_seconds", "second"},
	{"output_cost_per_second", "output_seconds", "second"},
	{"input_cost_per_request", "requests", "request"},
	{"input_cost_per_query", "queries", "query"},
	{"computer_use_input_cost_per_1k_tokens", "computer_use_input_tokens_1k", "1k_tokens"},
	{"computer_use_output_cost_per_1k_tokens", "computer_use_output_tokens_1k", "1k_tokens"},
	{"input_dbu_cost_per_token", "input_dbu_tokens", "dbu_token"},
	{"output_dbu_cost_per_token", "output_dbu_tokens", "dbu_token"},
	{"code_interpreter_cost_per_session", "code_interpreter_sessions", "session"},
	{"file_search_cost_per_1k_calls", "file_search_calls_1k", "1k_calls"},
	{"file_search_cost_per_gb_per_day", "file_search_gb_days", "gb_day"},
	{"vector_store_cost_per_gb_per_day", "vector_store_gb_days", "gb_day"},
	{"google_maps_grounding_cost_per_query", "google_maps_grounding_queries", "query"},
	{"ocr_cost_per_credit", "ocr_credits", "credit"},
	{"ocr_cost_per_page", "ocr_pages", "page"},
	{"annotation_cost_per_page", "annotation_pages", "page"},
	{"input_cost_per_token_priority", "input_tokens_priority", "token"},
	{"output_cost_per_token_priority", "output_tokens_priority", "token"},
	{"input_cost_per_token_flex", "input_tokens_flex", "token"},
	{"output_cost_per_token_flex", "output_tokens_flex", "token"},
	{"input_cost_per_token_batches", "input_tokens_batches", "token"},
	{"output_cost_per_token_batches", "output_tokens_batches", "token"},
	{"output_cost_per_second_480p", "output_seconds_480p", "second"},
	{"output_cost_per_second_1080p", "output_seconds_1080p", "second"},
	{"output_cost_per_second_4k", "output_seconds_4k", "second"},
}

func (s *SQLSyncService) Sync(ctx context.Context) (SyncResult, error) {
	if s == nil || s.db == nil || s.client == nil {
		return SyncResult{}, ErrUnavailable
	}
	response, err := s.client.Get(s.sourceURL)
	if err != nil {
		return SyncResult{}, ErrFetchFailed
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return SyncResult{}, ErrFetchFailed
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil {
		return SyncResult{}, ErrFetchFailed
	}
	var source map[string]sourceRecord
	if err := json.Unmarshal(body, &source); err != nil {
		return SyncResult{}, fmt.Errorf("%w: %v", ErrInvalidData, err)
	}
	configured, err := s.configuredModels(ctx)
	if err != nil {
		return SyncResult{}, err
	}
	fetchedAt := s.now().UTC()
	result := SyncResult{SourceURL: s.sourceURL, FetchedAt: fetchedAt, ModelsSeen: len(configured), Unmatched: make([]string, 0)}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SyncResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	for _, model := range configured {
		price, ok := findPrice(source, model.Provider, model.Name)
		if !ok {
			if len(result.Unmatched) < 100 {
				result.Unmatched = append(result.Unmatched, model.Provider+":"+model.Name)
			}
			continue
		}
		result.ModelsMatched++
		changed, err := upsertPrice(ctx, tx, model.ID, price, s.sourceURL, fetchedAt)
		if err != nil {
			return SyncResult{}, err
		}
		if changed {
			result.ModelsUpdated++
		} else {
			result.ModelsUnchanged++
		}
	}
	if err := tx.Commit(); err != nil {
		return SyncResult{}, err
	}
	return result, nil
}

func (s *SQLSyncService) configuredModels(ctx context.Context) ([]configuredModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT m.id::text, m.provider, m.model_name
		FROM models m
		JOIN channel_models cm ON cm.model_id = m.id AND cm.enabled = true
		JOIN channels c ON c.id = cm.channel_id AND c.deleted_at IS NULL
		WHERE m.status = 'active'
		ORDER BY m.provider, m.model_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]configuredModel, 0)
	for rows.Next() {
		var item configuredModel
		if err := rows.Scan(&item.ID, &item.Provider, &item.Name); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func findPrice(source map[string]sourceRecord, provider, model string) (normalizedPrice, bool) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	model = strings.TrimSpace(model)
	for _, candidate := range sourceModelCandidates(provider, model) {
		for key, record := range source {
			if !strings.EqualFold(strings.TrimSpace(key), candidate) || !sameProvider(provider, record.stringValue("litellm_provider")) {
				continue
			}
			price, ok := normalizeRecord(key, record)
			if ok {
				return price, true
			}
		}
	}
	return normalizedPrice{}, false
}

func sourceModelCandidates(provider, model string) []string {
	items := []string{model}
	for _, alias := range providerAliases(provider) {
		items = append(items, alias+"/"+model)
	}
	return uniqueStrings(items)
}

func providerAliases(provider string) []string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "grok", "xai":
		return []string{"grok", "xai"}
	case "gemini", "google":
		return []string{"gemini", "google", "vertex_ai", "vertex_ai-language-models"}
	default:
		return []string{strings.ToLower(strings.TrimSpace(provider))}
	}
}

func sameProvider(platformProvider, sourceProvider string) bool {
	for _, alias := range providerAliases(platformProvider) {
		if strings.EqualFold(alias, strings.TrimSpace(sourceProvider)) {
			return true
		}
	}
	return false
}

func (record sourceRecord) value(name string) json.RawMessage {
	if record.raw != nil {
		if value := record.raw[name]; len(value) > 0 {
			return value
		}
	}
	switch name {
	case "input_cost_per_token":
		return record.InputCostPerToken
	case "output_cost_per_token":
		return record.OutputCostPerToken
	case "cache_read_input_token_cost":
		return record.CacheReadInputTokenCost
	case "output_cost_per_reasoning_token":
		return record.OutputCostPerReasoningToken
	case "litellm_provider":
		return json.RawMessage(strconv.Quote(record.LiteLLMProvider))
	case "mode":
		return json.RawMessage(strconv.Quote(record.Mode))
	case "source":
		return json.RawMessage(strconv.Quote(record.Source))
	default:
		return nil
	}
}

func (record sourceRecord) stringValue(name string) string {
	var value string
	if raw := record.value(name); len(raw) > 0 && json.Unmarshal(raw, &value) == nil {
		return strings.TrimSpace(value)
	}
	return ""
}

func sourceNumber(raw json.RawMessage) (string, bool) {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		return "", false
	}
	value = strings.Trim(value, `"`)
	return canonicalDecimal(value, 30, 30)
}

func normalizeRecord(key string, record sourceRecord) (normalizedPrice, bool) {
	components := collectComponents(record)
	if len(components) == 0 {
		return normalizedPrice{}, false
	}
	inputPresent := hasNormalizedComponent(components, "input_tokens")
	outputPresent := hasNormalizedComponent(components, "output_tokens")
	if inputPresent != outputPresent && !singleSidedPricingMode(record.stringValue("mode")) {
		return normalizedPrice{}, false
	}
	prices := make(map[string]string, len(components))
	for _, component := range components {
		if _, exists := prices[component.Code]; !exists {
			prices[component.Code] = component.Price
		}
	}
	metadata := map[string]any{
		"litellm_provider": record.stringValue("litellm_provider"),
		"mode":             record.stringValue("mode"),
		"source":           record.stringValue("source"),
		"raw":              record.raw,
	}
	price := normalizedPrice{
		SourceModelKey: key,
		Input:          zeroDecimal(prices["input_tokens"]),
		Output:         zeroDecimal(prices["output_tokens"]),
		CachedInput:    zeroDecimal(prices["cached_input_tokens"]),
		Reasoning:      zeroDecimal(prices["reasoning_tokens"]),
		Components:     components,
		Metadata:       metadata,
	}
	encoded, _ := json.Marshal(components)
	digest := sha256.Sum256(encoded)
	price.Fingerprint = fmt.Sprintf("%x", digest[:])
	metadata["pricing_fingerprint"] = price.Fingerprint
	return price, true
}

func hasNormalizedComponent(components []normalizedComponent, code string) bool {
	for _, component := range components {
		if component.Code == code {
			return true
		}
	}
	return false
}

func singleSidedPricingMode(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "embedding", "embeddings", "image_generation", "video_generation", "audio_transcription", "audio_speech", "moderation":
		return true
	default:
		return false
	}
}

func collectComponents(record sourceRecord) []normalizedComponent {
	components := make(map[string]normalizedComponent)
	for _, field := range componentFields {
		if price, ok := sourceNumber(record.value(field.Field)); ok {
			setComponent(components, normalizedComponent{Code: field.Code, Unit: field.Unit, Price: price})
		}
	}
	setStructuredComponents(record, components)
	setTieredComponents(record, components)
	items := make([]normalizedComponent, 0, len(components))
	for _, component := range components {
		items = append(items, component)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Code < items[j].Code })
	return items
}

func setComponent(components map[string]normalizedComponent, component normalizedComponent) {
	if component.Code == "" || component.Unit == "" || component.Price == "" {
		return
	}
	if existing, exists := components[component.Code]; exists && existing.Price != "0" {
		return
	}
	components[component.Code] = component
}

func setStructuredComponents(record sourceRecord, components map[string]normalizedComponent) {
	var search map[string]json.RawMessage
	if raw := record.value("search_context_cost_per_query"); len(raw) > 0 && json.Unmarshal(raw, &search) == nil {
		for size, rawPrice := range search {
			if price, ok := sourceNumber(rawPrice); ok {
				part := strings.TrimPrefix(strings.ToLower(size), "search_context_size_")
				setComponent(components, normalizedComponent{Code: "search_context_" + componentPart(part), Unit: "query", Price: price})
			}
		}
	}
	var guardrails map[string]json.RawMessage
	if raw := record.value("guardrail_cost_per_unit"); len(raw) > 0 && json.Unmarshal(raw, &guardrails) == nil {
		for name, rawPrice := range guardrails {
			if price, ok := sourceNumber(rawPrice); ok {
				setComponent(components, normalizedComponent{Code: "guardrail_" + componentPart(name) + "_units", Unit: "unit", Price: price})
			}
		}
	}
}

type priceTier struct {
	Threshold    int64
	PricePerUnit string
}

func setTieredComponents(record sourceRecord, components map[string]normalizedComponent) {
	for _, field := range componentFields {
		if record.raw == nil {
			continue
		}
		base, baseExists := components[field.Code]
		ranges := tieredPricingTiers(record, field.Field)
		if !baseExists && len(ranges) > 0 && ranges[0].Threshold == 0 {
			base = normalizedComponent{Code: field.Code, Unit: field.Unit, Price: ranges[0].PricePerUnit}
			components[field.Code] = base
			baseExists = true
		}
		if !baseExists {
			continue
		}
		boundaries := map[string][]priceTier{}
		tierBasis := map[string]string{}
		tierMode := map[string]string{}
		if len(ranges) > 0 {
			boundaries[field.Code] = append(boundaries[field.Code], ranges...)
			tierBasis[field.Code] = "input_tokens"
			tierMode[field.Code] = "context"
		}
		prefix := field.Field + "_above_"
		for name, rawPrice := range record.raw {
			if !strings.HasPrefix(name, prefix) {
				continue
			}
			rest := strings.TrimPrefix(name, prefix)
			pricingTier, rest := splitPricingTier(rest)
			targetCode := field.Code
			if strings.HasPrefix(rest, "1hr_above_") || strings.HasPrefix(rest, "1hour_above_") {
				if field.Code != "cache_creation_1h_tokens" {
					continue
				}
				targetCode = "cache_creation_1h_tokens"
				rest = strings.TrimPrefix(strings.TrimPrefix(rest, "1hr_above_"), "1hour_above_")
			}
			threshold, ok := thresholdFromSuffix(rest)
			if !ok {
				continue
			}
			price, ok := sourceNumber(rawPrice)
			if ok {
				if pricingTier != "" {
					targetCode += "_" + pricingTier
				}
				boundaries[targetCode] = append(boundaries[targetCode], priceTier{Threshold: threshold, PricePerUnit: price})
				if strings.Contains(rest, "interval") {
					tierBasis[targetCode] = targetCode
					tierMode[targetCode] = "quantity"
				} else {
					tierBasis[targetCode] = "input_tokens"
					tierMode[targetCode] = "context"
				}
			}
		}
		for targetCode, tiers := range boundaries {
			target := base
			if existing, ok := components[targetCode]; ok {
				target = existing
			}
			target.Code = targetCode
			if tierBasis[targetCode] != "" {
				target.Metadata = mergeTierMetadata(target.Metadata, tierBasis[targetCode], tierMode[targetCode])
			}
			encoded, basePrice := encodeTieredComponents(target.Price, tiers)
			target.Tiers = encoded
			target.Price = basePrice
			components[targetCode] = target
		}
	}
}

func mergeTierMetadata(raw json.RawMessage, basis, mode string) json.RawMessage {
	value := map[string]any{}
	if len(raw) > 0 && string(raw) != "null" {
		_ = json.Unmarshal(raw, &value)
	}
	value["tier_basis"] = basis
	value["tier_mode"] = mode
	encoded, _ := json.Marshal(value)
	return encoded
}

func splitPricingTier(value string) (string, string) {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, tier := range []string{"priority", "flex", "batches"} {
		if strings.HasSuffix(value, "_"+tier) {
			return tier, strings.TrimSuffix(value, "_"+tier)
		}
	}
	return "", value
}

func appendTier(code string, component normalizedComponent, components map[string]normalizedComponent, tier priceTier) {
	existing := component
	if current, ok := components[code]; ok {
		existing = current
	}
	var tiers []priceTier
	if len(existing.Tiers) > 0 {
		_ = json.Unmarshal(existing.Tiers, &tiers)
	}
	tiers = append(tiers, tier)
	sort.Slice(tiers, func(i, j int) bool { return tiers[i].Threshold < tiers[j].Threshold })
	unique := tiers[:0]
	for _, item := range tiers {
		if len(unique) > 0 && unique[len(unique)-1].Threshold == item.Threshold {
			unique[len(unique)-1] = item
			continue
		}
		unique = append(unique, item)
	}
	encoded := make([]map[string]string, 0, len(unique)+1)
	for index, item := range unique {
		if index == 0 && item.Threshold > 0 {
			encoded = append(encoded, map[string]string{"up_to": strconv.FormatInt(item.Threshold, 10), "price_per_unit": existing.Price})
		}
		entry := map[string]string{"price_per_unit": item.PricePerUnit}
		if index+1 < len(unique) {
			entry["up_to"] = strconv.FormatInt(unique[index+1].Threshold, 10)
		}
		encoded = append(encoded, entry)
	}
	existing.Tiers, _ = json.Marshal(encoded)
	components[code] = existing
}

func encodeTieredComponents(basePrice string, boundaries []priceTier) (json.RawMessage, string) {
	sort.Slice(boundaries, func(i, j int) bool { return boundaries[i].Threshold < boundaries[j].Threshold })
	unique := make([]priceTier, 0, len(boundaries))
	for _, item := range boundaries {
		if len(unique) > 0 && unique[len(unique)-1].Threshold == item.Threshold {
			unique[len(unique)-1] = item
			continue
		}
		unique = append(unique, item)
	}
	if len(unique) == 0 {
		return []byte(`[]`), basePrice
	}
	if unique[0].Threshold == 0 {
		basePrice = unique[0].PricePerUnit
	}
	encoded := make([]map[string]string, 0, len(unique)+1)
	if unique[0].Threshold > 0 {
		encoded = append(encoded, map[string]string{"up_to": strconv.FormatInt(unique[0].Threshold, 10), "price_per_unit": basePrice})
	}
	for index, item := range unique {
		price := item.PricePerUnit
		if item.Threshold == 0 {
			price = basePrice
		}
		entry := map[string]string{"price_per_unit": price}
		if index+1 < len(unique) {
			entry["up_to"] = strconv.FormatInt(unique[index+1].Threshold, 10)
		}
		encoded = append(encoded, entry)
	}
	return jsonValue(encoded), basePrice
}

func tieredPricingTiers(record sourceRecord, field string) []priceTier {
	var entries []map[string]json.RawMessage
	if raw := record.value("tiered_pricing"); len(raw) == 0 || json.Unmarshal(raw, &entries) != nil {
		return nil
	}
	tiers := make([]priceTier, 0, len(entries))
	for _, entry := range entries {
		price, ok := sourceNumber(entry[field])
		if !ok {
			continue
		}
		var bounds []json.RawMessage
		if json.Unmarshal(entry["range"], &bounds) != nil || len(bounds) < 2 {
			continue
		}
		lower, ok := sourceNumber(bounds[0])
		if !ok {
			continue
		}
		threshold, err := strconv.ParseInt(lower, 10, 64)
		if err != nil || threshold < 0 {
			continue
		}
		tiers = append(tiers, priceTier{Threshold: threshold, PricePerUnit: price})
	}
	return tiers
}

func thresholdFromSuffix(value string) (int64, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if strings.HasSuffix(value, "hr") || strings.HasSuffix(value, "hour") {
		return 0, false
	}
	for _, suffix := range []string{"_priority", "_flex", "_batches"} {
		value = strings.TrimSuffix(value, suffix)
	}
	value = strings.TrimSuffix(value, "_interval")
	value = strings.TrimSuffix(strings.TrimSuffix(value, "_tokens"), "_token")
	multiplier := int64(1)
	if strings.HasSuffix(value, "s") && len(value) > 1 {
		value = strings.TrimSuffix(value, "s")
	} else if strings.HasSuffix(value, "k") {
		multiplier, value = 1000, strings.TrimSuffix(value, "k")
	} else if strings.HasSuffix(value, "m") {
		multiplier, value = 1000000, strings.TrimSuffix(value, "m")
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return 0, false
	}
	return parsed * multiplier, true
}

func componentPart(value string) string {
	var output strings.Builder
	underscore := false
	for _, runeValue := range strings.ToLower(value) {
		if unicode.IsLetter(runeValue) || unicode.IsDigit(runeValue) {
			output.WriteRune(runeValue)
			underscore = false
		} else if !underscore {
			output.WriteByte('_')
			underscore = true
		}
	}
	return strings.Trim(output.String(), "_")
}

func canonicalDecimal(value string, maxIntegerDigits, maxFractionDigits int) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "-") || strings.HasPrefix(value, "+") {
		return "", false
	}
	base, exponentText, hasExponent := value, "", false
	if position := strings.IndexAny(value, "eE"); position >= 0 {
		if strings.Count(value, "e")+strings.Count(value, "E") != 1 {
			return "", false
		}
		base, exponentText, hasExponent = value[:position], value[position+1:], true
	}
	parts := strings.Split(base, ".")
	if base == "" || len(parts) > 2 || parts[0] == "" {
		return "", false
	}
	for _, part := range parts {
		for _, runeValue := range part {
			if !unicode.IsDigit(runeValue) {
				return "", false
			}
		}
	}
	exponent := 0
	if hasExponent {
		parsed, err := strconv.Atoi(exponentText)
		if err != nil || parsed < -100 || parsed > 100 {
			return "", false
		}
		exponent = parsed
	}
	fractionDigits := 0
	if len(parts) == 2 {
		fractionDigits = len(parts[1])
	}
	digits := strings.TrimLeft(strings.Join(parts, ""), "0")
	if digits == "" {
		return "0", true
	}
	scale := fractionDigits - exponent
	result := ""
	if scale <= 0 {
		result = digits + strings.Repeat("0", -scale)
	} else if len(digits) > scale {
		result = digits[:len(digits)-scale] + "." + digits[len(digits)-scale:]
	} else {
		result = "0." + strings.Repeat("0", scale-len(digits)) + digits
	}
	if strings.Contains(result, ".") {
		result = strings.TrimRight(strings.TrimRight(result, "0"), ".")
	}
	if result == "" {
		result = "0"
	}
	integerPart, fractionPart := result, ""
	if point := strings.IndexByte(result, '.'); point >= 0 {
		integerPart, fractionPart = result[:point], result[point+1:]
	}
	integerPart = strings.TrimLeft(integerPart, "0")
	if integerPart == "" {
		integerPart = "0"
	}
	if len(integerPart) > maxIntegerDigits || len(fractionPart) > maxFractionDigits {
		return "", false
	}
	return result, true
}

func zeroDecimal(value string) string {
	if strings.TrimSpace(value) == "" {
		return "0"
	}
	return value
}

func upsertPrice(ctx context.Context, tx *sql.Tx, modelID string, price normalizedPrice, sourceURL string, fetchedAt time.Time) (bool, error) {
	var currentID, currentFingerprint string
	err := tx.QueryRowContext(ctx, `
		SELECT id::text, COALESCE(metadata_json ->> 'pricing_fingerprint', '')
		FROM official_model_price_versions
		WHERE model_id = $1 AND source = 'litellm' AND effective_to IS NULL
		LIMIT 1
	`, modelID).Scan(&currentID, &currentFingerprint)
	if err == nil && currentFingerprint == price.Fingerprint {
		_, err = tx.ExecContext(ctx, `
			UPDATE official_model_price_versions
			SET fetched_at = $2, source_url = $3, source_model_key = $4, metadata_json = $5::jsonb
			WHERE id = $1
		`, currentID, fetchedAt, sourceURL, price.SourceModelKey, jsonValue(price.Metadata))
		return false, err
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if currentID != "" {
		if _, err := tx.ExecContext(ctx, `UPDATE official_model_price_versions SET effective_to = $2 WHERE id = $1`, currentID, fetchedAt); err != nil {
			return false, err
		}
	}
	priceID, err := ids.New()
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO official_model_price_versions (
			id, model_id, source, source_url, source_model_key, currency,
			input_price_per_unit, output_price_per_unit,
			cached_input_price_per_unit, reasoning_price_per_unit,
			effective_from, fetched_at, metadata_json
		) VALUES ($1, $2, 'litellm', $3, $4, 'USD', $5::numeric, $6::numeric, $7::numeric, $8::numeric, $9, $9, $10::jsonb)
	`, priceID, modelID, sourceURL, price.SourceModelKey, price.Input, price.Output, price.CachedInput, price.Reasoning, fetchedAt, jsonValue(price.Metadata)); err != nil {
		return false, err
	}
	for _, component := range price.Components {
		componentID, err := ids.New()
		if err != nil {
			return false, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO official_price_components (
				id, official_price_version_id, component_code, unit, price_per_unit, tier_json, metadata_json
			) VALUES ($1, $2, $3, $4, $5::numeric, $6::jsonb, $7::jsonb)
		`, componentID, priceID, component.Code, component.Unit, component.Price, nullableJSON(component.Tiers, []byte(`[]`)), nullableJSON(component.Metadata, []byte(`{}`))); err != nil {
			return false, err
		}
	}
	return true, nil
}

func nullableJSON(value, fallback []byte) []byte {
	if len(value) == 0 || string(value) == "null" {
		return fallback
	}
	return value
}

func jsonValue(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		return []byte(`{}`)
	}
	return encoded
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}
