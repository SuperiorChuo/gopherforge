package system

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
	"gorm.io/gorm"
)

func TestOperationLogServiceGetLogListContextHonorsCanceledContext(t *testing.T) {
	setupSystemUserServiceContextTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := (&OperationLogService{}).GetLogListContext(ctx, OperationLogListRequest{
		PageRequest: pagination.PageRequest{Page: 1, PageSize: 10},
		DataScope:   authz.UserDataScope{Scope: authz.DataScopeAll},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetLogListContext() error = %v, want context.Canceled", err)
	}
}

func TestOperationLogServiceGetLogByIDContextReturnsNotFoundSentinel(t *testing.T) {
	mock := setupSystemUserServiceContextTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `operation_logs` WHERE `operation_logs`.`id` = ? ORDER BY `operation_logs`.`id` LIMIT ?")).
		WithArgs(7, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := (&OperationLogService{}).GetLogByIDContext(context.Background(), 7)
	if !errors.Is(err, ErrOperationLogNotFound) {
		t.Fatalf("GetLogByIDContext() error = %v, want ErrOperationLogNotFound", err)
	}
}
