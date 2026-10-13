package manager

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/go-admin-kit/clients/hermes-manager/internal/adapter"
	"github.com/go-admin-kit/clients/hermes-manager/internal/protocol"
	"github.com/go-admin-kit/clients/hermes-manager/internal/state"
)

const Version = "0.1.0-dev"

// mcpVarPattern 匹配 MCP 配置里的 ${VAR} 模板（大写字母数字下划线）。
var mcpVarPattern = regexp.MustCompile(`\$\{[A-Z][A-Z0-9_]*\}`)

type Manager struct {
	store      *state.Store
	httpClient *http.Client
	now        func() time.Time
	// random 是熵源，测试注入确定性 reader，生产用 crypto/rand。
	random io.Reader
	// adapter 是平台执行层。nil 时退化为只同步不执行（保留
	// PROCESS_ADAPTER_NOT_CONFIGURED 语义），供不支持的平台兜底。
	adapter adapter.Adapter
}

func New(store *state.Store, httpClient *http.Client) *Manager {
	return &Manager{store: store, httpClient: httpClient, now: time.Now, random: rand.Reader}
}

// SetAdapter 在装配处注入平台执行层。
func (m *Manager) SetAdapter(adp adapter.Adapter) { m.adapter = adp }

func (m *Manager) Enroll(ctx context.Context, serverURL, deviceName string, notify func(*protocol.DeviceAuthorization)) error {
	publicKey, privateKey, err := m.store.EnsureDeviceKey()
	if err != nil {
		return err
	}
	client, err := protocol.NewClient(serverURL, m.httpClient)
	if err != nil {
		return err
	}
	deviceName = strings.TrimSpace(deviceName)
	if deviceName == "" {
		deviceName = "Hermes device"
	}
	authorization, err := client.CreateDeviceAuthorization(ctx, protocol.DeviceAuthorizationRequest{
		PublicKey:  base64.RawStdEncoding.EncodeToString(publicKey),
		DeviceName: deviceName,
		Platform:   runtime.GOOS,
		Arch:       runtime.GOARCH,
	})
	if err != nil {
		return err
	}
	if notify != nil {
		notify(authorization)
	}

	expiresAt := m.now().Add(time.Duration(authorization.ExpiresIn) * time.Second)
	interval := time.Duration(authorization.Interval) * time.Second
	if interval < time.Second {
		interval = time.Second
	}
	for {
		identity, err := client.ExchangeDeviceCode(ctx, authorization.DeviceCode)
		if err == nil {
			return m.saveEnrollment(serverURL, publicKey, privateKey, identity)
		}
		var apiError *protocol.APIError
		if !errors.As(err, &apiError) || apiError.Code != "HERMES_AUTHORIZATION_PENDING" {
			return err
		}
		if !m.now().Add(interval).Before(expiresAt) {
			return errors.New("Hermes device authorization expired")
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (m *Manager) saveEnrollment(serverURL string, publicKey ed25519.PublicKey, privateKey ed25519.PrivateKey, identity *protocol.DeviceIdentity) error {
	if identity == nil || identity.DeviceID == "" || identity.PolicySigningPublicKey == "" {
		return errors.New("control plane returned an incomplete device identity")
	}
	if len(privateKey) != ed25519.PrivateKeySize {
		return errors.New("local device key is invalid")
	}
	return m.store.SaveConfig(&state.Config{
		ServerURL:              strings.TrimRight(strings.TrimSpace(serverURL), "/"),
		DeviceID:               identity.DeviceID,
		PublicKey:              base64.RawStdEncoding.EncodeToString(publicKey),
		PolicySigningPublicKey: identity.PolicySigningPublicKey,
		ManagerVersion:         Version,
		ActualState:            "registered",
		HealthStatus:           "unknown",
	})
}

// modelTokenRenewWindow 是提前续期窗口。服务端 TTL 30 分钟，留 10 分钟余量
// 足够覆盖一次同步周期加重启，又不至于频繁轮换（每次轮换都要重启 gateway）。
const modelTokenRenewWindow = 10 * time.Minute

const modelTokenRefreshFailed = "MODEL_TOKEN_REFRESH_FAILED"

// modelTokenDue 判断是否该换发模型 token：没有记录过到期时间（首次或旧版本
// 状态文件），或剩余 TTL 已进入续期窗口。
func (m *Manager) modelTokenDue(config *state.Config) bool {
	// stopped/revoked 时服务端会立即吊销旧 token。恢复 active 时
	// 即使本地记录的到期时间尚未到，也必须重新换发。
	if config.ActualState != "active" {
		return true
	}
	if config.ModelTokenExpiresAt.IsZero() {
		return true
	}
	return !m.now().Add(modelTokenRenewWindow).Before(config.ModelTokenExpiresAt)
}

// modelTokenRequired 区分“提前续期失败但旧 token 仍可用”和“本机没有任何
// 可继续使用的凭据”。后者必须 fail closed，不能把刚从 stopped 恢复的
// Hermes 用已被服务端吊销的旧 token 拉起来。
func (m *Manager) modelTokenRequired(config *state.Config) bool {
	return config.ActualState != "active" || config.ModelTokenExpiresAt.IsZero() ||
		!m.now().Before(config.ModelTokenExpiresAt)
}

// ensureAPIServerKey 确保 company_cluster + active 时 API server 密钥存在。
// 只在首轮生成（manager.json 已有则复用，保证崩溃重入不重复生成）；回落到
// company 或非 active 时清空（.env 里的键由 reconcile ApplyAPIServer 移除）。
// 指纹 = sha256(key)[:16] 的 hex（控制面只存指纹）。
func (m *Manager) ensureAPIServerKey(config *state.Config, desired *protocol.DesiredState) (key, fingerprint string, err error) {
	activeCluster := desired.DesiredState == "active" && desired.Profile == "company_cluster"
	if !activeCluster {
		return "", "", nil
	}
	if config.APIServerKey != "" {
		return config.APIServerKey, fingerprintOfAPIServerKey(config.APIServerKey), nil
	}
	raw := make([]byte, 32)
	if _, err := io.ReadFull(m.random, raw); err != nil {
		return "", "", fmt.Errorf("generate api server key: %w", err)
	}
	key = "ask_" + base64.RawURLEncoding.EncodeToString(raw)
	return key, fingerprintOfAPIServerKey(key), nil
}

func fingerprintOfAPIServerKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:16])
}

func (m *Manager) ensureStopped(ctx context.Context) (string, string) {
	running, err := m.adapter.Running(ctx)
	if err != nil {
		return "error", adapter.CodeOf(err)
	}
	if !running {
		return "stopped", ""
	}
	if err := m.adapter.Stop(ctx); err != nil {
		return "error", adapter.CodeOf(err)
	}
	if err := waitForStopped(ctx, m.adapter); err != nil {
		return "error", adapter.CodeOf(err)
	}
	return "stopped", ""

}

func (m *Manager) stopForMissingModelToken(ctx context.Context) (string, string) {
	actualState, stopErrorCode := m.ensureStopped(ctx)
	if stopErrorCode != "" {
		return actualState, stopErrorCode
	}
	return actualState, modelTokenRefreshFailed
}

func sameFailedUpdate(config *state.Config, desired *protocol.DesiredState) bool {
	return config.FailedUpdateArtifactID > 0 &&
		config.FailedUpdateArtifactID == desired.Hermes.ID &&
		config.FailedUpdatePolicyRevision == desired.Revision &&
		config.FailedUpdateVersion == desired.Hermes.Version
}

// checkpointUpdate 在适配器第一次可能触碰旧版本前，把目标写入 manager.json。
// manager 若在 Install 的停旧、切指针、应用配置或启动确认之间退出，下一轮可
// 识别并继续该事务；后续任何失败仍走同一条自动回滚路径。
func (m *Manager) checkpointUpdate(config *state.Config, desired *protocol.DesiredState) error {
	if desired.DesiredState != "active" || desired.Hermes.ID == 0 ||
		(sameFailedUpdate(config, desired) && desired.PendingAction != "retry_update") {
		return nil
	}
	installedVersion, err := m.adapter.InstalledVersion()
	if err != nil || installedVersion == "" || installedVersion == desired.Hermes.Version {
		return nil
	}
	if config.UpdateInProgressArtifactID == desired.Hermes.ID &&
		config.UpdateInProgressPolicyRevision == desired.Revision &&
		config.UpdateInProgressVersion == desired.Hermes.Version {
		return nil
	}
	config.UpdateInProgressArtifactID = desired.Hermes.ID
	config.UpdateInProgressPolicyRevision = desired.Revision
	config.UpdateInProgressVersion = desired.Hermes.Version
	return m.store.SaveConfig(config)
}

func convergenceError(desired *protocol.DesiredState, config *state.Config) error {
	if desired == nil || config == nil {
		return errors.New("Hermes convergence state is incomplete")
	}
	if config.ActualState != desired.DesiredState {
		return fmt.Errorf("Hermes state did not converge: desired=%s actual=%s error=%s",
			desired.DesiredState, config.ActualState, config.LastErrorCode)
	}
	if config.AppliedPolicyRevision != desired.Revision {
		return fmt.Errorf("Hermes policy did not converge: desired_revision=%d applied_revision=%d error=%s",
			desired.Revision, config.AppliedPolicyRevision, config.LastErrorCode)
	}
	if config.LastErrorCode != "" {
		return fmt.Errorf("Hermes convergence reported error=%s", config.LastErrorCode)
	}
	switch desired.DesiredState {
	case "active":
		if config.HealthStatus != "healthy" {
			return fmt.Errorf("Hermes active state is not healthy: health=%s", config.HealthStatus)
		}
		if desired.Hermes.ID > 0 && config.HermesVersion != desired.Hermes.Version {
			return fmt.Errorf("Hermes version did not converge: desired=%s actual=%s",
				desired.Hermes.Version, config.HermesVersion)
		}
	case "stopped":
		if config.HealthStatus != "healthy" {
			return fmt.Errorf("Hermes stopped state is not healthy: health=%s", config.HealthStatus)
		}
	case "revoked":
		// 吊销后的健康状态没有运行中含义，unknown 是成功终态。
	default:
		return fmt.Errorf("unsupported Hermes desired state %q", desired.DesiredState)
	}
	return nil
}

func (m *Manager) Sync(ctx context.Context) (*protocol.DesiredState, error) {
	config, identity, client, err := m.clientIdentity()
	if err != nil {
		return nil, err
	}
	desired, err := client.DesiredState(ctx, identity)
	if err != nil {
		return nil, err
	}
	if err := m.store.SaveDesiredState(desired); err != nil {
		return nil, err
	}

	if m.adapter == nil {
		// 无适配器平台的兜底：只同步不执行，如实上报未配置。
		config.DesiredPolicyRevision = desired.Revision
		config.ActualState = "stopped"
		config.HealthStatus = "degraded"
		config.LastErrorCode = "PROCESS_ADAPTER_NOT_CONFIGURED"
		if desired.DesiredState == "revoked" {
			config.ActualState = "revoked"
		}
		if err := m.store.SaveConfig(config); err != nil {
			return desired, err
		}
		return desired, convergenceError(desired, config)
	}
	// 更新事务必须在模型 token 换发之前落盘。换发会立刻吊销旧 token；
	// 若先换发、再因状态文件写入失败退出，运行中的旧进程会失去凭据，且
	// manager 没有留下可恢复的更新断点。
	if err := m.checkpointUpdate(config, desired); err != nil {
		return desired, err
	}

	// active 且策略走企业代理时按需换模型 token。
	//
	// 刻意不每轮都换：服务端换发即吊销该设备旧 token，而上游 Hermes 只在
	// 启动时读 .env——每个同步周期（最短 15s）换一次会把正在跑的 gateway
	// 手里的凭据打废。因此只在「没有凭据」或「剩余 TTL 不足续期窗口」时换，
	// 换成功后由下面的 tokenRotated 触发一次重启让新凭据生效。
	//
	// 提前续期失败且旧 token 仍在 TTL 内时继续收敛并标 degraded；恢复 active、
	// 首次签发或 token 已过期时没有可用凭据，必须保持 stopped，等待下轮重试。
	modelToken := ""
	tokenRotated := false
	modelTokenExpiry := time.Time{}
	tokenRefreshFailed := false
	tokenRequired := false
	if desired.DesiredState == "active" && desired.Model.Mode == "company_proxy" {
		if m.modelTokenDue(config) {
			if token, tokenErr := client.IssueModelToken(ctx, identity); tokenErr == nil &&
				token != nil && strings.TrimSpace(token.Token) != "" && token.ExpiresAt.After(m.now()) {
				modelToken = token.Token
				tokenRotated = true
				modelTokenExpiry = token.ExpiresAt
			} else {
				tokenRefreshFailed = true
				tokenRequired = m.modelTokenRequired(config)
			}
		}
	}
	if tokenRefreshFailed && tokenRequired {
		config.DesiredPolicyRevision = desired.Revision
		config.ActualState, config.LastErrorCode = m.stopForMissingModelToken(ctx)
		config.HealthStatus = "unhealthy"
		if installedVersion, versionErr := m.adapter.InstalledVersion(); versionErr == nil {
			config.HermesVersion = installedVersion
		}
		if err := m.store.SaveConfig(config); err != nil {
			return desired, err
		}
		return desired, convergenceError(desired, config)
	}
	// 三期：company_cluster + active 时确保 API server 密钥存在。首轮生成并
	// 提前落盘（防 reconcile 中途崩溃丢 key），随后 reconcile 里的 ApplyAPIServer
	// 把它写进 profile .env。指纹随心跳上报，控制面只存指纹不持原始 key。
	//
	// 为什么在 reconcile 前生成：ApplyProfile 已把 config 写进 desired-state 摘要
	// 语义，而 API server 密钥不属于策略字段；在 reconcile 前生成 + SaveConfig 保证
	// 崩溃重入后不会重复生成（manager.json 里已有）。
	apiServerKey, apiServerFingerprint, err := m.ensureAPIServerKey(config, desired)
	if err != nil {
		return desired, err
	}
	if apiServerKey != config.APIServerKey || apiServerFingerprint != config.APIServerFingerprint {
		config.APIServerKey = apiServerKey
		config.APIServerFingerprint = apiServerFingerprint
		if err := m.store.SaveConfig(config); err != nil {
			return desired, err
		}
	}
	outcome := reconcile(ctx, m.adapter, config, desired, modelToken, tokenRotated, func(eventType, result, errorCode string, appliedRevision uint64) {
		// 事件上报失败不影响收敛；服务端靠心跳兜底可观测。
		_ = client.SendEvent(ctx, identity, protocol.DeviceEvent{
			EventType:             eventType,
			Result:                result,
			ErrorCode:             errorCode,
			DesiredPolicyRevision: desired.Revision,
			AppliedPolicyRevision: appliedRevision,
			OccurredAt:            m.now().UTC(),
		})
	})
	if tokenRotated && !outcome.ModelTokenApplied {
		// 服务端签发新 token 时已吊销旧值。新值没能写入 .env 时，继续运行
		// 旧进程只会持续 401；必须在写状态前确认它已经停下。
		actualState, stopErrorCode := m.ensureStopped(ctx)
		outcome.ActualState = actualState
		outcome.HealthStatus = "unhealthy"
		if stopErrorCode != "" {
			outcome.LastErrorCode = stopErrorCode
		}
	}
	applyOutcome(config, desired, outcome, m.now())
	if outcome.ModelTokenApplied && outcome.ActualState == "active" {
		config.ModelTokenExpiresAt = modelTokenExpiry
	}
	if tokenRefreshFailed && config.LastErrorCode == "" {
		config.LastErrorCode = modelTokenRefreshFailed
		if config.HealthStatus == "healthy" {
			config.HealthStatus = "degraded"
		}
	}
	if err := m.store.SaveConfig(config); err != nil {
		return desired, err
	}
	// 三期：任务下发。收敛（设备 healthy）后，把 desired-state 携带的任务意图
	// 推给本地 Hermes API server，成功后回传结果。失败不失败收敛（设备保持
	// healthy），靠 LastPushedRunID 幂等：下轮 sync 拿到同一个 run 时跳过已成功
	// 的，未成功的重试。
	// Kanban 里程碑：company_cluster + Kanban 指令时先确保 gateway 加载 kanban
	// 工具（config 摘要变则重启一次），再驱动 kanban fan-out（与 push 互斥）。
	if err := m.ensureKanbanGateway(ctx, desired, config); err != nil {
		return desired, err
	}
	// M2 技能市场：worker 先有技能再 fan-out（技能随集群下发到所有 profile）
	if err := m.ensureSkills(ctx, desired, config, client, identity); err != nil {
		return desired, err
	}
	// M3 MCP 管理：注入 mcp_servers 密钥到 .env（config 块由 ApplyProfile 已渲染）
	if err := m.ensureMcpServers(ctx, desired, config, client, identity); err != nil {
		return desired, err
	}
	m.dispatchKanban(ctx, desired, config, client, identity)
	m.dispatchTask(ctx, desired, config, client, identity)
	// M5 Cron：设备端评估租户 cron 任务，到点推提示词（门控 company_cluster）
	m.dispatchCron(ctx, desired, config, client, identity)
	return desired, convergenceError(desired, config)
}

// skillsDigest 是技能集摘要：对 "name@version@sha256|" 排序后 sha256。用于
// SkillsAppliedDigest 幂等——技能集不变则跳过下载/解压/重启。
func skillsDigest(skills []protocol.DesiredStateSkill) string {
	parts := make([]string, 0, len(skills))
	for _, skill := range skills {
		parts = append(parts, fmt.Sprintf("%s@%s@%s", skill.Name, skill.Version, skill.SHA256))
	}
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}

// ensureSkills 下载并应用 desired Skills 到受管 profile 的 skills 树。
// 幂等：SkillsAppliedDigest 一致则跳过；技能集变化 → DownloadSkill 全部 →
// ApplySkills 解压 → 重启 gateway 加载。失败发 skill_install failed 事件、
// 不失败收敛、不记 digest → 下轮重试。
func (m *Manager) ensureSkills(ctx context.Context, desired *protocol.DesiredState, config *state.Config, client *protocol.Client, identity *protocol.Identity) error {
	if len(desired.Skills) == 0 {
		if config.SkillsAppliedDigest != "" {
			config.SkillsAppliedDigest = ""
			if err := m.store.SaveConfig(config); err != nil {
				return err
			}
		}
		return nil
	}
	digest := skillsDigest(desired.Skills)
	if config.SkillsAppliedDigest == digest {
		return nil
	}
	if m.adapter == nil {
		return nil
	}
	bundles := make([]adapter.SkillBundle, 0, len(desired.Skills))
	for _, skill := range desired.Skills {
		tarball, err := client.DownloadSkill(ctx, identity, skill)
		if err != nil {
			m.emitSkillInstallFailed(ctx, client, identity, desired, config, "SKILL_DOWNLOAD_FAILED")
			return nil // 不失败收敛，下轮重试
		}
		// 下载后校验 sha256（控制面 payload 已带摘要）
		sum := sha256.Sum256(tarball)
		if !strings.EqualFold(hex.EncodeToString(sum[:]), skill.SHA256) {
			m.emitSkillInstallFailed(ctx, client, identity, desired, config, "SKILL_DIGEST_MISMATCH")
			return nil
		}
		bundles = append(bundles, adapter.SkillBundle{Skill: skill, Tarball: tarball})
	}
	if err := m.adapter.ApplySkills(ctx, bundles); err != nil {
		m.emitSkillInstallFailed(ctx, client, identity, desired, config, "SKILL_INSTALL_FAILED")
		return nil
	}
	// 技能集变化 → 重启 gateway 加载新技能（复用 kanban 的重启 + 等待模式）
	if err := m.adapter.Restart(ctx); err != nil {
		return adapter.Coded(adapter.ErrCodeStartFailed, err)
	}
	if err := waitForRunning(ctx, m.adapter); err != nil {
		return err
	}
	config.SkillsAppliedDigest = digest
	return m.store.SaveConfig(config)
}

// emitSkillInstallFailed 上报技能安装失败事件（不失败收敛，心跳兜底）。
func (m *Manager) emitSkillInstallFailed(ctx context.Context, client *protocol.Client, identity *protocol.Identity, desired *protocol.DesiredState, config *state.Config, errorCode string) {
	_ = client.SendEvent(ctx, identity, protocol.DeviceEvent{
		EventType:             "skill_install",
		Result:                "failed",
		ErrorCode:             errorCode,
		DesiredPolicyRevision: desired.Revision,
		AppliedPolicyRevision: config.AppliedPolicyRevision,
		OccurredAt:            m.now().UTC(),
	})
}

// mcpServersDigest 是 MCP server 集摘要：对 "name@type@url@command@secret_revision"
// 排序后 sha256。**含 secret_revision（密钥轮换 → 重新拉取 .env）不含 secret 值**。
func mcpServersDigest(servers []protocol.DesiredStateMcpServer) string {
	parts := make([]string, 0, len(servers))
	for _, server := range servers {
		parts = append(parts, fmt.Sprintf("%s@%s@%s@%s@%d",
			server.Name, server.Type, server.URL, server.Command, server.SecretRevision))
	}
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}

// ensureMcpServers 应用 desired McpServers 到受管 profile：渲染 config.yaml 的
// mcp_servers 块（ApplyProfile 已做）→ 拉取密钥写 .env → 重启 gateway 加载。
// 幂等：McpServersAppliedDigest 一致则跳过。失败发 mcp_install failed 事件、
// 不失败收敛、不记 digest → 下轮重试。
func (m *Manager) ensureMcpServers(ctx context.Context, desired *protocol.DesiredState, config *state.Config, client *protocol.Client, identity *protocol.Identity) error {
	if len(desired.McpServers) == 0 {
		if config.McpServersAppliedDigest != "" {
			config.McpServersAppliedDigest = ""
			if m.adapter != nil {
				if err := m.adapter.ApplyMcpSecrets(ctx, nil); err != nil {
					return adapter.Coded(adapter.ErrCodeProfileFailed, err)
				}
			}
			if err := m.store.SaveConfig(config); err != nil {
				return err
			}
			// 清空 MCP 后重启一次让 gateway 停用 server
			if m.adapter != nil {
				if err := m.adapter.Restart(ctx); err != nil {
					return adapter.Coded(adapter.ErrCodeStartFailed, err)
				}
				if err := waitForRunning(ctx, m.adapter); err != nil {
					return err
				}
			}
		}
		return nil
	}
	digest := mcpServersDigest(desired.McpServers)
	if config.McpServersAppliedDigest == digest {
		return nil
	}
	if m.adapter == nil {
		return nil
	}
	// 收集所有 ${VAR} 键（env + headers 引用）
	envKeys := make([]string, 0, 8)
	seen := map[string]bool{}
	for _, server := range desired.McpServers {
		for _, pair := range []map[string]string{server.Env, server.Headers} {
			for _, value := range pair {
				for _, token := range mcpVarPattern.FindAllString(value, -1) {
					varName := strings.TrimSuffix(strings.TrimPrefix(token, "${"), "}")
					if !seen[varName] {
						seen[varName] = true
						envKeys = append(envKeys, varName)
					}
				}
			}
		}
	}
	secrets, err := client.FetchMcpSecrets(ctx, identity, envKeys)
	if err != nil {
		m.emitMcpInstallFailed(ctx, client, identity, desired, config, "MCP_SECRETS_FETCH_FAILED")
		return nil // 不失败收敛，下轮重试
	}
	if err := m.adapter.ApplyMcpSecrets(ctx, secrets); err != nil {
		m.emitMcpInstallFailed(ctx, client, identity, desired, config, "MCP_SECRETS_APPLY_FAILED")
		return nil
	}
	if err := m.adapter.Restart(ctx); err != nil {
		return adapter.Coded(adapter.ErrCodeStartFailed, err)
	}
	if err := waitForRunning(ctx, m.adapter); err != nil {
		return err
	}
	config.McpServersAppliedDigest = digest
	return m.store.SaveConfig(config)
}

// emitMcpInstallFailed 上报 MCP 安装失败事件（不失败收敛，心跳兜底）。
func (m *Manager) emitMcpInstallFailed(ctx context.Context, client *protocol.Client, identity *protocol.Identity, desired *protocol.DesiredState, config *state.Config, errorCode string) {
	_ = client.SendEvent(ctx, identity, protocol.DeviceEvent{
		EventType:             "mcp_install",
		Result:                "failed",
		ErrorCode:             errorCode,
		DesiredPolicyRevision: desired.Revision,
		AppliedPolicyRevision: config.AppliedPolicyRevision,
		OccurredAt:            m.now().UTC(),
	})
}

// ensureKanbanGateway 确保 gateway 进程加载了 kanban 工具。company_cluster 且
// 带 Kanban 指令时，若运行中 gateway 的 config 摘要与已渲染摘要不一致（= 首次
// 进 kanban 或策略变更），重启一次让上游加载顶层 toolsets（含 kanban）。
// 重启失败等同 token 轮换失败，交由 convergenceError 让设备保持未收敛待重试。
func (m *Manager) ensureKanbanGateway(ctx context.Context, desired *protocol.DesiredState, config *state.Config) error {
	if desired == nil || desired.Kanban == nil || desired.Profile != "company_cluster" {
		config.KanbanConfigDigest = ""
		return nil
	}
	if config.KanbanConfigDigest == config.ConfigDigest {
		return nil // 运行中 gateway 已带 kanban 工具
	}
	if m.adapter == nil {
		return nil
	}
	if err := m.adapter.Restart(ctx); err != nil {
		return adapter.Coded(adapter.ErrCodeStartFailed, err)
	}
	if err := waitForRunning(ctx, m.adapter); err != nil {
		return err
	}
	config.KanbanConfigDigest = config.ConfigDigest
	return m.store.SaveConfig(config)
}

// dispatchKanban 驱动 kanban fan-out：渲染 worker profile → 确保 board → 建
// kanban 任务（幂等）→ 轮询 show → done 回传。KanbanReportedRunID 幂等。
// 失败发 task_dispatch failed 事件、不失败收敛（设备保持 healthy）、不记 run
// （下轮重试；create 幂等键保证重试不重复建任务）。
func (m *Manager) dispatchKanban(ctx context.Context, desired *protocol.DesiredState, config *state.Config, client *protocol.Client, identity *protocol.Identity) {
	if desired == nil || desired.Kanban == nil || desired.Profile != "company_cluster" || m.adapter == nil {
		return
	}
	if config.ActualState != "active" {
		return // gateway 未收敛先不派
	}
	if config.KanbanReportedRunID == desired.Kanban.RunID {
		return
	}
	if err := m.adapter.ApplyWorkerProfiles(ctx, desired); err != nil {
		m.emitKanbanDispatchFailed(ctx, client, identity, desired, config, "KANBAN_WORKER_PROFILE_FAILED")
		return
	}
	if err := m.adapter.EnsureKanbanBoard(ctx, desired.Kanban.BoardSlug); err != nil {
		m.emitKanbanDispatchFailed(ctx, client, identity, desired, config, "KANBAN_BOARD_FAILED")
		return
	}
	idempotencyKey := fmt.Sprintf("gak-kanban-%d-%d", desired.Kanban.TaskID, desired.Kanban.RunID)
	taskID, err := m.adapter.CreateKanbanTask(ctx, *desired.Kanban, idempotencyKey)
	if err != nil {
		m.emitKanbanDispatchFailed(ctx, client, identity, desired, config, "KANBAN_TASK_CREATE_FAILED")
		return
	}
	status, err := m.adapter.ShowKanbanTask(ctx, desired.Kanban.BoardSlug, taskID)
	if err != nil {
		m.emitKanbanDispatchFailed(ctx, client, identity, desired, config, "KANBAN_TASK_SHOW_FAILED")
		return
	}
	if status.Status != "done" {
		return // 未完成：下轮 sync 继续（create 幂等重取同一 id）
	}
	result := status.LatestSummary
	if result == "" {
		result = "kanban task completed"
	}
	if len(result) > 4000 {
		result = result[:4000]
	}
	config.KanbanReportedRunID = desired.Kanban.RunID
	if err := m.store.SaveConfig(config); err != nil {
		return
	}
	if reportErr := client.ReportTaskRun(ctx, identity, desired.Kanban.RunID, "completed", result); reportErr != nil {
		// 结果回传失败：run 已执行但控制面没收到。KanbanReportedRunID 已记，
		// 下轮不再重派（避免重复执行）；结果丢失留可观测。
		_ = reportErr
	}
}

// emitKanbanDispatchFailed 上报 kanban 派发失败事件（不失败收敛，心跳兜底）。
func (m *Manager) emitKanbanDispatchFailed(ctx context.Context, client *protocol.Client, identity *protocol.Identity, desired *protocol.DesiredState, config *state.Config, errorCode string) {
	_ = client.SendEvent(ctx, identity, protocol.DeviceEvent{
		EventType:             "task_dispatch",
		Result:                "failed",
		ErrorCode:             errorCode,
		DesiredPolicyRevision: desired.Revision,
		AppliedPolicyRevision: config.AppliedPolicyRevision,
		OccurredAt:            m.now().UTC(),
	})
}

// dispatchTask 把任务意图推给本地 Hermes 并回传结果。仅当：desired-state 带
// 任务、profile 为 company_cluster、API server 已启用（key 非空）、且该 run
// 尚未成功下发过（LastPushedRunID 幂等）。推成功 → 记 LastPushedRunID +
// 回传 completed；推失败 → 记 task_dispatch failed 事件，本轮不记 run（下轮
// 重试）。
func (m *Manager) dispatchTask(ctx context.Context, desired *protocol.DesiredState, config *state.Config, client *protocol.Client, identity *protocol.Identity) {
	if desired.Task == nil || desired.Profile != "company_cluster" || config.APIServerKey == "" {
		return
	}
	if config.LastPushedRunID == desired.Task.RunID {
		return
	}
	if m.adapter == nil {
		return
	}
	result, err := m.adapter.PushTask(ctx, *desired.Task, config.APIServerKey)
	if err != nil {
		if eventErr := client.SendEvent(ctx, identity, protocol.DeviceEvent{
			EventType:             "task_dispatch",
			Result:                "failed",
			ErrorCode:             "TASK_PUSH_FAILED",
			DesiredPolicyRevision: desired.Revision,
			AppliedPolicyRevision: config.AppliedPolicyRevision,
			OccurredAt:            m.now().UTC(),
		}); eventErr != nil {
			// 事件上报失败不影响收敛；心跳兜底可观测。
		}
		return
	}
	config.LastPushedRunID = desired.Task.RunID
	if err := m.store.SaveConfig(config); err != nil {
		return
	}
	if reportErr := client.ReportTaskRun(ctx, identity, desired.Task.RunID, "completed", result); reportErr != nil {
		// 结果回传失败：run 已执行但控制面没收到。任务会在下轮 sync 因
		// LastPushedRunID 已记而不再重推，避免重复执行；结果丢失留可观测。
		_ = reportErr
	}
}

func (m *Manager) Heartbeat(ctx context.Context) error {
	config, identity, client, err := m.clientIdentity()
	if err != nil {
		return err
	}
	return client.SendHeartbeat(ctx, identity, protocol.Heartbeat{
		ManagerVersion:        Version,
		HermesVersion:         config.HermesVersion,
		ActualState:           config.ActualState,
		HealthStatus:          config.HealthStatus,
		AppliedPolicyRevision: config.AppliedPolicyRevision,
		ConfigDigest:          config.ConfigDigest,
		LastErrorCode:         config.LastErrorCode,
		APIServerFingerprint:  config.APIServerFingerprint,
	})
}

func (m *Manager) Run(ctx context.Context, interval time.Duration, report func(error)) error {
	if interval < 15*time.Second {
		interval = 15 * time.Second
	}
	cycle := func() {
		desired, syncErr := m.Sync(ctx)
		if syncErr != nil {
			if desired != nil {
				if heartbeatErr := m.Heartbeat(ctx); heartbeatErr != nil && report != nil {
					report(fmt.Errorf("send heartbeat after convergence failure: %w", heartbeatErr))
				}
			}
			if report != nil {
				report(fmt.Errorf("sync desired state: %w", syncErr))
			}
			return
		}
		if heartbeatErr := m.Heartbeat(ctx); heartbeatErr != nil && report != nil {
			report(fmt.Errorf("send heartbeat: %w", heartbeatErr))
		}
	}
	cycle()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			cycle()
		}
	}
}

func (m *Manager) Status() (*state.Config, *protocol.DesiredState, error) {
	config, err := m.store.LoadConfig()
	if err != nil {
		return nil, nil, err
	}
	desired, err := m.store.LoadDesiredState()
	if err != nil && !errors.Is(err, state.ErrNotEnrolled) {
		// A missing desired-state file is expected immediately after enrollment.
		if !errors.Is(err, os.ErrNotExist) {
			return nil, nil, err
		}
	}
	return config, desired, nil
}

func (m *Manager) clientIdentity() (*state.Config, *protocol.Identity, *protocol.Client, error) {
	config, err := m.store.LoadConfig()
	if err != nil {
		return nil, nil, nil, err
	}
	privateKey, err := m.store.LoadPrivateKey()
	if err != nil {
		return nil, nil, nil, err
	}
	client, err := protocol.NewClient(config.ServerURL, m.httpClient)
	if err != nil {
		return nil, nil, nil, err
	}
	return config, &protocol.Identity{
		DeviceID:               config.DeviceID,
		PrivateKey:             privateKey,
		PolicySigningPublicKey: config.PolicySigningPublicKey,
	}, client, nil
}
