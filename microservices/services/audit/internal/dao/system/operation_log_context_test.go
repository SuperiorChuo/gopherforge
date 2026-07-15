package system

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/services/audit/internal/pkg/authz"
	"github.com/go-admin-kit/services/audit/internal/pkg/pagination"
)

func TestOperationLogDAOGetLogListContextHonorsCanceledContext(t *testing.T) {
	db, _ := setupSystemDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := NewOperationLogDAO(db).GetLogListContext(
		ctx,
		pagination.PageRequest{Page: 1, PageSize: 10},
		nil,
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		nil,
		nil,
		nil,
		authz.UserDataScope{Scope: authz.DataScopeAll},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetLogListContext() error = %v, want context.Canceled", err)
	}
}

func TestOperationLogDAOGetLogByIDContextUsesInjectedDB(t *testing.T) {
	db, mock := newInjectedLogFileDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "operation_logs" WHERE "operation_logs"."id" = $1 ORDER BY "operation_logs"."id" LIMIT $2`)).
		WithArgs(uint(11), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "module"}).AddRow(uint(11), "system"))

	log, err := NewOperationLogDAO(db).GetLogByIDContext(context.Background(), 11)
	if err != nil {
		t.Fatalf("GetLogByIDContext() error = %v", err)
	}
	if log.ID != 11 {
		t.Fatalf("GetLogByIDContext() id = %d, want 11", log.ID)
	}
}
