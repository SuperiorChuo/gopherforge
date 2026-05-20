package system

import (
	"context"
	"errors"
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
