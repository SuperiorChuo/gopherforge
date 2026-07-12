package monitor

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestJobDAOGetJobByIDContextHonorsCanceledContext(t *testing.T) {
	db, _ := newMonitorDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewJobDAO(db).GetJobByIDContext(ctx, 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetJobByIDContext() error = %v, want context.Canceled", err)
	}
}

func TestJobDAOUsesInjectedDB(t *testing.T) {
	db, mock := newMonitorDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `scheduled_jobs` WHERE `scheduled_jobs`.`id` = ? ORDER BY `scheduled_jobs`.`id` LIMIT ?")).
		WithArgs(uint(42), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(42, "daily-report"))

	job, err := NewJobDAO(db).GetJobByIDContext(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetJobByIDContext() error = %v", err)
	}
	if job.ID != 42 || job.Name != "daily-report" {
		t.Fatalf("job = %#v, want id=42 name=daily-report", job)
	}
}
