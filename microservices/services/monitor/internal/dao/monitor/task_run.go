package monitor

import (
	"context"
	"errors"
	"strings"
	"time"

	localmodel "github.com/go-admin-kit/services/monitor/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/pagination"
	"gorm.io/gorm"
)

type TaskRunFilter struct {
	Keyword string
	Service string
	Status  string
	Source  string
	StartAt *time.Time
	EndAt   *time.Time
}

type TaskRunSummaryRow struct {
	Total         int64      `gorm:"column:total"`
	Running       int64      `gorm:"column:running"`
	Succeeded     int64      `gorm:"column:succeeded"`
	Failed        int64      `gorm:"column:failed"`
	Canceled      int64      `gorm:"column:cancelled"`
	Services      int64      `gorm:"column:services"`
	AverageMS     float64    `gorm:"column:average_ms"`
	LatestRunTime *time.Time `gorm:"column:latest_run_time"`
}

func (d *JobDAO) ListTaskRunsContext(ctx context.Context, req pagination.PageRequest, filter TaskRunFilter) ([]localmodel.OpsTaskRun, int64, error) {
	var list []localmodel.OpsTaskRun
	var total int64
	query := d.dbWithContext(ctx).Model(&localmodel.OpsTaskRun{})
	if keyword := strings.ToLower(strings.TrimSpace(filter.Keyword)); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("LOWER(task_key) LIKE ? OR LOWER(description) LIKE ?", like, like)
	}
	if filter.Service != "" {
		query = query.Where("service = ?", filter.Service)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.Source != "" {
		query = query.Where("source = ?", filter.Source)
	}
	if filter.StartAt != nil {
		query = query.Where("started_at >= ?", *filter.StartAt)
	}
	if filter.EndAt != nil {
		query = query.Where("started_at <= ?", *filter.EndAt)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Scopes(pagination.Paginate(req)).Order("started_at DESC, id DESC").Find(&list).Error
	return list, total, err
}

func (d *JobDAO) GetTaskRunByIDContext(ctx context.Context, id uint64) (*localmodel.OpsTaskRun, error) {
	var run localmodel.OpsTaskRun
	err := d.dbWithContext(ctx).First(&run, id).Error
	return &run, err
}

func (d *JobDAO) GetTaskRunSummaryContext(ctx context.Context, since time.Time) (TaskRunSummaryRow, error) {
	var summary TaskRunSummaryRow
	err := d.dbWithContext(ctx).Model(&localmodel.OpsTaskRun{}).
		Select(`COUNT(*) AS total,
COALESCE(SUM(CASE WHEN status = 'running' THEN 1 ELSE 0 END), 0) AS running,
COALESCE(SUM(CASE WHEN status = 'succeeded' THEN 1 ELSE 0 END), 0) AS succeeded,
COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0) AS failed,
COALESCE(SUM(CASE WHEN status = 'cancelled' THEN 1 ELSE 0 END), 0) AS cancelled,
COUNT(DISTINCT service) AS services,
COALESCE(AVG(CASE WHEN status <> 'running' THEN duration_ms END), 0) AS average_ms`).
		Where("started_at >= ?", since).
		Scan(&summary).Error
	if err != nil {
		return summary, err
	}
	var latest localmodel.OpsTaskRun
	err = d.dbWithContext(ctx).Select("started_at").
		Where("started_at >= ?", since).
		Order("started_at DESC, id DESC").
		First(&latest).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return summary, nil
	}
	if err != nil {
		return summary, err
	}
	summary.LatestRunTime = &latest.StartedAt
	return summary, nil
}

func (d *JobDAO) StartTaskRunContext(ctx context.Context, run *localmodel.OpsTaskRun) error {
	return d.dbWithContext(ctx).Create(run).Error
}

func (d *JobDAO) FinishTaskRunContext(ctx context.Context, runID, status, message, errorMessage string, finishedAt time.Time, durationMS int64) error {
	result := d.dbWithContext(ctx).Model(&localmodel.OpsTaskRun{}).
		Where("run_id = ? AND status = ?", runID, localmodel.TaskRunStatusRunning).
		Updates(map[string]any{
			"status": status, "message": message, "error_message": errorMessage,
			"finished_at": finishedAt, "duration_ms": durationMS,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (d *JobDAO) CleanupTaskRunsBeforeContext(ctx context.Context, before time.Time) (int64, error) {
	result := d.dbWithContext(ctx).Where("started_at < ?", before).Delete(&localmodel.OpsTaskRun{})
	return result.RowsAffected, result.Error
}
