package monitor

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/server/internal/pkg/database"
)

func TestJobDAOGetJobByIDContextHonorsCanceledContext(t *testing.T) {
	setupMonitorDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewJobDAO().GetJobByIDContext(ctx, 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetJobByIDContext() error = %v, want context.Canceled", err)
	}
}

func TestJobDAOUsesInjectedDB(t *testing.T) {
	oldDB := database.DB
	database.DB = nil
	t.Cleanup(func() {
		database.DB = oldDB
	})

	db, mock := newInjectedMonitorDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `scheduled_jobs` WHERE `scheduled_jobs`.`id` = ? ORDER BY `scheduled_jobs`.`id` LIMIT ?")).
		WithArgs(uint(42), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(42, "daily-report"))

	job, err := NewJobDAO(db).GetJobByID(42)
	if err != nil {
		t.Fatalf("GetJobByID() error = %v", err)
	}
	if job.ID != 42 || job.Name != "daily-report" {
		t.Fatalf("job = %#v, want id=42 name=daily-report", job)
	}
}
