package channel

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCCVoiceSenderDisabledWithoutConfig(t *testing.T) {
	if NewCCVoice("", "tok").Enabled() || NewCCVoice("http://cc", "").Enabled() {
		t.Fatal("empty base or token must disable")
	}
}

func TestCCVoiceSenderPostsInternalEndpoint(t *testing.T) {
	var gotPath, gotToken, gotTenant, gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotToken = r.Header.Get("X-Internal-Token")
		gotTenant = r.Header.Get("X-Tenant-ID")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":200,"data":{"id":1,"status":"pending"}}`))
	}))
	t.Cleanup(ts.Close)
	s := NewCCVoice(ts.URL, "itok")
	s.Client = ts.Client()
	if err := s.Send(context.Background(), "13800138000", Payload{TenantID: 2, Title: "工单超时", Content: "请尽快处理"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if gotPath != "/api/v1/cc/internal/voice-notify" || gotToken != "itok" || gotTenant != "2" {
		t.Fatalf("headers path=%s token=%s tenant=%s", gotPath, gotToken, gotTenant)
	}
	if !strings.Contains(gotBody, "13800138000") || !strings.Contains(gotBody, "请尽快处理") {
		t.Fatalf("body=%s", gotBody)
	}
}
