package system

import (
	"time"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
)

// OperationLogDAO 操作日志数据访问对象
type OperationLogDAO struct{}

// CreateLog 创建操作日志
func (d *OperationLogDAO) CreateLog(log *model.OperationLog) error {
	return database.DB.Create(log).Error
}

// GetLogByID 根据ID获取操作日志
func (d *OperationLogDAO) GetLogByID(id uint) (*model.OperationLog, error) {
	var log model.OperationLog
	result := database.DB.First(&log, id)
	return &log, result.Error
}

// GetLogList 获取操作日志列表（分页 + 条件过滤）
func (d *OperationLogDAO) GetLogList(
	req pagination.PageRequest,
	userID *uint,
	username, actorType, actorID, requestID, method, path, module, action string,
	status *int,
	startTime, endTime *time.Time,
	dataScope authz.UserDataScope,
) ([]model.OperationLog, int64, error) {
	var logs []model.OperationLog
	var total int64

	query := database.DB.Model(&model.OperationLog{})
	query = authz.ApplyOwnerScope(query, dataScope, "user_id")

	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}
	if username != "" {
		query = query.Where("username LIKE ?", "%"+username+"%")
	}
	if actorType != "" {
		query = query.Where("actor_type = ?", actorType)
	}
	if actorID != "" {
		query = query.Where("actor_id LIKE ?", "%"+actorID+"%")
	}
	if requestID != "" {
		query = query.Where("request_id = ?", requestID)
	}
	if method != "" {
		query = query.Where("method = ?", method)
	}
	if path != "" {
		query = query.Where("path LIKE ?", "%"+path+"%")
	}
	if module != "" {
		query = query.Where("module = ?", module)
	}
	if action != "" {
		query = query.Where("action LIKE ?", "%"+action+"%")
	}
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	if startTime != nil {
		query = query.Where("created_at >= ?", *startTime)
	}
	if endTime != nil {
		query = query.Where("created_at <= ?", *endTime)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	result := query.
		Scopes(pagination.Paginate(req)).
		Order("created_at DESC").
		Find(&logs)

	return logs, total, result.Error
}

// DeleteLogsBefore 删除指定时间之前的日志
func (d *OperationLogDAO) DeleteLogsBefore(before time.Time) (int64, error) {
	result := database.DB.Where("created_at < ?", before).Delete(&model.OperationLog{})
	return result.RowsAffected, result.Error
}

// GetLogStats 获取日志统计信息
func (d *OperationLogDAO) GetLogStats(startTime, endTime *time.Time) (*LogStats, error) {
	stats := &LogStats{}

	query := database.DB.Model(&model.OperationLog{})
	if startTime != nil {
		query = query.Where("created_at >= ?", *startTime)
	}
	if endTime != nil {
		query = query.Where("created_at <= ?", *endTime)
	}

	// 总数
	if err := query.Count(&stats.Total).Error; err != nil {
		return nil, err
	}

	// 按模块统计
	var moduleStats []struct {
		Module string `json:"module"`
		Count  int64  `json:"count"`
	}
	if err := database.DB.Model(&model.OperationLog{}).
		Select("module, count(*) as count").
		Where("created_at >= ? AND created_at <= ?", startTime, endTime).
		Group("module").
		Find(&moduleStats).Error; err == nil {
		stats.ByModule = make(map[string]int64)
		for _, s := range moduleStats {
			stats.ByModule[s.Module] = s.Count
		}
	}

	// 按方法统计
	var methodStats []struct {
		Method string `json:"method"`
		Count  int64  `json:"count"`
	}
	if err := database.DB.Model(&model.OperationLog{}).
		Select("method, count(*) as count").
		Where("created_at >= ? AND created_at <= ?", startTime, endTime).
		Group("method").
		Find(&methodStats).Error; err == nil {
		stats.ByMethod = make(map[string]int64)
		for _, s := range methodStats {
			stats.ByMethod[s.Method] = s.Count
		}
	}

	// 错误数量（状态码 >= 400）
	database.DB.Model(&model.OperationLog{}).
		Where("created_at >= ? AND created_at <= ? AND status >= 400", startTime, endTime).
		Count(&stats.ErrorCount)

	return stats, nil
}

// LogStats 日志统计信息
type LogStats struct {
	Total      int64            `json:"total"`
	ByModule   map[string]int64 `json:"by_module"`
	ByMethod   map[string]int64 `json:"by_method"`
	ErrorCount int64            `json:"error_count"`
}
