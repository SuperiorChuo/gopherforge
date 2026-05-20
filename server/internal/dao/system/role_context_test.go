package system

import (
	"context"
	"errors"
	"testing"

	"github.com/go-admin-kit/server/internal/pkg/pagination"
)

func TestRoleDAOGetRoleListContextHonorsCanceledContext(t *testing.T) {
	setupSystemDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := (&RoleDAO{}).GetRoleListContext(ctx, pagination.PageRequest{Page: 1, PageSize: 10}, "")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetRoleListContext() error = %v, want context.Canceled", err)
	}
}
