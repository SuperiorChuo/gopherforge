package system

import (
	"context"
	"errors"
	"strings"
	"time"

	systemdao "github.com/go-admin-kit/server/internal/dao/system"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/ipinfo"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
	"gorm.io/gorm"
)

type LoginLogService struct {
	logDAO systemdao.LoginLogDAO
}

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

type LoginInfo struct {
	UserID    uint
	Username  string
	LoginType int8
	Status    int8
	IP        string
	UserAgent string
	Message   string
}

var ErrLoginLogNotFound = errors.New("login log not found")

// Deprecated: use RecordContext instead.
func (s *LoginLogService) Record(info *LoginInfo) error {
	return s.RecordContext(context.Background(), info)
}

func (s *LoginLogService) RecordContext(ctx context.Context, info *LoginInfo) error {
	device, os, browser := parseUserAgent(info.UserAgent)

	log := &model.LoginLog{
		UserID:    info.UserID,
		Username:  info.Username,
		LoginType: info.LoginType,
		Status:    info.Status,
		IP:        info.IP,
		Location:  getIPLocation(info.IP),
		Device:    device,
		OS:        os,
		Browser:   browser,
		UserAgent: truncateString(info.UserAgent, 500),
		Message:   info.Message,
	}

	return s.logDAO.CreateContext(ctx, log)
}

// Deprecated: use GetLogListContext instead.
func (s *LoginLogService) GetLogList(req LoginLogListRequest) ([]model.LoginLog, int64, error) {
	return s.GetLogListContext(context.Background(), req)
}

func (s *LoginLogService) GetLogListContext(ctx context.Context, req LoginLogListRequest) ([]model.LoginLog, int64, error) {
	return s.logDAO.GetListContext(
		ctx,
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

// Deprecated: use GetUserLastLoginContext instead.
func (s *LoginLogService) GetUserLastLogin(userID uint) (*model.LoginLog, error) {
	return s.GetUserLastLoginContext(context.Background(), userID)
}

func (s *LoginLogService) GetUserLastLoginContext(ctx context.Context, userID uint) (*model.LoginLog, error) {
	log, err := s.logDAO.GetUserLastLoginContext(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrLoginLogNotFound
		}
		return nil, err
	}
	return log, nil
}

// Deprecated: use GetLoginStatsContext instead.
func (s *LoginLogService) GetLoginStats(startTime, endTime *time.Time) (*systemdao.LoginLogStats, error) {
	return s.GetLoginStatsContext(context.Background(), startTime, endTime)
}

func (s *LoginLogService) GetLoginStatsContext(ctx context.Context, startTime, endTime *time.Time) (*systemdao.LoginLogStats, error) {
	return s.logDAO.GetStatsContext(ctx, startTime, endTime)
}

func (s *LoginLogService) GetLoginStatsInScopeContext(ctx context.Context, startTime, endTime *time.Time, dataScope authz.UserDataScope) (*systemdao.LoginLogStats, error) {
	return s.logDAO.GetStatsInScopeContext(ctx, startTime, endTime, dataScope)
}

// Deprecated: use ClearLogsContext instead.
func (s *LoginLogService) ClearLogs(days int) (int64, error) {
	return s.ClearLogsContext(context.Background(), days)
}

func (s *LoginLogService) ClearLogsContext(ctx context.Context, days int) (int64, error) {
	before := time.Now().AddDate(0, 0, -days)
	return s.logDAO.DeleteBeforeContext(ctx, before)
}

// Deprecated: use GetLoginTrendContext instead.
func (s *LoginLogService) GetLoginTrend(days int) ([]systemdao.LoginTrendItem, error) {
	return s.GetLoginTrendContext(context.Background(), days)
}

func (s *LoginLogService) GetLoginTrendContext(ctx context.Context, days int) ([]systemdao.LoginTrendItem, error) {
	return s.logDAO.GetLoginTrendContext(ctx, days)
}

func (s *LoginLogService) GetLoginTrendInScopeContext(ctx context.Context, days int, dataScope authz.UserDataScope) ([]systemdao.LoginTrendItem, error) {
	return s.logDAO.GetLoginTrendInScopeContext(ctx, days, dataScope)
}

// Deprecated: use CheckAbnormalLoginContext instead.
func (s *LoginLogService) CheckAbnormalLogin(userID uint, ip string) (bool, string) {
	return s.CheckAbnormalLoginContext(context.Background(), userID, ip)
}

func (s *LoginLogService) CheckAbnormalLoginContext(ctx context.Context, userID uint, ip string) (bool, string) {
	lastLogin, err := s.logDAO.GetUserLastLoginContext(ctx, userID)
	if err != nil {
		return false, ""
	}
	if lastLogin.IP != ip {
		return true, "new IP address detected"
	}
	return false, ""
}

// Deprecated: use GetFailedLoginCountContext instead.
func (s *LoginLogService) GetFailedLoginCount(username, ip string, minutes int) (int64, error) {
	return s.GetFailedLoginCountContext(context.Background(), username, ip, minutes)
}

func (s *LoginLogService) GetFailedLoginCountContext(ctx context.Context, username, ip string, minutes int) (int64, error) {
	since := time.Now().Add(-time.Duration(minutes) * time.Minute)
	return s.logDAO.GetFailedLoginCountContext(ctx, username, ip, since)
}

func parseUserAgent(ua string) (device, os, browser string) {
	ua = strings.ToLower(ua)

	if strings.Contains(ua, "mobile") || strings.Contains(ua, "android") || strings.Contains(ua, "iphone") {
		device = "Mobile"
	} else if strings.Contains(ua, "tablet") || strings.Contains(ua, "ipad") {
		device = "Tablet"
	} else {
		device = "Desktop"
	}

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

func getIPLocation(ip string) string {
	return ipinfo.GetLocationByIP(ip)
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
