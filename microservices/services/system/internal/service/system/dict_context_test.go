package system

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestDictServiceCreateTypeContextHonorsCanceledContext(t *testing.T) {
	db, _ := setupSystemUserServiceContextTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc := NewDictServiceWithDB(db)
	_, err := (&svc).CreateTypeContext(ctx, CreateDictTypeRequest{
		Name: "Gender",
		Code: "gender",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CreateTypeContext() error = %v, want context.Canceled", err)
	}
}

func TestDictServiceCreateTypeContextReturnsCodeLookupError(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	lookupErr := errors.New("database lookup failed")
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "dict_types" WHERE code = $1 ORDER BY "dict_types"."id" LIMIT $2`)).
		WithArgs("gender", 1).
		WillReturnError(lookupErr)

	svc := NewDictServiceWithDB(db)
	_, err := (&svc).CreateTypeContext(context.Background(), CreateDictTypeRequest{
		Name: "Gender",
		Code: "gender",
	})
	if !errors.Is(err, lookupErr) {
		t.Fatalf("CreateTypeContext() error = %v, want code lookup error", err)
	}
}

func TestDictServiceGetMultipleDictDataContextReturnsLookupError(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	lookupErr := errors.New("database lookup failed")
	// Codes are now resolved in one batched query instead of two per code.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "dict_types" WHERE code IN ($1) AND status = $2`)).
		WithArgs("gender", int8(1)).
		WillReturnError(lookupErr)

	svc := NewDictServiceWithDB(db)
	_, err := (&svc).GetMultipleDictDataContext(context.Background(), []string{"gender"})
	if !errors.Is(err, lookupErr) {
		t.Fatalf("GetMultipleDictDataContext() error = %v, want lookup error", err)
	}
}

func TestDictServiceGetMultipleDictDataContextUsesTwoQueriesForManyCodes(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	// Three codes used to cost six queries (two per code). Now: one for the
	// types, one for all of their items — regardless of code count.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "dict_types" WHERE code IN ($1,$2,$3) AND status = $4`)).
		WithArgs("gender", "status", "missing", int8(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "status"}).
			AddRow(uint(1), "gender", int8(1)).
			AddRow(uint(2), "status", int8(1)))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "dict_items" WHERE dict_type_id IN ($1,$2) AND status = $3 ORDER BY dict_type_id ASC, sort ASC, created_at ASC`)).
		WithArgs(uint(1), uint(2), int8(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "dict_type_id", "label", "value"}).
			AddRow(uint(11), uint(1), "男", "1").
			AddRow(uint(12), uint(1), "女", "2").
			AddRow(uint(21), uint(2), "启用", "1"))

	svc := NewDictServiceWithDB(db)
	data, err := (&svc).GetMultipleDictDataContext(context.Background(), []string{"gender", "status", "missing"})
	if err != nil {
		t.Fatalf("GetMultipleDictDataContext() error = %v", err)
	}
	if len(data) != 2 {
		t.Fatalf("GetMultipleDictDataContext() = %+v, want two codes (unknown code omitted)", data)
	}
	if len(data["gender"]) != 2 || data["gender"][0].Label != "男" {
		t.Fatalf("GetMultipleDictDataContext()[gender] = %+v, want two ordered items", data["gender"])
	}
	if len(data["status"]) != 1 {
		t.Fatalf("GetMultipleDictDataContext()[status] = %+v, want one item", data["status"])
	}
}

func TestDictServiceGetAllDictDataContextUsesTwoQueries(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	// /dicts/all was 1+N; it is now types + one grouped item query.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "dict_types" WHERE status = $1 ORDER BY created_at DESC`)).
		WithArgs(int8(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "status"}).
			AddRow(uint(1), "gender", int8(1)).
			AddRow(uint(2), "status", int8(1)))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "dict_items" WHERE dict_type_id IN ($1,$2) AND status = $3 ORDER BY dict_type_id ASC, sort ASC, created_at ASC`)).
		WithArgs(uint(1), uint(2), int8(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "dict_type_id", "label", "value"}).
			AddRow(uint(11), uint(1), "男", "1").
			AddRow(uint(21), uint(2), "启用", "1"))

	svc := NewDictServiceWithDB(db)
	data, err := (&svc).GetAllDictDataContext(context.Background())
	if err != nil {
		t.Fatalf("GetAllDictDataContext() error = %v", err)
	}
	if len(data) != 2 || len(data["gender"]) != 1 || len(data["status"]) != 1 {
		t.Fatalf("GetAllDictDataContext() = %+v, want two codes with one item each", data)
	}
}

func TestDictServiceGetDictDataContextReturnsNotFoundForUnknownCode(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "dict_types" WHERE code IN ($1) AND status = $2`)).
		WithArgs("nope", int8(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "status"}))

	svc := NewDictServiceWithDB(db)
	_, err := (&svc).GetDictDataContext(context.Background(), "nope")
	if !errors.Is(err, ErrDictTypeNotFound) {
		t.Fatalf("GetDictDataContext() error = %v, want ErrDictTypeNotFound", err)
	}
}

func TestNormalizeDictCodesTrimsAndDeduplicates(t *testing.T) {
	got := normalizeDictCodes([]string{" gender ", "gender", "", "  ", "status"})
	if len(got) != 2 || got[0] != "gender" || got[1] != "status" {
		t.Fatalf("normalizeDictCodes() = %#v, want [gender status]", got)
	}
	if len(normalizeDictCodes(nil)) != 0 {
		t.Fatalf("normalizeDictCodes(nil) should be empty")
	}
}
