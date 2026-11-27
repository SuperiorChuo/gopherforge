package connpool

import "testing"

func TestEnvIntDefaultsAndOverrides(t *testing.T) {
	if DefaultMaxOpenConns != 10 || DefaultMaxIdleConns != 5 {
		t.Fatalf("unexpected defaults: open=%d idle=%d, want 10/5", DefaultMaxOpenConns, DefaultMaxIdleConns)
	}
	t.Setenv("DB_MAX_OPEN_CONNS", "")
	if got := envInt("DB_MAX_OPEN_CONNS", DefaultMaxOpenConns); got != 10 {
		t.Fatalf("empty env = %d, want 10", got)
	}
	t.Setenv("DB_MAX_OPEN_CONNS", "25")
	if got := envInt("DB_MAX_OPEN_CONNS", DefaultMaxOpenConns); got != 25 {
		t.Fatalf("override env = %d, want 25", got)
	}
	t.Setenv("DB_MAX_OPEN_CONNS", "0")
	if got := envInt("DB_MAX_OPEN_CONNS", DefaultMaxOpenConns); got != 10 {
		t.Fatalf("zero env = %d, want fallback 10", got)
	}
}
