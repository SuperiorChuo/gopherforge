package monitor

import (
	"context"
	"fmt"
	"testing"
	"time"

	localmodel "github.com/go-admin-kit/services/monitor/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/pagination"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTaskRunQueriesFilterAndSummarize(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:task-run-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&localmodel.OpsTaskRun{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	rows := []localmodel.OpsTaskRun{
		{RunID: "r1", TaskKey: "monitor.health", Service: "monitor-service", Description: "health", Source: "scheduler", TriggerType: "manual", Status: localmodel.TaskRunStatusSucceeded, Attempt: 1, StartedAt: now.Add(-time.Minute), DurationMS: 25, CreatedAt: now},
		{RunID: "r2", TaskKey: "ops.backup", Service: "ops-cron", Description: "backup", Source: "ops-cron", TriggerType: "shell", Status: localmodel.TaskRunStatusFailed, Attempt: 1, StartedAt: now.Add(-2 * time.Minute), DurationMS: 75, ErrorMessage: "failed", CreatedAt: now},
		{RunID: "r3", TaskKey: "audit.cleanup", Service: "audit-service", Description: "cleanup", Source: "worker", TriggerType: "scheduled", Status: localmodel.TaskRunStatusRunning, Attempt: 1, StartedAt: now.Add(-3 * time.Minute), CreatedAt: now},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	dao := NewJobDAO(db)
	list, total, err := dao.ListTaskRunsContext(context.Background(), pagination.PageRequest{Page: 1, PageSize: 10}, TaskRunFilter{Keyword: "BACK", Status: localmodel.TaskRunStatusFailed})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(list) != 1 || list[0].RunID != "r2" {
		t.Fatalf("filtered runs total=%d list=%#v", total, list)
	}

	summary, err := dao.GetTaskRunSummaryContext(context.Background(), now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if summary.Total != 3 || summary.Running != 1 || summary.Succeeded != 1 || summary.Failed != 1 || summary.Services != 3 {
		t.Fatalf("summary = %#v", summary)
	}
	if summary.AverageMS != 50 {
		t.Fatalf("average = %v, want 50", summary.AverageMS)
	}
}
