package system

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/go-admin-kit/services/identity/internal/pkg/authz"
	"github.com/go-admin-kit/services/identity/internal/pkg/pagination"
	"gorm.io/gorm"
)

func TestOperationLogServiceGetLogListContextHonorsCanceledContext(t *testing.T) {
	db, _ := setupSystemUserServiceContextTestDB(t)

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
	db, mock := setupSystemUserServiceContextTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "operation_logs" WHERE "operation_logs"."id" = $1 ORDER BY "operation_logs"."id" LIMIT $2`)).
		WithArgs(7, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	svc := NewOperationLogServiceWithDB(db)
	_, err := (&svc).GetLogByIDContext(context.Background(), 7)
	if !errors.Is(err, ErrOperationLogNotFound) {
		t.Fatalf("GetLogByIDContext() error = %v, want ErrOperationLogNotFound", err)
	}
}
