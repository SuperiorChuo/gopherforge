package service

import (
	"context"
	"time"

	"github.com/go-admin-kit/services/ai/internal/dao"
)

// 统计窗口与返回条数的上界。防止前端传个超大 days 把 SQL 拖垮。
const (
	maxHermesUsageDays = 90
	maxHermesUsageRows = 50
)

// HermesUsageService 汇总模型代理的调用元数据。
//
// 这里的一切都只涉及元数据（次数、token、费用、耗时、错误码）——prompt 与
// response 正文从来没有入库，所以也无从统计。
type HermesUsageService struct {
	dao *dao.HermesUsageDAO
	now func() time.Time
}

func NewHermesUsageService(d *dao.HermesUsageDAO, now func() time.Time) *HermesUsageService {
	if now == nil {
		now = time.Now
	}
	return &HermesUsageService{dao: d, now: now}
}

// window 把 days 钳制到合法范围并算出统计区间。
func (s *HermesUsageService) window(days int) (time.Time, time.Time) {
	if days < 1 {
		days = 7
	}
	if days > maxHermesUsageDays {
		days = maxHermesUsageDays
	}
	now := s.now()
	to := now.Add(time.Minute) // 容一点时钟抖动，避免刚写入的记录落在窗口外
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).
		AddDate(0, 0, -(days - 1))
	return from, to
}

type HermesUsageOverview struct {
	Days    int                       `json:"days"`
	Summary *dao.HermesUsageSummary   `json:"summary"`
	Daily   []dao.HermesUsageDaily    `json:"daily"`
	Devices []dao.HermesUsageByDevice `json:"devices"`
}

func (s *HermesUsageService) Overview(ctx context.Context, days, topN int) (*HermesUsageOverview, error) {
	from, to := s.window(days)
	if topN < 1 {
		topN = 10
	}
	if topN > maxHermesUsageRows {
		topN = maxHermesUsageRows
	}

	summary, err := s.dao.Summary(ctx, from, to)
	if err != nil {
		return nil, err
	}
	daily, err := s.dao.Daily(ctx, from, to)
	if err != nil {
		return nil, err
	}
	devices, err := s.dao.ByDevice(ctx, from, to, topN)
	if err != nil {
		return nil, err
	}
	return &HermesUsageOverview{
		Days:    days,
		Summary: summary,
		Daily:   daily,
		Devices: devices,
	}, nil
}

type HermesDeviceUsage struct {
	Summary  *dao.HermesUsageSummary `json:"summary"`
	Failures []HermesUsageFailure    `json:"failures"`
}

// HermesUsageFailure 只暴露排障需要的字段，不含任何请求正文。
type HermesUsageFailure struct {
	ModelName string    `json:"model_name"`
	Status    string    `json:"status"`
	ErrorMsg  string    `json:"error_msg"`
	LatencyMs int       `json:"latency_ms"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *HermesUsageService) DeviceUsage(ctx context.Context, deviceID string, days int) (*HermesDeviceUsage, error) {
	from, to := s.window(days)
	summary, err := s.dao.DeviceSummary(ctx, deviceID, from, to)
	if err != nil {
		return nil, err
	}
	rows, err := s.dao.RecentFailures(ctx, deviceID, from, to, 10)
	if err != nil {
		return nil, err
	}
	failures := make([]HermesUsageFailure, 0, len(rows))
	for _, r := range rows {
		latency := 0
		if r.LatencyMs != nil {
			latency = *r.LatencyMs
		}
		failures = append(failures, HermesUsageFailure{
			ModelName: r.ModelName,
			Status:    r.Status,
			ErrorMsg:  r.ErrorMsg,
			LatencyMs: latency,
			CreatedAt: r.CreatedAt,
		})
	}
	return &HermesDeviceUsage{Summary: summary, Failures: failures}, nil
}

// HermesMyUsage 是员工个人用量视图（M6）。userID 永远来自会话。
type HermesMyUsage struct {
	Days    int                     `json:"days"`
	Summary *dao.HermesUsageSummary `json:"summary"`
	Daily   []dao.HermesUsageDaily  `json:"daily"`
}

// MyUsage 返回员工本人的调用统计与按日趋势。
func (s *HermesUsageService) MyUsage(ctx context.Context, userID uint, days int) (*HermesMyUsage, error) {
	from, to := s.window(days)
	summary, err := s.dao.UserSummary(ctx, userID, from, to)
	if err != nil {
		return nil, err
	}
	daily, err := s.dao.UserDaily(ctx, userID, from, to)
	if err != nil {
		return nil, err
	}
	if days < 1 {
		days = 7
	}
	if days > maxHermesUsageDays {
		days = maxHermesUsageDays
	}
	return &HermesMyUsage{Days: days, Summary: summary, Daily: daily}, nil
}
