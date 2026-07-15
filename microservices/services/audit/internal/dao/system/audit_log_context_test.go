package system

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/services/audit/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestAuditLogDAOListLogsContextHonorsCanceledContext(t *testing.T) {
	db, _ := setupSystemDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewAuditLogDAO(db).ListLogsContext(ctx, AuditLogListQuery{Page: 1, PageSize: 10})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ListLogsContext() error = %v, want context.Canceled", err)
	}
}

func TestAuditLogDAOCreateLogContextUsesInjectedDB(t *testing.T) {
	db, mock := newInjectedLogFileDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "audit_logs"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	err := NewAuditLogDAO(db).CreateLogContext(context.Background(), &model.AuditLog{
		Action:     "create",
		TargetType: "file",
		TargetID:   "42",
	})
	if err != nil {
		t.Fatalf("CreateLogContext() error = %v", err)
	}
}

func newInjectedLogFileDAOTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open injected sqlmock db: %v", err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("open injected gorm sqlmock db: %v", err)
	}

	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet injected database expectations: %v", err)
		}
		_ = sqlDB.Close()
	})

	return db, mock
}
