package groups

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"ai-token/internal/ids"
)

var (
	ErrUnavailable           = errors.New("group service is unavailable")
	ErrInvalidRequest        = errors.New("invalid group request")
	ErrGroupNotFound         = errors.New("group is not found")
	ErrChannelNotFound       = errors.New("group channel is not found")
	ErrDefaultGroupProtected = errors.New("default group is protected")
	ErrGroupInUse            = errors.New("group is assigned to active tokens")
	ErrMonitorNotFound       = errors.New("model monitor is not found")
	ErrMonitorInUse          = errors.New("group already has a model monitor")
	ErrMonitorBusy           = errors.New("model monitor is already probing")
	ErrMonitorGroupInactive  = errors.New("model monitor group is inactive")
	ErrMonitorModeInvalid    = errors.New("active probing is not enabled for this monitor")
)

const (
	StatusActive   = "active"
	StatusDisabled = "disabled"

	BillingPrepaid = "prepaid"
	BillingFree    = "free"

	MeteringToken        = "token"
	MeteringImageCount   = "image_count"
	MeteringVideoSeconds = "video_seconds"
	MeteringVideoRequest = "video_request"
)

type ChannelSummary struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Status   string `json:"status"`
}

type Summary struct {
	ID           string           `json:"id"`
	Code         string           `json:"code"`
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	Status       string           `json:"status"`
	Multiplier   string           `json:"multiplier"`
	RPMLimit     int              `json:"rpm_limit"`
	BillingType  string           `json:"billing_type"`
	MeteringMode string           `json:"metering_mode"`
	Priority     int              `json:"priority"`
	Channels     []ChannelSummary `json:"channels"`
	Models       []string         `json:"models"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
}

type TokenGroupSummary struct {
	ID           string   `json:"id"`
	Code         string   `json:"code"`
	Name         string   `json:"name"`
	Multiplier   string   `json:"multiplier"`
	BillingType  string   `json:"billing_type"`
	MeteringMode string   `json:"metering_mode"`
	Status       string   `json:"status"`
	Models       []string `json:"models"`
}

type TokenGroupLister interface {
	ListTokenGroups(context.Context) ([]TokenGroupSummary, error)
}

type ModelMonitor struct {
	ID                   string     `json:"id"`
	GroupID              string     `json:"group_id"`
	GroupCode            string     `json:"group_code"`
	GroupName            string     `json:"group_name"`
	Name                 string     `json:"name"`
	SelectionMode        string     `json:"selection_mode"`
	Mode                 string     `json:"mode"`
	PrimaryModel         string     `json:"primary_model"`
	ProbeIntervalSeconds int        `json:"probe_interval_seconds"`
	RecentRequestLimit   int        `json:"recent_request_limit"`
	Enabled              bool       `json:"enabled"`
	ModelNames           []string   `json:"model_names"`
	AvailableModels      []string   `json:"available_models"`
	LastProbeStartedAt   *time.Time `json:"last_probe_started_at,omitempty"`
	LastProbeFinishedAt  *time.Time `json:"last_probe_finished_at,omitempty"`
	LastProbeStatus      string     `json:"last_probe_status"`
	LastProbeError       string     `json:"last_probe_error,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type ModelMonitorMutation struct {
	GroupID              string   `json:"group_id"`
	Name                 string   `json:"name"`
	SelectionMode        string   `json:"selection_mode"`
	ModelNames           []string `json:"model_names"`
	Mode                 string   `json:"mode"`
	PrimaryModel         string   `json:"primary_model"`
	ProbeIntervalSeconds int      `json:"probe_interval_seconds"`
	RecentRequestLimit   int      `json:"recent_request_limit"`
	Enabled              bool     `json:"enabled"`
}

type ModelMonitorService interface {
	ListAdminModelMonitors(context.Context) ([]ModelMonitor, error)
	CreateAdminModelMonitor(context.Context, string, ModelMonitorMutation) (ModelMonitor, error)
	UpdateAdminModelMonitor(context.Context, string, string, ModelMonitorMutation) (ModelMonitor, error)
	DeleteAdminModelMonitor(context.Context, string, string) error
	ClaimDueActiveModelMonitor(context.Context) (*ModelMonitor, error)
	ClaimActiveModelMonitor(context.Context, string) (*ModelMonitor, error)
	CompleteActiveModelMonitor(context.Context, string, string, string) error
}

type Mutation struct {
	Code         string   `json:"code"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Status       string   `json:"status"`
	Multiplier   string   `json:"multiplier"`
	RPMLimit     int      `json:"rpm_limit"`
	BillingType  string   `json:"billing_type"`
	MeteringMode string   `json:"metering_mode"`
	Priority     int      `json:"priority"`
	ChannelIDs   []string `json:"channel_ids"`
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
	m.MeteringMode = strings.ToLower(strings.TrimSpace(m.MeteringMode))
	if m.Status == "" {
		m.Status = StatusActive
	}
	if m.Multiplier == "" {
		m.Multiplier = "1"
	}
	if m.BillingType == "" {
		m.BillingType = BillingPrepaid
	}
	if m.MeteringMode == "" {
		m.MeteringMode = MeteringToken
	}
	if !validCode(m.Code) || m.Name == "" || len(m.Name) > 128 || len(m.Description) > 1000 {
		return Mutation{}, ErrInvalidRequest
	}
	if !validStatus(m.Status) || !validBillingType(m.BillingType) || !validMeteringMode(m.MeteringMode) {
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

func validMeteringMode(value string) bool {
	switch value {
	case MeteringToken, MeteringImageCount, MeteringVideoSeconds, MeteringVideoRequest:
		return true
	default:
		return false
	}
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

const (
	MonitorSelectionAll      = "all"
	MonitorSelectionSelected = "selected"
	MonitorModePassive       = "passive"
	MonitorModeActive        = "active"
	MonitorProbeSuccess      = "success"
	MonitorProbeFailed       = "failed"
	MonitorProbeSkipped      = "skipped"
)

func (m ModelMonitorMutation) validate() (ModelMonitorMutation, error) {
	m.GroupID = strings.TrimSpace(m.GroupID)
	m.Name = strings.TrimSpace(m.Name)
	m.SelectionMode = strings.ToLower(strings.TrimSpace(m.SelectionMode))
	m.Mode = strings.ToLower(strings.TrimSpace(m.Mode))
	m.PrimaryModel = strings.TrimSpace(m.PrimaryModel)
	m.ModelNames = cleanModelNames(m.ModelNames)
	if m.SelectionMode == "" {
		m.SelectionMode = MonitorSelectionAll
	}
	if m.Mode == "" {
		m.Mode = MonitorModePassive
	}
	if m.ProbeIntervalSeconds == 0 {
		m.ProbeIntervalSeconds = 300
	}
	if m.RecentRequestLimit == 0 {
		m.RecentRequestLimit = 60
	}
	if m.GroupID == "" || !ids.Valid(m.GroupID) || m.Name == "" || len(m.Name) > 128 {
		return ModelMonitorMutation{}, ErrInvalidRequest
	}
	if len(m.PrimaryModel) > 256 {
		return ModelMonitorMutation{}, ErrInvalidRequest
	}
	if m.SelectionMode != MonitorSelectionAll && m.SelectionMode != MonitorSelectionSelected {
		return ModelMonitorMutation{}, ErrInvalidRequest
	}
	if m.Mode != MonitorModePassive && m.Mode != MonitorModeActive {
		return ModelMonitorMutation{}, ErrInvalidRequest
	}
	if m.ProbeIntervalSeconds < 60 || m.ProbeIntervalSeconds > 86400 {
		return ModelMonitorMutation{}, ErrInvalidRequest
	}
	if m.RecentRequestLimit != 30 && m.RecentRequestLimit != 60 && m.RecentRequestLimit != 120 {
		return ModelMonitorMutation{}, ErrInvalidRequest
	}
	if m.SelectionMode == MonitorSelectionSelected && len(m.ModelNames) == 0 {
		return ModelMonitorMutation{}, ErrInvalidRequest
	}
	if m.SelectionMode == MonitorSelectionSelected && m.PrimaryModel != "" {
		foundPrimary := false
		for _, modelName := range m.ModelNames {
			if modelName == m.PrimaryModel {
				foundPrimary = true
				break
			}
		}
		if !foundPrimary {
			return ModelMonitorMutation{}, ErrInvalidRequest
		}
	}
	if m.SelectionMode == MonitorSelectionAll {
		m.ModelNames = nil
	}
	return m, nil
}

func cleanModelNames(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 256 {
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
