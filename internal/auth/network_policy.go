package auth

import (
	"net"
	"net/url"
	"strings"
)

// NetworkAllowlistAllows treats an empty allowlist as disabled. IP/CIDR entries
// match the actual peer address and are suitable for server-to-server calls;
// domain entries match browser Origin/Referer metadata and are an alternative
// policy for browser clients. Origin and Referer are client-controlled outside
// a browser, so domain-only tokens must not be treated as a strong bearer-token
// boundary for untrusted non-browser callers.
func NetworkAllowlistAllows(principal *Principal, remoteAddr, origin, referer string) bool {
	if principal == nil || principal.Type != PrincipalAPIToken {
		return true
	}
	if len(principal.AllowedIPs) == 0 && len(principal.AllowedDomains) == 0 {
		return true
	}
	if len(principal.AllowedIPs) > 0 && sourceIPMatches(principal.AllowedIPs, remoteAddr) {
		return true
	}
	if len(principal.AllowedDomains) > 0 {
		for _, sourceDomain := range requestDomains(origin, referer) {
			if sourceDomainMatches(principal.AllowedDomains, sourceDomain) {
				return true
			}
		}
	}
	return false
}

func sourceIPMatches(allowed map[string]struct{}, remoteAddr string) bool {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		remoteAddr = host
	}
	ip := net.ParseIP(remoteAddr)
	if ip == nil {
		return false
	}
	for entry := range allowed {
		entry = strings.TrimSpace(entry)
		if exact := net.ParseIP(entry); exact != nil && exact.Equal(ip) {
			return true
		}
		if _, network, err := net.ParseCIDR(entry); err == nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

func requestDomains(origin, referer string) []string {
	result := make([]string, 0, 2)
	for _, raw := range []string{origin, referer} {
		raw = strings.TrimSpace(raw)
		if raw == "" || strings.EqualFold(raw, "null") {
			continue
		}
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Hostname() == "" {
			continue
		}
		host := normalizeDomain(parsed.Hostname())
		if host != "" {
			result = append(result, host)
		}
	}
	return result
}

func sourceDomainMatches(allowed map[string]struct{}, source string) bool {
	source = normalizeDomain(source)
	for entry := range allowed {
		entry = normalizeDomain(entry)
		if strings.HasPrefix(entry, "*.") {
			base := strings.TrimPrefix(entry, "*.")
			if source != base && strings.HasSuffix(source, "."+base) {
				return true
			}
			continue
		}
		if source == entry {
			return true
		}
	}
	return false
}

func normalizeDomain(value string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
}
