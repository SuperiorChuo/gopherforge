package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-admin-kit/clients/hermes-manager/internal/protocol"
)

// 三期后续：kanban 里程碑。manager 通过 hermes kanban CLI 驱动上游 dispatcher
// 做真多 Agent fan-out。所有任务正文/worker 名作为 argv 元素传递（Runner 用
// exec.CommandContext 无 shell），绝不 shell 拼接——worker 名会落到文件系统。

// kanban spawn 限流（DoS 治理，对应上游 issue #29034 的无界派生风险）。
const (
	// kanbanMaxInProgress 整板并发 worker 上限。
	kanbanMaxInProgress = 4
	// kanbanMaxInProgressPerProfile 单 worker profile 并发上限。
	kanbanMaxInProgressPerProfile = 1
	// kanbanFailureLimit 上游默认 circuit breaker，显式写出。
	kanbanFailureLimit = 2
)

// kanbanTaskIDPattern 匹配 kanban create 输出里的任务 id（t_<8hex>）。
var kanbanTaskIDPattern = regexp.MustCompile(`t_[0-9a-f]{8}`)

// kanbanWorkerNamePattern 是 kanban worker profile 名白名单（与 protocol 侧
// validateKanban 同构）。worker 名会成为文件系统目录名，必须安全校验。
var kanbanWorkerNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)

// workerConfigToolsets 是 worker profile 的顶层 toolsets。worker 是叶子，不再
// 派生（去掉 delegation）；kanban 生命周期工具由 dispatcher 派生时自动 force，
// 但顶层列出让独立可观测。含 skills/mcp（策略白名单子集）。
func workerConfigToolsets(clusterToolsets []string) []string {
	seen := make(map[string]bool, 8)
	out := make([]string, 0, 8)
	for _, tool := range clusterToolsets {
		if tool == "delegation" {
			continue // worker 是叶子，不再派生
		}
		if !seen[tool] {
			seen[tool] = true
			out = append(out, tool)
		}
	}
	if !seen["kanban"] {
		out = append(out, "kanban")
	}
	return out
}

// ApplyWorkerProfiles 渲染每个 kanban worker 的受管 profile。
func (b *Base) ApplyWorkerProfiles(_ context.Context, desired *protocol.DesiredState) error {
	if desired == nil || desired.Kanban == nil {
		return nil
	}
	// orchestrator .env 的当前模型 token，拷贝给 worker（同一信任边界）
	modelToken := b.readEnvKey(b.profileDir(), envManagedKey)
	for _, worker := range desired.Kanban.Workers {
		if !kanbanWorkerNamePattern.MatchString(worker.Name) {
			return Coded(ErrCodeProfileFailed, fmt.Errorf("unsafe kanban worker name %q", worker.Name))
		}
		dir := filepath.Join(b.profileDir(), "profiles", worker.Name)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return Coded(ErrCodeProfileFailed, err)
		}
		if err := atomicWriteFile(dir, "config.yaml", []byte(renderWorkerConfig(desired, worker.Name)), 0o600); err != nil {
			return Coded(ErrCodeProfileFailed, err)
		}
		if modelToken != "" {
			if err := atomicWriteFile(dir, ".env", []byte(envManagedKey+"="+modelToken+"\n"), 0o600); err != nil {
				return Coded(ErrCodeProfileFailed, err)
			}
		}
	}
	return nil
}

// EnsureKanbanBoard 确保命名 board 存在（幂等：已存在也返回成功）。
func (b *Base) EnsureKanbanBoard(ctx context.Context, boardSlug string) error {
	binary, err := b.hermesBinaryForRun()
	if err != nil {
		return err
	}
	// boards create 幂等（mkdir -p 语义），已存在不报错
	if _, err := b.runner.Run(ctx, b.profileDir(), b.kanbanEnv(), binary,
		"-p", b.profileProfile(), "kanban", "--board", boardSlug, "boards", "create", boardSlug); err != nil {
		return Coded(ErrCodeProfileFailed, err)
	}
	return nil
}

// CreateKanbanTask 创建 kanban 任务；idempotencyKey 保证重试安全。返回 t_<hex>。
func (b *Base) CreateKanbanTask(ctx context.Context, task protocol.DesiredStateKanban, idempotencyKey string) (string, error) {
	binary, err := b.hermesBinaryForRun()
	if err != nil {
		return "", err
	}
	args := []string{
		"-p", b.profileProfile(),
		"kanban", "--board", task.BoardSlug,
		"create", task.Title,
		"--body", task.Body,
		"--assignee", task.Assignee,
		"--idempotency-key", idempotencyKey,
	}
	if task.Model != "" {
		args = append(args, "-m", task.Model, "--provider", managedProviderName)
	}
	output, err := b.runner.Run(ctx, b.profileDir(), b.kanbanEnv(), binary, args...)
	if err != nil {
		return "", Coded(ErrCodeProfileFailed, err)
	}
	taskID := kanbanTaskIDPattern.FindString(string(output))
	if taskID == "" {
		return "", Coded(ErrCodeProfileFailed, fmt.Errorf("kanban create did not return a task id"))
	}
	return taskID, nil
}

// ShowKanbanTask 返回 kanban 任务当前状态与最新 summary。
func (b *Base) ShowKanbanTask(ctx context.Context, boardSlug, taskID string) (*protocol.KanbanTaskStatus, error) {
	binary, err := b.hermesBinaryForRun()
	if err != nil {
		return nil, err
	}
	output, err := b.runner.Run(ctx, b.profileDir(), b.kanbanEnv(), binary,
		"-p", b.profileProfile(), "kanban", "--board", boardSlug, "show", taskID, "--json")
	if err != nil {
		return nil, Coded(ErrCodeProfileFailed, err)
	}
	var parsed struct {
		ID            string `json:"id"`
		Status        string `json:"status"`
		LatestSummary string `json:"latest_summary"`
		Runs          []struct {
			Summary string `json:"summary"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(output, &parsed); err != nil {
		return nil, Coded(ErrCodeProfileFailed, fmt.Errorf("parse kanban show: %w", err))
	}
	summary := parsed.LatestSummary
	if summary == "" && len(parsed.Runs) > 0 {
		for i := len(parsed.Runs) - 1; i >= 0; i-- {
			if parsed.Runs[i].Summary != "" {
				summary = parsed.Runs[i].Summary
				break
			}
		}
	}
	return &protocol.KanbanTaskStatus{
		ID:            parsed.ID,
		Status:        parsed.Status,
		LatestSummary: summary,
	}, nil
}

// hermesBinaryForRun 返回当前安装版本的 hermes 可执行文件路径。
func (b *Base) hermesBinaryForRun() (string, error) {
	version, err := b.InstalledVersion()
	if err != nil {
		return "", err
	}
	if version == "" {
		return "", Coded(ErrCodeNotInstalled, errors.New("hermes not installed"))
	}
	return b.hermesBinary(version), nil
}

// kanbanEnv 返回 kanban CLI 调用的环境变量：HERMES_HOME 指向 orchestrator
// profile（与 gateway 共享同一 root，board/worker 落 <root>/kanban、<root>/profiles）。
func (b *Base) kanbanEnv() []string {
	return []string{"HERMES_HOME=" + b.profileDir()}
}

// readEnvKey 读指定目录 .env 里某键的值；文件不存在或键不在返回空串。
func (b *Base) readEnvKey(dir, key string) string {
	data, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, key+"=") {
			return strings.TrimPrefix(trimmed, key+"=")
		}
	}
	return ""
}

// renderWorkerConfig 渲染 worker 的 config.yaml。worker 是叶子（去 delegation），
// 含 kanban 顶层工具集 + spawn 限流。
func renderWorkerConfig(desired *protocol.DesiredState, name string) string {
	var builder strings.Builder
	builder.WriteString("# generated by gak-hermes-manager for kanban worker ")
	builder.WriteString(name)
	builder.WriteString("\n")
	fmt.Fprintf(&builder, "_config_version: %d\n\n", managedConfigVersion)

	builder.WriteString("model:\n")
	if desired.Model.Mode == "company_proxy" {
		fmt.Fprintf(&builder, "  provider: %q\n", managedProviderName)
	}
	if desired.Model.Model != "" {
		fmt.Fprintf(&builder, "  default: %q\n", desired.Model.Model)
	}

	if desired.Model.Mode == "company_proxy" {
		builder.WriteString("\nproviders:\n")
		fmt.Fprintf(&builder, "  %s:\n", managedProviderName)
		fmt.Fprintf(&builder, "    base_url: %q\n", desired.Model.BaseURL)
		fmt.Fprintf(&builder, "    key_env: %q\n", envManagedKey)
	}

	// 顶层 toolsets：worker 是叶子，去 delegation，含 kanban；有 MCP server 时
	// 剔除 no_mcp（上游哨兵：无 no_mcp = MCP ON）
	toolsets := workerConfigToolsets(desired.Toolsets)
	mcpEnabled := len(desired.McpServers) > 0
	if mcpEnabled {
		toolsets = mcpToolsets(toolsets)
	}
	builder.WriteString("\ntoolsets: [")
	for index, tool := range toolsets {
		if index > 0 {
			builder.WriteString(", ")
		}
		fmt.Fprintf(&builder, "%q", tool)
	}
	builder.WriteString("]\n")
	// M3：worker 也显式渲染 platform_toolsets.cli（MCP 平台门控读它）+ mcp_servers 块
	if mcpEnabled {
		builder.WriteString("platform_toolsets:\n  cli: [")
		for index, tool := range toolsets {
			if index > 0 {
				builder.WriteString(", ")
			}
			fmt.Fprintf(&builder, "%q", tool)
		}
		builder.WriteString("]\n")
		writeMcpServersBlock(&builder, desired.McpServers)
	}

	// kanban spawn 限流（DoS 治理）
	fmt.Fprintf(&builder, "\nkanban:\n  max_in_progress: %d\n  max_in_progress_per_profile: %d\n  failure_limit: %d\n",
		kanbanMaxInProgress, kanbanMaxInProgressPerProfile, kanbanFailureLimit)

	builder.WriteString("\ntool_loop_guardrails:\n")
	builder.WriteString("  hard_stop_enabled: true\n")
	builder.WriteString("  hard_stop_after:\n")
	builder.WriteString("    exact_failure: 5\n")
	builder.WriteString("    idempotent_no_progress: 5\n")
	return builder.String()
}
