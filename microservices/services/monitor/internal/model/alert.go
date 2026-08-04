package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// NotifyChannelList is the set of alert notification channels a rule opts
// into. Stored as a JSON-array TEXT column, but serializes to/from a []string
// so the API and frontend see a plain array. Empty means "use every channel
// that is configured".
type NotifyChannelList []string

func (l NotifyChannelList) Value() (driver.Value, error) {
	return json.Marshal([]string(l))
}

func (l *NotifyChannelList) Scan(value any) error {
	if value == nil {
		*l = NotifyChannelList{}
		return nil
	}
	raw, ok := value.([]byte)
	if !ok {
		if s, ok := value.(string); ok {
			raw = []byte(s)
		} else {
			return errors.New("NotifyChannelList scan: unexpected value type")
		}
	}
	return json.Unmarshal(raw, (*[]string)(l))
}

func (l NotifyChannelList) MarshalJSON() ([]byte, error) {
	return json.Marshal([]string(l))
}

func (l *NotifyChannelList) UnmarshalJSON(b []byte) error {
	return json.Unmarshal(b, (*[]string)(l))
}

// MonitorAlertRule stores an alert condition and its durable evaluation state.
type MonitorAlertRule struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	Name            string     `gorm:"size:100;not null;uniqueIndex" json:"name"`
	Metric          string     `gorm:"size:80;not null;index" json:"metric"`
	Operator        string     `gorm:"size:8;not null" json:"operator"`
	Threshold       float64    `gorm:"not null" json:"threshold"`
	DurationSeconds int64      `gorm:"not null;default:0" json:"duration_seconds"`
	Severity        string     `gorm:"size:16;not null;default:warning;index" json:"severity"`
	Enabled         bool       `gorm:"not null;default:true;index" json:"enabled"`
	NotifyOnResolve bool       `gorm:"not null;default:true" json:"notify_on_resolve"`
	NotifyChannels  NotifyChannelList `gorm:"type:text;not null;default:'[]'" json:"notify_channels"`
	SilenceUntil    *time.Time `json:"silence_until"`
	State           string     `gorm:"size:16;not null;default:ok;index" json:"state"`
	PendingSince    *time.Time `json:"pending_since"`
	FiringSince     *time.Time `json:"firing_since"`
	LastValue       *float64   `json:"last_value"`
	LastEvaluatedAt *time.Time `json:"last_evaluated_at"`
	LastError       string     `gorm:"size:1000;not null;default:''" json:"last_error"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (MonitorAlertRule) TableName() string {
	return "monitor_alert_rules"
}

// MonitorAlertEvent is an immutable firing or resolved transition. Notification
// delivery metadata is updated after the state transaction commits.
type MonitorAlertEvent struct {
	ID           uint64     `gorm:"primaryKey" json:"id"`
	RuleID       *uint      `gorm:"index" json:"rule_id"`
	RuleName     string     `gorm:"size:100;not null" json:"rule_name"`
	Metric       string     `gorm:"size:80;not null;index" json:"metric"`
	Severity     string     `gorm:"size:16;not null;index" json:"severity"`
	Status       string     `gorm:"size:16;not null;index" json:"status"`
	Value        float64    `gorm:"not null" json:"value"`
	Threshold    float64    `gorm:"not null" json:"threshold"`
	Message      string     `gorm:"size:1000;not null" json:"message"`
	NotifyStatus string     `gorm:"size:16;not null;default:pending;index" json:"notify_status"`
	NotifyError  string     `gorm:"size:1000;not null;default:''" json:"notify_error"`
	NotifiedAt   *time.Time `json:"notified_at"`
	CreatedAt    time.Time  `gorm:"index" json:"created_at"`
}

func (MonitorAlertEvent) TableName() string {
	return "monitor_alert_events"
}

type MonitorAlertSummary struct {
	Total     int64     `json:"total"`
	Enabled   int64     `json:"enabled"`
	Firing    int64     `json:"firing"`
	Pending   int64     `json:"pending"`
	Error     int64     `json:"error"`
	CheckedAt time.Time `json:"checked_at"`
}
