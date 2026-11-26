package consoleauth

import (
	"context"
	"strings"
	"testing"
	"time"

	model "github.com/go-admin-kit/services/shared/pkg/model"
)

func TestHashSummaryAndTruncateRunes(t *testing.T) {
	if got := HashSummary("   "); got != "" {
		t.Fatalf("HashSummary(blank) = %q, want empty", got)
	}
	got := HashSummary("127.0.0.1")
	if len(got) != 64 {
		t.Fatalf("HashSummary length = %d, want 64", len(got))
	}
	if got != HashSummary(" 127.0.0.1 ") {
		t.Fatalf("HashSummary should trim whitespace")
	}

	if tr := TruncateRunes("abc世界", 4); tr != "abc世" {
		t.Fatalf("TruncateRunes = %q, want %q", tr, "abc世")
	}
	if tr := TruncateRunes("short", 10); tr != "short" {
		t.Fatalf("TruncateRunes(short) = %q, want short", tr)
	}
}

func TestConsoleRoleCodesAndBuildConsoleSession(t *testing.T) {
	codes := ConsoleRoleCodes([]model.Role{
		{Code: " viewer "},
		{Code: "admin"},
		{Code: "admin"},
		{Code: "  "},
	})
	if strings.Join(codes, ",") != "admin,viewer" {
		t.Fatalf("ConsoleRoleCodes = %v, want [admin viewer]", codes)
	}

	resp := BuildConsoleSession(context.Background(), &model.User{
		ID:                 42,
		Username:           "alice",
		Nickname:           "  ",
		Avatar:             "https://example.com/a.png",
		MustChangePassword: true,
		Roles:              []model.Role{{Code: "operator"}, {Code: "admin"}},
	}, []string{"users.read"}, "", "")

	if !resp.Authenticated || !resp.AuthEnabled {
		t.Fatalf("expected authenticated session response")
	}
	if resp.User.DisplayName != "alice" {
		t.Fatalf("DisplayName = %q, want alice", resp.User.DisplayName)
	}
	if resp.User.Role != "admin" || len(resp.User.Roles) != 2 {
		t.Fatalf("Role/Roles = %q / %v, want admin / [admin operator]", resp.User.Role, resp.User.Roles)
	}
	if !resp.User.MustChangePassword {
		t.Fatalf("expected MustChangePassword=true")
	}
}

func TestConsoleSessionTouchInterval(t *testing.T) {
	t.Setenv(EnvConsoleSessionTouchInterval, "")
	if got := ConsoleSessionTouchInterval(); got != 60*time.Second {
		t.Fatalf("default interval = %v, want 60s", got)
	}
	t.Setenv(EnvConsoleSessionTouchInterval, "0")
	if got := ConsoleSessionTouchInterval(); got != 0 {
		t.Fatalf("zero interval = %v, want 0", got)
	}
	t.Setenv(EnvConsoleSessionTouchInterval, "-5")
	if got := ConsoleSessionTouchInterval(); got != 60*time.Second {
		t.Fatalf("negative interval fallback = %v, want 60s", got)
	}
	t.Setenv(EnvConsoleSessionTouchInterval, "15")
	if got := ConsoleSessionTouchInterval(); got != 15*time.Second {
		t.Fatalf("custom interval = %v, want 15s", got)
	}
}
