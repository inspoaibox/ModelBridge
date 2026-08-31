package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"ai-token/internal/tokens"
)

// RequestThrottle applies a transactional request quota to unauthenticated
// entry points. Subjects are salted before persistence.
type RequestThrottle struct {
	db          *sql.DB
	hasher      *tokens.Hasher
	maxRequests int
	window      time.Duration
	lockFor     time.Duration
	now         func() time.Time
}

func NewRequestThrottle(db *sql.DB, hasher *tokens.Hasher, maxRequests int, window, lockFor time.Duration) (*RequestThrottle, error) {
	if db == nil || hasher == nil || maxRequests < 1 || window <= 0 || lockFor <= 0 {
		return nil, errors.New("invalid request throttle configuration")
	}
	return &RequestThrottle{
		db: db, hasher: hasher, maxRequests: maxRequests,
		window: window, lockFor: lockFor, now: time.Now,
	}, nil
}

func (t *RequestThrottle) Allow(ctx context.Context, scope, subject string) (bool, error) {
	if t == nil || t.db == nil || t.hasher == nil {
		return false, errors.New("request throttle is not configured")
	}
	scope, subject = strings.TrimSpace(scope), strings.TrimSpace(subject)
	if scope == "" || subject == "" || len(scope)+len(subject) > 512 {
		return false, errors.New("invalid request throttle subject")
	}

	tx, err := t.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	key := t.hasher.Digest(scope + ":" + subject)
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key); err != nil {
		return false, err
	}

	var (
		count      int
		first      time.Time
		lockedTill sql.NullTime
	)
	err = tx.QueryRowContext(ctx, `
		SELECT failure_count, first_failed_at, locked_until
		FROM login_throttles
		WHERE subject_hash = $1
		FOR UPDATE
	`, key).Scan(&count, &first, &lockedTill)
	now := t.now()
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO login_throttles (
				subject_hash, failure_count, first_failed_at, locked_until, updated_at
			) VALUES ($1, 1, $2, NULL, $2)
		`, key, now); err != nil {
			return false, err
		}
		return true, tx.Commit()
	}
	if err != nil {
		return false, err
	}
	if lockedTill.Valid && lockedTill.Time.After(now) {
		return false, tx.Commit()
	}
	if first.Before(now.Add(-t.window)) {
		_, err = tx.ExecContext(ctx, `
			UPDATE login_throttles
			SET failure_count = 1, first_failed_at = $2, locked_until = NULL, updated_at = $2
			WHERE subject_hash = $1
		`, key, now)
		if err != nil {
			return false, err
		}
		return true, tx.Commit()
	}
	if count >= t.maxRequests {
		_, err = tx.ExecContext(ctx, `
			UPDATE login_throttles
			SET locked_until = $2, updated_at = $3
			WHERE subject_hash = $1
		`, key, now.Add(t.lockFor), now)
		if err != nil {
			return false, err
		}
		return false, tx.Commit()
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE login_throttles
		SET failure_count = failure_count + 1, updated_at = $2
		WHERE subject_hash = $1
	`, key, now)
	if err != nil {
		return false, err
	}
	return true, tx.Commit()
}
