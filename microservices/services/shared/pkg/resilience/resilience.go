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

// ErrTransient 标记「不可重试但属瞬时故障」的错误（如超时）：不重试（超时歧义，
// 写类重试会重复写入），但计入熔断——服务变慢/挂住也应打开熔断，防级联。
var ErrTransient = errors.New("resilience: transient, not retried")

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
// fn 返回错误时的分类：
//   - errors.Is(err, ErrNoRetry)：永久/业务错误，不重试、不计熔断；
//   - errors.Is(err, ErrTransient)：瞬时但超时歧义（不重试防重复写），计入熔断；
//   - 其余错误视为可重试，按 MaxRetries 重试并计入熔断；
//   - 退避期间 ctx 取消则立即返回 ctx.Err()。
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
		// 除 ErrNoRetry（业务/参数错误，不代表服务健康）外全部计入熔断——
		// 超时/瞬时故障也应打熔断，防「服务变慢但健康」场景级联。
		if cb != nil && !errors.Is(err, ErrNoRetry) {
			cb.Failure()
		}
		if !shouldRetry || attempt >= maxRetries {
			break
		}
		// 指数退避 + 抖动，封顶防溢出（1<<attempt 大次数会归零）。
		delay := baseDelay * time.Duration(1<<attempt)
		if delay > maxBackoff {
			delay = maxBackoff
		}
		if delay <= 0 {
			delay = maxBackoff
		}
		jitter := time.Duration(0)
		if delay >= 4 {
			jitter = time.Duration(rand.Int63n(int64(delay / 4)))
		}
		timer := time.NewTimer(delay + jitter)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		}
	}
	return lastErr
}

// maxBackoff 退避封顶：指数增长过大时不再增长，防溢出与无限等待。
const maxBackoff = 10 * time.Second

// DoResult 与 Do 相同，但返回 fn 的 typed 结果（重试成功后取最后一次成功值）。
// 方法自 Go 1.27 起可声明自己的类型参数，使结果型调用与其 Options 配置自然绑定。
func (opts Options) DoResult[T any](ctx context.Context, fn func(ctx context.Context) (T, error)) (T, error) {
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

// DoResult 保留 package function 兼容入口；新代码优先使用 Options.DoResult。
func DoResult[T any](ctx context.Context, opts Options, fn func(ctx context.Context) (T, error)) (T, error) {
	return opts.DoResult(ctx, fn)
}

// callOnce 执行一次，区分可重试错误（shouldRetry=true）。
// 规则：ErrNoRetry（永久）与 ErrTransient（超时歧义）均不重试；其余视为可重试。
func callOnce(ctx context.Context, fn func(ctx context.Context) error) (bool, error) {
	err := fn(ctx)
	if err == nil {
		return false, nil
	}
	if errors.Is(err, ErrNoRetry) || errors.Is(err, ErrTransient) {
		return false, err
	}
	return true, err
}
