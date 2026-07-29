package middleware

import (
	"context"
	"errors"
	"testing"

	"github.com/go-admin-kit/services/identity/internal/model"
	"github.com/go-admin-kit/services/identity/internal/pkg/cache"
	"github.com/go-admin-kit/services/identity/internal/pkg/jwt"
)

var errConsoleSessionRevokedForTest = errors.New("console session has been revoked")

// countingSessionValidator counts the DB-backed session validations.
type countingSessionValidator struct {
	calls int
	err   error
}

func (v *countingSessionValidator) ValidateActiveSessionContext(ctx context.Context, sessionID, username string) (*model.ConsoleSession, error) {
	v.calls++
	if v.err != nil {
		return nil, v.err
	}
	return &model.ConsoleSession{SessionID: sessionID, Username: username}, nil
}

type statusUserStore struct {
	calls  int
	status int8
	roles  []string
}

func (s *statusUserStore) GetUserWithRolesContext(ctx context.Context, id uint) (*model.User, error) {
	s.calls++
	user := &model.User{Status: s.status}
	user.ID = id
	for _, code := range s.roles {
		user.Roles = append(user.Roles, model.Role{Code: code})
	}
	return user, nil
}

func consoleClaims(sessionID string, userID uint, username string) *jwt.Claims {
	claims := &jwt.Claims{UserID: userID, Username: username}
	claims.ID = sessionID
	return claims
}

func newConsoleSessionDeps(t *testing.T, status int8, roles ...string) (*countingSessionValidator, *statusUserStore) {
	t.Helper()
	sessions := &countingSessionValidator{}
	users := &statusUserStore{status: status, roles: roles}
	restore := SetAuthMiddlewareDependencies(AuthMiddlewareDependencies{
		Users:           users,
		ConsoleSessions: sessions,
	})
	t.Cleanup(restore)
	return sessions, users
}

func TestConsoleSessionAuthorizedServesRepeatCallsFromCache(t *testing.T) {
	setupRateLimitTestRedis(t) // installs a miniredis-backed package client

	sessions, users := newConsoleSessionDeps(t, 1, "auditor")
	ctx := context.Background()
	claims := consoleClaims("session-hit", 42, "alice")

	for i := 0; i < 5; i++ {
		if !consoleSessionAuthorized(ctx, currentAuthDeps(), claims) {
			t.Fatalf("call %d: expected the session to be authorized", i)
		}
	}

	if sessions.calls != 1 {
		t.Fatalf("session validated %d times, want 1 (repeat requests must be served from cache)", sessions.calls)
	}
	if users.calls != 1 {
		t.Fatalf("user store hit %d times, want 1 (repeat requests must not re-read user+roles)", users.calls)
	}
}

// The first validation warms the role cache from the user it already loaded, so
// the role/permission middlewares later in the same chain query nothing.
func TestConsoleSessionAuthorizedWarmsRoleCache(t *testing.T) {
	setupRateLimitTestRedis(t)

	_, users := newConsoleSessionDeps(t, 1, "super_admin")
	ctx := context.Background()

	if !consoleSessionAuthorized(ctx, currentAuthDeps(), consoleClaims("session-warm", 7, "root")) {
		t.Fatal("expected the session to be authorized")
	}
	if !hasRoleContext(ctx, 7, "super_admin") {
		t.Fatal("expected super_admin to be resolvable after the session validation")
	}
	if users.calls != 1 {
		t.Fatalf("user store hit %d times, want 1 (the role check must reuse the warmed cache)", users.calls)
	}
}

// Logout and administrative revoke both funnel through
// ConsoleSessionService.RevokeBySessionIDContext, which drops this key. A stale
// entry surviving that call would leave a logged-out cookie usable.
func TestConsoleSessionAuthorizedRejectsAfterLogout(t *testing.T) {
	setupRateLimitTestRedis(t)

	sessions, _ := newConsoleSessionDeps(t, 1, "auditor")
	ctx := context.Background()
	claims := consoleClaims("session-logout", 11, "bob")

	if !consoleSessionAuthorized(ctx, currentAuthDeps(), claims) {
		t.Fatal("expected the session to be authorized before logout")
	}

	// Logout: the session row is revoked and the cache entry dropped.
	sessions.err = errConsoleSessionRevokedForTest
	if err := cache.NewCacheService().DelConsoleSessionContext(ctx, claims.ID); err != nil {
		t.Fatalf("invalidate console session cache: %v", err)
	}

	if consoleSessionAuthorized(ctx, currentAuthDeps(), claims) {
		t.Fatal("logged-out session still authorized; the cookie would keep working")
	}
}

// Force-logout and account disable route through the permission-cache channel
// (InvalidatePermissionCacheForUsersContext), which drops every cached session
// of the affected user.
func TestConsoleSessionAuthorizedRejectsAfterUserInvalidation(t *testing.T) {
	setupRateLimitTestRedis(t)

	sessions, users := newConsoleSessionDeps(t, 1, "auditor")
	ctx := context.Background()
	claims := consoleClaims("session-kick", 21, "carol")

	if !consoleSessionAuthorized(ctx, currentAuthDeps(), claims) {
		t.Fatal("expected the session to be authorized before the kick")
	}

	// The account is disabled: the session row is still live, but status flips.
	users.status = 0
	if err := cache.NewCacheService().DelConsoleSessionsForUsersContext(ctx, []uint{21}); err != nil {
		t.Fatalf("invalidate console sessions for user: %v", err)
	}

	if consoleSessionAuthorized(ctx, currentAuthDeps(), claims) {
		t.Fatal("disabled account still authorized after invalidation")
	}
	if sessions.calls != 2 {
		t.Fatalf("session validated %d times, want 2 (one per cache miss)", sessions.calls)
	}
}

// A cached entry is bound to the token that produced it: a mismatched user id
// must fall through to a full re-validation rather than be accepted.
func TestConsoleSessionAuthorizedRejectsMismatchedIdentity(t *testing.T) {
	setupRateLimitTestRedis(t)

	_, users := newConsoleSessionDeps(t, 1, "auditor")
	ctx := context.Background()

	if !consoleSessionAuthorized(ctx, currentAuthDeps(), consoleClaims("shared-id", 31, "dave")) {
		t.Fatal("expected the first session to be authorized")
	}

	users.status = 0
	if consoleSessionAuthorized(ctx, currentAuthDeps(), consoleClaims("shared-id", 32, "eve")) {
		t.Fatal("a different user rode another user's cached session entry")
	}
}

func TestConsoleSessionAuthorizedRejectsDisabledUserOnCacheMiss(t *testing.T) {
	setupRateLimitTestRedis(t)

	newConsoleSessionDeps(t, 0, "auditor")

	if consoleSessionAuthorized(context.Background(), currentAuthDeps(), consoleClaims("session-disabled", 51, "frank")) {
		t.Fatal("disabled account must never be authorized")
	}
}
