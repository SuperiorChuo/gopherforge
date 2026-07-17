package ratelimit

import (
	"testing"
	"time"
)

func newTestLimiter(ratePerSec float64, burst int) (*Limiter, *time.Time) {
	now := time.Unix(1_700_000_000, 0)
	l := &Limiter{
		rate:  ratePerSec,
		burst: float64(burst),
		m:     make(map[string]*bucket),
		now:   func() time.Time { return now },
	}
	return l, &now
}

func TestBurstThenDeny(t *testing.T) {
	l, _ := newTestLimiter(1, 3)
	for i := 0; i < 3; i++ {
		if !l.Allow("k") {
			t.Fatalf("burst request %d denied", i+1)
		}
	}
	if l.Allow("k") {
		t.Fatal("request beyond burst allowed")
	}
}

func TestRefillOverTime(t *testing.T) {
	l, now := newTestLimiter(1, 2)
	l.Allow("k")
	l.Allow("k")
	if l.Allow("k") {
		t.Fatal("empty bucket allowed")
	}
	*now = now.Add(1500 * time.Millisecond) // 1.5 tokens back
	if !l.Allow("k") {
		t.Fatal("refilled token denied")
	}
	if l.Allow("k") {
		t.Fatal("only one token should have refilled")
	}
}

func TestKeysAreIndependent(t *testing.T) {
	l, _ := newTestLimiter(1, 1)
	if !l.Allow("a") {
		t.Fatal("first key denied")
	}
	if !l.Allow("b") {
		t.Fatal("second key should have its own bucket")
	}
	if l.Allow("a") {
		t.Fatal("key a should be empty")
	}
}

func TestRefillCapsAtBurst(t *testing.T) {
	l, now := newTestLimiter(10, 2)
	l.Allow("k")
	*now = now.Add(time.Hour)
	for i := 0; i < 2; i++ {
		if !l.Allow("k") {
			t.Fatalf("token %d after long idle denied", i+1)
		}
	}
	if l.Allow("k") {
		t.Fatal("bucket exceeded burst cap")
	}
}
