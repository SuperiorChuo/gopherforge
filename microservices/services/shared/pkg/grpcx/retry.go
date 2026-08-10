package grpcx

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Retryable 判断一次 gRPC 内部调用错误是否值得重试（供 resilience.Do 区分）。
// 口径：只重试「请求大概率未到达处理端」或「服务过载」的瞬时故障；
// 业务/参数错误、超时歧义（可能已处理，写类重试会重复写入）、Unimplemented
// （版本错配，重试无益且会把健康服务的熔断器打穿）均不重试。
func Retryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false
	}
	st, ok := status.FromError(err)
	if !ok {
		// 非 gRPC status 错误（Dial/resolve/网络层裸错）——多数为瞬时，重试。
		return true
	}
	switch st.Code() {
	case codes.Unavailable, codes.Aborted, codes.ResourceExhausted:
		return true
	default:
		// InvalidArgument / NotFound / PermissionDenied / Unimplemented / Internal / Unknown 等——重试无益。
		return false
	}
}

// IsTransient 判断错误是否属「瞬时故障但不可重试」：超时（服务变慢/挂住）等。
// 供 resilience.ErrTransient 包装用——计入熔断防级联，但不重试（超时歧义）。
// 与 Retryable 的区别：Unavailable/Aborted/ResourceExhausted 两者都 true（可重试）；
// DeadlineExceeded 只 IsTransient true（不重试但计熔断）；Canceled（调用方取消）、
// Unimplemented（版本错配）两者都 false。
func IsTransient(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		// 调用方取消——非服务故障，不计熔断。
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	st, ok := status.FromError(err)
	if !ok {
		// 非 gRPC status 错误（Dial/resolve/网络层裸错）——瞬时。
		return true
	}
	switch st.Code() {
	case codes.DeadlineExceeded, codes.Unavailable, codes.Aborted, codes.ResourceExhausted:
		return true
	default:
		return false
	}
}
