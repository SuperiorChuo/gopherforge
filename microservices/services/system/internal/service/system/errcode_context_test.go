package system

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"
)

func TestErrCodeServiceCreateContextHonorsCanceledContext(t *testing.T) {
	db, _ := setupSystemUserServiceContextTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc := NewErrCodeServiceWithDB(db)
	_, err := svc.CreateContext(ctx, CreateErrCodeRequest{
		Code:    "DEMO_ERROR",
		Message: "示例文案",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CreateContext() error = %v, want context.Canceled", err)
	}
}

func TestErrCodeServiceCreateContextRejectsDuplicateCode(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "error_codes" WHERE code = $1 ORDER BY "error_codes"."id" LIMIT $2`)).
		WithArgs("DICT_TYPE_NOT_FOUND", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "message", "status"}).
			AddRow(1, "DICT_TYPE_NOT_FOUND", "字典类型不存在", 1))

	svc := NewErrCodeServiceWithDB(db)
	_, err := svc.CreateContext(context.Background(), CreateErrCodeRequest{
		Code:    "DICT_TYPE_NOT_FOUND",
		Message: "重复的错误码",
	})
	if !errors.Is(err, ErrErrorCodeAlreadyExists) {
		t.Fatalf("CreateContext() error = %v, want ErrErrorCodeAlreadyExists", err)
	}
}

func TestErrCodeServiceCreateContextInsertsRow(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "error_codes" WHERE code = $1 ORDER BY "error_codes"."id" LIMIT $2`)).
		WithArgs("SYSTEM_DEMO_ERROR", 1).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "error_codes" ("code","message","memo","scope","status","created_at","updated_at") VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING "id"`)).
		WithArgs("SYSTEM_DEMO_ERROR", "示例文案", "内部备注", "system", int8(1), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(9))
	mock.ExpectCommit()

	svc := NewErrCodeServiceWithDB(db)
	created, err := svc.CreateContext(context.Background(), CreateErrCodeRequest{
		Code:    "SYSTEM_DEMO_ERROR",
		Message: "示例文案",
		Memo:    "内部备注",
		Scope:   "system",
	})
	if err != nil {
		t.Fatalf("CreateContext() error = %v", err)
	}
	if created.ID != 9 || created.Status != 1 || created.Scope != "system" {
		t.Fatalf("created = %#v, want id=9 status=1 scope=system", created)
	}
}

func TestErrCodeServiceUpdateContextReturnsNotFound(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "error_codes" WHERE "error_codes"."id" = $1 ORDER BY "error_codes"."id" LIMIT $2`)).
		WithArgs(404, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	svc := NewErrCodeServiceWithDB(db)
	_, err := svc.UpdateContext(context.Background(), 404, UpdateErrCodeRequest{Message: "新文案"})
	if !errors.Is(err, ErrErrorCodeNotFound) {
		t.Fatalf("UpdateContext() error = %v, want ErrErrorCodeNotFound", err)
	}
}

func TestErrCodeServiceDeleteContextReturnsNotFound(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "error_codes" WHERE "error_codes"."id" = $1 ORDER BY "error_codes"."id" LIMIT $2`)).
		WithArgs(404, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	svc := NewErrCodeServiceWithDB(db)
	if err := svc.DeleteContext(context.Background(), 404); !errors.Is(err, ErrErrorCodeNotFound) {
		t.Fatalf("DeleteContext() error = %v, want ErrErrorCodeNotFound", err)
	}
}
