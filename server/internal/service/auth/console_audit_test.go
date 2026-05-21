package auth

import "testing"

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
