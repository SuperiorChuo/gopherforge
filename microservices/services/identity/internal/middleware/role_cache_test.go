package middleware

import (
	"context"
	"testing"

	"github.com/go-admin-kit/services/identity/internal/model"
	"github.com/go-admin-kit/services/identity/internal/pkg/cache"
)

// countingUserStore records how many times the DB-backed lookup was hit.
type countingUserStore struct {
	calls int
	roles []string
}

func (s *countingUserStore) GetUserWithRolesContext(ctx context.Context, id uint) (*model.User, error) {
	s.calls++
	user := &model.User{Status: 1}
	user.ID = id
	for _, code := range s.roles {
		user.Roles = append(user.Roles, model.Role{Code: code})
	}
	return user, nil
}

func TestUserRoleCodesCachesAfterFirstLookup(t *testing.T) {
	setupRateLimitTestRedis(t) // installs a miniredis-backed package client

	store := &countingUserStore{roles: []string{"super_admin", "auditor"}}
	restore := SetAuthMiddlewareDependencies(AuthMiddlewareDependencies{Users: store})
	t.Cleanup(restore)

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if !hasRoleContext(ctx, 42, "super_admin") {
			t.Fatalf("call %d: expected super_admin to be granted", i)
		}
	}

	if store.calls != 1 {
		t.Fatalf("user store hit %d times, want 1 (subsequent calls must be served from cache)", store.calls)
	}
}

func TestUserRoleCodesRefetchAfterInvalidation(t *testing.T) {
	setupRateLimitTestRedis(t)

	store := &countingUserStore{roles: []string{"super_admin"}}
	restore := SetAuthMiddlewareDependencies(AuthMiddlewareDependencies{Users: store})
	t.Cleanup(restore)

	ctx := context.Background()
	if !hasRoleContext(ctx, 7, "super_admin") {
		t.Fatal("expected super_admin before revocation")
	}

	// Revoke the role and drop the cache the same way the service layer does.
	store.roles = []string{"viewer"}
	if err := cache.NewCacheService().DelUserRolesContext(ctx, 7); err != nil {
		t.Fatalf("invalidate role cache: %v", err)
	}

	if hasRoleContext(ctx, 7, "super_admin") {
		t.Fatal("revoked super_admin still granted after cache invalidation; privilege escalation window")
	}
	if store.calls != 2 {
		t.Fatalf("user store hit %d times, want 2 (one per cache miss)", store.calls)
	}
}

func TestUserRoleCodesDeniesWhenStoreMissing(t *testing.T) {
	setupRateLimitTestRedis(t)

	restore := SetAuthMiddlewareDependencies(AuthMiddlewareDependencies{})
	t.Cleanup(restore)

	if hasRoleContext(context.Background(), 9, "super_admin") {
		t.Fatal("role check must deny when the user store is unavailable")
	}
}
