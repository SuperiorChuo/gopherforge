package system

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/response"
	"github.com/go-admin-kit/server/internal/service/system"
)

// LoginLogAPI 登录日志 API
type LoginLogAPI struct {
	logService system.LoginLogService
}

// NewLoginLogAPI 创建 LoginLogAPI 实例
func NewLoginLogAPI() *LoginLogAPI {
	return &LoginLogAPI{
		logService: system.LoginLogService{},
	}
}

// GetLoginLogs 获取登录日志列表
func (a *LoginLogAPI) GetLoginLogs(c *gin.Context) {
	var req system.LoginLogListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, err.Error())
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
		response.InternalServerError(c, err.Error())
		return
	}
	req.DataScope = dataScope

	logs, total, err := a.logService.GetLogList(req)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.PageSuccess(c, logs, total, req.Page, req.PageSize)
}

// GetMyLoginLogs 获取当前用户的登录日志
func (a *LoginLogAPI) GetMyLoginLogs(c *gin.Context) {
	var req system.LoginLogListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, err.Error())
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

	logs, total, err := a.logService.GetLogList(req)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.PageSuccess(c, logs, total, req.Page, req.PageSize)
}

// GetLastLogin 获取最后登录信息
func (a *LoginLogAPI) GetLastLogin(c *gin.Context) {
	userID, _ := c.Get("user_id")

	log, err := a.logService.GetUserLastLogin(userID.(uint))
	if err != nil {
		response.Success(c, nil) // 如果没有记录，返回空
		return
	}

	response.Success(c, log)
}

// GetLoginStats 获取登录统计
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

	// 默认最近7天
	if startTime == nil {
		t := time.Now().AddDate(0, 0, -7)
		startTime = &t
	}
	if endTime == nil {
		t := time.Now()
		endTime = &t
	}

	stats, err := a.logService.GetLoginStats(startTime, endTime)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.Success(c, stats)
}

// GetLoginTrend 获取登录趋势
func (a *LoginLogAPI) GetLoginTrend(c *gin.Context) {
	days := 7 // 默认最近7天
	if daysStr := c.Query("days"); daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil && d > 0 && d <= 30 {
			days = d
		}
	}

	trend, err := a.logService.GetLoginTrend(days)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.Success(c, trend)
}

// ClearLoginLogs 清理登录日志
func (a *LoginLogAPI) ClearLoginLogs(c *gin.Context) {
	var req struct {
		Days int `json:"days" binding:"required,min=1"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	count, err := a.logService.ClearLogs(req.Days)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "logs cleared successfully", gin.H{
		"deleted_count": count,
	})
}

// GetUserLoginHistory 获取指定用户的登录历史
func (a *LoginLogAPI) GetUserLoginHistory(c *gin.Context) {
	userIDStr := c.Param("user_id")
	userID, err := strconv.ParseUint(userIDStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid user id")
		return
	}

	var req system.LoginLogListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, err.Error())
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
		response.InternalServerError(c, err.Error())
		return
	}
	req.DataScope = dataScope

	logs, total, err := a.logService.GetLogList(req)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.PageSuccess(c, logs, total, req.Page, req.PageSize)
}
