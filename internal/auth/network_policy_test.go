package auth

import "testing"

func TestNetworkAllowlistSupportsDomainOnlyBrowserPolicy(t *testing.T) {
	principal := &Principal{
		Type: PrincipalAPIToken,
		AllowedDomains: map[string]struct{}{
			"app.example.com": {},
		},
	}
	if !NetworkAllowlistAllows(principal, "203.0.113.10", "https://app.example.com", "") {
		t.Fatal("a matching browser origin should authorize a domain-only browser policy")
	}
	if NetworkAllowlistAllows(principal, "203.0.113.10", "", "") {
		t.Fatal("a domain-only token must reject requests without browser origin metadata")
	}
}

func TestNetworkAllowlistRequiresIPAndConstrainsBrowserOrigin(t *testing.T) {
	principal := &Principal{
		Type: PrincipalAPIToken,
		AllowedIPs: map[string]struct{}{
			"203.0.113.0/24": {},
		},
		AllowedDomains: map[string]struct{}{
			"app.example.com": {},
		},
	}
	if !NetworkAllowlistAllows(principal, "203.0.113.10", "https://app.example.com", "") {
		t.Fatal("matching IP and browser origin should be allowed")
	}
	if !NetworkAllowlistAllows(principal, "198.51.100.10", "https://app.example.com", "") {
		t.Fatal("a matching configured IP or domain policy should be allowed")
	}
	if !NetworkAllowlistAllows(principal, "203.0.113.10", "https://attacker.example", "") {
		t.Fatal("a matching IP entry should remain an alternative to the domain policy")
	}
	if !NetworkAllowlistAllows(principal, "203.0.113.10", "", "") {
		t.Fatal("an approved server-to-server IP without browser headers should be allowed")
	}
}
