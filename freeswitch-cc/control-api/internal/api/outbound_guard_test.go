package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/config"
)

func TestGuardOutboundKillsOnForbidden(t *testing.T) {
	var killed string
	var mu sync.Mutex
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":403,"message":"被叫号码在外呼黑名单中"}`))
	}))
	t.Cleanup(ts.Close)
	srv := &Server{
		Cfg: config.Config{RiskURL: ts.URL, WebhookSecret: "s"},
		killChannel: func(uuid string) error {
			mu.Lock()
			killed = uuid
			mu.Unlock()
			return nil
		},
		riskHTTP: ts.Client(),
	}
	srv.GuardOutbound(map[string]string{
		"Event-Name":                "CHANNEL_CREATE",
		"Unique-ID":                 "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		"Channel-Call-UUID":         "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		"Call-Direction":            "outbound",
		"Caller-Username":           "1000",
		"Caller-Destination-Number": "13800138000",
	})
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		k := killed
		mu.Unlock()
		if k != "" {
			if k != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" {
				t.Fatalf("killed=%s", k)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected uuid_kill")
}

func TestGuardOutboundSkipsLocalAndInbound(t *testing.T) {
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ts.Close)
	srv := &Server{Cfg: config.Config{RiskURL: ts.URL}, riskHTTP: ts.Client()}
	base := map[string]string{
		"Event-Name":        "CHANNEL_CREATE",
		"Unique-ID":         "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		"Channel-Call-UUID": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
	}
	local := copyMap(base)
	local["Call-Direction"] = "outbound"
	local["Caller-Destination-Number"] = "1001"
	srv.GuardOutbound(local)

	in := copyMap(base)
	in["Call-Direction"] = "inbound"
	in["Caller-Destination-Number"] = "13800138000"
	srv.GuardOutbound(in)

	checked := copyMap(base)
	checked["Call-Direction"] = "outbound"
	checked["Caller-Destination-Number"] = "13800138000"
	checked["variable_cc_risk_checked"] = "1"
	srv.GuardOutbound(checked)

	time.Sleep(50 * time.Millisecond)
	if called {
		t.Fatal("risk-check must not be called for local/inbound/already-checked")
	}
}

func copyMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
