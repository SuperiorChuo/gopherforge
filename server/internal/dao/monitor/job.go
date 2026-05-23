package monitor

import (
	"context"
	"errors"
	"time"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
	"gorm.io/gorm"
)

type JobDAO struct {
	db *gorm.DB
}

func NewJobDAO(dbs ...*gorm.DB) *JobDAO {
	db := database.DB
	if len(dbs) > 0 {
		db = dbs[0]
	}
	return &JobDAO{db: db}
}

func (d *JobDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	if d != nil && d.db != nil {
		return d.db.WithContext(ctx)
	}
	return database.DB.WithContext(ctx)
}

func (d *JobDAO) Ready() bool {
	return (d != nil && d.db != nil) || database.DB != nil
}

func (d *JobDAO) GetJobByIDContext(ctx context.Context, id uint) (*model.ScheduledJob, error) {
	var job model.ScheduledJob
	result := d.dbWithContext(ctx).First(&job, id)
	return &job, result.Error
}

func (d *JobDAO) GetJobListContext(ctx context.Context, req pagination.PageRequest, name string, status *int8) ([]model.ScheduledJob, int64, error) {
	var jobs []model.ScheduledJob
	var total int64

	query := d.dbWithContext(ctx).Model(&model.ScheduledJob{})

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

func (d *JobDAO) CreateJobContext(ctx context.Context, job *model.ScheduledJob) error {
	return d.dbWithContext(ctx).Create(job).Error
}

func (d *JobDAO) UpdateJobContext(ctx context.Context, job *model.ScheduledJob) error {
	return d.dbWithContext(ctx).Save(job).Error
}

func (d *JobDAO) DeleteJobContext(ctx context.Context, id uint) error {
	return d.dbWithContext(ctx).Delete(&model.ScheduledJob{}, id).Error
}

func (d *JobDAO) CreateJobLogContext(ctx context.Context, log *model.ScheduledJobLog) error {
	return d.dbWithContext(ctx).Create(log).Error
}

func (d *JobDAO) CleanupJobLogsBeforeContext(ctx context.Context, before time.Time) (int64, error) {
	result := d.dbWithContext(ctx).Where("created_at < ?", before).Delete(&model.ScheduledJobLog{})
	return result.RowsAffected, result.Error
}

func (d *JobDAO) GetJobLogListContext(ctx context.Context, req pagination.PageRequest, jobID uint, success *int8) ([]model.ScheduledJobLog, int64, error) {
	var logs []model.ScheduledJobLog
	var total int64

	query := d.dbWithContext(ctx).Model(&model.ScheduledJobLog{})

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

func (d *JobDAO) GetAllActiveJobsContext(ctx context.Context) ([]model.ScheduledJob, error) {
	var jobs []model.ScheduledJob
	result := d.dbWithContext(ctx).Where("status = ?", 1).Find(&jobs)
	return jobs, result.Error
}

func (d *JobDAO) GetAllJobsContext(ctx context.Context) ([]model.ScheduledJob, error) {
	var jobs []model.ScheduledJob
	result := d.dbWithContext(ctx).Order("created_at DESC").Find(&jobs)
	return jobs, result.Error
}

func (d *JobDAO) CountJobsByStatusContext(ctx context.Context, status *int8) (int64, error) {
	var count int64
	query := d.dbWithContext(ctx).Model(&model.ScheduledJob{})
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	err := query.Count(&count).Error
	return count, err
}

func (d *JobDAO) CountFailedJobLogsSinceContext(ctx context.Context, since time.Time) (int64, error) {
	var count int64
	err := d.dbWithContext(ctx).Model(&model.ScheduledJobLog{}).
		Where("status = ? AND created_at >= ?", 0, since).
		Count(&count).Error
	return count, err
}

func (d *JobDAO) GetLatestJobRunTimeContext(ctx context.Context) (*time.Time, error) {
	var job model.ScheduledJob
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

func (d *JobDAO) GetLatestJobLogContext(ctx context.Context, jobID uint) (*model.ScheduledJobLog, error) {
	var log model.ScheduledJobLog
	err := d.dbWithContext(ctx).
		Where("job_id = ?", jobID).
		Order("created_at DESC").
		First(&log).Error
	return &log, err
}
