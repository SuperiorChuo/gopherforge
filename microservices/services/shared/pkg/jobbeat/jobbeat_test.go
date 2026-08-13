package jobbeat

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestReportTargetsMonitorServiceSchema(t *testing.T) {
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

	mock.ExpectExec(`INSERT INTO monitor_svc\.ops_job_heartbeats`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	Report(db, Run{Key: "system.cleanup", Service: "system-service", StartedAt: time.Now()})
}
