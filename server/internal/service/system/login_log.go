package system

import (
	"strings"
	"time"

	"github.com/go-admin-kit/server/internal/dao/system"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/ipinfo"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
)

// LoginLogService 登录日志服务
type LoginLogService struct {
	logDAO system.LoginLogDAO
}

// LoginLogListRequest 登录日志列表请求
type LoginLogListRequest struct {
	pagination.PageRequest
	UserID    *uint               `form:"user_id" json:"user_id"`
	Username  string              `form:"username" json:"username"`
	IP        string              `form:"ip" json:"ip"`
	Status    *int8               `form:"status" json:"status"`
	LoginType *int8               `form:"login_type" json:"login_type"`
	StartTime *time.Time          `form:"start_time" time_format:"2006-01-02 15:04:05" json:"start_time"`
	EndTime   *time.Time          `form:"end_time" time_format:"2006-01-02 15:04:05" json:"end_time"`
	DataScope authz.UserDataScope `json:"-" form:"-"`
}

// LoginInfo 登录信息（用于记录日志）
type LoginInfo struct {
	UserID    uint
	Username  string
	LoginType int8 // 1账号密码，2GitHub，3微信
	Status    int8 // 1成功，0失败
	IP        string
	UserAgent string
	Message   string
}

// Record 记录登录日志
func (s *LoginLogService) Record(info *LoginInfo) error {
	// 解析 UserAgent
	device, os, browser := parseUserAgent(info.UserAgent)

	log := &model.LoginLog{
		UserID:    info.UserID,
		Username:  info.Username,
		LoginType: info.LoginType,
		Status:    info.Status,
		IP:        info.IP,
		Location:  getIPLocation(info.IP), // 获取IP归属地
		Device:    device,
		OS:        os,
		Browser:   browser,
		UserAgent: truncateString(info.UserAgent, 500),
		Message:   info.Message,
	}

	return s.logDAO.Create(log)
}

// GetLogList 获取登录日志列表
func (s *LoginLogService) GetLogList(req LoginLogListRequest) ([]model.LoginLog, int64, error) {
	return s.logDAO.GetList(
		req.PageRequest,
		req.UserID,
		req.Username,
		req.IP,
		req.Status,
		req.LoginType,
		req.StartTime,
		req.EndTime,
		req.DataScope,
	)
}

// GetUserLastLogin 获取用户最后登录记录
func (s *LoginLogService) GetUserLastLogin(userID uint) (*model.LoginLog, error) {
	return s.logDAO.GetUserLastLogin(userID)
}

// GetLoginStats 获取登录统计
func (s *LoginLogService) GetLoginStats(startTime, endTime *time.Time) (*system.LoginLogStats, error) {
	return s.logDAO.GetStats(startTime, endTime)
}

// ClearLogs 清理旧日志
func (s *LoginLogService) ClearLogs(days int) (int64, error) {
	before := time.Now().AddDate(0, 0, -days)
	return s.logDAO.DeleteBefore(before)
}

// GetLoginTrend 获取登录趋势
func (s *LoginLogService) GetLoginTrend(days int) ([]system.LoginTrendItem, error) {
	return s.logDAO.GetLoginTrend(days)
}

// CheckAbnormalLogin 检查异常登录
func (s *LoginLogService) CheckAbnormalLogin(userID uint, ip string) (bool, string) {
	// 获取用户最后登录记录
	lastLogin, err := s.logDAO.GetUserLastLogin(userID)
	if err != nil {
		return false, ""
	}

	// 检查是否是新设备/新IP
	if lastLogin.IP != ip {
		return true, "new IP address detected"
	}

	// 可以添加更多检查：
	// - 异地登录检测
	// - 短时间内多次登录
	// - 非常规时间登录

	return false, ""
}

// GetFailedLoginCount 获取登录失败次数
func (s *LoginLogService) GetFailedLoginCount(username, ip string, minutes int) (int64, error) {
	since := time.Now().Add(-time.Duration(minutes) * time.Minute)
	return s.logDAO.GetFailedLoginCount(username, ip, since)
}

// parseUserAgent 解析 UserAgent
func parseUserAgent(ua string) (device, os, browser string) {
	ua = strings.ToLower(ua)

	// 设备类型
	if strings.Contains(ua, "mobile") || strings.Contains(ua, "android") || strings.Contains(ua, "iphone") {
		device = "Mobile"
	} else if strings.Contains(ua, "tablet") || strings.Contains(ua, "ipad") {
		device = "Tablet"
	} else {
		device = "Desktop"
	}

	// 操作系统
	switch {
	case strings.Contains(ua, "windows"):
		os = "Windows"
	case strings.Contains(ua, "mac os"):
		os = "macOS"
	case strings.Contains(ua, "linux"):
		os = "Linux"
	case strings.Contains(ua, "android"):
		os = "Android"
	case strings.Contains(ua, "iphone") || strings.Contains(ua, "ipad"):
		os = "iOS"
	default:
		os = "Unknown"
	}

	// 浏览器
	switch {
	case strings.Contains(ua, "chrome") && !strings.Contains(ua, "edg"):
		browser = "Chrome"
	case strings.Contains(ua, "firefox"):
		browser = "Firefox"
	case strings.Contains(ua, "safari") && !strings.Contains(ua, "chrome"):
		browser = "Safari"
	case strings.Contains(ua, "edg"):
		browser = "Edge"
	case strings.Contains(ua, "opera") || strings.Contains(ua, "opr"):
		browser = "Opera"
	case strings.Contains(ua, "msie") || strings.Contains(ua, "trident"):
		browser = "IE"
	default:
		browser = "Unknown"
	}

	return
}

// getIPLocation 获取IP归属地
// 使用 ip-api.com 免费 API 查询
// 文档：https://ip-api.com/docs/api:json
func getIPLocation(ip string) string {
	return ipinfo.GetLocationByIP(ip)
}

// truncateString 截断字符串
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
