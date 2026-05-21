package system

import (
	"context"
	"errors"
	"regexp"
	"testing"
)

func TestDictServiceCreateTypeContextHonorsCanceledContext(t *testing.T) {
	setupSystemUserServiceContextTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (&DictService{}).CreateTypeContext(ctx, CreateDictTypeRequest{
		Name: "Gender",
		Code: "gender",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CreateTypeContext() error = %v, want context.Canceled", err)
	}
}

func TestDictServiceCreateTypeContextReturnsCodeLookupError(t *testing.T) {
	mock := setupSystemUserServiceContextTestDB(t)
	lookupErr := errors.New("database lookup failed")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `dict_types` WHERE code = ? ORDER BY `dict_types`.`id` LIMIT ?")).
		WithArgs("gender", 1).
		WillReturnError(lookupErr)

	_, err := (&DictService{}).CreateTypeContext(context.Background(), CreateDictTypeRequest{
		Name: "Gender",
		Code: "gender",
	})
	if !errors.Is(err, lookupErr) {
		t.Fatalf("CreateTypeContext() error = %v, want code lookup error", err)
	}
}

func TestDictServiceGetMultipleDictDataContextReturnsLookupError(t *testing.T) {
	mock := setupSystemUserServiceContextTestDB(t)
	lookupErr := errors.New("database lookup failed")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `dict_types` WHERE code = ? ORDER BY `dict_types`.`id` LIMIT ?")).
		WithArgs("gender", 1).
		WillReturnError(lookupErr)

	_, err := (&DictService{}).GetMultipleDictDataContext(context.Background(), []string{"gender"})
	if !errors.Is(err, lookupErr) {
		t.Fatalf("GetMultipleDictDataContext() error = %v, want lookup error", err)
	}
}
