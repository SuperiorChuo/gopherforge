package system

import (
	"context"
	"errors"
	"regexp"
	"testing"
)

func TestMenuServiceCreateMenuContextHonorsCanceledParentLookup(t *testing.T) {
	setupSystemUserServiceContextTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (&MenuService{}).CreateMenuContext(ctx, CreateMenuRequest{
		Name:     "system",
		Title:    "System",
		ParentID: 1,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CreateMenuContext() error = %v, want context.Canceled", err)
	}
}

func TestMenuServiceCreateMenuContextReturnsParentLookupError(t *testing.T) {
	mock := setupSystemUserServiceContextTestDB(t)
	lookupErr := errors.New("database lookup failed")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `menus` WHERE `menus`.`id` = ? ORDER BY `menus`.`id` LIMIT ?")).
		WithArgs(1, 1).
		WillReturnError(lookupErr)

	_, err := (&MenuService{}).CreateMenuContext(context.Background(), CreateMenuRequest{
		Name:     "system",
		Title:    "System",
		ParentID: 1,
	})
	if !errors.Is(err, lookupErr) {
		t.Fatalf("CreateMenuContext() error = %v, want parent lookup error", err)
	}
}
