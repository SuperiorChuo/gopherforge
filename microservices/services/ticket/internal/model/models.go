// Package model defines the ticket M1 entities: work orders linked to IM
// conversations / CRM customers, with a minimal status machine and event log.
package model

import "time"

// Status values.
const (
	StatusOpen       = "open"
	StatusInProgress = "in_progress"
	StatusResolved   = "resolved"
	StatusClosed     = "closed"
)

// Priority values.
const (
	PriorityLow    = "low"
	PriorityNormal = "normal"
	PriorityHigh   = "high"
	PriorityUrgent = "urgent"
)

// Source values.
const (
	SourceManual = "manual"
	SourceIM     = "im"
	SourceCall   = "call"
	SourceOther  = "other"
)

// Event kinds.
const (
	EventCreated = "created"
	EventComment = "comment"
	EventStatus  = "status_change"
	EventAssign  = "assign"
	EventUpdate  = "update"
)

// Ticket is a support work order.
type Ticket struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;default:1;index" json:"tenant_id"`
	// TicketNo human-readable id, unique per tenant (e.g. TK-12).
	TicketNo string `gorm:"size:32;not null;index" json:"ticket_no"`
	Title    string `gorm:"size:256;not null" json:"title"`
	// Description free text / IM summary paste.
	Description string `gorm:"type:text" json:"description,omitempty"`
	// Status: open | in_progress | resolved | closed
	Status string `gorm:"size:32;not null;default:open;index" json:"status"`
	// Priority: low | normal | high | urgent
	Priority string `gorm:"size:16;not null;default:normal;index" json:"priority"`
	// Source: manual | im | call | other
	Source string `gorm:"size:32;not null;default:manual;index" json:"source"`

	CreatorUserID  uint64 `gorm:"not null;index" json:"creator_user_id"`
	AssigneeUserID uint64 `gorm:"not null;default:0;index" json:"assignee_user_id"`

	// Optional CRM / IM links (soft references, no cross-service FK).
	CustomerID           uint64 `gorm:"not null;default:0;index" json:"customer_id,omitempty"`
	ConversationPublicID string `gorm:"size:64;index" json:"conversation_public_id,omitempty"`
	IMVisitorID          uint64 `gorm:"not null;default:0;index" json:"im_visitor_id,omitempty"`

	ContactName  string `gorm:"size:128" json:"contact_name,omitempty"`
	ContactPhone string `gorm:"size:32;index" json:"contact_phone,omitempty"`

	// DueAt optional SLA deadline; overdue = now > DueAt && status open|in_progress.
	DueAt       *time.Time `json:"due_at,omitempty"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
	ResolveNote string     `gorm:"type:text" json:"resolve_note,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Ticket) TableName() string { return "ticket_tickets" }

// TicketEvent is an append-only timeline entry (comments + system changes).
type TicketEvent struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;default:1;index" json:"tenant_id"`
	TicketID uint64 `gorm:"not null;index" json:"ticket_id"`
	// Kind: created | comment | status_change | assign | update
	Kind       string    `gorm:"size:32;not null;index" json:"kind"`
	Content    string    `gorm:"type:text" json:"content,omitempty"`
	FromStatus string    `gorm:"size:32" json:"from_status,omitempty"`
	ToStatus   string    `gorm:"size:32" json:"to_status,omitempty"`
	ByUserID   uint64    `gorm:"not null;default:0;index" json:"by_user_id"`
	CreatedAt  time.Time `json:"created_at"`
}

func (TicketEvent) TableName() string { return "ticket_events" }

// NotificationJob durably delivers user-facing ticket notifications.
type NotificationJob struct {
	ID           uint64    `gorm:"primaryKey"`
	TenantID     uint64    `gorm:"not null;index"`
	DedupeKey    string    `gorm:"size:160;not null;uniqueIndex"`
	UserID       uint64    `gorm:"not null"`
	TemplateCode string    `gorm:"size:64;not null"`
	RefType      string    `gorm:"size:64;not null"`
	RefID        string    `gorm:"size:96;not null"`
	VarsJSON     []byte    `gorm:"type:jsonb;not null"`
	Status       string    `gorm:"size:16;not null;default:pending;index:idx_ticket_notify_due,priority:1"`
	Attempts     int       `gorm:"not null;default:0"`
	NextAt       time.Time `gorm:"not null;index:idx_ticket_notify_due,priority:2"`
	LastError    string    `gorm:"size:512"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (NotificationJob) TableName() string { return "ticket_notification_jobs" }
