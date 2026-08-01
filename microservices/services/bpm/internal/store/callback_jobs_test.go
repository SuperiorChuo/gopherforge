package store

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/bpm/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var callbackDBSeq atomic.Int64

func openCallbackStore(t *testing.T) *Store {
	t.Helper()
	dsn := fmt.Sprintf("file:bpmcallback%d?mode=memory&cache=shared", callbackDBSeq.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("sqlite: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	s, err := NewWithDB(db)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	return s
}

func TestCallbackJobLeaseExpiresAndCanBeReclaimed(t *testing.T) {
	s := openCallbackStore(t)
	job := model.CallbackJob{TenantID: 1, InstanceID: 10, Status: "pending", NextAt: time.Now().Add(-time.Second)}
	if err := s.DB().Create(&job).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	claimed, err := s.ClaimCallbackJobs(10, time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("first claim: jobs=%d err=%v", len(claimed), err)
	}
	claimed, err = s.ClaimCallbackJobs(10, time.Minute)
	if err != nil || len(claimed) != 0 {
		t.Fatalf("active lease should hide job: jobs=%d err=%v", len(claimed), err)
	}
	if err := s.DB().Model(&job).Update("next_at", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatalf("expire lease: %v", err)
	}
	claimed, err = s.ClaimCallbackJobs(10, time.Minute)
	if err != nil || len(claimed) != 1 || claimed[0].ID != job.ID {
		t.Fatalf("reclaim: jobs=%+v err=%v", claimed, err)
	}
}

func TestCallbackJobRetryBecomesDead(t *testing.T) {
	s := openCallbackStore(t)
	job := model.CallbackJob{TenantID: 2, InstanceID: 20, Status: "pending", NextAt: time.Now()}
	if err := s.DB().Create(&job).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.RetryCallbackJob(job.ID, 1, 2, "temporary"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	var got model.CallbackJob
	if err := s.DB().First(&got, job.ID).Error; err != nil {
		t.Fatalf("load retry: %v", err)
	}
	if got.Status != "pending" || got.Attempts != 1 || !got.NextAt.After(time.Now()) {
		t.Fatalf("retry state: %+v", got)
	}
	if err := s.RetryCallbackJob(job.ID, 2, 2, "permanent"); err != nil {
		t.Fatalf("dead: %v", err)
	}
	if err := s.DB().First(&got, job.ID).Error; err != nil {
		t.Fatalf("load dead: %v", err)
	}
	if got.Status != "dead" || got.Attempts != 2 || got.LastError != "permanent" {
		t.Fatalf("dead state: %+v", got)
	}
}
