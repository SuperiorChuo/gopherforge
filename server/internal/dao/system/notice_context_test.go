package system

import (
	"context"
	"errors"
	"testing"

	"github.com/go-admin-kit/server/internal/pkg/pagination"
)

func TestNoticeDAOGetListContextHonorsCanceledContext(t *testing.T) {
	setupSystemDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := (&NoticeDAO{}).GetListContext(ctx, pagination.PageRequest{Page: 1, PageSize: 10}, nil, nil, "")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetListContext() error = %v, want context.Canceled", err)
	}
}
