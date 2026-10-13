package localmodel

import "time"

// OperationLog 存储请求级别的操作审计数据。
type OperationLog struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	UserID       uint      `gorm:"index" json:"user_id"`
	Username     string    `gorm:"size:50" json:"username"`
	ActorType    string    `gorm:"size:64;default:operator;index" json:"actor_type"`
	ActorID      string    `gorm:"size:128;default:web-console;index" json:"actor_id"`
	RequestID    string    `gorm:"size:64;index" json:"request_id"`
	Module       string    `gorm:"size:50" json:"module"`
	Action       string    `gorm:"size:50" json:"action"`
	Method       string    `gorm:"size:10" json:"method"`
	Path         string    `gorm:"size:255" json:"path"`
	Query        string    `gorm:"size:1024" json:"query"`
	RequestBody  string    `gorm:"type:text" json:"request_body"`
	ResponseBody string    `gorm:"type:text" json:"response_body"`
	Status       int       `json:"status"`
	IP           string    `gorm:"size:45" json:"ip"`
	UserAgent    string    `gorm:"size:500" json:"user_agent"`
	Latency      int64     `json:"latency"`
	ErrorMsg     string    `gorm:"size:1024" json:"error_msg"`
	CreatedAt    time.Time `gorm:"index" json:"created_at"`
}

// AuditLog 存储业务审计事件。
type AuditLog struct {
	ID         uint           `gorm:"primaryKey" json:"id"`
	TenantID   uint           `gorm:"not null;default:1;index" json:"tenant_id"`
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

// LoginLog 存储认证尝试记录。
type LoginLog struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	UserID    uint   `gorm:"index" json:"user_id"`
	Username  string `gorm:"size:50" json:"username"`
	LoginType int8   `gorm:"default:1" json:"login_type"`
	// Status 0 = failed 是真实数据，因此不加 gorm default 标签：若加了，GORM
	// 会在 INSERT 时丢弃零值，列默认值（1）会生效，导致失败记录被存成成功。
	Status    int8      `json:"status"`
	IP        string    `gorm:"size:45" json:"ip"`
	Location  string    `gorm:"size:100" json:"location"`
	Device    string    `gorm:"size:100" json:"device"`
	OS        string    `gorm:"size:50" json:"os"`
	Browser   string    `gorm:"size:100" json:"browser"`
	UserAgent string    `gorm:"size:500" json:"user_agent"`
	Message   string    `gorm:"size:255" json:"message"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}
