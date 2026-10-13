package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
)

type fakeModel struct {
	validate func(context.Context, uint64, bool) error
	complete func(context.Context, ModelRequest) (ModelResponse, error)
	calls    atomic.Int32
}

func (m *fakeModel) Validate(ctx context.Context, modelID uint64, tools bool) error {
	if m.validate != nil {
		return m.validate(ctx, modelID, tools)
	}
	return nil
}

func (m *fakeModel) Complete(ctx context.Context, request ModelRequest) (ModelResponse, error) {
	m.calls.Add(1)
	return m.complete(ctx, request)
}

func newTestRepository(t *testing.T) *Repository {
	t.Helper()
	dsn := fmt.Sprintf("file:agentcore-%d?mode=memory&cache=shared&_pragma=busy_timeout(5000)", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := tenant.Register(db); err != nil {
		t.Fatalf("register tenant plugin: %v", err)
	}
	repository := NewRepository(db)
	if err := repository.AutoMigrateForTest(); err != nil {
		t.Fatalf("migrate agent tables: %v", err)
	}
	return repository
}

func registerEchoTool(t *testing.T, registry *ToolRegistry, calls *atomic.Int32, executed chan<- struct{}) {
	t.Helper()
	err := registry.Register(ToolDefinition{
		Schema: ToolSpecSchema, Name: "knowledge_search", Description: "test search", ReadOnly: true, TimeoutMS: 1000,
		InputSchema: MustJSONValue(map[string]any{"type": "object"}),
	}, func(ctx context.Context, arguments JSONValue) (JSONValue, error) {
		if tenant.FromContext(ctx) == 0 {
			return nil, errors.New("missing tenant context")
		}
		calls.Add(1)
		if executed != nil {
			select {
			case executed <- struct{}{}:
			default:
			}
		}
		return MustJSONValue(map[string]any{"matches": []string{"durable agents"}}), nil
	})
	if err != nil {
		t.Fatalf("register tool: %v", err)
	}
}

func createPublishedAgent(t *testing.T, service *Service, tenantID uint, tools []string, maxSteps int) (context.Context, *AgentDefinition) {
	t.Helper()
	ctx := tenant.WithContext(context.Background(), tenantID)
	definition, err := service.CreateDefinition(ctx, 99, CreateDefinitionInput{AgentKey: "knowledge_agent", Name: "知识 Agent"})
	if err != nil {
		t.Fatalf("create definition: %v", err)
	}
	_, err = service.PublishVersion(ctx, definition.ID, 99, PublishVersionInput{
		Instructions: "Use tools when needed.", ModelID: 7, Tools: tools, MaxSteps: maxSteps, TimeoutSeconds: 30,
	})
	if err != nil {
		t.Fatalf("publish version: %v", err)
	}
	definition, err = service.GetDefinition(ctx, definition.ID)
	if err != nil {
		t.Fatalf("reload definition: %v", err)
	}
	return ctx, definition
}

func TestPublishVersionNormalizesNilToolsToArray(t *testing.T) {
	repository := newTestRepository(t)
	service := NewService(repository, &fakeModel{}, NewToolRegistry())
	ctx := tenant.WithContext(context.Background(), 49)
	definition, err := service.CreateDefinition(ctx, 99, CreateDefinitionInput{AgentKey: "no_tools_agent", Name: "No Tools"})
	if err != nil {
		t.Fatalf("create definition: %v", err)
	}
	version, err := service.PublishVersion(ctx, definition.ID, 99, PublishVersionInput{
		Instructions: "answer without tools", ModelID: 7, MaxSteps: 4, TimeoutSeconds: 30,
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if string(version.Tools) != `[]` {
		t.Fatalf("stored tools = %s, want []", version.Tools)
	}
	var spec AgentSpec
	if err := json.Unmarshal(version.Spec, &spec); err != nil {
		t.Fatalf("decode spec: %v", err)
	}
	if spec.Tools == nil {
		t.Fatal("published spec tools is nil, want empty array")
	}
}

func TestServiceLimitsCountUnicodeCharacters(t *testing.T) {
	repository := newTestRepository(t)
	service := NewService(repository, &fakeModel{}, NewToolRegistry())
	ctx := tenant.WithContext(context.Background(), 50)
	definition, err := service.CreateDefinition(ctx, 99, CreateDefinitionInput{
		AgentKey: "unicode_agent", Name: strings.Repeat("知", 100), Description: strings.Repeat("描", 500),
	})
	if err != nil {
		t.Fatalf("create definition: %v", err)
	}
	if _, err := service.PublishVersion(ctx, definition.ID, 99, PublishVersionInput{
		Instructions: strings.Repeat("指", 20_000), ModelID: 7, MaxSteps: 4, TimeoutSeconds: 30,
	}); err != nil {
		t.Fatalf("publish at unicode boundary: %v", err)
	}
	if _, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{
		Input: strings.Repeat("问", 20_000), IdempotencyKey: "unicode-boundary",
	}); err != nil {
		t.Fatalf("run at unicode boundary: %v", err)
	}
}

func TestServiceRejectsIDsOutsideSignedDatabaseRange(t *testing.T) {
	repository := newTestRepository(t)
	service := NewService(repository, &fakeModel{}, NewToolRegistry())
	ctx := tenant.WithContext(context.Background(), 53)
	if _, err := service.CreateDefinition(ctx, maxDatabaseID+1, CreateDefinitionInput{AgentKey: "range_agent", Name: "Range"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("oversized actor id error = %v, want validation", err)
	}
	definition, err := service.CreateDefinition(ctx, 99, CreateDefinitionInput{AgentKey: "range_agent", Name: "Range"})
	if err != nil {
		t.Fatalf("create definition: %v", err)
	}
	if _, err := service.PublishVersion(ctx, definition.ID, 99, PublishVersionInput{
		Instructions: "range", ModelID: maxDatabaseID + 1, MaxSteps: 4, TimeoutSeconds: 30,
	}); !errors.Is(err, ErrValidation) {
		t.Fatalf("oversized model id error = %v, want validation", err)
	}
}

func TestToolCallIdempotencyHashCanonicalizesJSON(t *testing.T) {
	first := ToolCall{Name: "knowledge_search", Arguments: JSONValue(`{"b":2,"a":1}`)}
	second := ToolCall{Name: "knowledge_search", Arguments: JSONValue(`
 {
  "a": 1, "b": 2
} `)}
	firstKey, firstHash, err := toolCallIdempotencyKey(12, 3, 0, first)
	if err != nil {
		t.Fatalf("first key: %v", err)
	}
	secondKey, secondHash, err := toolCallIdempotencyKey(12, 3, 0, second)
	if err != nil {
		t.Fatalf("second key: %v", err)
	}
	if firstKey != secondKey || firstHash != secondHash {
		t.Fatalf("equivalent arguments produced different identity: %s/%s vs %s/%s", firstKey, firstHash, secondKey, secondHash)
	}
}

func TestBeginToolCallFindsLegacyRawIdentity(t *testing.T) {
	repository := newTestRepository(t)
	registry := NewToolRegistry()
	var toolCalls atomic.Int32
	registerEchoTool(t, registry, &toolCalls, nil)
	service := NewService(repository, &fakeModel{}, registry)
	ctx, definition := createPublishedAgent(t, service, 47, []string{"knowledge_search"}, 4)
	created, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: "legacy-tool", IdempotencyKey: "legacy-tool-1"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	claimed, err := repository.ClaimNext(context.Background(), "worker-legacy", time.Minute)
	if err != nil || claimed == nil {
		t.Fatalf("claim: run=%v err=%v", claimed, err)
	}
	call := ToolCall{Name: "knowledge_search", ID: "call-legacy", Arguments: JSONValue(`{"b":2,"a":1}`)}
	step, err := repository.StartStep(ctx, *claimed, "worker-legacy", 1, StepKindTool, call.Name, call)
	if err != nil {
		t.Fatalf("start step: %v", err)
	}
	primaryKey, primaryHash, legacyKey, legacyHash, err := toolCallIdentity(created.Run.ID, 1, 0, call)
	if err != nil || legacyKey == "" {
		t.Fatalf("identities primary=%s legacy=%s err=%v", primaryKey, legacyKey, err)
	}
	now := time.Now().UTC()
	legacyCall := &AgentToolCall{
		TenantID: 47, RunID: claimed.ID, StepID: step.ID, IdempotencyKey: legacyKey, ToolName: call.Name,
		Arguments: call.Arguments.Clone(), ArgumentsSHA256: legacyHash, Status: StepSucceeded,
		Result: MustJSONValue(map[string]any{"legacy": true}), Attempt: 1, StartedAt: now, FinishedAt: &now, CreatedAt: now, UpdatedAt: now,
	}
	if err := repository.DB().WithContext(ctx).Create(legacyCall).Error; err != nil {
		t.Fatalf("insert legacy call: %v", err)
	}
	got, cached, err := repository.BeginToolCallWithLegacy(ctx, *claimed, "worker-legacy", *step, call, primaryKey, primaryHash, legacyKey, legacyHash)
	if err != nil || !cached || got.ID != legacyCall.ID || string(got.Result) != `{"legacy":true}` {
		t.Fatalf("legacy lookup call=%#v cached=%v err=%v", got, cached, err)
	}
	var count int64
	if err := repository.DB().Model(&AgentToolCall{}).Where("tenant_id = ? AND run_id = ?", 47, claimed.ID).Count(&count).Error; err != nil {
		t.Fatalf("count calls: %v", err)
	}
	if count != 1 {
		t.Fatalf("tool call rows = %d, want 1", count)
	}
}

func TestWorkerRejectsMalformedCheckpointWithoutPanic(t *testing.T) {
	repository := newTestRepository(t)
	model := &fakeModel{complete: func(context.Context, ModelRequest) (ModelResponse, error) {
		return ModelResponse{Content: "must not run"}, nil
	}}
	service := NewService(repository, model, NewToolRegistry())
	ctx, definition := createPublishedAgent(t, service, 46, nil, 4)
	created, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: "bad-checkpoint", IdempotencyKey: "bad-checkpoint-1"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	malformed := NewCheckpoint("bad-checkpoint")
	malformed.PendingToolCalls = []ToolCall{{ID: "call-1", Name: "knowledge_search", Arguments: MustJSONValue(map[string]any{"query": "x"})}}
	malformed.PendingToolIndex = -1
	malformed.PendingModelStep = 1
	checkpoint := MustJSONValue(malformed)
	if err := repository.DB().WithContext(ctx).Model(&AgentRun{}).
		Where("tenant_id = ? AND id = ?", 46, created.Run.ID).Update("checkpoint", checkpoint).Error; err != nil {
		t.Fatalf("corrupt checkpoint: %v", err)
	}
	worker := NewWorker(repository, model, NewToolRegistry(), WorkerConfig{ID: "worker-bad-checkpoint", LeaseDuration: time.Minute})
	worked, err := worker.RunOnce(context.Background())
	if err != nil || !worked {
		t.Fatalf("worker worked=%v err=%v", worked, err)
	}
	details, err := service.GetRunDetails(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("details: %v", err)
	}
	if details.Run.Status != RunFailed || details.Run.ErrorCode != "invalid_checkpoint" {
		t.Fatalf("malformed checkpoint = status %s code %s", details.Run.Status, details.Run.ErrorCode)
	}
	if model.calls.Load() != 0 {
		t.Fatalf("model calls after malformed checkpoint = %d, want 0", model.calls.Load())
	}
}

func TestToolRegistryRejectsNonObjectSchema(t *testing.T) {
	registry := NewToolRegistry()
	for _, schema := range []JSONValue{JSONValue(`null`), JSONValue(`[]`), JSONValue(`"string"`)} {
		err := registry.Register(ToolDefinition{
			Schema: ToolSpecSchema, Name: "schema_tool", Description: "invalid schema", ReadOnly: true, TimeoutMS: 1000,
			InputSchema: schema,
		}, func(context.Context, JSONValue) (JSONValue, error) { return MustJSONValue(map[string]any{}), nil })
		if !errors.Is(err, ErrValidation) {
			t.Fatalf("schema %s registration error = %v, want validation", schema, err)
		}
	}
}

func TestToolRegistryCopiesMutableSchemaInput(t *testing.T) {
	registry := NewToolRegistry()
	schema := JSONValue(`{"type":"object"}`)
	if err := registry.Register(ToolDefinition{
		Schema: ToolSpecSchema, Name: "schema_copy", Description: "copy", ReadOnly: true, TimeoutMS: 1000,
		InputSchema: schema,
	}, func(context.Context, JSONValue) (JSONValue, error) {
		return MustJSONValue(map[string]any{}), nil
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	schema[2] = 'X'
	definitions := registry.Definitions()
	if len(definitions) != 1 || string(definitions[0].InputSchema) != `{"type":"object"}` {
		t.Fatalf("stored schema = %s, want original schema", definitions[0].InputSchema)
	}
}

func TestServicePublishesImmutableVersionAndRunIdempotencyIsTenantScoped(t *testing.T) {
	repository := newTestRepository(t)
	model := &fakeModel{complete: func(context.Context, ModelRequest) (ModelResponse, error) {
		return ModelResponse{Content: "ok"}, nil
	}}
	registry := NewToolRegistry()
	var toolCalls atomic.Int32
	registerEchoTool(t, registry, &toolCalls, nil)
	service := NewService(repository, model, registry)
	ctx, definition := createPublishedAgent(t, service, 7, []string{"knowledge_search"}, 8)

	versions, err := service.ListVersions(ctx, definition.ID)
	if err != nil || len(versions) != 1 || versions[0].Version != 1 || versions[0].SchemaVersion != AgentSpecSchema {
		t.Fatalf("versions = %#v, err=%v", versions, err)
	}

	first, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: "find it", IdempotencyKey: "request-1"})
	if err != nil || first.Reused {
		t.Fatalf("first run = %#v, err=%v", first, err)
	}
	second, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: "find it", IdempotencyKey: "request-1"})
	if err != nil || !second.Reused || second.Run.ID != first.Run.ID {
		t.Fatalf("second run = %#v, err=%v", second, err)
	}
	_, err = service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: "different", IdempotencyKey: "request-1"})
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("different payload error = %v, want idempotency conflict", err)
	}

	otherTenant := tenant.WithContext(context.Background(), 8)
	if _, err := service.GetDefinition(otherTenant, definition.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant definition error = %v, want not found", err)
	}
	if _, err := service.GetRunDetails(otherTenant, first.Run.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant run error = %v, want not found", err)
	}
}

func TestCompleteToolStepRejectsLeaseLossBeforeCheckpointCommit(t *testing.T) {
	repository := newTestRepository(t)
	registry := NewToolRegistry()
	var toolCalls atomic.Int32
	registerEchoTool(t, registry, &toolCalls, nil)
	model := &fakeModel{complete: func(context.Context, ModelRequest) (ModelResponse, error) {
		return ModelResponse{Content: "ok"}, nil
	}}
	service := NewService(repository, model, registry)
	ctx, definition := createPublishedAgent(t, service, 7, []string{"knowledge_search"}, 4)
	created, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: "lease-race", IdempotencyKey: "lease-race-1"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	claimed, err := repository.ClaimNext(context.Background(), "worker-a", time.Minute)
	if err != nil || claimed == nil {
		t.Fatalf("claim: run=%v err=%v", claimed, err)
	}
	call := ToolCall{ID: "call-lease", Name: "knowledge_search", Arguments: MustJSONValue(map[string]any{"query": "lease"})}
	step, err := repository.StartStep(ctx, *claimed, "worker-a", 1, StepKindTool, call.Name, call)
	if err != nil {
		t.Fatalf("start step: %v", err)
	}
	idempotencyKey, argumentsHash, err := toolCallIdempotencyKey(claimed.ID, 1, 0, call)
	if err != nil {
		t.Fatalf("tool key: %v", err)
	}
	persisted, cached, err := repository.BeginToolCall(ctx, *claimed, "worker-a", *step, call, idempotencyKey, argumentsHash)
	if err != nil || cached || persisted == nil {
		t.Fatalf("begin tool call: call=%v cached=%v err=%v", persisted, cached, err)
	}
	// 负控：step 标记成功时模拟另一个 worker 抢走租约；随后 checkpoint UPDATE
	// 的 WHERE lease_owner 应为 0 行，整个事务必须回滚而不是静默提交半条轨迹。
	if err := repository.DB().Exec(`
CREATE TRIGGER steal_agent_lease_after_step
AFTER UPDATE OF status ON agent_steps
WHEN NEW.status = 'succeeded'
BEGIN
  UPDATE agent_runs SET lease_owner = 'worker-b' WHERE id = NEW.run_id;
END;`).Error; err != nil {
		t.Fatalf("create lease trigger: %v", err)
	}
	checkpoint := NewCheckpoint("lease-race")
	checkpoint.NextOrdinal = 2
	result := MustJSONValue(map[string]any{"matches": []string{"ok"}})
	if err := repository.CompleteToolStep(ctx, *claimed, "worker-a", *step, *persisted, result, checkpoint, false); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("complete tool step error = %v, want lease lost", err)
	}
	var current AgentRun
	if err := repository.DB().Where("id = ?", created.Run.ID).First(&current).Error; err != nil {
		t.Fatalf("reload run: %v", err)
	}
	if current.LeaseOwner != "worker-a" {
		t.Fatalf("lease owner after rollback = %q, want worker-a", current.LeaseOwner)
	}
	var currentStep AgentStep
	if err := repository.DB().Where("id = ?", step.ID).First(&currentStep).Error; err != nil {
		t.Fatalf("reload step: %v", err)
	}
	if currentStep.Status != StepRunning {
		t.Fatalf("step status after rollback = %q, want running", currentStep.Status)
	}
}

func TestToolPanicBecomesToolFailure(t *testing.T) {
	repository := newTestRepository(t)
	registry := NewToolRegistry()
	if err := registry.Register(ToolDefinition{
		Schema: ToolSpecSchema, Name: "knowledge_search", Description: "panic search", ReadOnly: true, TimeoutMS: 1000,
		InputSchema: MustJSONValue(map[string]any{"type": "object"}),
	}, func(context.Context, JSONValue) (JSONValue, error) {
		panic("test-only tool panic")
	}); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	model := &fakeModel{complete: func(context.Context, ModelRequest) (ModelResponse, error) {
		return ModelResponse{ToolCalls: []ToolCall{{ID: "call-panic", Name: "knowledge_search", Arguments: MustJSONValue(map[string]any{"query": "panic"})}}}, nil
	}}
	service := NewService(repository, model, registry)
	ctx, definition := createPublishedAgent(t, service, 52, []string{"knowledge_search"}, 4)
	created, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: "tool-panic", IdempotencyKey: "tool-panic-1"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	worker := NewWorker(repository, model, registry, WorkerConfig{ID: "worker-tool-panic", LeaseDuration: time.Minute})
	if worked, err := worker.RunOnce(context.Background()); err != nil || !worked {
		t.Fatalf("worker worked=%v err=%v", worked, err)
	}
	details, err := service.GetRunDetails(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("details: %v", err)
	}
	if details.Run.Status != RunFailed || details.Run.ErrorCode != "tool_execution_failed" || len(details.ToolCalls) != 1 || details.ToolCalls[0].Status != StepFailed {
		t.Fatalf("panic run=%s/%s calls=%#v", details.Run.Status, details.Run.ErrorCode, details.ToolCalls)
	}
}

func TestModelPanicBecomesModelFailure(t *testing.T) {
	repository := newTestRepository(t)
	model := &fakeModel{complete: func(context.Context, ModelRequest) (ModelResponse, error) {
		panic("test-only model panic")
	}}
	service := NewService(repository, model, NewToolRegistry())
	ctx, definition := createPublishedAgent(t, service, 53, nil, 4)
	created, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: "model-panic", IdempotencyKey: "model-panic-1"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	worker := NewWorker(repository, model, NewToolRegistry(), WorkerConfig{ID: "worker-model-panic", LeaseDuration: time.Minute})
	if worked, err := worker.RunOnce(context.Background()); err != nil || !worked {
		t.Fatalf("worker worked=%v err=%v", worked, err)
	}
	details, err := service.GetRunDetails(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("details: %v", err)
	}
	if details.Run.Status != RunFailed || details.Run.ErrorCode != "model_call_failed" || !strings.Contains(details.Run.ErrorMessage, "model adapter panicked") {
		t.Fatalf("panic run=%s/%s/%s", details.Run.Status, details.Run.ErrorCode, details.Run.ErrorMessage)
	}
}

func TestToolExecutionFailureClosesToolCall(t *testing.T) {
	repository := newTestRepository(t)
	registry := NewToolRegistry()
	if err := registry.Register(ToolDefinition{
		Schema: ToolSpecSchema, Name: "knowledge_search", Description: "failing search", ReadOnly: true, TimeoutMS: 1000,
		InputSchema: MustJSONValue(map[string]any{"type": "object"}),
	}, func(context.Context, JSONValue) (JSONValue, error) {
		return nil, errors.New("backend unavailable")
	}); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	model := &fakeModel{complete: func(context.Context, ModelRequest) (ModelResponse, error) {
		return ModelResponse{ToolCalls: []ToolCall{{ID: "call-fail", Name: "knowledge_search", Arguments: MustJSONValue(map[string]any{"query": "failure"})}}}, nil
	}}
	service := NewService(repository, model, registry)
	ctx, definition := createPublishedAgent(t, service, 48, []string{"knowledge_search"}, 4)
	created, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: "tool-failure", IdempotencyKey: "tool-failure-1"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	worker := NewWorker(repository, model, registry, WorkerConfig{ID: "worker-tool-failure", LeaseDuration: time.Minute})
	if worked, err := worker.RunOnce(context.Background()); err != nil || !worked {
		t.Fatalf("worker worked=%v err=%v", worked, err)
	}
	details, err := service.GetRunDetails(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("details: %v", err)
	}
	if details.Run.Status != RunFailed || details.Run.ErrorCode != "tool_execution_failed" {
		t.Fatalf("run = status %s code %s", details.Run.Status, details.Run.ErrorCode)
	}
	if len(details.Steps) != 2 || details.Steps[1].Status != StepFailed {
		t.Fatalf("steps = %#v", details.Steps)
	}
	if len(details.ToolCalls) != 1 || details.ToolCalls[0].Status != StepFailed || details.ToolCalls[0].ErrorCode != "tool_execution_failed" {
		t.Fatalf("tool calls = %#v", details.ToolCalls)
	}
	foundToolFailure := false
	for _, event := range details.Events {
		if event.EventType == "tool.failed" {
			foundToolFailure = true
			break
		}
	}
	if !foundToolFailure {
		t.Fatalf("events do not contain tool.failed: %#v", details.Events)
	}
}

func TestCancellationClosesRunningToolCall(t *testing.T) {
	repository := newTestRepository(t)
	registry := NewToolRegistry()
	toolStarted := make(chan struct{}, 1)
	if err := registry.Register(ToolDefinition{
		Schema: ToolSpecSchema, Name: "knowledge_search", Description: "blocking search", ReadOnly: true, TimeoutMS: 10_000,
		InputSchema: MustJSONValue(map[string]any{"type": "object"}),
	}, func(ctx context.Context, arguments JSONValue) (JSONValue, error) {
		toolStarted <- struct{}{}
		<-ctx.Done()
		return nil, ctx.Err()
	}); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	model := &fakeModel{complete: func(context.Context, ModelRequest) (ModelResponse, error) {
		return ModelResponse{ToolCalls: []ToolCall{{ID: "call-cancel", Name: "knowledge_search", Arguments: MustJSONValue(map[string]any{"query": "cancel"})}}}, nil
	}}
	service := NewService(repository, model, registry)
	ctx, definition := createPublishedAgent(t, service, 51, []string{"knowledge_search"}, 4)
	created, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: "tool-cancel", IdempotencyKey: "tool-cancel-1"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	worker := NewWorker(repository, model, registry, WorkerConfig{ID: "worker-tool-cancel", LeaseDuration: 300 * time.Millisecond})
	done := make(chan error, 1)
	go func() {
		_, runErr := worker.RunOnce(context.Background())
		done <- runErr
	}()
	select {
	case <-toolStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("tool did not start")
	}
	if _, err := service.CancelRun(ctx, created.Run.ID); err != nil {
		t.Fatalf("request cancel: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("worker result: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not finish cancellation")
	}
	details, err := service.GetRunDetails(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("details: %v", err)
	}
	if details.Run.Status != RunCancelled || len(details.ToolCalls) != 1 || details.ToolCalls[0].Status != StepCancelled {
		t.Fatalf("cancelled run=%s tool_calls=%#v", details.Run.Status, details.ToolCalls)
	}
	foundToolCancelled := false
	for _, event := range details.Events {
		if event.EventType == "tool.cancelled" {
			foundToolCancelled = true
			break
		}
	}
	if !foundToolCancelled {
		t.Fatalf("events do not contain tool.cancelled: %#v", details.Events)
	}
}

func TestWorkerRunsModelToolModelAndPersistsTrace(t *testing.T) {
	repository := newTestRepository(t)
	registry := NewToolRegistry()
	var toolCalls atomic.Int32
	registerEchoTool(t, registry, &toolCalls, nil)
	model := &fakeModel{}
	model.complete = func(ctx context.Context, request ModelRequest) (ModelResponse, error) {
		if model.calls.Load() == 1 {
			return ModelResponse{
				ToolCalls: []ToolCall{{ID: "call_1", Name: "knowledge_search", Arguments: MustJSONValue(map[string]any{"query": "agents"})}},
				Usage:     Usage{InputTokens: 10, OutputTokens: 3}, StopReason: "tool_calls",
			}, nil
		}
		if got := request.Messages[len(request.Messages)-1].Role; got != "tool" {
			return ModelResponse{}, fmt.Errorf("last role = %s, want tool", got)
		}
		return ModelResponse{Content: "durable agents", Usage: Usage{InputTokens: 20, OutputTokens: 4}, StopReason: "stop"}, nil
	}
	service := NewService(repository, model, registry)
	ctx, definition := createPublishedAgent(t, service, 7, []string{"knowledge_search"}, 8)
	created, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: "search", IdempotencyKey: "tool-flow"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	worker := NewWorker(repository, model, registry, WorkerConfig{ID: "worker-a", LeaseDuration: time.Second})
	worked, err := worker.RunOnce(context.Background())
	if err != nil || !worked {
		t.Fatalf("worker result worked=%v err=%v", worked, err)
	}
	details, err := service.GetRunDetails(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("run details: %v", err)
	}
	if details.Run.Status != RunSucceeded || details.Run.Output != "durable agents" {
		t.Fatalf("run terminal = (%s,%q)", details.Run.Status, details.Run.Output)
	}
	if len(details.Steps) != 3 || details.Steps[0].Kind != StepKindModel || details.Steps[1].Kind != StepKindTool || details.Steps[2].Kind != StepKindModel {
		t.Fatalf("steps = %#v", details.Steps)
	}
	if len(details.ToolCalls) != 1 || details.ToolCalls[0].Status != StepSucceeded || toolCalls.Load() != 1 {
		t.Fatalf("tool calls persisted=%d executed=%d", len(details.ToolCalls), toolCalls.Load())
	}
	if len(details.Events) < 9 || details.Events[len(details.Events)-1].EventType != "run.succeeded" {
		t.Fatalf("events = %#v", details.Events)
	}
}

func TestWorkerRejectsVersionSpecDrift(t *testing.T) {
	repository := newTestRepository(t)
	model := &fakeModel{complete: func(context.Context, ModelRequest) (ModelResponse, error) {
		return ModelResponse{Content: "must not run"}, nil
	}}
	service := NewService(repository, model, NewToolRegistry())
	ctx, definition := createPublishedAgent(t, service, 44, nil, 4)
	if definition.LatestVersionID == nil {
		t.Fatal("latest version is nil")
	}
	if err := repository.DB().WithContext(ctx).Model(&AgentVersion{}).
		Where("tenant_id = ? AND id = ?", 44, *definition.LatestVersionID).
		Update("spec_sha256", "0000000000000000000000000000000000000000000000000000000000000000").Error; err != nil {
		t.Fatalf("corrupt version spec hash: %v", err)
	}
	created, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: "spec-drift", IdempotencyKey: "spec-drift-1"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	worker := NewWorker(repository, model, NewToolRegistry(), WorkerConfig{ID: "worker-spec-drift", LeaseDuration: time.Minute})
	worked, err := worker.RunOnce(context.Background())
	if err != nil || !worked {
		t.Fatalf("worker worked=%v err=%v", worked, err)
	}
	details, err := service.GetRunDetails(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("details: %v", err)
	}
	if details.Run.Status != RunFailed || details.Run.ErrorCode != "invalid_version" {
		t.Fatalf("drifted version = status %s code %s", details.Run.Status, details.Run.ErrorCode)
	}
	if model.calls.Load() != 0 {
		t.Fatalf("model calls after version drift = %d, want 0", model.calls.Load())
	}
}

func TestWorkerRejectsPublishedToolSpecDrift(t *testing.T) {
	repository := newTestRepository(t)
	registry := NewToolRegistry()
	var toolCalls atomic.Int32
	registerEchoTool(t, registry, &toolCalls, nil)
	var modelCalls atomic.Int32
	model := &fakeModel{complete: func(context.Context, ModelRequest) (ModelResponse, error) {
		modelCalls.Add(1)
		return ModelResponse{Content: "must not run"}, nil
	}}
	service := NewService(repository, model, registry)
	ctx, definition := createPublishedAgent(t, service, 45, []string{"knowledge_search"}, 4)
	registered := registry.tools["knowledge_search"]
	registered.definition.Description = "changed after publication"
	registered.hash = "0000000000000000000000000000000000000000000000000000000000000000"
	registry.tools["knowledge_search"] = registered
	created, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: "tool-drift", IdempotencyKey: "tool-drift-1"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	worker := NewWorker(repository, model, registry, WorkerConfig{ID: "worker-tool-drift", LeaseDuration: time.Minute})
	worked, err := worker.RunOnce(context.Background())
	if err != nil || !worked {
		t.Fatalf("worker worked=%v err=%v", worked, err)
	}
	details, err := service.GetRunDetails(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("details: %v", err)
	}
	if details.Run.Status != RunFailed || details.Run.ErrorCode != "tool_spec_changed" {
		t.Fatalf("drifted run = status %s code %s, want tool_spec_changed failure", details.Run.Status, details.Run.ErrorCode)
	}
	if modelCalls.Load() != 0 {
		t.Fatalf("model calls after tool drift = %d, want 0", modelCalls.Load())
	}
}

func TestRunTimeoutCoversLeaseRecoveryAttempts(t *testing.T) {
	repository := newTestRepository(t)
	registry := NewToolRegistry()
	var modelCalls atomic.Int32
	model := &fakeModel{complete: func(context.Context, ModelRequest) (ModelResponse, error) {
		modelCalls.Add(1)
		return ModelResponse{Content: "unexpected after timeout"}, nil
	}}
	service := NewService(repository, model, registry)
	ctx, definition := createPublishedAgent(t, service, 43, nil, 4)
	created, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: "timeout-recovery", IdempotencyKey: "timeout-recovery-1"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	startedAt := time.Now().Add(-time.Minute)
	if err := repository.DB().WithContext(ctx).Model(&AgentRun{}).
		Where("tenant_id = ? AND id = ?", 43, created.Run.ID).
		Update("started_at", startedAt).Error; err != nil {
		t.Fatalf("age run start: %v", err)
	}
	worker := NewWorker(repository, model, registry, WorkerConfig{ID: "worker-timeout", LeaseDuration: time.Minute})
	worked, err := worker.RunOnce(context.Background())
	if err != nil || !worked {
		t.Fatalf("worker worked=%v err=%v", worked, err)
	}
	details, err := service.GetRunDetails(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("details: %v", err)
	}
	if details.Run.Status != RunFailed || details.Run.ErrorCode != "run_timeout" {
		t.Fatalf("expired run = status %s code %s, want run_timeout failure", details.Run.Status, details.Run.ErrorCode)
	}
	if modelCalls.Load() != 0 {
		t.Fatalf("model calls after exhausted timeout = %d, want 0", modelCalls.Load())
	}
}

func TestRunCannotSucceedAfterPersistedTimeout(t *testing.T) {
	repository := newTestRepository(t)
	registry := NewToolRegistry()
	model := &fakeModel{complete: func(context.Context, ModelRequest) (ModelResponse, error) {
		// 故意忽略 runCtx 的 deadline，验证 repository 的持久化总预算仍然
		// 拦截成功终态，而不是把 Provider 的不合作当成成功。
		time.Sleep(1200 * time.Millisecond)
		return ModelResponse{Content: "late result"}, nil
	}}
	service := NewService(repository, model, registry)
	ctx, definition := createPublishedAgent(t, service, 42, nil, 4)
	created, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: "late-timeout", IdempotencyKey: "late-timeout-1"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	// 测试 helper 发布的是 30 秒版本；让 Run 在模型返回前越过这个
	// 持久化 deadline。
	startedAt := time.Now().UTC().Add(-29 * time.Second)
	if err := repository.DB().WithContext(ctx).Model(&AgentRun{}).
		Where("tenant_id = ? AND id = ?", 42, created.Run.ID).Update("started_at", startedAt).Error; err != nil {
		t.Fatalf("age run start: %v", err)
	}
	worker := NewWorker(repository, model, registry, WorkerConfig{ID: "worker-late-timeout", LeaseDuration: 2 * time.Second})
	worked, err := worker.RunOnce(context.Background())
	if err != nil || !worked {
		t.Fatalf("worker worked=%v err=%v", worked, err)
	}
	details, err := service.GetRunDetails(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("details: %v", err)
	}
	if details.Run.Status != RunFailed || details.Run.ErrorCode != "run_timeout" {
		t.Fatalf("late result = status %s code %s output %q", details.Run.Status, details.Run.ErrorCode, details.Run.Output)
	}
}

func TestExpiredLeaseFinalModelStepIsNotReplayed(t *testing.T) {
	repository := newTestRepository(t)
	registry := NewToolRegistry()
	var modelCalls atomic.Int32
	model := &fakeModel{complete: func(context.Context, ModelRequest) (ModelResponse, error) {
		modelCalls.Add(1)
		return ModelResponse{Content: "unexpected replay"}, nil
	}}
	service := NewService(repository, model, registry)
	ctx, definition := createPublishedAgent(t, service, 41, nil, 4)
	created, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: "final-crash", IdempotencyKey: "final-crash-1"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	claimed, err := repository.ClaimNext(context.Background(), "worker-a", time.Minute)
	if err != nil || claimed == nil {
		t.Fatalf("claim: run=%v err=%v", claimed, err)
	}
	version, err := repository.LoadVersionForRun(ctx, *claimed)
	if err != nil {
		t.Fatalf("load version: %v", err)
	}
	checkpoint, err := claimed.DecodeCheckpoint()
	if err != nil {
		t.Fatalf("decode checkpoint: %v", err)
	}
	step, err := repository.StartStep(ctx, *claimed, "worker-a", checkpoint.NextOrdinal, StepKindModel, "model.complete", map[string]any{
		"model_id": version.ModelID, "messages": checkpoint.Messages,
	})
	if err != nil {
		t.Fatalf("start final step: %v", err)
	}
	response := ModelResponse{Content: "durable final", Usage: Usage{InputTokens: 2, OutputTokens: 1}, StopReason: "stop"}
	checkpoint.Messages = append(checkpoint.Messages, Message{Role: "assistant", Content: response.Content})
	checkpoint.NextOrdinal++
	if err := repository.CompleteModelStep(ctx, *claimed, "worker-a", *step, response, checkpoint); err != nil {
		t.Fatalf("commit final step: %v", err)
	}
	// 模拟进程在 step/checkpoint 事务提交后、CompleteRun 前崩溃。
	if err := repository.ReleaseLease(context.Background(), *claimed, "worker-a"); err != nil {
		t.Fatalf("release crashed lease: %v", err)
	}
	worker := NewWorker(repository, model, registry, WorkerConfig{ID: "worker-b", LeaseDuration: time.Minute})
	worked, err := worker.RunOnce(context.Background())
	if err != nil || !worked {
		t.Fatalf("resume worker worked=%v err=%v", worked, err)
	}
	details, err := service.GetRunDetails(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("details: %v", err)
	}
	if details.Run.Status != RunSucceeded || details.Run.Output != "durable final" {
		t.Fatalf("resumed final run = status %s output %q", details.Run.Status, details.Run.Output)
	}
	if modelCalls.Load() != 0 || len(details.Steps) != 1 {
		t.Fatalf("final step was replayed: model calls=%d steps=%d", modelCalls.Load(), len(details.Steps))
	}
}

func TestExpiredLeaseResumesCheckpointWithoutRepeatingCompletedTool(t *testing.T) {
	repository := newTestRepository(t)
	registry := NewToolRegistry()
	var toolCalls atomic.Int32
	toolExecuted := make(chan struct{}, 1)
	registerEchoTool(t, registry, &toolCalls, toolExecuted)
	secondModelStarted := make(chan struct{}, 1)
	model := &fakeModel{}
	model.complete = func(ctx context.Context, request ModelRequest) (ModelResponse, error) {
		switch model.calls.Load() {
		case 1:
			return ModelResponse{ToolCalls: []ToolCall{{ID: "call_1", Name: "knowledge_search", Arguments: MustJSONValue(map[string]any{"query": "resume"})}}}, nil
		case 2:
			secondModelStarted <- struct{}{}
			<-ctx.Done()
			return ModelResponse{}, ctx.Err()
		default:
			return ModelResponse{Content: "resumed"}, nil
		}
	}
	service := NewService(repository, model, registry)
	ctx, definition := createPublishedAgent(t, service, 7, []string{"knowledge_search"}, 8)
	created, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: "resume", IdempotencyKey: "resume-1"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstWorker := NewWorker(repository, model, registry, WorkerConfig{ID: "worker-crash", LeaseDuration: 300 * time.Millisecond})
	firstDone := make(chan error, 1)
	go func() {
		_, runErr := firstWorker.RunOnce(firstCtx)
		firstDone <- runErr
	}()
	select {
	case <-toolExecuted:
	case <-time.After(2 * time.Second):
		t.Fatal("tool did not execute")
	}
	select {
	case <-secondModelStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("second model step did not start")
	}
	cancelFirst()
	if err := <-firstDone; err != nil {
		t.Fatalf("crashed worker release: %v", err)
	}

	secondWorker := NewWorker(repository, model, registry, WorkerConfig{ID: "worker-resume", LeaseDuration: time.Second})
	worked, err := secondWorker.RunOnce(context.Background())
	if err != nil || !worked {
		t.Fatalf("resume worker worked=%v err=%v", worked, err)
	}
	details, err := service.GetRunDetails(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("details: %v", err)
	}
	if details.Run.Status != RunSucceeded || details.Run.Attempt != 2 || details.Run.Output != "resumed" {
		t.Fatalf("resumed run = status %s attempt %d output %q", details.Run.Status, details.Run.Attempt, details.Run.Output)
	}
	if toolCalls.Load() != 1 || len(details.ToolCalls) != 1 || len(details.Steps) != 3 {
		t.Fatalf("recovery duplicated work: tool executions=%d calls=%d steps=%d", toolCalls.Load(), len(details.ToolCalls), len(details.Steps))
	}
	var resumedEvents, ordinalThreeStarts, ordinalThreeResumed int
	for _, event := range details.Events {
		if event.EventType == "run.resumed" {
			resumedEvents++
		}
		if event.EventType == "step.started" && event.StepID != nil && *event.StepID == details.Steps[2].ID {
			ordinalThreeStarts++
			var payload map[string]any
			if err := json.Unmarshal(event.Data, &payload); err != nil {
				t.Fatalf("decode event payload: %v", err)
			}
			if payload["resumed"] == true {
				ordinalThreeResumed++
			}
		}
	}
	// 恢复重入同一 model step 会补发一条带 resumed 标记的 step.started（审计可区分首次/恢复执行）。
	if resumedEvents != 1 || ordinalThreeStarts != 2 || ordinalThreeResumed != 1 {
		t.Fatalf("resume events=%d ordinal-3 starts=%d resumed=%d", resumedEvents, ordinalThreeStarts, ordinalThreeResumed)
	}
}

func TestOnlyOneWorkerCanHoldUnexpiredLease(t *testing.T) {
	repository := newTestRepository(t)
	registry := NewToolRegistry()
	model := &fakeModel{complete: func(context.Context, ModelRequest) (ModelResponse, error) { return ModelResponse{Content: "ok"}, nil }}
	service := NewService(repository, model, registry)
	ctx, definition := createPublishedAgent(t, service, 7, nil, 4)
	if _, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: "claim", IdempotencyKey: "claim-1"}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	var claimed atomic.Int32
	var wg sync.WaitGroup
	errorsCh := make(chan error, 2)
	for _, owner := range []string{"worker-a", "worker-b"} {
		wg.Add(1)
		go func(owner string) {
			defer wg.Done()
			run, err := repository.ClaimNext(context.Background(), owner, time.Second)
			if err != nil {
				errorsCh <- err
				return
			}
			if run != nil {
				claimed.Add(1)
			}
		}(owner)
	}
	wg.Wait()
	close(errorsCh)
	for err := range errorsCh {
		t.Fatalf("claim error: %v", err)
	}
	if claimed.Load() != 1 {
		t.Fatalf("claimed workers = %d, want 1", claimed.Load())
	}
}

func TestModelFailureYieldsToConcurrentCancel(t *testing.T) {
	repository := newTestRepository(t)
	registry := NewToolRegistry()
	modelStarted := make(chan struct{}, 1)
	releaseModel := make(chan struct{})
	model := &fakeModel{complete: func(ctx context.Context, request ModelRequest) (ModelResponse, error) {
		modelStarted <- struct{}{}
		<-releaseModel
		return ModelResponse{}, errors.New("provider failed")
	}}
	service := NewService(repository, model, registry)
	ctx, definition := createPublishedAgent(t, service, 37, nil, 4)
	created, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: "failure-cancel", IdempotencyKey: "failure-cancel-1"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	worker := NewWorker(repository, model, registry, WorkerConfig{ID: "worker-failure-cancel", LeaseDuration: time.Minute})
	done := make(chan error, 1)
	go func() {
		_, runErr := worker.RunOnce(context.Background())
		done <- runErr
	}()
	select {
	case <-modelStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("model did not start")
	}
	if _, err := service.CancelRun(ctx, created.Run.ID); err != nil {
		t.Fatalf("request cancel: %v", err)
	}
	close(releaseModel)
	if err := <-done; err != nil {
		t.Fatalf("worker result: %v", err)
	}
	details, err := service.GetRunDetails(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("details: %v", err)
	}
	if details.Run.Status != RunCancelled {
		t.Fatalf("run status = %s (error=%s), want cancelled", details.Run.Status, details.Run.ErrorMessage)
	}
}

func TestCancellationInterruptsBlockingModelAndBecomesTerminal(t *testing.T) {
	repository := newTestRepository(t)
	registry := NewToolRegistry()
	modelStarted := make(chan struct{}, 1)
	model := &fakeModel{complete: func(ctx context.Context, request ModelRequest) (ModelResponse, error) {
		modelStarted <- struct{}{}
		<-ctx.Done()
		return ModelResponse{}, ctx.Err()
	}}
	service := NewService(repository, model, registry)
	ctx, definition := createPublishedAgent(t, service, 7, nil, 4)
	created, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: "cancel", IdempotencyKey: "cancel-1"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	worker := NewWorker(repository, model, registry, WorkerConfig{ID: "worker-cancel", LeaseDuration: 300 * time.Millisecond})
	done := make(chan error, 1)
	go func() {
		_, runErr := worker.RunOnce(context.Background())
		done <- runErr
	}()
	select {
	case <-modelStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("model did not start")
	}
	if _, err := service.CancelRun(ctx, created.Run.ID); err != nil {
		t.Fatalf("request cancel: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("worker cancel result: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not observe cancellation")
	}
	details, err := service.GetRunDetails(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("details: %v", err)
	}
	if details.Run.Status != RunCancelled || details.Run.FinishedAt == nil {
		t.Fatalf("cancelled run = %#v", details.Run)
	}
	if len(details.Steps) != 1 || details.Steps[0].Status != StepCancelled {
		t.Fatalf("cancelled step trace = %#v", details.Steps)
	}
}

func TestClaimNextSQLiteReadsBackExactCandidateUnderFixedClock(t *testing.T) {
	repository := newTestRepository(t)
	registry := NewToolRegistry()
	model := &fakeModel{complete: func(context.Context, ModelRequest) (ModelResponse, error) { return ModelResponse{Content: "ok"}, nil }}
	service := NewService(repository, model, registry)
	ctx, definition := createPublishedAgent(t, service, 11, nil, 4)
	first, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: "one", IdempotencyKey: "clock-1"})
	if err != nil {
		t.Fatalf("create run 1: %v", err)
	}
	second, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: "two", IdempotencyKey: "clock-2"})
	if err != nil {
		t.Fatalf("create run 2: %v", err)
	}

	// 固定时钟：旧实现按 lease_owner+heartbeat_at 回读，同一 now 值连续 claim
	// 两次会 First 命中同一行，导致第二条 run 被置为 Running 却无人处理。
	frozen := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	repository.now = func() time.Time { return frozen }

	runA, err := repository.ClaimNext(context.Background(), "worker-a", time.Minute)
	if err != nil || runA == nil {
		t.Fatalf("claim 1: run=%v err=%v", runA, err)
	}
	runB, err := repository.ClaimNext(context.Background(), "worker-a", time.Minute)
	if err != nil || runB == nil {
		t.Fatalf("claim 2: run=%v err=%v", runB, err)
	}
	if runA.ID != first.Run.ID || runB.ID != second.Run.ID {
		t.Fatalf("claims = (%d,%d), want (%d,%d)", runA.ID, runB.ID, first.Run.ID, second.Run.ID)
	}
	if runA.Status != RunRunning || runB.Status != RunRunning {
		t.Fatalf("claimed statuses = (%s,%s), want both running", runA.Status, runB.Status)
	}
}

// 模拟真实 GORM tenant 插件/ModelPort 的前提：模型调用 ctx 必须带租户，
// 否则拒绝（缺少租户会导致配额归因、用量记账与模型目录查询全部失败）。
type tenantCheckingModel struct {
	fakeModel
}

func (m *tenantCheckingModel) Complete(ctx context.Context, request ModelRequest) (ModelResponse, error) {
	if tenant.FromContext(ctx) == 0 {
		return ModelResponse{}, errors.New("model call requires tenant context")
	}
	return m.fakeModel.Complete(ctx, request)
}

func TestProcessRunPassesTenantContextToModelCalls(t *testing.T) {
	repository := newTestRepository(t)
	registry := NewToolRegistry()
	model := &tenantCheckingModel{fakeModel{complete: func(context.Context, ModelRequest) (ModelResponse, error) {
		return ModelResponse{Content: "ok", Usage: Usage{InputTokens: 1, OutputTokens: 1}, StopReason: "stop"}, nil
	}}}
	service := NewService(repository, model, registry)
	tenantID := uint(31)
	ctx, definition := createPublishedAgent(t, service, tenantID, nil, 4)
	created, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: "tenant-model", IdempotencyKey: "tenant-model-1"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	worker := NewWorker(repository, model, registry, WorkerConfig{ID: "worker-a", LeaseDuration: time.Minute})
	if _, err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	details, err := service.GetRunDetails(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("run details: %v", err)
	}
	if details.Run.Status != RunSucceeded {
		t.Fatalf("run status = %s (err=%s), want succeeded via tenant-scoped model call", details.Run.Status, details.Run.ErrorMessage)
	}
	if model.calls.Load() == 0 {
		t.Fatalf("model was never called")
	}
}

func TestCompleteRunYieldsToConcurrentCancel(t *testing.T) {
	repository := newTestRepository(t)
	registry := NewToolRegistry()
	tenantID := uint(23)
	model := &fakeModel{complete: func(context.Context, ModelRequest) (ModelResponse, error) {
		return ModelResponse{Content: "done", Usage: Usage{InputTokens: 1, OutputTokens: 1}, StopReason: "stop"}, nil
	}}
	service := NewService(repository, model, registry)
	ctx, definition := createPublishedAgent(t, service, tenantID, nil, 4)
	created, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: "cancel-race", IdempotencyKey: "cancel-race-1"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	worker := NewWorker(repository, model, registry, WorkerConfig{ID: "worker-a", LeaseDuration: time.Minute})

	// 先持有租约再请求取消：模拟取消请求与执行中并发（queued 取消另有终态分支）。
	claimed, err := repository.ClaimNext(context.Background(), worker.config.ID, worker.config.LeaseDuration)
	if err != nil || claimed == nil {
		t.Fatalf("claim: run=%v err=%v", claimed, err)
	}
	cancelCtx := tenant.WithContext(context.Background(), tenantID)
	if _, err := repository.RequestCancel(cancelCtx, created.Run.ID); err != nil {
		t.Fatalf("request cancel: %v", err)
	}

	if err := worker.processRun(context.Background(), *claimed); err != nil {
		t.Fatalf("process run: %v", err)
	}
	details, err := service.GetRunDetails(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("run details: %v", err)
	}
	if details.Run.Status != RunCancelled || details.Run.FinishedAt == nil {
		t.Fatalf("run terminal = (%s,%v), want cancelled", details.Run.Status, details.Run.FinishedAt)
	}
	if len(details.Steps) != 1 || details.Steps[0].Status != StepSucceeded {
		t.Fatalf("model step = %#v, want succeeded under cancelling", details.Steps)
	}
}

func TestStartStepMarksResumedEventOnReentry(t *testing.T) {
	repository := newTestRepository(t)
	registry := NewToolRegistry()
	model := &fakeModel{complete: func(context.Context, ModelRequest) (ModelResponse, error) { return ModelResponse{Content: "ok"}, nil }}
	service := NewService(repository, model, registry)
	tenantID := uint(29)
	ctx, definition := createPublishedAgent(t, service, tenantID, nil, 4)
	created, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: "resume-event", IdempotencyKey: "resume-event-1"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := repository.ClaimNext(context.Background(), "worker-a", time.Minute); err != nil {
		t.Fatalf("claim: %v", err)
	}
	runCtx := tenant.WithContext(context.Background(), tenantID)
	run := &AgentRun{TenantID: tenantID, ID: created.Run.ID}
	request := MustJSONValue(map[string]any{"messages": []any{}})
	if _, err := repository.StartStep(runCtx, *run, "worker-a", 1, StepKindModel, "model.complete", request); err != nil {
		t.Fatalf("start step: %v", err)
	}
	// 模拟崩溃恢复重入同一 ordinal。
	if _, err := repository.StartStep(runCtx, *run, "worker-a", 1, StepKindModel, "model.complete", request); err != nil {
		t.Fatalf("restart step: %v", err)
	}
	details, err := repository.GetRunDetails(runCtx, created.Run.ID)
	if err != nil {
		t.Fatalf("details: %v", err)
	}
	started := 0
	resumed := false
	for _, event := range details.Events {
		if event.EventType != "step.started" {
			continue
		}
		started++
		var payload map[string]any
		if err := json.Unmarshal(event.Data, &payload); err != nil {
			t.Fatalf("decode event payload: %v", err)
		}
		if payload["resumed"] == true {
			resumed = true
		}
	}
	if started != 2 || !resumed {
		t.Fatalf("step.started events=%d resumed=%v, want 2/true", started, resumed)
	}
}

func TestTruncateRespectsUTF8Boundary(t *testing.T) {
	value := "中文中文中文"
	if got := truncate(value, 7); !utf8.ValidString(got) || got != "中文" {
		t.Fatalf("truncate = %q (%d bytes), want valid %q", got, len(got), "中文")
	}
	if got := truncate(value, 3); got != "中" {
		t.Fatalf("truncate = %q, want %q", got, "中")
	}
	if got := truncate("ascii-value", 5); got != "ascii" {
		t.Fatalf("ascii truncate = %q", got)
	}
}
