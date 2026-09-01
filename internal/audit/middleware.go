package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strings"

	"ai-token/internal/auth"
	"ai-token/internal/ids"
)

// HTTPMiddleware records authenticated mutations after the response is
// written. It intentionally stores no request body, headers, token or secret.
func HTTPMiddleware(next http.Handler, writer Writer) http.Handler {
	if next == nil || writer == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isAuditableMutation(r) {
			next.ServeHTTP(w, r)
			return
		}
		wrapped := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(wrapped, r)
		requestID := strings.TrimSpace(wrapped.Header().Get("X-Request-ID"))
		if requestID == "" {
			requestID = strings.TrimSpace(r.Header.Get("X-Request-ID"))
		}
		if requestID == "" {
			requestID, _ = ids.New()
		}
		actorType, actorID, tenantID := "anonymous", "", ""
		if principal, ok := auth.PrincipalFromContext(r.Context()); ok {
			actorType = string(principal.Type)
			actorID = principal.ID
			tenantID = principal.TenantID
		}
		eventID, err := ids.New()
		if err != nil {
			return
		}
		resourceType := auditResource(r.URL.Path)
		resourceID := r.PathValue("userID")
		if resourceID == "" {
			resourceID = r.PathValue("channelID")
		}
		if resourceID == "" {
			resourceID = r.PathValue("groupID")
		}
		if resourceID == "" {
			resourceID = r.PathValue("tokenID")
		}
		if resourceID == "" {
			resourceID = r.PathValue("projectID")
		}
		if resourceID == "" {
			resourceID = r.PathValue("tenantID")
		}
		if !looksLikeUUID(resourceID) {
			resourceID = ""
		}
		result := "failed"
		if wrapped.status >= 200 && wrapped.status < 400 {
			result = "success"
		} else if wrapped.status == http.StatusUnauthorized || wrapped.status == http.StatusForbidden || wrapped.status == http.StatusNotFound {
			result = "denied"
		}
		_ = writer.Append(context.Background(), Event{
			ID:            eventID,
			RequestID:     requestID,
			ActorType:     actorType,
			ActorID:       actorID,
			TenantID:      tenantID,
			Action:        strings.ToLower(r.Method) + " " + resourceType,
			ResourceType:  resourceType,
			ResourceID:    resourceID,
			Result:        result,
			IPHash:        hashValue(clientIP(r)),
			UserAgentHash: hashValue(r.UserAgent()),
			Reason:        "http_status=" + http.StatusText(wrapped.status),
		})
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

// Preserve SSE flushing when audit wraps the application handler. Without this
// delegation, a streaming relay response is buffered until completion.
func (w *statusWriter) Flush() {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func isAuditableMutation(r *http.Request) bool {
	if r == nil {
		return false
	}
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return strings.HasPrefix(r.URL.Path, "/admin/") || strings.HasPrefix(r.URL.Path, "/console/") || strings.HasPrefix(r.URL.Path, "/v1/")
	default:
		return false
	}
}

func auditResource(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 3 && parts[0] == "admin" && parts[1] == "v1" {
		if parts[2] == "auth" {
			return "auth"
		}
		return strings.TrimSuffix(parts[2], "s")
	}
	if strings.HasPrefix(path, "/v1/") {
		return "relay"
	}
	return "console"
}

func hashValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func clientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func looksLikeUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, char := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if char != '-' {
				return false
			}
			continue
		}
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			return false
		}
	}
	return true
}
