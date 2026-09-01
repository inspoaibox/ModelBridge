package auth

import (
	"context"
	"testing"
)

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

type emailSwitchProvider struct {
	enabled       bool
	featureEnable bool
}

func (p emailSwitchProvider) GetSMTPSettings(context.Context) (SMTPSettings, error) {
	return SMTPSettings{Host: "smtp.example.com", Port: 587, From: "no-reply@example.com", TLS: true, Configured: true}, nil
}

func (p emailSwitchProvider) EmailEnabled(context.Context) (bool, error) {
	return p.enabled, nil
}

func (p emailSwitchProvider) EmailFeatureEnabled(context.Context, string) (bool, error) {
	return p.featureEnable, nil
}

func TestDynamicSMTPNotifierRespectsFeatureSwitches(t *testing.T) {
	notifier := NewDynamicSMTPPasswordResetNotifier(emailSwitchProvider{enabled: false, featureEnable: true}, SMTPSettings{})
	if err := notifier.SendPasswordReset(context.Background(), "user@example.com", "reset-token"); err != ErrEmailDisabled {
		t.Fatalf("disabled email system error = %v, want ErrEmailDisabled", err)
	}
	notifier = NewDynamicSMTPPasswordResetNotifier(emailSwitchProvider{enabled: true, featureEnable: false}, SMTPSettings{})
	if err := notifier.SendPasswordReset(context.Background(), "user@example.com", "reset-token"); err != ErrEmailDisabled {
		t.Fatalf("disabled email event error = %v, want ErrEmailDisabled", err)
	}
}
