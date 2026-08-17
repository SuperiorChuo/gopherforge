package system

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	systemdao "github.com/go-admin-kit/services/audit/internal/dao/system"
	localmodel "github.com/go-admin-kit/services/audit/internal/model"
	"github.com/go-admin-kit/services/audit/internal/pkg/ipinfo"
	"github.com/go-admin-kit/services/audit/internal/pkg/runtimeconfig"
	"github.com/go-admin-kit/services/shared/pkg/authz"
	"github.com/go-admin-kit/services/shared/pkg/iploc"
	"github.com/go-admin-kit/services/shared/pkg/notifyclient"
	"github.com/go-admin-kit/services/shared/pkg/pagination"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
)

type LoginLogService struct {
	logDAO  systemdao.LoginLogDAO
	riskDAO *systemdao.LoginRiskEventDAO
	// notify delivers the new-device/new-IP login alert to the user; nil
	// (or unconfigured) disables it.
	notify *notifyclient.Client
}

// NewLoginLogServiceWithDB builds a LoginLogService backed by an injected
// database handle.
func NewLoginLogServiceWithDB(db *gorm.DB) LoginLogService {
	return LoginLogService{
		logDAO:  *systemdao.NewLoginLogDAO(db),
		riskDAO: systemdao.NewLoginRiskEventDAO(db),
	}
}

// WithNotifier attaches the in-console alert client (new-device login).
func (s LoginLogService) WithNotifier(n *notifyclient.Client) LoginLogService {
	s.notify = n
	return s
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

// LoginInfo.Status 取值（与 events 包的登录事件状态对齐）。
const (
	loginStatusSuccess int8 = 1
	loginStatusFailed  int8 = 0
)

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
	DeviceID  string
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

	log := &localmodel.LoginLog{
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
		DeviceID:  truncateString(info.DeviceID, 64),
		Message:   info.Message,
		CreatedAt: info.OccurredAt,
	}

	// 登录成功后风控：新 IP 或新设备 → 落库事件 + 站内信提醒。
	// 对比基线必须在插入当前行之前取——否则"上次登录"就是刚写的自己，永远不异常。
	var previous *localmodel.LoginLog
	if info.Status == loginStatusSuccess && info.UserID > 0 {
		if last, err := s.logDAO.GetUserLastLoginContext(ctx, info.UserID); err == nil {
			previous = last
		}
	}

	if err := s.logDAO.CreateContext(ctx, log); err != nil {
		return err
	}
	if previous != nil {
		s.recordRiskEvent(ctx, info, log, previous)
	}
	return nil
}

func (s *LoginLogService) GetLogListContext(ctx context.Context, req LoginLogListRequest) ([]localmodel.LoginLog, int64, error) {
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
func (s *LoginLogService) ExportLogsContext(ctx context.Context, req LoginLogListRequest) ([]localmodel.LoginLog, error) {
	req.Page = 1
	req.PageSize = loginLogExportMaxRows
	logs, _, err := s.GetLogListContext(ctx, req)
	return logs, err
}

func (s *LoginLogService) GetUserLastLoginContext(ctx context.Context, userID uint) (*localmodel.LoginLog, error) {
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

// ClearRiskEventsContext removes abnormal-login risk events older than the
// retention window (login_risk_events 无界增长防护——随登录日志同周期清理)。
func (s *LoginLogService) ClearRiskEventsContext(ctx context.Context, days int) (int64, error) {
	before := time.Now().AddDate(0, 0, -days)
	return s.riskDAO.DeleteBeforeContext(ctx, before)
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

// LoginGeoDistItem 一条登录地域分布：保留 location 原文供前端展示，
// province/city 是尽力拆解结果，前端据此查坐标表落图。
type LoginGeoDistItem struct {
	Location string `json:"location"`
	Province string `json:"province"`
	City     string `json:"city"`
	Total    int64  `json:"total"`
	Success  int64  `json:"success"`
	Failed   int64  `json:"failed"`
}

func (s *LoginLogService) GetLoginGeoDistributionInScopeContext(ctx context.Context, startTime, endTime *time.Time, dataScope authz.UserDataScope) ([]LoginGeoDistItem, error) {
	rows, err := s.logDAO.GetGeoDistributionInScopeContext(ctx, startTime, endTime, dataScope)
	if err != nil {
		return nil, err
	}
	items := make([]LoginGeoDistItem, 0, len(rows))
	for _, r := range rows {
		province, city := parseLocation(r.Location)
		items = append(items, LoginGeoDistItem{
			Location: r.Location,
			Province: province,
			City:     city,
			Total:    r.Total,
			Success:  r.Success,
			Failed:   r.Failed,
		})
	}
	return items, nil
}

// parseLocation 从归属地串拆省/市。iploc 中文串形如「广东省 深圳市 电信」
// 「北京 北京市 联通」「香港」；ipinfo 英文兜底串（如「Guangdong Shenzhen」）
// 只取首段当省，市留空由前端按省兜底；空串与内网各归独立桶。
func parseLocation(location string) (province, city string) {
	loc := strings.TrimSpace(location)
	if loc == "" {
		return "未知", ""
	}
	if loc == iploc.IntranetLabel || loc == "Private Network" {
		return iploc.IntranetLabel, ""
	}
	parts := strings.Fields(loc)
	province = parts[0]
	if len(parts) > 1 && looksLikeCity(parts[1]) {
		city = parts[1]
	}
	return province, city
}

// looksLikeCity 报告段是否像市/州级行政区，用于排除 ISP 段（「广东省 电信」）。
func looksLikeCity(s string) bool {
	for _, suffix := range []string{"市", "州", "盟", "地区", "县", "区"} {
		if strings.HasSuffix(s, suffix) {
			return true
		}
	}
	return false
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
	// 分支此前永远不可达（生产库里真机 iPhone 登录被记成 macOS）。
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

// notifyAbnormalLogin sends an in-console alert when a login comes from an IP
// or device different from the user's previous login (captured before the
// recordRiskEvent persists an abnormal-login event (new IP / new device
// relative to the user's previous successful login, captured before the current
// row was inserted) and — when the alert toggle is on and notify is available —
// sends the in-console alert. Both are best-effort: any failure is logged and
// never blocks the login path.
func (s *LoginLogService) recordRiskEvent(ctx context.Context, info *LoginInfo, current, previous *localmodel.LoginLog) {
	reason := abnormalLoginReason(current, previous)
	if reason == "" {
		return
	}
	event := &localmodel.LoginRiskEvent{
		TenantID:  tenant.EnsureID(ctx, info.TenantID),
		UserID:    info.UserID,
		Username:  current.Username,
		IP:        current.IP,
		DeviceID:  current.DeviceID,
		Reason:    reason,
		CreatedAt: current.CreatedAt,
	}
	// 落库失败不阻断登录，但也不该吞掉告警：INSERT 失败仍尝试发站内信
	// （告警独立于事件表，DB 故障时用户最需要收到提醒）。
	eventErr := s.riskDAO.CreateContext(ctx, event)

	policy := runtimeconfig.DefaultSecurityPolicyReader().SecurityPolicy(ctx)
	if !policy.LoginAlertEnabled || s.notify == nil || !s.notify.Enabled() {
		return
	}
	if _, err := s.notify.Send(ctx, notifyclient.SendInput{
		TenantID:     uint64(tenant.EnsureID(ctx, info.TenantID)),
		UserID:       uint64(info.UserID),
		TemplateCode: "login_alert",
		Type:         "security",
		RefType:      "login",
		RefID:        fmt.Sprintf("login-%d", info.UserID),
		Title:        "新登录提醒",
		Content: fmt.Sprintf("你的账号刚从%s登录（IP %s，%s）。如非本人操作，请立即修改密码。",
			riskReasonLabel(reason), current.IP, current.Location),
		Link: "/system/login-log",
	}); err != nil {
		// 发送失败不标已提醒——事件表保持「未提醒」，管理员可跟进重发/排查。
		return
	}
	if eventErr == nil {
		_ = s.riskDAO.MarkNotifiedContext(ctx, event.ID)
	}
}

// abnormalLoginReason classifies why a login differs from the previous one.
// Empty means the login is not abnormal.
func abnormalLoginReason(current, previous *localmodel.LoginLog) string {
	if previous == nil {
		return ""
	}
	switch {
	case previous.IP != current.IP:
		return localmodel.LoginRiskReasonNewIP
	case current.DeviceID != "" && previous.DeviceID != "" && previous.DeviceID != current.DeviceID:
		return localmodel.LoginRiskReasonNewDevice
	}
	return ""
}

func riskReasonLabel(reason string) string {
	switch reason {
	case localmodel.LoginRiskReasonNewIP:
		return "新的 IP 地址"
	case localmodel.LoginRiskReasonNewDevice:
		return "新的设备"
	}
	return reason
}
