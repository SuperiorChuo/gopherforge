package monitor

import (
	"context"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
)

type ServerService struct{}

func NewServerService() *ServerService {
	return &ServerService{}
}

// GetServerInfo returns server information.
// Deprecated: use GetServerInfoContext instead.
func (s *ServerService) GetServerInfo() (map[string]any, error) {
	return s.GetServerInfoContext(context.Background())
}

func (s *ServerService) GetServerInfoContext(ctx context.Context) (map[string]any, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	data := make(map[string]any)

	// Memory information.
	vMem, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		vMem = &mem.VirtualMemoryStat{}
	}

	data["memory"] = map[string]any{
		"total":        vMem.Total,
		"used":         vMem.Used,
		"free":         vMem.Free,
		"used_percent": vMem.UsedPercent,
	}

	// CPU information.
	cpuInfo, err := cpu.InfoWithContext(ctx)
	var modelName string
	var cores int
	if err == nil && len(cpuInfo) > 0 {
		modelName = cpuInfo[0].ModelName
		cores = len(cpuInfo)
	}

	cpuPercent, err := cpu.PercentWithContext(ctx, 0, false)
	var usedPercent float64
	if err == nil && len(cpuPercent) > 0 {
		usedPercent = cpuPercent[0]
	}

	data["cpu"] = map[string]any{
		"model_name":   modelName,
		"cores":        cores,
		"used_percent": usedPercent,
	}

	// Host information.
	hostInfo, err := host.InfoWithContext(ctx)
	if err != nil {
		hostInfo = &host.InfoStat{}
	}

	data["os"] = map[string]any{
		"go_os":         runtime.GOOS,
		"arch":          runtime.GOARCH,
		"compiler":      runtime.Compiler,
		"go_version":    runtime.Version(),
		"num_goroutine": runtime.NumGoroutine(),
		"hostname":      hostInfo.Hostname,
		"platform":      hostInfo.Platform,
		"boot_time":     time.Unix(int64(hostInfo.BootTime), 0).Format("2006-01-02 15:04:05"),
	}

	// Disk information for the root filesystem.
	diskInfo, err := disk.UsageWithContext(ctx, "/")
	if err != nil {
		diskInfo = &disk.UsageStat{}
	}

	data["disk"] = map[string]any{
		"total":        diskInfo.Total,
		"used":         diskInfo.Used,
		"free":         diskInfo.Free,
		"used_percent": diskInfo.UsedPercent,
	}

	return data, nil
}
