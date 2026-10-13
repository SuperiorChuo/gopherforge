package channel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSystemSMSSendUsesInternalContract(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Token") != "internal-token" {
			t.Fatalf("internal token header missing")
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":"success","data":{"status":"success"}}`))
	}))
	defer server.Close()

	sender := NewSystemSMS(server.URL, "internal-token", server.Client())
	err := sender.Send(context.Background(), "13800000000", Payload{
		TenantID: 7, TemplateCode: "ops_alert", Params: map[string]string{"name": "CPU"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got["tenant_id"] != float64(7) || got["template_code"] != "ops_alert" || got["mobile"] != "13800000000" {
		t.Fatalf("unexpected request body: %#v", got)
	}
}

func TestSystemSMSSendReportsBusinessFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":200,"message":"success","data":{"status":"failure","error":"provider rejected"}}`))
	}))
	defer server.Close()

	err := NewSystemSMS(server.URL, "token", server.Client()).Send(context.Background(), "13800000000", Payload{
		TenantID: 1, TemplateCode: "code",
	})
	if err == nil {
		t.Fatal("expected provider business failure")
	}
}

func TestSystemSMSDisabledWithoutURLOrToken(t *testing.T) {
	if NewSystemSMS("", "token", nil).Enabled() || NewSystemSMS("http://system", "", nil).Enabled() {
		t.Fatal("incomplete system sms configuration must fail closed")
	}
}
