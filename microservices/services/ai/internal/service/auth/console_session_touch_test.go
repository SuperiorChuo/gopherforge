package auth

import (
	"testing"
	"time"
)

func TestConsoleSessionTouchIntervalDefaultsToOneMinute(t *testing.T) {
	t.Setenv(envConsoleSessionTouchInterval, "")
	if got := ConsoleSessionTouchInterval(); got != defaultConsoleSessionTouchInterval {
		t.Fatalf("ConsoleSessionTouchInterval() = %v, want %v", got, defaultConsoleSessionTouchInterval)
	}
}

func TestConsoleSessionTouchIntervalHonorsEnvironmentOverride(t *testing.T) {
	t.Setenv(envConsoleSessionTouchInterval, "15")
	if got := ConsoleSessionTouchInterval(); got != 15*time.Second {
		t.Fatalf("ConsoleSessionTouchInterval() = %v, want 15s", got)
	}

	// 0 restores a write per request.
	t.Setenv(envConsoleSessionTouchInterval, "0")
	if got := ConsoleSessionTouchInterval(); got != 0 {
		t.Fatalf("ConsoleSessionTouchInterval() = %v, want 0", got)
	}
}

func TestConsoleSessionTouchIntervalRejectsGarbage(t *testing.T) {
	for _, raw := range []string{"soon", "-30", "1.5"} {
		t.Setenv(envConsoleSessionTouchInterval, raw)
		if got := ConsoleSessionTouchInterval(); got != defaultConsoleSessionTouchInterval {
			t.Fatalf("ConsoleSessionTouchInterval() with %q = %v, want the default", raw, got)
		}
	}
}
