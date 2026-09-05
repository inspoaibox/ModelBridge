package models

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"time"
)

var ErrUnavailable = errors.New("model catalog is unavailable")

type Pricing struct {
	Currency                           string           `json:"currency"`
	InputPricePerUnit                  string           `json:"input_price_per_unit"`
	OutputPricePerUnit                 string           `json:"output_price_per_unit"`
	CachedInputPricePerUnit            string           `json:"cached_input_price_per_unit"`
	CacheCreationPricePerUnit          string           `json:"cache_creation_price_per_unit"`
	ReasoningPricePerUnit              string           `json:"reasoning_price_per_unit"`
	MinimumCharge                      string           `json:"minimum_charge"`
	Unit                               string           `json:"unit"`
	InputPricePerMillionTokens         string           `json:"input_price_per_million_tokens"`
	OutputPricePerMillionTokens        string           `json:"output_price_per_million_tokens"`
	CachedInputPricePerMillionTokens   string           `json:"cached_input_price_per_million_tokens"`
	CacheCreationPricePerMillionTokens string           `json:"cache_creation_price_per_million_tokens"`
	ReasoningPricePerMillionTokens     string           `json:"reasoning_price_per_million_tokens"`
	Source                             string           `json:"source"`
	SourceURL                          string           `json:"source_url,omitempty"`
	UpdatedAt                          string           `json:"updated_at,omitempty"`
	Components                         []PriceComponent `json:"components,omitempty"`
	PlatformPrices                     []PlatformPrice  `json:"platform_prices,omitempty"`
}

type PriceComponent struct {
	ComponentCode string          `json:"component_code"`
	Unit          string          `json:"unit"`
	PricePerUnit  string          `json:"price_per_unit"`
	Tiers         json.RawMessage `json:"tiers,omitempty"`
	Metadata      json.RawMessage `json:"metadata,omitempty"`
}

type PlatformPrice struct {
	GroupID                            string           `json:"group_id"`
	GroupCode                          string           `json:"group_code"`
	GroupName                          string           `json:"group_name"`
	Multiplier                         string           `json:"multiplier"`
	BillingType                        string           `json:"billing_type"`
	MeteringMode                       string           `json:"metering_mode"`
	MeteringPrice                      string           `json:"metering_price"`
	MeteringPricePerUnit               string           `json:"metering_price_per_unit,omitempty"`
	MeteringUnit                       string           `json:"metering_unit,omitempty"`
	InputPricePerMillionTokens         string           `json:"input_price_per_million_tokens"`
	OutputPricePerMillionTokens        string           `json:"output_price_per_million_tokens"`
	CachedInputPricePerMillionTokens   string           `json:"cached_input_price_per_million_tokens"`
	CacheCreationPricePerMillionTokens string           `json:"cache_creation_price_per_million_tokens"`
	ReasoningPricePerMillionTokens     string           `json:"reasoning_price_per_million_tokens"`
	Components                         []PriceComponent `json:"components,omitempty"`
}

type Summary struct {
	ID                 string         `json:"id"`
	Provider           string         `json:"provider"`
	Name               string         `json:"name"`
	DisplayName        string         `json:"display_name"`
	ProtocolFamily     string         `json:"protocol_family"`
	Category           string         `json:"category"`
	Capabilities       map[string]any `json:"capabilities"`
	ChannelCount       int            `json:"channel_count"`
	ActiveChannelCount int            `json:"active_channel_count"`
	Available          bool           `json:"available"`
	GroupIDs           []string       `json:"group_ids,omitempty"`
	Pricing            *Pricing       `json:"pricing,omitempty"`
}

type Catalog interface {
	ListPublic(context.Context) ([]Summary, error)
}

type SQLCatalog struct {
	db *sql.DB
}

func NewCatalog(db *sql.DB) (*SQLCatalog, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &SQLCatalog{db: db}, nil
}

func (c *SQLCatalog) ListPublic(ctx context.Context) ([]Summary, error) {
	if c == nil || c.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := c.db.QueryContext(ctx, `
		SELECT m.id::text, m.provider, m.model_name, m.protocol_family,
		       m.capabilities_json,
		       COUNT(DISTINCT cm.channel_id) FILTER (WHERE c.id IS NOT NULL),
		       COUNT(DISTINCT cm.channel_id) FILTER (
		           WHERE c.status = 'active'
		             AND (c.auto_disabled_until IS NULL OR c.auto_disabled_until <= now())
		             AND (cm.auto_disabled_until IS NULL OR cm.auto_disabled_until <= now())
		       ),
		       COALESCE(pricing.currency, official.currency),
		       COALESCE(pricing.input_price_per_unit, official.input_price_per_unit),
		       COALESCE(pricing.output_price_per_unit, official.output_price_per_unit),
		       COALESCE(pricing.cached_input_price_per_unit, official.cached_input_price_per_unit),
		       COALESCE(pricing.reasoning_price_per_unit, official.reasoning_price_per_unit),
		       COALESCE(pricing.minimum_charge, official.minimum_charge),
		       COALESCE(pricing.source, official.source),
		       COALESCE(pricing.source_url, official.source_url),
		       COALESCE(pricing.updated_at, official.updated_at),
		       COALESCE(pricing.id::text, official.id::text, ''),
		       COALESCE((
		           SELECT jsonb_agg(
		               jsonb_build_object(
		                   'group_id', rg.id::text,
		                   'group_code', rg.code,
				           'group_name', rg.name,
				           'multiplier', rg.multiplier::text,
				           'billing_type', rg.billing_type,
				           'metering_mode', rg.metering_mode,
				           'metering_price', rg.metering_price::text
		               ) ORDER BY rg.priority DESC, rg.code ASC
		           )
			           FROM routing_groups rg
			           WHERE rg.status = 'active' AND rg.deleted_at IS NULL
			             AND EXISTS (
			                 SELECT 1
			                 FROM routing_group_channels rgc
			                 JOIN channels group_c ON group_c.id = rgc.channel_id
			                 JOIN channel_models group_cm ON group_cm.channel_id = rgc.channel_id
			                 WHERE rgc.group_id = rg.id
		                   AND group_c.status = 'active'
		                   AND (group_c.auto_disabled_until IS NULL OR group_c.auto_disabled_until <= now())
		                   AND group_c.deleted_at IS NULL
		                   AND group_cm.model_id = m.id
	                   AND group_cm.enabled = true
	                   AND (group_cm.auto_disabled_until IS NULL OR group_cm.auto_disabled_until <= now())
	             )
		       ), '[]'::jsonb)
		FROM models m
		JOIN channel_models cm
		       ON cm.model_id = m.id
		      AND cm.enabled = true
		JOIN channels c
		       ON c.id = cm.channel_id
		      AND c.deleted_at IS NULL
		LEFT JOIN LATERAL (
			SELECT pv.id, pv.currency,
			       pv.input_price_per_unit::text,
			       pv.output_price_per_unit::text,
			       pv.cached_input_price_per_unit::text,
			       pv.reasoning_price_per_unit::text,
			       pv.minimum_charge::text,
			       'manual'::text AS source,
			       ''::text AS source_url,
			       pv.effective_from AS updated_at
			FROM price_versions pv
			WHERE pv.model_id = m.id
			  AND pv.scope_type = 'platform_default'
			  AND pv.scope_id IS NULL
			  AND pv.currency = 'USD'
			  AND pv.status = 'active'
			  AND pv.effective_from <= now()
			  AND (pv.effective_to IS NULL OR pv.effective_to > now())
			ORDER BY pv.effective_from DESC, pv.version DESC
			LIMIT 1
		) pricing ON true
		LEFT JOIN LATERAL (
			SELECT omp.id, omp.currency,
			       omp.input_price_per_unit::text,
			       omp.output_price_per_unit::text,
			       omp.cached_input_price_per_unit::text,
			       omp.reasoning_price_per_unit::text,
			       '0'::text AS minimum_charge,
			       omp.source,
			       omp.source_url,
			       omp.fetched_at AS updated_at
			FROM official_model_price_versions omp
			WHERE omp.model_id = m.id
			  AND omp.source = 'litellm'
			  AND omp.effective_to IS NULL
			ORDER BY omp.effective_from DESC
			LIMIT 1
		) official ON true
		WHERE m.status = 'active'
		GROUP BY m.id, m.provider, m.model_name, m.protocol_family,
		         m.capabilities_json, pricing.id, official.id, pricing.currency, official.currency,
		         pricing.input_price_per_unit, official.input_price_per_unit,
		         pricing.output_price_per_unit, official.output_price_per_unit,
		         pricing.cached_input_price_per_unit, official.cached_input_price_per_unit,
		         pricing.reasoning_price_per_unit, official.reasoning_price_per_unit,
		         pricing.minimum_charge, official.minimum_charge,
		         pricing.source, official.source, pricing.source_url, official.source_url,
		         pricing.updated_at, official.updated_at
		ORDER BY m.provider ASC, m.model_name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Summary, 0)
	for rows.Next() {
		var (
			item               Summary
			capabilitiesRaw    []byte
			channelCount       int
			activeChannelCount int
			currency           sql.NullString
			inputPrice         sql.NullString
			outputPrice        sql.NullString
			cachedInputPrice   sql.NullString
			reasoningPrice     sql.NullString
			minimumCharge      sql.NullString
			pricingSource      sql.NullString
			pricingSourceURL   sql.NullString
			pricingUpdatedAt   sql.NullTime
			componentPriceID   string
			groupsRaw          []byte
		)
		if err := rows.Scan(
			&item.ID,
			&item.Provider,
			&item.Name,
			&item.ProtocolFamily,
			&capabilitiesRaw,
			&channelCount,
			&activeChannelCount,
			&currency,
			&inputPrice,
			&outputPrice,
			&cachedInputPrice,
			&reasoningPrice,
			&minimumCharge,
			&pricingSource,
			&pricingSourceURL,
			&pricingUpdatedAt,
			&componentPriceID,
			&groupsRaw,
		); err != nil {
			return nil, err
		}
		item.Provider = strings.ToLower(strings.TrimSpace(item.Provider))
		item.Name = strings.TrimSpace(item.Name)
		item.DisplayName = item.Name
		item.Capabilities = map[string]any{}
		if len(capabilitiesRaw) > 0 {
			if err := json.Unmarshal(capabilitiesRaw, &item.Capabilities); err != nil {
				return nil, err
			}
		}
		item.Category = modelCategory(item.Provider, item.Name, item.ProtocolFamily, item.Capabilities)
		var groupRefs []groupPriceBase
		if len(groupsRaw) > 0 {
			_ = json.Unmarshal(groupsRaw, &groupRefs)
			item.GroupIDs = make([]string, 0, len(groupRefs))
			for _, group := range groupRefs {
				if strings.TrimSpace(group.GroupID) != "" {
					item.GroupIDs = append(item.GroupIDs, group.GroupID)
				}
			}
		}
		item.ChannelCount = channelCount
		item.ActiveChannelCount = activeChannelCount
		item.Available = activeChannelCount > 0
		if currency.Valid {
			item.Pricing = &Pricing{
				Currency:                strings.ToUpper(strings.TrimSpace(currency.String)),
				InputPricePerUnit:       inputPrice.String,
				OutputPricePerUnit:      outputPrice.String,
				CachedInputPricePerUnit: cachedInputPrice.String,
				ReasoningPricePerUnit:   reasoningPrice.String,
				MinimumCharge:           minimumCharge.String,
				Unit:                    "per_1m_tokens",
				Source:                  pricingSource.String,
				SourceURL:               pricingSourceURL.String,
				Components:              loadPublicPriceComponents(ctx, c.db, componentPriceID, pricingSource.String),
			}
			item.Pricing.CacheCreationPricePerUnit = publicComponentPrice(item.Pricing.Components, "cache_creation_tokens")
			item.Pricing.InputPricePerMillionTokens = perMillionTokens(inputPrice.String)
			item.Pricing.OutputPricePerMillionTokens = perMillionTokens(outputPrice.String)
			item.Pricing.CachedInputPricePerMillionTokens = perMillionTokens(cachedInputPrice.String)
			item.Pricing.CacheCreationPricePerMillionTokens = perMillionTokens(item.Pricing.CacheCreationPricePerUnit)
			item.Pricing.ReasoningPricePerMillionTokens = perMillionTokens(reasoningPrice.String)
			item.Pricing.PlatformPrices = platformPrices(groupsRaw, item.Pricing)
			if pricingUpdatedAt.Valid {
				item.Pricing.UpdatedAt = pricingUpdatedAt.Time.UTC().Format(time.RFC3339)
			}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func modelCategory(provider, name, protocol string, capabilities map[string]any) string {
	for _, item := range []struct{ key, category string }{
		{"image_generation", "image"},
		{"video_generation", "video"},
		{"audio", "audio"},
		{"embedding", "embedding"},
	} {
		if enabled, ok := capabilities[item.key].(bool); ok && enabled {
			return item.category
		}
	}
	if value, ok := capabilities["category"].(string); ok {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "text", "image", "video", "audio", "embedding":
			return strings.ToLower(strings.TrimSpace(value))
		}
	}
	if value, ok := capabilities["mode"].(string); ok {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "image", "image_generation":
			return "image"
		case "video", "video_generation":
			return "video"
		case "embedding", "embeddings":
			return "embedding"
		}
	}
	value := strings.ToLower(strings.TrimSpace(provider + " " + name + " " + protocol))
	switch {
	case strings.Contains(value, "embedding"), strings.Contains(value, "embed"):
		return "embedding"
	case strings.Contains(value, "audio"), strings.Contains(value, "whisper"), strings.Contains(value, "transcri"), strings.Contains(value, "speech"), strings.Contains(value, "tts"):
		return "audio"
	case strings.Contains(value, "video"), strings.Contains(value, "veo"), strings.Contains(value, "sora"), strings.Contains(value, "kling"):
		return "video"
	case strings.Contains(value, "image"), strings.Contains(value, "dall-e"), strings.Contains(value, "imagen"), strings.Contains(value, "flux"):
		return "image"
	default:
		return "text"
	}
}

func loadPublicPriceComponents(ctx context.Context, db *sql.DB, priceID, source string) []PriceComponent {
	if db == nil || strings.TrimSpace(priceID) == "" {
		return []PriceComponent{}
	}
	table, column := "price_components", "price_version_id"
	if source == "litellm" {
		table, column = "official_price_components", "official_price_version_id"
	}
	rows, err := db.QueryContext(ctx, "SELECT component_code, unit, price_per_unit::text, tier_json, metadata_json FROM "+table+" WHERE "+column+" = $1 ORDER BY component_code", priceID)
	if err != nil {
		return []PriceComponent{}
	}
	defer rows.Close()
	components := make([]PriceComponent, 0)
	for rows.Next() {
		var component PriceComponent
		if err := rows.Scan(&component.ComponentCode, &component.Unit, &component.PricePerUnit, &component.Tiers, &component.Metadata); err != nil {
			return []PriceComponent{}
		}
		components = append(components, component)
	}
	return components
}

func publicComponentPrice(components []PriceComponent, code string) string {
	for _, component := range components {
		if component.ComponentCode == code {
			return strings.TrimSpace(component.PricePerUnit)
		}
	}
	return ""
}

type groupPriceBase struct {
	GroupID       string `json:"group_id"`
	GroupCode     string `json:"group_code"`
	GroupName     string `json:"group_name"`
	Multiplier    string `json:"multiplier"`
	BillingType   string `json:"billing_type"`
	MeteringMode  string `json:"metering_mode"`
	MeteringPrice string `json:"metering_price"`
}

func platformPrices(raw []byte, pricing *Pricing) []PlatformPrice {
	if len(raw) == 0 || pricing == nil {
		return []PlatformPrice{}
	}
	var groups []groupPriceBase
	if err := json.Unmarshal(raw, &groups); err != nil {
		return []PlatformPrice{}
	}
	prices := make([]PlatformPrice, 0, len(groups))
	for _, group := range groups {
		components := multiplyComponents(pricing.Components, group.Multiplier)
		meteringMode := strings.ToLower(strings.TrimSpace(group.MeteringMode))
		meteringUnit := ""
		switch meteringMode {
		case "image_count":
			meteringUnit = "image"
		case "video_seconds":
			meteringUnit = "second"
		case "video_request":
			meteringUnit = "request"
		}
		prices = append(prices, PlatformPrice{
			GroupID:                            group.GroupID,
			GroupCode:                          group.GroupCode,
			GroupName:                          group.GroupName,
			Multiplier:                         group.Multiplier,
			BillingType:                        group.BillingType,
			MeteringMode:                       meteringMode,
			MeteringPrice:                      strings.TrimSpace(group.MeteringPrice),
			MeteringPricePerUnit:               multiplyDecimal(group.MeteringPrice, group.Multiplier),
			MeteringUnit:                       meteringUnit,
			InputPricePerMillionTokens:         multiplyDecimal(pricing.InputPricePerMillionTokens, group.Multiplier),
			OutputPricePerMillionTokens:        multiplyDecimal(pricing.OutputPricePerMillionTokens, group.Multiplier),
			CachedInputPricePerMillionTokens:   multiplyDecimal(pricing.CachedInputPricePerMillionTokens, group.Multiplier),
			CacheCreationPricePerMillionTokens: multiplyDecimal(pricing.CacheCreationPricePerMillionTokens, group.Multiplier),
			ReasoningPricePerMillionTokens:     multiplyDecimal(pricing.ReasoningPricePerMillionTokens, group.Multiplier),
			Components:                         components,
		})
	}
	return prices
}

func multiplyComponents(components []PriceComponent, multiplier string) []PriceComponent {
	result := make([]PriceComponent, 0, len(components))
	for _, component := range components {
		component.PricePerUnit = multiplyDecimal(component.PricePerUnit, multiplier)
		if len(component.Tiers) > 0 && string(component.Tiers) != "null" {
			var tiers []map[string]any
			if json.Unmarshal(component.Tiers, &tiers) == nil {
				for _, tier := range tiers {
					if price, ok := tier["price_per_unit"].(string); ok {
						tier["price_per_unit"] = multiplyDecimal(price, multiplier)
					}
				}
				component.Tiers, _ = json.Marshal(tiers)
			}
		}
		result = append(result, component)
	}
	return result
}

func multiplyDecimal(left, right string) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return ""
	}
	leftValue, ok := new(big.Rat).SetString(left)
	if !ok {
		return ""
	}
	rightValue, ok := new(big.Rat).SetString(right)
	if !ok {
		return ""
	}
	leftValue.Mul(leftValue, rightValue)
	result := strings.TrimRight(strings.TrimRight(leftValue.FloatString(30), "0"), ".")
	if result == "" {
		return "0"
	}
	return result
}

func perMillionTokens(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, _, err := big.ParseFloat(value, 10, 256, big.ToNearestEven)
	if err != nil {
		return ""
	}
	million := new(big.Float).Mul(parsed, big.NewFloat(1_000_000))
	result := million.Text('f', 6)
	result = strings.TrimRight(strings.TrimRight(result, "0"), ".")
	if result == "" {
		return "0"
	}
	return result
}
