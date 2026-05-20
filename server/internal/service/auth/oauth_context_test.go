package auth

import (
	"context"
	"errors"
	"testing"
)

func TestOAuthServiceFindOrCreateUserContextHonorsCanceledContext(t *testing.T) {
	setupAuthServiceContextTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (&OAuthService{}).findOrCreateUserContext(ctx, "github", "123", "alice", "alice@example.com", "")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("findOrCreateUserContext() error = %v, want context.Canceled", err)
	}
}
