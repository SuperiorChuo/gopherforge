package system

import (
	"context"
	"errors"
	"testing"
)

func TestAuditLogDAOListLogsContextHonorsCanceledContext(t *testing.T) {
	setupSystemDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (&AuditLogDAO{}).ListLogsContext(ctx, AuditLogListQuery{Page: 1, PageSize: 10})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ListLogsContext() error = %v, want context.Canceled", err)
	}
}
