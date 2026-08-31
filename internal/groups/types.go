package groups

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

var (
	ErrUnavailable           = errors.New("group service is unavailable")
	ErrInvalidRequest        = errors.New("invalid group request")
	ErrGroupNotFound         = errors.New("group is not found")
	ErrChannelNotFound       = errors.New("group channel is not found")
	ErrDefaultGroupProtected = errors.New("default group is protected")
	ErrGroupInUse            = errors.New("group is assigned to active tokens")
)

const (
	StatusActive   = "active"
	StatusDisabled = "disabled"

	BillingPrepaid = "prepaid"
	BillingFree    = "free"
)

type ChannelSummary struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Status   string `json:"status"`
}

type Summary struct {
	ID          string           `json:"id"`
	Code        string           `json:"code"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Status      string           `json:"status"`
	Multiplier  string           `json:"multiplier"`
	RPMLimit    int              `json:"rpm_limit"`
	BillingType string           `json:"billing_type"`
	Priority    int              `json:"priority"`
	Channels    []ChannelSummary `json:"channels"`
	Models      []string         `json:"models"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

type TokenGroupSummary struct {
	ID          string   `json:"id"`
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Multiplier  string   `json:"multiplier"`
	BillingType string   `json:"billing_type"`
	Status      string   `json:"status"`
	Models      []string `json:"models"`
}

type TokenGroupLister interface {
	ListTokenGroups(context.Context) ([]TokenGroupSummary, error)
}

type Mutation struct {
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Multiplier  string   `json:"multiplier"`
	RPMLimit    int      `json:"rpm_limit"`
	BillingType string   `json:"billing_type"`
	Priority    int      `json:"priority"`
	ChannelIDs  []string `json:"channel_ids"`
}

type Service interface {
	List(context.Context) ([]Summary, error)
	Create(context.Context, string, Mutation) (Summary, error)
	Update(context.Context, string, string, Mutation) (Summary, error)
	Delete(context.Context, string, string) error
}

func NewSQLService(db *sql.DB) (*SQLService, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &SQLService{db: db}, nil
}

type SQLService struct {
	db *sql.DB
}

func (m Mutation) validate() (Mutation, error) {
	m.Code = strings.ToLower(strings.TrimSpace(m.Code))
	m.Name = strings.TrimSpace(m.Name)
	m.Description = strings.TrimSpace(m.Description)
	m.Status = strings.ToLower(strings.TrimSpace(m.Status))
	m.Multiplier = strings.TrimSpace(m.Multiplier)
	m.BillingType = strings.ToLower(strings.TrimSpace(m.BillingType))
	if m.Status == "" {
		m.Status = StatusActive
	}
	if m.Multiplier == "" {
		m.Multiplier = "1"
	}
	if m.BillingType == "" {
		m.BillingType = BillingPrepaid
	}
	if !validCode(m.Code) || m.Name == "" || len(m.Name) > 128 || len(m.Description) > 1000 {
		return Mutation{}, ErrInvalidRequest
	}
	if !validStatus(m.Status) || !validBillingType(m.BillingType) {
		return Mutation{}, ErrInvalidRequest
	}
	if normalized, ok := normalizeMultiplier(m.Multiplier); ok {
		m.Multiplier = normalized
	} else {
		return Mutation{}, ErrInvalidRequest
	}
	if m.RPMLimit < 0 || m.RPMLimit > 10_000_000 || m.Priority < 0 || m.Priority > 10_000 {
		return Mutation{}, ErrInvalidRequest
	}
	m.ChannelIDs = cleanIDs(m.ChannelIDs)
	return m, nil
}

func validCode(value string) bool {
	if len(value) < 1 || len(value) > 64 || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, char := range value[1:] {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' && char != '-' {
			return false
		}
	}
	return true
}

func validStatus(value string) bool {
	return value == StatusActive || value == StatusDisabled
}

func validBillingType(value string) bool {
	return value == BillingPrepaid || value == BillingFree
}

func cleanIDs(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
