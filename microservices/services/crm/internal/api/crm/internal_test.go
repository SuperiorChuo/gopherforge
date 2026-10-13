package api

// 内部端点（cc → crm）鉴权与行为：X-Internal-Token 校验、号码识客、挂机写跟进。
// 未配置 InternalToken 时端点 503（拒绝裸奔）；配置后错误 token 401、正确 token 通过。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

const testInternalToken = "internal-secret-token"

// internalCall 带 X-Internal-Token / X-Tenant-ID 头请求内部端点。
func internalCall(t *testing.T, r *gin.Engine, method, path, token, tenant string, body any) (*httptest.ResponseRecorder, envelope) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Internal-Token", token)
	}
	if tenant != "" {
		req.Header.Set("X-Tenant-ID", tenant)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var env envelope
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	return w, env
}

func TestInternalEndpointsAuth(t *testing.T) {
	r, srv := newTestRouterWithServer(t)
	srv.InternalToken = testInternalToken

	// 缺 token → 401
	if w, _ := internalCall(t, r, http.MethodGet, "/api/v1/crm/internal/lookup?phone=13800000000", "", "", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("no token: %d, want 401", w.Code)
	}
	// 错 token → 401
	if w, _ := internalCall(t, r, http.MethodGet, "/api/v1/crm/internal/lookup?phone=13800000000", "wrong", "", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("bad token: %d, want 401", w.Code)
	}
}

func TestInternalEndpointsDisabledWithoutToken(t *testing.T) {
	r, srv := newTestRouterWithServer(t)
	srv.InternalToken = "" // 未配置密钥

	if w, _ := internalCall(t, r, http.MethodGet, "/api/v1/crm/internal/lookup?phone=1", "anything", "", nil); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("disabled: %d, want 503", w.Code)
	}
}

func TestInternalLookupAndFollowUp(t *testing.T) {
	r, srv := newTestRouterWithServer(t)
	srv.InternalToken = testInternalToken

	// 先经普通接口建一个客户（走网关头鉴权）
	_, env := call(t, r, http.MethodPost, "/api/v1/crm/customers",
		map[string]any{"name": "宏图集团", "phone": "13800000000"}, "")
	var cu struct {
		ID uint64 `json:"id"`
	}
	decode(t, env.Data, &cu)
	if cu.ID == 0 {
		t.Fatalf("create customer failed: %s", env.Data)
	}

	// 内部识客：命中客户
	w, env := internalCall(t, r, http.MethodGet, "/api/v1/crm/internal/lookup?phone=13800000000", testInternalToken, "1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("internal lookup: %d %s", w.Code, w.Body.String())
	}
	var lk struct {
		Matched  string `json:"matched"`
		Customer *struct {
			ID   uint64 `json:"id"`
			Name string `json:"name"`
		} `json:"customer"`
	}
	decode(t, env.Data, &lk)
	if lk.Matched != "customer" || lk.Customer == nil || lk.Customer.ID != cu.ID {
		t.Fatalf("lookup matched=%s customer=%v", lk.Matched, lk.Customer)
	}

	// 内部写跟进
	w, _ = internalCall(t, r, http.MethodPost, "/api/v1/crm/internal/followups", testInternalToken, "1",
		map[string]any{"target_type": "customer", "target_id": cu.ID, "content": "[通话] 呼入 · 时长 42 秒 · 坐席分机 1000 · 挂断原因 NORMAL_CLEARING", "by_user_id": 5})
	if w.Code != http.StatusOK {
		t.Fatalf("internal followup: %d %s", w.Code, w.Body.String())
	}

	// 跟进已落库（走普通列表接口校验）
	_, env = call(t, r, http.MethodGet, "/api/v1/crm/followups?target_type=customer&target_id="+fmt.Sprintf("%d", cu.ID), nil, "")
	var fl struct {
		List []struct {
			Content  string `json:"content"`
			ByUserID uint64 `json:"by_user_id"`
		} `json:"list"`
	}
	decode(t, env.Data, &fl)
	if len(fl.List) != 1 || fl.List[0].ByUserID != 5 {
		t.Fatalf("followups = %+v, want 1 with by_user_id=5", fl.List)
	}

	// 跟进目标不存在 → 400
	if w, _ := internalCall(t, r, http.MethodPost, "/api/v1/crm/internal/followups", testInternalToken, "1",
		map[string]any{"target_type": "customer", "target_id": 999999, "content": "x"}); w.Code != http.StatusBadRequest {
		t.Fatalf("missing target: %d, want 400", w.Code)
	}
}
