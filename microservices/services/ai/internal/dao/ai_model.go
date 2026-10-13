package dao

import (
	"context"
	"errors"
	"fmt"
	"time"

	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/fullread"
	model "github.com/go-admin-kit/services/shared/pkg/model"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
)

// isPlatformAdminContext 读取认证中间件写入的平台管理员标志（middleware/auth.go）。
func isPlatformAdminContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(tenant.PlatformAdminContextKey).(bool)
	return v
}

// scopeUsageLogsToTenant 行级多租户隔离：非平台管理员一律强制过滤到当前租户。
// 平台管理员保留全平台视角（与 usage 统计接口的管理端语义一致）——同时显式
// DisableScope，避免 GORM 租户兜底插件按 ctx 租户误加过滤。
func scopeUsageLogsToTenant(ctx context.Context, q *gorm.DB) *gorm.DB {
	if isPlatformAdminContext(ctx) {
		return q.WithContext(tenant.DisableScope(ctx))
	}
	return q.Where("tenant_id = ?", uint64(tenant.FromContextOrDefault(ctx)))
}

// AIModelDAO 模型配置数据访问
type AIModelDAO struct {
	db *gorm.DB
}

func NewAIModelDAO(db *gorm.DB) *AIModelDAO {
	return &AIModelDAO{db: db}
}

// List 分页查询模型列表
func (d *AIModelDAO) List(ctx context.Context, page, pageSize int, provider string, status *int) ([]*localmodel.AIModel, int64, error) {
	var models []*localmodel.AIModel
	var total int64

	q := d.db.WithContext(ctx).Model(&localmodel.AIModel{})
	if provider != "" {
		q = q.Where("provider = ?", provider)
	}
	if status != nil {
		q = q.Where("status = ?", *status)
	}

	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err := q.Order("sort ASC, id DESC").Offset(offset).Limit(pageSize).Find(&models).Error
	return models, total, err
}

// GetByID 根据ID查询模型
func (d *AIModelDAO) GetByID(ctx context.Context, id uint64) (*localmodel.AIModel, error) {
	var m localmodel.AIModel
	err := d.db.WithContext(ctx).First(&m, id).Error
	return &m, err
}

// GetByName 根据名称查询模型
func (d *AIModelDAO) GetByName(ctx context.Context, name string) (*localmodel.AIModel, error) {
	var m localmodel.AIModel
	err := d.db.WithContext(ctx).Where("name = ?", name).First(&m).Error
	return &m, err
}

// Create 创建模型
func (d *AIModelDAO) Create(ctx context.Context, m *localmodel.AIModel) error {
	return d.db.WithContext(ctx).Create(m).Error
}

// Update 更新模型
func (d *AIModelDAO) Update(ctx context.Context, m *localmodel.AIModel) error {
	return d.db.WithContext(ctx).Save(m).Error
}

// Delete 删除模型
func (d *AIModelDAO) Delete(ctx context.Context, id uint64) error {
	return d.db.WithContext(ctx).Delete(&localmodel.AIModel{}, id).Error
}

// ListEnabled 查询所有启用的模型（用于下拉选择）
func (d *AIModelDAO) ListEnabled(ctx context.Context) ([]*localmodel.AIModel, error) {
	return fullread.All[*localmodel.AIModel](d.db.WithContext(ctx).
		Where("status = ?", localmodel.AIModelStatusEnabled).Order("sort ASC"))
}

// AIUsageLogDAO 调用记录数据访问
type AIUsageLogDAO struct {
	db *gorm.DB
}

func NewAIUsageLogDAO(db *gorm.DB) *AIUsageLogDAO {
	return &AIUsageLogDAO{db: db}
}

// Create 创建调用记录
func (d *AIUsageLogDAO) Create(ctx context.Context, log *localmodel.AIUsageLog) error {
	return d.db.WithContext(ctx).Create(log).Error
}

// List 分页查询调用记录
func (d *AIUsageLogDAO) List(ctx context.Context, page, pageSize int, modelID, userID *uint64, status string, startTime, endTime *time.Time) ([]*localmodel.AIUsageLog, int64, error) {
	var logs []*localmodel.AIUsageLog
	var total int64

	q := scopeUsageLogsToTenant(ctx, d.db.WithContext(ctx).Model(&localmodel.AIUsageLog{}))
	if modelID != nil {
		q = q.Where("model_id = ?", *modelID)
	}
	if userID != nil {
		q = q.Where("user_id = ?", *userID)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if startTime != nil {
		q = q.Where("created_at >= ?", *startTime)
	}
	if endTime != nil {
		q = q.Where("created_at <= ?", *endTime)
	}

	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err := q.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&logs).Error
	return logs, total, err
}

// GetDailyStats 获取每日统计（用于趋势图）
func (d *AIUsageLogDAO) GetDailyStats(ctx context.Context, startDate, endDate time.Time, userID, tenantID *uint64) ([]DailyStats, error) {
	type result struct {
		Date            string  `gorm:"column:date"`
		TotalRequests   int     `gorm:"column:total_requests"`
		SuccessRequests int     `gorm:"column:success_requests"`
		TotalTokens     int     `gorm:"column:total_tokens"`
		TotalCost       float64 `gorm:"column:total_cost"`
		AvgLatencyMs    float64 `gorm:"column:avg_latency_ms"`
	}

	var results []result
	q := scopeUsageLogsToTenant(ctx, d.db.WithContext(ctx).Model(&localmodel.AIUsageLog{})).
		Select(`DATE(created_at) as date,
				COUNT(*) as total_requests,
				SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as success_requests,
				SUM(COALESCE(total_tokens, 0)) as total_tokens,
				SUM(cost) as total_cost,
				AVG(COALESCE(latency_ms, 0)) as avg_latency_ms`).
		Where("created_at >= ? AND created_at < ?", startDate, endDate).
		Group("DATE(created_at)").
		Order("date ASC")

	if userID != nil {
		q = q.Where("user_id = ?", *userID)
	}
	if tenantID != nil {
		q = q.Where("tenant_id = ?", *tenantID)
	}

	if err := q.Scan(&results).Error; err != nil {
		return nil, err
	}

	stats := make([]DailyStats, len(results))
	for i, r := range results {
		stats[i] = DailyStats{
			Date:            r.Date,
			TotalRequests:   r.TotalRequests,
			SuccessRequests: r.SuccessRequests,
			TotalTokens:     r.TotalTokens,
			TotalCost:       r.TotalCost,
			AvgLatencyMs:    r.AvgLatencyMs,
		}
	}
	return stats, nil
}

// GetTopUsers 获取Top用户排行（用于报表）
func (d *AIUsageLogDAO) GetTopUsers(ctx context.Context, startTime, endTime time.Time, limit int) ([]UserStats, error) {
	type result struct {
		UserID        uint64  `gorm:"column:user_id"`
		Username      string  `gorm:"column:username"`
		TotalRequests int     `gorm:"column:total_requests"`
		TotalTokens   int     `gorm:"column:total_tokens"`
		TotalCost     float64 `gorm:"column:total_cost"`
	}

	var results []result
	err := scopeUsageLogsToTenant(ctx, d.db.WithContext(ctx).Model(&localmodel.AIUsageLog{})).
		Select(`user_id, username,
				COUNT(*) as total_requests,
				SUM(COALESCE(total_tokens, 0)) as total_tokens,
				SUM(cost) as total_cost`).
		Where("created_at >= ? AND created_at < ? AND user_id IS NOT NULL", startTime, endTime).
		Group("user_id, username").
		Order("total_cost DESC").
		Limit(limit).
		Scan(&results).Error

	if err != nil {
		return nil, err
	}

	stats := make([]UserStats, len(results))
	for i, r := range results {
		stats[i] = UserStats{
			UserID:        r.UserID,
			Username:      r.Username,
			TotalRequests: r.TotalRequests,
			TotalTokens:   r.TotalTokens,
			TotalCost:     r.TotalCost,
		}
	}
	return stats, nil
}

// GetUsageSummary 获取用量汇总（指定时间范围）
func (d *AIUsageLogDAO) GetUsageSummary(ctx context.Context, startTime, endTime time.Time, userID, tenantID *uint64) (*UsageSummary, error) {
	var summary UsageSummary
	q := scopeUsageLogsToTenant(ctx, d.db.WithContext(ctx).Model(&localmodel.AIUsageLog{})).
		Select(`COUNT(*) as total_requests,
				SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as success_requests,
				SUM(COALESCE(total_tokens, 0)) as total_tokens,
				SUM(cost) as total_cost`).
		Where("created_at >= ? AND created_at < ?", startTime, endTime)

	if userID != nil {
		q = q.Where("user_id = ?", *userID)
	}
	if tenantID != nil {
		q = q.Where("tenant_id = ?", *tenantID)
	}

	err := q.Scan(&summary).Error
	return &summary, err
}

// DailyStats 每日统计
type DailyStats struct {
	Date            string  `json:"date"`
	TotalRequests   int     `json:"total_requests"`
	SuccessRequests int     `json:"success_requests"`
	TotalTokens     int     `json:"total_tokens"`
	TotalCost       float64 `json:"total_cost"`
	AvgLatencyMs    float64 `json:"avg_latency_ms"`
}

// UserStats 用户统计
type UserStats struct {
	UserID        uint64  `json:"user_id"`
	Username      string  `json:"username"`
	TotalRequests int     `json:"total_requests"`
	TotalTokens   int     `json:"total_tokens"`
	TotalCost     float64 `json:"total_cost"`
}

// UsageSummary 用量汇总
type UsageSummary struct {
	TotalRequests   int     `json:"total_requests"`
	SuccessRequests int     `json:"success_requests"`
	TotalTokens     int     `json:"total_tokens"`
	TotalCost       float64 `json:"total_cost"`
}

// AIQuotaDAO 配额数据访问
type AIQuotaDAO struct {
	db *gorm.DB
}

func NewAIQuotaDAO(db *gorm.DB) *AIQuotaDAO {
	return &AIQuotaDAO{db: db}
}

// ErrQuotaScopeForbidden 当前租户无权访问该配额记录（跨租户 scope 或 global scope）。
var ErrQuotaScopeForbidden = errors.New("quota scope not accessible for current tenant")

// GetByScope 根据作用域查询配额
func (d *AIQuotaDAO) GetByScope(ctx context.Context, scope string, scopeID *uint64) (*localmodel.AIQuota, error) {
	var q localmodel.AIQuota
	query := d.db.WithContext(ctx).Where("scope = ?", scope)
	if scopeID != nil {
		query = query.Where("scope_id = ?", *scopeID)
	} else {
		query = query.Where("scope_id IS NULL")
	}
	err := query.First(&q).Error
	return &q, err
}

// authorizeQuotaWrite 校验当前请求方对该配额记录的写权限：
// 平台管理员放行；普通用户只能写本租户的 tenant 配额与本租户用户的 user 配额，global 一律拒绝。
func (d *AIQuotaDAO) authorizeQuotaWrite(ctx context.Context, q *localmodel.AIQuota) error {
	if isPlatformAdminContext(ctx) {
		return nil
	}
	tid := uint64(tenant.FromContextOrDefault(ctx))
	switch q.Scope {
	case localmodel.AIQuotaScopeGlobal:
		return ErrQuotaScopeForbidden
	case localmodel.AIQuotaScopeTenant:
		if q.ScopeID == nil || *q.ScopeID != tid {
			return ErrQuotaScopeForbidden
		}
		return nil
	case localmodel.AIQuotaScopeUser:
		if q.ScopeID == nil {
			return ErrQuotaScopeForbidden
		}
		var count int64
		if err := d.db.WithContext(ctx).Model(&model.User{}).
			Where("id = ? AND tenant_id = ?", *q.ScopeID, tid).
			Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return ErrQuotaScopeForbidden
		}
		return nil
	default:
		return fmt.Errorf("invalid quota scope: %s", q.Scope)
	}
}

// Upsert 创建或更新配额（按 scope+scope_id 定位，写入前强制归属校验）。
// 非平台管理员：tenant 配额的 scope_id 强制归到当前租户，user 配额校验用户归属，global 仅平台管理员可写。
func (d *AIQuotaDAO) Upsert(ctx context.Context, q *localmodel.AIQuota) error {
	if !isPlatformAdminContext(ctx) && q.Scope == localmodel.AIQuotaScopeTenant {
		tid := uint64(tenant.FromContextOrDefault(ctx))
		q.ScopeID = &tid
	}
	if err := d.authorizeQuotaWrite(ctx, q); err != nil {
		return err
	}

	existing, err := d.GetByScope(ctx, q.Scope, q.ScopeID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err == nil {
		q.ID = existing.ID
		q.CreatedAt = existing.CreatedAt
	} else {
		q.ID = 0
	}
	return d.db.WithContext(ctx).Save(q).Error
}

// List 查询配额列表（非平台管理员只可见：本租户 tenant 配额、本租户用户的 user 配额与 global 配额）
func (d *AIQuotaDAO) List(ctx context.Context, scope string) ([]*localmodel.AIQuota, error) {
	query := d.db.WithContext(ctx).Model(&localmodel.AIQuota{})
	if scope != "" {
		query = query.Where("scope = ?", scope)
	}
	if !isPlatformAdminContext(ctx) {
		tid := uint64(tenant.FromContextOrDefault(ctx))
		query = query.Where(
			"(scope = ? AND scope_id = ?) OR (scope = ? AND scope_id IN (?)) OR scope = ?",
			localmodel.AIQuotaScopeTenant, tid,
			localmodel.AIQuotaScopeUser, d.db.Model(&model.User{}).Select("id").Where("tenant_id = ?", tid),
			localmodel.AIQuotaScopeGlobal,
		)
	}
	return fullread.All[*localmodel.AIQuota](query.Order("scope ASC, scope_id ASC"))
}

// Delete 删除配额（先按 id 取出记录做归属校验，再删除）
func (d *AIQuotaDAO) Delete(ctx context.Context, id uint64) error {
	var q localmodel.AIQuota
	if err := d.db.WithContext(ctx).First(&q, id).Error; err != nil {
		return err
	}
	if err := d.authorizeQuotaWrite(ctx, &q); err != nil {
		return err
	}
	return d.db.WithContext(ctx).Delete(&localmodel.AIQuota{}, id).Error
}
