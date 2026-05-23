package system

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/server/internal/model"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestAuditLogDAOListLogsContextHonorsCanceledContext(t *testing.T) {
	setupSystemDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (&AuditLogDAO{}).ListLogsContext(ctx, AuditLogListQuery{Page: 1, PageSize: 10})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ListLogsContext() error = %v, want context.Canceled", err)
	}
}

func TestAuditLogDAOCreateLogContextUsesInjectedDB(t *testing.T) {
	setupSystemDAOTestDB(t)
	db, mock := newInjectedLogFileDAOTestDB(t)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `audit_logs`")).
		WillReturnResult(sqlmock.NewResult(1, 1))

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
	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
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
