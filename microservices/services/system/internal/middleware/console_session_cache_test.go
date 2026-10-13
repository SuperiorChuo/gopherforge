package middleware

import (
	"context"
	"errors"
	"testing"

	"github.com/go-admin-kit/services/shared/pkg/cache"
	"github.com/go-admin-kit/services/shared/pkg/jwt"
	model "github.com/go-admin-kit/services/shared/pkg/model"
)

var errConsoleSessionRevokedForTest = errors.New("console session has been revoked")

// countingSessionValidator 统计基于 DB 的会话校验次数。
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

// 首次校验会用已加载的用户回填角色缓存，因此同一链路中
// 后面的角色/权限中间件不再发起任何查询。
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

// 登出与管理端吊销都会经过 ConsoleSessionService.RevokeBySessionIDContext，
// 该方法会删除该键。若过期的缓存条目存活下来，已登出的 cookie 就仍可使用。
func TestConsoleSessionAuthorizedRejectsAfterLogout(t *testing.T) {
	setupRateLimitTestRedis(t)

	sessions, _ := newConsoleSessionDeps(t, 1, "auditor")
	ctx := context.Background()
	claims := consoleClaims("session-logout", 11, "bob")

	if !consoleSessionAuthorized(ctx, currentAuthDeps(), claims) {
		t.Fatal("expected the session to be authorized before logout")
	}

	// 登出：会话行被吊销，缓存条目被删除。
	sessions.err = errConsoleSessionRevokedForTest
	if err := cache.NewCacheService().DelConsoleSessionContext(ctx, claims.ID); err != nil {
		t.Fatalf("invalidate console session cache: %v", err)
	}

	if consoleSessionAuthorized(ctx, currentAuthDeps(), claims) {
		t.Fatal("logged-out session still authorized; the cookie would keep working")
	}
}

// 强制登出与账号禁用走权限缓存通道
// （InvalidatePermissionCacheForUsersContext），它会删除受影响用户的
// 所有缓存会话。
func TestConsoleSessionAuthorizedRejectsAfterUserInvalidation(t *testing.T) {
	setupRateLimitTestRedis(t)

	sessions, users := newConsoleSessionDeps(t, 1, "auditor")
	ctx := context.Background()
	claims := consoleClaims("session-kick", 21, "carol")

	if !consoleSessionAuthorized(ctx, currentAuthDeps(), claims) {
		t.Fatal("expected the session to be authorized before the kick")
	}

	// 账号被禁用：会话行仍然有效，但状态已翻转。
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

// 缓存条目与生成它的令牌绑定：不匹配的用户 ID 必须回退到
// 完整重新校验，而不能被直接接受。
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
