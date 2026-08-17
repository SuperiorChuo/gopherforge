package system

import (
	"encoding/csv"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/audit/internal/service/system"
	"github.com/go-admin-kit/services/shared/pkg/authz"
	"github.com/go-admin-kit/services/shared/pkg/logger"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

// LoginLogAPI handles login log endpoints.
type LoginLogAPI struct {
	logService system.LoginLogService
}

// NewLoginLogAPI creates a LoginLogAPI instance.
func NewLoginLogAPI() *LoginLogAPI {
	return &LoginLogAPI{
		logService: system.LoginLogService{},
	}
}

// NewLoginLogAPIWithService creates a LoginLogAPI instance from an injected service.
func NewLoginLogAPIWithService(logService system.LoginLogService) *LoginLogAPI {
	return &LoginLogAPI{logService: logService}
}

// GetLoginLogs returns paginated login logs.
func (a *LoginLogAPI) GetLoginLogs(c *gin.Context) {
	var req system.LoginLogListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "invalid query parameters")
		return
	}

	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 10
	}

	dataScope, err := authz.ResolveUserDataScopeFromContext(c)
	if err != nil {
		logLoginLogError("failed to resolve login log data scope", err)
		response.InternalServerError(c, "failed to get login logs")
		return
	}
	req.DataScope = dataScope

	logs, total, err := a.logService.GetLogListContext(c.Request.Context(), req)
	if err != nil {
		logLoginLogError("failed to get login logs", err)
		response.InternalServerError(c, "failed to get login logs")
		return
	}

	response.PageSuccess(c, logs, total, req.Page, req.PageSize)
}

// ExportLoginLogs exports login logs as CSV, honouring the same filters and
// data scope as the list endpoint.
func (a *LoginLogAPI) ExportLoginLogs(c *gin.Context) {
	var req system.LoginLogListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "invalid query parameters")
		return
	}

	dataScope, err := authz.ResolveUserDataScopeFromContext(c)
	if err != nil {
		logLoginLogError("failed to resolve login log export data scope", err)
		response.InternalServerError(c, "failed to export login logs")
		return
	}
	req.DataScope = dataScope

	logs, err := a.logService.ExportLogsContext(c.Request.Context(), req)
	if err != nil {
		logLoginLogError("failed to export login logs", err)
		response.InternalServerError(c, "failed to export login logs")
		return
	}

	filename := fmt.Sprintf("login_logs_%s.csv", time.Now().Format("20060102150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))

	// UTF-8 BOM: Excel on Windows otherwise reads the Chinese columns
	// (location, message) as mojibake.
	_, _ = c.Writer.WriteString("\xEF\xBB\xBF")

	writer := csv.NewWriter(c.Writer)
	defer writer.Flush()

	_ = writer.Write([]string{
		"ID", "User ID", "Username", "Login Type", "Status", "IP", "Location",
		"Device", "OS", "Browser", "Message", "Created At",
	})

	for _, entry := range logs {
		_ = writer.Write([]string{
			strconv.FormatUint(uint64(entry.ID), 10),
			strconv.FormatUint(uint64(entry.UserID), 10),
			entry.Username,
			loginTypeLabel(entry.LoginType),
			loginStatusLabel(entry.Status),
			entry.IP,
			entry.Location,
			entry.Device,
			entry.OS,
			entry.Browser,
			entry.Message,
			entry.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
}

// loginTypeLabel maps the stored code to a readable name. Codes are defined by
// events.LoginType* (1=password, 2=github, 3=wechat, 4=totp); an unknown code
// falls back to the raw number rather than silently reading as a known type.
func loginTypeLabel(loginType int8) string {
	switch loginType {
	case 1:
		return "password"
	case 2:
		return "github"
	case 3:
		return "wechat"
	case 4:
		return "totp"
	default:
		return strconv.Itoa(int(loginType))
	}
}

func loginStatusLabel(status int8) string {
	if status == 1 {
		return "success"
	}
	return "failed"
}

// GetMyLoginLogs returns login logs for the current user.
func (a *LoginLogAPI) GetMyLoginLogs(c *gin.Context) {
	var req system.LoginLogListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "invalid query parameters")
		return
	}

	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 10
	}

	userID, _ := c.Get("user_id")
	uid := userID.(uint)
	req.UserID = &uid
	req.DataScope = authz.UserDataScope{
		Scope:  authz.DataScopeSelf,
		UserID: uid,
	}

	logs, total, err := a.logService.GetLogListContext(c.Request.Context(), req)
	if err != nil {
		logLoginLogError("failed to get current user login logs", err)
		response.InternalServerError(c, "failed to get login logs")
		return
	}

	response.PageSuccess(c, logs, total, req.Page, req.PageSize)
}

// GetLastLogin returns the current user's last login record.
func (a *LoginLogAPI) GetLastLogin(c *gin.Context) {
	userID, _ := c.Get("user_id")

	log, err := a.logService.GetUserLastLoginContext(c.Request.Context(), userID.(uint))
	if err != nil {
		if errors.Is(err, system.ErrLoginLogNotFound) {
			response.Success(c, nil) // Return empty data when no record exists.
			return
		}
		internalServerError(c, "failed to get last login", err)
		return
	}

	response.Success(c, log)
}

// GetLoginStats returns login statistics.
func (a *LoginLogAPI) GetLoginStats(c *gin.Context) {
	var startTime, endTime *time.Time

	if startStr := c.Query("start_time"); startStr != "" {
		if t, err := time.Parse("2006-01-02 15:04:05", startStr); err == nil {
			startTime = &t
		}
	}
	if endStr := c.Query("end_time"); endStr != "" {
		if t, err := time.Parse("2006-01-02 15:04:05", endStr); err == nil {
			endTime = &t
		}
	}

	// Default to the last 7 days.
	if startTime == nil {
		t := time.Now().AddDate(0, 0, -7)
		startTime = &t
	}
	if endTime == nil {
		t := time.Now()
		endTime = &t
	}

	dataScope, err := authz.ResolveUserDataScopeFromContext(c)
	if err != nil {
		logLoginLogError("failed to resolve login stats data scope", err)
		response.InternalServerError(c, "failed to get login stats")
		return
	}

	stats, err := a.logService.GetLoginStatsInScopeContext(c.Request.Context(), startTime, endTime, dataScope)
	if err != nil {
		logLoginLogError("failed to get login stats", err)
		response.InternalServerError(c, "failed to get login stats")
		return
	}

	response.Success(c, stats)
}

// GetLoginGeoDistribution returns login counts grouped by location.
// Window semantics match GetLoginTrend: only a `days` count is accepted and
// the range is computed on the server clock, so client-local time strings
// never get misparsed as UTC.
func (a *LoginLogAPI) GetLoginGeoDistribution(c *gin.Context) {
	days := 7
	if daysStr := c.Query("days"); daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil && d > 0 && d <= 90 {
			days = d
		}
	}
	start := time.Now().AddDate(0, 0, -days)
	end := time.Now()
	startTime, endTime := &start, &end

	dataScope, err := authz.ResolveUserDataScopeFromContext(c)
	if err != nil {
		logLoginLogError("failed to resolve login geo distribution data scope", err)
		response.InternalServerError(c, "failed to get login geo distribution")
		return
	}

	items, err := a.logService.GetLoginGeoDistributionInScopeContext(c.Request.Context(), startTime, endTime, dataScope)
	if err != nil {
		logLoginLogError("failed to get login geo distribution", err)
		response.InternalServerError(c, "failed to get login geo distribution")
		return
	}

	response.Success(c, items)
}

// GetLoginTrend returns the login trend.
func (a *LoginLogAPI) GetLoginTrend(c *gin.Context) {
	days := 7 // Default to the last 7 days.
	if daysStr := c.Query("days"); daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil && d > 0 && d <= 30 {
			days = d
		}
	}

	dataScope, err := authz.ResolveUserDataScopeFromContext(c)
	if err != nil {
		logLoginLogError("failed to resolve login trend data scope", err)
		response.InternalServerError(c, "failed to get login trend")
		return
	}

	trend, err := a.logService.GetLoginTrendInScopeContext(c.Request.Context(), days, dataScope)
	if err != nil {
		logLoginLogError("failed to get login trend", err)
		response.InternalServerError(c, "failed to get login trend")
		return
	}

	response.Success(c, trend)
}

// ClearLoginLogs deletes old login logs.
func (a *LoginLogAPI) ClearLoginLogs(c *gin.Context) {
	var req struct {
		Days int `json:"days" binding:"required,min=1"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	count, err := a.logService.ClearLogsContext(c.Request.Context(), req.Days)
	if err != nil {
		logLoginLogError("failed to clear login logs", err)
		response.InternalServerError(c, "failed to clear login logs")
		return
	}

	response.SuccessWithMessage(c, "logs cleared successfully", gin.H{
		"deleted_count": count,
	})
}

// GetUserLoginHistory returns login history for a specific user.
func (a *LoginLogAPI) GetUserLoginHistory(c *gin.Context) {
	userIDStr := c.Param("user_id")
	userID, err := strconv.ParseUint(userIDStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid user id")
		return
	}

	var req system.LoginLogListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "invalid query parameters")
		return
	}

	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 10
	}

	uid := uint(userID)
	req.UserID = &uid
	dataScope, err := authz.ResolveUserDataScopeFromContext(c)
	if err != nil {
		logLoginLogError("failed to resolve user login history data scope", err)
		response.InternalServerError(c, "failed to get user login history")
		return
	}
	req.DataScope = dataScope

	logs, total, err := a.logService.GetLogListContext(c.Request.Context(), req)
	if err != nil {
		logLoginLogError("failed to get user login history", err)
		response.InternalServerError(c, "failed to get user login history")
		return
	}

	response.PageSuccess(c, logs, total, req.Page, req.PageSize)
}

func logLoginLogError(message string, err error) {
	if logger.Logger == nil {
		return
	}
	logger.Error(message, logger.Err(err))
}
