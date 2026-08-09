package grpcx

import (
	"context"
	"net"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	monitorv1 "github.com/go-admin-kit/services/api/gen/monitor/v1"
)

// mockMonitorService 最小实现，验证 server/client 装配与 OTel 不破坏调用。
type mockMonitorService struct {
	monitorv1.UnimplementedMonitorServiceServer
}

func (m *mockMonitorService) GetSummary(context.Context, *monitorv1.GetSummaryRequest) (*monitorv1.GetSummaryResponse, error) {
	return &monitorv1.GetSummaryResponse{CpuPercent: 42, ServicesUp: 3, ServicesTotal: 5}, nil
}

// TestRoundTrip 经 bufconn 走 grpcx.NewServer + 客户端 StatsHandler 往返，
// 验证 OTel 装配可正常完成一次 gRPC 调用。
func TestRoundTrip(t *testing.T) {
	lis := bufconn.Listen(1024 * 1024)
	srv := NewServer()
	monitorv1.RegisterMonitorServiceServer(srv, &mockMonitorService{})
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	dialer := func(context.Context, string) (net.Conn, error) { return lis.Dial() }
	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	defer conn.Close()

	client := monitorv1.NewMonitorServiceClient(conn)
	resp, err := client.GetSummary(context.Background(), &monitorv1.GetSummaryRequest{})
	if err != nil {
		t.Fatalf("GetSummary: %v", err)
	}
	if resp.CpuPercent != 42 || resp.ServicesTotal != 5 {
		t.Fatalf("意外返回: %+v", resp)
	}
}
