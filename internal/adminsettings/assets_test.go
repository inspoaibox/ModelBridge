package adminsettings

import "testing"

func TestValidAssetURLSupportsSVGWithoutAllowingUnsafeSources(t *testing.T) {
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "https svg", value: "https://cdn.example.com/brand/logo.svg", valid: true},
		{name: "https svg with cache query", value: "https://cdn.example.com/brand/favicon.svg?v=2", valid: true},
		{name: "same origin svg", value: "/assets/brand/logo.svg", valid: true},
		{name: "http is rejected", value: "http://cdn.example.com/brand/logo.svg", valid: false},
		{name: "javascript is rejected", value: "javascript:alert(1)", valid: false},
		{name: "inline data svg is rejected", value: "data:image/svg+xml,<svg></svg>", valid: false},
		{name: "embedded credentials are rejected", value: "https://user:pass@cdn.example.com/logo.svg", valid: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validAssetURL(test.value); got != test.valid {
				t.Fatalf("validAssetURL(%q) = %t, want %t", test.value, got, test.valid)
			}
		})
	}
}
