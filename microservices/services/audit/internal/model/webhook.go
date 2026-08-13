package localmodel

import "time"

const (
	WebhookSubscriptionDisabled int8 = 0
	WebhookSubscriptionEnabled  int8 = 1

	WebhookDeliveryPending  = "pending"
	WebhookDeliveryRetrying = "retrying"
	WebhookDeliverySent     = "sent"
	WebhookDeliveryFailed   = "failed"
)

// WebhookSubscription is a tenant-owned outbound audit event subscription.
type WebhookSubscription struct {
	ID                  uint       `gorm:"primaryKey" json:"id"`
	TenantID            uint       `gorm:"not null;default:1;index" json:"tenant_id"`
	Name                string     `gorm:"size:128;not null" json:"name"`
	EndpointURL         string     `gorm:"size:2048;not null" json:"endpoint_url"`
	EventActions        []string   `gorm:"type:json;serializer:json" json:"event_actions"`
	StartAuditLogID     uint       `gorm:"not null;default:0" json:"-"`
	SecretHash          string     `gorm:"size:64;not null" json:"-"`
	SecretCiphertext    string     `gorm:"type:text;not null" json:"-"`
	Status              int8       `gorm:"not null;default:1;index" json:"status"`
	ConsecutiveFailures int        `gorm:"not null;default:0" json:"consecutive_failures"`
	LastDeliveredAt     *time.Time `json:"last_delivered_at"`
	LastError           string     `gorm:"type:text;not null;default:''" json:"last_error"`
	CreatedBy           uint       `gorm:"not null;default:0" json:"created_by"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

func (WebhookSubscription) TableName() string { return "webhook_subscriptions" }

// WebhookDelivery is a durable delivery task. Secret material never appears in it.
type WebhookDelivery struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	TenantID       uint           `gorm:"not null;default:1;index" json:"tenant_id"`
	SubscriptionID uint           `gorm:"not null;index" json:"subscription_id"`
	AuditLogID     uint           `gorm:"not null" json:"audit_log_id"`
	EventID        string         `gorm:"size:96;not null" json:"event_id"`
	EventAction    string         `gorm:"size:128;not null" json:"event_action"`
	Payload        map[string]any `gorm:"type:json;serializer:json" json:"payload"`
	Status         string         `gorm:"size:24;not null;default:pending;index" json:"status"`
	Attempts       int            `gorm:"not null;default:0" json:"attempts"`
	ResponseStatus *int           `json:"response_status"`
	ResponseBody   string         `gorm:"size:1024;not null;default:''" json:"response_body"`
	LastError      string         `gorm:"size:1024;not null;default:''" json:"last_error"`
	NextAttemptAt  time.Time      `gorm:"not null;index" json:"next_attempt_at"`
	DeliveredAt    *time.Time     `json:"delivered_at"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

func (WebhookDelivery) TableName() string { return "webhook_deliveries" }

type WebhookCursor struct {
	TenantID       uint      `gorm:"primaryKey"`
	LastAuditLogID uint      `gorm:"not null;default:0"`
	UpdatedAt      time.Time `gorm:"not null"`
}

func (WebhookCursor) TableName() string { return "webhook_cursors" }
