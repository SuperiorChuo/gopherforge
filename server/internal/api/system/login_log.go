package system

import (
	"errors"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/logger"
	"github.com/go-admin-kit/server/internal/pkg/response"
	"github.com/go-admin-kit/server/internal/service/system"
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

	stats, err := a.logService.GetLoginStatsContext(c.Request.Context(), startTime, endTime)
	if err != nil {
		logLoginLogError("failed to get login stats", err)
		response.InternalServerError(c, "failed to get login stats")
		return
	}

	response.Success(c, stats)
}

// GetLoginTrend returns the login trend.
func (a *LoginLogAPI) GetLoginTrend(c *gin.Context) {
	days := 7 // Default to the last 7 days.
	if daysStr := c.Query("days"); daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil && d > 0 && d <= 30 {
			days = d
		}
	}

	trend, err := a.logService.GetLoginTrendContext(c.Request.Context(), days)
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
