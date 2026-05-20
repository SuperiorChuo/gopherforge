package model

import "time"

// ConsoleSession stores the web-console cookie session registry migrated from Python.
type ConsoleSession struct {
	SessionID        string     `gorm:"column:session_id;size:64;primaryKey" json:"session_id"`
	Username         string     `gorm:"size:128;not null;index" json:"username"`
	IssuedAt         time.Time  `gorm:"not null;index" json:"issued_at"`
	ExpiresAt        time.Time  `gorm:"not null;index" json:"expires_at"`
	RevokedAt        *time.Time `gorm:"index" json:"revoked_at"`
	LastSeenAt       *time.Time `json:"last_seen_at"`
	ClientIPHash     string     `gorm:"size:64" json:"client_ip_hash"`
	UserAgentHash    string     `gorm:"size:64" json:"user_agent_hash"`
	UserAgentPreview string     `gorm:"size:255" json:"user_agent_preview"`
	CreatedAt        time.Time  `gorm:"not null" json:"created_at"`
}

func (ConsoleSession) TableName() string {
	return "wm_console_session"
}
