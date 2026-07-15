package auth

import (
	"reflect"
	"testing"
	"time"

	"github.com/go-admin-kit/server/internal/model"
)

func TestConsoleAuditTargetFallsBackForBlankValues(t *testing.T) {
	if got := ConsoleAuditTarget(" alice ", "unknown"); got != "alice" {
		t.Fatalf("ConsoleAuditTarget() = %q, want alice", got)
	}
	if got := ConsoleAuditTarget("   ", "unknown"); got != "unknown" {
		t.Fatalf("ConsoleAuditTarget() blank = %q, want unknown", got)
	}
}

func TestConsoleAuthAuditSummary(t *testing.T) {
	tests := []struct {
		action string
		target string
		want   string
	}{
		{action: "auth.login.success", target: "alice", want: "Console login succeeded for alice"},
		{action: "auth.login.failed", target: "alice", want: "Console login failed for alice"},
		{action: "auth.logout", target: "alice", want: "Console logout for alice"},
		{action: "auth.custom", target: "", want: "Console auth event for unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			if got := ConsoleAuthAuditSummary(tt.action, tt.target); got != tt.want {
				t.Fatalf("ConsoleAuthAuditSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConsoleAuthAttemptSnapshot(t *testing.T) {
	got := ConsoleAuthAttemptSnapshot(
		ConsoleAuthRequestMetadata{
			IP:        "127.0.0.1",
			UserAgent: "unit-agent",
			Origin:    "https://console.example.test",
			Referer:   "https://console.example.test/login",
		},
		" alice ",
		"FAILED",
		"invalid_credentials",
	)
	want := map[string]any{
		"username":   "alice",
		"ip":         "127.0.0.1",
		"user_agent": "unit-agent",
		"origin":     "https://console.example.test",
		"referer":    "https://console.example.test/login",
		"result":     "FAILED",
		"reason":     "invalid_credentials",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ConsoleAuthAttemptSnapshot() = %#v, want %#v", got, want)
	}
}

func TestConsoleLoginSuccessSnapshot(t *testing.T) {
	expiresAt := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)
	got := ConsoleLoginSuccessSnapshot(
		ConsoleAuthRequestMetadata{
			IP:        "127.0.0.1",
			UserAgent: "unit-agent",
		},
		&model.ConsoleSession{
			SessionID: "session-1",
			Username:  "alice",
			ExpiresAt: expiresAt,
		},
		600,
	)

	if got["username"] != "alice" || got["result"] != "SUCCESS" {
		t.Fatalf("login success snapshot identity = %#v, want alice success", got)
	}
	if got["session_id"] != "session-1" || got["expires_at"] != expiresAt || got["ttl_sec"] != 600 {
		t.Fatalf("login success snapshot session fields = %#v", got)
	}
}
