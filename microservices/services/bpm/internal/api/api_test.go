package api

// HTTP 全链路冒烟：走真实路由表 + sqlite 内存库（sqlite 内存库基架）。
// 覆盖：定义创建/发布 → internal 发起（X-Internal-Token）→ 待办列表 →
// 同意 → 终态 → 终态回调派发到业务方 mock；以及 internal 未配 token 503。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/bpm/internal/callback"
	"github.com/go-admin-kit/services/bpm/internal/engine"
	"github.com/go-admin-kit/services/bpm/internal/store"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var dbSeq atomic.Int64

func newTestServer(t *testing.T, cb *callback.Dispatcher) (*gin.Engine, *Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:bpmapi%d?mode=memory&cache=shared", dbSeq.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("sqlite: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	st, err := store.NewWithDB(db)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	srv := &Server{
		Store:         st,
		Engine:        engine.New(db),
		Secret:        "test-secret-at-least-32-characters!!",
		InternalToken: "itok",
		Callback:      cb,
	}
	r := gin.New()
	srv.RegisterRoutes(r)
	return r, srv
}

type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func call(t *testing.T, r *gin.Engine, method, path string, body any, hdr map[string]string) (*httptest.ResponseRecorder, envelope) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var env envelope
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	return w, env
}

func asUser(id string) map[string]string {
	return map[string]string{"X-Auth-User-ID": id, "X-Auth-Tenant-ID": "1"}
}

func internalHdr() map[string]string {
	return map[string]string{"X-Internal-Token": "itok", "X-Tenant-ID": "1"}
}

// 全链路：定义 → 发布 → internal 发起 → 待办 → 同意 → 终态回调。
func TestHTTPFlowEndToEnd(t *testing.T) {
	// 业务方回调 mock
	var cbCalls atomic.Int64
	var cbBody callback.Payload
	bizSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cbCalls.Add(1)
		_ = json.NewDecoder(r.Body).Decode(&cbBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer bizSrv.Close()
	cb := callback.New(map[string]string{"demo_expense": bizSrv.URL}, "cbtok")
	cb.Delays = []time.Duration{0, 0, 0}

	r, _ := newTestServer(t, cb)

	// 1. 建定义 + 发布（管理员=用户 1）
	tree := map[string]any{
		"version": 1,
		"start": map[string]any{
			"id": "n-start", "name": "发起", "type": "start",
			"next": map[string]any{
				"id": "n-a1", "name": "经理审批", "type": "approval",
				"multiMode": "OR",
				"assignee":  map[string]any{"type": "users", "userIds": []uint64{2}},
			},
		},
	}
	w, env := call(t, r, "POST", "/api/v1/bpm/definitions", map[string]any{
		"key": "demo_expense_approval", "name": "报销审批", "biz_type": "demo_expense",
		"node_tree": tree,
	}, asUser("1"))
	if w.Code != 200 {
		t.Fatalf("create def: %d %s", w.Code, w.Body.String())
	}
	var def struct {
		ID uint64 `json:"id"`
	}
	_ = json.Unmarshal(env.Data, &def)
	if w, _ := call(t, r, "POST", fmt.Sprintf("/api/v1/bpm/definitions/%d/publish", def.ID), nil, asUser("1")); w.Code != 200 {
		t.Fatalf("publish: %d %s", w.Code, w.Body.String())
	}
	// 按 key 取 active
	if w, _ := call(t, r, "GET", "/api/v1/bpm/definitions/keys/demo_expense_approval/active", nil, asUser("1")); w.Code != 200 {
		t.Fatalf("active by key: %d %s", w.Code, w.Body.String())
	}

	// 2. internal 发起（业务方服务端到服务端）
	w, env = call(t, r, "POST", "/api/v1/bpm/internal/instances", map[string]any{
		"definition_key": "demo_expense_approval",
		"title":          "报销审批：测试单据",
		"biz_type":       "demo_expense", "biz_id": "42",
		"form_snapshot": map[string]any{"amount_cents": 120000},
		"initiator_id":  9,
	}, internalHdr())
	if w.Code != 200 {
		t.Fatalf("internal start: %d %s", w.Code, w.Body.String())
	}
	var started struct {
		InstanceID uint64 `json:"instance_id"`
		Status     string `json:"status"`
	}
	_ = json.Unmarshal(env.Data, &started)
	if started.Status != "running" {
		t.Fatalf("started: %+v", started)
	}

	// 3. 审批人 2 的待办
	w, env = call(t, r, "GET", "/api/v1/bpm/tasks/todo", nil, asUser("2"))
	if w.Code != 200 {
		t.Fatalf("todo: %d", w.Code)
	}
	var todo struct {
		List []struct {
			ID            uint64 `json:"id"`
			InstanceTitle string `json:"instance_title"`
		} `json:"list"`
		Total int64 `json:"total"`
	}
	_ = json.Unmarshal(env.Data, &todo)
	if todo.Total != 1 || todo.List[0].InstanceTitle != "报销审批：测试单据" {
		t.Fatalf("todo list: %+v", todo)
	}

	// 4. 任务详情含可用动作
	w, env = call(t, r, "GET", fmt.Sprintf("/api/v1/bpm/tasks/%d", todo.List[0].ID), nil, asUser("2"))
	if w.Code != 200 {
		t.Fatalf("task detail: %d", w.Code)
	}
	var detail struct {
		Actions []string `json:"actions"`
	}
	_ = json.Unmarshal(env.Data, &detail)
	// M2：普通审批任务 = approve/reject/transfer/return_start（无上一审批
	// 节点且未开 allowBackPrev，不含 return_prev）
	wantActions := map[string]bool{"approve": true, "reject": true, "transfer": true, "return_start": true}
	if len(detail.Actions) != len(wantActions) {
		t.Fatalf("actions: %+v", detail.Actions)
	}
	for _, a := range detail.Actions {
		if !wantActions[a] {
			t.Fatalf("未知动作 %s: %+v", a, detail.Actions)
		}
	}

	// 5. 同意 → 终态
	w, env = call(t, r, "POST", fmt.Sprintf("/api/v1/bpm/tasks/%d/approve", todo.List[0].ID),
		map[string]any{"comment": "同意"}, asUser("2"))
	if w.Code != 200 {
		t.Fatalf("approve: %d %s", w.Code, w.Body.String())
	}
	var acted struct {
		InstanceStatus string `json:"instance_status"`
	}
	_ = json.Unmarshal(env.Data, &acted)
	if acted.InstanceStatus != "approved" {
		t.Fatalf("instance_status: %s", acted.InstanceStatus)
	}

	// 6. 终态回调异步派发 → 等待 mock 收到
	deadline := time.Now().Add(2 * time.Second)
	for cbCalls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if cbCalls.Load() == 0 {
		t.Fatal("终态回调未派发")
	}
	if cbBody.BizID != "42" || cbBody.Result != "approved" || cbBody.InstanceID != started.InstanceID {
		t.Fatalf("回调体: %+v", cbBody)
	}

	// 7. 时间线与流转图（发起人 9 可见；无关用户 8 不可见）
	if w, _ := call(t, r, "GET", fmt.Sprintf("/api/v1/bpm/instances/%d/timeline", started.InstanceID), nil, asUser("9")); w.Code != 200 {
		t.Fatalf("timeline: %d", w.Code)
	}
	if w, _ := call(t, r, "GET", fmt.Sprintf("/api/v1/bpm/instances/%d/diagram", started.InstanceID), nil, asUser("9")); w.Code != 200 {
		t.Fatalf("diagram: %d", w.Code)
	}
	if w, _ := call(t, r, "GET", fmt.Sprintf("/api/v1/bpm/instances/%d", started.InstanceID), nil, asUser("8")); w.Code != http.StatusForbidden {
		t.Fatalf("无关用户应 403, got %d", w.Code)
	}
	// by-biz internal 反查
	w, env = call(t, r, "GET", "/api/v1/bpm/internal/instances/by-biz?biz_type=demo_expense&biz_id=42", nil, internalHdr())
	if w.Code != 200 {
		t.Fatalf("by-biz: %d", w.Code)
	}
	var byBiz struct {
		List []json.RawMessage `json:"list"`
	}
	_ = json.Unmarshal(env.Data, &byBiz)
	if len(byBiz.List) != 1 {
		t.Fatalf("by-biz list: %d", len(byBiz.List))
	}
}

// internal 鉴权：token 错 → 401；未配置 token → 503。
func TestInternalAuth(t *testing.T) {
	r, srv := newTestServer(t, nil)
	if w, _ := call(t, r, "POST", "/api/v1/bpm/internal/instances", map[string]any{},
		map[string]string{"X-Internal-Token": "wrong"}); w.Code != http.StatusUnauthorized {
		t.Fatalf("错 token 应 401, got %d", w.Code)
	}
	srv.InternalToken = ""
	if w, _ := call(t, r, "POST", "/api/v1/bpm/internal/instances", map[string]any{}, nil); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("未配 token 应 503, got %d", w.Code)
	}
}
