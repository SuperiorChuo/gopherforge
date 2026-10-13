package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *Repository) ClaimNext(ctx context.Context, owner string, leaseDuration time.Duration) (*AgentRun, error) {
	if owner == "" || leaseDuration <= 0 {
		return nil, fmt.Errorf("%w: invalid worker lease", ErrValidation)
	}
	now := r.now().UTC()
	expires := now.Add(leaseDuration)
	claimCtx := tenant.DisableScope(ctx)
	if r.db.Dialector.Name() == "sqlite" {
		return r.claimNextSQLite(claimCtx, owner, now, expires)
	}
	var claimed AgentRun
	err := r.db.WithContext(claimCtx).Transaction(func(tx *gorm.DB) error {
		query := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status IN ? AND (status = ? OR lease_expires_at IS NULL OR lease_expires_at < ?)",
				[]string{RunQueued, RunRunning, RunCancelling}, RunQueued, now).
			Order("id ASC").Limit(1)
		if err := query.First(&claimed).Error; err != nil {
			return err
		}
		previousAttempt := claimed.Attempt
		result := tx.Model(&AgentRun{}).
			Where("id = ? AND tenant_id = ? AND status IN ? AND (status = ? OR lease_expires_at IS NULL OR lease_expires_at < ?)",
				claimed.ID, claimed.TenantID, []string{RunQueued, RunRunning, RunCancelling}, RunQueued, now).
			Updates(map[string]any{
				"status": RunRunning, "attempt": gorm.Expr("attempt + 1"), "lease_owner": owner,
				"lease_expires_at": expires, "heartbeat_at": now, "started_at": gorm.Expr("COALESCE(started_at, ?)", now),
				"updated_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrLeaseLost
		}
		claimed.Status = RunRunning
		claimed.Attempt = previousAttempt + 1
		claimed.LeaseOwner = owner
		claimed.LeaseExpiresAt = &expires
		claimed.HeartbeatAt = &now
		if claimed.StartedAt == nil {
			claimed.StartedAt = &now
		}
		eventType := "run.started"
		if previousAttempt > 0 {
			eventType = "run.resumed"
		}
		return appendEvent(tx, claimed.TenantID, claimed.ID, nil, eventType, map[string]any{"attempt": claimed.Attempt}, now)
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &claimed, nil
}

func (r *Repository) claimNextSQLite(ctx context.Context, owner string, now, expires time.Time) (*AgentRun, error) {
	// 候选选择在事务外（普通读）：避免 SQLite deferred 事务内先读后写
	// 在并发 claim 时升级写锁产生死锁；资格条件在 UPDATE 内重验，
	// 候选被抢时 RowsAffected=0 视为本轮无任务。
	var candidateIDs []uint64
	if err := r.db.WithContext(ctx).Model(&AgentRun{}).
		Where("status IN ? AND (status = ? OR lease_expires_at IS NULL OR lease_expires_at < ?)",
			[]string{RunQueued, RunRunning, RunCancelling}, RunQueued, now).
		Order("id ASC").Limit(1).Pluck("id", &candidateIDs).Error; err != nil {
		return nil, err
	}
	if len(candidateIDs) == 0 {
		return nil, nil
	}
	var claimed AgentRun
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 回读必须按候选 id 精确定位：若按 lease_owner+heartbeat_at 回读，
		// 同一 worker 在同一时钟值下连续 claim 两条 run 会命中多行，
		// First 取错行，导致另一条 run 被置为 Running 却无人处理。
		result := tx.Model(&AgentRun{}).
			Where("id = ? AND status IN ? AND (status = ? OR lease_expires_at IS NULL OR lease_expires_at < ?)",
				candidateIDs[0], []string{RunQueued, RunRunning, RunCancelling}, RunQueued, now).
			Updates(map[string]any{
				"status": RunRunning, "attempt": gorm.Expr("attempt + 1"), "lease_owner": owner,
				"lease_expires_at": expires, "heartbeat_at": now, "started_at": gorm.Expr("COALESCE(started_at, ?)", now),
				"updated_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		if err := tx.Where("id = ? AND lease_owner = ?", candidateIDs[0], owner).First(&claimed).Error; err != nil {
			return err
		}
		eventType := "run.started"
		if claimed.Attempt > 1 {
			eventType = "run.resumed"
		}
		return appendEvent(tx, claimed.TenantID, claimed.ID, nil, eventType, map[string]any{"attempt": claimed.Attempt}, now)
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &claimed, nil
}

// RenewLease returns whether ownership is still valid and whether cancellation
// has been requested. It never acquires a lease owned by another worker.
func (r *Repository) RenewLease(ctx context.Context, run AgentRun, owner string, leaseDuration time.Duration) (bool, bool, error) {
	now := r.now().UTC()
	expires := now.Add(leaseDuration)
	tenantCtx := tenant.WithContext(context.WithoutCancel(ctx), run.TenantID)
	result := r.db.WithContext(tenantCtx).Model(&AgentRun{}).
		Where("tenant_id = ? AND id = ? AND lease_owner = ? AND status IN ?", run.TenantID, run.ID, owner, []string{RunRunning, RunCancelling}).
		Updates(map[string]any{"lease_expires_at": expires, "heartbeat_at": now, "updated_at": now})
	if result.Error != nil {
		return false, false, result.Error
	}
	if result.RowsAffected != 1 {
		return false, false, nil
	}
	var current AgentRun
	if err := r.db.WithContext(tenantCtx).Select("cancel_requested_at", "status").
		Where("tenant_id = ? AND id = ? AND lease_owner = ?", run.TenantID, run.ID, owner).First(&current).Error; err != nil {
		return false, false, normalizeDBError(err)
	}
	return true, current.CancelRequestedAt != nil || current.Status == RunCancelling, nil
}

func (r *Repository) ReleaseLease(ctx context.Context, run AgentRun, owner string) error {
	now := r.now().UTC()
	tenantCtx := tenant.WithContext(context.WithoutCancel(ctx), run.TenantID)
	result := r.db.WithContext(tenantCtx).Model(&AgentRun{}).
		Where("tenant_id = ? AND id = ? AND lease_owner = ? AND status IN ?", run.TenantID, run.ID, owner, []string{RunRunning, RunCancelling}).
		Updates(map[string]any{"lease_owner": "", "lease_expires_at": now, "heartbeat_at": now, "updated_at": now})
	return result.Error
}

func (r *Repository) LoadVersionForRun(ctx context.Context, run AgentRun) (*AgentVersion, error) {
	tenantCtx := tenant.WithContext(ctx, run.TenantID)
	var version AgentVersion
	err := r.db.WithContext(tenantCtx).
		Where("tenant_id = ? AND id = ? AND agent_definition_id = ?", run.TenantID, run.AgentVersionID, run.AgentDefinitionID).
		First(&version).Error
	if err != nil {
		return nil, normalizeDBError(err)
	}
	return &version, nil
}

func (r *Repository) LoadVersionToolHashes(ctx context.Context, version AgentVersion) (map[string]string, error) {
	names, err := version.ToolNames()
	if err != nil {
		return nil, err
	}
	hashes := make(map[string]string, len(names))
	if len(names) == 0 {
		return hashes, nil
	}
	tenantCtx := tenant.WithContext(ctx, version.TenantID)
	var grants []AgentVersionTool
	// 查询上限由不可变版本中的工具名数量决定，额外取一行用于检测异常多的授权。
	//nolint:unbounded-find
	if err := r.db.WithContext(tenantCtx).
		Where("tenant_id = ? AND agent_version_id = ?", version.TenantID, version.ID).
		Limit(len(names) + 1).Find(&grants).Error; err != nil {
		return nil, err
	}
	if len(grants) != len(names) {
		return nil, fmt.Errorf("version tool grants are incomplete")
	}
	for _, grant := range grants {
		if _, duplicate := hashes[grant.ToolName]; duplicate {
			return nil, fmt.Errorf("version tool grants contain duplicate names")
		}
		hashes[grant.ToolName] = grant.ToolSpecSHA256
	}
	for _, name := range names {
		if _, exists := hashes[name]; !exists {
			return nil, fmt.Errorf("version tool grant is missing %s", name)
		}
	}
	return hashes, nil
}

func (r *Repository) StartStep(
	ctx context.Context,
	run AgentRun,
	owner string,
	ordinal int,
	kind, name string,
	request any,
) (*AgentStep, error) {
	if ordinal < 1 || (kind != StepKindModel && kind != StepKindTool) || len(name) > 100 {
		return nil, ErrValidation
	}
	tenantCtx := tenant.WithContext(ctx, run.TenantID)
	now := r.now().UTC()
	requestJSON, err := NewJSONValue(request)
	if err != nil {
		return nil, err
	}
	var step AgentStep
	err = r.db.WithContext(tenantCtx).Transaction(func(tx *gorm.DB) error {
		created := false
		if err := ensureOwnedRun(tx, run, owner); err != nil {
			return err
		}
		findErr := tx.Where("tenant_id = ? AND run_id = ? AND ordinal = ?", run.TenantID, run.ID, ordinal).First(&step).Error
		switch {
		case findErr == nil:
			if step.Status == StepSucceeded {
				return fmt.Errorf("%w: step %d already succeeded", ErrConflict, ordinal)
			}
			if err := tx.Model(&AgentStep{}).Where("tenant_id = ? AND id = ?", run.TenantID, step.ID).
				Updates(map[string]any{
					"kind": kind, "name": name, "status": StepRunning, "request": requestJSON,
					"response": MustJSONValue(map[string]any{}), "error_code": "", "error_message": "",
					"started_at": now, "finished_at": nil,
				}).Error; err != nil {
				return err
			}
			step.Kind, step.Name, step.Status, step.Request, step.StartedAt = kind, name, StepRunning, requestJSON, now
		case errors.Is(findErr, gorm.ErrRecordNotFound):
			created = true
			step = AgentStep{
				TenantID: run.TenantID, RunID: run.ID, Ordinal: ordinal, Kind: kind, Name: name,
				Status: StepRunning, Request: requestJSON, Response: MustJSONValue(map[string]any{}),
				StartedAt: now, CreatedAt: now,
			}
			if err := tx.Create(&step).Error; err != nil {
				return err
			}
		default:
			return findErr
		}
		eventType := "step.started"
		if kind == StepKindTool {
			eventType = "tool.started"
		}
		if !created {
			// 崩溃恢复重入同一 step：补发带 resumed 标记的启动事件，
			// 让账本可区分首次执行与恢复执行（审计与 SSE 重放语义）。
			return appendEvent(tx, run.TenantID, run.ID, &step.ID, eventType, map[string]any{
				"ordinal": ordinal, "name": name, "resumed": true,
			}, now)
		}
		return appendEvent(tx, run.TenantID, run.ID, &step.ID, eventType, map[string]any{"ordinal": ordinal, "name": name}, now)
	})
	if err != nil {
		return nil, err
	}
	return &step, nil
}

func (r *Repository) CompleteModelStep(
	ctx context.Context,
	run AgentRun,
	owner string,
	step AgentStep,
	response ModelResponse,
	checkpoint Checkpoint,
) error {
	if step.Kind != StepKindModel {
		return ErrValidation
	}
	if err := validateModelResponseShape(response); err != nil {
		return fmt.Errorf("invalid model response: %w", err)
	}
	if err := validateCheckpoint(checkpoint); err != nil {
		return fmt.Errorf("invalid checkpoint: %w", err)
	}
	tenantCtx := tenant.WithContext(ctx, run.TenantID)
	now := r.now().UTC()
	responseJSON, err := NewJSONValue(response)
	if err != nil {
		return err
	}
	checkpointJSON, err := NewJSONValue(checkpoint)
	if err != nil {
		return err
	}
	return r.db.WithContext(tenantCtx).Transaction(func(tx *gorm.DB) error {
		if err := ensureOwnedRun(tx, run, owner); err != nil {
			return err
		}
		result := tx.Model(&AgentStep{}).
			Where("tenant_id = ? AND id = ? AND run_id = ? AND status = ?", run.TenantID, step.ID, run.ID, StepRunning).
			Updates(map[string]any{
				"status": StepSucceeded, "response": responseJSON,
				"input_tokens": response.Usage.InputTokens, "output_tokens": response.Usage.OutputTokens,
				"stop_reason": truncate(response.StopReason, 64), "finished_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrLeaseLost
		}
		runResult := tx.Model(&AgentRun{}).
			Where("tenant_id = ? AND id = ? AND lease_owner = ? AND status IN ?", run.TenantID, run.ID, owner, []string{RunRunning, RunCancelling}).
			Updates(map[string]any{"checkpoint": checkpointJSON, "updated_at": now})
		if runResult.Error != nil {
			return runResult.Error
		}
		if runResult.RowsAffected != 1 {
			// run checkpoint 与 step 更新同票：租约被抢或状态已终态时不得静默不写。
			return ErrLeaseLost
		}
		return appendEvent(tx, run.TenantID, run.ID, &step.ID, "step.completed", map[string]any{
			"ordinal": step.Ordinal, "kind": StepKindModel, "tool_calls": len(response.ToolCalls),
		}, now)
	})
}

func (r *Repository) BeginToolCall(
	ctx context.Context,
	run AgentRun,
	owner string,
	step AgentStep,
	call ToolCall,
	idempotencyKey, argumentsHash string,
) (*AgentToolCall, bool, error) {
	return r.beginToolCall(ctx, run, owner, step, call, idempotencyKey, argumentsHash, "", "")
}

// BeginToolCallWithLegacy also checks the pre-canonicalization identity during
// rolling deploys. It keeps an old worker's committed call exactly-once from
// turning into a second read-only execution on a new worker.
func (r *Repository) BeginToolCallWithLegacy(
	ctx context.Context,
	run AgentRun,
	owner string,
	step AgentStep,
	call ToolCall,
	idempotencyKey, argumentsHash, legacyKey, legacyHash string,
) (*AgentToolCall, bool, error) {
	return r.beginToolCall(ctx, run, owner, step, call, idempotencyKey, argumentsHash, legacyKey, legacyHash)
}

func (r *Repository) beginToolCall(
	ctx context.Context,
	run AgentRun,
	owner string,
	step AgentStep,
	call ToolCall,
	idempotencyKey, argumentsHash, legacyKey, legacyHash string,
) (*AgentToolCall, bool, error) {
	tenantCtx := tenant.WithContext(ctx, run.TenantID)
	now := r.now().UTC()
	var toolCall AgentToolCall
	cached := false
	err := r.db.WithContext(tenantCtx).Transaction(func(tx *gorm.DB) error {
		if err := ensureOwnedRun(tx, run, owner); err != nil {
			return err
		}
		findErr := tx.Where("tenant_id = ? AND idempotency_key = ?", run.TenantID, idempotencyKey).First(&toolCall).Error
		if errors.Is(findErr, gorm.ErrRecordNotFound) && legacyKey != "" {
			findErr = tx.Where("tenant_id = ? AND idempotency_key = ?", run.TenantID, legacyKey).First(&toolCall).Error
		}
		if findErr == nil {
			if toolCall.RunID != run.ID || toolCall.ToolName != call.Name ||
				(toolCall.ArgumentsSHA256 != argumentsHash && (legacyHash == "" || toolCall.ArgumentsSHA256 != legacyHash)) {
				return ErrIdempotencyConflict
			}
			if toolCall.Status == StepSucceeded {
				cached = true
				return nil
			}
			return tx.Model(&AgentToolCall{}).Where("tenant_id = ? AND id = ? AND run_id = ?", run.TenantID, toolCall.ID, run.ID).
				Updates(map[string]any{
					"step_id": step.ID, "provider_call_id": truncate(call.ID, 128), "status": StepRunning,
					"attempt": gorm.Expr("attempt + 1"), "error_code": "", "error_message": "",
					"started_at": now, "finished_at": nil, "updated_at": now,
				}).Error
		}
		if !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}
		toolCall = AgentToolCall{
			TenantID: run.TenantID, RunID: run.ID, StepID: step.ID, ProviderCallID: truncate(call.ID, 128),
			IdempotencyKey: idempotencyKey, ToolName: call.Name, Arguments: call.Arguments.Clone(),
			ArgumentsSHA256: argumentsHash, Status: StepRunning, Result: MustJSONValue(map[string]any{}), Attempt: 1,
			StartedAt: now, CreatedAt: now, UpdatedAt: now,
		}
		return tx.Create(&toolCall).Error
	})
	return &toolCall, cached, err
}

func (r *Repository) CompleteToolStep(
	ctx context.Context,
	run AgentRun,
	owner string,
	step AgentStep,
	toolCall AgentToolCall,
	result JSONValue,
	checkpoint Checkpoint,
	cached bool,
) error {
	if step.Kind != StepKindTool || toolCall.RunID != run.ID || toolCall.StepID != step.ID || !json.Valid(result) || len(result) > maxToolResultBytes {
		return ErrValidation
	}
	if err := validateCheckpoint(checkpoint); err != nil {
		return fmt.Errorf("invalid checkpoint: %w", err)
	}
	tenantCtx := tenant.WithContext(ctx, run.TenantID)
	now := r.now().UTC()
	checkpointJSON, err := NewJSONValue(checkpoint)
	if err != nil {
		return err
	}
	return r.db.WithContext(tenantCtx).Transaction(func(tx *gorm.DB) error {
		if err := ensureOwnedRun(tx, run, owner); err != nil {
			return err
		}
		if !cached {
			updated := tx.Model(&AgentToolCall{}).
				Where("tenant_id = ? AND id = ? AND run_id = ?", run.TenantID, toolCall.ID, run.ID).
				Updates(map[string]any{"status": StepSucceeded, "result": result, "finished_at": now, "updated_at": now})
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected != 1 {
				return ErrLeaseLost
			}
		}
		stepResult := tx.Model(&AgentStep{}).
			Where("tenant_id = ? AND id = ? AND run_id = ? AND status = ?", run.TenantID, step.ID, run.ID, StepRunning).
			Updates(map[string]any{"status": StepSucceeded, "response": result, "finished_at": now})
		if stepResult.Error != nil {
			return stepResult.Error
		}
		if stepResult.RowsAffected != 1 {
			return ErrLeaseLost
		}
		runResult := tx.Model(&AgentRun{}).
			Where("tenant_id = ? AND id = ? AND lease_owner = ? AND status IN ?", run.TenantID, run.ID, owner, []string{RunRunning, RunCancelling}).
			Updates(map[string]any{"checkpoint": checkpointJSON, "updated_at": now})
		if runResult.Error != nil {
			return runResult.Error
		}
		if runResult.RowsAffected != 1 {
			// 工具结果与 checkpoint 必须同票提交；租约丢失时回滚前面的
			// tool/step 更新，避免留下不可恢复的半条执行轨迹。
			return ErrLeaseLost
		}
		if err := appendEvent(tx, run.TenantID, run.ID, &step.ID, "tool.completed", map[string]any{
			"ordinal": step.Ordinal, "tool": toolCall.ToolName, "cached": cached,
		}, now); err != nil {
			return err
		}
		return appendEvent(tx, run.TenantID, run.ID, &step.ID, "step.completed", map[string]any{
			"ordinal": step.Ordinal, "kind": StepKindTool,
		}, now)
	})
}

func (r *Repository) CompleteRun(ctx context.Context, run AgentRun, owner, output string) error {
	return r.finishOwnedRun(ctx, run, owner, RunSucceeded, output, "", "", "run.succeeded")
}

// finishFromStatuses 限定终态迁移的合法源状态：成功与失败都必须从
// running 出发，不能覆盖并发取消；取消可以从 running/cancelling 收敛。
func finishFromStatuses(target string) []string {
	if target == RunSucceeded || target == RunFailed {
		return []string{RunRunning}
	}
	return []string{RunRunning, RunCancelling}
}

func (r *Repository) CancelOwnedRun(ctx context.Context, run AgentRun, owner, reason string) error {
	return r.finishOwnedRun(ctx, run, owner, RunCancelled, "", "cancelled", reason, "run.cancelled")
}

func (r *Repository) FailOwnedRun(ctx context.Context, run AgentRun, owner, code, message string, step *AgentStep) error {
	tenantCtx := tenant.WithContext(context.WithoutCancel(ctx), run.TenantID)
	now := r.now().UTC()
	return r.db.WithContext(tenantCtx).Transaction(func(tx *gorm.DB) error {
		if err := ensureOwnedRun(tx, run, owner); err != nil {
			return err
		}
		steps, err := runningStepsForFailure(tx, run, step)
		if err != nil {
			return err
		}
		for _, step := range steps {
			stepResult := tx.Model(&AgentStep{}).
				Where("tenant_id = ? AND id = ? AND run_id = ? AND status = ?", run.TenantID, step.ID, run.ID, StepRunning).
				Updates(map[string]any{
					"status": StepFailed, "error_code": truncate(code, 64), "error_message": truncate(message, 500), "finished_at": now,
				})
			if stepResult.Error != nil {
				return stepResult.Error
			}
			if stepResult.RowsAffected != 1 {
				return ErrLeaseLost
			}
			toolClosed := false
			if step.Kind == StepKindTool {
				toolResult := tx.Model(&AgentToolCall{}).
					Where("tenant_id = ? AND run_id = ? AND step_id = ? AND status = ?", run.TenantID, run.ID, step.ID, StepRunning).
					Updates(map[string]any{
						"status": StepFailed, "error_code": truncate(code, 64), "error_message": truncate(message, 500), "finished_at": now, "updated_at": now,
					})
				if toolResult.Error != nil {
					return toolResult.Error
				}
				toolClosed = toolResult.RowsAffected == 1
			}
			if err := appendEvent(tx, run.TenantID, run.ID, &step.ID, "step.failed", map[string]any{"code": truncate(code, 64)}, now); err != nil {
				return err
			}
			if toolClosed {
				if err := appendEvent(tx, run.TenantID, run.ID, &step.ID, "tool.failed", map[string]any{"code": truncate(code, 64)}, now); err != nil {
					return err
				}
			}
		}
		result := tx.Model(&AgentRun{}).
			Where("tenant_id = ? AND id = ? AND lease_owner = ? AND status IN ?", run.TenantID, run.ID, owner, []string{RunRunning}).
			Updates(map[string]any{
				"status": RunFailed, "error_code": truncate(code, 64), "error_message": truncate(message, 500),
				"lease_owner": "", "lease_expires_at": nil, "finished_at": now, "updated_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrLeaseLost
		}
		return appendEvent(tx, run.TenantID, run.ID, nil, "run.failed", map[string]any{"code": truncate(code, 64)}, now)
	})
}

func (r *Repository) finishOwnedRun(ctx context.Context, run AgentRun, owner, status, output, code, message, eventType string) error {
	tenantCtx := tenant.WithContext(context.WithoutCancel(ctx), run.TenantID)
	now := r.now().UTC()
	return r.db.WithContext(tenantCtx).Transaction(func(tx *gorm.DB) error {
		if err := ensureOwnedRun(tx, run, owner); err != nil {
			return err
		}
		if status == RunCancelled {
			// 单 Run 的 running 步骤数由 max_steps ≤ 32 约束。
			//nolint:unbounded-find
			var runningSteps []AgentStep
			if err := tx.Select("id", "ordinal", "kind").Where("tenant_id = ? AND run_id = ? AND status = ?", run.TenantID, run.ID, StepRunning).Find(&runningSteps).Error; err != nil {
				return err
			}
			if len(runningSteps) > 0 {
				if err := tx.Model(&AgentStep{}).
					Where("tenant_id = ? AND run_id = ? AND status = ?", run.TenantID, run.ID, StepRunning).
					Updates(map[string]any{"status": StepCancelled, "error_code": "cancelled", "error_message": truncate(message, 500), "finished_at": now}).Error; err != nil {
					return err
				}
				for _, step := range runningSteps {
					toolClosed := false
					if step.Kind == StepKindTool {
						toolResult := tx.Model(&AgentToolCall{}).
							Where("tenant_id = ? AND run_id = ? AND step_id = ? AND status = ?", run.TenantID, run.ID, step.ID, StepRunning).
							Updates(map[string]any{
								"status": StepCancelled, "error_code": "cancelled", "error_message": truncate(message, 500), "finished_at": now, "updated_at": now,
							})
						if toolResult.Error != nil {
							return toolResult.Error
						}
						toolClosed = toolResult.RowsAffected == 1
					}
					if toolClosed {
						if err := appendEvent(tx, run.TenantID, run.ID, &step.ID, "tool.cancelled", map[string]any{"ordinal": step.Ordinal}, now); err != nil {
							return err
						}
					}
					if err := appendEvent(tx, run.TenantID, run.ID, &step.ID, "step.cancelled", map[string]any{"ordinal": step.Ordinal}, now); err != nil {
						return err
					}
				}
			}
		}
		if status == RunSucceeded && run.StartedAt != nil && run.TimeoutSeconds > 0 && !now.Before(run.StartedAt.UTC().Add(time.Duration(run.TimeoutSeconds)*time.Second)) {
			// 终态成功也必须受持久化总超时约束；仅依赖 runCtx 会让
			// 忽略 context 的 Provider 在 deadline 后仍把 Run 标成成功。
			return ErrRunTimeout
		}
		result := tx.Model(&AgentRun{}).
			Where("tenant_id = ? AND id = ? AND lease_owner = ? AND status IN ?", run.TenantID, run.ID, owner, finishFromStatuses(status)).
			Updates(map[string]any{
				"status": status, "output": truncate(output, 128*1024), "error_code": truncate(code, 64), "error_message": truncate(message, 500),
				"lease_owner": "", "lease_expires_at": nil, "finished_at": now, "updated_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrLeaseLost
		}
		return appendEvent(tx, run.TenantID, run.ID, nil, eventType, map[string]any{}, now)
	})
}

func ensureOwnedRun(tx *gorm.DB, run AgentRun, owner string) error {
	query := tx.Model(&AgentRun{}).
		Select("id").
		Where("tenant_id = ? AND id = ? AND lease_owner = ? AND status IN ?", run.TenantID, run.ID, owner, []string{RunRunning, RunCancelling})
	if tx.Dialector.Name() != "sqlite" {
		// Serialize every event-producing transaction for one Run. PostgreSQL
		// sequence values are allocated before commit; without this lock an SSE
		// cursor could observe a larger event ID and later miss a smaller one.
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var current AgentRun
	if err := query.First(&current).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrLeaseLost
		}
		return err
	}
	return nil
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	cut := value[:max]
	// 不在多字节 UTF-8 字符中间截断：剥掉末尾不完整的 continuation bytes
	//（最多 3 个），避免落库非法 UTF-8 破坏 JSON 序列化与前端渲染。
	for len(cut) > 0 {
		r, size := utf8.DecodeLastRuneInString(cut)
		if r != utf8.RuneError || size > 1 {
			break
		}
		cut = cut[:len(cut)-1]
	}
	return cut
}
