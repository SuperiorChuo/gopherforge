package monitor

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-admin-kit/services/monitor/internal/dao/monitor"
	localmodel "github.com/go-admin-kit/services/monitor/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/pagination"
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
	ErrUnknownInvokeTarget   = errors.New("unknown invoke target")
)

// JobTarget describes a built-in schedulable target for the console dropdown.
type JobTarget struct {
	Target      string `json:"target"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// jobTargetHandler runs a built-in target and returns the execution-log message.
type jobTargetHandler func(context.Context, *JobService) (string, error)

// jobTargets is the single source of truth for schedulable targets: both the
// executor's dispatch and the console dropdown read it. Dispatch used to be a
// hardcoded switch while the console offered a free-text field, with nothing
// tying them together — an unknown target persisted fine and only surfaced as
// "unknown target" in the execution log on the next trigger.
var jobTargets = map[string]struct {
	meta    JobTarget
	execute jobTargetHandler
}{
	"CleanExpiredLogs": {
		// Title/Description stay English here: this package is guarded by
		// TestMonitorServiceUsesEnglishSourceText. The console maps these
		// target ids to localized labels and falls back to the id.
		meta: JobTarget{
			Target:      "CleanExpiredLogs",
			Title:       "Clean expired job logs",
			Description: fmt.Sprintf("Delete scheduled-job execution logs older than %d days", DefaultJobLogRetentionDays),
		},
		execute: func(ctx context.Context, s *JobService) (string, error) {
			result, err := s.CleanupJobLogsContext(ctx, DefaultJobLogRetentionDays)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("deleted %d job logs before %s", result.DeletedRows, result.CutoffTime.Format(time.RFC3339)), nil
		},
	},
	"HealthCheck": {
		meta: JobTarget{
			Target:      "HealthCheck",
			Title:       "Scheduler health check",
			Description: fmt.Sprintf("Summarize job runs and failures over the last %d hours", DefaultJobHealthWindowHours),
		},
		execute: func(ctx context.Context, s *JobService) (string, error) {
			health, err := s.CheckJobHealthContext(ctx, DefaultJobHealthWindowHours)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("total=%d enabled=%d paused=%d recent_failed=%d abnormal=%d",
				health.Total, health.Enabled, health.Paused, health.RecentFailed, len(health.AbnormalJobs)), nil
		},
	},
}

// ListJobTargets returns the built-in targets sorted by id, for the console
// dropdown.
func ListJobTargets() []JobTarget {
	targets := make([]JobTarget, 0, len(jobTargets))
	for _, entry := range jobTargets {
		targets = append(targets, entry.meta)
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].Target < targets[j].Target })
	return targets
}

// validateInvokeTarget rejects unknown targets at write time instead of
// letting them fail silently on the next trigger.
func validateInvokeTarget(target string) error {
	if _, ok := jobTargets[strings.TrimSpace(target)]; !ok {
		return fmt.Errorf("%w: %s", ErrUnknownInvokeTarget, target)
	}
	return nil
}

var jobCronParser = cron.NewParser(
	cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

type jobDAO interface {
	GetJobByIDContext(ctx context.Context, id uint) (*localmodel.ScheduledJob, error)
	GetJobListContext(ctx context.Context, req pagination.PageRequest, name string, status *int8) ([]localmodel.ScheduledJob, int64, error)
	CreateJobContext(ctx context.Context, job *localmodel.ScheduledJob) error
	UpdateJobContext(ctx context.Context, job *localmodel.ScheduledJob) error
	DeleteJobContext(ctx context.Context, id uint) error
	CreateJobLogContext(ctx context.Context, log *localmodel.ScheduledJobLog) error
	GetAllActiveJobsContext(ctx context.Context) ([]localmodel.ScheduledJob, error)
	GetAllJobsContext(ctx context.Context) ([]localmodel.ScheduledJob, error)
	CleanupJobLogsBeforeContext(ctx context.Context, before time.Time) (int64, error)
	CountJobsByStatusContext(ctx context.Context, status *int8) (int64, error)
	CountFailedJobLogsSinceContext(ctx context.Context, since time.Time) (int64, error)
	GetLatestJobRunTimeContext(ctx context.Context) (*time.Time, error)
	GetLatestJobLogContext(ctx context.Context, jobID uint) (*localmodel.ScheduledJobLog, error)
	GetJobLogListContext(ctx context.Context, req pagination.PageRequest, jobID uint, success *int8) ([]localmodel.ScheduledJobLog, int64, error)
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

// GetJobService returns the singleton job service instance, or nil if it has
// not been initialized via InitJobService.
func GetJobService() *JobService {
	return jobService
}

// InitJobService initializes the singleton job service with an injected
// database handle. It must run before the first GetJobService call to take
// effect; later calls return the already-initialized singleton.
func InitJobService(db *gorm.DB) *JobService {
	once.Do(func() {
		jobDAO := monitor.NewJobDAO(db)
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

// Stop stops the scheduler and returns a context that is done after running jobs finish.
func (s *JobService) Stop() context.Context {
	if s == nil || s.cron == nil {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx
	}

	s.runningMap.Range(func(jobID, entryID any) bool {
		s.cron.Remove(entryID.(cron.EntryID))
		s.runningMap.Delete(jobID)
		return true
	})

	return s.cron.Stop()
}

// Shutdown is an alias for Stop for service lifecycle integration.
func (s *JobService) Shutdown() context.Context {
	return s.Stop()
}

// initJobs loads active jobs into the scheduler.
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

// StartJob starts a scheduled job.
func (s *JobService) StartJob(job localmodel.ScheduledJob) error {
	// Stop an existing schedule before registering the new one.
	if _, ok := s.runningMap.Load(job.ID); ok {
		s.StopJob(job.ID)
	}

	if err := validateCronExpression(job.CronExpression); err != nil {
		return err
	}

	// Define the scheduled task function.
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

// StopJob stops a scheduled job.
func (s *JobService) StopJob(jobID uint) {
	if entryID, ok := s.runningMap.Load(jobID); ok {
		s.cron.Remove(entryID.(cron.EntryID))
		s.runningMap.Delete(jobID)
	}
}

// runTask executes a job and records a log entry for cron and manual runs.
func (s *JobService) runTask(job localmodel.ScheduledJob) {
	s.runTaskContext(context.Background(), job)
}

func (s *JobService) runTaskContext(ctx context.Context, job localmodel.ScheduledJob) {
	startTime := time.Now()
	var status int8 = 1
	message := "Success"

	// Execute the task.
	executeMessage, err := s.executeTaskContext(ctx, job.InvokeTarget)
	if err != nil {
		status = 0
		message = err.Error()
	} else if executeMessage != "" {
		message = executeMessage
	}

	duration := int(time.Since(startTime).Milliseconds())

	// Record the job log.
	logEntry := localmodel.ScheduledJobLog{
		JobID:    job.ID,
		JobName:  job.Name,
		Status:   status,
		Message:  message,
		Duration: duration,
	}
	if err := s.dao.CreateJobLogContext(ctx, &logEntry); err != nil {
		log.Printf("Failed to create job log for %s: %v", job.Name, err)
	}

	// Update the job's last run time.
	job.LastRunTime = &startTime
	if err := s.dao.UpdateJobContext(ctx, &job); err != nil {
		log.Printf("Failed to update last run time for %s: %v", job.Name, err)
	}
}

// executeTaskContext executes a specific job target.
func (s *JobService) executeTaskContext(ctx context.Context, target string) (string, error) {
	entry, ok := jobTargets[strings.TrimSpace(target)]
	if !ok {
		// Rows predating the write-time check may still hold an unlisted
		// target; report it into the execution log rather than skipping.
		return "", fmt.Errorf("%w: %s", ErrUnknownInvokeTarget, target)
	}
	return entry.execute(ctx, s)
}

// GetJobLogListContext pages through scheduled-job execution logs. jobID=0
// means no job filter; a nil success means no status filter.
func (s *JobService) GetJobLogListContext(ctx context.Context, req pagination.PageRequest, jobID uint, success *int8) ([]localmodel.ScheduledJobLog, int64, error) {
	return s.dao.GetJobLogListContext(ctx, req, jobID, success)
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

func (s *JobService) buildAbnormalJobsContext(ctx context.Context, jobs []localmodel.ScheduledJob, since time.Time) ([]JobAbnormalStatus, error) {
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

func (s *JobService) GetJobListContext(ctx context.Context, req pagination.PageRequest, name string, status *int8) ([]localmodel.ScheduledJob, int64, error) {
	return s.dao.GetJobListContext(ctx, req, name, status)
}

func (s *JobService) CreateJobContext(ctx context.Context, job *localmodel.ScheduledJob) error {
	// Validate the cron expression.
	if err := validateCronExpression(job.CronExpression); err != nil {
		return err
	}
	if err := validateInvokeTarget(job.InvokeTarget); err != nil {
		return err
	}

	if err := s.dao.CreateJobContext(ctx, job); err != nil {
		return err
	}
	if job.Status == 1 {
		// Keep the persisted job even if scheduler registration fails.
		if err := s.StartJob(*job); err != nil {
			log.Printf("Failed to start job %s: %v", job.Name, err)
			return nil // Creation succeeded, but startup may have a warning.
		}
	}
	return nil
}

func (s *JobService) UpdateJobContext(ctx context.Context, job *localmodel.ScheduledJob) error {
	existingJob, err := s.dao.GetJobByIDContext(ctx, job.ID)
	if err != nil {
		return err
	}

	// Validate the cron expression.
	if err := validateCronExpression(job.CronExpression); err != nil {
		return err
	}
	if err := validateInvokeTarget(job.InvokeTarget); err != nil {
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

	// Stop the old schedule first.
	s.StopJob(job.ID)

	if err := s.dao.UpdateJobContext(ctx, job); err != nil {
		return err
	}

	// Restart when the job should be running.
	if job.Status == 1 {
		return s.StartJob(*job)
	}
	return nil
}

func (s *JobService) StartJobByIDContext(ctx context.Context, id uint) error {
	job, err := s.dao.GetJobByIDContext(ctx, id)
	if err != nil {
		return err
	}

	// Validate the expression before registering it in the scheduler.
	if err := validateCronExpression(job.CronExpression); err != nil {
		return err
	}

	if err := s.StartJob(*job); err != nil {
		return err
	}

	// Update database status only after successful startup.
	job.Status = 1
	return s.dao.UpdateJobContext(ctx, job)
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

func (s *JobService) DeleteJobContext(ctx context.Context, id uint) error {
	if _, err := s.dao.GetJobByIDContext(ctx, id); err != nil {
		return err
	}
	s.StopJob(id)
	return s.dao.DeleteJobContext(ctx, id)
}

func (s *JobService) RunJobContext(ctx context.Context, id uint) error {
	job, err := s.dao.GetJobByIDContext(ctx, id)
	if err != nil {
		return err
	}

	// Execute asynchronously and record the job log.
	go s.runTaskContext(context.WithoutCancel(ctx), *job)
	return nil
}

func validateCronExpression(expression string) error {
	if _, err := jobCronParser.Parse(expression); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCronExpression, err)
	}
	return nil
}

// ---- distributed job heartbeats (task center M1) ----

// JobHeartbeatView is a heartbeat row plus the aggregation-side staleness flag.
type JobHeartbeatView struct {
	localmodel.OpsJobHeartbeat
	// Stale means the job missed its schedule: interval_sec > 0 and
	// now - last_run_at > 2*interval (one-cycle jitter tolerated).
	Stale bool `json:"stale"`
}

// heartbeatLister is an optional dao capability (fake daos compile without it
// and the service degrades to an empty list).
type heartbeatLister interface {
	ListHeartbeatsContext(ctx context.Context) ([]localmodel.OpsJobHeartbeat, error)
}

// ListJobHeartbeatsContext lists distributed job heartbeats with staleness
// computed. Returns an empty list when the dao lacks the capability (test
// fakes, no-DB degradation).
func (s *JobService) ListJobHeartbeatsContext(ctx context.Context) ([]JobHeartbeatView, error) {
	hl, ok := s.dao.(heartbeatLister)
	if !ok {
		return []JobHeartbeatView{}, nil
	}
	rows, err := hl.ListHeartbeatsContext(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	out := make([]JobHeartbeatView, 0, len(rows))
	for _, r := range rows {
		v := JobHeartbeatView{OpsJobHeartbeat: r}
		if r.IntervalSec > 0 && now.Sub(r.LastRunAt) > 2*time.Duration(r.IntervalSec)*time.Second {
			v.Stale = true
		}
		out = append(out, v)
	}
	return out, nil
}
