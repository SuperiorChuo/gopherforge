package system

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/services/system/internal/pkg/pagination"
)

func TestErrCodeDAOGetListContextHonorsCanceledContext(t *testing.T) {
	db, _ := setupSystemDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := NewErrCodeDAO(db).GetListContext(ctx, pagination.PageRequest{Page: 1, PageSize: 10}, "", "", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetListContext() error = %v, want context.Canceled", err)
	}
}

func TestErrCodeDAOGetAllEnabledContextUsesInjectedDB(t *testing.T) {
	db, mock := newInjectedDictNoticeSeedDAOTestDB(t)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "error_codes" WHERE status = $1 ORDER BY code ASC`)).
		WithArgs(int8(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "message", "scope", "status"}).
			AddRow(1, "DICT_TYPE_NOT_FOUND", "字典类型不存在", "system", 1).
			AddRow(2, "NOTICE_NOT_FOUND", "公告不存在或已下线", "system", 1))

	codes, err := NewErrCodeDAO(db).GetAllEnabledContext(context.Background())
	if err != nil {
		t.Fatalf("GetAllEnabledContext() error = %v", err)
	}
	if len(codes) != 2 || codes[0].Code != "DICT_TYPE_NOT_FOUND" || codes[1].Message != "公告不存在或已下线" {
		t.Fatalf("codes = %#v, want 2 injected rows", codes)
	}
}
