package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Event struct {
	ID            string `json:"id"`
	RequestID     string `json:"request_id"`
	ActorType     string `json:"actor_type"`
	ActorID       string `json:"actor_id,omitempty"`
	TenantID      string `json:"tenant_id,omitempty"`
	Action        string `json:"action"`
	ResourceType  string `json:"resource_type"`
	ResourceID    string `json:"resource_id,omitempty"`
	Result        string `json:"result"`
	Before        any    `json:"before,omitempty"`
	After         any    `json:"after,omitempty"`
	IPHash        string `json:"ip_hash,omitempty"`
	UserAgentHash string `json:"user_agent_hash,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

type Writer interface {
	Append(ctx context.Context, event Event) error
}

type Query struct {
	Limit        int
	Offset       int
	Action       string
	ResourceType string
	Result       string
	Search       string
	From         *time.Time
	To           *time.Time
}

type Record struct {
	Event
	CreatedAt time.Time `json:"created_at"`
}

type Report struct {
	Records []Record `json:"records"`
	Total   int64    `json:"total"`
	Limit   int      `json:"limit"`
	Offset  int      `json:"offset"`
}

type Reader interface {
	List(context.Context, Query) (Report, error)
}

type SQLWriter struct {
	db *sql.DB
}

func NewSQLWriter(db *sql.DB) (*SQLWriter, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &SQLWriter{db: db}, nil
}

func (w *SQLWriter) Append(ctx context.Context, event Event) error {
	if w == nil || w.db == nil {
		return errors.New("audit writer is not configured")
	}
	if event.ID == "" || event.RequestID == "" || event.ActorType == "" ||
		event.Action == "" || event.ResourceType == "" || event.Result == "" {
		return errors.New("audit event is missing required fields")
	}

	before, err := marshalOptional(event.Before)
	if err != nil {
		return fmt.Errorf("marshal audit before: %w", err)
	}
	after, err := marshalOptional(event.After)
	if err != nil {
		return fmt.Errorf("marshal audit after: %w", err)
	}

	_, err = w.db.ExecContext(ctx, `
		INSERT INTO audit_events (
			id, request_id, actor_type, actor_id, tenant_id, action,
			resource_type, resource_id, result, before_json, after_json,
			ip_hash, user_agent_hash, reason
		) VALUES (
			$1, $2, $3, NULLIF($4, '')::uuid, NULLIF($5, '')::uuid, $6,
			$7, NULLIF($8, '')::uuid, $9, $10, $11, $12, $13, $14
		)
	`, event.ID, event.RequestID, event.ActorType, event.ActorID, event.TenantID,
		event.Action, event.ResourceType, event.ResourceID, event.Result, before, after,
		event.IPHash, event.UserAgentHash, event.Reason)
	return err
}

func marshalOptional(value any) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	return json.Marshal(value)
}

func (w *SQLWriter) List(ctx context.Context, query Query) (Report, error) {
	if w == nil || w.db == nil {
		return Report{}, errors.New("audit writer is not configured")
	}
	if query.Limit <= 0 || query.Limit > 200 {
		query.Limit = 50
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	clauses := []string{"1 = 1"}
	args := make([]any, 0, 8)
	add := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}
	if query.Action = normalize(query.Action); query.Action != "" {
		add("action = $%d", query.Action)
	}
	if query.ResourceType = normalize(query.ResourceType); query.ResourceType != "" {
		add("resource_type = $%d", query.ResourceType)
	}
	if query.Result = normalize(query.Result); query.Result != "" {
		add("result = $%d", query.Result)
	}
	if query.Search = normalize(query.Search); query.Search != "" {
		pattern := "%" + query.Search + "%"
		start := len(args) + 1
		args = append(args, pattern, pattern, pattern, pattern)
		clauses = append(clauses, fmt.Sprintf("(action ILIKE $%d OR resource_type ILIKE $%d OR resource_id::text ILIKE $%d OR actor_id::text ILIKE $%d)", start, start+1, start+2, start+3))
	}
	if query.From != nil {
		add("created_at >= $%d", *query.From)
	}
	if query.To != nil {
		add("created_at < $%d", *query.To)
	}
	where := strings.Join(clauses, " AND ")
	var total int64
	if err := w.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_events WHERE "+where, args...).Scan(&total); err != nil {
		return Report{}, err
	}
	listArgs := append([]any{}, args...)
	limitPos := len(listArgs) + 1
	offsetPos := len(listArgs) + 2
	listArgs = append(listArgs, query.Limit, query.Offset)
	rows, err := w.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT id::text, request_id, actor_type, COALESCE(actor_id::text, ''),
		       COALESCE(tenant_id::text, ''), action, resource_type,
		       COALESCE(resource_id::text, ''), result, before_json, after_json,
		       COALESCE(ip_hash, ''), COALESCE(user_agent_hash, ''),
		       COALESCE(reason, ''), created_at
		FROM audit_events
		WHERE %s
		ORDER BY created_at DESC, id DESC
		LIMIT $%d OFFSET $%d`, where, limitPos, offsetPos), listArgs...)
	if err != nil {
		return Report{}, err
	}
	defer rows.Close()
	records := make([]Record, 0, query.Limit)
	for rows.Next() {
		var record Record
		var before, after []byte
		if err := rows.Scan(&record.ID, &record.RequestID, &record.ActorType, &record.ActorID,
			&record.TenantID, &record.Action, &record.ResourceType, &record.ResourceID,
			&record.Result, &before, &after, &record.IPHash, &record.UserAgentHash,
			&record.Reason, &record.CreatedAt); err != nil {
			return Report{}, err
		}
		if len(before) > 0 {
			if err := json.Unmarshal(before, &record.Before); err != nil {
				return Report{}, err
			}
		}
		if len(after) > 0 {
			if err := json.Unmarshal(after, &record.After); err != nil {
				return Report{}, err
			}
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return Report{}, err
	}
	return Report{Records: records, Total: total, Limit: query.Limit, Offset: query.Offset}, nil
}

func normalize(value string) string {
	return strings.TrimSpace(value)
}
