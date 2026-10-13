package manager

import (
	"context"
	"errors"
	"time"

	"github.com/go-admin-kit/clients/hermes-manager/internal/adapter"
	"github.com/go-admin-kit/clients/hermes-manager/internal/protocol"
	"github.com/go-admin-kit/clients/hermes-manager/internal/state"
)

// reconcileOutcome 是一轮收敛后要写回本地状态并随心跳上报的快照。
type reconcileOutcome struct {
	ActualState   string
	HealthStatus  string
	LastErrorCode string
	HermesVersion string
	// AppliedRevision 只在期望状态被完整执行后置为 desired.Revision，
	// 部分失败保留旧值——控制台以此判断策略是否真正生效。
	AppliedRevision uint64
	ConfigDigest    string
	// UpdateQuarantined 表示本轮已回滚或命中相同失败目标。applyOutcome
	// 会把目标持久化，直到策略变化或管理员显式 retry_update。
	UpdateQuarantined bool
	// ModelTokenApplied 只在新凭据已成功写入受管 .env 时为 true。manager
	// 据此更新本地到期时间，避免写盘失败后错误地抑制下一轮重试。
	ModelTokenApplied bool
	// PreserveUpdateInProgress 表示 current 指针可能仍停在未验证的新版本。
	// 只有回滚动作本身失败才保留，下一轮继续按未完成事务处理。
	PreserveUpdateInProgress bool
}

// eventSink 上报结构化事件。与网络解耦是为了单测：Sync 里接 client.SendEvent，
// 测试里接切片收集器。事件失败不中断收敛（先把本机收敛对，再操心上报）。
type eventSink func(eventType, result, errorCode string, appliedRevision uint64)

const (
	processStartCheckAttempts = 20
	processStableCheckCount   = 3
)

var processStartCheckInterval = 250 * time.Millisecond

// waitForRunning closes the gap between a platform launcher accepting a start
// request and the Hermes process actually becoming live. schtasks and launchctl
// can both return before a short-lived child has settled.
func waitForRunning(ctx context.Context, adp adapter.Adapter) error {
	var lastErr error
	stableChecks := 0
	for attempt := 0; attempt < processStartCheckAttempts; attempt++ {
		running, err := adp.Running(ctx)
		if err == nil {
			if running {
				stableChecks++
				if stableChecks >= processStableCheckCount {
					return nil
				}
			} else {
				stableChecks = 0
			}
		}
		if err != nil {
			lastErr = err
			stableChecks = 0
		}
		if attempt == processStartCheckAttempts-1 {
			break
		}
		timer := time.NewTimer(processStartCheckInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return adapter.Coded(adapter.ErrCodeStartFailed, ctx.Err())
		case <-timer.C:
		}
	}
	if lastErr != nil {
		return adapter.Coded(adapter.ErrCodeStartFailed, lastErr)
	}
	return adapter.Coded(adapter.ErrCodeStartFailed, errors.New("Hermes process did not reach running state"))
}

func waitForStopped(ctx context.Context, adp adapter.Adapter) error {
	var lastErr error
	for attempt := 0; attempt < processStartCheckAttempts; attempt++ {
		running, err := adp.Running(ctx)
		if err == nil && !running {
			return nil
		}
		if err != nil {
			lastErr = err
		}
		if attempt == processStartCheckAttempts-1 {
			break
		}
		timer := time.NewTimer(processStartCheckInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return adapter.Coded(adapter.ErrCodeStopFailed, ctx.Err())
		case <-timer.C:
		}
	}
	if lastErr != nil {
		return adapter.Coded(adapter.ErrCodeStopFailed, lastErr)
	}
	return adapter.Coded(adapter.ErrCodeStopFailed, errors.New("Hermes process did not reach stopped state"))
}

// reconcile 把期望状态收敛到本机。所有动作幂等，可整轮重跑。
//
// 事件只发终态（succeeded/failed）：服务端按终态事件清除 pending_action，
// started 事件会让动作在执行中途被误清。
func reconcile(
	ctx context.Context,
	adp adapter.Adapter,
	config *state.Config,
	desired *protocol.DesiredState,
	modelToken string,
	// tokenRotated 表示本轮换了模型凭据。上游 Hermes 只在启动时读 .env，
	// 所以换了凭据必须重启进程，否则运行中的 gateway 手里仍是已被服务端
	// 吊销的旧 token，模型调用会全部 401。
	tokenRotated bool,
	emit eventSink,
) reconcileOutcome {
	out := reconcileOutcome{
		AppliedRevision: config.AppliedPolicyRevision,
		ConfigDigest:    config.ConfigDigest,
		HermesVersion:   config.HermesVersion,
	}
	emitEvent := func(eventType, result, errorCode string) {
		emit(eventType, result, errorCode, out.AppliedRevision)
	}

	fail := func(state, code string) reconcileOutcome {
		out.ActualState = state
		out.HealthStatus = "unhealthy"
		out.LastErrorCode = code
		return out
	}

	switch desired.DesiredState {
	case "revoked":
		// 吊销：停进程即可。不删本地数据——文档明确不承诺远程擦除。
		if err := adp.Stop(ctx); err != nil {
			emitEvent("revoke", "failed", adapter.CodeOf(err))
			return fail("error", adapter.CodeOf(err))
		}
		if err := waitForStopped(ctx, adp); err != nil {
			emitEvent("revoke", "failed", adapter.CodeOf(err))
			return fail("error", adapter.CodeOf(err))
		}
		out.ActualState = "revoked"
		out.HealthStatus = "unknown"
		out.AppliedRevision = desired.Revision
		emitEvent("revoke", "succeeded", "")
		return out

	case "stopped":
		running, err := adp.Running(ctx)
		if err != nil {
			return fail("error", adapter.CodeOf(err))
		}
		if running {
			if err := adp.Stop(ctx); err != nil {
				emitEvent("stop", "failed", adapter.CodeOf(err))
				return fail("error", adapter.CodeOf(err))
			}
			if err := waitForStopped(ctx, adp); err != nil {
				emitEvent("stop", "failed", adapter.CodeOf(err))
				return fail("error", adapter.CodeOf(err))
			}
			out.AppliedRevision = desired.Revision
			emitEvent("stop", "succeeded", "")
		}
		out.ActualState = "stopped"
		out.HealthStatus = "healthy"
		out.AppliedRevision = desired.Revision
		return out
	}

	// ---- desired = active ----

	// 1. 版本收敛：制品与已装版本不符就装（首次装报 install，其后报 update）。
	installedVersion, err := adp.InstalledVersion()
	if err != nil {
		return fail("error", adapter.CodeOf(err))
	}
	previousVersion := installedVersion
	previousAppliedRevision := out.AppliedRevision
	previousConfigDigest := out.ConfigDigest
	persistedUpdate := config.UpdateInProgressArtifactID > 0 &&
		config.UpdateInProgressArtifactID == desired.Hermes.ID &&
		config.UpdateInProgressPolicyRevision == desired.Revision &&
		config.UpdateInProgressVersion == desired.Hermes.Version
	updateInProgress := persistedUpdate && installedVersion == desired.Hermes.Version &&
		config.HermesVersion != "" && config.HermesVersion != desired.Hermes.Version
	if updateInProgress {
		previousVersion = config.HermesVersion
	}
	quarantinedTarget := sameFailedUpdate(config, desired)
	if quarantinedTarget && desired.PendingAction != "retry_update" {
		out.UpdateQuarantined = true
		// 保持已经回滚的旧版本和旧配置，不再触碰失败目标。进程若被外部
		// 停掉仍恢复旧版本，避免隔离状态反而造成长期不可用。token 换发
		// 会立即吊销旧值，因此即使隔离也必须只更新 .env 并重启旧版本。
		if tokenRotated {
			if tokenErr := adp.ApplyModelToken(ctx, modelToken); tokenErr != nil {
				return fail("error", adapter.CodeOf(tokenErr))
			}
			out.ModelTokenApplied = true
			if restartErr := adp.Restart(ctx); restartErr != nil {
				return fail("error", adapter.CodeOf(restartErr))
			}
			if runningErr := waitForRunning(ctx, adp); runningErr != nil {
				return fail("error", adapter.CodeOf(runningErr))
			}
			emitEvent("start", "succeeded", "")
		}
		running, runErr := adp.Running(ctx)
		if runErr != nil {
			return fail("error", adapter.CodeOf(runErr))
		}
		if !running {
			if startErr := adp.Start(ctx); startErr != nil {
				return fail("stopped", adapter.CodeOf(startErr))
			}
			if runningErr := waitForRunning(ctx, adp); runningErr != nil {
				return fail("stopped", adapter.CodeOf(runningErr))
			}
			emitEvent("start", "succeeded", "")
		}
		out.ActualState = "active"
		out.HealthStatus = "degraded"
		out.LastErrorCode = config.LastErrorCode
		if out.LastErrorCode == "" {
			out.LastErrorCode = "UPDATE_QUARANTINED"
		}
		out.HermesVersion = installedVersion
		return out
	}
	if desired.Hermes.ID > 0 && installedVersion != desired.Hermes.Version {
		eventType := "install"
		if installedVersion != "" {
			eventType = "update"
		}
		out.ActualState = "installing"
		if err := adp.Install(ctx, desired.Hermes); err != nil {
			emitEvent(eventType, "failed", adapter.CodeOf(err))
			if eventType == "update" {
				out.UpdateQuarantined = true
				if tokenRotated {
					if tokenErr := adp.ApplyModelToken(ctx, modelToken); tokenErr != nil {
						return fail("error", adapter.CodeOf(tokenErr))
					}
					out.ModelTokenApplied = true
					if restartErr := adp.Restart(ctx); restartErr != nil {
						return fail("error", adapter.CodeOf(restartErr))
					}
					if runningErr := waitForRunning(ctx, adp); runningErr != nil {
						return fail("error", adapter.CodeOf(runningErr))
					}
				}
			}
			return fail("error", adapter.CodeOf(err))
		}
		// update 到这里只是切了指针；配置、重启和运行确认都
		// 完成后才能上报成功。首次 install 不存在可回滚的旧版本，
		// 保持原有的「制品已安装」事件语义。
		if eventType == "install" {
			emitEvent(eventType, "succeeded", "")
		} else {
			updateInProgress = true
		}
		installedVersion = desired.Hermes.Version
	}
	out.HermesVersion = installedVersion

	rollbackUpdate := func(originalCode string) reconcileOutcome {
		emitEvent("update", "failed", originalCode)
		rollbackRestored := false
		rollbackFailed := func(code string) reconcileOutcome {
			emitEvent("rollback", "failed", code)
			out.AppliedRevision = previousAppliedRevision
			out.ConfigDigest = previousConfigDigest
			out.ActualState = "error"
			out.HealthStatus = "unhealthy"
			out.LastErrorCode = code
			if rollbackRestored {
				// 指针已恢复旧版本，只是凭据/启动确认失败。隔离坏目标，
				// 下一轮在隔离分支修复旧版本，而不是再次安装坏目标。
				out.UpdateQuarantined = true
				out.HermesVersion = previousVersion
			} else {
				// Rollback 自身失败后指针位置不可信，保留事务标记继续恢复。
				out.UpdateQuarantined = false
				out.PreserveUpdateInProgress = true
			}
			if currentVersion, versionErr := adp.InstalledVersion(); versionErr == nil {
				if !rollbackRestored {
					out.HermesVersion = currentVersion
				}
			}
			return out
		}
		if rollbackErr := adp.Rollback(ctx); rollbackErr != nil {
			return rollbackFailed(adapter.CodeOf(rollbackErr))
		}
		rollbackRestored = true
		if tokenRotated {
			// Rollback 恢复的是更新前 .env，其中的旧 token 已在换发时被
			// 服务端吊销。只重写凭据并重启旧版本，不能重新套用失败策略。
			if tokenErr := adp.ApplyModelToken(ctx, modelToken); tokenErr != nil {
				return rollbackFailed(adapter.CodeOf(tokenErr))
			}
			out.ModelTokenApplied = true
			if restartErr := adp.Restart(ctx); restartErr != nil {
				return rollbackFailed(adapter.CodeOf(restartErr))
			}
		}
		if runningErr := waitForRunning(ctx, adp); runningErr != nil {
			return rollbackFailed(adapter.CodeOf(runningErr))
		}
		emitEvent("rollback", "succeeded", "")
		out.AppliedRevision = previousAppliedRevision
		out.ConfigDigest = previousConfigDigest
		out.HermesVersion = previousVersion
		out.ActualState = "active"
		out.HealthStatus = "degraded"
		out.LastErrorCode = originalCode
		out.UpdateQuarantined = true
		return out
	}

	// 2. 受管配置收敛。changed 且 revision 未变 = 磁盘被本机改过 → 漂移事件；
	//    revision 变了则属正常策略更新，不按漂移记。
	digest, profileChanged, err := adp.ApplyProfile(ctx, desired, modelToken)
	if err != nil {
		if updateInProgress {
			return rollbackUpdate(adapter.CodeOf(err))
		}
		emitEvent("apply_policy", "failed", adapter.CodeOf(err))
		return fail("error", adapter.CodeOf(err))
	}
	if tokenRotated {
		out.ModelTokenApplied = true
	}
	// M3 漂移抑制：mcp_servers 块在 config.yaml 里，绑定 MCP server 会改变渲染
	// 文件而 desired.Revision 不变。有 MCP 且应用未完成（digest 不等）时的 config
	// 变化是预期的、pending 的 MCP 应用，不是员工篡改 → 不发 drift；无 MCP 或
	// digest 一致仍漂移 = 真篡改。
	mcpPending := len(desired.McpServers) > 0 &&
		config.McpServersAppliedDigest != mcpServersDigest(desired.McpServers)
	if profileChanged && desired.Revision == config.AppliedPolicyRevision && !mcpPending {
		emitEvent("policy_drift", "succeeded", "")
	}
	out.ConfigDigest = digest

	// 2.5 三期：company_cluster 时把 API server 密钥写进 profile .env（gateway
	// 重启后启用 loopback api_server）；回落到 company 时移除。quarantine/更新中
	// 不写——回滚到旧二进制时不要带着新 profile 的 API server 配置。
	apiServerKey := ""
	if desired.Profile == "company_cluster" {
		apiServerKey = config.APIServerKey
	}
	if err := adp.ApplyAPIServerKey(ctx, apiServerKey); err != nil {
		if updateInProgress {
			return rollbackUpdate(adapter.CodeOf(err))
		}
		emitEvent("apply_policy", "failed", adapter.CodeOf(err))
		return fail("error", adapter.CodeOf(err))
	}

	// 3. 运维动作（在进程收敛前执行：retry_update/apply_policy 已被上面幂等
	//    步骤覆盖，此处只需补发终态事件；restart 是真动作）。
	switch desired.PendingAction {
	case "retry_update":
	case "apply_policy":
	case "restart":
		if err := adp.Restart(ctx); err != nil {
			if updateInProgress {
				return rollbackUpdate(adapter.CodeOf(err))
			}
			emitEvent("start", "failed", adapter.CodeOf(err))
			return fail("error", adapter.CodeOf(err))
		}
		if err := waitForRunning(ctx, adp); err != nil {
			if updateInProgress {
				return rollbackUpdate(adapter.CodeOf(err))
			}
			emitEvent("start", "failed", adapter.CodeOf(err))
			return fail("error", adapter.CodeOf(err))
		}
		emitEvent("start", "succeeded", "")
	}

	// 4. 进程收敛：期望 active 就得在跑。
	//    已经在跑但本轮换了凭据时要重启——.env 只在启动时被读取。
	//    restart 动作刚重启过就不必再来一次。
	running, err := adp.Running(ctx)
	if err != nil {
		if updateInProgress {
			return rollbackUpdate(adapter.CodeOf(err))
		}
		return fail("error", adapter.CodeOf(err))
	}
	switch {
	case !running:
		if err := adp.Start(ctx); err != nil {
			if updateInProgress {
				return rollbackUpdate(adapter.CodeOf(err))
			}
			emitEvent("start", "failed", adapter.CodeOf(err))
			return fail("stopped", adapter.CodeOf(err))
		}
		if err := waitForRunning(ctx, adp); err != nil {
			if updateInProgress {
				return rollbackUpdate(adapter.CodeOf(err))
			}
			emitEvent("start", "failed", adapter.CodeOf(err))
			return fail("stopped", adapter.CodeOf(err))
		}
		emitEvent("start", "succeeded", "")
	case tokenRotated && desired.PendingAction != "restart":
		if err := adp.Restart(ctx); err != nil {
			if updateInProgress {
				return rollbackUpdate(adapter.CodeOf(err))
			}
			emitEvent("start", "failed", adapter.CodeOf(err))
			return fail("error", adapter.CodeOf(err))
		}
		if err := waitForRunning(ctx, adp); err != nil {
			if updateInProgress {
				return rollbackUpdate(adapter.CodeOf(err))
			}
			emitEvent("start", "failed", adapter.CodeOf(err))
			return fail("error", adapter.CodeOf(err))
		}
		emitEvent("start", "succeeded", "")
	}

	out.ActualState = "active"
	out.HealthStatus = "healthy"
	out.AppliedRevision = desired.Revision
	if updateInProgress || desired.PendingAction == "retry_update" {
		emitEvent("update", "succeeded", "")
	}
	if desired.PendingAction == "apply_policy" {
		emitEvent("apply_policy", "succeeded", "")
	}
	return out
}

// applyOutcome 把收敛结果写回本地配置（随后由心跳上报）。
func applyOutcome(config *state.Config, desired *protocol.DesiredState, out reconcileOutcome, now time.Time) {
	config.DesiredPolicyRevision = desired.Revision
	config.AppliedPolicyRevision = out.AppliedRevision
	config.ActualState = out.ActualState
	config.HealthStatus = out.HealthStatus
	config.LastErrorCode = out.LastErrorCode
	config.HermesVersion = out.HermesVersion
	config.ConfigDigest = out.ConfigDigest
	if out.UpdateQuarantined {
		config.FailedUpdateArtifactID = desired.Hermes.ID
		config.FailedUpdatePolicyRevision = desired.Revision
		config.FailedUpdateVersion = desired.Hermes.Version
	} else {
		config.FailedUpdateArtifactID = 0
		config.FailedUpdatePolicyRevision = 0
		config.FailedUpdateVersion = ""
	}
	if !out.PreserveUpdateInProgress {
		config.UpdateInProgressArtifactID = 0
		config.UpdateInProgressPolicyRevision = 0
		config.UpdateInProgressVersion = ""
	}
	config.UpdatedAt = now.UTC()
}
