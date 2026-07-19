package monitor

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

type jobContextTestKey struct{}

func TestJobServiceGetJobListContextPropagatesContext(t *testing.T) {
	dao := &fakeJobDAO{}
	service := &JobService{dao: dao}
	ctx := context.WithValue(context.Background(), jobContextTestKey{}, "request-context")

	_, _, err := service.GetJobListContext(ctx, pagination.PageRequest{}, "", nil)
	if err != nil {
		t.Fatalf("GetJobListContext() error = %v", err)
	}
	if dao.contextMarker != "request-context" {
		t.Fatalf("context marker = %#v, want request-context", dao.contextMarker)
	}
}

func TestCleanupJobLogsDeletesOlderThanRetention(t *testing.T) {
	dao := &fakeJobDAO{cleanupRows: 3}
	service := &JobService{dao: dao}

	startCutoff := time.Now().AddDate(0, 0, -7)
	result, err := service.CleanupJobLogsContext(context.Background(), 7)
	endCutoff := time.Now().AddDate(0, 0, -7)
	if err != nil {
		t.Fatalf("cleanup job logs: %v", err)
	}

	if result.RetentionDays != 7 {
		t.Fatalf("retention days = %d, want 7", result.RetentionDays)
	}
	if result.DeletedRows != 3 {
		t.Fatalf("deleted rows = %d, want 3", result.DeletedRows)
	}
	if dao.cleanupBefore.Before(startCutoff.Add(-time.Second)) || dao.cleanupBefore.After(endCutoff.Add(time.Second)) {
		t.Fatalf("cleanup cutoff = %s, want between %s and %s", dao.cleanupBefore, startCutoff, endCutoff)
	}
	if !result.CutoffTime.Equal(dao.cleanupBefore) {
		t.Fatalf("result cutoff = %s, dao cutoff = %s", result.CutoffTime, dao.cleanupBefore)
	}
}

func TestCleanupJobLogsRejectsInvalidRetention(t *testing.T) {
	service := &JobService{dao: &fakeJobDAO{}}

	_, err := service.CleanupJobLogsContext(context.Background(), 0)
	if !errors.Is(err, ErrInvalidRetentionDays) {
		t.Fatalf("cleanup error = %v, want ErrInvalidRetentionDays", err)
	}
}

func TestCheckJobHealthReportsAbnormalJobs(t *testing.T) {
	now := time.Now()
	lastRun := now.Add(-10 * time.Minute)
	olderRun := now.Add(-2 * time.Hour)

	dao := &fakeJobDAO{
		jobs: []model.ScheduledJob{
			{
				ID:             1,
				Name:           "healthy",
				GroupName:      "system",
				CronExpression: "0 */5 * * * ?",
				Status:         1,
				LastRunTime:    &lastRun,
			},
			{
				ID:             2,
				Name:           "missing-schedule",
				GroupName:      "system",
				CronExpression: "0 */10 * * * ?",
				Status:         1,
				LastRunTime:    &olderRun,
			},
			{
				ID:             3,
				Name:           "bad-cron",
				GroupName:      "system",
				CronExpression: "bad cron",
				Status:         0,
			},
			{
				ID:             4,
				Name:           "recent-failure",
				GroupName:      "system",
				CronExpression: "0 */15 * * * ?",
				Status:         1,
				LastRunTime:    &olderRun,
			},
		},
		logs: map[uint][]model.ScheduledJobLog{
			1: {
				{JobID: 1, Status: 1, CreatedAt: now.Add(-30 * time.Minute), Message: "ok"},
			},
			4: {
				{JobID: 4, Status: 0, CreatedAt: now.Add(-20 * time.Minute), Message: "boom"},
			},
		},
	}

	service := &JobService{dao: dao, cron: cron.New(cron.WithParser(jobCronParser))}
	service.runningMap.Store(uint(1), cron.EntryID(1))
	service.runningMap.Store(uint(4), cron.EntryID(4))

	health, err := service.CheckJobHealthContext(context.Background(), 24)
	if err != nil {
		t.Fatalf("check job health: %v", err)
	}

	if health.Total != 4 || health.Enabled != 3 || health.Paused != 1 {
		t.Fatalf("counts = total:%d enabled:%d paused:%d, want 4/3/1", health.Total, health.Enabled, health.Paused)
	}
	if health.RecentFailed != 1 {
		t.Fatalf("recent failed = %d, want 1", health.RecentFailed)
	}
	if health.LastRunTime == nil || !health.LastRunTime.Equal(lastRun) {
		t.Fatalf("last run = %v, want %v", health.LastRunTime, lastRun)
	}

	reasonsByID := map[uint]string{}
	for _, abnormalJob := range health.AbnormalJobs {
		reasonsByID[abnormalJob.ID] = abnormalJob.Reason
	}
	if gotIDs := sortedKeys(reasonsByID); !slices.Equal(gotIDs, []uint{2, 3, 4}) {
		t.Fatalf("abnormal job ids = %#v, want []uint{2, 3, 4}", gotIDs)
	}
	assertReasonContains(t, reasonsByID[2], "not registered")
	assertReasonContains(t, reasonsByID[3], "invalid cron")
	assertReasonContains(t, reasonsByID[4], "failed")
}

func TestNewJobServiceCanSkipActiveJobBootstrap(t *testing.T) {
	dao := &fakeJobDAO{panicOnActiveJobs: true}

	service := newJobService(dao, false)
	defer service.Stop()

	if service.dao != dao {
		t.Fatal("newJobService did not keep injected dao")
	}
}

func TestJobServiceStopClearsScheduledJobsAndIsIdempotent(t *testing.T) {
	service := newJobService(&fakeJobDAO{}, false)

	job := model.ScheduledJob{
		ID:             10,
		Name:           "cleanup",
		CronExpression: "0 * * * * *",
		InvokeTarget:   "CleanExpiredLogs",
		Status:         1,
	}
	if err := service.StartJob(job); err != nil {
		t.Fatalf("StartJob() error = %v", err)
	}
	if len(service.cron.Entries()) != 1 {
		t.Fatalf("cron entries = %d, want 1", len(service.cron.Entries()))
	}

	ctx := service.Stop()
	<-ctx.Done()
	ctx = service.Shutdown()
	<-ctx.Done()

	if _, ok := service.runningMap.Load(job.ID); ok {
		t.Fatal("runningMap still has stopped job")
	}
	if len(service.cron.Entries()) != 0 {
		t.Fatalf("cron entries after Stop = %d, want 0", len(service.cron.Entries()))
	}
}

func TestJobServiceStopWaitsForRunningJobs(t *testing.T) {
	cleanupStarted := make(chan struct{}, 1)
	cleanupContinue := make(chan struct{})
	dao := &fakeJobDAO{
		cleanupStarted:  cleanupStarted,
		cleanupContinue: cleanupContinue,
	}
	service := newJobService(dao, false)

	job := model.ScheduledJob{
		ID:             11,
		Name:           "blocking-cleanup",
		CronExpression: "* * * * * *",
		InvokeTarget:   "CleanExpiredLogs",
		Status:         1,
	}
	if err := service.StartJob(job); err != nil {
		t.Fatalf("StartJob() error = %v", err)
	}

	select {
	case <-cleanupStarted:
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("scheduled job did not start")
	}

	ctx := service.Stop()
	select {
	case <-ctx.Done():
		t.Fatal("Stop context completed before running job finished")
	case <-time.After(50 * time.Millisecond):
	}

	close(cleanupContinue)
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("Stop context did not complete after running job finished")
	}

	if _, ok := service.runningMap.Load(job.ID); ok {
		t.Fatal("runningMap still has stopped job")
	}
}

func assertReasonContains(t *testing.T, reason, want string) {
	t.Helper()
	if !strings.Contains(reason, want) {
		t.Fatalf("reason %q does not contain %q", reason, want)
	}
}

func sortedKeys(values map[uint]string) []uint {
	keys := make([]uint, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

type fakeJobDAO struct {
	jobs              []model.ScheduledJob
	logs              map[uint][]model.ScheduledJobLog
	cleanupBefore     time.Time
	cleanupRows       int64
	cleanupStarted    chan struct{}
	cleanupContinue   chan struct{}
	panicOnActiveJobs bool
	contextMarker     any
	mu                sync.Mutex
}

func (d *fakeJobDAO) GetJobByIDContext(ctx context.Context, id uint) (*model.ScheduledJob, error) {
	d.contextMarker = ctx.Value(jobContextTestKey{})
	for i := range d.jobs {
		if d.jobs[i].ID == id {
			return &d.jobs[i], nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (d *fakeJobDAO) GetJobListContext(ctx context.Context, req pagination.PageRequest, name string, status *int8) ([]model.ScheduledJob, int64, error) {
	d.contextMarker = ctx.Value(jobContextTestKey{})
	return d.jobs, int64(len(d.jobs)), nil
}

func (d *fakeJobDAO) CreateJobContext(ctx context.Context, job *model.ScheduledJob) error {
	d.contextMarker = ctx.Value(jobContextTestKey{})
	d.jobs = append(d.jobs, *job)
	return nil
}

func (d *fakeJobDAO) UpdateJobContext(ctx context.Context, job *model.ScheduledJob) error {
	d.contextMarker = ctx.Value(jobContextTestKey{})
	for i := range d.jobs {
		if d.jobs[i].ID == job.ID {
			d.jobs[i] = *job
			return nil
		}
	}
	return nil
}

func (d *fakeJobDAO) DeleteJobContext(ctx context.Context, id uint) error {
	d.contextMarker = ctx.Value(jobContextTestKey{})
	for i := range d.jobs {
		if d.jobs[i].ID == id {
			d.jobs = append(d.jobs[:i], d.jobs[i+1:]...)
			return nil
		}
	}
	return nil
}

func (d *fakeJobDAO) CreateJobLogContext(ctx context.Context, log *model.ScheduledJobLog) error {
	d.contextMarker = ctx.Value(jobContextTestKey{})
	if d.logs == nil {
		d.logs = map[uint][]model.ScheduledJobLog{}
	}
	d.logs[log.JobID] = append(d.logs[log.JobID], *log)
	return nil
}

func (d *fakeJobDAO) GetAllActiveJobsContext(ctx context.Context) ([]model.ScheduledJob, error) {
	d.contextMarker = ctx.Value(jobContextTestKey{})
	if d.panicOnActiveJobs {
		panic("GetAllActiveJobs should not be called")
	}
	activeJobs := make([]model.ScheduledJob, 0, len(d.jobs))
	for _, job := range d.jobs {
		if job.Status == 1 {
			activeJobs = append(activeJobs, job)
		}
	}
	return activeJobs, nil
}

func (d *fakeJobDAO) GetAllJobsContext(ctx context.Context) ([]model.ScheduledJob, error) {
	d.contextMarker = ctx.Value(jobContextTestKey{})
	return d.jobs, nil
}

func (d *fakeJobDAO) CleanupJobLogsBeforeContext(ctx context.Context, before time.Time) (int64, error) {
	if d.cleanupStarted != nil {
		select {
		case d.cleanupStarted <- struct{}{}:
		default:
		}
	}
	if d.cleanupContinue != nil {
		<-d.cleanupContinue
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.contextMarker = ctx.Value(jobContextTestKey{})
	d.cleanupBefore = before
	return d.cleanupRows, nil
}

func (d *fakeJobDAO) CountJobsByStatusContext(ctx context.Context, status *int8) (int64, error) {
	d.contextMarker = ctx.Value(jobContextTestKey{})
	if status == nil {
		return int64(len(d.jobs)), nil
	}

	var count int64
	for _, job := range d.jobs {
		if job.Status == *status {
			count++
		}
	}
	return count, nil
}

func (d *fakeJobDAO) CountFailedJobLogsSinceContext(ctx context.Context, since time.Time) (int64, error) {
	d.contextMarker = ctx.Value(jobContextTestKey{})
	var count int64
	for _, logs := range d.logs {
		for _, log := range logs {
			if log.Status == 0 && !log.CreatedAt.Before(since) {
				count++
			}
		}
	}
	return count, nil
}

func (d *fakeJobDAO) GetLatestJobRunTimeContext(ctx context.Context) (*time.Time, error) {
	d.contextMarker = ctx.Value(jobContextTestKey{})
	var latest *time.Time
	for _, job := range d.jobs {
		if job.LastRunTime == nil {
			continue
		}
		if latest == nil || job.LastRunTime.After(*latest) {
			runTime := *job.LastRunTime
			latest = &runTime
		}
	}
	return latest, nil
}

func (d *fakeJobDAO) GetLatestJobLogContext(ctx context.Context, jobID uint) (*model.ScheduledJobLog, error) {
	d.contextMarker = ctx.Value(jobContextTestKey{})
	logs := d.logs[jobID]
	if len(logs) == 0 {
		return nil, gorm.ErrRecordNotFound
	}

	latest := logs[0]
	for _, log := range logs[1:] {
		if log.CreatedAt.After(latest.CreatedAt) {
			latest = log
		}
	}
	return &latest, nil
}
