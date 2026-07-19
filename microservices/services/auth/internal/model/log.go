package model

import "time"

// AuditLog stores business audit events.
//
// Copied from the monolith's model/log.go; the unrelated OperationLog and
// LoginLog structs were trimmed because the auth service does not use them.
type AuditLog struct {
	ID         uint           `gorm:"primaryKey" json:"id"`
	ActorType  string         `gorm:"size:64;default:operator;index" json:"actor_type"`
	ActorID    string         `gorm:"size:128;default:web-console;index" json:"actor_id"`
	Action     string         `gorm:"size:128;not null;index" json:"action"`
	TargetType string         `gorm:"size:64;not null;index" json:"target_type"`
	TargetID   string         `gorm:"size:128;not null;index" json:"target_id"`
	BeforeJSON map[string]any `gorm:"column:before_json;type:json;serializer:json" json:"before"`
	AfterJSON  map[string]any `gorm:"column:after_json;type:json;serializer:json" json:"after"`
	Summary    string         `gorm:"type:text" json:"summary"`
	CreatedAt  time.Time      `gorm:"index" json:"created_at"`
}

func (AuditLog) TableName() string {
	return "audit_logs"
}
