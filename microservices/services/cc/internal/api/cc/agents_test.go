package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/cc/internal/store/cc"
)

func doAgentReq(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	} else {
		reader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Auth-User-ID", "1") // 网关注入头
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestAgentsRequireAuth(t *testing.T) {
	_, r := newTestServer(t, "")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cc/agents", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestBindAgentLifecycle(t *testing.T) {
	_, r := newTestServer(t, "")

	// 绑定
	w := doAgentReq(r, http.MethodPut, "/api/v1/cc/agents/9", `{"extension":"1001","display_name":"小王"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("bind: status = %d body=%s", w.Code, w.Body.String())
	}

	// 同用户改绑新分机 → 更新而非新增
	w = doAgentReq(r, http.MethodPut, "/api/v1/cc/agents/9", `{"extension":"1002"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("rebind: status = %d body=%s", w.Code, w.Body.String())
	}

	// 其他用户抢占已占用分机 → 409
	w = doAgentReq(r, http.MethodPut, "/api/v1/cc/agents/10", `{"extension":"1002"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("conflict: status = %d, want 409; body=%s", w.Code, w.Body.String())
	}

	// 列表只有一条（user 9 → 1002）
	w = doAgentReq(r, http.MethodGet, "/api/v1/cc/agents", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list: status = %d", w.Code)
	}
	var listResp struct {
		Data struct {
			List  []store.Agent `json:"list"`
			Total int           `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if listResp.Data.Total != 1 || listResp.Data.List[0].Extension != "1002" || listResp.Data.List[0].UserID != 9 {
		t.Fatalf("list = %+v, want single binding user 9 → 1002", listResp.Data)
	}

	// 解绑
	if w = doAgentReq(r, http.MethodDelete, "/api/v1/cc/agents/9", ""); w.Code != http.StatusOK {
		t.Fatalf("unbind: status = %d body=%s", w.Code, w.Body.String())
	}
	if w = doAgentReq(r, http.MethodDelete, "/api/v1/cc/agents/9", ""); w.Code != http.StatusNotFound {
		t.Fatalf("unbind twice: status = %d, want 404", w.Code)
	}
}

func TestBindAgentValidatesExtension(t *testing.T) {
	_, r := newTestServer(t, "")
	for _, body := range []string{
		`{"extension":""}`,
		`{"extension":"a"}`,
		`{"extension":"has space"}`,
		`{"extension":"` + strings.Repeat("9", 33) + `"}`,
	} {
		if w := doAgentReq(r, http.MethodPut, "/api/v1/cc/agents/9", body); w.Code != http.StatusBadRequest {
			t.Fatalf("body %s: status = %d, want 400", body, w.Code)
		}
	}
	if w := doAgentReq(r, http.MethodPut, "/api/v1/cc/agents/0", `{"extension":"1001"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("user_id 0: status = %d, want 400", w.Code)
	}
}

func TestWebhookAttributesBoundAgent(t *testing.T) {
	srv, r := newTestServer(t, "")

	// 先绑定 1000 → user 7，再回放话单事件
	if w := doAgentReq(r, http.MethodPut, "/api/v1/cc/agents/7", `{"extension":"1000"}`); w.Code != http.StatusOK {
		t.Fatalf("bind: status = %d", w.Code)
	}
	body := envelope("call.answered", "call-attr", map[string]any{"agent_ext": "1000"})
	if w := postWebhook(r, body, ""); w.Code != http.StatusOK {
		t.Fatalf("webhook: status = %d body=%s", w.Code, w.Body.String())
	}

	list, _, err := srv.Store.ListCalls(nil, store.ListCallsQuery{TenantID: 1})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if list[0].AgentUserID == nil || *list[0].AgentUserID != 7 {
		t.Fatalf("agent_user_id = %v, want 7", list[0].AgentUserID)
	}

	// 未绑定分机不归因
	body = envelope("call.answered", "call-nobind", map[string]any{"agent_ext": "2000"})
	_ = postWebhook(r, body, "")
	list, _, _ = srv.Store.ListCalls(nil, store.ListCallsQuery{TenantID: 1})
	for _, c := range list {
		if c.CallID == "call-nobind" && c.AgentUserID != nil {
			t.Fatalf("unbound ext must not attribute, got %v", *c.AgentUserID)
		}
	}
}
