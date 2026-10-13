package dao

import (
	"context"
	"time"

	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"gorm.io/gorm"
)

// HermesProxyOperation 是模型代理写入 ai_usage_logs 时使用的 operation 值。
// 统计一律按它过滤，避免把控制台自身的 AI 调用混算进 Hermes 用量。
const HermesProxyOperation = "hermes_proxy_chat"

type HermesUsageSummary struct {
	TotalRequests  int     `json:"total_requests"`
	FailedRequests int     `json:"failed_requests"`
	TotalTokens    int     `json:"total_tokens"`
	TotalCost      float64 `json:"total_cost"`
	AvgLatencyMs   float64 `json:"avg_latency_ms"`
	ActiveDevices  int     `json:"active_devices"`
}

type HermesUsageByDevice struct {
	DeviceID       string  `json:"device_id"`
	UserID         uint64  `json:"user_id"`
	TotalRequests  int     `json:"total_requests"`
	FailedRequests int     `json:"failed_requests"`
	TotalTokens    int     `json:"total_tokens"`
	TotalCost      float64 `json:"total_cost"`
	// 用 string 接而非 *time.Time：聚合函数 MAX(created_at) 的返回值在 sqlite
	// 下是字符串，database/sql 扫不进 time.Time。前端只用于展示，不做时间运算。
	LastCallAt string `json:"last_call_at,omitempty"`
}

type HermesUsageDaily struct {
	Day            string  `json:"day"`
	TotalRequests  int     `json:"total_requests"`
	FailedRequests int     `json:"failed_requests"`
	TotalTokens    int     `json:"total_tokens"`
	TotalCost      float64 `json:"total_cost"`
}

type HermesUsageDAO struct {
	db *gorm.DB
}

func NewHermesUsageDAO(db *gorm.DB) *HermesUsageDAO {
	return &HermesUsageDAO{db: db}
}

// base 返回已按租户与 Hermes operation 收窄的查询。所有统计都从它出发，
// 保证不会漏掉租户过滤，也不会把控制台调用算进来。
func (d *HermesUsageDAO) base(ctx context.Context, from, to time.Time) *gorm.DB {
	return scopeUsageLogsToTenant(ctx, d.db.WithContext(ctx).Model(&localmodel.AIUsageLog{})).
		Where("operation = ?", HermesProxyOperation).
		Where("created_at >= ? AND created_at < ?", from, to)
}

func (d *HermesUsageDAO) Summary(ctx context.Context, from, to time.Time) (*HermesUsageSummary, error) {
	var summary HermesUsageSummary
	err := d.base(ctx, from, to).
		Select(`COUNT(*) AS total_requests,
			SUM(CASE WHEN status <> 'success' THEN 1 ELSE 0 END) AS failed_requests,
			SUM(COALESCE(total_tokens, 0)) AS total_tokens,
			COALESCE(SUM(cost), 0) AS total_cost,
			COALESCE(AVG(NULLIF(latency_ms, 0)), 0) AS avg_latency_ms,
			COUNT(DISTINCT NULLIF(device_id, '')) AS active_devices`).
		Scan(&summary).Error
	return &summary, err
}

// ByDevice 按设备聚合，用于找出用量异常的终端。limit 由调用方钳制。
func (d *HermesUsageDAO) ByDevice(ctx context.Context, from, to time.Time, limit int) ([]HermesUsageByDevice, error) {
	rows := make([]HermesUsageByDevice, 0)
	err := d.base(ctx, from, to).
		Select(`device_id,
			COALESCE(MAX(user_id), 0) AS user_id,
			COUNT(*) AS total_requests,
			SUM(CASE WHEN status <> 'success' THEN 1 ELSE 0 END) AS failed_requests,
			SUM(COALESCE(total_tokens, 0)) AS total_tokens,
			COALESCE(SUM(cost), 0) AS total_cost,
			MAX(created_at) AS last_call_at`).
		Where("device_id <> ''").
		Group("device_id").
		Order("total_requests DESC").
		Limit(limit).
		Scan(&rows).Error
	return rows, err
}

// Daily 按天聚合趋势。日期在 SQL 里截断，不拉全量到内存分组。
func (d *HermesUsageDAO) Daily(ctx context.Context, from, to time.Time) ([]HermesUsageDaily, error) {
	rows := make([]HermesUsageDaily, 0)
	// 用 DATE(created_at) 而非 TO_CHAR：前者 PG 与 sqlite（测试用）都支持，
	// 与本仓既有 GetDailyStats 的写法保持一致。
	err := d.base(ctx, from, to).
		Select(`DATE(created_at) AS day,
			COUNT(*) AS total_requests,
			SUM(CASE WHEN status <> 'success' THEN 1 ELSE 0 END) AS failed_requests,
			SUM(COALESCE(total_tokens, 0)) AS total_tokens,
			COALESCE(SUM(cost), 0) AS total_cost`).
		Group("DATE(created_at)").
		Order("day ASC").
		Scan(&rows).Error
	return rows, err
}

// DeviceSummary 是单设备的调用统计，用于设备详情抽屉。
func (d *HermesUsageDAO) DeviceSummary(ctx context.Context, deviceID string, from, to time.Time) (*HermesUsageSummary, error) {
	var summary HermesUsageSummary
	err := d.base(ctx, from, to).
		Where("device_id = ?", deviceID).
		Select(`COUNT(*) AS total_requests,
			SUM(CASE WHEN status <> 'success' THEN 1 ELSE 0 END) AS failed_requests,
			SUM(COALESCE(total_tokens, 0)) AS total_tokens,
			COALESCE(SUM(cost), 0) AS total_cost,
			COALESCE(AVG(NULLIF(latency_ms, 0)), 0) AS avg_latency_ms,
			COUNT(DISTINCT NULLIF(device_id, '')) AS active_devices`).
		Scan(&summary).Error
	return &summary, err
}

// RecentFailures 返回最近的失败调用。只取元数据与错误码，不含任何正文。
func (d *HermesUsageDAO) RecentFailures(ctx context.Context, deviceID string, from, to time.Time, limit int) ([]localmodel.AIUsageLog, error) {
	rows := make([]localmodel.AIUsageLog, 0)
	q := d.base(ctx, from, to).Where("status <> ?", "success")
	if deviceID != "" {
		q = q.Where("device_id = ?", deviceID)
	}
	err := q.Select("id, device_id, user_id, model_name, status, error_msg, latency_ms, created_at").
		Order("created_at DESC").Limit(limit).Scan(&rows).Error
	return rows, err
}

// UserSummary 是单员工的调用统计（M6 员工自助用量视图）。userID 永远来自会话，
// 不接受请求参数指定他人。
func (d *HermesUsageDAO) UserSummary(ctx context.Context, userID uint, from, to time.Time) (*HermesUsageSummary, error) {
	var summary HermesUsageSummary
	err := d.base(ctx, from, to).
		Where("user_id = ?", userID).
		Select(`COUNT(*) AS total_requests,
			SUM(CASE WHEN status <> 'success' THEN 1 ELSE 0 END) AS failed_requests,
			SUM(COALESCE(total_tokens, 0)) AS total_tokens,
			COALESCE(SUM(cost), 0) AS total_cost,
			COALESCE(AVG(NULLIF(latency_ms, 0)), 0) AS avg_latency_ms,
			COUNT(DISTINCT NULLIF(device_id, '')) AS active_devices`).
		Scan(&summary).Error
	return &summary, err
}

// UserDaily 是单员工的按日趋势（M6）。
func (d *HermesUsageDAO) UserDaily(ctx context.Context, userID uint, from, to time.Time) ([]HermesUsageDaily, error) {
	rows := make([]HermesUsageDaily, 0)
	err := d.base(ctx, from, to).
		Where("user_id = ?", userID).
		Select(`DATE(created_at) AS day,
			COUNT(*) AS total_requests,
			SUM(CASE WHEN status <> 'success' THEN 1 ELSE 0 END) AS failed_requests,
			SUM(COALESCE(total_tokens, 0)) AS total_tokens,
			COALESCE(SUM(cost), 0) AS total_cost`).
		Group("DATE(created_at)").
		Order("day ASC").
		Scan(&rows).Error
	return rows, err
}
