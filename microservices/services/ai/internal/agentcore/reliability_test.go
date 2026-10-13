package agentcore

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/tenant"
)

// 请求的版本是服务端选择的快照；重试必须复用首次提交，不被后续发布/禁用改变。
func TestRunRetrySurvivesDefinitionChanges(t *testing.T) {
	for _, change := range []string{"publish", "disable"} {
		t.Run(change, func(t *testing.T) {
			repository := newTestRepository(t)
			service := NewService(repository, &fakeModel{}, NewToolRegistry())
			ctx, definition := createPublishedAgent(t, service, 71, nil, 4)
			input := CreateRunInput{Input: "same input", IdempotencyKey: "stable-key"}
			first, err := service.CreateRun(ctx, definition.ID, 99, input)
			if err != nil {
				t.Fatal(err)
			}
			if change == "publish" {
				_, err = service.PublishVersion(ctx, definition.ID, 99, PublishVersionInput{Instructions: "new version", ModelID: 7, MaxSteps: 4, TimeoutSeconds: 30})
			} else {
				_, err = service.UpdateDefinition(ctx, definition.ID, UpdateDefinitionInput{Name: definition.Name, Status: DefinitionDisabled})
			}
			if err != nil {
				t.Fatal(err)
			}
			again, err := service.CreateRun(ctx, definition.ID, 99, input)
			if err != nil {
				t.Fatalf("identical retry after %s: %v", change, err)
			}
			if !again.Reused || again.Run.ID != first.Run.ID || again.Run.AgentVersionID != first.Run.AgentVersionID {
				t.Fatalf("retry did not reuse original version: %+v", again)
			}
			if _, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: "changed input", IdempotencyKey: input.IdempotencyKey}); !errors.Is(err, ErrIdempotencyConflict) {
				t.Fatalf("different input: %v", err)
			}
			fresh, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: input.Input, IdempotencyKey: "new-key"})
			if change == "disable" && !errors.Is(err, ErrDefinitionDisabled) {
				t.Fatalf("new disabled run: %v", err)
			}
			if change == "publish" && (err != nil || fresh.Run.AgentVersionID == first.Run.AgentVersionID) {
				t.Fatalf("new run did not use new version: %v", err)
			}
			// 模拟预查为空后，另一个请求先插入旧版本：事务内复核也不能依赖版本hash。
			candidate := *first.Run
			candidate.ID = 0
			candidate.AgentVersionID++
			candidate.RequestSHA256 = "different-server-version-hash"
			reused, hit, err := repository.CreateRun(ctx, candidate)
			if err != nil || !hit || reused.ID != first.Run.ID {
				t.Fatalf("repository race fallback: reused=%v err=%v", hit, err)
			}
		})
	}
}

// 稳定字段复用仍需严格区分租户、调用人和目标定义，不能把相同key当作全局缓存。
func TestRunRetryIdentityBoundaries(t *testing.T) {
	repository := newTestRepository(t)
	service := NewService(repository, &fakeModel{}, NewToolRegistry())
	ctx, definition := createPublishedAgent(t, service, 74, nil, 4)
	input := CreateRunInput{Input: "same", IdempotencyKey: "same-key"}
	first, err := service.CreateRun(ctx, definition.ID, 99, input)
	if err != nil {
		t.Fatal(err)
	}
	otherActor, err := service.CreateRun(ctx, definition.ID, 100, input)
	if err != nil || otherActor.Reused || otherActor.Run.ID == first.Run.ID {
		t.Fatalf("actor isolation: %v", err)
	}
	otherCtx, otherDefinition := createPublishedAgent(t, service, 75, nil, 4)
	otherRun, err := service.CreateRun(otherCtx, otherDefinition.ID, 99, input)
	if err != nil || otherRun.Reused || otherRun.Run.ID == first.Run.ID {
		t.Fatalf("tenant isolation: %v", err)
	}
	if _, err := service.CreateRun(ctx, definition.ID+1000, 99, input); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("definition conflict: %v", err)
	}
	if _, err := service.GetRunDetailsAfter(tenant.WithContext(ctx, 75), first.Run.ID, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign events: %v", err)
	}
	candidate := *first.Run
	candidate.ID = 0
	candidate.Input = "changed"
	// 故意保留旧hash，证明事务内比较不能被伪装成相同摘要的新payload欺骗。
	if _, _, err := repository.CreateRun(ctx, candidate); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("repository payload conflict: %v", err)
	}
}

// 使用真实StartStep/BeginToolCall及租约重认领构造崩溃边界，不能只改started_at盖掉子记录遗漏。
func TestRecoveryTimeoutClosesRunningChildren(t *testing.T) {
	for _, kind := range []string{StepKindModel, StepKindTool} {
		t.Run(kind, func(t *testing.T) {
			repository := newTestRepository(t)
			model := &fakeModel{}
			registry := NewToolRegistry()
			var toolCalls atomic.Int32
			var tools []string
			if kind == StepKindTool {
				registerEchoTool(t, registry, &toolCalls, nil)
				tools = []string{"knowledge_search"}
			}
			service := NewService(repository, model, registry)
			ctx, definition := createPublishedAgent(t, service, 72, tools, 4)
			created, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: "timeout", IdempotencyKey: "timeout"})
			if err != nil {
				t.Fatal(err)
			}
			run, err := repository.ClaimNext(context.Background(), "dead-worker", time.Minute)
			if err != nil || run == nil {
				t.Fatalf("claim: %v", err)
			}
			ordinal := 1
			call := ToolCall{ID: "call-1", Name: "knowledge_search", Arguments: MustJSONValue(map[string]any{})}
			if kind == StepKindTool {
				// 先持久化模型生成的工具请求，模拟真实worker进入工具步骤前的checkpoint。
				modelStep, err := repository.StartStep(ctx, *run, "dead-worker", 1, StepKindModel, "model.complete", map[string]any{})
				if err != nil {
					t.Fatal(err)
				}
				checkpoint, err := run.DecodeCheckpoint()
				if err != nil {
					t.Fatal(err)
				}
				checkpoint.Messages = append(checkpoint.Messages, Message{Role: "assistant", ToolCalls: []ToolCall{call}})
				checkpoint.PendingToolCalls = []ToolCall{call}
				checkpoint.PendingModelStep = 1
				checkpoint.NextOrdinal++
				if err := repository.CompleteModelStep(ctx, *run, "dead-worker", *modelStep, ModelResponse{ToolCalls: []ToolCall{call}}, checkpoint); err != nil {
					t.Fatal(err)
				}
				ordinal = 2
			}
			step, err := repository.StartStep(ctx, *run, "dead-worker", ordinal, kind, "test", map[string]any{})
			if err != nil {
				t.Fatal(err)
			}
			if kind == StepKindTool {
				_, _, err := repository.BeginToolCall(ctx, *run, "dead-worker", *step, call, "call-key", "args-hash")
				if err != nil {
					t.Fatal(err)
				}
			}
			past := time.Now().UTC().Add(-time.Minute)
			if err := repository.DB().WithContext(ctx).Model(&AgentRun{}).Where("tenant_id = ? AND id = ?", run.TenantID, run.ID).Updates(map[string]any{"started_at": past, "lease_expires_at": past}).Error; err != nil {
				t.Fatal(err)
			}
			worker := NewWorker(repository, model, registry, WorkerConfig{ID: "new-worker", LeaseDuration: time.Minute})
			worked, err := worker.RunOnce(context.Background())
			if err != nil || !worked {
				t.Fatalf("recovery: %v", err)
			}
			details, err := service.GetRunDetails(ctx, created.Run.ID)
			if err != nil {
				t.Fatal(err)
			}
			if details.Run.Status != RunFailed || details.Run.ErrorCode != "run_timeout" || model.calls.Load() != 0 || toolCalls.Load() != 0 {
				t.Fatalf("bad terminal: %+v", details.Run)
			}
			if len(details.Steps) != ordinal || details.Steps[ordinal-1].Status != StepFailed || details.Steps[ordinal-1].FinishedAt == nil {
				t.Fatalf("orphan running step: %+v", details.Steps)
			}
			if kind == StepKindTool && (len(details.ToolCalls) != 1 || details.ToolCalls[0].Status != StepFailed || details.ToolCalls[0].FinishedAt == nil) {
				t.Fatalf("orphan running tool: %+v", details.ToolCalls)
			}
			if ordinal == 2 && details.Steps[0].Status != StepSucceeded {
				t.Fatal("completed model step was overwritten")
			}
			counts := map[string]int{}
			for _, event := range details.Events {
				counts[event.EventType]++
			}
			if counts["step.failed"] != 1 || counts["run.failed"] != 1 || (kind == StepKindTool && counts["tool.failed"] != 1) {
				t.Fatalf("missing or duplicate failure events: %v", counts)
			}
			// 旧owner不能再次失败收尾或追加事件。
			if err := repository.FailOwnedRun(ctx, *run, "dead-worker", "stale", "stale", step); !errors.Is(err, ErrLeaseLost) {
				t.Fatalf("old owner: %v", err)
			}
		})
	}
}

// 恢复事件数不受max_steps约束，详情必须独立限制事件加载量。
func TestRunDetailsBoundsRecoveryEvents(t *testing.T) {
	repository := newTestRepository(t)
	service := NewService(repository, &fakeModel{}, NewToolRegistry())
	ctx, definition := createPublishedAgent(t, service, 73, nil, 1)
	created, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: "events", IdempotencyKey: "events"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := repository.ClaimNext(context.Background(), "worker", time.Minute)
	if err != nil || run == nil {
		t.Fatalf("claim: %v", err)
	}
	for i := 0; i < 205; i++ {
		if _, err := repository.StartStep(ctx, *run, "worker", 1, StepKindModel, "model", map[string]any{}); err != nil {
			t.Fatal(err)
		}
	}
	details, err := service.GetRunDetails(ctx, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(details.Events) != 200 || !details.EventsHasMore || details.EventsNextCursor != details.Events[199].ID {
		t.Fatalf("unbounded or incomplete first page: %d events", len(details.Events))
	}
	second, err := service.GetRunDetailsAfter(ctx, created.Run.ID, details.EventsNextCursor)
	if err != nil {
		t.Fatal(err)
	}
	all, _, err := service.ListEventsAfter(ctx, created.Run.ID, 0, 500)
	if err != nil {
		t.Fatal(err)
	}
	joined := append(details.Events, second.Events...)
	if len(joined) != 207 || len(joined) != len(all) || second.EventsHasMore {
		t.Fatalf("lost/duplicate events: %d", len(joined))
	}
	for i := range all {
		if joined[i].ID != all[i].ID {
			t.Fatalf("cursor skipped event %d", all[i].ID)
		}
	}
	empty, err := service.GetRunDetailsAfter(ctx, created.Run.ID, second.EventsNextCursor)
	if err != nil || len(empty.Events) != 0 || empty.EventsHasMore || empty.EventsNextCursor != second.EventsNextCursor {
		t.Fatalf("empty tail: %v", err)
	}
	if _, err := service.GetRunDetailsAfter(ctx, created.Run.ID, maxDatabaseID+1); !errors.Is(err, ErrValidation) {
		t.Fatalf("invalid cursor: %v", err)
	}
}
