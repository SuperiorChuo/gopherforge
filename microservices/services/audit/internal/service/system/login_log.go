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

// loginLogExportMaxRows 与操作日志导出保持同一上限：一次导出封顶 1 万行，
// 超出的部分要靠时间范围筛选，避免无界查询把内存吃满。
const loginLogExportMaxRows = 10000

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

// ExportLogsContext 取导出用的登录日志。与 OperationLogService.ExportLogsContext
// 同构：复用列表查询并把页大小抬到导出上限，过滤条件与页面所见完全一致。
func (s *LoginLogService) ExportLogsContext(ctx context.Context, req LoginLogListRequest) ([]model.LoginLog, error) {
	req.Page = 1
	req.PageSize = loginLogExportMaxRows
	logs, _, err := s.GetLogListContext(ctx, req)
	return logs, err
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

// ClearLogsAllTenantsContext 跨租户清理 days 天前的登录日志，仅供保留策略
// 后台任务使用（管理端 API 走带租户过滤的 ClearLogsContext）。
func (s *LoginLogService) ClearLogsAllTenantsContext(ctx context.Context, days int) (int64, error) {
	before := time.Now().AddDate(0, 0, -days)
	return s.logDAO.DeleteAllTenantsBeforeContext(ctx, before)
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

	// 平板先判：真实 iPad 的 UA 里带 "Mobile/15E148"，先判 mobile 会把平板
	// 全记成手机。
	if strings.Contains(ua, "ipad") || strings.Contains(ua, "tablet") {
		device = "Tablet"
	} else if strings.Contains(ua, "mobile") || strings.Contains(ua, "android") || strings.Contains(ua, "iphone") {
		device = "Mobile"
	} else {
		device = "Desktop"
	}

	// 移动端必须先判：iOS 的 UA 含 "like Mac OS X"，Android 的含 "Linux"，
	// 桌面关键字在前会把手机全部吞成 macOS / Linux——iOS 与 Android 两个
	// 分支此前永远不可达。
	switch {
	case strings.Contains(ua, "iphone") || strings.Contains(ua, "ipad") || strings.Contains(ua, "ipod"):
		os = "iOS"
	case strings.Contains(ua, "android"):
		os = "Android"
	case strings.Contains(ua, "windows"):
		os = "Windows"
	case strings.Contains(ua, "mac os") || strings.Contains(ua, "macos"):
		os = "macOS"
	case strings.Contains(ua, "linux"):
		os = "Linux"
	default:
		os = "Unknown"
	}

	// iOS 上所有浏览器都套 WebKit，UA 一律含 "safari"，靠各自前缀区分：
	// Chrome=CriOS、Firefox=FxiOS、Edge=EdgiOS，都必须排在 safari 之前。
	switch {
	case strings.Contains(ua, "crios"):
		browser = "Chrome"
	case strings.Contains(ua, "fxios"):
		browser = "Firefox"
	case strings.Contains(ua, "edg"): // 覆盖桌面 Edg/ 与 iOS EdgiOS/
		browser = "Edge"
	case strings.Contains(ua, "opera") || strings.Contains(ua, "opr"):
		browser = "Opera"
	case strings.Contains(ua, "chrome"):
		browser = "Chrome"
	case strings.Contains(ua, "firefox"):
		browser = "Firefox"
	case strings.Contains(ua, "safari"):
		browser = "Safari"
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
