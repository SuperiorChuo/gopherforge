package store

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/ticket/internal/model"
	"gorm.io/gorm"
)

func TestNotificationJobsAreDeduplicatedAndRetried(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	st, err := NewWithDB(db)
	if err != nil {
		t.Fatal(err)
	}
	vars := map[string]string{"ticket_no": "TK-1"}
	for i := 0; i < 2; i++ {
		if err := st.EnqueueNotification(7, 9, "ticket.assigned", "ticket", "1", vars); err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	if err := db.Model(&model.NotificationJob{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("jobs=%d, want 1", count)
	}
	jobs, err := st.ClaimNotificationJobs(10, time.Minute)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("claim jobs=%d err=%v", len(jobs), err)
	}
	if err := st.RetryNotificationJob(jobs[0].ID, 1, 8, "notify unavailable"); err != nil {
		t.Fatal(err)
	}
	var got model.NotificationJob
	if err := db.First(&got, jobs[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Status != "pending" || got.Attempts != 1 || got.LastError == "" || !got.NextAt.After(time.Now()) {
		t.Fatalf("retry state=%+v", got)
	}
}
