package auth

import "testing"

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
	got := truncateRunes("abc世界", 4)
	if got != "abc世" {
		t.Fatalf("truncateRunes = %q, want abc世", got)
	}
}
