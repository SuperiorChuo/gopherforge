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

func TestFileDAOGetListContextHonorsCanceledContext(t *testing.T) {
	setupSystemDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := (&FileDAO{}).GetListContext(
		ctx,
		pagination.PageRequest{Page: 1, PageSize: 10},
		nil,
		"",
		"",
		nil,
		nil,
		authz.UserDataScope{Scope: authz.DataScopeAll},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetListContext() error = %v, want context.Canceled", err)
	}
}

func TestFileDAOGetByHashContextUsesInjectedDB(t *testing.T) {
	setupSystemDAOTestDB(t)
	db, mock := newInjectedLogFileDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `files` WHERE hash = ? ORDER BY `files`.`id` LIMIT ?")).
		WithArgs("abc123", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "hash", "file_name", "file_path"}).
			AddRow(uint(7), "abc123", "report.pdf", "/tmp/report.pdf"))

	file, err := NewFileDAO(db).GetByHashContext(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("GetByHashContext() error = %v", err)
	}
	if file.ID != 7 {
		t.Fatalf("GetByHashContext() id = %d, want 7", file.ID)
	}
}
