package auth

import "testing"

func TestNewSMTPPasswordResetNotifierValidatesConfiguration(t *testing.T) {
	if _, err := NewSMTPPasswordResetNotifier("smtp.example.com:587", "no-reply@example.com", "user", "", "https://gateway.example.com"); err == nil {
		t.Fatal("expected partial SMTP credentials to be rejected")
	}
	if _, err := NewSMTPPasswordResetNotifier("smtp.example.com:587", "no-reply@example.com", "", "", "http://gateway.example.com"); err == nil {
		t.Fatal("expected insecure public URL to be rejected")
	}
	notifier, err := NewSMTPPasswordResetNotifier("smtp.example.com:587", "no-reply@example.com", "user", "password", "https://gateway.example.com/")
	if err != nil {
		t.Fatal(err)
	}
	if notifier.host != "smtp.example.com" || notifier.from != "no-reply@example.com" || notifier.baseURL != "https://gateway.example.com" {
		t.Fatalf("unexpected notifier: %#v", notifier)
	}
}
