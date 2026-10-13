package crmclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// gRPC 不可用时走 HTTP：必须带同一 X-Idempotency-Key 与 body 字段。
func TestCreateFollowUpHTTPSendsIdempotencyKey(t *testing.T) {
	var gotHeader string
	var gotBody struct {
		IdempotencyKey string `json:"idempotency_key"`
		TargetType     string `json:"target_type"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Idempotency-Key")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
	}))
	defer srv.Close()

	c := &Client{
		base:  srv.URL,
		token: "t",
		http:  srv.Client(),
		// resolver nil → 直走 HTTP
	}
	if err := c.CreateFollowUp(context.Background(), 1, "customer", 9, 3, "hello"); err != nil {
		t.Fatalf("CreateFollowUp: %v", err)
	}
	if gotHeader == "" || gotBody.IdempotencyKey == "" {
		t.Fatalf("missing idempotency key header=%q body=%q", gotHeader, gotBody.IdempotencyKey)
	}
	if gotHeader != gotBody.IdempotencyKey {
		t.Fatalf("header/body key mismatch: %q vs %q", gotHeader, gotBody.IdempotencyKey)
	}
	if gotBody.TargetType != "customer" {
		t.Fatalf("target_type=%q", gotBody.TargetType)
	}
}
