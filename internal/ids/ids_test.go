package ids

import (
	"regexp"
	"testing"
)

func TestNewReturnsUUID(t *testing.T) {
	value, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(value) {
		t.Fatalf("invalid UUID %q", value)
	}
}

func TestValid(t *testing.T) {
	if !Valid("11111111-1111-4111-8111-111111111111") {
		t.Fatal("expected UUID to be valid")
	}
	if Valid("not-a-uuid") || Valid("11111111-1111-4111-8111-11111111111z") {
		t.Fatal("expected malformed UUID to be invalid")
	}
}
