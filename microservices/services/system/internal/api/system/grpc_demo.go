package system

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	monitorv1 "github.com/go-admin-kit/services/api/gen/monitor/v1"
	"github.com/go-admin-kit/services/shared/pkg/grpcx"
)

// GRPCDemoAPI 演示经 Consul 服务发现 + gRPC 调监控服务（Phase 1 链路验证）。
type GRPCDemoAPI struct{}

// NewGRPCDemoAPI 创建演示 API。
func NewGRPCDemoAPI() *GRPCDemoAPI { return &GRPCDemoAPI{} }

// GetMonitorSummary 经 Consul 解析 monitor-service → gRPC 调 GetSummary，
// 返回真实监控摘要（宿主统计 + 服务健康数），验证发现/契约/链路完整。
func (a *GRPCDemoAPI) GetMonitorSummary(c *gin.Context) {
	resolver, err := grpcx.NewResolver("")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	conn, err := grpcx.Dial(ctx, resolver, "monitor-service")
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "monitor-service 不可达: " + err.Error()})
		return
	}
	defer conn.Close()

	resp, err := monitorv1.NewMonitorServiceClient(conn).GetSummary(ctx, &monitorv1.GetSummaryRequest{})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}
