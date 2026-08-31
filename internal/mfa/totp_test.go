package mfa

import (
	"bytes"
	"encoding/base32"
	"encoding/base64"
	"testing"
	"time"
)

func TestTOTPUsesRFC6238Vector(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	got, err := Code(secret, time.Unix(59, 0).UTC(), 8)
	if err != nil {
		t.Fatal(err)
	}
	if got != "94287082" {
		t.Fatalf("expected RFC 6238 code 94287082, got %s", got)
	}
	if !Verify(secret, got, time.Unix(59, 0).UTC(), 0) {
		t.Fatal("expected code to verify")
	}
}

func TestSecretBoxRoundTripAndTamperRejection(t *testing.T) {
	box, err := NewSecretBox(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := box.Seal([]byte("mfa-secret"))
	if err != nil {
		t.Fatal(err)
	}
	opened, err := box.Open(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if string(opened) != "mfa-secret" {
		t.Fatalf("unexpected plaintext %q", opened)
	}

	raw, err := base64.RawURLEncoding.DecodeString(sealed)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] ^= 1
	tampered := base64.RawURLEncoding.EncodeToString(raw)
	if _, err := box.Open(tampered); err == nil {
		t.Fatal("tampered secret must be rejected")
	}
}
