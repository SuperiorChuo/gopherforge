package grpcx

import (
	"context"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Dial 经 Resolver 解析服务名后建立 gRPC 连接。
// 自动带 OTel trace 传播（客户端拦截器），insecure 传输（内网）。
func Dial(ctx context.Context, r *Resolver, serviceName string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	addr, err := r.Resolve(ctx, serviceName)
	if err != nil {
		return nil, err
	}
	dialOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	}
	dialOpts = append(dialOpts, opts...)
	return grpc.DialContext(ctx, addr, dialOpts...)
}
