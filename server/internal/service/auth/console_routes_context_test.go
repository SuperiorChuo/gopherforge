package auth

import (
	"context"
	"errors"
	"testing"
)

func TestConsoleRouteServiceListRoutesContextHonorsCanceledContext(t *testing.T) {
	setupAuthServiceContextTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (ConsoleRouteService{}).ListRoutesContext(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ListRoutesContext() error = %v, want context.Canceled", err)
	}
}
