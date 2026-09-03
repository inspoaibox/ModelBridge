package enterprise

import "testing"

func TestValidUnifiedCreditCode(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "valid code", value: "91350100M000100Y43", want: true},
		{name: "wrong check digit", value: "91350100M000100Y46", want: false},
		{name: "unsupported character", value: "91350100I000100Y43", want: false},
		{name: "wrong length", value: "91350100M000100Y4", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validUnifiedCreditCode(test.value); got != test.want {
				t.Fatalf("validUnifiedCreditCode(%q) = %t, want %t", test.value, got, test.want)
			}
		})
	}
}
