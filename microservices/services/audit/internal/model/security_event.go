package localmodel

import "time"

// SecurityEvent records one detected abnormal pattern in the audit trail
// (high-volume writes, permission storms, failure bursts). NotifiedAt non-nil
// means the in-console alert has been sent (dedupe basis).
type SecurityEvent struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	TenantID   uint       `gorm:"not null;default:1;index" json:"tenant_id"`
	Rule       string     `gorm:"size:64;not null;index" json:"rule"`
	Severity   string     `gorm:"size:16;not null;default:'warning'" json:"severity"`
	Summary    string     `gorm:"size:512;not null" json:"summary"`
	ActorID    string     `gorm:"size:128;not null;default:''" json:"actor_id"`
	ActorType  string     `gorm:"size:32;not null;default:'operator'" json:"actor_type"`
	Target     string     `gorm:"size:128;not null;default:''" json:"target"`
	OccurredAt time.Time  `json:"occurred_at"`
	NotifiedAt *time.Time `json:"notified_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (SecurityEvent) TableName() string { return "security_events" }
