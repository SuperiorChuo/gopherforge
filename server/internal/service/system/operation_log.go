package system

import (
	"time"

	"github.com/go-admin-kit/server/internal/dao/system"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
)

// OperationLogService 操作日志服务
type OperationLogService struct {
	logDAO system.OperationLogDAO
}

// OperationLogListRequest 操作日志列表请求
type OperationLogListRequest struct {
	pagination.PageRequest
	UserID    *uint               `form:"user_id" json:"user_id"`
	Username  string              `form:"username" json:"username"`
	ActorType string              `form:"actor_type" json:"actor_type"`
	ActorID   string              `form:"actor_id" json:"actor_id"`
	RequestID string              `form:"request_id" json:"request_id"`
	Method    string              `form:"method" json:"method"`
	Path      string              `form:"path" json:"path"`
	Module    string              `form:"module" json:"module"`
	Action    string              `form:"action" json:"action"`
	Status    *int                `form:"status" json:"status"`
	StartTime *time.Time          `form:"start_time" time_format:"2006-01-02 15:04:05" json:"start_time"`
	EndTime   *time.Time          `form:"end_time" time_format:"2006-01-02 15:04:05" json:"end_time"`
	DataScope authz.UserDataScope `json:"-" form:"-"`
}

// ClearLogsRequest 清理日志请求
type ClearLogsRequest struct {
	Days int `json:"days" binding:"required,min=1"` // 保留最近多少天的日志
}

// Record 记录操作日志
func (s *OperationLogService) Record(log *model.OperationLog) error {
	return s.logDAO.CreateLog(log)
}

// GetLogByID 根据ID获取操作日志详情
func (s *OperationLogService) GetLogByID(id uint) (*model.OperationLog, error) {
	return s.logDAO.GetLogByID(id)
}

// GetLogList 获取操作日志列表
func (s *OperationLogService) GetLogList(req OperationLogListRequest) ([]model.OperationLog, int64, error) {
	return s.logDAO.GetLogList(
		req.PageRequest,
		req.UserID,
		req.Username,
		req.ActorType,
		req.ActorID,
		req.RequestID,
		req.Method,
		req.Path,
		req.Module,
		req.Action,
		req.Status,
		req.StartTime,
		req.EndTime,
		req.DataScope,
	)
}

// ClearLogs 清理旧日志
func (s *OperationLogService) ClearLogs(days int) (int64, error) {
	before := time.Now().AddDate(0, 0, -days)
	return s.logDAO.DeleteLogsBefore(before)
}

// GetLogStats 获取日志统计信息
func (s *OperationLogService) GetLogStats(startTime, endTime *time.Time) (*system.LogStats, error) {
	return s.logDAO.GetLogStats(startTime, endTime)
}

// ExportLogs 导出日志（返回符合条件的所有日志）
func (s *OperationLogService) ExportLogs(req OperationLogListRequest) ([]model.OperationLog, error) {
	// 导出时不分页，获取所有符合条件的日志
	req.Page = 1
	req.PageSize = 10000 // 最大导出数量限制

	logs, _, err := s.logDAO.GetLogList(
		req.PageRequest,
		req.UserID,
		req.Username,
		req.ActorType,
		req.ActorID,
		req.RequestID,
		req.Method,
		req.Path,
		req.Module,
		req.Action,
		req.Status,
		req.StartTime,
		req.EndTime,
		req.DataScope,
	)
	return logs, err
}
