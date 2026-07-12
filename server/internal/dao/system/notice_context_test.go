package system

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
)

func TestNoticeDAOGetListContextHonorsCanceledContext(t *testing.T) {
	db, _ := setupSystemDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := NewNoticeDAO(db).GetListContext(ctx, pagination.PageRequest{Page: 1, PageSize: 10}, nil, nil, "")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetListContext() error = %v, want context.Canceled", err)
	}
}

func TestNoticeDAOUsesInjectedDB(t *testing.T) {
	db, mock := newInjectedDictNoticeSeedDAOTestDB(t)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `notices` WHERE `notices`.`id` = ? ORDER BY `notices`.`id` LIMIT ?")).
		WithArgs(uint(9), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "content", "type", "status"}).
			AddRow(9, "Injected", "from injected db", 1, 1))

	notice, err := NewNoticeDAO(db).GetByIDContext(context.Background(), 9)
	if err != nil {
		t.Fatalf("GetByIDContext() error = %v", err)
	}
	if notice.ID != 9 || notice.Title != "Injected" {
		t.Fatalf("notice = %#v, want injected row", notice)
	}
}
