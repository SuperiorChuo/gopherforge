package monitor

import (
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

// GetJobByID 根据ID获取任务
func (d *JobDAO) GetJobByID(id uint) (*model.ScheduledJob, error) {
	var job model.ScheduledJob
	result := database.DB.First(&job, id)
	return &job, result.Error
}

// GetJobList 获取任务列表（分页）
func (d *JobDAO) GetJobList(req pagination.PageRequest, name string, status *int8) ([]model.ScheduledJob, int64, error) {
	var jobs []model.ScheduledJob
	var total int64

	query := database.DB.Model(&model.ScheduledJob{})

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

// CreateJob 创建任务
func (d *JobDAO) CreateJob(job *model.ScheduledJob) error {
	return database.DB.Create(job).Error
}

// UpdateJob 更新任务
func (d *JobDAO) UpdateJob(job *model.ScheduledJob) error {
	return database.DB.Save(job).Error
}

// DeleteJob 删除任务
func (d *JobDAO) DeleteJob(id uint) error {
	return database.DB.Delete(&model.ScheduledJob{}, id).Error
}

// CreateJobLog 创建任务日志
func (d *JobDAO) CreateJobLog(log *model.ScheduledJobLog) error {
	return database.DB.Create(log).Error
}

// CleanupJobLogsBefore 清理指定时间之前的任务日志
func (d *JobDAO) CleanupJobLogsBefore(before time.Time) (int64, error) {
	result := database.DB.Where("created_at < ?", before).Delete(&model.ScheduledJobLog{})
	return result.RowsAffected, result.Error
}

// GetJobLogList 获取任务日志列表（分页）
func (d *JobDAO) GetJobLogList(req pagination.PageRequest, jobID uint, success *int8) ([]model.ScheduledJobLog, int64, error) {
	var logs []model.ScheduledJobLog
	var total int64

	query := database.DB.Model(&model.ScheduledJobLog{})

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

// GetAllActiveJobs 获取所有激活的任务
func (d *JobDAO) GetAllActiveJobs() ([]model.ScheduledJob, error) {
	var jobs []model.ScheduledJob
	result := database.DB.Where("status = ?", 1).Find(&jobs)
	return jobs, result.Error
}

// GetAllJobs 获取所有任务
func (d *JobDAO) GetAllJobs() ([]model.ScheduledJob, error) {
	var jobs []model.ScheduledJob
	result := database.DB.Order("created_at DESC").Find(&jobs)
	return jobs, result.Error
}

// CountJobsByStatus 按状态统计任务数量，status 为 nil 时统计全部
func (d *JobDAO) CountJobsByStatus(status *int8) (int64, error) {
	var count int64
	query := database.DB.Model(&model.ScheduledJob{})
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	err := query.Count(&count).Error
	return count, err
}

// CountFailedJobLogsSince 统计指定时间之后失败的任务日志数量
func (d *JobDAO) CountFailedJobLogsSince(since time.Time) (int64, error) {
	var count int64
	err := database.DB.Model(&model.ScheduledJobLog{}).
		Where("status = ? AND created_at >= ?", 0, since).
		Count(&count).Error
	return count, err
}

// GetLatestJobRunTime 获取最近一次任务运行时间
func (d *JobDAO) GetLatestJobRunTime() (*time.Time, error) {
	var job model.ScheduledJob
	err := database.DB.
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

// GetLatestJobLog 获取任务最近一条执行日志
func (d *JobDAO) GetLatestJobLog(jobID uint) (*model.ScheduledJobLog, error) {
	var log model.ScheduledJobLog
	err := database.DB.
		Where("job_id = ?", jobID).
		Order("created_at DESC").
		First(&log).Error
	return &log, err
}
