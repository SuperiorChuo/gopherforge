package resilience

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

var boom = errors.New("boom")

// failN 前 n 次失败、之后成功，并统计调用次数。
func failN(calls *int, n int) func(context.Context) error {
	return func(context.Context) error {
		*calls++
		if *calls <= n {
			return boom
		}
		return nil
	}
}

func TestDoSuccessFirstTry(t *testing.T) {
	var calls int
	err := Do(context.Background(), Options{MaxRetries: 2}, failN(&calls, 0))
	if err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("成功首次应只调 1 次，实际 %d", calls)
	}
}

func TestDoRetriesThenSuccess(t *testing.T) {
	var calls int
	err := Do(context.Background(), Options{MaxRetries: 2}, failN(&calls, 2))
	if err != nil {
		t.Fatalf("want nil after retries, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("失败 2 次后成功应调 3 次（初始+2 重试），实际 %d", calls)
	}
}

func TestDoExhaustsRetries(t *testing.T) {
	var calls int
	err := Do(context.Background(), Options{MaxRetries: 2, BaseDelay: time.Millisecond}, failN(&calls, 99))
	if !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("MaxRetries=2 应调 3 次后放弃，实际 %d", calls)
	}
}

func TestMaxRetriesZeroMeansNoRetry(t *testing.T) {
	var calls int
	err := Do(context.Background(), Options{MaxRetries: 0}, failN(&calls, 99))
	if !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("MaxRetries=0 应只调 1 次（不重试），实际 %d", calls)
	}
}

func TestDoCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var calls int
	err := Do(ctx, Options{MaxRetries: 2}, failN(&calls, 1))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if calls != 0 {
		t.Fatalf("ctx 已取消不应调 fn，实际 %d", calls)
	}
}

// 退避期间 ctx 被取消 → 立即返回 ctx.Err()，不继续睡完整退避。
func TestDoBackoffRespectsContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls int
	err := Do(ctx, Options{MaxRetries: 3, BaseDelay: time.Hour}, func(context.Context) error {
		calls++
		if calls == 1 {
			cancel() // 首次尝试期间取消
		}
		return boom
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("取消后不应再重试，实际 %d", calls)
	}
}

func TestDoNoRetrySentinel(t *testing.T) {
	var calls int
	err := Do(context.Background(), Options{MaxRetries: 3}, func(context.Context) error {
		calls++
		return fmt.Errorf("%w: invalid arg", ErrNoRetry)
	})
	if !errors.Is(err, ErrNoRetry) {
		t.Fatalf("want ErrNoRetry, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("ErrNoRetry 不应重试，实际 %d", calls)
	}
}

func TestDoResultReturnsValue(t *testing.T) {
	var calls int
	v, err := DoResult(context.Background(), Options{MaxRetries: 2}, func(context.Context) (string, error) {
		calls++
		if calls <= 1 {
			return "", boom
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	if v != "ok" {
		t.Fatalf("want ok, got %q", v)
	}
}

func TestDoResultErrNoRetry(t *testing.T) {
	var calls int
	_, err := DoResult(context.Background(), Options{MaxRetries: 3}, func(context.Context) (int, error) {
		calls++
		return 0, fmt.Errorf("%w: nope", ErrNoRetry)
	})
	if !errors.Is(err, ErrNoRetry) {
		t.Fatalf("want ErrNoRetry, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("ErrNoRetry 不应重试，实际 %d", calls)
	}
}

func TestBreakerOpensAndCooldown(t *testing.T) {
	b := NewBreaker(2, 40*time.Millisecond)
	if !b.Allow() {
		t.Fatal("初始应放行")
	}
	b.Failure()
	b.Failure()
	if b.Allow() {
		t.Fatal("达阈值应熔断")
	}
	time.Sleep(60 * time.Millisecond)
	if !b.Allow() {
		t.Fatal("熔断窗口过后应 half-open 放行")
	}
}

func TestBreakerSuccessResets(t *testing.T) {
	b := NewBreaker(2, time.Hour)
	b.Failure()
	b.Success()
	if !b.Allow() {
		t.Fatal("成功应重置熔断")
	}
	// 重置后重新累计阈值：失败 1 次不应熔断。
	b.Failure()
	if !b.Allow() {
		t.Fatal("重置后 1 次失败不应熔断")
	}
}

func TestDoBreakerOpenSkipsCall(t *testing.T) {
	b := NewBreaker(2, time.Hour)
	b.Failure()
	b.Failure() // 打开
	var calls int
	err := Do(context.Background(), Options{MaxRetries: 3, CircuitBreaker: b}, failN(&calls, 0))
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("want ErrCircuitOpen, got %v", err)
	}
	if calls != 0 {
		t.Fatalf("熔断打开不应调 fn，实际 %d", calls)
	}
}

func TestDoBreakerRecoversAfterCooldown(t *testing.T) {
	b := NewBreaker(2, 30*time.Millisecond)
	var calls int
	// 两次失败打开熔断。
	_ = Do(context.Background(), Options{MaxRetries: 1, BaseDelay: time.Millisecond, CircuitBreaker: b}, failN(&calls, 2))
	if b.Allow() {
		t.Fatal("期望已熔断")
	}
	time.Sleep(60 * time.Millisecond)
	// 窗口过后：成功调用应放行并重置熔断。
	if err := Do(context.Background(), Options{MaxRetries: 0, CircuitBreaker: b}, failN(&calls, 0)); err != nil {
		t.Fatalf("half-open 成功调用应通过，got %v", err)
	}
	if !b.Allow() {
		t.Fatal("成功调用后熔断应完全关闭")
	}
}

// 永久错误（ErrNoRetry）不计入熔断——业务参数错误不把健康服务的熔断器打穿。
func TestDoBreakerIgnoresPermanentFailures(t *testing.T) {
	b := NewBreaker(2, time.Hour)
	var calls int
	err := Do(context.Background(), Options{MaxRetries: 0, CircuitBreaker: b}, func(context.Context) error {
		calls++
		return fmt.Errorf("%w: bad request", ErrNoRetry)
	})
	if !errors.Is(err, ErrNoRetry) {
		t.Fatalf("want ErrNoRetry, got %v", err)
	}
	if !b.Allow() {
		t.Fatal("永久错误不应打开熔断")
	}
}

// 熔断器并发安全：多个 goroutine 同时 Allow/Failure/Success 不 panic、状态一致。
func TestBreakerConcurrency(t *testing.T) {
	b := NewBreaker(10, time.Hour)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				b.Success()
			} else {
				b.Failure()
			}
			_ = b.Allow()
		}(i)
	}
	wg.Wait()
	if !b.Allow() {
		t.Fatal("10 阈值 + 交错成功不应熔断")
	}
}
