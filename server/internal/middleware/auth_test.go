package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/config"
	"github.com/go-admin-kit/server/internal/pkg/consoleauth"
	jwtpkg "github.com/go-admin-kit/server/internal/pkg/jwt"
	"github.com/go-admin-kit/server/internal/pkg/response"
	jwtlib "github.com/golang-jwt/jwt/v5"
)

func TestHasAnyRequiredPermission(t *testing.T) {
	tests := []struct {
		name        string
		granted     []string
		required    []string
		wantAllowed bool
	}{
		{
			name:        "single required permission",
			granted:     []string{"system:user:list"},
			required:    []string{"system:user:list"},
			wantAllowed: true,
		},
		{
			name:        "any required permission",
			granted:     []string{"system:user:update"},
			required:    []string{"system:user:list", "system:user:update"},
			wantAllowed: true,
		},
		{
			name:        "wildcard permission",
			granted:     []string{"*:*:*"},
			required:    []string{"system:user:delete"},
			wantAllowed: true,
		},
		{
			name:        "missing permission",
			granted:     []string{"system:user:list"},
			required:    []string{"system:user:update", "system:user:delete"},
			wantAllowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasAnyRequiredPermission(tt.granted, tt.required)
			if got != tt.wantAllowed {
				t.Fatalf("hasAnyRequiredPermission() = %v, want %v", got, tt.wantAllowed)
			}
		})
	}
}

func TestAuthMiddlewareUsesStableErrorCodeForInvalidToken(t *testing.T) {
	recorder := requestThroughAuthMiddleware(t, "not-a-jwt")

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	assertAuthErrorCode(t, recorder.Body.Bytes(), response.ErrorCodeAuthTokenInvalid)
}

func TestAuthMiddlewareUsesStableErrorCodeForMissingAuthorizationHeader(t *testing.T) {
	recorder := requestThroughAuthMiddlewareWithoutCredentials(t)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	assertAuthErrorCode(t, recorder.Body.Bytes(), response.ErrorCodeAuthHeaderMissing)
}

func TestAuthMiddlewareUsesStableErrorCodeForInvalidAuthorizationHeader(t *testing.T) {
	recorder := requestThroughAuthMiddlewareWithAuthorizationHeader(t, "Basic token")

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	assertAuthErrorCode(t, recorder.Body.Bytes(), response.ErrorCodeAuthHeaderInvalid)
}

func TestAuthMiddlewareUsesStableErrorCodeForConsoleLoginRequired(t *testing.T) {
	setAuthMiddlewareJWTConfig(t)

	accessToken, _, err := jwtpkg.GenerateToken(42, "alice")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	recorder := requestThroughAuthMiddlewareWithCookie(t, accessToken)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	assertAuthErrorCode(t, recorder.Body.Bytes(), response.ErrorCodeConsoleLoginRequired)
}

func TestRoleMiddlewareUsesStableErrorCodeForMissingUserContext(t *testing.T) {
	recorder := requestThroughMiddleware(t, RoleMiddleware("admin"))

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	assertAuthErrorCode(t, recorder.Body.Bytes(), response.ErrorCodeAuthContextMissing)
}

func TestPermissionMiddlewareUsesStableErrorCodeForMissingUserContext(t *testing.T) {
	recorder := requestThroughMiddleware(t, PermissionMiddleware("system:user:list"))

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	assertAuthErrorCode(t, recorder.Body.Bytes(), response.ErrorCodeAuthContextMissing)
}

func TestAuthMiddlewareUsesStableErrorCodeForExpiredToken(t *testing.T) {
	setAuthMiddlewareJWTConfig(t)

	token := signedAuthMiddlewareToken(t, jwtpkg.Claims{
		UserID:    42,
		Username:  "alice",
		TokenType: jwtpkg.AccessTokenType,
		RegisteredClaims: jwtlib.RegisteredClaims{
			ExpiresAt: jwtlib.NewNumericDate(time.Now().Add(-time.Minute)),
			IssuedAt:  jwtlib.NewNumericDate(time.Now().Add(-2 * time.Minute)),
			NotBefore: jwtlib.NewNumericDate(time.Now().Add(-2 * time.Minute)),
			Issuer:    config.Cfg.JWT.Issuer,
			Subject:   "42",
			ID:        "expired-token-id",
		},
	})

	recorder := requestThroughAuthMiddleware(t, token)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	assertAuthErrorCode(t, recorder.Body.Bytes(), response.ErrorCodeAuthTokenExpired)
}

func TestAuthMiddlewareUsesStableErrorCodeForRevokedToken(t *testing.T) {
	setAuthMiddlewareJWTConfig(t)
	store := &authMiddlewareBlacklistStore{revoked: make(map[string]bool)}
	restoreStore := jwtpkg.SetTokenBlacklistStore(store)
	t.Cleanup(restoreStore)

	accessToken, _, err := jwtpkg.GenerateToken(42, "alice")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	claims, err := jwtpkg.ParseToken(accessToken)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	store.revoked[claims.ID] = true

	recorder := requestThroughAuthMiddleware(t, accessToken)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	assertAuthErrorCode(t, recorder.Body.Bytes(), response.ErrorCodeAuthTokenRevoked)
}

func TestAuthMiddlewareUsesStableErrorCodeForWrongTokenType(t *testing.T) {
	setAuthMiddlewareJWTConfig(t)

	_, refreshToken, err := jwtpkg.GenerateToken(42, "alice")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	recorder := requestThroughAuthMiddleware(t, refreshToken)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	assertAuthErrorCode(t, recorder.Body.Bytes(), response.ErrorCodeAuthTokenInvalid)
}

func requestThroughAuthMiddleware(t *testing.T, token string) *httptest.ResponseRecorder {
	t.Helper()
	return requestThroughAuthMiddlewareWithAuthorizationHeader(t, "Bearer "+token)
}

func requestThroughAuthMiddlewareWithAuthorizationHeader(t *testing.T, authHeader string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(AuthMiddleware())
	router.GET("/", func(c *gin.Context) {
		response.Success(c, nil)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", authHeader)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func requestThroughAuthMiddlewareWithoutCredentials(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(AuthMiddleware())
	router.GET("/", func(c *gin.Context) {
		response.Success(c, nil)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func requestThroughAuthMiddlewareWithCookie(t *testing.T, token string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(AuthMiddleware())
	router.GET("/", func(c *gin.Context) {
		response.Success(c, nil)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: consoleauth.SessionCookieName, Value: token})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func requestThroughMiddleware(t *testing.T, middleware gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(middleware)
	router.GET("/", func(c *gin.Context) {
		response.Success(c, nil)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func assertAuthErrorCode(t *testing.T, body []byte, want response.ErrorCode) {
	t.Helper()

	var payload response.Response
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.ErrorCode != want {
		t.Fatalf("error_code = %q, want %q; body=%s", payload.ErrorCode, want, string(body))
	}
}

func setAuthMiddlewareJWTConfig(t *testing.T) {
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

func signedAuthMiddlewareToken(t *testing.T, claims jwtpkg.Claims) string {
	t.Helper()

	token, err := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims).SignedString([]byte(config.Cfg.JWT.Secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}

type authMiddlewareBlacklistStore struct {
	revoked map[string]bool
}

func (s *authMiddlewareBlacklistStore) SetTokenID(_ context.Context, tokenID string, _ time.Duration) error {
	s.revoked[tokenID] = true
	return nil
}

func (s *authMiddlewareBlacklistStore) HasTokenID(_ context.Context, tokenID string) (bool, error) {
	return s.revoked[tokenID], nil
}
