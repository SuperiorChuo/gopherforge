package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/go-admin-kit/services/cc/internal/store/cc"
)

func TestEarlyMediaClassSurvivesLocalRejectedHangup(t *testing.T) {
	const secret = "secret"
	srv, r := newTestServer(t, secret)
	callID := "11111111-2222-3333-4444-555555555555"
	for _, tc := range []struct {
		event string
		extra map[string]any
	}{
		{"call.ringing", map[string]any{"direction": "outbound", "callee": "13800138000"}},
		{"call.early_media", map[string]any{"class": "empty"}},
		{"call.hangup", map[string]any{"cause": "CALL_REJECTED"}},
	} {
		body := envelope(tc.event, callID, tc.extra)
		if w := postWebhook(r, body, sign(secret, body)); w.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", tc.event, w.Code, w.Body.String())
		}
	}
	got, err := srv.Store.GetCall(context.Background(), 1, callID)
	if err != nil {
		t.Fatal(err)
	}
	if got.HangupClass != "empty" || got.HangupCause != "CALL_REJECTED" {
		t.Fatalf("class=%q cause=%q", got.HangupClass, got.HangupCause)
	}
}

func TestEarlyMediaWebhookRejectsUnknownClass(t *testing.T) {
	const secret = "secret"
	_, r := newTestServer(t, secret)
	body := envelope("call.early_media", "11111111-2222-3333-4444-555555555555", map[string]any{"class": "ok"})
	if w := postWebhook(r, body, sign(secret, body)); w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestVoiceEarlyMediaClassIsNotOverwritten(t *testing.T) {
	const secret = "secret"
	srv, r := newTestServer(t, secret)
	job := &store.CCVoiceNotify{TenantID: 1, To: "13800138000", Text: "x", Status: "queued"}
	if err := srv.Store.CreateVoiceNotify(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	callID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	body := envelope("call.early_media", callID, map[string]any{"class": "unavailable", "voice_id": job.ID})
	if w := postWebhook(r, body, sign(secret, body)); w.Code != http.StatusOK {
		t.Fatalf("early status=%d body=%s", w.Code, w.Body.String())
	}
	if _, err := srv.Store.ApplyVoiceHangup(context.Background(), job.ID, callID, "CALL_REJECTED", 0, false); err != nil {
		t.Fatal(err)
	}
	got, err := srv.Store.GetVoiceNotify(context.Background(), 1, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.HangupClass != "unavailable" {
		t.Fatalf("class=%q", got.HangupClass)
	}
}
