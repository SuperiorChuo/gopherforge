// Package resilience 内部调用弹性层（Phase 4）：重试 + 熔断 + 超时。
// 套在 gRPC/HTTP 内部客户端上，防止瞬时故障误报 + 级联故障拖垮全链路。
package resilience

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"time"
)

// ErrNoRetry 标记永久性错误：fn 可返回 fmt.Errorf("%w: ...", ErrNoRetry, err)，
// Do 遇此错误不重试、不计入熔断，原样返回。用于区分「重试无意义的业务/参数错误」
// 与「值得重试的瞬时故障」。
var ErrNoRetry = errors.New("resilience: not retryable")

// ErrCircuitOpen 熔断器打开期间调用被快速失败。
var ErrCircuitOpen = errors.New("resilience: circuit breaker open")

// Breaker 简易熔断器：连续失败达到阈值 → 熔断窗口内快速失败；窗口后 half-open 试放。
type Breaker struct {
	mu          sync.Mutex
	failures    int
	threshold   int
	openedUntil time.Time
	cooldown    time.Duration
}

// NewBreaker 创建熔断器。threshold=连续失败阈值，cooldown=熔断窗口。
func NewBreaker(threshold int, cooldown time.Duration) *Breaker {
	if threshold <= 0 {
		threshold = 5
	}
	if cooldown <= 0 {
		cooldown = 10 * time.Second
	}
	return &Breaker{threshold: threshold, cooldown: cooldown}
}

// Allow 熔断器是否放行本次调用（熔断窗口未过返回 false）。
func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return !time.Now().Before(b.openedUntil)
}

// Success 记录一次成功（重置失败计数并关闭熔断）。
func (b *Breaker) Success() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = 0
	b.openedUntil = time.Time{}
}

// Failure 记录一次失败；达到阈值打开熔断器。
func (b *Breaker) Failure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures++
	if b.failures >= b.threshold {
		b.openedUntil = time.Now().Add(b.cooldown)
		b.failures = 0
	}
}

// Options 弹性调用参数。
type Options struct {
	// MaxRetries 失败重试次数（0=不重试，只调 1 次）。
	MaxRetries int
	// BaseDelay 重试基础退避（指数增长 + 抖动）。
	BaseDelay time.Duration
	// CircuitBreaker 可选熔断器。nil=不熔断。
	CircuitBreaker *Breaker
}

// Do 弹性调用：重试（指数退避 + 抖动）+ 熔断 + ctx 超时。
// fn 返回错误时：若 errors.Is(err, ErrNoRetry) 或熔断器存在且本次为不可重试错误，
// 不重试原样返回；否则按 MaxRetries 重试，退避期间 ctx 取消则立即返回 ctx.Err()。
func Do(ctx context.Context, opts Options, fn func(ctx context.Context) error) error {
	maxRetries := opts.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	baseDelay := opts.BaseDelay
	if baseDelay <= 0 {
		baseDelay = 200 * time.Millisecond
	}
	cb := opts.CircuitBreaker

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if cb != nil && !cb.Allow() {
			return ErrCircuitOpen
		}
		shouldRetry, err := callOnce(ctx, fn)
		if err == nil {
			if cb != nil {
				cb.Success()
			}
			return nil
		}
		lastErr = err
		// 只把值得重试的失败计入熔断；永久错误（ErrNoRetry）不算服务健康问题。
		if cb != nil && shouldRetry {
			cb.Failure()
		}
		if !shouldRetry || attempt >= maxRetries {
			break
		}
		// 指数退避 + 抖动。
		delay := baseDelay * time.Duration(1<<attempt)
		jitter := time.Duration(rand.Int63n(int64(delay / 4)))
		select {
		case <-time.After(delay + jitter):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return lastErr
}

// DoResult 与 Do 相同，但返回 fn 的 typed 结果（重试成功后取最后一次成功值）。
func DoResult[T any](ctx context.Context, opts Options, fn func(ctx context.Context) (T, error)) (T, error) {
	var zero T
	var last T
	err := Do(ctx, opts, func(ctx context.Context) error {
		var e error
		last, e = fn(ctx)
		return e
	})
	if err != nil {
		return zero, err
	}
	return last, nil
}

// callOnce 执行一次，区分可重试错误（shouldRetry=true）。
// 规则：ErrNoRetry（或包装链中的 ErrNoRetry）为永久错误，不重试；其余可重试。
func callOnce(ctx context.Context, fn func(ctx context.Context) error) (bool, error) {
	err := fn(ctx)
	if err == nil {
		return false, nil
	}
	if errors.Is(err, ErrNoRetry) {
		return false, err
	}
	return true, err
}
