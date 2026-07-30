package cache

import (
	"strings"
	"testing"
	"time"
)

// Redis key names surface in SCAN, MONITOR and the slowlog, so the raw session
// token must never appear in one.
func TestConsoleSessionKeyHashesTheSessionToken(t *testing.T) {
	const token = "test-session-token-fixture"
	key, ok := consoleSessionKey(token)
	if !ok {
		t.Fatal("consoleSessionKey() returned not-ok for a non-empty token")
	}
	if strings.Contains(key, token) {
		t.Fatalf("key %q leaks the raw session token", key)
	}
	if !strings.HasPrefix(key, "console:session:") {
		t.Fatalf("key %q lost its namespace prefix", key)
	}

	again, _ := consoleSessionKey(token)
	if again != key {
		t.Fatal("consoleSessionKey() is not deterministic")
	}
	other, _ := consoleSessionKey(token + "x")
	if other == key {
		t.Fatal("distinct tokens produced the same key")
	}
	if _, ok := consoleSessionKey("   "); ok {
		t.Fatal("blank session id must not produce a key")
	}
}

func TestConsoleSessionExpireHonorsEnvironmentOverride(t *testing.T) {
	t.Setenv(EnvConsoleSessionCacheTTL, "45")
	if got := ConsoleSessionExpire(); got != 45*time.Second {
		t.Fatalf("ConsoleSessionExpire() = %v, want 45s", got)
	}
	if !ConsoleSessionCacheEnabled() {
		t.Fatal("cache should be enabled for a positive TTL")
	}
}

// 0 turns the cache off entirely, restoring per-request validation. That is the
// escape hatch if the short window is ever judged too wide.
func TestConsoleSessionCacheDisabledAtZeroTTL(t *testing.T) {
	t.Setenv(EnvConsoleSessionCacheTTL, "0")
	if ConsoleSessionCacheEnabled() {
		t.Fatal("cache must be disabled at TTL 0")
	}
	if _, ok := NewCacheService().GetConsoleSessionContext(t.Context(), "session-1"); ok {
		t.Fatal("a disabled cache must never report a hit")
	}
}

func TestConsoleSessionExpireRejectsGarbageAndCaps(t *testing.T) {
	t.Setenv(EnvConsoleSessionCacheTTL, "not-a-number")
	if got := ConsoleSessionExpire(); got != ConsoleSessionDefaultExpire {
		t.Fatalf("ConsoleSessionExpire() = %v, want the default for garbage input", got)
	}

	t.Setenv(EnvConsoleSessionCacheTTL, "-5")
	if got := ConsoleSessionExpire(); got != ConsoleSessionDefaultExpire {
		t.Fatalf("ConsoleSessionExpire() = %v, want the default for a negative TTL", got)
	}

	t.Setenv(EnvConsoleSessionCacheTTL, "86400")
	if got := ConsoleSessionExpire(); got != consoleSessionMaxExpire {
		t.Fatalf("ConsoleSessionExpire() = %v, want the cap %v", got, consoleSessionMaxExpire)
	}
}

// NewCacheService is called on the hot auth path; it must not allocate a new
// value per request.
func TestNewCacheServiceReturnsSharedInstance(t *testing.T) {
	first := NewCacheService()
	second := NewCacheService()
	if first != second {
		t.Fatal("NewCacheService() allocated a new instance per call")
	}
}
