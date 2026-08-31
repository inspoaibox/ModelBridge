package audit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"ai-token/internal/auth"
)

type testWriter struct {
	events []Event
}

func (w *testWriter) Append(_ context.Context, event Event) error {
	w.events = append(w.events, event)
	return nil
}

func TestHTTPMiddlewareRecordsAuthenticatedActorAndPreservesFlushing(t *testing.T) {
	writer := &testWriter{}
	middleware := auth.NewMiddleware(auth.ResolverFunc(func(context.Context, string) (*auth.Principal, error) {
		return &auth.Principal{
			ID:          "11111111-1111-4111-8111-111111111111",
			Type:        auth.PrincipalPlatformUser,
			Audience:    auth.AudienceAdmin,
			Permissions: map[string]struct{}{"channel:update": {}},
		}, nil
	}))
	flushed := false
	handler := HTTPMiddleware(middleware.Protect(auth.AudienceAdmin, "channel:update")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("audit wrapper must preserve http.Flusher")
		}
		flusher.Flush()
		flushed = true
	})), writer)

	req := httptest.NewRequest(http.MethodPost, "/admin/v1/channels", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if !flushed {
		t.Fatal("expected streaming flush to reach wrapped handler")
	}
	if len(writer.events) != 1 {
		t.Fatalf("expected one audit event, got %d", len(writer.events))
	}
	if writer.events[0].ActorType != string(auth.PrincipalPlatformUser) || writer.events[0].ActorID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("expected authenticated audit actor, got %#v", writer.events[0])
	}
}
