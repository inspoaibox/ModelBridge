package relay

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var errUnsafeUpstreamURL = errors.New("unsafe upstream URL")

const relayUserAgent = "ai-token-relay"

func parseAndValidateBaseURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" ||
		parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errUnsafeUpstreamURL
	}
	if err := validateHost(parsed.Hostname()); err != nil {
		return nil, err
	}
	return parsed, nil
}

func validateHost(host string) error {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") ||
		host == "metadata.google.internal" || host == "metadata" {
		return errUnsafeUpstreamURL
	}
	if ip := net.ParseIP(host); ip != nil {
		return validateResolvedIP(ip)
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return errUnsafeUpstreamURL
	}
	for _, ip := range ips {
		if err := validateResolvedIP(ip); err != nil {
			return err
		}
	}
	return nil
}

func validateResolvedIP(ip net.IP) error {
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return errUnsafeUpstreamURL
	}
	// Explicitly cover the cloud metadata range even on older Go net stacks.
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 169 && ip4[1] == 254 {
		return errUnsafeUpstreamURL
	}
	return nil
}

func validUpstreamBaseURL(value string) bool {
	_, err := parseAndValidateBaseURL(value)
	return err == nil
}

// providerHTTPClient pins every new connection to a freshly checked public IP.
// Local hosts are left to the default client only for direct provider unit tests;
// persisted channel URLs are rejected by validUpstreamBaseURL before they reach
// this path.
func providerHTTPClient(rawBaseURL string) (*http.Client, error) {
	parsed, err := parseAndValidateBaseURL(rawBaseURL)
	if err != nil {
		if parsedURL, parseErr := url.Parse(strings.TrimSpace(rawBaseURL)); parseErr == nil && isLoopbackHost(parsedURL.Hostname()) {
			return http.DefaultClient, nil
		}
		return nil, err
	}
	host := parsed.Hostname()
	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	baseTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("default HTTP transport is not a TCP transport")
	}
	transport := baseTransport.Clone()
	transport.Proxy = nil
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		ips, lookupErr := net.LookupIP(host)
		if lookupErr != nil || len(ips) == 0 {
			return nil, errUnsafeUpstreamURL
		}
		for _, ip := range ips {
			if validateResolvedIP(ip) != nil {
				continue
			}
			conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if dialErr == nil {
				return conn, nil
			}
		}
		return nil, errUnsafeUpstreamURL
	}
	return &http.Client{
		Transport: transport,
		Timeout:   90 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errors.New("upstream redirects are not allowed")
		},
	}, nil
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(host)
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func upstreamPort(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return ""
	}
	if port := parsed.Port(); port != "" {
		return port
	}
	if parsed.Scheme == "https" {
		return strconv.Itoa(443)
	}
	return strconv.Itoa(80)
}
