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

type JobDAO struct{}

func NewJobDAO() *JobDAO {
	return &JobDAO{}
}

func (d *JobDAO) Ready() bool {
	return database.DB != nil
}

// GetJobByID returns a job by ID.
func (d *JobDAO) GetJobByID(id uint) (*model.ScheduledJob, error) {
	return d.GetJobByIDContext(context.Background(), id)
}

func (d *JobDAO) GetJobByIDContext(ctx context.Context, id uint) (*model.ScheduledJob, error) {
	var job model.ScheduledJob
	result := database.DB.WithContext(ctx).First(&job, id)
	return &job, result.Error
}

// GetJobList returns jobs with pagination.
func (d *JobDAO) GetJobList(req pagination.PageRequest, name string, status *int8) ([]model.ScheduledJob, int64, error) {
	return d.GetJobListContext(context.Background(), req, name, status)
}

func (d *JobDAO) GetJobListContext(ctx context.Context, req pagination.PageRequest, name string, status *int8) ([]model.ScheduledJob, int64, error) {
	var jobs []model.ScheduledJob
	var total int64

	query := database.DB.WithContext(ctx).Model(&model.ScheduledJob{})

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

// CreateJob creates a job.
func (d *JobDAO) CreateJob(job *model.ScheduledJob) error {
	return d.CreateJobContext(context.Background(), job)
}

func (d *JobDAO) CreateJobContext(ctx context.Context, job *model.ScheduledJob) error {
	return database.DB.WithContext(ctx).Create(job).Error
}

// UpdateJob updates a job.
func (d *JobDAO) UpdateJob(job *model.ScheduledJob) error {
	return d.UpdateJobContext(context.Background(), job)
}

func (d *JobDAO) UpdateJobContext(ctx context.Context, job *model.ScheduledJob) error {
	return database.DB.WithContext(ctx).Save(job).Error
}

// DeleteJob deletes a job.
func (d *JobDAO) DeleteJob(id uint) error {
	return d.DeleteJobContext(context.Background(), id)
}

func (d *JobDAO) DeleteJobContext(ctx context.Context, id uint) error {
	return database.DB.WithContext(ctx).Delete(&model.ScheduledJob{}, id).Error
}

// CreateJobLog creates a job log.
func (d *JobDAO) CreateJobLog(log *model.ScheduledJobLog) error {
	return d.CreateJobLogContext(context.Background(), log)
}

func (d *JobDAO) CreateJobLogContext(ctx context.Context, log *model.ScheduledJobLog) error {
	return database.DB.WithContext(ctx).Create(log).Error
}

// CleanupJobLogsBefore deletes job logs before the given time.
func (d *JobDAO) CleanupJobLogsBefore(before time.Time) (int64, error) {
	return d.CleanupJobLogsBeforeContext(context.Background(), before)
}

func (d *JobDAO) CleanupJobLogsBeforeContext(ctx context.Context, before time.Time) (int64, error) {
	result := database.DB.WithContext(ctx).Where("created_at < ?", before).Delete(&model.ScheduledJobLog{})
	return result.RowsAffected, result.Error
}

// GetJobLogList returns job logs with pagination.
func (d *JobDAO) GetJobLogList(req pagination.PageRequest, jobID uint, success *int8) ([]model.ScheduledJobLog, int64, error) {
	return d.GetJobLogListContext(context.Background(), req, jobID, success)
}

func (d *JobDAO) GetJobLogListContext(ctx context.Context, req pagination.PageRequest, jobID uint, success *int8) ([]model.ScheduledJobLog, int64, error) {
	var logs []model.ScheduledJobLog
	var total int64

	query := database.DB.WithContext(ctx).Model(&model.ScheduledJobLog{})

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

// GetAllActiveJobs returns all active jobs.
func (d *JobDAO) GetAllActiveJobs() ([]model.ScheduledJob, error) {
	return d.GetAllActiveJobsContext(context.Background())
}

func (d *JobDAO) GetAllActiveJobsContext(ctx context.Context) ([]model.ScheduledJob, error) {
	var jobs []model.ScheduledJob
	result := database.DB.WithContext(ctx).Where("status = ?", 1).Find(&jobs)
	return jobs, result.Error
}

// GetAllJobs returns all jobs.
func (d *JobDAO) GetAllJobs() ([]model.ScheduledJob, error) {
	return d.GetAllJobsContext(context.Background())
}

func (d *JobDAO) GetAllJobsContext(ctx context.Context) ([]model.ScheduledJob, error) {
	var jobs []model.ScheduledJob
	result := database.DB.WithContext(ctx).Order("created_at DESC").Find(&jobs)
	return jobs, result.Error
}

// CountJobsByStatus counts jobs by status, or all jobs when status is nil.
func (d *JobDAO) CountJobsByStatus(status *int8) (int64, error) {
	return d.CountJobsByStatusContext(context.Background(), status)
}

func (d *JobDAO) CountJobsByStatusContext(ctx context.Context, status *int8) (int64, error) {
	var count int64
	query := database.DB.WithContext(ctx).Model(&model.ScheduledJob{})
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	err := query.Count(&count).Error
	return count, err
}

// CountFailedJobLogsSince counts failed job logs since the given time.
func (d *JobDAO) CountFailedJobLogsSince(since time.Time) (int64, error) {
	return d.CountFailedJobLogsSinceContext(context.Background(), since)
}

func (d *JobDAO) CountFailedJobLogsSinceContext(ctx context.Context, since time.Time) (int64, error) {
	var count int64
	err := database.DB.WithContext(ctx).Model(&model.ScheduledJobLog{}).
		Where("status = ? AND created_at >= ?", 0, since).
		Count(&count).Error
	return count, err
}

// GetLatestJobRunTime returns the latest job run time.
func (d *JobDAO) GetLatestJobRunTime() (*time.Time, error) {
	return d.GetLatestJobRunTimeContext(context.Background())
}

func (d *JobDAO) GetLatestJobRunTimeContext(ctx context.Context) (*time.Time, error) {
	var job model.ScheduledJob
	err := database.DB.WithContext(ctx).
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

// GetLatestJobLog returns the latest execution log for a job.
func (d *JobDAO) GetLatestJobLog(jobID uint) (*model.ScheduledJobLog, error) {
	return d.GetLatestJobLogContext(context.Background(), jobID)
}

func (d *JobDAO) GetLatestJobLogContext(ctx context.Context, jobID uint) (*model.ScheduledJobLog, error) {
	var log model.ScheduledJobLog
	err := database.DB.WithContext(ctx).
		Where("job_id = ?", jobID).
		Order("created_at DESC").
		First(&log).Error
	return &log, err
}
