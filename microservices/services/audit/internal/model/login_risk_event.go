package model

import "time"

// LoginRiskEventReason classifies why a login was flagged abnormal.
const (
	LoginRiskReasonNewIP     = "new_ip"
	LoginRiskReasonNewDevice = "new_device"
)

// LoginRiskEvent records one login that came from a new IP or new device
// relative to the user's previous successful login. Alerted/NotifiedAt mark
// whether the in-console alert was sent; Processed marks an admin follow-up.
type LoginRiskEvent struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	TenantID    uint       `gorm:"not null;default:1" json:"tenant_id"`
	UserID      uint       `gorm:"not null;index" json:"user_id"`
	Username    string     `gorm:"size:100;not null;default:''" json:"username"`
	IP          string     `gorm:"size:64;not null;default:''" json:"ip"`
	DeviceID    string     `gorm:"size:64;not null;default:''" json:"device_id"`
	Reason      string     `gorm:"size:16;not null" json:"reason"`
	Alerted     bool       `gorm:"not null;default:false" json:"alerted"`
	NotifiedAt  *time.Time `json:"notified_at,omitempty"`
	Processed   bool       `gorm:"not null;default:false;index" json:"processed"`
	ProcessedBy *uint      `json:"processed_by,omitempty"`
	ProcessedAt *time.Time `json:"processed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

func (LoginRiskEvent) TableName() string { return "login_risk_events" }
