package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

var fanSyncTestStoreSequence atomic.Uint64

func newFanSyncTestStore(t *testing.T, name string) *Store {
	t.Helper()
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", name, fanSyncTestStoreSequence.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sqlite db handle: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	st, err := NewWithDB(db)
	if err != nil {
		t.Fatalf("migrate store: %v", err)
	}
	return st
}

func claimOneFanSyncJob(t *testing.T, st *Store, lease time.Duration) MpFanSyncJob {
	t.Helper()
	jobs, err := st.ClaimFanSyncJobs(context.Background(), 1, lease)
	if err != nil {
		t.Fatalf("claim fan sync job: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("claimed jobs=%d, want 1", len(jobs))
	}
	return jobs[0]
}

func TestFanSyncJobPersistsPageOffsetAndFinishesIdempotently(t *testing.T) {
	ctx := context.Background()
	st := newFanSyncTestStore(t, "fan-sync-progress")
	job, reused, err := st.EnqueueFanSyncJob(ctx, 1, 7, 42)
	if err != nil || reused {
		t.Fatalf("enqueue job=%+v reused=%v err=%v", job, reused, err)
	}
	again, reused, err := st.EnqueueFanSyncJob(ctx, 1, 7, 99)
	if err != nil || !reused || again.ID != job.ID {
		t.Fatalf("duplicate enqueue=%+v reused=%v err=%v", again, reused, err)
	}

	claimed := claimOneFanSyncJob(t, st, time.Minute)
	if err := st.StageFanSyncPage(ctx, &claimed, 4, []string{"o1", "o2", "o3"}, "cursor-2"); err != nil {
		t.Fatalf("stage first page: %v", err)
	}
	claimed = claimOneFanSyncJob(t, st, time.Minute)
	finished, err := st.CommitFanSyncBatch(ctx, &claimed, []MpFan{
		{TenantID: 1, AccountID: 7, OpenID: "o1", Nickname: "甲", Subscribed: true},
		{TenantID: 1, AccountID: 7, OpenID: "o2", Nickname: "乙", Subscribed: true},
	}, time.Now(), 3, 0)
	if err != nil || finished {
		t.Fatalf("commit first batch finished=%v err=%v", finished, err)
	}
	latest, err := st.LatestFanSyncJob(ctx, 1, 7)
	if err != nil || latest.PageOffset != 2 || latest.SyncedCount != 2 || latest.ListedCount != 3 {
		t.Fatalf("progress=%+v err=%v", latest, err)
	}

	claimed = claimOneFanSyncJob(t, st, time.Minute)
	finished, err = st.CommitFanSyncBatch(ctx, &claimed, []MpFan{
		{TenantID: 1, AccountID: 7, OpenID: "o3", Nickname: "丙", Subscribed: true},
	}, time.Now(), 3, 0)
	if err != nil || finished {
		t.Fatalf("finish first page finished=%v err=%v", finished, err)
	}
	latest, _ = st.LatestFanSyncJob(ctx, 1, 7)
	if latest.Cursor != "cursor-2" || latest.PageOpenIDs != "" || latest.SyncedCount != 3 {
		t.Fatalf("advanced job=%+v", latest)
	}

	claimed = claimOneFanSyncJob(t, st, time.Minute)
	if err := st.StageFanSyncPage(ctx, &claimed, 4, []string{"o4"}, ""); err != nil {
		t.Fatalf("stage final page: %v", err)
	}
	claimed = claimOneFanSyncJob(t, st, time.Minute)
	finished, err = st.CommitFanSyncBatch(ctx, &claimed, []MpFan{
		{TenantID: 1, AccountID: 7, OpenID: "o4", Nickname: "丁", Tags: "7,8", Subscribed: true},
	}, time.Now(), 1, 0)
	if err != nil || !finished {
		t.Fatalf("finish job finished=%v err=%v", finished, err)
	}
	latest, _ = st.LatestFanSyncJob(ctx, 1, 7)
	if latest.Status != "succeeded" || latest.ActiveKey != nil || latest.SyncedCount != 4 || latest.ListedCount != 4 || latest.TotalCount != 4 {
		t.Fatalf("completed job=%+v", latest)
	}
	var fans []MpFan
	if err := st.DB().Where("tenant_id = ? AND account_id = ?", 1, 7).Order("open_id").Find(&fans).Error; err != nil {
		t.Fatalf("list fans: %v", err)
	}
	if len(fans) != 4 || fans[3].Tags != "7,8" || fans[3].LastSyncJobID != job.ID {
		t.Fatalf("fans=%+v", fans)
	}
}

func TestFanSyncConcurrentEnqueueReusesOneActiveJob(t *testing.T) {
	ctx := context.Background()
	st := newFanSyncTestStore(t, "fan-sync-concurrent-enqueue")
	sqlDB, err := st.DB().DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	// SQLite has database-level write locking. One connection keeps this test
	// focused on the concurrent API contract; PostgreSQL lock behavior is
	// verified separately during integration deployment.
	sqlDB.SetMaxOpenConns(1)

	const workers = 16
	type result struct {
		job    *MpFanSyncJob
		reused bool
		err    error
	}
	results := make(chan result, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(createdBy uint64) {
			defer wg.Done()
			job, reused, err := st.EnqueueFanSyncJob(ctx, 1, 70, createdBy)
			results <- result{job: job, reused: reused, err: err}
		}(uint64(i + 1))
	}
	wg.Wait()
	close(results)

	var jobID uint64
	created := 0
	for item := range results {
		if item.err != nil || item.job == nil {
			t.Fatalf("enqueue result=%+v", item)
		}
		if jobID == 0 {
			jobID = item.job.ID
		} else if item.job.ID != jobID {
			t.Fatalf("multiple active jobs: first=%d got=%d", jobID, item.job.ID)
		}
		if !item.reused {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("created jobs=%d, want 1", created)
	}
	var activeCount int64
	if err := st.DB().Model(&MpFanSyncJob{}).Where("active_key IS NOT NULL").Count(&activeCount).Error; err != nil {
		t.Fatalf("count active jobs: %v", err)
	}
	if activeCount != 1 {
		t.Fatalf("active jobs=%d, want 1", activeCount)
	}
}

func TestFanSyncExpiredLeaseRejectsStaleWorker(t *testing.T) {
	ctx := context.Background()
	st := newFanSyncTestStore(t, "fan-sync-lease")
	if _, _, err := st.EnqueueFanSyncJob(ctx, 1, 8, 42); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	stale := claimOneFanSyncJob(t, st, time.Nanosecond)
	time.Sleep(time.Millisecond)
	recovered := claimOneFanSyncJob(t, st, time.Minute)
	if stale.LeaseToken == recovered.LeaseToken {
		t.Fatal("recovered lease token must change")
	}
	if err := st.StageFanSyncPage(ctx, &stale, 1, []string{"old"}, ""); !errors.Is(err, ErrFanSyncLeaseLost) {
		t.Fatalf("stale stage err=%v, want lease lost", err)
	}
	if err := st.StageFanSyncPage(ctx, &recovered, 1, []string{"current"}, ""); err != nil {
		t.Fatalf("recovered stage: %v", err)
	}
}

func TestFanSyncRetryAndTerminalFailureReleaseActiveKey(t *testing.T) {
	ctx := context.Background()
	st := newFanSyncTestStore(t, "fan-sync-retry")
	job, _, err := st.EnqueueFanSyncJob(ctx, 1, 9, 42)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	claimed := claimOneFanSyncJob(t, st, time.Minute)
	if err := st.RetryFanSyncJob(ctx, &claimed, 2, "temporary"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if err := st.DB().Model(&MpFanSyncJob{}).Where("id = ?", job.ID).Update("next_at", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatalf("make retry due: %v", err)
	}
	claimed = claimOneFanSyncJob(t, st, time.Minute)
	if err := st.RetryFanSyncJob(ctx, &claimed, 2, "still failing"); err != nil {
		t.Fatalf("exhaust retry: %v", err)
	}
	latest, _ := st.LatestFanSyncJob(ctx, 1, 9)
	if latest.Status != "failed" || latest.ActiveKey != nil || latest.Attempts != 2 || latest.FinishedAt == nil {
		t.Fatalf("failed job=%+v", latest)
	}
	newJob, reused, err := st.EnqueueFanSyncJob(ctx, 1, 9, 42)
	if err != nil || reused || newJob.ID == job.ID {
		t.Fatalf("enqueue after failure=%+v reused=%v err=%v", newJob, reused, err)
	}
}

func TestFanSyncCompletionMarksMissingFansUnsubscribed(t *testing.T) {
	ctx := context.Background()
	st := newFanSyncTestStore(t, "fan-sync-missing")
	oldSnapshot := time.Now().Add(-time.Hour)
	if err := st.UpsertFansContext(ctx, []MpFan{
		{TenantID: 1, AccountID: 10, OpenID: "missing-old", Subscribed: true},
		{TenantID: 1, AccountID: 10, OpenID: "missing-with-new-callback", Subscribed: true},
	}, oldSnapshot, 77); err != nil {
		t.Fatalf("seed old fans: %v", err)
	}
	job, _, err := st.EnqueueFanSyncJob(ctx, 1, 10, 42)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	claimed := claimOneFanSyncJob(t, st, time.Minute)
	if err := st.StageFanSyncPage(ctx, &claimed, 1, []string{"present"}, ""); err != nil {
		t.Fatalf("stage page: %v", err)
	}
	if claimed.StartedAt == nil {
		t.Fatal("claimed job has no start time")
	}
	// A same-second callback is newer than the full-job reconciliation point
	// even though WeChat cannot express its sub-second ordering.
	if err := st.SetFanSubscribed(1, 10, "missing-with-new-callback", true, claimed.StartedAt.Truncate(time.Second)); err != nil {
		t.Fatalf("concurrent callback: %v", err)
	}
	claimed = claimOneFanSyncJob(t, st, time.Minute)
	finished, err := st.CommitFanSyncBatch(ctx, &claimed, []MpFan{{
		TenantID: 1, AccountID: 10, OpenID: "present", Subscribed: true,
	}}, time.Now(), 1, 0)
	if err != nil || !finished {
		t.Fatalf("finish sync finished=%v err=%v", finished, err)
	}

	var fans []MpFan
	if err := st.DB().Where("tenant_id = ? AND account_id = ?", 1, 10).Order("open_id").Find(&fans).Error; err != nil {
		t.Fatalf("load fans: %v", err)
	}
	states := make(map[string]bool, len(fans))
	for _, fan := range fans {
		states[fan.OpenID] = fan.Subscribed
	}
	if states["missing-old"] || !states["missing-with-new-callback"] || !states["present"] {
		t.Fatalf("subscription states=%v", states)
	}
	latest, err := st.LatestFanSyncJob(ctx, 1, 10)
	if err != nil || latest.ID != job.ID || latest.Status != "succeeded" || latest.SyncedCount != 1 {
		t.Fatalf("completed job=%+v err=%v", latest, err)
	}
}

func TestEmptyFanSyncCompletionReconcilesExistingFans(t *testing.T) {
	ctx := context.Background()
	st := newFanSyncTestStore(t, "fan-sync-empty")
	if err := st.UpsertFansContext(ctx, []MpFan{{
		TenantID: 1, AccountID: 11, OpenID: "formerly-subscribed", Subscribed: true,
	}}, time.Now().Add(-time.Hour), 71); err != nil {
		t.Fatalf("seed fan: %v", err)
	}
	if _, _, err := st.EnqueueFanSyncJob(ctx, 1, 11, 42); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	claimed := claimOneFanSyncJob(t, st, time.Minute)
	if err := st.FinishEmptyFanSyncJob(ctx, &claimed, 0); err != nil {
		t.Fatalf("finish empty sync: %v", err)
	}
	var fan MpFan
	if err := st.DB().Where("account_id = ? AND open_id = ?", 11, "formerly-subscribed").First(&fan).Error; err != nil {
		t.Fatalf("load fan: %v", err)
	}
	if fan.Subscribed {
		t.Fatalf("empty full sync left stale subscription: %+v", fan)
	}
	latest, err := st.LatestFanSyncJob(ctx, 1, 11)
	if err != nil || latest.Status != "succeeded" || latest.ActiveKey != nil || latest.FinishedAt == nil {
		t.Fatalf("completed empty job=%+v err=%v", latest, err)
	}
}

func TestValidateFanSyncCompletion(t *testing.T) {
	tests := []struct {
		name                  string
		listed, total, synced int
		wantErr               bool
	}{
		{name: "exact", listed: 3, total: 3, synced: 3},
		{name: "total shrank during paging", listed: 3, total: 2, synced: 3, wantErr: true},
		{name: "terminal cursor too early", listed: 2, total: 3, synced: 2, wantErr: true},
		{name: "cross-page duplicate", listed: 3, total: 3, synced: 2, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFanSyncCompletion(tt.listed, tt.total, tt.synced)
			if tt.wantErr != errors.Is(err, ErrFanSyncIncomplete) {
				t.Fatalf("validate err=%v wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestFanProfileSyncDoesNotOverwriteNewerSubscriptionCallback(t *testing.T) {
	ctx := context.Background()
	st := newFanSyncTestStore(t, "fan-sync-subscription")
	eventAt := time.Now()
	if err := st.SetFanSubscribed(1, 10, "openid-1", false, eventAt); err != nil {
		t.Fatalf("unsubscribe callback: %v", err)
	}
	if err := st.UpsertFansContext(ctx, []MpFan{{
		TenantID: 1, AccountID: 10, OpenID: "openid-1", Nickname: "同步昵称", Subscribed: true,
	}}, eventAt.Add(-time.Second), 88); err != nil {
		t.Fatalf("older profile sync: %v", err)
	}
	var fan MpFan
	if err := st.DB().Where("account_id = ? AND open_id = ?", 10, "openid-1").First(&fan).Error; err != nil {
		t.Fatalf("load fan: %v", err)
	}
	if fan.Subscribed || fan.Nickname != "同步昵称" || fan.LastSyncJobID != 88 {
		t.Fatalf("fan after older snapshot=%+v", fan)
	}
	if err := st.UpsertFansContext(ctx, []MpFan{{
		TenantID: 1, AccountID: 10, OpenID: "openid-1", Nickname: "同秒同步", Subscribed: true,
	}}, eventAt.Truncate(time.Second).Add(time.Second-500*time.Nanosecond), 89); err != nil {
		t.Fatalf("same-second profile sync: %v", err)
	}
	if err := st.DB().Where("account_id = ? AND open_id = ?", 10, "openid-1").First(&fan).Error; err != nil {
		t.Fatalf("reload same-second fan: %v", err)
	}
	if fan.Subscribed {
		t.Fatalf("same-second sync overwrote callback: %+v", fan)
	}
	if err := st.SetFanSubscribed(1, 10, "openid-1", true, eventAt.Add(-2*time.Second)); err != nil {
		t.Fatalf("out-of-order callback: %v", err)
	}
	if err := st.DB().Where("account_id = ? AND open_id = ?", 10, "openid-1").First(&fan).Error; err != nil {
		t.Fatalf("reload fan: %v", err)
	}
	if fan.Subscribed {
		t.Fatalf("older callback overwrote newer unsubscribe: %+v", fan)
	}
	if err := st.UpsertFansContext(ctx, []MpFan{{
		TenantID: 1, AccountID: 10, OpenID: "openid-1", Nickname: "最新昵称", Subscribed: true,
	}}, eventAt.Add(time.Second), 90); err != nil {
		t.Fatalf("newer profile sync: %v", err)
	}
	if err := st.DB().Where("account_id = ? AND open_id = ?", 10, "openid-1").First(&fan).Error; err != nil {
		t.Fatalf("reload latest fan: %v", err)
	}
	if !fan.Subscribed || fan.Nickname != "最新昵称" || fan.LastSyncJobID != 90 {
		t.Fatalf("fan after newer snapshot=%+v", fan)
	}
	if err := st.SetFanSubscribed(1, 10, "openid-1", false, eventAt.Add(-time.Second)); err != nil {
		t.Fatalf("late old callback: %v", err)
	}
	if err := st.DB().Where("account_id = ? AND open_id = ?", 10, "openid-1").First(&fan).Error; err != nil {
		t.Fatalf("reload after late callback: %v", err)
	}
	if !fan.Subscribed {
		t.Fatalf("late old callback overwrote newer sync: %+v", fan)
	}
}

func TestSameSecondOppositeCallbacksPreferUnsubscribe(t *testing.T) {
	st := newFanSyncTestStore(t, "fan-sync-same-second-callbacks")
	eventAt := time.Now().Truncate(time.Second)

	if err := st.SetFanSubscribed(1, 12, "subscribe-then-unsubscribe", true, eventAt); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if err := st.SetFanSubscribed(1, 12, "subscribe-then-unsubscribe", false, eventAt); err != nil {
		t.Fatalf("unsubscribe: %v", err)
	}
	if err := st.SetFanSubscribed(1, 12, "unsubscribe-then-subscribe", false, eventAt); err != nil {
		t.Fatalf("unsubscribe first: %v", err)
	}
	if err := st.SetFanSubscribed(1, 12, "unsubscribe-then-subscribe", true, eventAt); err != nil {
		t.Fatalf("same-second subscribe replay: %v", err)
	}

	var fans []MpFan
	if err := st.DB().Where("tenant_id = ? AND account_id = ?", 1, 12).Order("open_id").Find(&fans).Error; err != nil {
		t.Fatalf("load fans: %v", err)
	}
	if len(fans) != 2 || fans[0].Subscribed || fans[1].Subscribed {
		t.Fatalf("same-second states=%+v", fans)
	}
}
