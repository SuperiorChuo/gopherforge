package system

import (
	"context"
	"errors"
	"testing"
)

func TestAuditLogServiceListLogsContextHonorsCanceledContext(t *testing.T) {
	db, _ := setupSystemUserServiceContextTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc := NewAuditLogServiceWithDB(db)
	_, err := (&svc).ListLogsContext(ctx, AuditLogListRequest{Page: 1, PageSize: 10})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ListLogsContext() error = %v, want context.Canceled", err)
	}
}
