package system

import (
	"context"
	"errors"
	"testing"

	"github.com/go-admin-kit/server/internal/pkg/pagination"
)

func TestDictDAOGetTypeListContextHonorsCanceledContext(t *testing.T) {
	setupSystemDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := (&DictDAO{}).GetTypeListContext(ctx, pagination.PageRequest{Page: 1, PageSize: 10}, "", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetTypeListContext() error = %v, want context.Canceled", err)
	}
}
