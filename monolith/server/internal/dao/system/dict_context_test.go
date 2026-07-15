package system

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestDictDAOGetTypeListContextHonorsCanceledContext(t *testing.T) {
	db, _ := setupSystemDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := NewDictDAO(db).GetTypeListContext(ctx, pagination.PageRequest{Page: 1, PageSize: 10}, "", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetTypeListContext() error = %v, want context.Canceled", err)
	}
}

func TestDictDAOUsesInjectedDB(t *testing.T) {
	db, mock := newInjectedDictNoticeSeedDAOTestDB(t)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "dict_types" WHERE code = $1 ORDER BY "dict_types"."id" LIMIT $2`)).
		WithArgs("gender", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code", "status"}).
			AddRow(7, "Gender", "gender", 1))

	dictType, err := NewDictDAO(db).GetTypeByCodeContext(context.Background(), "gender")
	if err != nil {
		t.Fatalf("GetTypeByCodeContext() error = %v", err)
	}
	if dictType.ID != 7 || dictType.Code != "gender" {
		t.Fatalf("dictType = %#v, want injected row", dictType)
	}
}

func newInjectedDictNoticeSeedDAOTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open injected sqlmock db: %v", err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{})
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
