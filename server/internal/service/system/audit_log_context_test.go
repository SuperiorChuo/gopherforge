package system

import (
	"context"
	"errors"
	"testing"
)

func TestAuditLogServiceListLogsContextHonorsCanceledContext(t *testing.T) {
	setupSystemUserServiceContextTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (&AuditLogService{}).ListLogsContext(ctx, AuditLogListRequest{Page: 1, PageSize: 10})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ListLogsContext() error = %v, want context.Canceled", err)
	}
}
