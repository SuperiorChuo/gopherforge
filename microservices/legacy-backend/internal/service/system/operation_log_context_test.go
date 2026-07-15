package system

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func setupSystemServiceContextTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock db: %v", err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm sqlmock db: %v", err)
	}

	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet database expectations: %v", err)
		}
		_ = sqlDB.Close()
	})

	return db, mock
}

func TestOperationLogServiceGetLogListContextHonorsCanceledContext(t *testing.T) {
	db, _ := setupSystemServiceContextTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc := NewOperationLogServiceWithDB(db)
	_, _, err := (&svc).GetLogListContext(ctx, OperationLogListRequest{
		PageRequest: pagination.PageRequest{Page: 1, PageSize: 10},
		DataScope:   authz.UserDataScope{Scope: authz.DataScopeAll},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetLogListContext() error = %v, want context.Canceled", err)
	}
}

func TestOperationLogServiceGetLogByIDContextReturnsNotFoundSentinel(t *testing.T) {
	db, mock := setupSystemServiceContextTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "operation_logs" WHERE "operation_logs"."id" = $1 ORDER BY "operation_logs"."id" LIMIT $2`)).
		WithArgs(7, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	svc := NewOperationLogServiceWithDB(db)
	_, err := (&svc).GetLogByIDContext(context.Background(), 7)
	if !errors.Is(err, ErrOperationLogNotFound) {
		t.Fatalf("GetLogByIDContext() error = %v, want ErrOperationLogNotFound", err)
	}
}
