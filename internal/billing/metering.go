package billing

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
)

// MeteredUsage keeps quantities as decimal strings so fractional units such as
// audio seconds and storage GB-days never pass through a floating point value.
type MeteredUsage map[string]string

type PriceTier struct {
	UpTo         string `json:"up_to,omitempty"`
	PricePerUnit string `json:"price_per_unit"`
}

type PriceComponent struct {
	ComponentCode string          `json:"component_code"`
	Unit          string          `json:"unit"`
	PricePerUnit  string          `json:"price_per_unit"`
	Tiers         json.RawMessage `json:"tiers,omitempty"`
	Metadata      json.RawMessage `json:"metadata,omitempty"`
}

type ChargeLine struct {
	ComponentCode string `json:"component_code"`
	Unit          string `json:"unit"`
	Quantity      string `json:"quantity"`
	PricePerUnit  string `json:"price_per_unit"`
	Amount        string `json:"amount"`
}

type MeteredCharge struct {
	Amount string       `json:"amount"`
	Lines  []ChargeLine `json:"lines"`
}

type PriceComponentInput struct {
	ComponentCode string          `json:"component_code"`
	Unit          string          `json:"unit"`
	PricePerUnit  string          `json:"price_per_unit"`
	Tiers         json.RawMessage `json:"tiers,omitempty"`
	Metadata      json.RawMessage `json:"metadata,omitempty"`
}

func normalizeMeteringMode(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return MeteringToken, nil
	}
	switch value {
	case MeteringToken, MeteringImageCount, MeteringVideoSeconds, MeteringVideoRequest:
		return value, nil
	default:
		return "", ErrInvalidRequest
	}
}

// customerMeteringUsage projects the provider usage onto the unit selected by
// the routing group. The original usage remains available for upstream cost
// estimation and usage reporting; this projection is only for the customer
// charge.
func customerMeteringUsage(usage MeteredUsage, mode string) (MeteredUsage, error) {
	normalizedMode, err := normalizeMeteringMode(mode)
	if err != nil {
		return nil, err
	}
	normalized, err := normalizeMeteredUsage(usage)
	if err != nil {
		return nil, err
	}
	switch normalizedMode {
	case MeteringToken:
		return normalized, nil
	case MeteringImageCount:
		if value := firstPositiveMetric(normalized, "output_images"); value != "" {
			return MeteredUsage{"output_images": value}, nil
		}
		return MeteredUsage{}, nil
	case MeteringVideoSeconds:
		if value := firstPositiveMetric(normalized,
			"output_seconds", "output_video_seconds",
			"output_seconds_480p", "output_seconds_1080p", "output_seconds_4k",
		); value != "" {
			return MeteredUsage{"output_seconds": value}, nil
		}
		return MeteredUsage{}, nil
	case MeteringVideoRequest:
		return MeteredUsage{"requests": "1"}, nil
	default:
		return nil, ErrInvalidRequest
	}
}

// customerMeteringUsageWithEstimate uses a request estimate only for media
// meters that providers commonly omit from their final response. Token
// billing deliberately remains dependent on provider-reported token usage.
func customerMeteringUsageWithEstimate(actual, estimate MeteredUsage, mode string) (MeteredUsage, error) {
	normalizedMode, err := normalizeMeteringMode(mode)
	if err != nil {
		return nil, err
	}
	customer, err := customerMeteringUsage(actual, normalizedMode)
	if err != nil {
		return nil, err
	}
	if normalizedMode == MeteringToken || meteredUsageHasPositiveQuantity(customer) {
		return customer, nil
	}
	fallback, err := customerMeteringUsage(estimate, normalizedMode)
	if err != nil {
		return nil, err
	}
	if meteredUsageHasPositiveQuantity(fallback) {
		return fallback, nil
	}
	return customer, nil
}

func firstPositiveMetric(usage MeteredUsage, codes ...string) string {
	for _, code := range codes {
		value := strings.TrimSpace(usage[code])
		if value != "" && rationalValue(value).Sign() > 0 {
			return ratDecimal(rationalValue(value))
		}
	}
	return ""
}

var validComponentCodes = map[string]struct{}{
	"input_tokens": {}, "output_tokens": {}, "cached_input_tokens": {}, "reasoning_tokens": {},
	"input_images": {}, "output_images": {}, "input_audio_seconds": {}, "output_audio_seconds": {},
	"input_audio_tokens": {}, "output_audio_tokens": {}, "input_video_seconds": {}, "output_video_seconds": {},
	"input_video_tokens": {}, "output_video_tokens": {}, "input_pixels": {}, "output_pixels": {},
	"input_characters": {}, "output_characters": {}, "requests": {}, "queries": {}, "sessions": {},
	"pages": {}, "storage_gb_days": {}, "input_seconds": {}, "output_seconds": {},
	"code_interpreter_sessions": {}, "file_search_gb_days": {}, "vector_store_gb_days": {},
	"google_maps_grounding_queries": {}, "annotation_pages": {}, "output_seconds_480p": {},
	"output_seconds_1080p": {}, "output_seconds_4k": {}, "input_dbu_tokens": {}, "output_dbu_tokens": {},
	"input_image_tokens": {}, "output_image_tokens": {}, "cache_creation_tokens": {}, "cache_creation_1h_tokens": {}, "cache_creation_audio_tokens": {},
	"cached_audio_tokens": {}, "cached_image_tokens": {}, "cached_video_tokens": {}, "citation_tokens": {}, "file_search_calls_1k": {},
	"cache_creation_tokens_priority": {}, "cache_creation_tokens_flex": {}, "cached_input_tokens_priority": {}, "cached_input_tokens_flex": {},
	"input_audio_tokens_priority": {}, "output_audio_tokens_priority": {}, "computer_use_input_tokens_1k": {}, "computer_use_output_tokens_1k": {},
	"grounding_queries": {}, "search_context_queries": {}, "search_context_low": {}, "search_context_medium": {},
	"search_context_high": {}, "guardrail_units": {}, "guardrail_automated_reasoning_policy_units": {},
	"guardrail_content_policy_image_units": {}, "guardrail_content_policy_units": {},
	"guardrail_contextual_grounding_policy_units": {}, "guardrail_sensitive_information_policy_units": {},
	"guardrail_topic_policy_units": {}, "guardrail_word_policy_units": {}, "ocr_credits": {}, "ocr_pages": {},
	"input_tokens_priority": {}, "output_tokens_priority": {}, "input_tokens_flex": {}, "output_tokens_flex": {},
	"input_tokens_batches": {}, "output_tokens_batches": {},
}

func normalizeMeteredUsage(usage MeteredUsage) (MeteredUsage, error) {
	normalized := make(MeteredUsage, len(usage)+4)
	for code, value := range usage {
		code = strings.TrimSpace(code)
		value = strings.TrimSpace(value)
		if code == "" || value == "" {
			continue
		}
		if !validComponentCode(code) {
			return nil, errors.New("unsupported usage component: " + code)
		}
		if !validMeteredDecimal(value) {
			return nil, errors.New("invalid usage quantity for " + code)
		}
		normalized[code] = value
	}
	normalizeMeteredRelationships(normalized)
	return normalized, nil
}

// Provider usage APIs expose parent totals together with detail subsets. Keep
// the subsets disjoint before pricing so malformed or adapter-generated usage
// cannot make one token count multiple times. This is a normalization step,
// not a price fallback: an absent component still fails settlement later.
func normalizeMeteredRelationships(usage MeteredUsage) {
	ensureParentMetric(usage, "input_audio_tokens", []string{"cache_creation_audio_tokens", "cached_audio_tokens"})
	ensureParentMetric(usage, "input_image_tokens", []string{"cached_image_tokens"})
	ensureParentMetric(usage, "input_video_tokens", []string{"cached_video_tokens"})
	ensureParentMetric(usage, "input_tokens", []string{
		"cached_input_tokens", "cache_creation_tokens", "cache_creation_1h_tokens",
		"input_audio_tokens", "input_image_tokens", "input_video_tokens",
	})
	ensureParentMetric(usage, "output_tokens", []string{
		"reasoning_tokens", "output_audio_tokens", "output_image_tokens", "output_video_tokens",
	})
	limitChildrenToParent(usage, "input_tokens", []string{
		"cached_input_tokens", "cache_creation_tokens", "cache_creation_1h_tokens",
		"input_audio_tokens", "input_image_tokens", "input_video_tokens",
	})
	// A modality can itself have cached children. Re-apply the nested clamp
	// after the parent clamp in case the parent total was smaller than the
	// sum of generic and modality subsets.
	limitChildrenToParent(usage, "input_audio_tokens", []string{"cache_creation_audio_tokens", "cached_audio_tokens"})
	limitChildrenToParent(usage, "input_image_tokens", []string{"cached_image_tokens"})
	limitChildrenToParent(usage, "input_video_tokens", []string{"cached_video_tokens"})
	limitChildrenToParent(usage, "output_tokens", []string{
		"reasoning_tokens", "output_audio_tokens", "output_image_tokens", "output_video_tokens",
	})
}

func ensureParentMetric(usage MeteredUsage, parentCode string, childCodes []string) {
	if _, exists := usage[parentCode]; exists {
		return
	}
	total := new(big.Rat)
	for _, code := range childCodes {
		total.Add(total, rationalValue(usage[code]))
	}
	if total.Sign() > 0 {
		usage[parentCode] = ratDecimal(total)
	}
}

func limitChildrenToParent(usage MeteredUsage, parentCode string, childCodes []string) {
	parent := rationalValue(usage[parentCode])
	remaining := new(big.Rat).Set(parent)
	for _, code := range childCodes {
		value := rationalValue(usage[code])
		if value.Cmp(remaining) > 0 {
			value.Set(remaining)
		}
		if _, exists := usage[code]; exists {
			usage[code] = ratDecimal(value)
		}
		remaining.Sub(remaining, value)
	}
}

// Component codes are data identifiers, not a closed list. The explicit
// built-in map documents supported meters while this guard allows a newly
// introduced provider meter to be stored and priced without a code release.
func validComponentCode(code string) bool {
	if _, ok := validComponentCodes[code]; ok {
		return true
	}
	if len(code) == 0 || len(code) > 128 || code[0] < 'a' || code[0] > 'z' {
		return false
	}
	for index := 1; index < len(code); index++ {
		value := code[index]
		if (value < 'a' || value > 'z') && (value < '0' || value > '9') && value != '_' {
			return false
		}
	}
	return true
}

// validMeteredDecimal accepts large fractional quantities without routing them
// through floats. Usage stays in JSON rather than a fixed-scale SQL column,
// so it may safely be wider than legacy monetary inputs.
func validMeteredDecimal(value string) bool {
	_, ok := canonicalDecimal(value, 30, 30)
	return ok
}

func usageFromLegacy(input, output, cached, reasoning int64) MeteredUsage {
	return MeteredUsage{
		"input_tokens":        integerString(input),
		"output_tokens":       integerString(output),
		"cached_input_tokens": integerString(cached),
		"reasoning_tokens":    integerString(reasoning),
	}
}

func integerString(value int64) string {
	return new(big.Int).SetInt64(value).String()
}

func legacyPriceComponents(price Price) []PriceComponent {
	components := make([]PriceComponent, 0, 4)
	for _, item := range []struct {
		code, unit, value string
		fallback          bool
	}{
		{"input_tokens", "token", price.InputPricePerUnit, false},
		{"output_tokens", "token", price.OutputPricePerUnit, false},
		{"cached_input_tokens", "token", price.CachedInputPricePerUnit, true},
		{"reasoning_tokens", "token", price.ReasoningPricePerUnit, true},
	} {
		if strings.TrimSpace(item.value) != "" && (!item.fallback || !isZeroAmount(item.value)) {
			components = append(components, PriceComponent{ComponentCode: item.code, Unit: item.unit, PricePerUnit: item.value})
		}
	}
	return components
}

func calculateMeteredCharge(components []PriceComponent, usage MeteredUsage, minimumCharge string) (MeteredCharge, error) {
	return calculateMeteredChargeForTier(components, usage, minimumCharge, "")
}

// calculateMeteredChargeForTier applies an explicitly selected service tier
// (priority, flex, or batch) when a matching component is published. Any
// non-zero upstream meter must resolve to a published component or to a
// documented parent meter; otherwise settlement fails instead of treating it
// as a zero-cost request.
func calculateMeteredChargeForTier(components []PriceComponent, usage MeteredUsage, minimumCharge, pricingTier string) (MeteredCharge, error) {
	usage = addImplicitRequestMetric(components, usage, pricingTier)
	normalized, err := normalizeMeteredUsage(usage)
	if err != nil {
		return MeteredCharge{}, err
	}
	total := new(big.Rat)
	selected, err := selectMeteredComponents(components, normalized, pricingTier)
	if err != nil {
		return MeteredCharge{}, err
	}
	lines := make([]ChargeLine, 0, len(selected))
	sorted := append([]PriceComponent(nil), selected...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ComponentCode < sorted[j].ComponentCode })
	for _, component := range sorted {
		quantity := effectiveQuantity(componentBaseCode(component.ComponentCode), normalized)
		if quantity == "" {
			quantity = "0"
		}
		qty, ok := new(big.Rat).SetString(quantity)
		if !ok || qty.Sign() < 0 {
			return MeteredCharge{}, errors.New("invalid quantity for " + component.ComponentCode)
		}
		lineAmount, effectivePrice, err := tieredAmount(component, qty, normalized)
		if err != nil {
			return MeteredCharge{}, err
		}
		total.Add(total, lineAmount)
		if qty.Sign() > 0 {
			lines = append(lines, ChargeLine{ComponentCode: component.ComponentCode, Unit: component.Unit, Quantity: quantity, PricePerUnit: effectivePrice, Amount: ratDecimal(lineAmount)})
		}
	}
	minimum, ok := new(big.Rat).SetString(strings.TrimSpace(minimumCharge))
	if !ok || minimum.Sign() < 0 {
		return MeteredCharge{}, errors.New("invalid minimum charge")
	}
	if total.Cmp(minimum) < 0 {
		total = minimum
	}
	return MeteredCharge{Amount: ratDecimal(total), Lines: lines}, nil
}

var pricingTierSuffixes = []string{"priority", "flex", "batches"}

func normalizePricingTier(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "batch" {
		return "batches"
	}
	for _, suffix := range pricingTierSuffixes {
		if value == suffix {
			return value
		}
	}
	return ""
}

func componentBaseCode(code string) string {
	for _, suffix := range pricingTierSuffixes {
		code = strings.TrimSuffix(code, "_"+suffix)
	}
	return code
}

func addImplicitRequestMetric(components []PriceComponent, usage MeteredUsage, pricingTier string) MeteredUsage {
	if usage == nil {
		usage = MeteredUsage{}
	}
	if value, exists := usage["requests"]; exists && !isZeroAmount(value) {
		return usage
	}
	if !hasComponentForTier(components, "requests", pricingTier) {
		return usage
	}
	copy := make(MeteredUsage, len(usage)+1)
	for key, value := range usage {
		copy[key] = value
	}
	copy["requests"] = "1"
	return copy
}

func hasComponentForTier(components []PriceComponent, baseCode, pricingTier string) bool {
	tier := normalizePricingTier(pricingTier)
	for _, component := range components {
		if component.ComponentCode == baseCode || (tier != "" && component.ComponentCode == baseCode+"_"+tier) {
			return true
		}
	}
	return false
}

func selectMeteredComponents(components []PriceComponent, usage MeteredUsage, pricingTier string) ([]PriceComponent, error) {
	byCode := make(map[string]PriceComponent, len(components))
	for _, component := range components {
		code := strings.TrimSpace(component.ComponentCode)
		if !validComponentCode(code) || strings.TrimSpace(component.Unit) == "" {
			return nil, errors.New("invalid price component: " + code)
		}
		if _, exists := byCode[code]; exists {
			return nil, errors.New("duplicate price component: " + code)
		}
		byCode[code] = component
	}

	baseCodes := make(map[string]struct{}, len(usage))
	for code := range usage {
		baseCode := componentBaseCode(code)
		if rationalValue(effectiveQuantity(baseCode, usage)).Sign() > 0 {
			baseCodes[baseCode] = struct{}{}
		}
	}
	selected := make([]PriceComponent, 0, len(baseCodes))
	for baseCode := range baseCodes {
		component, ok := componentForMeter(byCode, baseCode, pricingTier)
		if !ok {
			return nil, fmt.Errorf("%w: usage component %s", ErrPriceNotConfigured, baseCode)
		}
		// Make the billing loop consistently look up the physical meter rather
		// than the tier-specific price key.
		component.ComponentCode = baseCode
		selected = append(selected, component)
	}
	return selected, nil
}

func componentForMeter(components map[string]PriceComponent, code, pricingTier string) (PriceComponent, bool) {
	tier := normalizePricingTier(pricingTier)
	if tier != "" {
		if component, ok := components[code+"_"+tier]; ok {
			return component, true
		}
	}
	if component, ok := components[code]; ok {
		return component, true
	}

	// These price families are explicit provider sub-meters. They can safely
	// inherit a parent price because their quantities are removed from the
	// parent meter below, so they are never double billed or silently free.
	var parent string
	switch code {
	case "cached_input_tokens", "cache_creation_tokens", "cached_input_tokens_priority", "cached_input_tokens_flex", "cache_creation_tokens_priority", "cache_creation_tokens_flex":
		parent = "input_tokens"
	case "cache_creation_1h_tokens":
		parent = "cache_creation_tokens"
	case "reasoning_tokens":
		parent = "output_tokens"
	case "input_audio_tokens", "input_image_tokens", "input_video_tokens":
		parent = "input_tokens"
	case "output_audio_tokens", "output_image_tokens", "output_video_tokens":
		parent = "output_tokens"
	case "cache_creation_audio_tokens", "cached_audio_tokens":
		parent = "input_audio_tokens"
	case "cached_image_tokens":
		parent = "input_image_tokens"
	case "cached_video_tokens":
		parent = "input_video_tokens"
	}
	if parent == "" || parent == code {
		return PriceComponent{}, false
	}
	return componentForMeter(components, parent, pricingTier)
}

func isFallbackComponent(code string) bool {
	switch code {
	case "cached_input_tokens", "cache_creation_tokens", "cache_creation_1h_tokens",
		"reasoning_tokens", "cached_input_tokens_priority", "cached_input_tokens_flex",
		"cache_creation_tokens_priority", "cache_creation_tokens_flex":
		return true
	default:
		return false
	}
}

func effectiveQuantity(componentCode string, usage MeteredUsage) string {
	switch componentCode {
	case "input_tokens":
		total := rationalValue(usage["input_tokens"])
		cached := rationalValue(usage["cached_input_tokens"])
		cacheCreation := rationalValue(usage["cache_creation_tokens"])
		cacheCreation1h := rationalValue(usage["cache_creation_1h_tokens"])
		inputAudio := rationalValue(usage["input_audio_tokens"])
		inputImage := rationalValue(usage["input_image_tokens"])
		inputVideo := rationalValue(usage["input_video_tokens"])
		total.Sub(total, cached)
		total.Sub(total, cacheCreation)
		total.Sub(total, cacheCreation1h)
		// The parent prompt total includes every modality. Cached media is
		// already excluded from the generic cached token meter below.
		total.Sub(total, inputAudio)
		total.Sub(total, inputImage)
		total.Sub(total, inputVideo)
		if total.Sign() < 0 {
			return "0"
		}
		return ratDecimal(total)
	case "cached_input_tokens":
		return strings.TrimSpace(usage["cached_input_tokens"])
	case "output_tokens":
		total := rationalValue(usage["output_tokens"])
		reasoning := rationalValue(usage["reasoning_tokens"])
		outputAudio := rationalValue(usage["output_audio_tokens"])
		outputImage := rationalValue(usage["output_image_tokens"])
		outputVideo := rationalValue(usage["output_video_tokens"])
		total.Sub(total, reasoning)
		total.Sub(total, outputAudio)
		total.Sub(total, outputImage)
		total.Sub(total, outputVideo)
		if total.Sign() < 0 {
			return "0"
		}
		return ratDecimal(total)
	case "input_audio_tokens":
		total := rationalValue(usage["input_audio_tokens"])
		total.Sub(total, rationalValue(usage["cache_creation_audio_tokens"]))
		total.Sub(total, rationalValue(usage["cached_audio_tokens"]))
		if total.Sign() < 0 {
			return "0"
		}
		return ratDecimal(total)
	case "input_image_tokens":
		total := rationalValue(usage["input_image_tokens"])
		total.Sub(total, rationalValue(usage["cached_image_tokens"]))
		if total.Sign() < 0 {
			return "0"
		}
		return ratDecimal(total)
	case "input_video_tokens":
		total := rationalValue(usage["input_video_tokens"])
		total.Sub(total, rationalValue(usage["cached_video_tokens"]))
		if total.Sign() < 0 {
			return "0"
		}
		return ratDecimal(total)
	default:
		value := strings.TrimSpace(usage[componentCode])
		if value == "" {
			return "0"
		}
		return value
	}
}

func rationalValue(value string) *big.Rat {
	if parsed, ok := new(big.Rat).SetString(strings.TrimSpace(value)); ok && parsed.Sign() >= 0 {
		return parsed
	}
	return new(big.Rat)
}

func tieredAmount(component PriceComponent, quantity *big.Rat, usage MeteredUsage) (*big.Rat, string, error) {
	base, ok := new(big.Rat).SetString(strings.TrimSpace(component.PricePerUnit))
	if !ok || base.Sign() < 0 {
		return nil, "", errors.New("invalid price for " + component.ComponentCode)
	}
	var tiers []PriceTier
	if len(component.Tiers) > 0 && string(component.Tiers) != "null" && string(component.Tiers) != "[]" {
		if err := json.Unmarshal(component.Tiers, &tiers); err != nil {
			return nil, "", errors.New("invalid pricing tiers for " + component.ComponentCode)
		}
	}
	if len(tiers) == 0 {
		return new(big.Rat).Mul(base, quantity), component.PricePerUnit, nil
	}
	if basis, mode := tierMetadata(component.Metadata); mode == "context" {
		contextQuantity := rationalValue(usage[basis])
		for _, tier := range tiers {
			price, valid := new(big.Rat).SetString(strings.TrimSpace(tier.PricePerUnit))
			if !valid || price.Sign() < 0 {
				return nil, "", errors.New("invalid tier price for " + component.ComponentCode)
			}
			if strings.TrimSpace(tier.UpTo) == "" {
				return new(big.Rat).Mul(price, quantity), tier.PricePerUnit, nil
			}
			upTo, valid := new(big.Rat).SetString(strings.TrimSpace(tier.UpTo))
			if !valid || upTo.Sign() < 0 {
				return nil, "", errors.New("invalid tier limit for " + component.ComponentCode)
			}
			if contextQuantity.Cmp(upTo) <= 0 {
				return new(big.Rat).Mul(price, quantity), tier.PricePerUnit, nil
			}
		}
		return nil, "", errors.New("pricing tiers have no applicable price for " + component.ComponentCode)
	}
	total := new(big.Rat)
	previous := new(big.Rat)
	effective := component.PricePerUnit
	for index, tier := range tiers {
		price, ok := new(big.Rat).SetString(strings.TrimSpace(tier.PricePerUnit))
		if !ok || price.Sign() < 0 {
			return nil, "", errors.New("invalid tier price for " + component.ComponentCode)
		}
		upper := new(big.Rat).Set(quantity)
		if strings.TrimSpace(tier.UpTo) != "" {
			parsed, valid := new(big.Rat).SetString(strings.TrimSpace(tier.UpTo))
			if !valid || parsed.Sign() < 0 {
				return nil, "", errors.New("invalid tier limit for " + component.ComponentCode)
			}
			if upper.Cmp(parsed) > 0 {
				upper = parsed
			}
			if parsed.Cmp(previous) <= 0 {
				return nil, "", errors.New("pricing tiers must have increasing limits for " + component.ComponentCode)
			}
		} else if index != len(tiers)-1 {
			return nil, "", errors.New("only the final pricing tier may be unbounded for " + component.ComponentCode)
		}
		if upper.Cmp(previous) > 0 {
			total.Add(total, new(big.Rat).Mul(price, new(big.Rat).Sub(upper, previous)))
			effective = tier.PricePerUnit
		}
		previous = upper
		if previous.Cmp(quantity) >= 0 {
			break
		}
	}
	if previous.Cmp(quantity) < 0 {
		finalPrice := base
		if len(tiers) > 0 {
			if parsed, valid := new(big.Rat).SetString(strings.TrimSpace(tiers[len(tiers)-1].PricePerUnit)); valid {
				finalPrice = parsed
			}
		}
		total.Add(total, new(big.Rat).Mul(finalPrice, new(big.Rat).Sub(quantity, previous)))
	}
	return total, effective, nil
}

func tierMetadata(metadata json.RawMessage) (string, string) {
	if len(metadata) == 0 || string(metadata) == "null" {
		return "", ""
	}
	var value struct {
		Basis string `json:"tier_basis"`
		Mode  string `json:"tier_mode"`
	}
	if json.Unmarshal(metadata, &value) != nil {
		return "", ""
	}
	return strings.TrimSpace(value.Basis), strings.ToLower(strings.TrimSpace(value.Mode))
}

func ratDecimal(value *big.Rat) string {
	text := strings.TrimRight(strings.TrimRight(value.FloatString(30), "0"), ".")
	if text == "" || text == "-0" {
		return "0"
	}
	return text
}

func multiplierCharge(charge MeteredCharge, multiplier string) (MeteredCharge, error) {
	return factorCharge(charge, multiplier, "group multiplier")
}

func factorCharge(charge MeteredCharge, factorValue, label string) (MeteredCharge, error) {
	factor, ok := new(big.Rat).SetString(strings.TrimSpace(factorValue))
	if !ok || factor.Sign() < 0 {
		return MeteredCharge{}, errors.New("invalid " + label)
	}
	total, ok := new(big.Rat).SetString(charge.Amount)
	if !ok {
		return MeteredCharge{}, errors.New("invalid charge")
	}
	result := MeteredCharge{Amount: ratDecimal(new(big.Rat).Mul(total, factor)), Lines: make([]ChargeLine, 0, len(charge.Lines))}
	for _, line := range charge.Lines {
		amount, ok := new(big.Rat).SetString(line.Amount)
		if !ok {
			return MeteredCharge{}, errors.New("invalid charge line")
		}
		unitPrice, ok := new(big.Rat).SetString(line.PricePerUnit)
		if !ok {
			return MeteredCharge{}, errors.New("invalid component unit price")
		}
		line.Amount = ratDecimal(new(big.Rat).Mul(amount, factor))
		line.PricePerUnit = ratDecimal(new(big.Rat).Mul(unitPrice, factor))
		result.Lines = append(result.Lines, line)
	}
	return result, nil
}

func normalizeUpstreamCostDiscount(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "1"
	}
	if !validNonNegativeDecimal(value) {
		return "", ErrInvalidRequest
	}
	factor, ok := new(big.Rat).SetString(value)
	if !ok || factor.Sign() < 0 || factor.Cmp(big.NewRat(1000, 1)) > 0 {
		return "", ErrInvalidRequest
	}
	return ratDecimal(factor), nil
}

func calculateUpstreamCost(
	components []PriceComponent,
	usage MeteredUsage,
	minimumCharge string,
	discount string,
	pricingTier string,
) (string, error) {
	discount, err := normalizeUpstreamCostDiscount(discount)
	if err != nil {
		return "", err
	}
	charge, err := calculateMeteredChargeForTier(components, usage, minimumCharge, pricingTier)
	if err != nil {
		return "", err
	}
	factored, err := factorCharge(charge, discount, "upstream cost discount")
	if err != nil {
		return "", err
	}
	return factored.Amount, nil
}

// prepareReservationMetrics keeps only request estimates that can be priced by
// the selected price version. Estimates are intentionally best-effort: a
// provider may publish a different final meter (for example video tokens
// instead of seconds), so an unknown estimate must not reject the request
// before it reaches the provider. Final settlement remains strict.
func prepareReservationMetrics(components []PriceComponent, usage MeteredUsage, pricingTier string) (MeteredUsage, error) {
	if usage == nil {
		return MeteredUsage{}, nil
	}
	normalized, err := normalizeMeteredUsage(usage)
	if err != nil {
		return nil, err
	}

	byCode := make(map[string]PriceComponent, len(components))
	for _, component := range components {
		code := strings.TrimSpace(component.ComponentCode)
		if !validComponentCode(code) || strings.TrimSpace(component.Unit) == "" {
			return nil, errors.New("invalid price component: " + code)
		}
		if _, exists := byCode[code]; exists {
			return nil, errors.New("duplicate price component: " + code)
		}
		byCode[code] = component
	}

	filtered := make(MeteredUsage, len(normalized))
	for code, value := range normalized {
		baseCode := componentBaseCode(code)
		// Parent totals synthesized by normalizeMeteredRelationships have no
		// billable remainder when all of their quantity is a child meter.
		if rationalValue(effectiveQuantity(baseCode, normalized)).Sign() <= 0 &&
			baseCode != code {
			continue
		}
		if _, ok := componentForMeter(byCode, baseCode, pricingTier); ok {
			filtered[code] = value
		}
	}

	// Rebuild implicit parent totals after dropping unknown estimates. This
	// keeps child meters such as audio/video tokens internally consistent.
	return normalizeMeteredUsage(filtered)
}

// calculateReservationCharge uses the request's conservative estimates. The
// final charge is always calculated from the provider usage returned at settle
// time, while request-based components are counted once for every request.
func calculateReservationCharge(components []PriceComponent, usage MeteredUsage, minimumCharge string, pricingTier ...string) (MeteredCharge, error) {
	if usage == nil {
		usage = MeteredUsage{}
	}
	tier := ""
	if len(pricingTier) > 0 {
		tier = pricingTier[0]
	}
	prepared, err := prepareReservationMetrics(components, usage, tier)
	if err != nil {
		return MeteredCharge{}, err
	}
	return calculateMeteredChargeForTier(components, prepared, minimumCharge, tier)
}
