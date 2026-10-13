// Package adapter 定义 manager 在员工电脑上执行期望状态的能力面。
//
// 通用核心（协议/状态机）只依赖本接口；平台差异（launchd / 计划任务）与
// 安装细节（uv venv）都收在实现里。接口刻意只表达预定义动作——控制面
// 不下发任意命令，适配器也不提供执行任意命令的入口。
package adapter

import (
	"context"
	"errors"

	"github.com/go-admin-kit/clients/hermes-manager/internal/protocol"
)

// 错误码枚举：上报给控制面的结构化 error_code，不携带日志正文。
// 与服务端 hermesErrorCodePattern（大写字母/数字/下划线等）兼容。
const (
	ErrCodeDownloadFailed    = "DOWNLOAD_FAILED"
	ErrCodeDigestMismatch    = "DIGEST_MISMATCH"
	ErrCodeExtractFailed     = "EXTRACT_FAILED"
	ErrCodeBootstrapFailed   = "BOOTSTRAP_FAILED" // uv 引导下载/校验失败
	ErrCodeVenvFailed        = "VENV_FAILED"
	ErrCodeHealthCheckFailed = "HEALTH_CHECK_FAILED"
	ErrCodeStartFailed       = "START_FAILED"
	ErrCodeStopFailed        = "STOP_FAILED"
	ErrCodeProfileFailed     = "PROFILE_APPLY_FAILED"
	ErrCodeRollbackFailed    = "ROLLBACK_FAILED"
	ErrCodeDiskSpaceLow      = "DISK_SPACE_LOW"
	ErrCodeNotInstalled      = "HERMES_NOT_INSTALLED"
	ErrCodeSkillInstallFailed = "SKILL_INSTALL_FAILED"
)

// CodedError 把底层错误折成可上报的错误码。Unwrap 保留原因链便于本地排障；
// 上报控制面时只取 Code。
type CodedError struct {
	Code string
	Err  error
}

func (e *CodedError) Error() string {
	if e.Err == nil {
		return e.Code
	}
	return e.Code + ": " + e.Err.Error()
}

func (e *CodedError) Unwrap() error { return e.Err }

// Coded 构造带错误码的错误。
func Coded(code string, err error) error {
	return &CodedError{Code: code, Err: err}
}

// CodeOf 提取错误码；非 CodedError 一律归为 UNKNOWN。
func CodeOf(err error) string {
	if err == nil {
		return ""
	}
	var coded *CodedError
	if errors.As(err, &coded) {
		return coded.Code
	}
	return "UNKNOWN"
}

// Adapter 是平台执行层。所有方法幂等：重复 Start 已运行的实例、Stop 已停止
// 的实例都应当成功返回，reconcile 循环依赖这一点简化重试。
type Adapter interface {
	// Install 下载并安装钉版制品：校验 sha256 → 解压 → 建 venv → 装依赖 →
	// 健康检查 → 切换 current 指针。失败不改变当前活跃版本。
	Install(ctx context.Context, artifact protocol.DesiredStateArtifact) error
	// InstalledVersion 返回 current 指针指向的版本；未安装返回空串（非错误）。
	InstalledVersion() (string, error)
	// ApplyProfile 幂等渲染受管 profile（config.yaml/.env），返回受管字段摘要。
	// changed=true 表示重写前磁盘上的受管内容与期望渲染不一致——由调用方结合
	// 策略 revision 判断这是员工篡改（同 revision 下不一致 = 漂移）还是正常的
	// 策略变更。modelToken 为空表示本轮不更新凭据（保留 .env 现值）。
	ApplyProfile(ctx context.Context, desired *protocol.DesiredState, modelToken string) (digest string, changed bool, err error)
	// ApplyModelToken 只轮换受管模型凭据，不改写 config.yaml。更新失败回滚
	// 或命中失败目标隔离时，manager 需要保留旧策略配置但仍让新 token 生效。
	ApplyModelToken(ctx context.Context, modelToken string) error
	// ApplyAPIServerKey 注入/移除 Gateway API server 凭据到 profile .env
	// （API_SERVER_KEY/HOST/PORT）。非空 → upsert 三个键；空 → remove 三个键
	// （profile 回落到 company 时停止暴露 API server）。幂等。
	ApplyAPIServerKey(ctx context.Context, apiServerKey string) error
	// PushTask 把控制面下发的任务意图推给本地 Hermes（Gateway API server
	// 127.0.0.1:8642），返回执行结果文本。apiServerKey 是 manager 自持的密钥。
	PushTask(ctx context.Context, task protocol.DesiredStateTask, apiServerKey string) (string, error)
	// ApplyWorkerProfiles 渲染每个 kanban worker 的受管 profile
	// （<root>/profiles/<name>/config.yaml + .env）。模型 token 从 orchestrator
	// .env 拷贝；工具集 = 集群策略集去掉 delegation + kanban 进顶层 toolsets；
	// kanban spawn 限流写进配置。幂等。
	ApplyWorkerProfiles(ctx context.Context, desired *protocol.DesiredState) error
	// EnsureKanbanBoard 确保命名 board 存在（kanban boards create，幂等）。
	EnsureKanbanBoard(ctx context.Context, boardSlug string) error
	// CreateKanbanTask 创建 kanban 任务；idempotencyKey 保证重试安全（同 key
	// 返回既有任务 id）。返回 kanban 任务 id（t_<hex>）。
	CreateKanbanTask(ctx context.Context, task protocol.DesiredStateKanban, idempotencyKey string) (string, error)
	// ShowKanbanTask 返回 kanban 任务当前状态与最新 summary（worker 交接）。
	ShowKanbanTask(ctx context.Context, boardSlug, taskID string) (*protocol.KanbanTaskStatus, error)
	// ApplySkills 把已由 manager 下载并校验的 skill tarball 解压进受管 profile
	// 的 skills 树（orchestrator + 每个 worker profile）。bundles 与签名的
	// DesiredStateSkill 一一对应；tarball 数据在 manager 层（设备签名下载）。
	// 幂等：目标目录已存在同内容时覆盖重写，不报错。
	ApplySkills(ctx context.Context, bundles []SkillBundle) error
	// ApplyMcpSecrets 把 MCP secret 值写进 orchestrator + 每个 worker profile 的
	// .env（MCP_* 前缀），并移除 desired 集之外的 MCP_* 键（幂等）。config.yaml
	// 的 mcp_servers 块由 ApplyProfile 渲染，这里只补 .env 密钥。
	ApplyMcpSecrets(ctx context.Context, secrets map[string]string) error
	// Start / Stop / Restart 控制受监管的 hermes gateway 进程。
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Restart(ctx context.Context) error
	Running(ctx context.Context) (bool, error)
	// Rollback 把 current 指针切回上一版本并重启。没有上一版本时报错。
	Rollback(ctx context.Context) error
}
