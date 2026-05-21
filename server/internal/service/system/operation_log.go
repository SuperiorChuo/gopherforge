package system

import (
	"context"
	"errors"
	"time"

	systemdao "github.com/go-admin-kit/server/internal/dao/system"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
	"gorm.io/gorm"
)

type OperationLogService struct {
	logDAO systemdao.OperationLogDAO
}

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

type ClearLogsRequest struct {
	Days int `json:"days" binding:"required,min=1"`
}

var ErrOperationLogNotFound = errors.New("operation log not found")

// Deprecated: use RecordContext instead.
func (s *OperationLogService) Record(log *model.OperationLog) error {
	return s.RecordContext(context.Background(), log)
}

func (s *OperationLogService) RecordContext(ctx context.Context, log *model.OperationLog) error {
	return s.logDAO.CreateLogContext(ctx, log)
}

// Deprecated: use GetLogByIDContext instead.
func (s *OperationLogService) GetLogByID(id uint) (*model.OperationLog, error) {
	return s.GetLogByIDContext(context.Background(), id)
}

func (s *OperationLogService) GetLogByIDContext(ctx context.Context, id uint) (*model.OperationLog, error) {
	log, err := s.logDAO.GetLogByIDContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOperationLogNotFound
		}
		return nil, err
	}
	return log, nil
}

// Deprecated: use GetLogListContext instead.
func (s *OperationLogService) GetLogList(req OperationLogListRequest) ([]model.OperationLog, int64, error) {
	return s.GetLogListContext(context.Background(), req)
}

func (s *OperationLogService) GetLogListContext(ctx context.Context, req OperationLogListRequest) ([]model.OperationLog, int64, error) {
	return s.logDAO.GetLogListContext(
		ctx,
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

// Deprecated: use ClearLogsContext instead.
func (s *OperationLogService) ClearLogs(days int) (int64, error) {
	return s.ClearLogsContext(context.Background(), days)
}

func (s *OperationLogService) ClearLogsContext(ctx context.Context, days int) (int64, error) {
	before := time.Now().AddDate(0, 0, -days)
	return s.logDAO.DeleteLogsBeforeContext(ctx, before)
}

// Deprecated: use GetLogStatsContext instead.
func (s *OperationLogService) GetLogStats(startTime, endTime *time.Time) (*systemdao.LogStats, error) {
	return s.GetLogStatsContext(context.Background(), startTime, endTime)
}

func (s *OperationLogService) GetLogStatsContext(ctx context.Context, startTime, endTime *time.Time) (*systemdao.LogStats, error) {
	return s.logDAO.GetLogStatsContext(ctx, startTime, endTime)
}

// Deprecated: use ExportLogsContext instead.
func (s *OperationLogService) ExportLogs(req OperationLogListRequest) ([]model.OperationLog, error) {
	return s.ExportLogsContext(context.Background(), req)
}

func (s *OperationLogService) ExportLogsContext(ctx context.Context, req OperationLogListRequest) ([]model.OperationLog, error) {
	req.Page = 1
	req.PageSize = 10000
	logs, _, err := s.GetLogListContext(ctx, req)
	return logs, err
}
