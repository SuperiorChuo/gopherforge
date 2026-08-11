package monitor

import (
	"context"
	"errors"
	"time"

	localmodel "github.com/go-admin-kit/services/monitor/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/pagination"
	"gorm.io/gorm"
)

type JobDAO struct {
	db *gorm.DB
}

func NewJobDAO(db *gorm.DB) *JobDAO {
	return &JobDAO{db: db}
}

func (d *JobDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

func (d *JobDAO) Ready() bool {
	return d != nil && d.db != nil
}

func (d *JobDAO) GetJobByIDContext(ctx context.Context, id uint) (*localmodel.ScheduledJob, error) {
	var job localmodel.ScheduledJob
	result := d.dbWithContext(ctx).First(&job, id)
	return &job, result.Error
}

func (d *JobDAO) GetJobListContext(ctx context.Context, req pagination.PageRequest, name string, status *int8) ([]localmodel.ScheduledJob, int64, error) {
	var jobs []localmodel.ScheduledJob
	var total int64

	query := d.dbWithContext(ctx).Model(&localmodel.ScheduledJob{})

	if name != "" {
		query = query.Where("name LIKE ?", "%"+name+"%")
	}

	if status != nil {
		query = query.Where("status = ?", *status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	result := query.Scopes(pagination.Paginate(req)).
		Order("created_at DESC").
		Find(&jobs)

	return jobs, total, result.Error
}

func (d *JobDAO) CreateJobContext(ctx context.Context, job *localmodel.ScheduledJob) error {
	return d.dbWithContext(ctx).Create(job).Error
}

func (d *JobDAO) UpdateJobContext(ctx context.Context, job *localmodel.ScheduledJob) error {
	return d.dbWithContext(ctx).Save(job).Error
}

func (d *JobDAO) DeleteJobContext(ctx context.Context, id uint) error {
	return d.dbWithContext(ctx).Delete(&localmodel.ScheduledJob{}, id).Error
}

func (d *JobDAO) CreateJobLogContext(ctx context.Context, log *localmodel.ScheduledJobLog) error {
	return d.dbWithContext(ctx).Create(log).Error
}

func (d *JobDAO) CleanupJobLogsBeforeContext(ctx context.Context, before time.Time) (int64, error) {
	result := d.dbWithContext(ctx).Where("created_at < ?", before).Delete(&localmodel.ScheduledJobLog{})
	return result.RowsAffected, result.Error
}

func (d *JobDAO) GetJobLogListContext(ctx context.Context, req pagination.PageRequest, jobID uint, success *int8) ([]localmodel.ScheduledJobLog, int64, error) {
	var logs []localmodel.ScheduledJobLog
	var total int64

	query := d.dbWithContext(ctx).Model(&localmodel.ScheduledJobLog{})

	if jobID > 0 {
		query = query.Where("job_id = ?", jobID)
	}

	if success != nil {
		query = query.Where("status = ?", *success)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	result := query.Scopes(pagination.Paginate(req)).
		Order("created_at DESC").
		Find(&logs)

	return logs, total, result.Error
}

func (d *JobDAO) GetAllActiveJobsContext(ctx context.Context) ([]localmodel.ScheduledJob, error) {
	var jobs []localmodel.ScheduledJob
	result := d.dbWithContext(ctx).Where("status = ?", 1).Find(&jobs)
	return jobs, result.Error
}

func (d *JobDAO) GetAllJobsContext(ctx context.Context) ([]localmodel.ScheduledJob, error) {
	var jobs []localmodel.ScheduledJob
	result := d.dbWithContext(ctx).Order("created_at DESC").Find(&jobs)
	return jobs, result.Error
}

func (d *JobDAO) CountJobsByStatusContext(ctx context.Context, status *int8) (int64, error) {
	var count int64
	query := d.dbWithContext(ctx).Model(&localmodel.ScheduledJob{})
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	err := query.Count(&count).Error
	return count, err
}

func (d *JobDAO) CountFailedJobLogsSinceContext(ctx context.Context, since time.Time) (int64, error) {
	var count int64
	err := d.dbWithContext(ctx).Model(&localmodel.ScheduledJobLog{}).
		Where("status = ? AND created_at >= ?", 0, since).
		Count(&count).Error
	return count, err
}

func (d *JobDAO) GetLatestJobRunTimeContext(ctx context.Context) (*time.Time, error) {
	var job localmodel.ScheduledJob
	err := d.dbWithContext(ctx).
		Where("last_run_time IS NOT NULL").
		Order("last_run_time DESC").
		First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return job.LastRunTime, nil
}

func (d *JobDAO) GetLatestJobLogContext(ctx context.Context, jobID uint) (*localmodel.ScheduledJobLog, error) {
	var log localmodel.ScheduledJobLog
	err := d.dbWithContext(ctx).
		Where("job_id = ?", jobID).
		Order("created_at DESC").
		First(&log).Error
	return &log, err
}

// ListHeartbeatsContext lists all distributed job heartbeats (task center;
// row count equals the number of jobs, tens at most).
func (d *JobDAO) ListHeartbeatsContext(ctx context.Context) ([]localmodel.OpsJobHeartbeat, error) {
	var list []localmodel.OpsJobHeartbeat
	err := d.dbWithContext(ctx).Order("service ASC, job_key ASC").Find(&list).Error
	return list, err
}
