package system

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
)

type OperationLogDAO struct {
	db *gorm.DB
}

func NewOperationLogDAO(db *gorm.DB) *OperationLogDAO {
	return &OperationLogDAO{db: db}
}

func (d *OperationLogDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	if d != nil && d.db != nil {
		return d.db.WithContext(ctx)
	}
	return database.DB.WithContext(ctx)
}

// Deprecated: use CreateLogContext instead.
func (d *OperationLogDAO) CreateLog(log *model.OperationLog) error {
	return d.CreateLogContext(context.Background(), log)
}

func (d *OperationLogDAO) CreateLogContext(ctx context.Context, log *model.OperationLog) error {
	return d.dbWithContext(ctx).Create(log).Error
}

// Deprecated: use GetLogByIDContext instead.
func (d *OperationLogDAO) GetLogByID(id uint) (*model.OperationLog, error) {
	return d.GetLogByIDContext(context.Background(), id)
}

func (d *OperationLogDAO) GetLogByIDContext(ctx context.Context, id uint) (*model.OperationLog, error) {
	var log model.OperationLog
	result := d.dbWithContext(authz.DisableDataScope(ctx)).First(&log, id)
	return &log, result.Error
}

// Deprecated: use GetLogByIDInScopeContext instead.
func (d *OperationLogDAO) GetLogByIDInScope(id uint, dataScope authz.UserDataScope) (*model.OperationLog, error) {
	return d.GetLogByIDInScopeContext(context.Background(), id, dataScope)
}

func (d *OperationLogDAO) GetLogByIDInScopeContext(ctx context.Context, id uint, dataScope authz.UserDataScope) (*model.OperationLog, error) {
	var log model.OperationLog
	query := d.dbWithContext(authz.EnableDataScope(ctx, dataScope)).Model(&model.OperationLog{})
	result := query.Where("id = ?", id).First(&log)
	return &log, result.Error
}

// Deprecated: use GetLogListContext instead.
func (d *OperationLogDAO) GetLogList(
	req pagination.PageRequest,
	userID *uint,
	username, actorType, actorID, requestID, method, path, module, action string,
	status *int,
	startTime, endTime *time.Time,
	dataScope authz.UserDataScope,
) ([]model.OperationLog, int64, error) {
	return d.GetLogListContext(context.Background(), req, userID, username, actorType, actorID, requestID, method, path, module, action, status, startTime, endTime, dataScope)
}

func (d *OperationLogDAO) GetLogListContext(
	ctx context.Context,
	req pagination.PageRequest,
	userID *uint,
	username, actorType, actorID, requestID, method, path, module, action string,
	status *int,
	startTime, endTime *time.Time,
	dataScope authz.UserDataScope,
) ([]model.OperationLog, int64, error) {
	var logs []model.OperationLog
	var total int64

	query := d.dbWithContext(authz.EnableDataScope(ctx, dataScope)).Model(&model.OperationLog{})
	query = applyOperationLogFilters(query, userID, username, actorType, actorID, requestID, method, path, module, action, status, startTime, endTime)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	result := query.
		Scopes(pagination.Paginate(req)).
		Order("created_at DESC").
		Find(&logs)

	return logs, total, result.Error
}

func applyOperationLogFilters(
	query *gorm.DB,
	userID *uint,
	username, actorType, actorID, requestID, method, path, module, action string,
	status *int,
	startTime, endTime *time.Time,
) *gorm.DB {
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
	return applyTimeRange(query, startTime, endTime)
}

// Deprecated: use DeleteLogsBeforeContext instead.
func (d *OperationLogDAO) DeleteLogsBefore(before time.Time) (int64, error) {
	return d.DeleteLogsBeforeContext(context.Background(), before)
}

func (d *OperationLogDAO) DeleteLogsBeforeContext(ctx context.Context, before time.Time) (int64, error) {
	result := d.dbWithContext(ctx).Where("created_at < ?", before).Delete(&model.OperationLog{})
	return result.RowsAffected, result.Error
}

// Deprecated: use GetLogStatsContext instead.
func (d *OperationLogDAO) GetLogStats(startTime, endTime *time.Time) (*LogStats, error) {
	return d.GetLogStatsContext(context.Background(), startTime, endTime)
}

func (d *OperationLogDAO) GetLogStatsContext(ctx context.Context, startTime, endTime *time.Time) (*LogStats, error) {
	return d.getLogStatsContext(authz.DisableDataScope(ctx), startTime, endTime)
}

// Deprecated: use GetLogStatsInScopeContext instead.
func (d *OperationLogDAO) GetLogStatsInScope(startTime, endTime *time.Time, dataScope authz.UserDataScope) (*LogStats, error) {
	return d.GetLogStatsInScopeContext(context.Background(), startTime, endTime, dataScope)
}

func (d *OperationLogDAO) GetLogStatsInScopeContext(ctx context.Context, startTime, endTime *time.Time, dataScope authz.UserDataScope) (*LogStats, error) {
	return d.getLogStatsContext(authz.EnableDataScope(ctx, dataScope), startTime, endTime)
}

func (d *OperationLogDAO) getLogStatsContext(ctx context.Context, startTime, endTime *time.Time) (*LogStats, error) {
	stats := &LogStats{ByModule: map[string]int64{}, ByMethod: map[string]int64{}}

	query := applyTimeRange(d.dbWithContext(ctx).Model(&model.OperationLog{}), startTime, endTime)
	if err := query.Count(&stats.Total).Error; err != nil {
		return nil, err
	}

	var moduleStats []struct {
		Module string `json:"module"`
		Count  int64  `json:"count"`
	}
	if err := applyTimeRange(d.dbWithContext(ctx).Model(&model.OperationLog{}), startTime, endTime).
		Select("module, count(*) as count").
		Group("module").
		Find(&moduleStats).Error; err != nil {
		return nil, err
	}
	for _, s := range moduleStats {
		stats.ByModule[s.Module] = s.Count
	}

	var methodStats []struct {
		Method string `json:"method"`
		Count  int64  `json:"count"`
	}
	if err := applyTimeRange(d.dbWithContext(ctx).Model(&model.OperationLog{}), startTime, endTime).
		Select("method, count(*) as count").
		Group("method").
		Find(&methodStats).Error; err != nil {
		return nil, err
	}
	for _, s := range methodStats {
		stats.ByMethod[s.Method] = s.Count
	}

	if err := applyTimeRange(d.dbWithContext(ctx).Model(&model.OperationLog{}), startTime, endTime).
		Where("status >= 400").
		Count(&stats.ErrorCount).Error; err != nil {
		return nil, err
	}

	return stats, nil
}

func applyTimeRange(query *gorm.DB, startTime, endTime *time.Time) *gorm.DB {
	if startTime != nil {
		query = query.Where("created_at >= ?", *startTime)
	}
	if endTime != nil {
		query = query.Where("created_at <= ?", *endTime)
	}
	return query
}

type LogStats struct {
	Total      int64            `json:"total"`
	ByModule   map[string]int64 `json:"by_module"`
	ByMethod   map[string]int64 `json:"by_method"`
	ErrorCount int64            `json:"error_count"`
}
