package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/auth/internal/pkg/response"
	authsvc "github.com/go-admin-kit/services/auth/internal/service/auth"
)

func TestOAuthAPIBindRequiresAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := &OAuthAPI{oauthService: &stubOAuthAPIService{}}
	router.POST("/oauth/bind", api.BindOAuth)

	req := httptest.NewRequest(http.MethodPost, "/oauth/bind", strings.NewReader(`{"provider":"github","code":"oauth-code"}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	assertErrorCode(t, recorder.Body.Bytes(), response.ErrorCodeAuthContextMissing)
}

func TestOAuthAPIBindPassesCurrentUserToService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &stubOAuthAPIService{}
	router := gin.New()
	api := &OAuthAPI{oauthService: service}
	router.POST("/oauth/bind", func(c *gin.Context) {
		c.Set("user_id", uint(42))
		api.BindOAuth(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/oauth/bind", strings.NewReader(`{"provider":"github","code":"oauth-code"}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if service.bindUserID != 42 {
		t.Fatalf("bind user id = %d, want 42", service.bindUserID)
	}
	if service.bindReq.Provider != "github" || service.bindReq.Code != "oauth-code" {
		t.Fatalf("bind request = %#v, want github/oauth-code", service.bindReq)
	}
}

func TestOAuthAPIUnbindMapsMissingBindingToNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &stubOAuthAPIService{unbindErr: authsvc.ErrOAuthBindingNotFound}
	router := gin.New()
	api := &OAuthAPI{oauthService: service}
	router.POST("/oauth/unbind", func(c *gin.Context) {
		c.Set("user_id", uint(42))
		api.UnbindOAuth(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/oauth/unbind", strings.NewReader(`{"provider":"github"}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
	assertErrorCode(t, recorder.Body.Bytes(), response.ErrorCodeAuthOAuthBindingNotFound)
}

func TestOAuthAPIInvalidJSONReturnsGenericBodyMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := &OAuthAPI{oauthService: &stubOAuthAPIService{}}
	router.POST("/oauth/bind", func(c *gin.Context) {
		c.Set("user_id", uint(42))
		api.BindOAuth(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/oauth/bind", strings.NewReader(`{"provider":`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}

	var payload response.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Message != "invalid request body" {
		t.Fatalf("message = %q, want invalid request body", payload.Message)
	}
}

func TestOAuthAPIGithubLoginMapsProviderUnavailableToServiceUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &stubOAuthAPIService{githubAuthURLErr: authsvc.ErrOAuthProviderUnavailable}
	router := gin.New()
	api := &OAuthAPI{oauthService: service}
	router.GET("/oauth/github/login", api.GithubLogin)

	req := httptest.NewRequest(http.MethodGet, "/oauth/github/login", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusServiceUnavailable, recorder.Body.String())
	}
	assertErrorCode(t, recorder.Body.Bytes(), response.ErrorCodeAuthOAuthProviderUnavailable)
}

func TestOAuthAPIGithubLoginRedirectsToProvider(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := &OAuthAPI{oauthService: &stubOAuthAPIService{}}
	router.GET("/oauth/github/login", api.GithubLogin)

	req := httptest.NewRequest(http.MethodGet, "/oauth/github/login", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusFound)
	}
	if got := recorder.Header().Get("Location"); got != "https://github.example.test/login" {
		t.Fatalf("Location = %q, want provider URL", got)
	}
}

type stubOAuthAPIService struct {
	bindReq    authsvc.BindOAuthRequest
	bindUserID uint
	bindErr    error

	unbindReq    authsvc.UnbindOAuthRequest
	unbindUserID uint
	unbindErr    error

	githubAuthURLErr error
}

func (s *stubOAuthAPIService) GetGithubAuthURLContext(ctx context.Context) (string, error) {
	if s.githubAuthURLErr != nil {
		return "", s.githubAuthURLErr
	}
	return "https://github.example.test/login", nil
}

func (s *stubOAuthAPIService) GithubCallbackContext(ctx context.Context, code, state string) (*authsvc.OAuthResponse, error) {
	return &authsvc.OAuthResponse{}, nil
}

func (s *stubOAuthAPIService) GetWechatAuthURLContext(ctx context.Context) (string, error) {
	return "https://wechat.example.test/login", nil
}

func (s *stubOAuthAPIService) WechatCallbackContext(ctx context.Context, code, state string) (*authsvc.OAuthResponse, error) {
	return &authsvc.OAuthResponse{}, nil
}

func (s *stubOAuthAPIService) BindOAuthContext(ctx context.Context, userID uint, req authsvc.BindOAuthRequest) error {
	s.bindUserID = userID
	s.bindReq = req
	return s.bindErr
}

func (s *stubOAuthAPIService) UnbindOAuthContext(ctx context.Context, userID uint, req authsvc.UnbindOAuthRequest) error {
	s.unbindUserID = userID
	s.unbindReq = req
	return s.unbindErr
}
