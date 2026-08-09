package grpcx

import (
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// NewServer 创建带 OTel 的 gRPC server。
// StatsHandler 注入 trace 上下文，跨服务链路在 Jaeger 完整可见。
// 注册 reflection 便于 grpcurl 调试契约发现（内网明文）。
func NewServer() *grpc.Server {
	srv := grpc.NewServer(grpc.StatsHandler(otelgrpc.NewServerHandler()))
	reflection.Register(srv)
	return srv
}
