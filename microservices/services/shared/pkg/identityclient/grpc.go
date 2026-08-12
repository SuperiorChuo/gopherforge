package identityclient

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"

	"github.com/go-admin-kit/services/shared/pkg/grpcx"
	"github.com/go-admin-kit/services/shared/pkg/resilience"
)

// grpcxConn 包装 gRPC 连接（确保关闭）。
type grpcxConn struct {
	Conn *grpc.ClientConn
}

// grpc 执行一次经 Consul 发现 + gRPC 的调用（泛型，返回 typed 响应）。
// Phase 4：套 resilience——重试（仅瞬时故障）+ 熔断（全端点共享）+ 3s 总超时。
// ctx 来自调用方（Phase 4 收尾：取消传播，替代原 Background）。
func grpcCall[T any](ctx context.Context, c *Client, fn func(ctx context.Context, conn *grpcxConn) (T, error)) (T, error) {
	var zero T
	if c.consulResolver == nil {
		return zero, errNoResolver
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return resilience.DoResult(ctx, resilience.Options{
		MaxRetries:     2,
		BaseDelay:      100 * time.Millisecond,
		CircuitBreaker: c.breaker,
	}, func(ctx context.Context) (T, error) {
		conn, err := c.pool.Get(ctx)
		if err != nil {
			return zero, err
		}
		resp, err := fn(ctx, &grpcxConn{Conn: conn})
		if err == nil {
			return resp, nil
		}
		if grpcx.Retryable(err) {
			c.pool.Invalidate() // 瞬时故障可能是陈旧连接，下次重试重拨
			return zero, err    // 瞬时可重试：Do 重试并计熔断
		}
		if grpcx.IsTransient(err) {
			return zero, fmt.Errorf("%w: %w", resilience.ErrTransient, err) // 超时歧义：不重试但计熔断
		}
		return zero, fmt.Errorf("%w: %w", resilience.ErrNoRetry, err) // 永久错误：不重试不计熔断
	})
}

var errNoResolver = &noResolverError{}

type noResolverError struct{}

func (*noResolverError) Error() string { return "identityclient: no consul resolver" }
