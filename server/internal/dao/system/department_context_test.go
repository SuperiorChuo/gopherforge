package system

import (
	"context"
	"errors"
	"testing"
)

func TestDepartmentDAOGetTreeContextHonorsCanceledContext(t *testing.T) {
	setupSystemDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (&DepartmentDAO{}).GetTreeContext(ctx, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetTreeContext() error = %v, want context.Canceled", err)
	}
}
