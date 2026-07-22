package system

import (
	"context"
	"errors"
	"strings"
	"time"

	systemdao "github.com/go-admin-kit/services/audit/internal/dao/system"
	"github.com/go-admin-kit/services/audit/internal/model"
	"github.com/go-admin-kit/services/audit/internal/pkg/authz"
	"github.com/go-admin-kit/services/audit/internal/pkg/ipinfo"
	"github.com/go-admin-kit/services/audit/internal/pkg/pagination"
	"github.com/go-admin-kit/services/audit/internal/pkg/tenant"
	"github.com/go-admin-kit/services/shared/pkg/iploc"
	"gorm.io/gorm"
)

type LoginLogService struct {
	logDAO systemdao.LoginLogDAO
}

// NewLoginLogServiceWithDB builds a LoginLogService backed by an injected
// database handle.
func NewLoginLogServiceWithDB(db *gorm.DB) LoginLogService {
	return LoginLogService{logDAO: *systemdao.NewLoginLogDAO(db)}
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
	UserID uint
	// TenantID scopes the login log row. Zero means resolve from context
	// (default tenant 1) at record time — used by NATS events that carry it.
	TenantID  uint
	Username  string
	LoginType int8
	Status    int8
	IP        string
	UserAgent string
	Message   string
	// OccurredAt is when the login happened. Zero means "now"; event
	// consumers set it so replayed backlogs keep their original times.
	OccurredAt time.Time
}

var ErrLoginLogNotFound = errors.New("login log not found")

func (s *LoginLogService) RecordContext(ctx context.Context, info *LoginInfo) error {
	device, os, browser := parseUserAgent(info.UserAgent)

	log := &model.LoginLog{
		TenantID:  tenant.EnsureID(ctx, info.TenantID),
		UserID:    info.UserID,
		Username:  info.Username,
		LoginType: info.LoginType,
		Status:    info.Status,
		IP:        info.IP,
		Location:  getIPLocation(ctx, info.IP),
		Device:    device,
		OS:        os,
		Browser:   browser,
		UserAgent: truncateString(info.UserAgent, 500),
		Message:   info.Message,
		CreatedAt: info.OccurredAt,
	}

	return s.logDAO.CreateContext(ctx, log)
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

func (s *LoginLogService) GetLoginStatsContext(ctx context.Context, startTime, endTime *time.Time) (*systemdao.LoginLogStats, error) {
	return s.logDAO.GetStatsContext(ctx, startTime, endTime)
}

func (s *LoginLogService) GetLoginStatsInScopeContext(ctx context.Context, startTime, endTime *time.Time, dataScope authz.UserDataScope) (*systemdao.LoginLogStats, error) {
	return s.logDAO.GetStatsInScopeContext(ctx, startTime, endTime, dataScope)
}

func (s *LoginLogService) ClearLogsContext(ctx context.Context, days int) (int64, error) {
	before := time.Now().AddDate(0, 0, -days)
	return s.logDAO.DeleteBeforeContext(ctx, before)
}

func (s *LoginLogService) GetLoginTrendContext(ctx context.Context, days int) ([]systemdao.LoginTrendItem, error) {
	return s.logDAO.GetLoginTrendContext(ctx, days)
}

func (s *LoginLogService) GetLoginTrendInScopeContext(ctx context.Context, days int, dataScope authz.UserDataScope) ([]systemdao.LoginTrendItem, error) {
	return s.logDAO.GetLoginTrendInScopeContext(ctx, days, dataScope)
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

// getIPLocation 解析 IP 归属地：优先走 ip2region 离线库（内网返回「内网」，
// 微秒级无外呼）；离线库未部署或查不到时回退 ip-api.com 在线查询，保持旧行为。
func getIPLocation(ctx context.Context, ip string) string {
	if loc := iploc.Lookup(ip); loc != "" {
		return loc
	}
	return ipinfo.GetLocationByIPContext(ctx, ip)
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
