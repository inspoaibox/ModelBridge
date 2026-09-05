package payments

import (
	"encoding/json"
	"fmt"
	"strings"
)

// providerErrorSummary extracts actionable provider fields while bounding the
// detail stored in order records and returned to the customer.
func providerErrorSummary(raw []byte) string {
	var payload any
	if json.Unmarshal(raw, &payload) == nil {
		parts := make([]string, 0, 8)
		collectProviderErrorParts(payload, &parts)
		if len(parts) > 0 {
			return truncateProviderDetail(strings.Join(uniqueProviderErrorParts(parts), ": "))
		}
	}
	return truncateProviderDetail(strings.Join(strings.Fields(string(raw)), " "))
}

func collectProviderErrorParts(value any, parts *[]string) {
	switch item := value.(type) {
	case map[string]any:
		for _, key := range []string{"type", "name", "code", "sub_code", "subcode", "issue", "message", "msg", "sub_msg", "description", "debug_id", "param", "param_value"} {
			if text, ok := item[key].(string); ok && strings.TrimSpace(text) != "" {
				*parts = append(*parts, strings.TrimSpace(text))
			}
		}
		for key, nested := range item {
			if key == "error" || key == "details" || key == "errors" || key == "alipay_trade_precreate_response" {
				collectProviderErrorParts(nested, parts)
			}
		}
	case []any:
		for _, nested := range item {
			collectProviderErrorParts(nested, parts)
		}
	}
}

func uniqueProviderErrorParts(parts []string) []string {
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		result = append(result, part)
	}
	return result
}

func truncateProviderDetail(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "empty response"
	}
	if len(value) > 240 {
		return fmt.Sprintf("%s...", value[:237])
	}
	return value
}
