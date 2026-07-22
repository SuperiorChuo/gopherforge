package callback

// 终态回调重试用例：前两次失败第三次成功 / 三次全失败 / 未注册 biz_type
// 跳过 / 鉴权与租户头正确携带。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func fastDispatcher(target, token string) *Dispatcher {
	d := New(map[string]string{"demo_expense": target}, token)
	d.Delays = []time.Duration{0, 0, 0} // 测试免等
	return d
}

func payload() Payload {
	return Payload{
		InstanceID: 1, DefinitionKey: "demo_expense_approval",
		BizType: "demo_expense", BizID: "42", Result: "approved",
		FormSnapshot: json.RawMessage(`{"amount_cents":100}`),
		FinishedAt:   time.Now().Format(time.RFC3339),
	}
}

// 前两次 500，第三次 200：重试后成功，总计三次请求。
func TestRetrySucceedsOnThird(t *testing.T) {
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := fastDispatcher(srv.URL, "tok")
	if err := d.DispatchSync(1, payload()); err != nil {
		t.Fatalf("第三次应成功: %v", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("应请求 3 次, got %d", calls.Load())
	}
}

// 三次全失败：返回最终错误，不再多试。
func TestRetryExhausted(t *testing.T) {
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	d := fastDispatcher(srv.URL, "tok")
	if err := d.DispatchSync(1, payload()); err == nil {
		t.Fatal("三次全失败应返回错误")
	}
	if calls.Load() != 3 {
		t.Fatalf("应请求 3 次, got %d", calls.Load())
	}
}

// 未注册 biz_type：静默跳过，不发请求。
func TestUnregisteredBizSkipped(t *testing.T) {
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
	}))
	defer srv.Close()

	d := fastDispatcher(srv.URL, "tok")
	p := payload()
	p.BizType = "unknown_biz"
	if err := d.DispatchSync(1, p); err != nil {
		t.Fatalf("未注册应静默跳过: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("不应发请求, got %d", calls.Load())
	}
}

// 请求头与体：X-Internal-Token / X-Tenant-ID / 回调体契约字段。
func TestHeadersAndBody(t *testing.T) {
	var gotToken, gotTenant string
	var gotBody Payload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("X-Internal-Token")
		gotTenant = r.Header.Get("X-Tenant-ID")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := fastDispatcher(srv.URL, "sec-token")
	if err := d.DispatchSync(7, payload()); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if gotToken != "sec-token" || gotTenant != "7" {
		t.Fatalf("headers: token=%q tenant=%q", gotToken, gotTenant)
	}
	if gotBody.BizType != "demo_expense" || gotBody.BizID != "42" ||
		gotBody.Result != "approved" || gotBody.InstanceID != 1 {
		t.Fatalf("body: %+v", gotBody)
	}
}
