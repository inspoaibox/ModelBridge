package announcements

import (
	"testing"
	"time"
)

func TestStatusForAnnouncement(t *testing.T) {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	if got := statusFor(false, now.Add(-time.Hour), nil, now); got != "disabled" {
		t.Fatalf("disabled status = %q", got)
	}
	if got := statusFor(true, now.Add(time.Hour), nil, now); got != "scheduled" {
		t.Fatalf("scheduled status = %q", got)
	}
	expires := now.Add(time.Hour)
	if got := statusFor(true, now.Add(-time.Hour), &expires, now); got != "active" {
		t.Fatalf("active status = %q", got)
	}
	if got := statusFor(true, now.Add(-2*time.Hour), &expires, now.Add(2*time.Hour)); got != "expired" {
		t.Fatalf("expired status = %q", got)
	}
}

func TestNormalizeAnnouncementRequest(t *testing.T) {
effective := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	request, err := normalizeRequest(CreateRequest{Title: "  Notice ", Content: "  **hello** ", EffectiveAt: effective, Enabled: true})
	if err != nil || request.Title != "Notice" || request.Content != "**hello**" {
		t.Fatalf("normalized request = %#v, %v", request, err)
	}
	if _, err := normalizeRequest(CreateRequest{Title: "Notice", Content: "body", EffectiveAt: effective, ExpiresAt: &effective}); err == nil {
		t.Fatal("expiration equal to effective time must be rejected")
	}
}
