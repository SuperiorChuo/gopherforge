package monitor

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

func TestCleanupJobLogsDeletesOlderThanRetention(t *testing.T) {
	dao := &fakeJobDAO{cleanupRows: 3}
	service := &JobService{dao: dao}

	startCutoff := time.Now().AddDate(0, 0, -7)
	result, err := service.CleanupJobLogs(7)
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

	_, err := service.CleanupJobLogs(0)
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

	health, err := service.CheckJobHealth(24)
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
	defer service.cron.Stop()

	if service.dao != dao {
		t.Fatal("newJobService did not keep injected dao")
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
	panicOnActiveJobs bool
}

func (d *fakeJobDAO) GetJobByID(id uint) (*model.ScheduledJob, error) {
	for i := range d.jobs {
		if d.jobs[i].ID == id {
			return &d.jobs[i], nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (d *fakeJobDAO) GetJobList(pagination.PageRequest, string, *int8) ([]model.ScheduledJob, int64, error) {
	return d.jobs, int64(len(d.jobs)), nil
}

func (d *fakeJobDAO) CreateJob(job *model.ScheduledJob) error {
	d.jobs = append(d.jobs, *job)
	return nil
}

func (d *fakeJobDAO) UpdateJob(job *model.ScheduledJob) error {
	for i := range d.jobs {
		if d.jobs[i].ID == job.ID {
			d.jobs[i] = *job
			return nil
		}
	}
	return nil
}

func (d *fakeJobDAO) DeleteJob(id uint) error {
	for i := range d.jobs {
		if d.jobs[i].ID == id {
			d.jobs = append(d.jobs[:i], d.jobs[i+1:]...)
			return nil
		}
	}
	return nil
}

func (d *fakeJobDAO) CreateJobLog(log *model.ScheduledJobLog) error {
	if d.logs == nil {
		d.logs = map[uint][]model.ScheduledJobLog{}
	}
	d.logs[log.JobID] = append(d.logs[log.JobID], *log)
	return nil
}

func (d *fakeJobDAO) GetAllActiveJobs() ([]model.ScheduledJob, error) {
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

func (d *fakeJobDAO) GetAllJobs() ([]model.ScheduledJob, error) {
	return d.jobs, nil
}

func (d *fakeJobDAO) CleanupJobLogsBefore(before time.Time) (int64, error) {
	d.cleanupBefore = before
	return d.cleanupRows, nil
}

func (d *fakeJobDAO) CountJobsByStatus(status *int8) (int64, error) {
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

func (d *fakeJobDAO) CountFailedJobLogsSince(since time.Time) (int64, error) {
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

func (d *fakeJobDAO) GetLatestJobRunTime() (*time.Time, error) {
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

func (d *fakeJobDAO) GetLatestJobLog(jobID uint) (*model.ScheduledJobLog, error) {
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
