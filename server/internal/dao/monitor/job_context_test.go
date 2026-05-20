package monitor

import (
	"context"
	"errors"
	"testing"
)

func TestJobDAOGetJobByIDContextHonorsCanceledContext(t *testing.T) {
	setupMonitorDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewJobDAO().GetJobByIDContext(ctx, 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetJobByIDContext() error = %v, want context.Canceled", err)
	}
}
