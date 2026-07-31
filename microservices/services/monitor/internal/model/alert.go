package model

import "time"

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
