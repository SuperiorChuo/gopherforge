package monitor

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/go-admin-kit/server/internal/dao/monitor"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

const (
	DefaultJobLogRetentionDays  = 30
	DefaultJobHealthWindowHours = 24
)

var (
	ErrInvalidCronExpression = errors.New("invalid cron expression")
	ErrInvalidRetentionDays  = errors.New("retention_days must be greater than 0")
)

var jobCronParser = cron.NewParser(
	cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

type jobDAO interface {
	GetJobByIDContext(ctx context.Context, id uint) (*model.ScheduledJob, error)
	GetJobListContext(ctx context.Context, req pagination.PageRequest, name string, status *int8) ([]model.ScheduledJob, int64, error)
	CreateJobContext(ctx context.Context, job *model.ScheduledJob) error
	UpdateJobContext(ctx context.Context, job *model.ScheduledJob) error
	DeleteJobContext(ctx context.Context, id uint) error
	CreateJobLogContext(ctx context.Context, log *model.ScheduledJobLog) error
	GetAllActiveJobsContext(ctx context.Context) ([]model.ScheduledJob, error)
	GetAllJobsContext(ctx context.Context) ([]model.ScheduledJob, error)
	CleanupJobLogsBeforeContext(ctx context.Context, before time.Time) (int64, error)
	CountJobsByStatusContext(ctx context.Context, status *int8) (int64, error)
	CountFailedJobLogsSinceContext(ctx context.Context, since time.Time) (int64, error)
	GetLatestJobRunTimeContext(ctx context.Context) (*time.Time, error)
	GetLatestJobLogContext(ctx context.Context, jobID uint) (*model.ScheduledJobLog, error)
}

type JobService struct {
	dao        jobDAO
	cron       *cron.Cron
	runningMap sync.Map // map[uint]cron.EntryID
}

type JobLogCleanupResult struct {
	RetentionDays int       `json:"retention_days"`
	CutoffTime    time.Time `json:"cutoff_time"`
	DeletedRows   int64     `json:"deleted_rows"`
}

type JobHealthCheck struct {
	Total        int64               `json:"total"`
	Enabled      int64               `json:"enabled"`
	Paused       int64               `json:"paused"`
	RecentFailed int64               `json:"recent_failed"`
	LastRunTime  *time.Time          `json:"last_run_time"`
	AbnormalJobs []JobAbnormalStatus `json:"abnormal_jobs"`
	WindowHours  int                 `json:"window_hours"`
	CheckedAt    time.Time           `json:"checked_at"`
}

type JobAbnormalStatus struct {
	ID                 uint       `json:"id"`
	Name               string     `json:"name"`
	GroupName          string     `json:"group_name"`
	Status             int8       `json:"status"`
	Reason             string     `json:"reason"`
	LastRunTime        *time.Time `json:"last_run_time"`
	LastFailureTime    *time.Time `json:"last_failure_time,omitempty"`
	LastFailureMessage string     `json:"last_failure_message,omitempty"`
}

var jobService *JobService
var once sync.Once

// GetJobService 单例模式获取服务实例
func GetJobService() *JobService {
	once.Do(func() {
		jobDAO := monitor.NewJobDAO()
		jobService = newJobService(jobDAO, jobDAO.Ready())
	})
	return jobService
}

func newJobService(dao jobDAO, bootstrapJobs bool) *JobService {
	service := &JobService{
		dao:  dao,
		cron: cron.New(cron.WithParser(jobCronParser)),
	}
	service.cron.Start()
	if bootstrapJobs {
		service.initJobs()
	}
	return service
}

// 初始化任务
func (s *JobService) initJobs() {
	jobs, err := s.dao.GetAllActiveJobsContext(context.Background())
	if err != nil {
		log.Printf("Failed to load jobs: %v", err)
		return
	}

	for _, job := range jobs {
		s.StartJob(job)
	}
}

// StartJob 启动任务
func (s *JobService) StartJob(job model.ScheduledJob) error {
	// 如果已经在运行，先停止
	if _, ok := s.runningMap.Load(job.ID); ok {
		s.StopJob(job.ID)
	}

	if err := validateCronExpression(job.CronExpression); err != nil {
		return err
	}

	// 定义任务函数
	cmd := func() {
		s.runTask(job)
	}

	entryID, err := s.cron.AddFunc(job.CronExpression, cmd)
	if err != nil {
		return err
	}

	s.runningMap.Store(job.ID, entryID)
	return nil
}

// StopJob 停止任务
func (s *JobService) StopJob(jobID uint) {
	if entryID, ok := s.runningMap.Load(jobID); ok {
		s.cron.Remove(entryID.(cron.EntryID))
		s.runningMap.Delete(jobID)
	}
}

// runTask 执行任务并记录日志 (供Cron和手动调用复用)
func (s *JobService) runTask(job model.ScheduledJob) {
	s.runTaskContext(context.Background(), job)
}

func (s *JobService) runTaskContext(ctx context.Context, job model.ScheduledJob) {
	startTime := time.Now()
	var status int8 = 1
	message := "Success"

	// 执行任务
	executeMessage, err := s.executeTaskContext(ctx, job.InvokeTarget)
	if err != nil {
		status = 0
		message = err.Error()
	} else if executeMessage != "" {
		message = executeMessage
	}

	duration := int(time.Since(startTime).Milliseconds())

	// 记录日志
	logEntry := model.ScheduledJobLog{
		JobID:    job.ID,
		JobName:  job.Name,
		Status:   status,
		Message:  message,
		Duration: duration,
	}
	if err := s.dao.CreateJobLogContext(ctx, &logEntry); err != nil {
		log.Printf("Failed to create job log for %s: %v", job.Name, err)
	}

	// 更新任务最后运行时间
	job.LastRunTime = &startTime
	if err := s.dao.UpdateJobContext(ctx, &job); err != nil {
		log.Printf("Failed to update last run time for %s: %v", job.Name, err)
	}
}

// executeTask 执行具体任务逻辑 (利用反射或switch case)
func (s *JobService) executeTaskContext(ctx context.Context, target string) (string, error) {
	// 这里简单演示，实际可以用反射调用注册的函数
	switch target {
	case "CleanExpiredLogs":
		result, err := s.CleanupJobLogsContext(ctx, DefaultJobLogRetentionDays)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("deleted %d job logs before %s", result.DeletedRows, result.CutoffTime.Format(time.RFC3339)), nil
	case "HealthCheck":
		health, err := s.CheckJobHealthContext(ctx, DefaultJobHealthWindowHours)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("total=%d enabled=%d paused=%d recent_failed=%d abnormal=%d",
			health.Total, health.Enabled, health.Paused, health.RecentFailed, len(health.AbnormalJobs)), nil
	default:
		return "", fmt.Errorf("unknown target: %s", target)
	}
}

// CleanupJobLogs 按保留天数清理定时任务执行日志。
func (s *JobService) CleanupJobLogs(retentionDays int) (*JobLogCleanupResult, error) {
	return s.CleanupJobLogsContext(context.Background(), retentionDays)
}

func (s *JobService) CleanupJobLogsContext(ctx context.Context, retentionDays int) (*JobLogCleanupResult, error) {
	if retentionDays <= 0 {
		return nil, ErrInvalidRetentionDays
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	deletedRows, err := s.dao.CleanupJobLogsBeforeContext(ctx, cutoff)
	if err != nil {
		return nil, err
	}

	return &JobLogCleanupResult{
		RetentionDays: retentionDays,
		CutoffTime:    cutoff,
		DeletedRows:   deletedRows,
	}, nil
}

// CheckJobHealth 汇总任务治理健康状态。
func (s *JobService) CheckJobHealth(windowHours int) (*JobHealthCheck, error) {
	return s.CheckJobHealthContext(context.Background(), windowHours)
}

func (s *JobService) CheckJobHealthContext(ctx context.Context, windowHours int) (*JobHealthCheck, error) {
	if windowHours <= 0 {
		windowHours = DefaultJobHealthWindowHours
	}

	enabledStatus := int8(1)
	pausedStatus := int8(0)
	since := time.Now().Add(-time.Duration(windowHours) * time.Hour)

	total, err := s.dao.CountJobsByStatusContext(ctx, nil)
	if err != nil {
		return nil, err
	}
	enabled, err := s.dao.CountJobsByStatusContext(ctx, &enabledStatus)
	if err != nil {
		return nil, err
	}
	paused, err := s.dao.CountJobsByStatusContext(ctx, &pausedStatus)
	if err != nil {
		return nil, err
	}
	recentFailed, err := s.dao.CountFailedJobLogsSinceContext(ctx, since)
	if err != nil {
		return nil, err
	}
	lastRunTime, err := s.dao.GetLatestJobRunTimeContext(ctx)
	if err != nil {
		return nil, err
	}

	jobs, err := s.dao.GetAllJobsContext(ctx)
	if err != nil {
		return nil, err
	}

	abnormalJobs, err := s.buildAbnormalJobsContext(ctx, jobs, since)
	if err != nil {
		return nil, err
	}

	return &JobHealthCheck{
		Total:        total,
		Enabled:      enabled,
		Paused:       paused,
		RecentFailed: recentFailed,
		LastRunTime:  lastRunTime,
		AbnormalJobs: abnormalJobs,
		WindowHours:  windowHours,
		CheckedAt:    time.Now(),
	}, nil
}

func (s *JobService) buildAbnormalJobsContext(ctx context.Context, jobs []model.ScheduledJob, since time.Time) ([]JobAbnormalStatus, error) {
	abnormalJobs := make([]JobAbnormalStatus, 0)

	for _, job := range jobs {
		reasons := make([]string, 0, 3)
		var lastFailureTime *time.Time
		var lastFailureMessage string

		if err := validateCronExpression(job.CronExpression); err != nil {
			reasons = append(reasons, "invalid cron expression")
		}

		if job.Status == 1 {
			if _, ok := s.runningMap.Load(job.ID); !ok {
				reasons = append(reasons, "enabled job is not registered in scheduler")
			}
		}

		latestLog, err := s.dao.GetLatestJobLogContext(ctx, job.ID)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		if err == nil && latestLog.Status == 0 && !latestLog.CreatedAt.Before(since) {
			reasons = append(reasons, "latest run failed within health window")
			lastFailureTime = &latestLog.CreatedAt
			lastFailureMessage = latestLog.Message
		}

		if len(reasons) == 0 {
			continue
		}

		abnormalJobs = append(abnormalJobs, JobAbnormalStatus{
			ID:                 job.ID,
			Name:               job.Name,
			GroupName:          job.GroupName,
			Status:             job.Status,
			Reason:             strings.Join(reasons, "; "),
			LastRunTime:        job.LastRunTime,
			LastFailureTime:    lastFailureTime,
			LastFailureMessage: lastFailureMessage,
		})
	}

	return abnormalJobs, nil
}

// GetJobList 获取任务列表 (保持不变)
func (s *JobService) GetJobList(req pagination.PageRequest, name string, status *int8) ([]model.ScheduledJob, int64, error) {
	return s.GetJobListContext(context.Background(), req, name, status)
}

func (s *JobService) GetJobListContext(ctx context.Context, req pagination.PageRequest, name string, status *int8) ([]model.ScheduledJob, int64, error) {
	return s.dao.GetJobListContext(ctx, req, name, status)
}

// CreateJob 创建任务
func (s *JobService) CreateJob(job *model.ScheduledJob) error {
	return s.CreateJobContext(context.Background(), job)
}

func (s *JobService) CreateJobContext(ctx context.Context, job *model.ScheduledJob) error {
	// 验证Cron表达式
	if err := validateCronExpression(job.CronExpression); err != nil {
		return err
	}

	if err := s.dao.CreateJobContext(ctx, job); err != nil {
		return err
	}
	if job.Status == 1 {
		// 如果启动失败，仅记录错误，不影响创建（或根据需求回滚）
		if err := s.StartJob(*job); err != nil {
			log.Printf("Failed to start job %s: %v", job.Name, err)
			return nil // 返回 nil 表示创建成功，但启动可能有警告
		}
	}
	return nil
}

// UpdateJob 更新任务
func (s *JobService) UpdateJob(job *model.ScheduledJob) error {
	return s.UpdateJobContext(context.Background(), job)
}

func (s *JobService) UpdateJobContext(ctx context.Context, job *model.ScheduledJob) error {
	existingJob, err := s.dao.GetJobByIDContext(ctx, job.ID)
	if err != nil {
		return err
	}

	// 验证Cron表达式
	if err := validateCronExpression(job.CronExpression); err != nil {
		return err
	}

	if job.CreatedAt.IsZero() {
		job.CreatedAt = existingJob.CreatedAt
	}
	if job.LastRunTime == nil {
		job.LastRunTime = existingJob.LastRunTime
	}
	if job.NextRunTime == nil {
		job.NextRunTime = existingJob.NextRunTime
	}

	// 先停止旧任务
	s.StopJob(job.ID)

	if err := s.dao.UpdateJobContext(ctx, job); err != nil {
		return err
	}

	// 如果状态是运行，则重新启动
	if job.Status == 1 {
		return s.StartJob(*job)
	}
	return nil
}

// StartJobByID 根据ID启动任务
func (s *JobService) StartJobByID(id uint) error {
	return s.StartJobByIDContext(context.Background(), id)
}

func (s *JobService) StartJobByIDContext(ctx context.Context, id uint) error {
	job, err := s.dao.GetJobByIDContext(ctx, id)
	if err != nil {
		return err
	}

	// 尝试添加到调度器验证表达式
	if err := validateCronExpression(job.CronExpression); err != nil {
		return err
	}

	if err := s.StartJob(*job); err != nil {
		return err
	}

	// 只有启动成功才更新数据库状态
	job.Status = 1
	return s.dao.UpdateJobContext(ctx, job)
}

// StopJobByID 根据ID停止任务
func (s *JobService) StopJobByID(id uint) error {
	return s.StopJobByIDContext(context.Background(), id)
}

func (s *JobService) StopJobByIDContext(ctx context.Context, id uint) error {
	job, err := s.dao.GetJobByIDContext(ctx, id)
	if err != nil {
		return err
	}
	s.StopJob(id)

	job.Status = 0
	return s.dao.UpdateJobContext(ctx, job)
}

// DeleteJob 删除任务
func (s *JobService) DeleteJob(id uint) error {
	return s.DeleteJobContext(context.Background(), id)
}

func (s *JobService) DeleteJobContext(ctx context.Context, id uint) error {
	if _, err := s.dao.GetJobByIDContext(ctx, id); err != nil {
		return err
	}
	s.StopJob(id)
	return s.dao.DeleteJobContext(ctx, id)
}

// RunJob 立即执行一次
func (s *JobService) RunJob(id uint) error {
	return s.RunJobContext(context.Background(), id)
}

func (s *JobService) RunJobContext(ctx context.Context, id uint) error {
	job, err := s.dao.GetJobByIDContext(ctx, id)
	if err != nil {
		return err
	}

	// 异步执行并记录日志
	go s.runTaskContext(context.WithoutCancel(ctx), *job)
	return nil
}

func validateCronExpression(expression string) error {
	if _, err := jobCronParser.Parse(expression); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCronExpression, err)
	}
	return nil
}
