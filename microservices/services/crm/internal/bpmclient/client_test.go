package bpmclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStartInstanceHTTPFallbackPreservesIdempotencyKey(t *testing.T) {
	const wantKey = "crm-contract-42-attempt-1"
	var gotKey string
	var gotBody StartInput
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Idempotency-Key")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode fallback body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"instance_id":7,"status":"running"}}`))
	}))
	defer server.Close()

	client := &Client{base: server.URL, token: "internal", http: server.Client()}
	result, err := client.StartInstance(context.Background(), 1, StartInput{
		DefinitionKey: "contract", Title: "合同", BizType: "crm_contract", BizID: "42",
		InitiatorID: 1, IdempotencyKey: wantKey,
	})
	if err != nil {
		t.Fatalf("fallback start: %v", err)
	}
	if result.InstanceID != 7 || result.Status != "running" {
		t.Fatalf("fallback result: %+v", result)
	}
	if gotKey != wantKey || gotBody.IdempotencyKey != wantKey {
		t.Fatalf("idempotency key not preserved: header=%q body=%q", gotKey, gotBody.IdempotencyKey)
	}
}
