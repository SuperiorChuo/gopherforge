package adapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-admin-kit/clients/hermes-manager/internal/protocol"
)

// 上游配置键名（对钉定版本 NousResearch/hermes-agent@v2026.7.20 实测校准）：
//   - 命名端点走 providers.<名字>，条目支持 base_url 与 key_env
//     （cli-config.yaml.example:100-105；config.py:5532 注明 providers 字典是
//     custom_providers 列表的现代等价形式）
//   - model.provider 填该条目名即可选中它（runtime_provider.py:660-675）
//   - 模型名键为 model.default（cli-config.yaml.example:10-11 明确 default/model 皆可）
//   - 平台工具白名单 = platform_toolsets.cli（cli-config.yaml.example:811+）
//   - 无人值守护栏 = tool_loop_guardrails（cli-config.yaml.example:376-386，产品既定必开）
//
// 凭据为什么不用 OPENAI_API_KEY：上游把每个厂商 key 按 host 门控，
// OPENAI_API_KEY 只在 base_url 落在 openai.com / openai.azure.com 时才进候选
// （runtime_provider.py:1142-1157，上游注释引 issue #28660 与 GHSA-76xc-57q6-vm5m，
// 是防凭据泄漏到无关端点的有意设计）。企业代理是自有域名，那个键永远取不到，
// 而 runtime_provider.py:1182 会把空 key 兜底成字面量 "no-key-required" 直接发出去——
// 401 且不报配置错误，doctor 也只查 base_url 在不在，三道关全静默。
// key_env 是唯一与 host 无关的通路（runtime_provider.py:1017，且优先于 host 门控），
// 同时让凭据留在 .env 而不进 config.yaml：digest 不随换 token 变动，
// 批3「只在 TTL 窗口换、换完重启一次」的轮换设计得以保住。
//
// config.yaml 整文件受管（个性化走 SOUL.md 等其他文件，manager 不碰）；
// .env 只管一个受管键，员工自有变量原样保留。
const (
	// managedProviderName 是受管命名端点，同时作为 model.provider 的取值。
	managedProviderName = "gak-company"
	envManagedKey       = "GAK_HERMES_MODEL_TOKEN"
	// legacyEnvManagedKey 是 ≤批3 写入的键。它由 manager 写入、也只可能由
	// manager 写入，且值是企业代理的凭据——留在盘上既无用又有害（策略一旦
	// 把 base_url 指向 openai.com，这个已吊销的企业 token 就会被发出去），
	// 每轮渲染顺手清掉。
	legacyEnvManagedKey = "OPENAI_API_KEY"
	// Gateway API server 凭据（三期 manager 下发通道）。上游 gateway 在 .env
	// 出现 API_SERVER_KEY（≥16 字符）时自动启用 loopback api_server 平台；
	// HOST/PORT 钉死 loopback，防止暴露到局域网。manager 自持原始 key，控制面
	// 只见 sha256 指纹。
	envAPIServerKey  = "API_SERVER_KEY"
	envAPIServerHost = "API_SERVER_HOST"
	envAPIServerPort = "API_SERVER_PORT"
	apiServerHost    = "127.0.0.1"
	apiServerPort    = "8642"
	// managedConfigVersion 必须跟钉定的 Hermes 版本走：缺这个键会被判成 v0，
	// doctor 报 "Config version outdated" 并劝员工跑 `hermes doctor --fix` /
	// `hermes setup`，而迁移会重写整个 config.yaml（config.py:6545）——那是
	// manager 全权受管的文件，会被下轮 sync 判成漂移再覆盖回去，来回打架。
	// 换制品版本时同步核对上游 config.py 的 DEFAULT_CONFIG["_config_version"]。
	managedConfigVersion = 33
)

// renderManagedConfig 生成受管 config.yaml。确定性输出：digest 与漂移检测
// 都建立在「同一策略渲染结果字节一致」之上。
func renderManagedConfig(desired *protocol.DesiredState) string {
	var builder strings.Builder
	builder.WriteString("# 本文件由 gak-hermes-manager 按企业策略生成，手工修改会在下次同步时被覆盖并记录漂移。\n")
	fmt.Fprintf(&builder, "# policy_revision: %d\n\n", desired.Revision)

	fmt.Fprintf(&builder, "_config_version: %d\n\n", managedConfigVersion)

	builder.WriteString("model:\n")
	if desired.Model.Mode == "company_proxy" {
		fmt.Fprintf(&builder, "  provider: %q\n", managedProviderName)
	}
	if desired.Model.Model != "" {
		fmt.Fprintf(&builder, "  default: %q\n", desired.Model.Model)
	}

	// 命名端点单独成段：base_url 与凭据变量名都挂在这里，凭据本身不进本文件。
	if desired.Model.Mode == "company_proxy" {
		builder.WriteString("\nproviders:\n")
		fmt.Fprintf(&builder, "  %s:\n", managedProviderName)
		fmt.Fprintf(&builder, "    base_url: %q\n", desired.Model.BaseURL)
		fmt.Fprintf(&builder, "    key_env: %q\n", envManagedKey)
	}

	// M3 MCP 管理：有 MCP server 时渲染工具集剔除 no_mcp（上游哨兵：无
	// no_mcp = MCP ON，自动含所有 enabled server）。
	mcpEnabled := len(desired.McpServers) > 0
	toolsets := append([]string(nil), desired.Toolsets...)
	if mcpEnabled {
		toolsets = mcpToolsets(toolsets)
	}
	sort.Strings(toolsets)
	builder.WriteString("\nplatform_toolsets:\n  cli: [")
	for index, tool := range toolsets {
		if index > 0 {
			builder.WriteString(", ")
		}
		fmt.Fprintf(&builder, "%q", tool)
	}
	builder.WriteString("]\n")
	// 三期：company_cluster 同时开放 api_server 平台（loopback API server），
	// 工具集与 cli 一致（都来自策略白名单）。company 不渲染此段，保持字节不变。
	if desired.Profile == "company_cluster" {
		builder.WriteString("  api_server: [")
		for index, tool := range toolsets {
			if index > 0 {
				builder.WriteString(", ")
			}
			fmt.Fprintf(&builder, "%q", tool)
		}
		builder.WriteString("]\n")
		// Kanban 里程碑：company_cluster 是 orchestrator，顶层 toolsets 需含
		// kanban（kanban_list/unblock/create 工具由顶层 toolsets 门控，非
		// platform_toolsets.cli）；并写 spawn 限流（orchestrator 自身也可被派生）。
		orchestratorToolsets := append(append([]string(nil), toolsets...), "kanban")
		builder.WriteString("toolsets: [")
		for index, tool := range orchestratorToolsets {
			if index > 0 {
				builder.WriteString(", ")
			}
			fmt.Fprintf(&builder, "%q", tool)
		}
		builder.WriteString("]\n")
		fmt.Fprintf(&builder, "kanban:\n  max_in_progress: %d\n  max_in_progress_per_profile: %d\n  failure_limit: %d\n",
			kanbanMaxInProgress, kanbanMaxInProgressPerProfile, kanbanFailureLimit)
	}
	if mcpEnabled {
		writeMcpServersBlock(&builder, desired.McpServers)
	}

	// 无人值守护栏：员工机上的 gateway 没人盯着告警，必须硬停
	builder.WriteString("\ntool_loop_guardrails:\n")
	builder.WriteString("  hard_stop_enabled: true\n")
	builder.WriteString("  hard_stop_after:\n")
	builder.WriteString("    exact_failure: 5\n")
	builder.WriteString("    idempotent_no_progress: 5\n")
	return builder.String()
}

// mcpToolsets 在启用 MCP 时从工具集列表剔除 no_mcp（上游哨兵：无 no_mcp = MCP ON）。
func mcpToolsets(toolsets []string) []string {
	out := make([]string, 0, len(toolsets))
	for _, tool := range toolsets {
		if tool != "no_mcp" {
			out = append(out, tool)
		}
	}
	return out
}

// writeMcpServersBlock 渲染 config.yaml 的 mcp_servers 块（Claude-Desktop 风格，
// 上游读小写 mcp_servers）。stdio → command/args/env；sse/http → url/transport/headers。
// 密钥只以 ${VAR} 模板引用，明文在 .env。
func writeMcpServersBlock(builder *strings.Builder, servers []protocol.DesiredStateMcpServer) {
	builder.WriteString("\nmcp_servers:\n")
	for _, server := range servers {
		fmt.Fprintf(builder, "  %q:\n", server.Name)
		builder.WriteString("    enabled: true\n")
		switch server.Type {
		case "stdio":
			fmt.Fprintf(builder, "    command: %q\n", server.Command)
			if len(server.Args) > 0 {
				builder.WriteString("    args: [")
				for index, arg := range server.Args {
					if index > 0 {
						builder.WriteString(", ")
					}
					fmt.Fprintf(builder, "%q", arg)
				}
				builder.WriteString("]\n")
			}
			if len(server.Env) > 0 {
				builder.WriteString("    env:\n")
				writeMcpSecretMap(builder, server.Env, "      ")
			}
		case "sse":
			fmt.Fprintf(builder, "    url: %q\n", server.URL)
			builder.WriteString("    transport: \"sse\"\n")
			if len(server.Headers) > 0 {
				builder.WriteString("    headers:\n")
				writeMcpSecretMap(builder, server.Headers, "      ")
			}
		case "http":
			fmt.Fprintf(builder, "    url: %q\n", server.URL)
			if len(server.Headers) > 0 {
				builder.WriteString("    headers:\n")
				writeMcpSecretMap(builder, server.Headers, "      ")
			}
		}
		if len(server.ToolsInclude) > 0 {
			builder.WriteString("    tools:\n")
			builder.WriteString("      include: [")
			for index, name := range server.ToolsInclude {
				if index > 0 {
					builder.WriteString(", ")
				}
				fmt.Fprintf(builder, "%q", name)
			}
			builder.WriteString("]\n")
		}
		if server.Timeout > 0 {
			fmt.Fprintf(builder, "    timeout: %d\n", server.Timeout)
		}
	}
}

func writeMcpSecretMap(builder *strings.Builder, pairs map[string]string, indent string) {
	keys := make([]string, 0, len(pairs))
	for key := range pairs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(builder, "%s%q: %q\n", indent, key, pairs[key])
	}
}

// ApplyProfile 幂等渲染受管 profile。changed 只看 config.yaml：.env 里的
// token 轮换是设计内行为，不算漂移，也不进 digest。
func (b *Base) ApplyProfile(_ context.Context, desired *protocol.DesiredState, modelToken string) (string, bool, error) {
	if desired == nil {
		return "", false, Coded(ErrCodeProfileFailed, errors.New("desired state is nil"))
	}
	rendered := renderManagedConfig(desired)
	configPath := filepath.Join(b.profileDir(), "config.yaml")

	changed := false
	existing, err := os.ReadFile(configPath)
	switch {
	case err == nil:
		changed = string(existing) != rendered
	case errors.Is(err, os.ErrNotExist):
		// 首次渲染不算 changed：没有旧内容就谈不上偏离
	default:
		return "", false, Coded(ErrCodeProfileFailed, err)
	}

	if changed || errors.Is(err, os.ErrNotExist) {
		if writeErr := atomicWriteFile(b.profileDir(), "config.yaml", []byte(rendered), 0o600); writeErr != nil {
			return "", false, Coded(ErrCodeProfileFailed, writeErr)
		}
	}

	if err := b.applyModelToken(modelToken); err != nil {
		return "", false, Coded(ErrCodeProfileFailed, err)
	}

	sum := sha256.Sum256([]byte(rendered))
	return hex.EncodeToString(sum[:]), changed, nil
}

func (b *Base) ApplyModelToken(_ context.Context, modelToken string) error {
	if modelToken == "" {
		return Coded(ErrCodeProfileFailed, errors.New("model token is empty"))
	}
	if err := b.applyModelToken(modelToken); err != nil {
		return Coded(ErrCodeProfileFailed, err)
	}
	return nil
}

func (b *Base) applyModelToken(modelToken string) error {
	if modelToken != "" {
		if err := b.upsertEnvKey(envManagedKey, modelToken); err != nil {
			return err
		}
	}
	// 每轮都清：升级到本版本的机器上，批3 写的 OPENAI_API_KEY 可能一直没赶上
	// 一次 token 轮换，不能等到轮换才顺带清掉。
	return b.removeEnvKey(legacyEnvManagedKey)
}

// applyAPIServer 注入/移除 Gateway API server 凭据到 profile .env。
// 非空 → upsert API_SERVER_KEY/HOST/PORT（HOST/PORT 钉死 loopback）；
// 空 → remove 三个键（profile 回落到 company 时停止暴露 API server）。幂等。
func (b *Base) applyAPIServer(apiServerKey string) error {
	if apiServerKey == "" {
		if err := b.removeEnvKey(envAPIServerKey); err != nil {
			return err
		}
		if err := b.removeEnvKey(envAPIServerHost); err != nil {
			return err
		}
		return b.removeEnvKey(envAPIServerPort)
	}
	if err := b.upsertEnvKey(envAPIServerKey, apiServerKey); err != nil {
		return err
	}
	if err := b.upsertEnvKey(envAPIServerHost, apiServerHost); err != nil {
		return err
	}
	return b.upsertEnvKey(envAPIServerPort, apiServerPort)
}

// ApplyAPIServerKey 实现 Adapter 接口：把 API server 凭据写进受管 profile。
func (b *Base) ApplyAPIServerKey(_ context.Context, apiServerKey string) error {
	return b.applyAPIServer(apiServerKey)
}

// upsertEnvKey 原子更新 .env 中的受管键；员工自有行原样保留。
func (b *Base) upsertEnvKey(key, value string) error {
	envPath := filepath.Join(b.profileDir(), ".env")
	var lines []string
	if data, err := os.ReadFile(envPath); err == nil {
		lines = strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	replaced := false
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, key+"=") {
			lines[index] = key + "=" + value
			replaced = true
		}
	}
	if !replaced {
		lines = append(lines, key+"="+value)
	}
	content := strings.Join(lines, "\n") + "\n"
	return atomicWriteFile(b.profileDir(), ".env", []byte(content), 0o600)
}

// removeEnvKey 删掉一个曾由 manager 受管、现已作废的键。文件不存在或键不在
// 都算成功（幂等）；员工自有行原样保留。
func (b *Base) removeEnvKey(key string) error {
	envPath := filepath.Join(b.profileDir(), ".env")
	data, err := os.ReadFile(envPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	kept := make([]string, 0, len(lines))
	removed := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), key+"=") {
			removed = true
			continue
		}
		kept = append(kept, line)
	}
	if !removed {
		return nil
	}

	if len(kept) == 0 || (len(kept) == 1 && kept[0] == "") {
		return atomicWriteFile(b.profileDir(), ".env", nil, 0o600)
	}
	return atomicWriteFile(b.profileDir(), ".env", []byte(strings.Join(kept, "\n")+"\n"), 0o600)
}
