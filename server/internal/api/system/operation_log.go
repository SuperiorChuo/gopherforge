package system

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/response"
	"github.com/go-admin-kit/server/internal/service/system"
)

// OperationLogAPI 操作日志 API
type OperationLogAPI struct {
	logService system.OperationLogService
}

// NewOperationLogAPI 创建 OperationLogAPI 实例
func NewOperationLogAPI() *OperationLogAPI {
	return &OperationLogAPI{
		logService: system.OperationLogService{},
	}
}

// GetOperationLogs 获取操作日志列表
func (a *OperationLogAPI) GetOperationLogs(c *gin.Context) {
	var req system.OperationLogListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// 默认分页
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

// GetOperationLogDetail 获取操作日志详情
func (a *OperationLogAPI) GetOperationLogDetail(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid log id")
		return
	}

	log, err := a.logService.GetLogByID(uint(id))
	if err != nil {
		response.NotFound(c, "log not found")
		return
	}

	response.Success(c, log)
}

// ClearOperationLogs 清理操作日志
func (a *OperationLogAPI) ClearOperationLogs(c *gin.Context) {
	var req system.ClearLogsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	count, err := a.logService.ClearLogs(req.Days)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, fmt.Sprintf("successfully deleted %d logs", count), gin.H{
		"deleted_count": count,
	})
}

// GetOperationLogStats 获取操作日志统计
func (a *OperationLogAPI) GetOperationLogStats(c *gin.Context) {
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

	// 如果没有指定时间范围，默认最近7天
	if startTime == nil {
		t := time.Now().AddDate(0, 0, -7)
		startTime = &t
	}
	if endTime == nil {
		t := time.Now()
		endTime = &t
	}

	stats, err := a.logService.GetLogStats(startTime, endTime)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.Success(c, stats)
}

// ExportOperationLogs 导出操作日志
func (a *OperationLogAPI) ExportOperationLogs(c *gin.Context) {
	var req system.OperationLogListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	dataScope, err := authz.ResolveUserDataScopeFromContext(c)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}
	req.DataScope = dataScope

	logs, err := a.logService.ExportLogs(req)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	// 设置响应头
	filename := fmt.Sprintf("operation_logs_%s.csv", time.Now().Format("20060102150405"))
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))

	// 写入 CSV
	writer := csv.NewWriter(c.Writer)
	defer writer.Flush()

	// 写入表头
	headers := []string{"ID", "用户ID", "用户名", "模块", "操作", "方法", "路径", "状态码", "IP", "耗时(ms)", "创建时间"}
	headers = []string{"ID", "User ID", "Username", "Actor Type", "Actor ID", "Request ID", "Module", "Action", "Method", "Path", "Status", "IP", "Latency(ms)", "Created At"}
	writer.Write(headers)

	// 写入数据
	for _, log := range logs {
		row := []string{
			strconv.FormatUint(uint64(log.ID), 10),
			strconv.FormatUint(uint64(log.UserID), 10),
			log.Username,
			log.ActorType,
			log.ActorID,
			log.RequestID,
			log.Module,
			log.Action,
			log.Method,
			log.Path,
			strconv.Itoa(log.Status),
			log.IP,
			strconv.FormatInt(log.Latency, 10),
			log.CreatedAt.Format("2006-01-02 15:04:05"),
		}
		writer.Write(row)
	}
}
