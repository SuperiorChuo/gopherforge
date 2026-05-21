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

type LoginLogDAO struct {
	db *gorm.DB
}

func NewLoginLogDAO(db *gorm.DB) *LoginLogDAO {
	return &LoginLogDAO{db: db}
}

func (d *LoginLogDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	if d != nil && d.db != nil {
		return d.db.WithContext(ctx)
	}
	return database.DB.WithContext(ctx)
}

// Deprecated: use CreateContext instead.
func (d *LoginLogDAO) Create(log *model.LoginLog) error {
	return d.CreateContext(context.Background(), log)
}

func (d *LoginLogDAO) CreateContext(ctx context.Context, log *model.LoginLog) error {
	return d.dbWithContext(ctx).Create(log).Error
}

// Deprecated: use GetByIDContext instead.
func (d *LoginLogDAO) GetByID(id uint) (*model.LoginLog, error) {
	return d.GetByIDContext(context.Background(), id)
}

func (d *LoginLogDAO) GetByIDContext(ctx context.Context, id uint) (*model.LoginLog, error) {
	var log model.LoginLog
	result := d.dbWithContext(ctx).First(&log, id)
	return &log, result.Error
}

// Deprecated: use GetListContext instead.
func (d *LoginLogDAO) GetList(
	req pagination.PageRequest,
	userID *uint,
	username, ip string,
	status *int8,
	loginType *int8,
	startTime, endTime *time.Time,
	dataScope authz.UserDataScope,
) ([]model.LoginLog, int64, error) {
	return d.GetListContext(context.Background(), req, userID, username, ip, status, loginType, startTime, endTime, dataScope)
}

func (d *LoginLogDAO) GetListContext(
	ctx context.Context,
	req pagination.PageRequest,
	userID *uint,
	username, ip string,
	status *int8,
	loginType *int8,
	startTime, endTime *time.Time,
	dataScope authz.UserDataScope,
) ([]model.LoginLog, int64, error) {
	var logs []model.LoginLog
	var total int64

	query := d.dbWithContext(ctx).Model(&model.LoginLog{})
	query = authz.ApplyOwnerScope(query, dataScope, "user_id")
	query = applyLoginLogFilters(query, userID, username, ip, status, loginType, startTime, endTime)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	result := query.
		Scopes(pagination.Paginate(req)).
		Order("created_at DESC").
		Find(&logs)

	return logs, total, result.Error
}

func applyLoginLogFilters(query *gorm.DB, userID *uint, username, ip string, status *int8, loginType *int8, startTime, endTime *time.Time) *gorm.DB {
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}
	if username != "" {
		query = query.Where("username LIKE ?", "%"+username+"%")
	}
	if ip != "" {
		query = query.Where("ip LIKE ?", "%"+ip+"%")
	}
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	if loginType != nil {
		query = query.Where("login_type = ?", *loginType)
	}
	return applyTimeRange(query, startTime, endTime)
}

// Deprecated: use GetUserLastLoginContext instead.
func (d *LoginLogDAO) GetUserLastLogin(userID uint) (*model.LoginLog, error) {
	return d.GetUserLastLoginContext(context.Background(), userID)
}

func (d *LoginLogDAO) GetUserLastLoginContext(ctx context.Context, userID uint) (*model.LoginLog, error) {
	var log model.LoginLog
	result := d.dbWithContext(ctx).Where("user_id = ? AND status = 1", userID).
		Order("created_at DESC").
		First(&log)
	return &log, result.Error
}

// Deprecated: use GetUserLoginCountContext instead.
func (d *LoginLogDAO) GetUserLoginCount(userID uint, startTime, endTime *time.Time) (int64, error) {
	return d.GetUserLoginCountContext(context.Background(), userID, startTime, endTime)
}

func (d *LoginLogDAO) GetUserLoginCountContext(ctx context.Context, userID uint, startTime, endTime *time.Time) (int64, error) {
	var count int64
	query := d.dbWithContext(ctx).Model(&model.LoginLog{}).Where("user_id = ? AND status = 1", userID)
	query = applyTimeRange(query, startTime, endTime)
	err := query.Count(&count).Error
	return count, err
}

// Deprecated: use GetFailedLoginCountContext instead.
func (d *LoginLogDAO) GetFailedLoginCount(username, ip string, since time.Time) (int64, error) {
	return d.GetFailedLoginCountContext(context.Background(), username, ip, since)
}

func (d *LoginLogDAO) GetFailedLoginCountContext(ctx context.Context, username, ip string, since time.Time) (int64, error) {
	var count int64
	query := d.dbWithContext(ctx).Model(&model.LoginLog{}).Where("status = 0 AND created_at >= ?", since)
	if username != "" {
		query = query.Where("username = ?", username)
	}
	if ip != "" {
		query = query.Where("ip = ?", ip)
	}
	err := query.Count(&count).Error
	return count, err
}

// Deprecated: use DeleteBeforeContext instead.
func (d *LoginLogDAO) DeleteBefore(before time.Time) (int64, error) {
	return d.DeleteBeforeContext(context.Background(), before)
}

func (d *LoginLogDAO) DeleteBeforeContext(ctx context.Context, before time.Time) (int64, error) {
	result := d.dbWithContext(ctx).Where("created_at < ?", before).Delete(&model.LoginLog{})
	return result.RowsAffected, result.Error
}

// Deprecated: use GetStatsContext instead.
func (d *LoginLogDAO) GetStats(startTime, endTime *time.Time) (*LoginLogStats, error) {
	return d.GetStatsContext(context.Background(), startTime, endTime)
}

func (d *LoginLogDAO) GetStatsContext(ctx context.Context, startTime, endTime *time.Time) (*LoginLogStats, error) {
	stats := &LoginLogStats{ByDevice: map[string]int64{}, ByBrowser: map[string]int64{}}

	if err := applyTimeRange(d.dbWithContext(ctx).Model(&model.LoginLog{}), startTime, endTime).Count(&stats.Total).Error; err != nil {
		return nil, err
	}

	if err := applyTimeRange(d.dbWithContext(ctx).Model(&model.LoginLog{}).Where("status = 1"), startTime, endTime).Count(&stats.Success).Error; err != nil {
		return nil, err
	}
	stats.Failed = stats.Total - stats.Success

	today := time.Now().Truncate(24 * time.Hour)
	if err := d.dbWithContext(ctx).Model(&model.LoginLog{}).
		Where("status = 1 AND created_at >= ?", today).
		Distinct("user_id").
		Count(&stats.TodayUsers).Error; err != nil {
		return nil, err
	}

	var deviceStats []struct {
		Device string `json:"device"`
		Count  int64  `json:"count"`
	}
	if err := applyTimeRange(d.dbWithContext(ctx).Model(&model.LoginLog{}).Where("status = 1"), startTime, endTime).
		Select("device, COUNT(*) as count").
		Group("device").
		Find(&deviceStats).Error; err != nil {
		return nil, err
	}
	for _, s := range deviceStats {
		stats.ByDevice[s.Device] = s.Count
	}

	var browserStats []struct {
		Browser string `json:"browser"`
		Count   int64  `json:"count"`
	}
	if err := applyTimeRange(d.dbWithContext(ctx).Model(&model.LoginLog{}).Where("status = 1"), startTime, endTime).
		Select("browser, COUNT(*) as count").
		Group("browser").
		Find(&browserStats).Error; err != nil {
		return nil, err
	}
	for _, s := range browserStats {
		stats.ByBrowser[s.Browser] = s.Count
	}

	return stats, nil
}

type LoginLogStats struct {
	Total      int64            `json:"total"`
	Success    int64            `json:"success"`
	Failed     int64            `json:"failed"`
	TodayUsers int64            `json:"today_users"`
	ByDevice   map[string]int64 `json:"by_device"`
	ByBrowser  map[string]int64 `json:"by_browser"`
}

type LoginTrendItem struct {
	Date    string `json:"date"`
	Count   int64  `json:"count"`
	Success int64  `json:"success"`
	Failed  int64  `json:"failed"`
}

// Deprecated: use GetLoginTrendContext instead.
func (d *LoginLogDAO) GetLoginTrend(days int) ([]LoginTrendItem, error) {
	return d.GetLoginTrendContext(context.Background(), days)
}

func (d *LoginLogDAO) GetLoginTrendContext(ctx context.Context, days int) ([]LoginTrendItem, error) {
	result := make([]LoginTrendItem, days)
	now := time.Now()
	for i := days - 1; i >= 0; i-- {
		date := now.AddDate(0, 0, -i)
		dateStr := date.Format("2006-01-02")
		startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
		endOfDay := startOfDay.Add(24 * time.Hour)

		var total, success int64
		if err := d.dbWithContext(ctx).Model(&model.LoginLog{}).
			Where("created_at >= ? AND created_at < ?", startOfDay, endOfDay).
			Count(&total).Error; err != nil {
			return nil, err
		}

		if err := d.dbWithContext(ctx).Model(&model.LoginLog{}).
			Where("created_at >= ? AND created_at < ? AND status = 1", startOfDay, endOfDay).
			Count(&success).Error; err != nil {
			return nil, err
		}

		result[days-1-i] = LoginTrendItem{
			Date:    dateStr,
			Count:   total,
			Success: success,
			Failed:  total - success,
		}
	}

	return result, nil
}
