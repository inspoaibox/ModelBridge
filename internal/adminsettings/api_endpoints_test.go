package adminsettings

import "testing"

func TestNormalizeAPIEndpointMutation(t *testing.T) {
	enabled := false
	tests := []struct {
		name     string
		request  APIEndpointMutation
		creating bool
		wantName string
		wantURL  string
		wantOn   bool
		wantErr  bool
	}{
		{
			name:     "normalizes whitespace and trailing slash",
			request:  APIEndpointMutation{Name: " Primary ", BaseURL: " https://gateway.example.com/v1/ "},
			creating: true,
			wantName: "Primary",
			wantURL:  "https://gateway.example.com",
			wantOn:   true,
		},
		{
			name:     "retains requested disabled state",
			request:  APIEndpointMutation{Name: "Maintenance", BaseURL: "http://127.0.0.1:8080/v1", Enabled: &enabled},
			creating: true,
			wantName: "Maintenance",
			wantURL:  "http://127.0.0.1:8080",
			wantOn:   false,
		},
		{
			name:     "preserves gateway path prefix",
			request:  APIEndpointMutation{Name: "Prefixed", BaseURL: "https://gateway.example.com/api/v1"},
			creating: true,
			wantName: "Prefixed",
			wantURL:  "https://gateway.example.com/api",
			wantOn:   true,
		},
		{name: "blank name", request: APIEndpointMutation{BaseURL: "https://gateway.example.com/v1"}, creating: true, wantErr: true},
		{name: "unsupported scheme", request: APIEndpointMutation{Name: "FTP", BaseURL: "ftp://gateway.example.com/v1"}, creating: true, wantErr: true},
		{name: "embedded credentials", request: APIEndpointMutation{Name: "Unsafe", BaseURL: "https://user:pass@gateway.example.com/v1"}, creating: true, wantErr: true},
		{name: "query string", request: APIEndpointMutation{Name: "Unsafe", BaseURL: "https://gateway.example.com/v1?key=test"}, creating: true, wantErr: true},
		{name: "fragment", request: APIEndpointMutation{Name: "Unsafe", BaseURL: "https://gateway.example.com/v1#anchor"}, creating: true, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			name, baseURL, enabled, err := normalizeAPIEndpointMutation(test.request, test.creating)
			if test.wantErr {
				if err == nil {
					t.Fatal("expected validation error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if name != test.wantName || baseURL != test.wantURL || enabled != test.wantOn {
				t.Fatalf("normalized endpoint = (%q, %q, %t), want (%q, %q, %t)", name, baseURL, enabled, test.wantName, test.wantURL, test.wantOn)
			}
		})
	}
}

func TestPublicAPIEndpointFromProvidesProtocolURLs(t *testing.T) {
	item := PublicAPIEndpointFrom(APIEndpoint{Name: "Primary", BaseURL: "https://gateway.example.com/v1/"})
	if item.BaseURL != "https://gateway.example.com" ||
		item.OpenAIBaseURL != "https://gateway.example.com/v1" ||
		item.AnthropicBaseURL != "https://gateway.example.com" {
		t.Fatalf("unexpected public endpoint: %#v", item)
	}
}
