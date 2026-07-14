package system

import (
	"context"
	"errors"
	"regexp"
	"testing"
)

func TestMenuServiceCreateMenuContextHonorsCanceledParentLookup(t *testing.T) {
	db, _ := setupSystemUserServiceContextTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc := NewMenuServiceWithDB(db)
	_, err := (&svc).CreateMenuContext(ctx, CreateMenuRequest{
		Name:     "system",
		Title:    "System",
		ParentID: 1,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CreateMenuContext() error = %v, want context.Canceled", err)
	}
}

func TestMenuServiceCreateMenuContextReturnsParentLookupError(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	lookupErr := errors.New("database lookup failed")
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "menus" WHERE "menus"."id" = $1 ORDER BY "menus"."id" LIMIT $2`)).
		WithArgs(1, 1).
		WillReturnError(lookupErr)

	svc := NewMenuServiceWithDB(db)
	_, err := (&svc).CreateMenuContext(context.Background(), CreateMenuRequest{
		Name:     "system",
		Title:    "System",
		ParentID: 1,
	})
	if !errors.Is(err, lookupErr) {
		t.Fatalf("CreateMenuContext() error = %v, want parent lookup error", err)
	}
}
