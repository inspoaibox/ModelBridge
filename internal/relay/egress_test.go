package relay

import "testing"

func TestValidUpstreamBaseURLRejectsPrivateAndLocalTargets(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{name: "localhost", url: "http://localhost:8080/v1"},
		{name: "loopback", url: "http://127.0.0.1:8080/v1"},
		{name: "private", url: "http://10.0.0.4/v1"},
		{name: "link local", url: "http://169.254.169.254/latest/meta-data"},
		{name: "ipv6 private", url: "http://[fd00::1]/v1"},
		{name: "query", url: "https://api.example.com/v1?next=http://127.0.0.1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if validUpstreamBaseURL(test.url) {
				t.Fatalf("%s must be rejected", test.url)
			}
		})
	}
}

func TestProviderHTTPClientAllowsOnlyLoopbackInDirectUnitTests(t *testing.T) {
	if client, err := providerHTTPClient("http://127.0.0.1:8080/v1"); err != nil || client == nil {
		t.Fatalf("loopback test client should remain usable: client=%v err=%v", client, err)
	}
	if _, err := providerHTTPClient("http://10.0.0.4:8080/v1"); err == nil {
		t.Fatal("private target must not get an HTTP client")
	}
}
