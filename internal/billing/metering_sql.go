package billing

import (
	"context"
	"database/sql"
	"encoding/json"
	"math/big"
	"strconv"
	"strings"
	"unicode"

	"ai-token/internal/ids"
)

type componentQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func loadPriceComponents(ctx context.Context, queryer componentQueryer, priceVersionID string) ([]PriceComponent, error) {
	rows, err := queryer.QueryContext(ctx, "SELECT component_code, unit, price_per_unit::text, tier_json, metadata_json FROM price_components WHERE price_version_id = $1 ORDER BY component_code", priceVersionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	components := make([]PriceComponent, 0)
	for rows.Next() {
		var component PriceComponent
		if err := rows.Scan(&component.ComponentCode, &component.Unit, &component.PricePerUnit, &component.Tiers, &component.Metadata); err != nil {
			return nil, err
		}
		components = append(components, component)
	}
	return components, rows.Err()
}

func loadOfficialPriceComponents(ctx context.Context, queryer componentQueryer, priceVersionID string) ([]PriceComponent, error) {
	rows, err := queryer.QueryContext(ctx, "SELECT component_code, unit, price_per_unit::text, tier_json, metadata_json FROM official_price_components WHERE official_price_version_id = $1 ORDER BY component_code", priceVersionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	components := make([]PriceComponent, 0)
	for rows.Next() {
		var component PriceComponent
		if err := rows.Scan(&component.ComponentCode, &component.Unit, &component.PricePerUnit, &component.Tiers, &component.Metadata); err != nil {
			return nil, err
		}
		components = append(components, component)
	}
	return components, rows.Err()
}

func validatePriceComponentInputs(components []PriceComponentInput) error {
	seen := make(map[string]struct{}, len(components))
	for _, component := range components {
		code, unit, price := strings.TrimSpace(component.ComponentCode), strings.TrimSpace(component.Unit), strings.TrimSpace(component.PricePerUnit)
		if !validComponentCode(code) || unit == "" || len(unit) > 64 {
			return ErrInvalidPrice
		}
		if _, ok := canonicalDecimal(price, 24, 30); !ok {
			return ErrInvalidPrice
		}
		if _, ok := seen[code]; ok {
			return ErrInvalidPrice
		}
		seen[code] = struct{}{}
		if len(component.Tiers) > 0 && string(component.Tiers) != "null" {
			var tiers []PriceTier
			if err := json.Unmarshal(component.Tiers, &tiers); err != nil {
				return ErrInvalidPrice
			}
			for _, tier := range tiers {
				if _, ok := canonicalDecimal(tier.PricePerUnit, 24, 30); !ok {
					return ErrInvalidPrice
				}
				if strings.TrimSpace(tier.UpTo) != "" {
					if _, ok := canonicalDecimal(tier.UpTo, 30, 30); !ok {
						return ErrInvalidPrice
					}
				}
			}
			previous := new(big.Rat)
			for index, tier := range tiers {
				if strings.TrimSpace(tier.UpTo) == "" {
					if index != len(tiers)-1 {
						return ErrInvalidPrice
					}
					continue
				}
				limit, _ := new(big.Rat).SetString(strings.TrimSpace(tier.UpTo))
				if limit.Cmp(previous) <= 0 {
					return ErrInvalidPrice
				}
				previous = limit
			}
		}
	}
	return nil
}

func normalizePriceComponentInputs(components []PriceComponentInput) ([]PriceComponentInput, error) {
	if err := validatePriceComponentInputs(components); err != nil {
		return nil, err
	}
	result := make([]PriceComponentInput, 0, len(components))
	for _, component := range components {
		price, _ := canonicalDecimal(component.PricePerUnit, 24, 30)
		component.ComponentCode = strings.TrimSpace(component.ComponentCode)
		component.Unit = strings.TrimSpace(component.Unit)
		component.PricePerUnit = price
		if len(component.Tiers) > 0 && string(component.Tiers) != "null" {
			var tiers []PriceTier
			if err := json.Unmarshal(component.Tiers, &tiers); err != nil {
				return nil, ErrInvalidPrice
			}
			for index := range tiers {
				tiers[index].PricePerUnit, _ = canonicalDecimal(tiers[index].PricePerUnit, 24, 30)
				if strings.TrimSpace(tiers[index].UpTo) != "" {
					tiers[index].UpTo, _ = canonicalDecimal(tiers[index].UpTo, 30, 30)
				}
			}
			component.Tiers = marshalJSON(tiers, []byte(`[]`))
		}
		component.Tiers = nullableJSON(component.Tiers, []byte(`[]`))
		component.Metadata = nullableJSON(component.Metadata, []byte(`{}`))
		result = append(result, component)
	}
	return result, nil
}

func insertPriceComponents(ctx context.Context, tx *sql.Tx, priceVersionID string, components []PriceComponentInput) error {
	for _, component := range components {
		id, err := ids.New()
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO price_components (id, price_version_id, component_code, unit, price_per_unit, tier_json, metadata_json)
			VALUES ($1, $2, $3, $4, $5::numeric, $6::jsonb, $7::jsonb)
		`, id, priceVersionID, component.ComponentCode, component.Unit, component.PricePerUnit, component.Tiers, component.Metadata); err != nil {
			return err
		}
	}
	return nil
}

// canonicalDecimal parses decimal and scientific notation exactly. PostgreSQL
// price sources commonly use values such as 3E-08; accepting them without a
// float conversion keeps the eventual SQL numeric value deterministic.
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
	if base == "" {
		return "", false
	}
	parts := strings.Split(base, ".")
	if len(parts) > 2 || parts[0] == "" {
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
	var result string
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
	if _, ok := new(big.Rat).SetString(result); !ok {
		return "", false
	}
	return result, true
}

func legacyPriceComponentInputs(input, output, cached, reasoning string) []PriceComponentInput {
	result := make([]PriceComponentInput, 0, 4)
	for _, item := range []struct {
		code, value string
		optional    bool
	}{
		{"input_tokens", input, false},
		{"output_tokens", output, false},
		{"cached_input_tokens", cached, true},
		{"reasoning_tokens", reasoning, true},
	} {
		if strings.TrimSpace(item.value) == "" || item.optional && isZeroAmount(item.value) {
			continue
		}
		result = append(result, PriceComponentInput{ComponentCode: item.code, Unit: "token", PricePerUnit: item.value})
	}
	return result
}

func nullableJSON(value, fallback []byte) []byte {
	if len(value) == 0 || string(value) == "null" {
		return fallback
	}
	return value
}

func priceComponentsFor(price Price) []PriceComponent {
	if len(price.Components) > 0 {
		return price.Components
	}
	return legacyPriceComponents(price)
}

func usageMetricsFor(usage Usage) MeteredUsage {
	normalized := normalizeUsage(usage)
	if len(normalized.Metrics) > 0 {
		return normalized.Metrics
	}
	return usageFromLegacy(normalized.InputTokens, normalized.OutputTokens, normalized.CachedInputTokens, normalized.ReasoningTokens)
}

func requestMetricsFor(request Request) MeteredUsage {
	if len(request.EstimatedMetrics) > 0 {
		return request.EstimatedMetrics
	}
	return usageFromLegacy(request.EstimatedInputTokens, request.EstimatedOutputTokens, 0, 0)
}

func priceSnapshot(price Price) map[string]any {
	return map[string]any{
		"id":                          price.ID,
		"price_version_id":            price.PriceVersionID,
		"source":                      price.Source,
		"model_id":                    price.ModelID,
		"currency":                    price.Currency,
		"input_price_per_unit":        price.InputPricePerUnit,
		"output_price_per_unit":       price.OutputPricePerUnit,
		"cached_input_price_per_unit": price.CachedInputPricePerUnit,
		"reasoning_price_per_unit":    price.ReasoningPricePerUnit,
		"minimum_charge":              price.MinimumCharge,
		"components":                  priceComponentsFor(price),
	}
}

func marshalJSON(value any, fallback []byte) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fallback
	}
	return encoded
}
