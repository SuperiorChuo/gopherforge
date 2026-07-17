// Package ratelimit is a small in-process token-bucket limiter for the
// public embed endpoints. Single-instance only (like the WS hub); swap for a
// Redis-based limiter when IM goes multi-replica.
package ratelimit

import (
	"sync"
	"time"
)

type bucket struct {
	tokens   float64
	lastFill time.Time
}

// Limiter hands out tokens per key at rate tokens/sec with the given burst.
type Limiter struct {
	mu    sync.Mutex
	rate  float64
	burst float64
	m     map[string]*bucket
	// now is swappable in tests
	now func() time.Time
}

func New(ratePerSec float64, burst int) *Limiter {
	l := &Limiter{
		rate:  ratePerSec,
		burst: float64(burst),
		m:     make(map[string]*bucket),
		now:   time.Now,
	}
	go l.janitor()
	return l
}

// Allow reports whether one event for key may proceed now.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	b, ok := l.m[key]
	if !ok {
		b = &bucket{tokens: l.burst, lastFill: now}
		l.m[key] = b
	}
	b.tokens += now.Sub(b.lastFill).Seconds() * l.rate
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.lastFill = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// janitor drops buckets idle long enough to be full again, bounding memory.
func (l *Limiter) janitor() {
	idle := time.Duration(float64(time.Second) * (l.burst/l.rate + 60))
	for {
		time.Sleep(10 * time.Minute)
		cutoff := l.now().Add(-idle)
		l.mu.Lock()
		for k, b := range l.m {
			if b.lastFill.Before(cutoff) {
				delete(l.m, k)
			}
		}
		l.mu.Unlock()
	}
}
