package tokens

import "testing"

func TestGenerateAndVerify(t *testing.T) {
	hasher, err := NewHasher("01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}

	plain, prefix, digest, err := hasher.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if len(plain) <= len(prefix) || plain[:len(prefix)] != prefix {
		t.Fatalf("invalid prefix %q for token %q", prefix, plain)
	}
	if !hasher.Verify(plain, digest) {
		t.Fatal("expected generated token to verify")
	}
	if hasher.Verify(plain+"x", digest) {
		t.Fatal("modified token must not verify")
	}
	if hasher.Verify(plain, digest[:len(digest)-1]) {
		t.Fatal("modified digest must not verify")
	}
}

func TestHasherRequiresPepper(t *testing.T) {
	if _, err := NewHasher("too-short"); err == nil {
		t.Fatal("expected short pepper to be rejected")
	}
}

func TestVerifyRejectsMalformedTokens(t *testing.T) {
	hasher, err := NewHasher("01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}

	for _, token := range []string{
		"",
		"sk_test_abc",
		"sk_live_abc",
		"sk_live_!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!",
	} {
		if hasher.Verify(token, "deadbeef") {
			t.Fatalf("malformed token %q must not verify", token)
		}
	}
}
