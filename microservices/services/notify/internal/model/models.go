package model

import "time"

// Template is a message template keyed by code (e.g. ticket.overdue).
type Template struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;default:1;uniqueIndex:ux_notify_tpl_code,priority:1" json:"tenant_id"`
	Code     string `gorm:"size:64;not null;uniqueIndex:ux_notify_tpl_code,priority:2" json:"code"`
	TitleTpl string `gorm:"size:256;not null" json:"title_tpl"`
	BodyTpl  string `gorm:"type:text;not null" json:"body_tpl"`
	LinkTpl  string `gorm:"size:512" json:"link_tpl,omitempty"`
	// Channel legacy single channel (kept for AutoMigrate compat); prefer Channels.
	Channel string `gorm:"size:16;not null;default:inbox" json:"channel"`
	// Channels is a CSV of delivery channels: inbox,email,sms (M2).
	Channels  string    `gorm:"size:64;not null;default:inbox" json:"channels"`
	Enabled   bool      `gorm:"not null;default:true" json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Template) TableName() string { return "notify_templates" }

// Message is a per-user inbox notification.
type Message struct {
	ID           uint64 `gorm:"primaryKey" json:"id"`
	TenantID     uint64 `gorm:"not null;default:1;index" json:"tenant_id"`
	UserID       uint64 `gorm:"not null;index" json:"user_id"`
	TemplateCode string `gorm:"size:64;index" json:"template_code"`
	Title        string `gorm:"size:256;not null" json:"title"`
	Content      string `gorm:"type:text;not null" json:"content"`
	Link         string `gorm:"size:512" json:"link,omitempty"`
	// Type for UI: notice | ticket | im | system
	Type      string     `gorm:"size:32;not null;default:notice;index" json:"type"`
	RefType   string     `gorm:"size:32;index" json:"ref_type,omitempty"` // ticket | im_conversation
	RefID     string     `gorm:"size:64;index" json:"ref_id,omitempty"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

func (Message) TableName() string { return "notify_messages" }

// DeliveryLog records non-inbox channel attempts (email / sms).
type DeliveryLog struct {
	ID           uint64 `gorm:"primaryKey" json:"id"`
	TenantID     uint64 `gorm:"not null;default:1;index" json:"tenant_id"`
	UserID       uint64 `gorm:"not null;index" json:"user_id"`
	MessageID    uint64 `gorm:"not null;default:0;index" json:"message_id"`
	TemplateCode string `gorm:"size:64;index" json:"template_code"`
	// Channel: email | sms
	Channel string `gorm:"size:16;not null;index" json:"channel"`
	// Target: email address or phone
	Target string `gorm:"size:256" json:"target"`
	// Status: sent | skipped | failed | stub
	Status    string    `gorm:"size:16;not null;index" json:"status"`
	Detail    string    `gorm:"size:512" json:"detail,omitempty"`
	RefType   string    `gorm:"size:32;index" json:"ref_type,omitempty"`
	RefID     string    `gorm:"size:64;index" json:"ref_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func (DeliveryLog) TableName() string { return "notify_delivery_logs" }
