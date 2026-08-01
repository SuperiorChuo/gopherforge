package callback

// 终态回调单次投递用例：成功 / 失败 / 未注册 biz_type / 鉴权与租户头。

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func fastDispatcher(target, token string) *Dispatcher {
	return New(map[string]string{"demo_expense": target}, token)
}

func payload() Payload {
	return Payload{
		InstanceID: 1, DefinitionKey: "demo_expense_approval",
		BizType: "demo_expense", BizID: "42", Result: "approved",
		FormSnapshot: json.RawMessage(`{"amount_cents":100}`),
		FinishedAt:   time.Now().Format(time.RFC3339),
	}
}

func TestDeliverSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := fastDispatcher(srv.URL, "tok")
	if err := d.Deliver(1, payload()); err != nil {
		t.Fatalf("deliver: %v", err)
	}
}

func TestDeliverFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	d := fastDispatcher(srv.URL, "tok")
	if err := d.Deliver(1, payload()); err == nil {
		t.Fatal("HTTP 失败应返回错误")
	}
}

func TestUnregisteredBizReturnsSentinel(t *testing.T) {
	d := fastDispatcher("http://127.0.0.1", "tok")
	p := payload()
	p.BizType = "unknown_biz"
	if err := d.Deliver(1, p); !errors.Is(err, ErrTargetNotRegistered) {
		t.Fatalf("未注册应返回 sentinel: %v", err)
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
	if err := d.Deliver(7, payload()); err != nil {
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
