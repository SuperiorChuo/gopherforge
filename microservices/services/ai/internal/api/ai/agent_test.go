package ai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/ai/internal/agentcore"
	aisvc "github.com/go-admin-kit/services/ai/internal/service/ai"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
)

type handlerModelPort struct{}

func (handlerModelPort) Validate(context.Context, uint64, bool) error { return nil }
func (handlerModelPort) Complete(context.Context, agentcore.ModelRequest) (agentcore.ModelResponse, error) {
	return agentcore.ModelResponse{Content: "ok"}, nil
}

// agentHandlerTestEnv 让用例既能用服务、也能直接用仓储造事件
// （SSE 跨批续作必须要有 200 条以上的事件才走得到循环的第二轮）。
type agentHandlerTestEnv struct {
	service    *agentcore.Service
	repository *agentcore.Repository
	ctx        context.Context
	definition *agentcore.AgentDefinition
}

func newAgentHandlerTestService(t *testing.T) (*agentcore.Service, context.Context, *agentcore.AgentDefinition) {
	t.Helper()
	env := newAgentHandlerTestEnv(t)
	return env.service, env.ctx, env.definition
}

func newAgentHandlerTestEnv(t *testing.T) agentHandlerTestEnv {
	t.Helper()
	dsn := fmt.Sprintf("file:agent-handler-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := tenant.Register(db); err != nil {
		t.Fatalf("tenant plugin: %v", err)
	}
	repository := agentcore.NewRepository(db)
	if err := repository.AutoMigrateForTest(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	service := agentcore.NewService(repository, handlerModelPort{}, agentcore.NewToolRegistry())
	ctx := tenant.WithContext(context.Background(), 7)
	definition, err := service.CreateDefinition(ctx, 9, agentcore.CreateDefinitionInput{AgentKey: "test_agent", Name: "测试 Agent"})
	if err != nil {
		t.Fatalf("create definition: %v", err)
	}
	if _, err := service.PublishVersion(ctx, definition.ID, 9, agentcore.PublishVersionInput{
		Instructions: "answer", ModelID: 2, MaxSteps: 4, TimeoutSeconds: 30,
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	return agentHandlerTestEnv{service: service, repository: repository, ctx: ctx, definition: definition}
}

func TestBindAgentJSONCapsRequestBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// 必须用**合法**的 JSON 超限：非法 JSON 无论有没有上限都会解码失败，
	// 这样的用例在拿掉 MaxBytesReader 之后依然通过，等于什么都没验证。
	body := `{"name":"` + strings.Repeat("a", maxAgentBodyBytes) + `"}`
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/agents", strings.NewReader(body))
	var input map[string]any
	err := bindAgentJSON(c, &input)
	if err == nil {
		t.Fatal("oversized Agent request body was accepted")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Fatalf("oversized body rejected for an unrelated reason: %v", err)
	}
}

func TestBindAgentJSONAcceptsBodyAtLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// 双侧边界：恰好等于上限的合法 JSON 必须通过，避免"上限"被实现成一律拒绝。
	prefix, suffix := `{"name":"`, `"}`
	body := prefix + strings.Repeat("a", maxAgentBodyBytes-len(prefix)-len(suffix)) + suffix
	if len(body) != maxAgentBodyBytes {
		t.Fatalf("test body = %d bytes, want %d", len(body), maxAgentBodyBytes)
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/agents", strings.NewReader(body))
	var input map[string]any
	if err := bindAgentJSON(c, &input); err != nil {
		t.Fatalf("body at the limit was rejected: %v", err)
	}
}

func TestBindAgentJSONRejectsMultipleValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/agents", strings.NewReader(`{"name":"one"}{"name":"two"}`))
	var input map[string]any
	if err := bindAgentJSON(c, &input); err == nil {
		t.Fatal("multiple JSON values were accepted")
	}
}

func TestListAgentRunsRejectsMalformedAgentFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, ctx, _ := newAgentHandlerTestService(t)
	handler := NewHandler(func(context.Context) StatusInfo { return StatusInfo{} }, aisvc.ChatService{}, aisvc.KnowledgeService{}, aisvc.InsightService{})
	handler.SetAgentService(service)
	router := gin.New()
	router.GET("/runs", func(c *gin.Context) {
		c.Request = c.Request.WithContext(ctx)
		handler.ListAgentRuns(c)
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/runs?agent_id=not-a-number", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s, want 400", recorder.Code, recorder.Body.String())
	}
}

func TestAgentIDsRejectUnsignedDatabaseRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, ctx, _ := newAgentHandlerTestService(t)
	handler := NewHandler(func(context.Context) StatusInfo { return StatusInfo{} }, aisvc.ChatService{}, aisvc.KnowledgeService{}, aisvc.InsightService{})
	handler.SetAgentService(service)
	router := gin.New()
	router.GET("/runs/:id/events", func(c *gin.Context) {
		c.Request = c.Request.WithContext(ctx)
		handler.StreamAgentRunEvents(c)
	})
	router.GET("/runs/:id", func(c *gin.Context) {
		c.Request = c.Request.WithContext(ctx)
		handler.GetAgentRun(c)
	})
	const oversized = "9223372036854775808"
	for _, path := range []string{"/runs/1/events?after_id=" + oversized, "/runs/" + oversized} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "AGENT_INVALID_") {
			t.Fatalf("path %s status=%d body=%s", path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestWriteSSERejectsInjectedEventType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	writeSSE(c, "run.started\ndata: forged", 7, map[string]string{"ok": "true"})
	body := recorder.Body.String()
	if strings.Contains(body, "forged") || !strings.Contains(body, "event: message\n") {
		t.Fatalf("unsafe SSE event type was written: %q", body)
	}
}

type shortSSEWriter struct{}

func (shortSSEWriter) Write(value []byte) (int, error) {
	if len(value) == 0 {
		return 0, nil
	}
	return len(value) - 1, nil
}

func TestWriteSSERejectsShortWrite(t *testing.T) {
	if writeSSEText(shortSSEWriter{}, "event: done\\n\\n") {
		t.Fatal("short SSE write was accepted")
	}
}

func TestStreamAgentRunEventsResumesAfterCursorAndEnds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, ctx, definition := newAgentHandlerTestService(t)
	created, err := service.CreateRun(ctx, definition.ID, 9, agentcore.CreateRunInput{Input: "hello", IdempotencyKey: "sse-1"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := service.CancelRun(ctx, created.Run.ID); err != nil {
		t.Fatalf("cancel queued run: %v", err)
	}
	events, terminal, err := service.ListEventsAfter(ctx, created.Run.ID, 0, 100)
	if err != nil || !terminal || len(events) != 3 {
		t.Fatalf("events=%#v terminal=%v err=%v", events, terminal, err)
	}

	handler := NewHandler(func(context.Context) StatusInfo { return StatusInfo{} }, aisvc.ChatService{}, aisvc.KnowledgeService{}, aisvc.InsightService{})
	handler.SetAgentService(service)
	router := gin.New()
	router.GET("/runs/:id/events", func(c *gin.Context) {
		c.Request = c.Request.WithContext(ctx)
		handler.StreamAgentRunEvents(c)
	})
	request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/runs/%d/events?after_id=%d", created.Run.ID, events[0].ID), nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.HasPrefix(recorder.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("status=%d content-type=%q body=%s", recorder.Code, recorder.Header().Get("Content-Type"), recorder.Body.String())
	}
	body := recorder.Body.String()
	if strings.Contains(body, "run.queued") || !strings.Contains(body, "run.cancel_requested") || !strings.Contains(body, "run.cancelled") || !strings.Contains(body, "event: done") {
		t.Fatalf("resumed SSE body = %s", body)
	}
}

// sseEventIDs 按出现顺序取出 SSE 响应体里的**事件帧** id，用来断言"不多不少、顺序一致"。
// 必须逐帧解析并排除 done 帧：writeSSE 对 id>0 一律写 id 行，done 帧也带 id（等于末条事件的），
// 按行扫描会把它多算成一条事件。保活帧是注释行，天然不含 event: 头。
func sseEventIDs(body string) []uint64 {
	var ids []uint64
	for _, frame := range strings.Split(body, "\n\n") {
		if !strings.Contains(frame, "event: ") || strings.Contains(frame, "event: done") {
			continue
		}
		for _, line := range strings.Split(frame, "\n") {
			if !strings.HasPrefix(line, "id: ") {
				continue
			}
			id, err := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(line, "id: ")), 10, 64)
			if err == nil {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// 终态 run 的事件多于一批（handler 每批限 200 条）时，首批 200 条不满足 done 条件，
// 循环必须继续取下一批并推进游标。两条错误实现都会在这里暴露：只取一批就收尾会丢事件，
// 不推进 afterID 会无限重复同一批。
func TestStreamAgentRunEventsContinuesAcrossBatches(t *testing.T) {
	gin.SetMode(gin.TestMode)
	restore := ssePollInterval
	ssePollInterval = 10 * time.Millisecond
	t.Cleanup(func() { ssePollInterval = restore })

	env := newAgentHandlerTestEnv(t)
	created, err := env.service.CreateRun(env.ctx, env.definition.ID, 9, agentcore.CreateRunInput{Input: "many", IdempotencyKey: "sse-batches"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	run, err := env.repository.ClaimNext(context.Background(), "worker", time.Minute)
	if err != nil || run == nil {
		t.Fatalf("claim run: %v", err)
	}
	for i := 0; i < 199; i++ {
		if _, err := env.repository.StartStep(env.ctx, *run, "worker", 1, agentcore.StepKindModel, "model", map[string]any{}); err != nil {
			t.Fatalf("start step %d: %v", i, err)
		}
	}
	if err := env.repository.FailOwnedRun(env.ctx, *run, "worker", "test_failure", "stop", nil); err != nil {
		t.Fatalf("fail run: %v", err)
	}
	expected, terminal, err := env.service.ListEventsAfter(env.ctx, created.Run.ID, 0, 500)
	if err != nil || !terminal {
		t.Fatalf("terminal=%v err=%v", terminal, err)
	}
	if len(expected) <= 200 {
		t.Fatalf("用例前提不成立：事件只有 %d 条，走不到第二批", len(expected))
	}

	handler := NewHandler(func(context.Context) StatusInfo { return StatusInfo{} }, aisvc.ChatService{}, aisvc.KnowledgeService{}, aisvc.InsightService{})
	handler.SetAgentService(env.service)
	router := gin.New()
	router.GET("/runs/:id/events", func(c *gin.Context) {
		c.Request = c.Request.WithContext(tenant.WithContext(c.Request.Context(), 7))
		handler.StreamAgentRunEvents(c)
	})
	recorder := httptest.NewRecorder()
	// 有界上下文：若游标推进被改坏，循环会永远重复同一批——那必须快速失败（断言序列不一致），
	// 而不是把测试挂到包级超时。
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/runs/%d/events", created.Run.ID), nil).WithContext(ctx))
	body := recorder.Body.String()

	want := make([]uint64, 0, len(expected))
	for _, event := range expected {
		want = append(want, event.ID)
	}
	if got := sseEventIDs(body); !reflect.DeepEqual(got, want) {
		t.Fatalf("事件序列不一致（丢/重/乱序）：got %d 条 want %d 条", len(got), len(want))
	}
	if strings.Count(body, "event: done") != 1 {
		t.Fatalf("done 必须恰好一次: %s", body)
	}
}

// run 未到终态时循环只能等待，空闲期必须发保活注释行；删掉 keepalive 分支的实现会在这里暴露
// ——真实链路里没有它，客户端与网关可能把长时间无输出的连接判为空闲断开。
func TestStreamAgentRunEventsEmitsKeepAliveWhileWaiting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	restoreKeepAlive, restorePoll := sseKeepAliveInterval, ssePollInterval
	sseKeepAliveInterval, ssePollInterval = 20*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { sseKeepAliveInterval, ssePollInterval = restoreKeepAlive, restorePoll })

	env := newAgentHandlerTestEnv(t)
	created, err := env.service.CreateRun(env.ctx, env.definition.ID, 9, agentcore.CreateRunInput{Input: "wait", IdempotencyKey: "sse-keepalive"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	// 不认领：run 停在 queued，非终态，循环拿不到 done 条件。

	handler := NewHandler(func(context.Context) StatusInfo { return StatusInfo{} }, aisvc.ChatService{}, aisvc.KnowledgeService{}, aisvc.InsightService{})
	handler.SetAgentService(env.service)
	router := gin.New()
	router.GET("/runs/:id/events", func(c *gin.Context) {
		c.Request = c.Request.WithContext(tenant.WithContext(c.Request.Context(), 7))
		handler.StreamAgentRunEvents(c)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/runs/%d/events", created.Run.ID), nil).WithContext(ctx))

	body := recorder.Body.String()
	if !strings.Contains(body, ": keepalive") {
		t.Fatalf("等待期间没有保活输出: %s", body)
	}
	if strings.Contains(body, "event: done") {
		t.Fatalf("非终态 run 不得发 done: %s", body)
	}
}

// 轮询间隔三条规则：未认领用长基线、已认领保持实时基线、连续空轮退避封顶。
// 退避把"客户端挂着但 run 不动"的空转压下来，而运行中的 run 一直在出新事件，
// 取到新事件就回基线，实时性契约不受影响。
func TestSSENextPollDelay(t *testing.T) {
	restorePoll, restoreQueued, restoreMax := ssePollInterval, sseQueuedPollInterval, ssePollMaxInterval
	ssePollInterval, sseQueuedPollInterval, ssePollMaxInterval = 500*time.Millisecond, 2*time.Second, 5*time.Second
	t.Cleanup(func() {
		ssePollInterval, sseQueuedPollInterval, ssePollMaxInterval = restorePoll, restoreQueued, restoreMax
	})

	for _, test := range []struct {
		name      string
		claimed   bool
		idlePolls int
		want      time.Duration
	}{
		{"未认领用长基线", false, 0, 2 * time.Second},
		{"已认领保持实时基线", true, 0, 500 * time.Millisecond},
		{"未认领空轮翻倍", false, 1, 4 * time.Second},
		{"未认领退避封顶", false, 5, 5 * time.Second},
		{"已认领空轮翻倍", true, 1, time.Second},
		{"已认领退避封顶", true, 10, 5 * time.Second},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := sseNextPollDelay(test.claimed, test.idlePolls); got != test.want {
				t.Fatalf("delay=%v want=%v", got, test.want)
			}
		})
	}
	// 取到新事件时调用方把手里的 idlePolls 归零，必须立刻回到基线。
	if got := sseNextPollDelay(true, 0); got != 500*time.Millisecond {
		t.Fatalf("新事件后未回基线: %v", got)
	}
}

func TestStreamAgentRunEventsHidesCrossTenantRun(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, ctx, definition := newAgentHandlerTestService(t)
	created, err := service.CreateRun(ctx, definition.ID, 9, agentcore.CreateRunInput{Input: "hello", IdempotencyKey: "sse-cross"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	handler := NewHandler(func(context.Context) StatusInfo { return StatusInfo{} }, aisvc.ChatService{}, aisvc.KnowledgeService{}, aisvc.InsightService{})
	handler.SetAgentService(service)
	router := gin.New()
	router.GET("/runs/:id/events", func(c *gin.Context) {
		c.Request = c.Request.WithContext(tenant.WithContext(context.Background(), 8))
		handler.StreamAgentRunEvents(c)
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/runs/%d/events", created.Run.ID), nil))
	if recorder.Code != http.StatusNotFound || strings.Contains(recorder.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("status=%d content-type=%q body=%s", recorder.Code, recorder.Header().Get("Content-Type"), recorder.Body.String())
	}
}
