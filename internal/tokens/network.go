package tokens

import (
	"errors"
	"net"
	"strings"
)

var ErrNetworkAllowlistInvalid = errors.New("invalid token network allowlist")

func normalizeAllowedIPs(values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if parsed := net.ParseIP(value); parsed != nil {
			value = parsed.String()
		} else if _, network, err := net.ParseCIDR(value); err == nil {
			value = network.String()
		} else {
			return nil, ErrNetworkAllowlistInvalid
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func normalizeAllowedDomains(values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
		if value == "" {
			continue
		}
		wildcard := strings.HasPrefix(value, "*.")
		if wildcard {
			value = strings.TrimPrefix(value, "*.")
		}
		if !validHostname(value) {
			return nil, ErrNetworkAllowlistInvalid
		}
		if wildcard {
			value = "*." + value
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func validHostname(value string) bool {
	if value == "localhost" {
		return true
	}
	if len(value) > 253 || net.ParseIP(value) != nil {
		return false
	}
	labels := strings.Split(value, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
				return false
			}
		}
	}
	return true
}
