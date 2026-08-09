package monitor

import (
	"context"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"

	monitorv1 "github.com/go-admin-kit/services/api/gen/monitor/v1"
)

// MonitorGRPC 实现 monitorv1.MonitorServiceServer（Phase 1 服务发现试点端点）。
type MonitorGRPC struct {
	monitorv1.UnimplementedMonitorServiceServer
}

// NewMonitorGRPC 创建 gRPC 服务实例。
func NewMonitorGRPC() *MonitorGRPC { return &MonitorGRPC{} }

// GetSummary 返回宿主/服务健康摘要，数据源与监控概览一致（gopsutil + 探活）。
func (g *MonitorGRPC) GetSummary(ctx context.Context, _ *monitorv1.GetSummaryRequest) (*monitorv1.GetSummaryResponse, error) {
	resp := &monitorv1.GetSummaryResponse{}

	if v, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(v) > 0 {
		resp.CpuPercent = int32(v[0])
	}
	if m, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		resp.MemUsedMb = int64(m.Used / 1024 / 1024)
		resp.MemTotalMb = int64(m.Total / 1024 / 1024)
	}
	if d, err := disk.UsageWithContext(ctx, "/"); err == nil {
		resp.DiskUsedMb = int64(d.Used / 1024 / 1024)
		resp.DiskTotalMb = int64(d.Total / 1024 / 1024)
	}

	total, up := 0, 0
	for _, t := range defaultHealthTargets() {
		total++
		if probeHealth(ctx, t).OK {
			up++
		}
	}
	resp.ServicesUp = int32(up)
	resp.ServicesTotal = int32(total)
	return resp, nil
}
