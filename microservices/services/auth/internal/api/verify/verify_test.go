package verify

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/auth/internal/config"
	"github.com/go-admin-kit/services/auth/internal/pkg/cache"
	"github.com/go-admin-kit/services/shared/pkg/consoleauth"
	jwtpkg "github.com/go-admin-kit/services/shared/pkg/jwt"
	model "github.com/go-admin-kit/services/shared/pkg/model"
	redisstore "github.com/go-admin-kit/services/shared/pkg/redis"
	goredis "github.com/redis/go-redis/v9"
)

func setVerifyJWTConfig(t *testing.T) {
	t.Helper()

	oldConfig := config.Cfg.JWT
	config.Cfg.JWT = config.JWTConfig{
		Secret:               "unit-test-secret-at-least-32-characters",
		AccessTokenExpire:    3600,
		RefreshTokenExpire:   7200,
		RefreshTokenRotation: true,
		Issuer:               "unit-test",
	}
	t.Cleanup(func() {
		config.Cfg.JWT = oldConfig
	})
}

func setupVerifyTestRedis(t *testing.T) {
	t.Helper()

	store, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	client := goredis.NewClient(&goredis.Options{Addr: store.Addr()})
	redisstore.Client = client
	jwtpkg.SetRedis(client)
	t.Cleanup(func() {
		_ = client.Close()
		redisstore.Client = nil
		jwtpkg.SetRedis(nil)
		store.Close()
	})
}

type fakeSessionValidator struct {
	err error
}

func (f *fakeSessionValidator) ValidateActiveSessionContext(ctx context.Context, sessionID, username string) (*model.ConsoleSession, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &model.ConsoleSession{SessionID: sessionID, Username: username}, nil
}

type fakeUserStore struct {
	user  *model.User
	err   error
	calls int
}

type fakePermissionStore struct {
	codes []string
	err   error
	calls int
}

func (f *fakePermissionStore) GetUserPermissionsContext(context.Context, uint) ([]string, error) {
	f.calls++
	return f.codes, f.err
}

func (f *fakeUserStore) GetUserWithRolesContext(ctx context.Context, id uint) (*model.User, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.user, nil
}

func performVerify(t *testing.T, handler *Handler, mutate func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/internal/verify", handler.Verify)

	req := httptest.NewRequest(http.MethodGet, "/internal/verify", nil)
	if mutate != nil {
		mutate(req)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func TestVerifyAnonymousRequestPassesThrough(t *testing.T) {
	recorder := performVerify(t, NewHandler(nil, nil, nil), nil)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get(HeaderUserID); got != "" {
		t.Fatalf("X-Auth-User-ID = %q, want empty for anonymous request", got)
	}
}

func TestVerifyValidBearerTokenInjectsIdentityHeaders(t *testing.T) {
	setVerifyJWTConfig(t)
	setupVerifyTestRedis(t)

	accessToken, _, err := jwtpkg.GenerateToken(42, "alice")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	recorder := performVerify(t, NewHandler(nil, nil, nil), func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if got := recorder.Header().Get(HeaderUserID); got != "42" {
		t.Fatalf("X-Auth-User-ID = %q, want 42", got)
	}
	if got := recorder.Header().Get(HeaderUsername); got != "alice" {
		t.Fatalf("X-Auth-Username = %q, want alice", got)
	}
}

func TestVerifyInvalidBearerTokenIsRejected(t *testing.T) {
	setVerifyJWTConfig(t)

	recorder := performVerify(t, NewHandler(nil, nil, nil), func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer not-a-jwt")
	})

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestVerifyMalformedAuthorizationHeaderIsRejected(t *testing.T) {
	setVerifyJWTConfig(t)

	recorder := performVerify(t, NewHandler(nil, nil, nil), func(req *http.Request) {
		req.Header.Set("Authorization", "Basic abc")
	})

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestVerifyRevokedTokenIsRejected(t *testing.T) {
	setVerifyJWTConfig(t)
	setupVerifyTestRedis(t)

	accessToken, _, err := jwtpkg.GenerateToken(42, "alice")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	claims, err := jwtpkg.ParseTokenContext(context.Background(), accessToken)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if err := jwtpkg.RevokeTokenContext(context.Background(), accessToken, claims); err != nil {
		t.Fatalf("revoke token: %v", err)
	}

	recorder := performVerify(t, NewHandler(nil, nil, nil), func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	})

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestVerifyCookieTokenValidatesConsoleSession(t *testing.T) {
	setVerifyJWTConfig(t)
	setupVerifyTestRedis(t)

	accessToken, _, err := jwtpkg.GenerateToken(42, "alice")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	activeUser := &model.User{Username: "alice", Status: 1}
	activeUser.ID = 42

	recorder := performVerify(t,
		NewHandler(&fakeSessionValidator{}, &fakeUserStore{user: activeUser}, nil),
		func(req *http.Request) {
			req.AddCookie(&http.Cookie{Name: consoleauth.SessionCookieName, Value: accessToken})
		})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if got := recorder.Header().Get(HeaderUsername); got != "alice" {
		t.Fatalf("X-Auth-Username = %q, want alice", got)
	}
}

func TestVerifyCookieTokenWithRevokedSessionIsRejected(t *testing.T) {
	setVerifyJWTConfig(t)
	setupVerifyTestRedis(t)

	accessToken, _, err := jwtpkg.GenerateToken(42, "alice")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	recorder := performVerify(t,
		NewHandler(&fakeSessionValidator{err: errors.New("session revoked")}, &fakeUserStore{}, nil),
		func(req *http.Request) {
			req.AddCookie(&http.Cookie{Name: consoleauth.SessionCookieName, Value: accessToken})
		})

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestVerifyInjectsSortedPermissionsHeader(t *testing.T) {
	setVerifyJWTConfig(t)
	setupVerifyTestRedis(t)

	accessToken, _, err := jwtpkg.GenerateToken(42, "alice")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	activeUser := &model.User{Username: "alice", Status: 1}
	activeUser.ID = 42

	recorder := performVerify(t,
		NewHandler(nil, &fakeUserStore{user: activeUser}, &fakePermissionStore{codes: []string{"crm:write", "crm:read", "crm:read"}}),
		func(req *http.Request) { req.Header.Set("Authorization", "Bearer "+accessToken) },
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if got := recorder.Header().Get(HeaderPermissions); got != "crm:read,crm:write" {
		t.Fatalf("%s = %q, want sorted unique permissions", HeaderPermissions, got)
	}
}

func TestVerifyCompressesSuperAdminPermissions(t *testing.T) {
	setVerifyJWTConfig(t)
	setupVerifyTestRedis(t)

	accessToken, _, err := jwtpkg.GenerateToken(42, "root")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	activeUser := &model.User{Username: "root", Status: 1, Roles: []model.Role{{Code: "super_admin"}}}
	activeUser.ID = 42

	recorder := performVerify(t,
		NewHandler(nil, &fakeUserStore{user: activeUser}, &fakePermissionStore{}),
		func(req *http.Request) { req.Header.Set("Authorization", "Bearer "+accessToken) },
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if got := recorder.Header().Get(HeaderPermissions); got != "*" {
		t.Fatalf("%s = %q, want wildcard", HeaderPermissions, got)
	}
}

// 归一化权限头缓存命中时不得触碰用户/权限存储（ForwardAuth 热路径收敛为单次 GET）。
func TestVerifyPermissionsHeaderCacheHitSkipsStores(t *testing.T) {
	setVerifyJWTConfig(t)
	setupVerifyTestRedis(t)

	accessToken, _, err := jwtpkg.GenerateToken(42, "alice")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if err := cache.NewCacheService().SetUserPermHeaderContext(context.Background(), 42, "crm:read"); err != nil {
		t.Fatalf("seed perm header cache: %v", err)
	}

	users := &fakeUserStore{err: errors.New("user store must not be hit on cache hit")}
	permissions := &fakePermissionStore{err: errors.New("permission store must not be hit on cache hit")}
	recorder := performVerify(t, NewHandler(nil, users, permissions), func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if got := recorder.Header().Get(HeaderPermissions); got != "crm:read" {
		t.Fatalf("%s = %q, want cached header", HeaderPermissions, got)
	}
	if users.calls != 0 || permissions.calls != 0 {
		t.Fatalf("stores hit on cache hit: users=%d permissions=%d, want 0/0", users.calls, permissions.calls)
	}
}

// 零角色/零权限用户的空头也要进缓存：曾因 SET 结构存不了空集，这类用户每请求穿透 DB。
func TestVerifyCachesEmptyPermissionsHeader(t *testing.T) {
	setVerifyJWTConfig(t)
	setupVerifyTestRedis(t)

	accessToken, _, err := jwtpkg.GenerateToken(42, "alice")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	noRoleUser := &model.User{Username: "alice", Status: 1}
	noRoleUser.ID = 42
	users := &fakeUserStore{user: noRoleUser}
	permissions := &fakePermissionStore{}
	handler := NewHandler(nil, users, permissions)

	for i := 0; i < 2; i++ {
		recorder := performVerify(t, handler, func(req *http.Request) {
			req.Header.Set("Authorization", "Bearer "+accessToken)
		})
		if recorder.Code != http.StatusOK {
			t.Fatalf("request %d status = %d, want %d, body = %s", i+1, recorder.Code, http.StatusOK, recorder.Body.String())
		}
		if got := recorder.Header().Get(HeaderPermissions); got != "" {
			t.Fatalf("request %d %s = %q, want empty", i+1, HeaderPermissions, got)
		}
	}

	// 第二个请求必须命中空头缓存：存储只在首个请求被读一次
	if users.calls != 1 || permissions.calls != 1 {
		t.Fatalf("stores hit = users %d / permissions %d, want 1/1 (second request must hit empty-header cache)", users.calls, permissions.calls)
	}
}
