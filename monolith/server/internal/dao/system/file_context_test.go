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
	db, _ := setupSystemDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := NewFileDAO(db).GetListContext(
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
	db, mock := newInjectedLogFileDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "files" WHERE hash = $1 ORDER BY "files"."id" LIMIT $2`)).
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

func TestFileDAOCountByPathExcludingIDUsesInjectedDB(t *testing.T) {
	db, mock := newInjectedLogFileDAOTestDB(t)
	dao := NewFileDAO(db)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "files" WHERE storage_type = $1 AND file_path = $2 AND id <> $3`)).
		WithArgs("local", "/uploads/avatar.png", uint(7)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(2)))
	filePathCount, err := dao.CountByFilePathExcludingIDContext(context.Background(), "local", "/uploads/avatar.png", 7)
	if err != nil {
		t.Fatalf("CountByFilePathExcludingIDContext() error = %v", err)
	}
	if filePathCount != 2 {
		t.Fatalf("CountByFilePathExcludingIDContext() count = %d, want 2", filePathCount)
	}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "files" WHERE storage_type = $1 AND thumbnail_path = $2 AND id <> $3`)).
		WithArgs("local", "/uploads/thumbs/avatar.png", uint(7)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	thumbnailPathCount, err := dao.CountByThumbnailPathExcludingIDContext(context.Background(), "local", "/uploads/thumbs/avatar.png", 7)
	if err != nil {
		t.Fatalf("CountByThumbnailPathExcludingIDContext() error = %v", err)
	}
	if thumbnailPathCount != 1 {
		t.Fatalf("CountByThumbnailPathExcludingIDContext() count = %d, want 1", thumbnailPathCount)
	}
}
