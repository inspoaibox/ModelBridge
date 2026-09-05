package announcements

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"ai-token/internal/ids"
)

var (
	ErrUnavailable = errors.New("announcement service is unavailable")
	ErrInvalid     = errors.New("invalid announcement request")
	ErrNotFound    = errors.New("announcement is not found")
)

type Announcement struct {
	ID               string     `json:"id"`
	Title            string     `json:"title"`
	Content          string     `json:"content"`
	EffectiveAt      time.Time  `json:"effective_at"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	Enabled          bool       `json:"enabled"`
	Status           string     `json:"status"`
	CreatedBy        string     `json:"created_by"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	TotalRecipients  int64      `json:"total_recipients"`
	ReadRecipients   int64      `json:"read_recipients"`
	UnreadRecipients int64      `json:"unread_recipients"`
	ReadAt           *time.Time `json:"read_at,omitempty"`
}

type Recipient struct {
	UserID      string     `json:"user_id"`
	Email       string     `json:"email"`
	DisplayName string     `json:"display_name"`
	UserStatus  string     `json:"user_status"`
	DeliveredAt time.Time  `json:"delivered_at"`
	ReadAt      *time.Time `json:"read_at,omitempty"`
}

type CreateRequest struct {
	Title       string
	Content     string
	EffectiveAt time.Time
	ExpiresAt   *time.Time
	Enabled     bool
}

type AdminService interface {
	List(context.Context) ([]Announcement, error)
	Create(context.Context, string, CreateRequest) (Announcement, error)
	Update(context.Context, string, CreateRequest) (Announcement, error)
	Delete(context.Context, string) error
	ListRecipients(context.Context, string) ([]Recipient, error)
}

type ConsoleService interface {
	ListForUser(context.Context, string) ([]Announcement, error)
	MarkRead(context.Context, string, string) error
}

type Service interface {
	AdminService
	ConsoleService
}

type SQLService struct{ db *sql.DB }

func NewSQLService(db *sql.DB) (*SQLService, error) {
	if db == nil {
		return nil, ErrUnavailable
	}
	return &SQLService{db: db}, nil
}

func (s *SQLService) List(ctx context.Context) ([]Announcement, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx, announcementSelect+` ORDER BY a.effective_at DESC, a.created_at DESC, a.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Announcement, 0)
	for rows.Next() {
		item, err := scanAnnouncement(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *SQLService) Create(ctx context.Context, actorID string, request CreateRequest) (Announcement, error) {
	if s == nil || s.db == nil {
		return Announcement{}, ErrUnavailable
	}
	request, err := normalizeRequest(request)
	if err != nil || !ids.Valid(strings.TrimSpace(actorID)) {
		return Announcement{}, ErrInvalid
	}
	id, err := ids.New()
	if err != nil {
		return Announcement{}, err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO announcements (id, title, content, effective_at, expires_at, enabled, created_by)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7::uuid)
	`, id, request.Title, request.Content, request.EffectiveAt, request.ExpiresAt, request.Enabled, actorID)
	if err != nil {
		return Announcement{}, err
	}
	return s.get(ctx, id)
}

func (s *SQLService) Update(ctx context.Context, announcementID string, request CreateRequest) (Announcement, error) {
	if s == nil || s.db == nil || !ids.Valid(strings.TrimSpace(announcementID)) {
		return Announcement{}, ErrInvalid
	}
	request, err := normalizeRequest(request)
	if err != nil {
		return Announcement{}, ErrInvalid
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE announcements
		SET title = $2, content = $3, effective_at = $4, expires_at = $5, enabled = $6, updated_at = now()
		WHERE id = $1::uuid
	`, announcementID, request.Title, request.Content, request.EffectiveAt, request.ExpiresAt, request.Enabled)
	if err != nil {
		return Announcement{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return Announcement{}, err
	}
	if affected != 1 {
		return Announcement{}, ErrNotFound
	}
	return s.get(ctx, announcementID)
}

func (s *SQLService) Delete(ctx context.Context, announcementID string) error {
	if s == nil || s.db == nil || !ids.Valid(strings.TrimSpace(announcementID)) {
		return ErrInvalid
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM announcements WHERE id = $1::uuid`, announcementID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLService) ListRecipients(ctx context.Context, announcementID string) ([]Recipient, error) {
	if s == nil || s.db == nil || !ids.Valid(strings.TrimSpace(announcementID)) {
		return nil, ErrInvalid
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT aus.user_id::text, u.email, u.display_name, u.status, aus.delivered_at, aus.read_at
		FROM announcement_user_states aus
		JOIN announcements a ON a.id = aus.announcement_id
		JOIN users u ON u.id = aus.user_id
		WHERE aus.announcement_id = $1::uuid
		ORDER BY aus.delivered_at DESC, aus.user_id
	`, announcementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Recipient, 0)
	for rows.Next() {
		var item Recipient
		var readAt sql.NullTime
		if err := rows.Scan(&item.UserID, &item.Email, &item.DisplayName, &item.UserStatus, &item.DeliveredAt, &readAt); err != nil {
			return nil, err
		}
		if readAt.Valid {
			item.ReadAt = &readAt.Time
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *SQLService) ListForUser(ctx context.Context, userID string) ([]Announcement, error) {
	if s == nil || s.db == nil || !ids.Valid(strings.TrimSpace(userID)) {
		return nil, ErrInvalid
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id::text, a.title, a.content, a.effective_at, a.expires_at, a.enabled,
		       a.created_by::text, a.created_at, a.updated_at,
		       aus.read_at
		FROM announcements a
		LEFT JOIN announcement_user_states aus
		  ON aus.announcement_id = a.id AND aus.user_id = $1::uuid
		WHERE a.enabled = true
		  AND a.effective_at <= now()
		  AND (a.expires_at IS NULL OR a.expires_at > now())
		ORDER BY a.effective_at DESC, a.created_at DESC, a.id DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Announcement, 0)
	for rows.Next() {
		var item Announcement
		var expiresAt, readAt sql.NullTime
		if err := rows.Scan(&item.ID, &item.Title, &item.Content, &item.EffectiveAt, &expiresAt, &item.Enabled, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt, &readAt); err != nil {
			return nil, err
		}
		if expiresAt.Valid {
			item.ExpiresAt = &expiresAt.Time
		}
		if readAt.Valid {
			item.ReadAt = &readAt.Time
		}
		item.Status = "active"
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range result {
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO announcement_user_states (announcement_id, user_id)
			VALUES ($1::uuid, $2::uuid)
			ON CONFLICT (announcement_id, user_id) DO NOTHING
		`, result[index].ID, userID); err != nil {
			return nil, err
		}
	}
	return s.ListForUserState(ctx, userID)
}

func (s *SQLService) ListForUserState(ctx context.Context, userID string) ([]Announcement, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id::text, a.title, a.content, a.effective_at, a.expires_at, a.enabled,
		       a.created_by::text, a.created_at, a.updated_at, aus.read_at
		FROM announcements a
		JOIN announcement_user_states aus ON aus.announcement_id = a.id AND aus.user_id = $1::uuid
		WHERE a.enabled = true AND a.effective_at <= now()
		  AND (a.expires_at IS NULL OR a.expires_at > now())
		ORDER BY a.effective_at DESC, a.created_at DESC, a.id DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Announcement, 0)
	for rows.Next() {
		var item Announcement
		var expiresAt, readAt sql.NullTime
		if err := rows.Scan(&item.ID, &item.Title, &item.Content, &item.EffectiveAt, &expiresAt, &item.Enabled, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt, &readAt); err != nil {
			return nil, err
		}
		if expiresAt.Valid {
			item.ExpiresAt = &expiresAt.Time
		}
		if readAt.Valid {
			item.ReadAt = &readAt.Time
		}
		item.Status = "active"
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *SQLService) MarkRead(ctx context.Context, userID, announcementID string) error {
	if s == nil || s.db == nil || !ids.Valid(strings.TrimSpace(userID)) || !ids.Valid(strings.TrimSpace(announcementID)) {
		return ErrInvalid
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE announcement_user_states
		SET read_at = COALESCE(read_at, now())
		WHERE user_id = $1::uuid AND announcement_id = $2::uuid
	`, userID, announcementID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrNotFound
	}
	return nil
}

const announcementSelect = `
	SELECT a.id::text, a.title, a.content, a.effective_at, a.expires_at, a.enabled,
	       a.created_by::text, a.created_at, a.updated_at,
	       COUNT(aus.user_id)::bigint,
	       COUNT(aus.user_id) FILTER (WHERE aus.read_at IS NOT NULL)::bigint
	FROM announcements a
	LEFT JOIN announcement_user_states aus ON aus.announcement_id = a.id
	GROUP BY a.id
`

type scanner interface{ Scan(...any) error }

func scanAnnouncement(row scanner) (Announcement, error) {
	var item Announcement
	var expiresAt sql.NullTime
	if err := row.Scan(&item.ID, &item.Title, &item.Content, &item.EffectiveAt, &expiresAt, &item.Enabled, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt, &item.TotalRecipients, &item.ReadRecipients); err != nil {
		return Announcement{}, err
	}
	if expiresAt.Valid {
		item.ExpiresAt = &expiresAt.Time
	}
	item.UnreadRecipients = item.TotalRecipients - item.ReadRecipients
	item.Status = statusFor(item.Enabled, item.EffectiveAt, item.ExpiresAt, time.Now())
	return item, nil
}

func (s *SQLService) get(ctx context.Context, announcementID string) (Announcement, error) {
	row := s.db.QueryRowContext(ctx, announcementSelect+` HAVING a.id = $1::uuid`, announcementID)
	item, err := scanAnnouncement(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Announcement{}, ErrNotFound
	}
	return item, err
}

func normalizeRequest(request CreateRequest) (CreateRequest, error) {
	request.Title = strings.TrimSpace(request.Title)
	request.Content = strings.TrimSpace(request.Content)
	if request.Title == "" || len(request.Title) > 200 || request.Content == "" || len(request.Content) > 50000 || request.EffectiveAt.IsZero() {
		return CreateRequest{}, ErrInvalid
	}
	if request.ExpiresAt != nil && !request.ExpiresAt.After(request.EffectiveAt) {
		return CreateRequest{}, ErrInvalid
	}
	return request, nil
}

func statusFor(enabled bool, effectiveAt time.Time, expiresAt *time.Time, now time.Time) string {
	if !enabled {
		return "disabled"
	}
	if now.Before(effectiveAt) {
		return "scheduled"
	}
	if expiresAt != nil && !now.Before(*expiresAt) {
		return "expired"
	}
	return "active"
}

var _ AdminService = (*SQLService)(nil)
var _ ConsoleService = (*SQLService)(nil)
