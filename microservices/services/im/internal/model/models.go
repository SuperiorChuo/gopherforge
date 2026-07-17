package model

import (
	"time"

	"github.com/google/uuid"
)

type Site struct {
	ID             uint64 `gorm:"primaryKey" json:"id"`
	TenantID       uint64 `gorm:"not null;default:1;index" json:"tenant_id"`
	AppKey         string `gorm:"size:64;uniqueIndex;not null" json:"app_key"`
	AppSecret      string `gorm:"size:128;not null" json:"-"`
	Name           string `gorm:"size:128;not null" json:"name"`
	AllowedOrigins string `gorm:"type:text" json:"allowed_origins"` // JSON array string
	WelcomeText    string `gorm:"type:text" json:"welcome_text"`
	Status         int16  `gorm:"default:1" json:"status"`
	// BotEnabled: new conversations start in bot_serving when true (M4).
	BotEnabled bool `gorm:"default:true" json:"bot_enabled"`
	// BotSystemPrompt optional site-level system prompt for AI bot.
	BotSystemPrompt string `gorm:"type:text" json:"bot_system_prompt,omitempty"`
	// DefaultSkillGroupID routes new conversations when visitor does not specify one.
	DefaultSkillGroupID *uint64   `json:"default_skill_group_id,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func (Site) TableName() string { return "im_sites" }

type Visitor struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	SiteID      uint64    `gorm:"uniqueIndex:ux_site_guest;not null" json:"site_id"`
	GuestKey    string    `gorm:"size:64;uniqueIndex:ux_site_guest;not null" json:"guest_key"`
	UserID      *uint64   `json:"user_id,omitempty"`
	DisplayName string    `gorm:"size:128" json:"display_name"`
	Meta        string    `gorm:"type:text" json:"meta,omitempty"`
	LastSeenAt  time.Time `json:"last_seen_at"`
	CreatedAt   time.Time `json:"created_at"`
}

func (Visitor) TableName() string { return "im_visitors" }

// SkillGroup routes queue assignment.
type SkillGroup struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;default:1;uniqueIndex:ux_im_skill_groups_tenant_code,priority:1;index" json:"tenant_id"`
	Name     string `gorm:"size:128;not null" json:"name"`
	Code     string `gorm:"size:64;uniqueIndex:ux_im_skill_groups_tenant_code,priority:2;not null" json:"code"`
	// Strategy: round_robin | least_load | manual
	Strategy string `gorm:"size:32;not null;default:round_robin" json:"strategy"`
	Status   int16  `gorm:"default:1" json:"status"`
	// RRCursor is last assigned agent_user_id for round-robin (best-effort).
	RRCursor  uint64    `gorm:"default:0" json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (SkillGroup) TableName() string { return "im_skill_groups" }

// AgentSkill binds a backend user to a skill group.
type AgentSkill struct {
	ID            uint64    `gorm:"primaryKey" json:"id"`
	AgentUserID   uint64    `gorm:"uniqueIndex:ux_agent_skill;index;not null" json:"agent_user_id"`
	SkillGroupID  uint64    `gorm:"uniqueIndex:ux_agent_skill;index;not null" json:"skill_group_id"`
	MaxConcurrent int       `gorm:"default:5;not null" json:"max_concurrent"`
	Status        int16     `gorm:"default:1" json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (AgentSkill) TableName() string { return "im_agent_skills" }

// AgentPresence is PG-backed for M3 (Redis can replace later).
type AgentPresence struct {
	AgentUserID uint64 `gorm:"primaryKey" json:"agent_user_id"`
	// Status: online | busy | offline
	Status      string    `gorm:"size:16;not null;default:offline;index" json:"status"`
	DisplayName string    `gorm:"size:128" json:"display_name"`
	LastSeenAt  time.Time `json:"last_seen_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (AgentPresence) TableName() string { return "im_agent_presence" }

type Conversation struct {
	ID           uint64    `gorm:"primaryKey" json:"id"`
	PublicID     uuid.UUID `gorm:"type:uuid;uniqueIndex;not null" json:"public_id"`
	TenantID     uint64    `gorm:"not null;default:1;index:idx_im_conversations_tenant_status,priority:1;index" json:"tenant_id"`
	SiteID       uint64    `gorm:"index;not null" json:"site_id"`
	Channel      string    `gorm:"size:32;not null;default:h5" json:"channel"`
	VisitorID    uint64    `gorm:"index;not null" json:"visitor_id"`
	AgentUserID  *uint64   `gorm:"index" json:"agent_user_id,omitempty"`
	SkillGroupID *uint64   `gorm:"index" json:"skill_group_id,omitempty"`
	Status       string    `gorm:"size:32;index:idx_im_conversations_tenant_status,priority:2;index;not null;default:queued" json:"status"`
	Context      string    `gorm:"type:text" json:"context,omitempty"`
	CloseReason  string    `gorm:"size:64" json:"close_reason,omitempty"`
	// Summary is AI/rule session summary (M4).
	Summary            string     `gorm:"type:text" json:"summary,omitempty"`
	QueuedAt           *time.Time `json:"queued_at,omitempty"`
	AssignedAt         *time.Time `json:"assigned_at,omitempty"`
	ClosedAt           *time.Time `json:"closed_at,omitempty"`
	LastMessageAt      *time.Time `json:"last_message_at,omitempty"`
	LastMessagePreview string     `gorm:"size:256" json:"last_message_preview,omitempty"`
	// Read cursors: last message seq each side has seen (0 = nothing).
	AgentLastReadSeq   int64     `gorm:"not null;default:0" json:"agent_last_read_seq"`
	VisitorLastReadSeq int64     `gorm:"not null;default:0" json:"visitor_last_read_seq"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func (Conversation) TableName() string { return "im_conversations" }

type Message struct {
	ID             uint64    `gorm:"primaryKey" json:"id"`
	ConversationID uint64    `gorm:"index:idx_conv_seq,priority:1;not null" json:"conversation_id"`
	ClientMsgID    *string   `gorm:"size:64" json:"client_msg_id,omitempty"`
	SenderType     string    `gorm:"size:16;not null" json:"sender_type"`
	SenderID       *uint64   `json:"sender_id,omitempty"`
	MsgType        string    `gorm:"size:32;not null;default:text" json:"msg_type"`
	Content        string    `gorm:"type:text;not null" json:"content"` // JSON string
	Seq            int64     `gorm:"index:idx_conv_seq,priority:2;not null" json:"seq"`
	CreatedAt      time.Time `json:"created_at"`
	// Replayed marks an idempotent client_msg_id resend: the row already
	// existed, so callers must skip hub broadcast / bot side effects.
	Replayed bool `gorm:"-" json:"-"`
}

func (Message) TableName() string { return "im_messages" }
