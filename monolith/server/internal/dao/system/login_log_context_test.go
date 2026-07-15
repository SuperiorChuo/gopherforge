package system

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
)

func TestLoginLogDAOGetListContextHonorsCanceledContext(t *testing.T) {
	db, _ := setupSystemDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := NewLoginLogDAO(db).GetListContext(
		ctx,
		pagination.PageRequest{Page: 1, PageSize: 10},
		nil,
		"",
		"",
		nil,
		nil,
		nil,
		nil,
		authz.UserDataScope{Scope: authz.DataScopeAll},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetListContext() error = %v, want context.Canceled", err)
	}
}

func TestLoginLogDAOGetByIDContextUsesInjectedDB(t *testing.T) {
	db, mock := newInjectedLogFileDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "login_logs" WHERE "login_logs"."id" = $1 ORDER BY "login_logs"."id" LIMIT $2`)).
		WithArgs(uint(9), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(uint(9), "alice"))

	log, err := NewLoginLogDAO(db).GetByIDContext(context.Background(), 9)
	if err != nil {
		t.Fatalf("GetByIDContext() error = %v", err)
	}
	if log.ID != 9 {
		t.Fatalf("GetByIDContext() id = %d, want 9", log.ID)
	}
}
