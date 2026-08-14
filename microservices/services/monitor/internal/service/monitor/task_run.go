package monitor

import (
	"context"
	"errors"
	"time"

	monitordao "github.com/go-admin-kit/services/monitor/internal/dao/monitor"
	localmodel "github.com/go-admin-kit/services/monitor/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/pagination"
	"gorm.io/gorm"
)

const (
	DefaultTaskRunWindowHours = 24
	MaxTaskRunPageSize        = 100
)

var (
	ErrInvalidTaskRunStatus = errors.New("invalid task run status")
	ErrInvalidTaskRunSource = errors.New("invalid task run source")
	ErrInvalidTaskRunWindow = errors.New("window_hours must be between 1 and 2160")
	ErrInvalidTaskRunRange  = errors.New("start_time must not be after end_time")
)

var validTaskRunStatuses = map[string]struct{}{
	localmodel.TaskRunStatusRunning: {}, localmodel.TaskRunStatusSucceeded: {},
	localmodel.TaskRunStatusFailed: {}, localmodel.TaskRunStatusCancelled: {},
}

var validTaskRunSources = map[string]struct{}{
	"worker": {}, "scheduler": {}, "ops-cron": {},
}

type TaskRunQuery struct {
	pagination.PageRequest
	Keyword string
	Service string
	Status  string
	Source  string
	StartAt *time.Time
	EndAt   *time.Time
}

type TaskRunSummary struct {
	Total         int64      `json:"total"`
	Running       int64      `json:"running"`
	Succeeded     int64      `json:"succeeded"`
	Failed        int64      `json:"failed"`
	Cancelled     int64      `json:"cancelled"`
	Services      int64      `json:"services"`
	SuccessRate   float64    `json:"success_rate"`
	AverageMS     float64    `json:"average_duration_ms"`
	LatestRunTime *time.Time `json:"latest_run_time"`
	WindowHours   int        `json:"window_hours"`
	CheckedAt     time.Time  `json:"checked_at"`
}

type taskRunReader interface {
	ListTaskRunsContext(ctx context.Context, req pagination.PageRequest, filter monitordao.TaskRunFilter) ([]localmodel.OpsTaskRun, int64, error)
	GetTaskRunByIDContext(ctx context.Context, id uint64) (*localmodel.OpsTaskRun, error)
	GetTaskRunSummaryContext(ctx context.Context, since time.Time) (monitordao.TaskRunSummaryRow, error)
}

type TaskRunService struct {
	dao taskRunReader
}

func NewTaskRunService(db *gorm.DB) *TaskRunService {
	return &TaskRunService{dao: monitordao.NewJobDAO(db)}
}

func newTaskRunService(dao taskRunReader) *TaskRunService {
	return &TaskRunService{dao: dao}
}

func (s *TaskRunService) ListContext(ctx context.Context, query TaskRunQuery) ([]localmodel.OpsTaskRun, int64, error) {
	if query.Status != "" {
		if _, ok := validTaskRunStatuses[query.Status]; !ok {
			return nil, 0, ErrInvalidTaskRunStatus
		}
	}
	if query.Source != "" {
		if _, ok := validTaskRunSources[query.Source]; !ok {
			return nil, 0, ErrInvalidTaskRunSource
		}
	}
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 10
	}
	if query.PageSize > MaxTaskRunPageSize {
		query.PageSize = MaxTaskRunPageSize
	}
	if query.StartAt != nil && query.EndAt != nil && query.StartAt.After(*query.EndAt) {
		return nil, 0, ErrInvalidTaskRunRange
	}
	return s.dao.ListTaskRunsContext(ctx, query.PageRequest, monitordao.TaskRunFilter{
		Keyword: query.Keyword, Service: query.Service, Status: query.Status, Source: query.Source,
		StartAt: query.StartAt, EndAt: query.EndAt,
	})
}

func (s *TaskRunService) GetContext(ctx context.Context, id uint64) (*localmodel.OpsTaskRun, error) {
	if id == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return s.dao.GetTaskRunByIDContext(ctx, id)
}

func (s *TaskRunService) SummaryContext(ctx context.Context, windowHours int) (*TaskRunSummary, error) {
	if windowHours == 0 {
		windowHours = DefaultTaskRunWindowHours
	}
	if windowHours < 0 || windowHours > 24*90 {
		return nil, ErrInvalidTaskRunWindow
	}
	row, err := s.dao.GetTaskRunSummaryContext(ctx, time.Now().Add(-time.Duration(windowHours)*time.Hour))
	if err != nil {
		return nil, err
	}
	completed := row.Succeeded + row.Failed + row.Cancelled
	successRate := 0.0
	if completed > 0 {
		successRate = float64(row.Succeeded) * 100 / float64(completed)
	}
	return &TaskRunSummary{
		Total: row.Total, Running: row.Running, Succeeded: row.Succeeded, Failed: row.Failed,
		Cancelled: row.Cancelled, Services: row.Services, SuccessRate: successRate,
		AverageMS: row.AverageMS, LatestRunTime: row.LatestRunTime,
		WindowHours: windowHours, CheckedAt: time.Now(),
	}, nil
}
