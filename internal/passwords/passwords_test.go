package passwords

import (
	"testing"
)

func TestHashAndVerify(t *testing.T) {
	password := "correct horse battery staple"
	encoded, err := Hash(password)
	if err != nil {
		t.Fatal(err)
	}
	if !Verify(password, encoded) {
		t.Fatal("expected password to verify")
	}
	if Verify("wrong password", encoded) {
		t.Fatal("wrong password must not verify")
	}
	if Verify(password, encoded+"x") {
		t.Fatal("modified hash must not verify")
	}
}

func TestHashRejectsWeakPassword(t *testing.T) {
	if _, err := Hash("short"); err == nil {
		t.Fatal("expected weak password to be rejected")
	}
}
