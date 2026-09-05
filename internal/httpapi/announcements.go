package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"ai-token/internal/announcements"
	"ai-token/internal/auth"
)

type announcementPayload struct {
	Title       string `json:"title"`
	Content     string `json:"content"`
	EffectiveAt string `json:"effective_at"`
	ExpiresAt   string `json:"expires_at"`
	Enabled     *bool  `json:"enabled"`
}

func adminAnnouncementListHandler(service announcements.AdminService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ANNOUNCEMENTS_UNAVAILABLE"})
			return
		}
		items, err := service.List(r.Context())
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ANNOUNCEMENTS_UNAVAILABLE"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"announcements": items})
	})
}

func adminAnnouncementCreateHandler(service announcements.AdminService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok || principal.ID == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		var payload announcementPayload
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_ANNOUNCEMENT"})
			return
		}
		request, err := parseAnnouncementPayload(payload)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_ANNOUNCEMENT"})
			return
		}
		item, err := service.Create(r.Context(), principal.ID, request)
		if err != nil {
			writeAnnouncementError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, item)
	})
}

func adminAnnouncementUpdateHandler(service announcements.AdminService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload announcementPayload
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_ANNOUNCEMENT"})
			return
		}
		request, err := parseAnnouncementPayload(payload)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_ANNOUNCEMENT"})
			return
		}
		item, err := service.Update(r.Context(), r.PathValue("announcementID"), request)
		if err != nil {
			writeAnnouncementError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	})
}

func adminAnnouncementDeleteHandler(service announcements.AdminService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := service.Delete(r.Context(), r.PathValue("announcementID")); err != nil {
			writeAnnouncementError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	})
}

func adminAnnouncementRecipientsHandler(service announcements.AdminService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		items, err := service.ListRecipients(r.Context(), r.PathValue("announcementID"))
		if err != nil {
			writeAnnouncementError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"recipients": items})
	})
}

func consoleAnnouncementListHandler(service announcements.ConsoleService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok || principal.ID == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		items, err := service.ListForUser(r.Context(), principal.ID)
		if err != nil {
			writeAnnouncementError(w, err)
			return
		}
		unread := 0
		for _, item := range items {
			if item.ReadAt == nil {
				unread++
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"announcements": items, "unread_count": unread})
	})
}

func consoleAnnouncementReadHandler(service announcements.ConsoleService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok || principal.ID == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		if err := service.MarkRead(r.Context(), principal.ID, r.PathValue("announcementID")); err != nil {
			writeAnnouncementError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "read"})
	})
}

func parseAnnouncementPayload(payload announcementPayload) (announcements.CreateRequest, error) {
	effectiveAt, err := time.Parse(time.RFC3339, strings.TrimSpace(payload.EffectiveAt))
	if err != nil {
		return announcements.CreateRequest{}, err
	}
	var expiresAt *time.Time
	if value := strings.TrimSpace(payload.ExpiresAt); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return announcements.CreateRequest{}, err
		}
		expiresAt = &parsed
	}
	enabled := true
	if payload.Enabled != nil {
		enabled = *payload.Enabled
	}
	return announcements.CreateRequest{Title: payload.Title, Content: payload.Content, EffectiveAt: effectiveAt, ExpiresAt: expiresAt, Enabled: enabled}, nil
}

func writeAnnouncementError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, announcements.ErrInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_ANNOUNCEMENT"})
	case errors.Is(err, announcements.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "ANNOUNCEMENT_NOT_FOUND"})
	case errors.Is(err, announcements.ErrUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ANNOUNCEMENTS_UNAVAILABLE"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ANNOUNCEMENTS_UNAVAILABLE"})
	}
}
