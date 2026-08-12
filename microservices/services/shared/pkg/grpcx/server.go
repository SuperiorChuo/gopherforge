package grpcx

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/tlsutil"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

const (
	defaultKeepAliveTime    = 30 * time.Second
	defaultKeepAliveTimeout = 10 * time.Second
)

// grpcWriter 用于 gRPC 拦截器日志输出，默认写 stderr，可由调用方覆盖。
var grpcWriter io.Writer = os.Stderr

func SetGRPCLogWriter(w io.Writer) {
	if w != nil {
		grpcWriter = w
	}
}

func NewServer(opts ...grpc.ServerOption) *grpc.Server {
	baseOpts := []grpc.ServerOption{
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(
			recoveryUnaryInterceptor,
			loggingUnaryInterceptor,
		),
		grpc.ChainStreamInterceptor(
			recoveryStreamInterceptor,
		),
	}
	baseOpts = append(baseOpts, opts...)
	srv := grpc.NewServer(baseOpts...)
	reflection.Register(srv)
	return srv
}

func NewServerWithTLS(paths tlsutil.CertPaths) (*grpc.Server, error) {
	if !paths.IsComplete() {
		return NewServer(), nil
	}
	creds, err := tlsutil.LoadServerCredentials(paths)
	if err != nil {
		return nil, err
	}
	return NewServer(grpc.Creds(creds)), nil
}

func NewServerWithEnvTLS() (*grpc.Server, error) {
	return NewServerWithTLS(tlsutil.LoadFromEnv())
}

func DialWithTLS(ctx context.Context, r *Resolver, serviceName string, caPath string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	creds, err := tlsutil.LoadClientCredentials(caPath)
	if err != nil {
		return nil, err
	}
	return Dial(ctx, r, serviceName, append([]grpc.DialOption{grpc.WithTransportCredentials(creds)}, opts...)...)
}

func DialWithEnvTLS(ctx context.Context, r *Resolver, serviceName string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	caPath := os.Getenv("TLS_CA_PATH")
	if caPath == "" {
		return Dial(ctx, r, serviceName, opts...)
	}
	return DialWithTLS(ctx, r, serviceName, caPath, opts...)
}

func Dial(ctx context.Context, r *Resolver, serviceName string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	addr, err := r.Resolve(ctx, serviceName)
	if err != nil {
		return nil, err
	}
	dialOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                defaultKeepAliveTime,
			Timeout:             defaultKeepAliveTimeout,
			PermitWithoutStream: true,
		}),
		grpc.WithDefaultServiceConfig(`{
			"loadBalancingPolicy": "round_robin",
			"retryPolicy": {
				"maxAttempts": 3,
				"initialBackoff": "0.1s",
				"maxBackoff": "2s",
				"backoffMultiplier": 2,
				"retryableStatusCodes": ["UNAVAILABLE", "ABORTED", "RESOURCE_EXHAUSTED"]
			}
		}`),
	}
	dialOpts = append(dialOpts, opts...)
	return grpc.DialContext(ctx, addr, dialOpts...)
}

func recoveryUnaryInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(grpcWriter, "[grpcx] panic recovered: method=%s panic=%v\n", info.FullMethod, r)
			err = status.Errorf(codes.Internal, "internal error")
		}
	}()
	return handler(ctx, req)
}

func recoveryStreamInterceptor(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(grpcWriter, "[grpcx] stream panic recovered: method=%s panic=%v\n", info.FullMethod, r)
			err = status.Errorf(codes.Internal, "internal error")
		}
	}()
	return handler(srv, ss)
}

func loggingUnaryInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	start := time.Now()
	resp, err := handler(ctx, req)
	if latency := time.Since(start); err != nil || latency > time.Second {
		if err != nil {
			fmt.Fprintf(grpcWriter, "[grpcx] call failed: method=%s latency=%v err=%v\n", info.FullMethod, latency, err)
		} else {
			fmt.Fprintf(grpcWriter, "[grpcx] slow call: method=%s latency=%v\n", info.FullMethod, latency)
		}
	}
	return resp, err
}
