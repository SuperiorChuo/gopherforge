package grpcx

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Retryable 判断一次 gRPC 内部调用错误是否值得重试（供 resilience.Do 区分）。
// 口径：只重试「请求大概率未到达处理端」或「服务过载」的瞬时故障；
// 业务/参数错误与超时歧义（可能已处理，写类调用重试会重复写入）不重试。
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
	case codes.Unavailable, codes.Unimplemented, codes.Aborted, codes.ResourceExhausted:
		return true
	default:
		// InvalidArgument / NotFound / PermissionDenied / Internal / Unknown 等——重试无益。
		return false
	}
}
