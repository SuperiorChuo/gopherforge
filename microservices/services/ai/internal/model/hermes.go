package localmodel

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

const (
	HermesDeviceCodePending  = "pending"
	HermesDeviceCodeApproved = "approved"
	HermesDeviceCodeConsumed = "consumed"

	HermesDesiredStateActive  = "active"
	HermesDesiredStateStopped = "stopped"
	HermesDesiredStateRevoked = "revoked"

	HermesArtifactPending  = "pending"
	HermesArtifactApproved = "approved"
	HermesArtifactRejected = "rejected"
	HermesArtifactRevoked  = "revoked"
)

// HermesToolsets is a JSON-backed allowlist. The service validates every item
// before persistence; the type only handles stable database encoding.
type HermesToolsets []string

func (v HermesToolsets) Value() (driver.Value, error) {
	return json.Marshal(v)
}

func (v *HermesToolsets) Scan(src any) error {
	if v == nil {
		return fmt.Errorf("scan hermes toolsets into nil receiver")
	}
	return scanHermesJSON(src, v)
}

type HermesPrivacy struct {
	UploadConversations bool `json:"upload_conversations"`
	UploadMemory        bool `json:"upload_memory"`
	UploadLocalFiles    bool `json:"upload_local_files"`
}

func (v HermesPrivacy) Value() (driver.Value, error) {
	return json.Marshal(v)
}

func (v *HermesPrivacy) Scan(src any) error {
	if v == nil {
		return fmt.Errorf("scan hermes privacy into nil receiver")
	}
	return scanHermesJSON(src, v)
}

func scanHermesJSON(src, dst any) error {
	var data []byte
	switch value := src.(type) {
	case nil:
		data = []byte("null")
	case []byte:
		data = value
	case string:
		data = []byte(value)
	default:
		return fmt.Errorf("unsupported Hermes JSON source %T", src)
	}
	if len(data) == 0 {
		data = []byte("null")
	}
	return json.Unmarshal(data, dst)
}

type HermesEntitlement struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	TenantID    uint      `gorm:"not null;index;uniqueIndex:uk_hermes_entitlements_tenant_user,priority:1" json:"tenant_id"`
	UserID      uint      `gorm:"not null;index;uniqueIndex:uk_hermes_entitlements_tenant_user,priority:2" json:"user_id"`
	Enabled     bool      `gorm:"not null" json:"enabled"`
	PolicyID    uint64    `gorm:"not null" json:"policy_id"`
	DeviceLimit int       `gorm:"not null" json:"device_limit"`
	CreatedBy   uint      `gorm:"not null" json:"created_by"`
	UpdatedBy   uint      `gorm:"not null" json:"updated_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (HermesEntitlement) TableName() string { return "hermes_entitlements" }

// HermesDeviceCode deliberately has no TenantID field. Enrollment starts
// before a console identity exists; the approver identity is recorded only in
// the Approved* fields.
type HermesDeviceCode struct {
	ID               uint64     `gorm:"primaryKey" json:"-"`
	DeviceCodeHash   string     `gorm:"size:64;not null;uniqueIndex" json:"-"`
	UserCodeHash     string     `gorm:"size:64;not null;uniqueIndex" json:"-"`
	PublicKey        string     `gorm:"size:128;not null" json:"-"`
	KeyFingerprint   string     `gorm:"size:64;not null" json:"key_fingerprint"`
	DeviceName       string     `gorm:"size:80;not null" json:"device_name"`
	Platform         string     `gorm:"size:16;not null" json:"platform"`
	Arch             string     `gorm:"size:16;not null" json:"arch"`
	Status           string     `gorm:"size:16;not null" json:"status"`
	ApprovedTenantID *uint      `json:"approved_tenant_id,omitempty"`
	ApprovedUserID   *uint      `json:"approved_user_id,omitempty"`
	ApprovedAt       *time.Time `json:"approved_at,omitempty"`
	ConsumedAt       *time.Time `json:"consumed_at,omitempty"`
	ExpiresAt        time.Time  `gorm:"not null" json:"expires_at"`
	CreatedAt        time.Time  `json:"created_at"`
}

func (HermesDeviceCode) TableName() string { return "hermes_device_codes" }

type HermesDevice struct {
	ID                    uint64 `gorm:"primaryKey" json:"id"`
	TenantID              uint   `gorm:"not null;index" json:"tenant_id"`
	DeviceID              string `gorm:"size:36;not null;uniqueIndex" json:"device_id"`
	UserID                uint   `gorm:"not null;index" json:"user_id"`
	PublicKey             string `gorm:"size:128;not null" json:"-"`
	KeyFingerprint        string `gorm:"size:64;not null;uniqueIndex" json:"key_fingerprint"`
	DeviceName            string `gorm:"size:80;not null" json:"device_name"`
	Platform              string `gorm:"size:16;not null" json:"platform"`
	Arch                  string `gorm:"size:16;not null" json:"arch"`
	ManagerVersion        string `gorm:"size:40;not null" json:"manager_version"`
	HermesVersion         string `gorm:"size:40;not null" json:"hermes_version"`
	ActualState           string `gorm:"size:24;not null" json:"actual_state"`
	HealthStatus          string `gorm:"size:24;not null" json:"health_status"`
	AppliedPolicyRevision uint64 `gorm:"not null" json:"applied_policy_revision"`
	ConfigDigest          string `gorm:"size:64;not null" json:"config_digest"`
	LastErrorCode         string `gorm:"size:64;not null" json:"last_error_code"`
	// APIServerFingerprint 是设备端 Gateway API server 密钥的 sha256 指纹（三期）。
	// 控制面只存指纹不持原始 key；空串表示未启用。
	APIServerFingerprint string `gorm:"size:64;not null" json:"api_server_fingerprint"`
	// 管理员置位的运维动作，随签名期望状态下发；manager 执行并回执事件后清空。
	// 取值受迁移 000035 的 chk_hermes_device_pending_action 约束，控制面不下发自由文本。
	PendingAction   string     `gorm:"size:24;not null" json:"pending_action"`
	PendingActionAt *time.Time `json:"pending_action_at,omitempty"`
	LastSeenAt      *time.Time `json:"last_seen_at,omitempty"`
	RevokedAt       *time.Time `json:"revoked_at,omitempty"`
	// RevocationConfirmed is derived for signed device authentication. It is
	// never persisted or returned by console APIs.
	RevocationConfirmed bool      `gorm:"-" json:"-"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func (HermesDevice) TableName() string { return "hermes_devices" }

type HermesPolicy struct {
	ID                        uint64         `gorm:"primaryKey" json:"id"`
	TenantID                  uint           `gorm:"not null;index" json:"tenant_id"`
	Revision                  uint64         `gorm:"not null" json:"revision"`
	Name                      string         `gorm:"size:80;not null" json:"name"`
	DesiredState              string         `gorm:"size:16;not null" json:"desired_state"`
	Profile                   string         `gorm:"size:32;not null" json:"profile"`
	AssistantTemplateID       string         `gorm:"size:64;not null" json:"assistant_template_id"`
	AssistantTemplateRevision uint64         `gorm:"not null" json:"assistant_template_revision"`
	ArtifactID                uint64         `gorm:"not null" json:"artifact_id"`
	ModelID                   uint64         `gorm:"not null" json:"model_id"`
	ModelName                 string         `gorm:"size:100;not null" json:"model_name"`
	ModelBaseURL              string         `gorm:"size:255;not null" json:"model_base_url"`
	Toolsets                  HermesToolsets `gorm:"type:jsonb;not null" json:"toolsets"`
	Privacy                   HermesPrivacy  `gorm:"type:jsonb;not null" json:"privacy"`
	LeaseSeconds              int            `gorm:"not null" json:"lease_seconds"`
	// EnableKnowledgeRAG 是服务端开关（M4）：为真时 Hermes 模型代理在转发前检索
	// 企业知识库相关块并注入 system message。不进签名载荷（只影响 proxy 行为）。
	EnableKnowledgeRAG bool      `gorm:"not null;default:false" json:"enable_knowledge_rag"`
	Signature          string    `gorm:"type:text;not null" json:"signature,omitempty"`
	CreatedBy          uint      `gorm:"not null" json:"created_by"`
	CreatedAt          time.Time `json:"created_at"`
}

func (HermesPolicy) TableName() string { return "hermes_policies" }

// HermesArtifact 的 (tenant,kind,platform,arch,version) 唯一约束在迁移
// 000033 里以 uk_hermes_artifacts_release 建立；这里同名声明一份，使
// AutoMigrate（测试用 sqlite）与 Postgres 真实 schema 不漂移。
type HermesArtifact struct {
	ID             uint64     `gorm:"primaryKey" json:"id"`
	TenantID       uint       `gorm:"not null;index;uniqueIndex:uk_hermes_artifacts_release,priority:1" json:"tenant_id"`
	Kind           string     `gorm:"size:16;not null;uniqueIndex:uk_hermes_artifacts_release,priority:2" json:"kind"`
	Platform       string     `gorm:"size:16;not null;uniqueIndex:uk_hermes_artifacts_release,priority:3" json:"platform"`
	Arch           string     `gorm:"size:16;not null;uniqueIndex:uk_hermes_artifacts_release,priority:4" json:"arch"`
	Version        string     `gorm:"size:40;not null;uniqueIndex:uk_hermes_artifacts_release,priority:5" json:"version"`
	UpstreamCommit string     `gorm:"size:64;not null" json:"upstream_commit"`
	SourceURL      string     `gorm:"size:1024;not null" json:"source_url"`
	SHA256         string     `gorm:"size:64;not null" json:"sha256"`
	Signature      string     `gorm:"type:text;not null" json:"signature,omitempty"`
	Status         string     `gorm:"size:16;not null" json:"status"`
	ApprovedBy     uint       `gorm:"not null" json:"approved_by"`
	ApprovedAt     *time.Time `json:"approved_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (HermesArtifact) TableName() string { return "hermes_artifacts" }

// HermesModelToken 是给受管 Hermes 用的短期模型访问凭据。
//
// 为什么不复用设备的 Ed25519 逐请求签名：Hermes 是原生上游程序，只会发标准
// OpenAI 请求 + Bearer token。manager 用自己的签名身份换取本 token，写进受管
// profile 的 .env，Hermes 拿它调企业模型代理。
//
// 只存 SHA-256 哈希，明文仅在签发响应里出现一次——控制面本身不持有可用凭据。
type HermesModelToken struct {
	ID         uint64     `gorm:"primaryKey" json:"id"`
	TenantID   uint       `gorm:"not null;index" json:"tenant_id"`
	TokenHash  string     `gorm:"size:64;not null;uniqueIndex" json:"-"`
	DeviceID   string     `gorm:"size:36;not null;index" json:"device_id"`
	UserID     uint       `gorm:"not null" json:"user_id"`
	ExpiresAt  time.Time  `gorm:"not null" json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (HermesModelToken) TableName() string { return "hermes_model_tokens" }

type HermesDeviceEvent struct {
	ID                    uint64    `gorm:"primaryKey" json:"id"`
	TenantID              uint      `gorm:"not null;index" json:"tenant_id"`
	DeviceID              string    `gorm:"size:36;not null;index" json:"device_id"`
	EventType             string    `gorm:"size:40;not null" json:"event_type"`
	Result                string    `gorm:"size:16;not null" json:"result"`
	ErrorCode             string    `gorm:"size:64;not null" json:"error_code"`
	DesiredPolicyRevision uint64    `gorm:"not null" json:"desired_policy_revision"`
	AppliedPolicyRevision uint64    `gorm:"not null" json:"applied_policy_revision"`
	OccurredAt            time.Time `gorm:"not null" json:"occurred_at"`
	CreatedAt             time.Time `json:"created_at"`
}

func (HermesDeviceEvent) TableName() string { return "hermes_device_events" }

// 实例状态。unassigned = 未绑定节点；pending = 已绑定但节点尚未跑起来。
const (
	HermesInstanceStatusUnassigned = "unassigned"
	HermesInstanceStatusPending    = "pending"
	HermesInstanceStatusRunning    = "running"
	HermesInstanceStatusError      = "error"
)

// HermesInstance 是逻辑 Agent 实例：有角色、有负责人、有自己的模型策略，
// 放置在某个运行节点（hermes_devices）上。
//
// 与设备的区别：设备是承载层（员工电脑 + manager），实例是逻辑体。两者生命
// 周期独立——实例可以换负责人不换机器，也可以先建好不绑节点。策略此前绑在
// 用户身上（hermes_entitlements），导致同一人的所有设备只能跑同一套配置；
// 实例层就是为拆开这一点而存在。
//
// DeviceID 不加唯一约束：一节点多实例是既定方向，当前仅因 manager 硬编码了
// 单实例（LaunchAgent 标签 / schtasks 任务名 / HERMES_HOME 均为常量）而由
// 服务层暂时拦截。约束留在服务层，manager 跟上后去掉拦截即可，不必改表。
type HermesInstance struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint   `gorm:"not null;index;uniqueIndex:uk_hermes_instances_tenant_name,priority:1" json:"tenant_id"`
	Name     string `gorm:"size:80;not null;uniqueIndex:uk_hermes_instances_tenant_name,priority:2" json:"name"`
	Role     string `gorm:"size:40;not null" json:"role"`
	// OwnerUserID 是实例负责人，与承载设备的 user_id 解耦
	OwnerUserID uint `gorm:"not null;index" json:"owner_user_id"`
	// DeviceID 为空表示实例已建但尚未放置到任何节点
	DeviceID string `gorm:"size:36;not null;index" json:"device_id"`
	// PolicyID 为 0 表示不覆盖，回落到权益策略 / 最新策略
	PolicyID uint64 `gorm:"not null" json:"policy_id"`
	// ClusterID 为 0 表示未归组；>0 表示实例属于某个 company_cluster（多 Agent 协作）
	ClusterID uint64    `gorm:"not null;index" json:"cluster_id"`
	Enabled   bool      `gorm:"not null" json:"enabled"`
	Status    string    `gorm:"size:16;not null" json:"status"`
	CreatedBy uint      `gorm:"not null" json:"created_by"`
	UpdatedBy uint      `gorm:"not null" json:"updated_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (HermesInstance) TableName() string { return "hermes_instances" }
