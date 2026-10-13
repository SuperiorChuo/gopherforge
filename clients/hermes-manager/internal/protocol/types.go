package protocol

import "time"

type Envelope[T any] struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	ErrorCode string `json:"error_code,omitempty"`
	Data      T      `json:"data"`
}

type DeviceAuthorizationRequest struct {
	PublicKey  string `json:"public_key"`
	DeviceName string `json:"device_name"`
	Platform   string `json:"platform"`
	Arch       string `json:"arch"`
}

type DeviceAuthorization struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type DeviceIdentity struct {
	DeviceID               string    `json:"device_id"`
	PolicySigningPublicKey string    `json:"policy_signing_public_key"`
	IssuedAt               time.Time `json:"issued_at"`
}

type Privacy struct {
	UploadConversations bool `json:"upload_conversations"`
	UploadMemory        bool `json:"upload_memory"`
	UploadLocalFiles    bool `json:"upload_local_files"`
}

type DesiredStateAssistant struct {
	TemplateID string `json:"template_id"`
	Revision   uint64 `json:"template_revision"`
}

type DesiredStateArtifact struct {
	ID        uint64 `json:"artifact_id"`
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	SHA256    string `json:"sha256"`
	SourceURL string `json:"source_url"`
}

type DesiredStateModel struct {
	Mode    string `json:"mode"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
}

// DesiredStateTask 是控制面下发给单个 Agent 的同步任务意图。manager 验签后把
// description 推给本地 Hermes（Gateway API server），执行完成后经
// POST /device/task-runs 回传结果。
type DesiredStateTask struct {
	TaskID      uint64 `json:"task_id"`
	RunID       uint64 `json:"run_id"`
	Description string `json:"description"`
	Model       string `json:"model,omitempty"`
}

// DesiredStateKanbanWorker 是一个 kanban worker profile 名。会成为文件系统
// 目录名，因此必须经过安全校验（见 validateKanban）。
type DesiredStateKanbanWorker struct {
	Name string `json:"name"`
}

// DesiredStateKanban 是 kanban 多 Agent fan-out 指令。与 DesiredStateTask
// （push 通道）互斥：Kanban != nil 时走 kanban 通道（manager 建 board + 建
// 任务 + 轮询，gateway dispatcher 派生 worker），否则走 push。
type DesiredStateKanban struct {
	TaskID    uint64                     `json:"task_id"`
	RunID     uint64                     `json:"run_id"`
	BoardSlug string                     `json:"board_slug"`
	Title     string                     `json:"title"`
	Body      string                     `json:"body"`
	Assignee  string                     `json:"assignee"`
	Model     string                     `json:"model,omitempty"`
	Workers   []DesiredStateKanbanWorker `json:"workers,omitempty"`
}

// KanbanTaskStatus 是 kanban CLI show --json 的解析结果（adapter 层）。
type KanbanTaskStatus struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	LatestSummary string `json:"latest_summary"`
}

// DesiredStateSkill 是下发给设备的一个技能引用（M2 技能市场）。manager 按
// SourceURL 下载 tarball、校验 sha256、解压进 profile skills 树。与 server 侧
// HermesDesiredStateSkill 逐字一致。
type DesiredStateSkill struct {
	ID        uint64 `json:"skill_id"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	SHA256    string `json:"sha256"`
	SourceURL string `json:"source_url"`
}

// DesiredStateMcpServer 是下发给设备的 MCP server 配置（M3 MCP 管理）。
// Env/Headers 值只含 ${VAR} 模板，明文密钥经设备签名端点另取；SecretRevision
// 让 manager 感知密钥轮换。与 server 侧 HermesDesiredStateMcpServer 逐字一致。
type DesiredStateMcpServer struct {
	ID             uint64            `json:"server_id"`
	Name           string            `json:"name"`
	Type           string            `json:"type"`
	URL            string            `json:"url,omitempty"`
	Command        string            `json:"command,omitempty"`
	Args           []string          `json:"args,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	ToolsInclude   []string          `json:"tools_include,omitempty"`
	ToolsExclude   []string          `json:"tools_exclude,omitempty"`
	Timeout        int               `json:"timeout,omitempty"`
	SecretRevision uint64            `json:"secret_revision,omitempty"`
}

type UnsignedDesiredState struct {
	SchemaVersion  int                   `json:"schema_version"`
	Revision       uint64                `json:"revision"`
	DesiredState   string                `json:"desired_state"`
	LeaseExpiresAt time.Time             `json:"lease_expires_at"`
	Profile        string                `json:"profile"`
	Assistant      DesiredStateAssistant `json:"assistant"`
	Hermes         DesiredStateArtifact  `json:"hermes"`
	Model          DesiredStateModel     `json:"model"`
	Toolsets       []string              `json:"toolsets"`
	Privacy        Privacy               `json:"privacy"`
	// PendingAction 是管理员排入的一次性运维动作，空串表示无。
	//
	// 字段形状必须与服务端签名载荷逐字一致（含 omitempty）：Verify 是把本结构体
	// 重新序列化后验签的，任何字段缺失或标签不一致都会导致签名校验失败。
	// 服务端在 hermesUnsignedDesiredState（services/ai/internal/service/hermes/hermes.go）。
	PendingAction string `json:"pending_action,omitempty"`
	// Task 是非空时的多 Agent 任务意图。用指针 + omitempty：零值 struct 不会被
	// encoding/json 省略，指针才能在无任务时让 P1 payload 字节不变（与
	// pending_action 同一契约）。必须是最后一个字段，与服务端 mirror 一致。
	Task *DesiredStateTask `json:"task,omitempty"`
	// Kanban 是非空时的 kanban fan-out 指令。与 Task 互斥。指针 + omitempty，
	// 必须是最后一个字段，与服务端 mirror 一致。
	Kanban *DesiredStateKanban `json:"kanban,omitempty"`
	// Skills 是集群绑定并已审批的技能引用（M2）。空时省略，P1 payload 字节不变。
	// 必须是最后一个字段，与服务端 mirror 一致。
	Skills []DesiredStateSkill `json:"skills,omitempty"`
	// McpServers 是集群绑定并已审批的 MCP server 配置（M3）。空时省略，P1 payload
	// 字节不变。必须是最后一个字段，与服务端 mirror 一致。
	McpServers []DesiredStateMcpServer `json:"mcp_servers,omitempty"`
	// Cron 是租户启用的定时任务（M5）。manager 在设备端评估调度。空时省略，
	// P1 payload 字节不变。必须是最后一个字段，与服务端 mirror 一致。
	Cron []DesiredStateCronJob `json:"cron,omitempty"`
}

// DesiredStateCronJob 是下发给设备的定时任务引用（M5）。与服务端
// hermesUnsignedDesiredState.HermesDesiredStateCronJob 逐字一致（签名字节校验）。
type DesiredStateCronJob struct {
	ID             uint64 `json:"job_id"`
	Name           string `json:"name"`
	CronExpression string `json:"cron_expression"`
	Prompt         string `json:"prompt"`
	Enabled        bool   `json:"enabled"`
}

type DesiredState struct {
	UnsignedDesiredState
	Signature string `json:"signature"`
}

type Heartbeat struct {
	ManagerVersion        string `json:"manager_version"`
	HermesVersion         string `json:"hermes_version"`
	ActualState           string `json:"actual_state"`
	HealthStatus          string `json:"health_status"`
	AppliedPolicyRevision uint64 `json:"applied_policy_revision"`
	ConfigDigest          string `json:"config_digest"`
	LastErrorCode         string `json:"last_error_code"`
	// APIServerFingerprint 是端侧自持的 Gateway API server 密钥的 sha256 前 16
	// 字节指纹（32 hex）。控制面只存指纹，不持有原始 key；空串表示未启用。
	APIServerFingerprint string `json:"api_server_fingerprint,omitempty"`
}

// ModelToken 是控制面签发的短期模型访问凭据（TTL 约 30 分钟，服务端换发即
// 吊销旧 token）。manager 把它写进受管 profile 的 .env 供本地 Hermes 调用
// 企业模型代理；明文只在内存与该 .env 里存在。
type ModelToken struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type DeviceEvent struct {
	EventType             string    `json:"event_type"`
	Result                string    `json:"result"`
	ErrorCode             string    `json:"error_code"`
	DesiredPolicyRevision uint64    `json:"desired_policy_revision"`
	AppliedPolicyRevision uint64    `json:"applied_policy_revision"`
	OccurredAt            time.Time `json:"occurred_at"`
}
