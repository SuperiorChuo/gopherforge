package monitor

import (
	"context"
	"errors"
	"testing"
)

func TestServerServiceGetServerInfoContextHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewServerService().GetServerInfoContext(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetServerInfoContext() error = %v, want context.Canceled", err)
	}
}
