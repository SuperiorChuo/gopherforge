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
	"github.com/go-admin-kit/services/auth/internal/model"
	jwtpkg "github.com/go-admin-kit/services/auth/internal/pkg/jwt"
	redisstore "github.com/go-admin-kit/services/auth/internal/pkg/redis"
	"github.com/go-admin-kit/services/shared/pkg/consoleauth"
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
	oldClient := redisstore.Client
	client := goredis.NewClient(&goredis.Options{Addr: store.Addr()})
	redisstore.Client = client
	t.Cleanup(func() {
		_ = client.Close()
		redisstore.Client = oldClient
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
	user *model.User
	err  error
}

func (f *fakeUserStore) GetUserWithRolesContext(ctx context.Context, id uint) (*model.User, error) {
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
	recorder := performVerify(t, NewHandler(nil, nil), nil)

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

	recorder := performVerify(t, NewHandler(nil, nil), func(req *http.Request) {
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

	recorder := performVerify(t, NewHandler(nil, nil), func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer not-a-jwt")
	})

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestVerifyMalformedAuthorizationHeaderIsRejected(t *testing.T) {
	setVerifyJWTConfig(t)

	recorder := performVerify(t, NewHandler(nil, nil), func(req *http.Request) {
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

	recorder := performVerify(t, NewHandler(nil, nil), func(req *http.Request) {
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
		NewHandler(&fakeSessionValidator{}, &fakeUserStore{user: activeUser}),
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
		NewHandler(&fakeSessionValidator{err: errors.New("session revoked")}, &fakeUserStore{}),
		func(req *http.Request) {
			req.AddCookie(&http.Cookie{Name: consoleauth.SessionCookieName, Value: accessToken})
		})

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}
