package jobbeat

import (
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func openMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock db: %v", err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet database expectations: %v", err)
		}
		_ = sqlDB.Close()
	})
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm sqlmock db: %v", err)
	}
	return db, mock
}

func TestReportUsesPublicTaskTablesWithoutAppendingHistory(t *testing.T) {
	db, mock := openMockDB(t)
	mock.ExpectExec(`INSERT INTO ops_job_heartbeats`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	Report(db, Run{Key: "system.cleanup", Service: "system-service", StartedAt: time.Now()})
}

func TestExecutionRecordsLifecycleAndHeartbeatOnce(t *testing.T) {
	db, mock := openMockDB(t)
	mock.ExpectExec(`INSERT INTO ops_task_runs`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	execution := Start(db, Run{
		RunID:       "run-1",
		Key:         "monitor.health",
		Service:     "monitor-service",
		Description: "health check",
		Source:      "scheduler",
		Trigger:     "manual",
		StartedAt:   time.Now().Add(-time.Second),
	})
	mock.ExpectExec(`UPDATE ops_task_runs`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO ops_job_heartbeats`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	execution.Finish(nil, "healthy")
	execution.Finish(errors.New("must be ignored"), "duplicate")
}

func TestExecutionStoresFailureAndKeepsTaskSoft(t *testing.T) {
	db, mock := openMockDB(t)
	mock.ExpectExec(`INSERT INTO ops_task_runs`).
		WillReturnError(errors.New("task run table unavailable"))
	execution := Start(db, Run{RunID: "run-2", Key: "audit.cleanup", Service: "audit-service"})
	mock.ExpectExec(`UPDATE ops_task_runs`).
		WillReturnError(errors.New("task run table unavailable"))
	mock.ExpectExec(`INSERT INTO ops_job_heartbeats`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	execution.Finish(errors.New("cleanup failed"), "")
}

func TestNewRunIDIsRandomHex(t *testing.T) {
	first, second := NewRunID(), NewRunID()
	if len(first) != 32 || len(second) != 32 {
		t.Fatalf("unexpected run id lengths: %q %q", first, second)
	}
	if first == second {
		t.Fatalf("run ids collided: %q", first)
	}
}
