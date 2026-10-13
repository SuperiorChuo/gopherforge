package api

// 合同审批接入用例：BPM_ENABLE 关闭时零影响 / 提交审批置 approving /
// approving 收口（禁编辑、禁裸改 active）/ 回调回写（approved→active、
// rejected→draft、幂等）/ 回调鉴权。bpm 侧用 httptest mock。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/crm/internal/bpmclient"
	"github.com/go-admin-kit/services/crm/internal/model"
)

func u64s(v uint64) string { return strconv.FormatUint(v, 10) }

// callHdr 自定义头版本（内部端点：不带 X-Auth，只带指定头）。
func callHdr(t *testing.T, r *gin.Engine, method, path string, body any, hdr map[string]string) (*httptest.ResponseRecorder, envelope) {
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

// seedContract 建一份草拟合同（走真实 API，归属用户 9）。
func seedContract(t *testing.T, r *gin.Engine, no string) uint64 {
	t.Helper()
	w, env := call(t, r, http.MethodPost, "/api/v1/crm/customers",
		map[string]any{"name": "测试客户-" + no}, "")
	if w.Code != http.StatusOK {
		t.Fatalf("create customer: %d %s", w.Code, w.Body.String())
	}
	var cust struct {
		ID uint64 `json:"id"`
	}
	decode(t, env.Data, &cust)
	w, env = call(t, r, http.MethodPost, "/api/v1/crm/contracts", map[string]any{
		"no": no, "title": "测试合同" + no, "customer_id": cust.ID, "amount_cents": 120000,
	}, "")
	if w.Code != http.StatusOK {
		t.Fatalf("create contract: %d %s", w.Code, w.Body.String())
	}
	var ct struct {
		ID uint64 `json:"id"`
	}
	decode(t, env.Data, &ct)
	return ct.ID
}

func contractStatus(t *testing.T, r *gin.Engine, id uint64) string {
	t.Helper()
	w, env := call(t, r, http.MethodGet, "/api/v1/crm/contracts/"+u64s(id), nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("get contract: %d", w.Code)
	}
	var data struct {
		Contract model.Contract `json:"contract"`
	}
	decode(t, env.Data, &data)
	return data.Contract.Status
}

// BPM_ENABLE=false（默认）：提交审批/进度端点 503，直改 active 照旧允许——零影响。
func TestBpmDisabledNoImpact(t *testing.T) {
	r := newTestRouter(t)
	id := seedContract(t, r, "HT-D01")

	if w, _ := call(t, r, http.MethodPost, "/api/v1/crm/contracts/"+u64s(id)+"/submit-approval",
		nil, ""); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("关闭时提交审批应 503, got %d", w.Code)
	}
	if w, _ := call(t, r, http.MethodGet, "/api/v1/crm/contracts/"+u64s(id)+"/approval",
		nil, ""); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("关闭时进度查询应 503, got %d", w.Code)
	}
	// 现状行为不变：draft → active 直改允许
	if w, _ := call(t, r, http.MethodPut, "/api/v1/crm/contracts/"+u64s(id),
		map[string]any{"status": "active"}, ""); w.Code != http.StatusOK {
		t.Fatalf("关闭时直改 active 应允许, got %d", w.Code)
	}
	if got := contractStatus(t, r, id); got != "active" {
		t.Fatalf("status = %s", got)
	}
}

// 开启后：提交审批 → bpm mock 收到正确契约体 → 合同 approving →
// approving 禁编辑、重复提交拒绝、裸改 active 拒绝、void 仍允许。
func TestSubmitApprovalAndGuards(t *testing.T) {
	var gotStart struct {
		DefinitionKey string          `json:"definition_key"`
		BizType       string          `json:"biz_type"`
		BizID         string          `json:"biz_id"`
		InitiatorID   uint64          `json:"initiator_id"`
		FormSnapshot  json.RawMessage `json:"form_snapshot"`
	}
	bpmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("X-Internal-Token") != "btok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewDecoder(req.Body).Decode(&gotStart)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 200, "message": "success",
			"data": map[string]any{"instance_id": 77, "status": "running"},
		})
	}))
	defer bpmSrv.Close()

	r, srv := newTestRouterWithServer(t)
	srv.BPMEnable = true
	srv.BPM = bpmclient.New(bpmSrv.URL, "btok")
	srv.ContractFlowKey = "crm_contract_approval"
	id := seedContract(t, r, "HT-001")

	w, env := call(t, r, http.MethodPost, "/api/v1/crm/contracts/"+u64s(id)+"/submit-approval", nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("submit: %d %s", w.Code, w.Body.String())
	}
	var res struct {
		InstanceID     uint64 `json:"instance_id"`
		ContractStatus string `json:"contract_status"`
	}
	decode(t, env.Data, &res)
	if res.InstanceID != 77 || res.ContractStatus != "approving" {
		t.Fatalf("submit result: %+v", res)
	}
	// bpm 收到的契约体
	if gotStart.DefinitionKey != "crm_contract_approval" || gotStart.BizType != "crm_contract" ||
		gotStart.BizID != u64s(id) || gotStart.InitiatorID != 9 {
		t.Fatalf("bpm start body: %+v", gotStart)
	}
	var form struct {
		AmountCents int64  `json:"amount_cents"`
		No          string `json:"no"`
	}
	decode(t, gotStart.FormSnapshot, &form)
	if form.AmountCents != 120000 || form.No != "HT-001" {
		t.Fatalf("form snapshot: %+v", form)
	}
	if got := contractStatus(t, r, id); got != "approving" {
		t.Fatalf("status = %s", got)
	}

	// approving 禁编辑
	if w, _ := call(t, r, http.MethodPut, "/api/v1/crm/contracts/"+u64s(id),
		map[string]any{"title": "改标题"}, ""); w.Code != http.StatusBadRequest {
		t.Fatalf("approving 禁编辑, got %d", w.Code)
	}
	// 重复提交拒绝（非 draft）
	if w, _ := call(t, r, http.MethodPost, "/api/v1/crm/contracts/"+u64s(id)+"/submit-approval",
		nil, ""); w.Code != http.StatusBadRequest {
		t.Fatalf("重复提交应 400, got %d", w.Code)
	}

	// 另一份 draft 合同：开启审批后禁止裸改 active；void 仍允许
	id2 := seedContract(t, r, "HT-002")
	if w, _ := call(t, r, http.MethodPut, "/api/v1/crm/contracts/"+u64s(id2),
		map[string]any{"status": "active"}, ""); w.Code != http.StatusBadRequest {
		t.Fatalf("开启后裸改 active 应 400, got %d", w.Code)
	}
	if w, _ := call(t, r, http.MethodPut, "/api/v1/crm/contracts/"+u64s(id2),
		map[string]any{"status": "void"}, ""); w.Code != http.StatusOK {
		t.Fatalf("void 应允许, got %d", w.Code)
	}
}

// 回调回写：approved → active（补 signed_at）；rejected → draft；幂等；鉴权。
func TestBpmCallback(t *testing.T) {
	r, srv := newTestRouterWithServer(t)
	srv.BPMEnable = true
	srv.InternalToken = "itok"
	id := seedContract(t, r, "HT-101")
	// 直接置 approving（模拟已提交审批）
	if err := srv.Store.MarkContractApproving(id, 1); err != nil {
		t.Fatalf("mark approving: %v", err)
	}

	cbBody := func(result string, bizID uint64) map[string]any {
		return map[string]any{
			"instance_id": 77, "definition_key": "crm_contract_approval",
			"biz_type": "crm_contract", "biz_id": u64s(bizID), "result": result,
		}
	}
	hdr := map[string]string{"X-Internal-Token": "itok", "X-Tenant-ID": "1"}

	// 鉴权：无 token 401
	if w, _ := callHdr(t, r, http.MethodPost, "/api/v1/crm/internal/bpm/callback",
		cbBody("approved", id), nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("无 token 应 401, got %d", w.Code)
	}
	// approved → active + signed_at
	if w, _ := callHdr(t, r, http.MethodPost, "/api/v1/crm/internal/bpm/callback",
		cbBody("approved", id), hdr); w.Code != http.StatusOK {
		t.Fatalf("callback approved: %d", w.Code)
	}
	ct, err := srv.Store.GetContract(id, 1)
	if err != nil || ct.Status != "active" || ct.SignedAt == nil {
		t.Fatalf("approved 回写: err=%v status=%s signed=%v", err, ct.Status, ct.SignedAt)
	}
	// 幂等：重复回调仍 200
	if w, _ := callHdr(t, r, http.MethodPost, "/api/v1/crm/internal/bpm/callback",
		cbBody("approved", id), hdr); w.Code != http.StatusOK {
		t.Fatalf("重复回调应幂等 200, got %d", w.Code)
	}

	// rejected → draft
	id2 := seedContract(t, r, "HT-102")
	if err := srv.Store.MarkContractApproving(id2, 1); err != nil {
		t.Fatalf("mark approving2: %v", err)
	}
	if w, _ := callHdr(t, r, http.MethodPost, "/api/v1/crm/internal/bpm/callback",
		cbBody("rejected", id2), hdr); w.Code != http.StatusOK {
		t.Fatalf("callback rejected: %d", w.Code)
	}
	if got := contractStatus(t, r, id2); got != "draft" {
		t.Fatalf("rejected 应回 draft, got %s", got)
	}
	// 未知 biz_type 拒绝
	if w, _ := callHdr(t, r, http.MethodPost, "/api/v1/crm/internal/bpm/callback",
		map[string]any{"biz_type": "unknown", "biz_id": "1", "result": "approved"},
		hdr); w.Code != http.StatusBadRequest {
		t.Fatalf("未知 biz_type 应 400, got %d", w.Code)
	}
}
