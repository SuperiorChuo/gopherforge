// 包 ratelimit 是面向公开 embed 端点的小型进程内令牌桶限流器。
// 仅限单实例部署（与 WS hub 相同）；当 IM 扩展到多副本时，
// 换用基于 Redis 的限流器。
package ratelimit

import (
	"sync"
	"time"
)

type bucket struct {
	tokens   float64
	lastFill time.Time
}

// Limiter 按 key 以 tokens/sec 的速率发放令牌，并支持给定的 burst 突发量。
type Limiter struct {
	mu    sync.Mutex
	rate  float64
	burst float64
	m     map[string]*bucket
	// now 可在测试中替换
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

// Allow 报告 key 的一次事件当前是否可放行。
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

// janitor 会清理空闲到足以重新装满的桶，以限制内存占用。
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
