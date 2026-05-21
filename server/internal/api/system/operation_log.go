package system

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/logger"
	"github.com/go-admin-kit/server/internal/pkg/response"
	"github.com/go-admin-kit/server/internal/service/system"
)

// OperationLogAPI handles operation log endpoints.
type OperationLogAPI struct {
	logService system.OperationLogService
}

// NewOperationLogAPI creates an OperationLogAPI instance.
func NewOperationLogAPI() *OperationLogAPI {
	return &OperationLogAPI{
		logService: system.OperationLogService{},
	}
}

// GetOperationLogs returns paginated operation logs.
func (a *OperationLogAPI) GetOperationLogs(c *gin.Context) {
	var req system.OperationLogListRequest
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
		logOperationLogError("failed to resolve operation log data scope", err)
		response.InternalServerError(c, "failed to get operation logs")
		return
	}
	req.DataScope = dataScope

	logs, total, err := a.logService.GetLogListContext(c.Request.Context(), req)
	if err != nil {
		logOperationLogError("failed to get operation logs", err)
		response.InternalServerError(c, "failed to get operation logs")
		return
	}

	response.PageSuccess(c, logs, total, req.Page, req.PageSize)
}

// GetOperationLogDetail returns an operation log by id.
func (a *OperationLogAPI) GetOperationLogDetail(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid log id")
		return
	}

	log, err := a.logService.GetLogByIDContext(c.Request.Context(), uint(id))
	if err != nil {
		writeSystemOperationLogServiceError(c, "failed to get operation log", err)
		return
	}

	response.Success(c, log)
}

// ClearOperationLogs deletes old operation logs.
func (a *OperationLogAPI) ClearOperationLogs(c *gin.Context) {
	var req system.ClearLogsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	count, err := a.logService.ClearLogsContext(c.Request.Context(), req.Days)
	if err != nil {
		logOperationLogError("failed to clear operation logs", err)
		response.InternalServerError(c, "failed to clear operation logs")
		return
	}

	response.SuccessWithMessage(c, fmt.Sprintf("successfully deleted %d logs", count), gin.H{
		"deleted_count": count,
	})
}

// GetOperationLogStats returns operation log statistics.
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

	// Default to the last 7 days when no range is specified.
	if startTime == nil {
		t := time.Now().AddDate(0, 0, -7)
		startTime = &t
	}
	if endTime == nil {
		t := time.Now()
		endTime = &t
	}

	stats, err := a.logService.GetLogStatsContext(c.Request.Context(), startTime, endTime)
	if err != nil {
		logOperationLogError("failed to get operation log stats", err)
		response.InternalServerError(c, "failed to get operation log stats")
		return
	}

	response.Success(c, stats)
}

// ExportOperationLogs exports operation logs as CSV.
func (a *OperationLogAPI) ExportOperationLogs(c *gin.Context) {
	var req system.OperationLogListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "invalid query parameters")
		return
	}

	dataScope, err := authz.ResolveUserDataScopeFromContext(c)
	if err != nil {
		logOperationLogError("failed to resolve operation log export data scope", err)
		response.InternalServerError(c, "failed to export operation logs")
		return
	}
	req.DataScope = dataScope

	logs, err := a.logService.ExportLogsContext(c.Request.Context(), req)
	if err != nil {
		logOperationLogError("failed to export operation logs", err)
		response.InternalServerError(c, "failed to export operation logs")
		return
	}

	// Set response headers.
	filename := fmt.Sprintf("operation_logs_%s.csv", time.Now().Format("20060102150405"))
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))

	// Write CSV content.
	writer := csv.NewWriter(c.Writer)
	defer writer.Flush()

	// Write headers.
	headers := []string{"ID", "User ID", "Username", "Actor Type", "Actor ID", "Request ID", "Module", "Action", "Method", "Path", "Status", "IP", "Latency(ms)", "Created At"}
	writer.Write(headers)

	// Write rows.
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

func logOperationLogError(message string, err error) {
	if logger.Logger == nil {
		return
	}
	logger.Error(message, logger.Err(err))
}
