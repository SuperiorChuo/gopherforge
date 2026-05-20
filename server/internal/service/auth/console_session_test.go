package auth

import (
	"context"
	"errors"
	"testing"
)

func TestHashSummaryMatchesConsoleSessionRules(t *testing.T) {
	if got := hashSummary("   "); got != "" {
		t.Fatalf("blank hash = %q, want empty", got)
	}

	got := hashSummary("127.0.0.1")
	if len(got) != 64 {
		t.Fatalf("hash length = %d, want 64", len(got))
	}
	if got != hashSummary(" 127.0.0.1 ") {
		t.Fatal("hash should trim whitespace before hashing")
	}
}

func TestTruncateRunesPreservesRuneBoundaries(t *testing.T) {
	got := truncateRunes("abc\u4e16\u754c", 4)
	if got != "abc\u4e16" {
		t.Fatalf("truncateRunes = %q, want abc\\u4e16", got)
	}
}

func TestConsoleSessionServiceValidateActiveSessionContextHonorsCanceledContext(t *testing.T) {
	setupAuthServiceContextTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (ConsoleSessionService{}).ValidateActiveSessionContext(ctx, "session-1", "alice")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ValidateActiveSessionContext() error = %v, want context.Canceled", err)
	}
}
