package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"github.com/google/uuid"
)

type WorkerConfig struct {
	ID            string
	PollInterval  time.Duration
	LeaseDuration time.Duration
	OnError       func(error)
}

type Worker struct {
	repository *Repository
	models     ModelPort
	tools      *ToolRegistry
	config     WorkerConfig
}

func NewWorker(repository *Repository, models ModelPort, tools *ToolRegistry, config WorkerConfig) *Worker {
	if config.ID == "" {
		hostname, _ := os.Hostname()
		config.ID = fmt.Sprintf("%s-%d-%s", hostname, os.Getpid(), uuid.NewString())
	}
	if config.PollInterval <= 0 {
		config.PollInterval = 500 * time.Millisecond
	}
	if config.LeaseDuration <= 0 {
		config.LeaseDuration = 30 * time.Second
	}
	return &Worker{repository: repository, models: models, tools: tools, config: config}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.config.PollInterval)
	defer ticker.Stop()
	for {
		worked, err := w.RunOnce(ctx)
		if err != nil && !errors.Is(err, context.Canceled) && w.config.OnError != nil {
			w.config.OnError(err)
		}
		if ctx.Err() != nil {
			return
		}
		if worked {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// RunOnce claims and processes at most one Run. It is exported for deterministic
// worker competition and crash-recovery tests.
func (w *Worker) RunOnce(ctx context.Context) (bool, error) {
	run, err := w.repository.ClaimNext(ctx, w.config.ID, w.config.LeaseDuration)
	if err != nil {
		return false, err
	}
	if run == nil {
		return false, nil
	}
	return true, w.processRun(ctx, *run)
}

// failOwnedRun fences a failure against the latest lease state. The monitor
// is intentionally best-effort, so a cancellation arriving between heartbeats
// must still win over a failure. A second check closes the race between the
// preflight and the conditional failure update.
func (w *Worker) failOwnedRun(parent context.Context, run AgentRun, code, message string, step *AgentStep) error {
	owned, cancelRequested, err := w.repository.RenewLease(parent, run, w.config.ID, w.config.LeaseDuration)
	if err != nil {
		return err
	}
	if !owned {
		return ErrLeaseLost
	}
	if cancelRequested {
		return w.repository.CancelOwnedRun(parent, run, w.config.ID, "cancel requested")
	}
	failureErr := w.repository.FailOwnedRun(parent, run, w.config.ID, code, message, step)
	if !errors.Is(failureErr, ErrLeaseLost) {
		return failureErr
	}
	owned, cancelRequested, checkErr := w.repository.RenewLease(parent, run, w.config.ID, w.config.LeaseDuration)
	if checkErr == nil && owned && cancelRequested {
		return w.repository.CancelOwnedRun(parent, run, w.config.ID, "cancel requested")
	}
	return failureErr
}

func (w *Worker) completeOwnedRun(parent context.Context, run AgentRun, monitor *leaseMonitor, output string) error {
	monitor.stop()
	if monitorErr := monitor.error(); monitorErr != nil {
		// Heartbeat failure means ownership is no longer provable; never
		// finalize a successful Run on that uncertainty.
		return monitorErr
	}
	switch {
	case monitor.lost.Load():
		return ErrLeaseLost
	case monitor.cancelRequested.Load():
		// 取消请求与最终完成并发：取消胜出，不把 run 标成 succeeded。
		return w.repository.CancelOwnedRun(parent, run, w.config.ID, "cancel requested")
	}
	if err := w.repository.CompleteRun(parent, run, w.config.ID, output); err != nil {
		if errors.Is(err, ErrRunTimeout) {
			return w.failOwnedRun(parent, run, "run_timeout", "run timeout exceeded", nil)
		}
		if errors.Is(err, ErrLeaseLost) {
			// DB 层兜底：CompleteRun 被并发取消抢先拒绝时，确认取消请求
			// 后收敛为 cancelled，不把 run 留在中间态。
			if ok, cancelRequested, checkErr := w.repository.RenewLease(parent, run, w.config.ID, w.config.LeaseDuration); checkErr == nil && ok && cancelRequested {
				return w.repository.CancelOwnedRun(parent, run, w.config.ID, "cancel requested")
			}
		}
		return err
	}
	return nil
}

type leaseMonitor struct {
	cancelRequested atomic.Bool
	lost            atomic.Bool
	errMu           sync.Mutex
	err             error
	stopOnce        sync.Once
	stopCh          chan struct{}
	doneCh          chan struct{}
	cancelRun       context.CancelFunc
}

func (m *leaseMonitor) stop() {
	m.stopOnce.Do(func() { close(m.stopCh) })
	<-m.doneCh
}

func (m *leaseMonitor) setError(err error) {
	m.errMu.Lock()
	m.err = err
	m.errMu.Unlock()
}

func (m *leaseMonitor) error() error {
	m.errMu.Lock()
	defer m.errMu.Unlock()
	return m.err
}

func (w *Worker) startLeaseMonitor(parent context.Context, runCtx context.Context, cancelRun context.CancelFunc, run AgentRun) *leaseMonitor {
	monitor := &leaseMonitor{stopCh: make(chan struct{}), doneCh: make(chan struct{}), cancelRun: cancelRun}
	interval := w.config.LeaseDuration / 3
	if interval < 100*time.Millisecond {
		interval = 100 * time.Millisecond
	}
	go func() {
		defer close(monitor.doneCh)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-monitor.stopCh:
				return
			case <-parent.Done():
				cancelRun()
				return
			case <-runCtx.Done():
				return
			case <-ticker.C:
				owned, cancelRequested, err := w.repository.RenewLease(parent, run, w.config.ID, w.config.LeaseDuration)
				if err != nil {
					monitor.setError(err)
					cancelRun()
					return
				}
				if !owned {
					monitor.lost.Store(true)
					cancelRun()
					return
				}
				if cancelRequested {
					monitor.cancelRequested.Store(true)
					cancelRun()
					return
				}
			}
		}
	}()
	return monitor
}

func (w *Worker) processRun(parent context.Context, run AgentRun) error {
	if run.CancelRequestedAt != nil {
		return w.repository.CancelOwnedRun(parent, run, w.config.ID, "cancelled before execution")
	}
	version, err := w.repository.LoadVersionForRun(parent, run)
	if err != nil {
		return w.failOwnedRun(parent, run, "version_unavailable", "published version unavailable", nil)
	}
	if err := version.ValidateIntegrity(); err != nil {
		return w.failOwnedRun(parent, run, "invalid_version", "published version integrity check failed", nil)
	}
	toolNames, err := version.ToolNames()
	if err != nil {
		return w.failOwnedRun(parent, run, "invalid_version", "published version is invalid", nil)
	}
	toolDefinitions, err := w.tools.Resolve(toolNames)
	if err != nil {
		return w.failOwnedRun(parent, run, "tool_unavailable", "published tool is unavailable", nil)
	}
	expectedToolHashes, err := w.repository.LoadVersionToolHashes(parent, *version)
	if err != nil {
		return w.failOwnedRun(parent, run, "invalid_version", "published tool grants are invalid", nil)
	}
	for _, definition := range toolDefinitions {
		actualHash, hashErr := w.tools.SpecHash(definition.Name)
		if hashErr != nil || expectedToolHashes[definition.Name] != actualHash {
			return w.failOwnedRun(parent, run, "tool_spec_changed", "published tool specification changed", nil)
		}
	}
	allowedTools := make(map[string]struct{}, len(toolDefinitions))
	for _, definition := range toolDefinitions {
		allowedTools[definition.Name] = struct{}{}
	}
	checkpoint, err := run.DecodeCheckpoint()
	if err != nil {
		return w.failOwnedRun(parent, run, "invalid_checkpoint", "run checkpoint is invalid", nil)
	}
	remaining := runTimeoutRemaining(run, time.Now().UTC())
	if remaining <= 0 {
		return w.failOwnedRun(parent, run, "run_timeout", "run timeout exceeded", nil)
	}

	runCtx, cancelRun := context.WithTimeout(parent, remaining)
	// 模型调用与工具执行都发生在租户上下文内：ModelPort 的配额归因、
	// 用量记账与模型目录查询（GORM tenant 插件）均要求 ctx 带租户。
	runCtx = tenant.WithContext(runCtx, run.TenantID)
	monitor := w.startLeaseMonitor(parent, runCtx, cancelRun, run)
	defer cancelRun()
	defer monitor.stop()

	fail := func(code, message string, step *AgentStep) error {
		monitor.stop()
		if monitor.lost.Load() {
			return ErrLeaseLost
		}
		if monitorErr := monitor.error(); monitorErr != nil {
			_ = w.repository.ReleaseLease(parent, run, w.config.ID)
			return monitorErr
		}
		if monitor.cancelRequested.Load() {
			return w.repository.CancelOwnedRun(parent, run, w.config.ID, "cancel requested")
		}
		if parent.Err() != nil {
			return w.repository.ReleaseLease(parent, run, w.config.ID)
		}
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return w.failOwnedRun(parent, run, "run_timeout", "run timeout exceeded", step)
		}
		return w.failOwnedRun(parent, run, code, message, step)
	}

	for {
		if runCtx.Err() != nil {
			return fail("run_interrupted", "run interrupted", nil)
		}
		if output, final := checkpointFinalOutput(checkpoint); final {
			// CompleteModelStep 与 CompleteRun 分属两个事务；若进程恰好在
			// 前者提交后崩溃，已持久化的最终 assistant 响应只能收敛为完成，
			// 不能再次调用模型。
			return w.completeOwnedRun(parent, run, monitor, output)
		}
		if checkpoint.NextOrdinal > run.MaxSteps {
			return fail("max_steps_exceeded", ErrMaxStepsExceeded.Error(), nil)
		}
		if checkpoint.PendingToolIndex < len(checkpoint.PendingToolCalls) {
			callIndex := checkpoint.PendingToolIndex
			call := checkpoint.PendingToolCalls[callIndex]
			if _, allowed := allowedTools[call.Name]; !allowed {
				return fail("tool_not_granted", "model requested a tool outside the published grant", nil)
			}
			if len(call.Arguments) == 0 || len(call.Arguments) > maxToolArgumentsBytes || !json.Valid(call.Arguments) {
				return fail("invalid_tool_arguments", "model returned invalid tool arguments", nil)
			}
			step, startErr := w.repository.StartStep(runCtx, run, w.config.ID, checkpoint.NextOrdinal, StepKindTool, call.Name, call)
			if startErr != nil {
				return fail("step_start_failed", "tool step could not start", nil)
			}
			idempotencyKey, argumentsHash, legacyKey, legacyHash, keyErr := toolCallIdentity(run.ID, checkpoint.PendingModelStep, callIndex, call)
			if keyErr != nil {
				return fail("tool_key_failed", "tool idempotency key could not be created", step)
			}
			persistedCall, cached, beginErr := w.repository.BeginToolCallWithLegacy(runCtx, run, w.config.ID, *step, call, idempotencyKey, argumentsHash, legacyKey, legacyHash)
			if beginErr != nil {
				return fail("tool_start_failed", "tool call could not start", step)
			}
			result := persistedCall.Result.Clone()
			if !cached {
				result, err = w.tools.Execute(runCtx, call.Name, call.Arguments)
				if err != nil {
					return fail("tool_execution_failed", "tool execution failed", step)
				}
			}
			checkpoint.Messages = append(checkpoint.Messages, Message{
				Role: "tool", ToolCallID: call.ID, Content: string(result),
			})
			checkpoint.PendingToolIndex++
			checkpoint.NextOrdinal++
			if checkpoint.PendingToolIndex >= len(checkpoint.PendingToolCalls) {
				checkpoint.PendingToolCalls = nil
				checkpoint.PendingToolIndex = 0
				checkpoint.PendingModelStep = 0
			}
			if err := validateCheckpointSize(checkpoint); err != nil {
				return fail("checkpoint_too_large", "run checkpoint exceeds limit", step)
			}
			if err := w.repository.CompleteToolStep(runCtx, run, w.config.ID, *step, *persistedCall, result, checkpoint, cached); err != nil {
				return fail("tool_commit_failed", "tool result could not be committed", step)
			}
			continue
		}

		step, startErr := w.repository.StartStep(runCtx, run, w.config.ID, checkpoint.NextOrdinal, StepKindModel, "model.complete", map[string]any{
			"model_id": version.ModelID, "messages": checkpoint.Messages, "tools": toolDefinitions,
		})
		if startErr != nil {
			return fail("step_start_failed", "model step could not start", nil)
		}
		response, completeErr := completeModelSafely(w.models, runCtx, ModelRequest{
			ModelID: version.ModelID, ActorID: run.RequestedBy, RunID: run.ID,
			Messages: cloneMessages(checkpoint.Messages), Tools: cloneToolDefinitions(toolDefinitions),
		})
		if completeErr != nil {
			return fail("model_call_failed", "model call failed: "+completeErr.Error(), step)
		}
		if err := validateModelResponse(response, allowedTools); err != nil {
			return fail("invalid_model_response", err.Error(), step)
		}
		modelOrdinal := checkpoint.NextOrdinal
		checkpoint.Messages = append(checkpoint.Messages, Message{
			Role: "assistant", Content: response.Content, ToolCalls: append([]ToolCall(nil), response.ToolCalls...),
		})
		checkpoint.NextOrdinal++
		if len(response.ToolCalls) > 0 {
			checkpoint.PendingToolCalls = cloneToolCalls(response.ToolCalls)
			checkpoint.PendingToolIndex = 0
			checkpoint.PendingModelStep = modelOrdinal
		}
		if err := validateCheckpointSize(checkpoint); err != nil {
			return fail("checkpoint_too_large", "run checkpoint exceeds limit", step)
		}
		if err := w.repository.CompleteModelStep(runCtx, run, w.config.ID, *step, response, checkpoint); err != nil {
			return fail("model_commit_failed", "model response could not be committed", step)
		}
		if len(response.ToolCalls) == 0 {
			return w.completeOwnedRun(parent, run, monitor, response.Content)
		}
	}
}

func completeModelSafely(models ModelPort, ctx context.Context, request ModelRequest) (response ModelResponse, err error) {
	defer func() {
		if recover() != nil {
			response = ModelResponse{}
			err = fmt.Errorf("model adapter panicked")
		}
	}()
	if models == nil {
		return ModelResponse{}, fmt.Errorf("model adapter is not configured")
	}
	return models.Complete(ctx, request)
}

func checkpointFinalOutput(checkpoint Checkpoint) (string, bool) {
	if len(checkpoint.PendingToolCalls) > 0 || checkpoint.PendingToolIndex != 0 || checkpoint.PendingModelStep != 0 || len(checkpoint.Messages) == 0 {
		return "", false
	}
	last := checkpoint.Messages[len(checkpoint.Messages)-1]
	if last.Role != "assistant" || len(last.ToolCalls) != 0 || last.Content == "" {
		return "", false
	}
	return last.Content, true
}

func runTimeoutRemaining(run AgentRun, now time.Time) time.Duration {
	timeout := time.Duration(run.TimeoutSeconds) * time.Second
	start := now
	if run.StartedAt != nil && !run.StartedAt.IsZero() {
		start = run.StartedAt.UTC()
	}
	elapsed := now.Sub(start)
	if elapsed <= 0 {
		return timeout
	}
	if elapsed >= timeout {
		return 0
	}
	return timeout - elapsed
}

func validateModelResponse(response ModelResponse, allowedTools map[string]struct{}) error {
	if err := validateModelResponseShape(response); err != nil {
		return err
	}
	for _, call := range response.ToolCalls {
		if _, allowed := allowedTools[call.Name]; !allowed {
			return fmt.Errorf("model requested a tool outside the published grant")
		}
	}
	return nil
}

func validateModelResponseShape(response ModelResponse) error {
	if len(response.Content) > maxModelContentBytes || len(response.ToolCalls) > maxModelToolCalls || response.Usage.InputTokens < 0 || response.Usage.OutputTokens < 0 {
		return fmt.Errorf("model response exceeds limits")
	}
	if response.Content == "" && len(response.ToolCalls) == 0 {
		return fmt.Errorf("model returned neither content nor tool calls")
	}
	seenIDs := make(map[string]struct{}, len(response.ToolCalls))
	for _, call := range response.ToolCalls {
		if call.ID == "" || len(call.ID) > 128 || !toolNamePattern.MatchString(call.Name) || len(call.Arguments) == 0 || len(call.Arguments) > maxToolArgumentsBytes || !json.Valid(call.Arguments) {
			return fmt.Errorf("model returned an invalid tool call")
		}
		if _, duplicate := seenIDs[call.ID]; duplicate {
			return fmt.Errorf("model returned duplicate tool call ids")
		}
		seenIDs[call.ID] = struct{}{}
	}
	return nil
}

func validateCheckpointSize(checkpoint Checkpoint) error {
	encoded, err := json.Marshal(checkpoint)
	if err != nil {
		return err
	}
	if len(encoded) > maxCheckpointBytes {
		return fmt.Errorf("checkpoint exceeds 512 KiB")
	}
	return nil
}

func cloneToolCalls(calls []ToolCall) []ToolCall {
	result := make([]ToolCall, len(calls))
	for index, call := range calls {
		result[index] = call
		result[index].Arguments = call.Arguments.Clone()
	}
	return result
}

func cloneMessages(messages []Message) []Message {
	result := make([]Message, len(messages))
	for index, message := range messages {
		result[index] = message
		result[index].ToolCalls = cloneToolCalls(message.ToolCalls)
	}
	return result
}

func cloneToolDefinitions(definitions []ToolDefinition) []ToolDefinition {
	result := make([]ToolDefinition, len(definitions))
	for index, definition := range definitions {
		result[index] = definition
		result[index].InputSchema = definition.InputSchema.Clone()
	}
	return result
}
