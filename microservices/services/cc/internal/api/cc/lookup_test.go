package api

// 号码识客聚合 + 挂机写 CRM 跟进的挂钩测试。CRM 侧用 httptest.Server 打桩
// （crmclient 走 HTTP），断言：contact 命中 / CRM 客户命中 / 都不命中；以及
// 挂机跟进在 CRM 失败时不影响 CDR 响应、命中时按内容写入。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/go-admin-kit/services/cc/internal/crmclient"
	"github.com/go-admin-kit/services/cc/internal/store/cc"
)

// authGet 打一个带网关鉴权头的 GET，返回解出的 envelope data。
func authGet(t *testing.T, r http.Handler, path string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("X-Auth-User-ID", "1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var env struct {
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	return w, env.Data
}

// mockCRM 打桩 crm 内部端点：lookup 命中给定客户，followups 记数。
type mockCRM struct {
	server         *httptest.Server
	lookupCustomer map[string]any // phone → customer(any)；nil=none
	followCount    atomic.Int64
	failFollow     bool
}

func newMockCRM(t *testing.T) *mockCRM {
	m := &mockCRM{lookupCustomer: map[string]any{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/crm/internal/lookup", func(w http.ResponseWriter, r *http.Request) {
		phone := r.URL.Query().Get("phone")
		out := map[string]any{"matched": "none"}
		if cu, ok := m.lookupCustomer[phone]; ok && cu != nil {
			out = map[string]any{"matched": "customer", "customer": cu}
		}
		writeEnvelope(w, out)
	})
	mux.HandleFunc("/api/v1/crm/internal/followups", func(w http.ResponseWriter, r *http.Request) {
		if m.failFollow {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"code":400,"message":"boom"}`))
			return
		}
		m.followCount.Add(1)
		writeEnvelope(w, map[string]any{"id": 1})
	})
	m.server = httptest.NewServer(mux)
	t.Cleanup(m.server.Close)
	return m
}

func writeEnvelope(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "success", "data": data})
}

func TestLookupAggregates(t *testing.T) {
	srv, r := newTestServer(t, "")

	// contact 备注名（cc_contacts）
	if _, err := srv.Store.UpsertContact(nil, &store.Contact{TenantID: 1, Number: "13800000000", Name: "老王", Remark: "VIP"}); err != nil {
		t.Fatalf("upsert contact: %v", err)
	}
	// 一通历史话单（caller 命中）
	if _, err := srv.Store.ApplyEvent(nil, "cdr.json", &store.Call{
		TenantID: 1, CallID: "c-hist-1", Direction: "inbound",
		Caller: "13800000000", Callee: "1000", AgentExt: "1000", Status: "hangup", DurationSec: 30,
	}); err != nil {
		t.Fatalf("apply call: %v", err)
	}

	// CRM 打桩：13800000000 命中客户
	mock := newMockCRM(t)
	mock.lookupCustomer["13800000000"] = map[string]any{"id": 42, "name": "宏图集团", "phone": "13800000000"}
	srv.CRM = crmclient.New(mock.server.URL, "tkn")

	// 1) contact + CRM 都命中
	w, data := authGet(t, r, "/api/v1/cc/lookup?number=13800000000")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	if data["has_crm"] != true {
		t.Fatalf("has_crm = %v, want true", data["has_crm"])
	}
	contact, _ := data["contact"].(map[string]any)
	if contact == nil || contact["name"] != "老王" {
		t.Fatalf("contact = %v, want name 老王", data["contact"])
	}
	cu, _ := data["crm_customer"].(map[string]any)
	if cu == nil || cu["name"] != "宏图集团" {
		t.Fatalf("crm_customer = %v", data["crm_customer"])
	}
	recent, _ := data["recent_calls"].([]any)
	if len(recent) != 1 {
		t.Fatalf("recent_calls = %d, want 1", len(recent))
	}

	// 2) 号码只有联系人、无 CRM、无历史
	if _, err := srv.Store.UpsertContact(nil, &store.Contact{TenantID: 1, Number: "13900000000", Name: "小李"}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	_, data = authGet(t, r, "/api/v1/cc/lookup?number=13900000000")
	if data["has_crm"] != false {
		t.Fatalf("has_crm = %v, want false", data["has_crm"])
	}
	if contact, _ := data["contact"].(map[string]any); contact == nil || contact["name"] != "小李" {
		t.Fatalf("contact = %v", data["contact"])
	}
	if data["crm_customer"] != nil {
		t.Fatalf("crm_customer = %v, want nil", data["crm_customer"])
	}

	// 3) 完全陌生号码：全空
	_, data = authGet(t, r, "/api/v1/cc/lookup?number=15000000000")
	if data["has_crm"] != false || data["contact"] != nil || data["crm_customer"] != nil {
		t.Fatalf("stranger lookup not empty: %+v", data)
	}
	if recent, _ := data["recent_calls"].([]any); len(recent) != 0 {
		t.Fatalf("stranger recent_calls = %d, want 0", len(recent))
	}
}

func TestLookupRequiresNumberAndAuth(t *testing.T) {
	_, r := newTestServer(t, "")
	// 缺鉴权
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cc/lookup?number=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no auth: status = %d, want 401", w.Code)
	}
	// 缺 number
	w, _ = authGet(t, r, "/api/v1/cc/lookup?number=")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty number: status = %d, want 400", w.Code)
	}
}

// postCDR 直投一条 mod_json_cdr 话单（无 Basic，CDRAuth 空=放行）。
func postCDR(t *testing.T, r http.Handler, vars map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"variables": vars})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cc/cdr", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestCDRWritesCRMFollowOnCustomerHit(t *testing.T) {
	srv, r := newTestServerWithCDR(t)

	mock := newMockCRM(t)
	mock.lookupCustomer["13800000000"] = map[string]any{"id": 7, "name": "宏图", "phone": "13800000000"}
	srv.CRM = crmclient.New(mock.server.URL, "tkn")

	// 呼入，主叫命中 CRM 客户 → 应写一条跟进
	w := postCDR(t, r, map[string]string{
		"uuid": "cdr-1", "direction": "inbound",
		"caller_id_number": "13800000000", "destination_number": "1000",
		"dialed_extension": "1000", "billsec": "42", "hangup_cause": "NORMAL_CLEARING",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("cdr status = %d body=%s", w.Code, w.Body.String())
	}
	if got := mock.followCount.Load(); got != 1 {
		t.Fatalf("followCount = %d, want 1", got)
	}
}

func TestCDRFollowFailureDoesNotBreakResponse(t *testing.T) {
	srv, r := newTestServerWithCDR(t)

	mock := newMockCRM(t)
	mock.lookupCustomer["13800000000"] = map[string]any{"id": 7, "name": "宏图"}
	mock.failFollow = true // 跟进写入必失败
	srv.CRM = crmclient.New(mock.server.URL, "tkn")

	w := postCDR(t, r, map[string]string{
		"uuid": "cdr-2", "direction": "inbound",
		"caller_id_number": "13800000000", "destination_number": "1000", "billsec": "10",
	})
	// 跟进失败也必须 200 + CDR 已入库
	if w.Code != http.StatusOK {
		t.Fatalf("cdr status = %d, want 200 despite follow failure; body=%s", w.Code, w.Body.String())
	}
	_, total, _ := srv.Store.ListCalls(nil, store.ListCallsQuery{TenantID: 1})
	if total != 1 {
		t.Fatalf("call rows = %d, want 1 (CDR must persist regardless)", total)
	}
}

func TestCDRNoCRMWhenNoCustomerMatch(t *testing.T) {
	srv, r := newTestServerWithCDR(t)
	mock := newMockCRM(t) // lookup 全 none
	srv.CRM = crmclient.New(mock.server.URL, "tkn")

	w := postCDR(t, r, map[string]string{
		"uuid": "cdr-3", "direction": "inbound", "caller_id_number": "15000000000", "billsec": "5",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if got := mock.followCount.Load(); got != 0 {
		t.Fatalf("followCount = %d, want 0 (no customer match)", got)
	}
}
