package localmodel

import "time"

// Hermes M3：MCP 管理。员工 Hermes 连接公司批准的内部系统（HTTP/SSE）与本地
// 工具（stdio），控制面管 MCP server 白名单与配置，manager 受控注入 config.yaml
// 的 mcp_servers 块 + .env 密钥。

const (
	HermesMcpPending  = "pending"
	HermesMcpApproved = "approved"
	HermesMcpRejected = "rejected"
	HermesMcpRevoked  = "revoked"
)

// HermesMcpServer 是已审批的 MCP server 定义。Env/Headers 值只含 ${VAR} 模板
// （不存明文密钥）；明文密钥在 SecretJSON（jsonb），只经设备签名端点读取。
type HermesMcpServer struct {
	ID           uint64            `gorm:"primaryKey" json:"id"`
	TenantID     uint              `gorm:"not null;index;uniqueIndex:uk_hermes_mcp_servers_tenant_name,priority:1" json:"tenant_id"`
	Name         string            `gorm:"size:80;not null;uniqueIndex:uk_hermes_mcp_servers_tenant_name,priority:2" json:"name"`
	DisplayName  string            `gorm:"size:120;not null" json:"display_name"`
	Description  string            `gorm:"type:text;not null" json:"description"`
	Type         string            `gorm:"size:8;not null" json:"type"` // stdio | sse | http
	Command      string            `gorm:"size:32;not null" json:"command"`
	Args         []string          `gorm:"type:jsonb;serializer:json" json:"args"`
	URL          string            `gorm:"size:1024;not null" json:"url"`
	Env          map[string]string `gorm:"type:jsonb;serializer:json" json:"env"`
	Headers      map[string]string `gorm:"type:jsonb;serializer:json" json:"headers"`
	ToolsInclude []string          `gorm:"type:jsonb;serializer:json" json:"tools_include"`
	ToolsExclude []string          `gorm:"type:jsonb;serializer:json" json:"tools_exclude"`
	Timeout      int               `gorm:"not null" json:"timeout"`
	Status       string            `gorm:"size:16;not null" json:"status"`
	ApprovedBy   uint              `gorm:"not null" json:"approved_by"`
	ApprovedAt   *time.Time        `json:"approved_at,omitempty"`
	// SecretRevision 是密钥版本：secret_json 变更时自增，digest 含它以感知轮换。
	SecretRevision uint64 `gorm:"not null" json:"secret_revision"`
	// SecretJSON 是明文密钥（jsonb），只经设备签名端点读取；list/创建响应均不返回。
	SecretJSON map[string]string `gorm:"type:jsonb;serializer:json" json:"-"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

func (HermesMcpServer) TableName() string { return "hermes_mcp_servers" }

// HermesClusterMcpServer 是集群绑定：集群绑定已审批 MCP server，随 desired-state
// 流到该集群全部 profile。
type HermesClusterMcpServer struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	TenantID  uint      `gorm:"not null;index;uniqueIndex:uk_hermes_cluster_mcp_servers_cluster_server,priority:1" json:"tenant_id"`
	ClusterID uint64    `gorm:"not null;uniqueIndex:uk_hermes_cluster_mcp_servers_cluster_server,priority:2" json:"cluster_id"`
	ServerID  uint64    `gorm:"not null;uniqueIndex:uk_hermes_cluster_mcp_servers_cluster_server,priority:3" json:"server_id"`
	CreatedBy uint      `gorm:"not null" json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

func (HermesClusterMcpServer) TableName() string { return "hermes_cluster_mcp_servers" }
