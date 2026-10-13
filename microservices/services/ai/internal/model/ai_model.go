package localmodel

import "time"

// AIModel 模型配置（数据库化管理多个AI供应商）
type AIModel struct {
	ID              uint64                 `gorm:"primaryKey" json:"id"`
	Name            string                 `gorm:"column:name;size:100;not null;uniqueIndex" json:"name"`
	DisplayName     string                 `gorm:"column:display_name;size:100;not null" json:"display_name"`
	Provider        string                 `gorm:"column:provider;size:50;not null;index:idx_provider_status" json:"provider"`
	APIKey          string                 `gorm:"column:api_key;type:text" json:"api_key,omitempty"`
	APIKeyIV        string                 `gorm:"column:api_key_iv;size:32" json:"-"`
	BaseURL         string                 `gorm:"column:base_url;size:255" json:"base_url,omitempty"`
	ModelConfig     map[string]interface{} `gorm:"column:model_config;type:json;serializer:json" json:"model_config,omitempty"`
	Capabilities    []string               `gorm:"column:capabilities;type:json;serializer:json" json:"capabilities"`
	Status          int                    `gorm:"column:status;not null;default:1;index:idx_provider_status" json:"status"`
	CostPer1KInput  float64                `gorm:"column:cost_per_1k_input;type:decimal(10,6);default:0" json:"cost_per_1k_input"`
	CostPer1KOutput float64                `gorm:"column:cost_per_1k_output;type:decimal(10,6);default:0" json:"cost_per_1k_output"`
	CostPerImage    float64                `gorm:"column:cost_per_image;type:decimal(10,4);default:0" json:"cost_per_image"`
	DailyLimit      *int                   `gorm:"column:daily_limit" json:"daily_limit,omitempty"`
	Description     string                 `gorm:"column:description;type:text" json:"description,omitempty"`
	Sort            int                    `gorm:"column:sort;not null;default:0;index" json:"sort"`
	CreatedAt       time.Time              `gorm:"column:created_at" json:"created_at"`
	UpdatedAt       time.Time              `gorm:"column:updated_at" json:"updated_at"`
}

func (AIModel) TableName() string { return "ai_models" }

// AIUsageLog AI调用记录（计量、审计、费用统计）
type AIUsageLog struct {
	ID        uint64  `gorm:"primaryKey" json:"id"`
	ModelID   uint64  `gorm:"column:model_id;not null;index:idx_model_time" json:"model_id"`
	ModelName string  `gorm:"column:model_name;size:100;not null" json:"model_name"`
	UserID    *uint64 `gorm:"column:user_id;index:idx_user_time" json:"user_id,omitempty"`
	Username  string  `gorm:"column:username;size:100" json:"username,omitempty"`
	TenantID  *uint64 `gorm:"column:tenant_id;index:idx_tenant_time" json:"tenant_id,omitempty"`
	// Hermes 模型代理的调用归属设备；控制台自身的调用留空。
	DeviceID         string    `gorm:"column:device_id;size:36;not null;default:''" json:"device_id,omitempty"`
	Operation        string    `gorm:"column:operation;size:50;not null" json:"operation"`
	PromptTokens     *int      `gorm:"column:prompt_tokens" json:"prompt_tokens,omitempty"`
	CompletionTokens *int      `gorm:"column:completion_tokens" json:"completion_tokens,omitempty"`
	TotalTokens      *int      `gorm:"column:total_tokens" json:"total_tokens,omitempty"`
	ImageCount       int       `gorm:"column:image_count;default:0" json:"image_count"`
	Cost             float64   `gorm:"column:cost;type:decimal(10,4);default:0" json:"cost"`
	LatencyMs        *int      `gorm:"column:latency_ms" json:"latency_ms,omitempty"`
	Status           string    `gorm:"column:status;size:20;not null;index" json:"status"`
	ErrorMsg         string    `gorm:"column:error_msg;type:text" json:"error_msg,omitempty"`
	RequestID        string    `gorm:"column:request_id;size:100;index" json:"request_id,omitempty"`
	IP               string    `gorm:"column:ip;size:50" json:"ip,omitempty"`
	UserAgent        string    `gorm:"column:user_agent;size:255" json:"user_agent,omitempty"`
	CreatedAt        time.Time `gorm:"column:created_at;index:idx_model_time,priority:2;index:idx_user_time,priority:2;index:idx_tenant_time,priority:2" json:"created_at"`
}

func (AIUsageLog) TableName() string { return "ai_usage_logs" }

// AIQuota 配额管理（限流与成本控制）
type AIQuota struct {
	ID                  uint64    `gorm:"primaryKey" json:"id"`
	Scope               string    `gorm:"column:scope;size:20;not null;uniqueIndex:uk_scope;index:idx_scope_enabled" json:"scope"`
	ScopeID             *uint64   `gorm:"column:scope_id;uniqueIndex:uk_scope" json:"scope_id,omitempty"`
	DailyTokenLimit     *int      `gorm:"column:daily_token_limit" json:"daily_token_limit,omitempty"`
	DailyCostLimit      *float64  `gorm:"column:daily_cost_limit;type:decimal(10,2)" json:"daily_cost_limit,omitempty"`
	DailyRequestLimit   *int      `gorm:"column:daily_request_limit" json:"daily_request_limit,omitempty"`
	MonthlyTokenLimit   *int      `gorm:"column:monthly_token_limit" json:"monthly_token_limit,omitempty"`
	MonthlyCostLimit    *float64  `gorm:"column:monthly_cost_limit;type:decimal(10,2)" json:"monthly_cost_limit,omitempty"`
	MonthlyRequestLimit *int      `gorm:"column:monthly_request_limit" json:"monthly_request_limit,omitempty"`
	ResetDayOfMonth     int       `gorm:"column:reset_day_of_month;default:1" json:"reset_day_of_month"`
	Enabled             int       `gorm:"column:enabled;not null;default:1;index:idx_scope_enabled" json:"enabled"`
	CreatedAt           time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt           time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (AIQuota) TableName() string { return "ai_quotas" }

// AIModelStatus 模型状态枚举
const (
	AIModelStatusDisabled = 0
	AIModelStatusEnabled  = 1
)

// AIUsageStatus 调用状态枚举
const (
	AIUsageStatusSuccess = "success"
	AIUsageStatusError   = "error"
)

// AIQuotaScope 配额作用域枚举
const (
	AIQuotaScopeGlobal = "global"
	AIQuotaScopeUser   = "user"
	AIQuotaScopeTenant = "tenant"
)

// AIOperation 操作类型枚举
const (
	AIOperationChat  = "chat"
	AIOperationEmbed = "embed"
	AIOperationImage = "image"
	AIOperationAgent = "agent"
)

// AIProvider 供应商枚举
const (
	AIProviderOpenAI    = "openai"
	AIProviderAnthropic = "anthropic"
	AIProviderAzure     = "azure"
	AIProviderCustom    = "custom"
)
