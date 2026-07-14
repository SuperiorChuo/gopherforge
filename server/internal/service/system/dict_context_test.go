package system

import (
	"context"
	"errors"
	"regexp"
	"testing"
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
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "dict_types" WHERE code = $1 ORDER BY "dict_types"."id" LIMIT $2`)).
		WithArgs("gender", 1).
		WillReturnError(lookupErr)

	svc := NewDictServiceWithDB(db)
	_, err := (&svc).GetMultipleDictDataContext(context.Background(), []string{"gender"})
	if !errors.Is(err, lookupErr) {
		t.Fatalf("GetMultipleDictDataContext() error = %v, want lookup error", err)
	}
}
