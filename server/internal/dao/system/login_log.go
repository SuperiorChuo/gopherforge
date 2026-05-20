package system

import (
	"time"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
)

// LoginLogDAO 登录日志数据访问对象
type LoginLogDAO struct{}

// Create 创建登录日志
func (d *LoginLogDAO) Create(log *model.LoginLog) error {
	return database.DB.Create(log).Error
}

// GetByID 根据ID获取登录日志
func (d *LoginLogDAO) GetByID(id uint) (*model.LoginLog, error) {
	var log model.LoginLog
	result := database.DB.First(&log, id)
	return &log, result.Error
}

// GetList 获取登录日志列表
func (d *LoginLogDAO) GetList(
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

	query := database.DB.Model(&model.LoginLog{})
	query = authz.ApplyOwnerScope(query, dataScope, "user_id")

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

// GetUserLastLogin 获取用户最后登录记录
func (d *LoginLogDAO) GetUserLastLogin(userID uint) (*model.LoginLog, error) {
	var log model.LoginLog
	result := database.DB.Where("user_id = ? AND status = 1", userID).
		Order("created_at DESC").
		First(&log)
	return &log, result.Error
}

// GetUserLoginCount 获取用户登录次数
func (d *LoginLogDAO) GetUserLoginCount(userID uint, startTime, endTime *time.Time) (int64, error) {
	var count int64
	query := database.DB.Model(&model.LoginLog{}).Where("user_id = ? AND status = 1", userID)
	if startTime != nil {
		query = query.Where("created_at >= ?", *startTime)
	}
	if endTime != nil {
		query = query.Where("created_at <= ?", *endTime)
	}
	err := query.Count(&count).Error
	return count, err
}

// GetFailedLoginCount 获取登录失败次数
func (d *LoginLogDAO) GetFailedLoginCount(username, ip string, since time.Time) (int64, error) {
	var count int64
	query := database.DB.Model(&model.LoginLog{}).Where("status = 0 AND created_at >= ?", since)
	if username != "" {
		query = query.Where("username = ?", username)
	}
	if ip != "" {
		query = query.Where("ip = ?", ip)
	}
	err := query.Count(&count).Error
	return count, err
}

// DeleteBefore 删除指定时间之前的日志
func (d *LoginLogDAO) DeleteBefore(before time.Time) (int64, error) {
	result := database.DB.Where("created_at < ?", before).Delete(&model.LoginLog{})
	return result.RowsAffected, result.Error
}

// GetStats 获取登录统计
func (d *LoginLogDAO) GetStats(startTime, endTime *time.Time) (*LoginLogStats, error) {
	stats := &LoginLogStats{}

	query := database.DB.Model(&model.LoginLog{})
	if startTime != nil {
		query = query.Where("created_at >= ?", *startTime)
	}
	if endTime != nil {
		query = query.Where("created_at <= ?", *endTime)
	}

	// 总登录次数
	query.Count(&stats.Total)

	// 成功次数
	database.DB.Model(&model.LoginLog{}).
		Where("status = 1").
		Where("created_at >= ? AND created_at <= ?", startTime, endTime).
		Count(&stats.Success)

	// 失败次数
	stats.Failed = stats.Total - stats.Success

	// 今日登录用户数
	today := time.Now().Truncate(24 * time.Hour)
	database.DB.Model(&model.LoginLog{}).
		Where("status = 1 AND created_at >= ?", today).
		Distinct("user_id").
		Count(&stats.TodayUsers)

	// 按设备统计
	var deviceStats []struct {
		Device string `json:"device"`
		Count  int64  `json:"count"`
	}
	database.DB.Model(&model.LoginLog{}).
		Select("device, COUNT(*) as count").
		Where("created_at >= ? AND created_at <= ? AND status = 1", startTime, endTime).
		Group("device").
		Find(&deviceStats)
	stats.ByDevice = make(map[string]int64)
	for _, s := range deviceStats {
		stats.ByDevice[s.Device] = s.Count
	}

	// 按浏览器统计
	var browserStats []struct {
		Browser string `json:"browser"`
		Count   int64  `json:"count"`
	}
	database.DB.Model(&model.LoginLog{}).
		Select("browser, COUNT(*) as count").
		Where("created_at >= ? AND created_at <= ? AND status = 1", startTime, endTime).
		Group("browser").
		Find(&browserStats)
	stats.ByBrowser = make(map[string]int64)
	for _, s := range browserStats {
		stats.ByBrowser[s.Browser] = s.Count
	}

	return stats, nil
}

// LoginLogStats 登录统计信息
type LoginLogStats struct {
	Total      int64            `json:"total"`
	Success    int64            `json:"success"`
	Failed     int64            `json:"failed"`
	TodayUsers int64            `json:"today_users"`
	ByDevice   map[string]int64 `json:"by_device"`
	ByBrowser  map[string]int64 `json:"by_browser"`
}

// LoginTrendItem 登录趋势项
type LoginTrendItem struct {
	Date    string `json:"date"`
	Count   int64  `json:"count"`
	Success int64  `json:"success"`
	Failed  int64  `json:"failed"`
}

// GetLoginTrend 获取登录趋势（最近N天）
func (d *LoginLogDAO) GetLoginTrend(days int) ([]LoginTrendItem, error) {
	result := make([]LoginTrendItem, days)

	// 从今天开始往前推N天
	now := time.Now()
	for i := days - 1; i >= 0; i-- {
		date := now.AddDate(0, 0, -i)
		dateStr := date.Format("2006-01-02")
		startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
		endOfDay := startOfDay.Add(24 * time.Hour)

		var total, success int64
		database.DB.Model(&model.LoginLog{}).
			Where("created_at >= ? AND created_at < ?", startOfDay, endOfDay).
			Count(&total)

		database.DB.Model(&model.LoginLog{}).
			Where("created_at >= ? AND created_at < ? AND status = 1", startOfDay, endOfDay).
			Count(&success)

		result[days-1-i] = LoginTrendItem{
			Date:    dateStr,
			Count:   total,
			Success: success,
			Failed:  total - success,
		}
	}

	return result, nil
}
