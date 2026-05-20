package system

import (
	"context"
	"errors"
	"testing"
)

func TestMenuDAOGetMenuTreeContextHonorsCanceledContext(t *testing.T) {
	setupSystemDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (&MenuDAO{}).GetMenuTreeContext(ctx, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetMenuTreeContext() error = %v, want context.Canceled", err)
	}
}
