package auth

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/auth/internal/events"
	"github.com/go-admin-kit/services/auth/internal/pkg/response"
	authsvc "github.com/go-admin-kit/services/auth/internal/service/auth"
)

type oauthService interface {
	GetGithubAuthURLContext(ctx context.Context) (string, error)
	GithubCallbackContext(ctx context.Context, code, state string) (*authsvc.OAuthResponse, error)
	GetWechatAuthURLContext(ctx context.Context) (string, error)
	WechatCallbackContext(ctx context.Context, code, state string) (*authsvc.OAuthResponse, error)
	BindOAuthContext(ctx context.Context, userID uint, req authsvc.BindOAuthRequest) error
	UnbindOAuthContext(ctx context.Context, userID uint, req authsvc.UnbindOAuthRequest) error
}

// OAuthAPI OAuth API
type OAuthAPI struct {
	oauthService oauthService
}

// NewOAuthAPI creates an OAuthAPI instance.
func NewOAuthAPI() *OAuthAPI {
	return &OAuthAPI{
		oauthService: &authsvc.OAuthService{},
	}
}

// NewOAuthAPIWithService creates an OAuthAPI instance from an injected service.
func NewOAuthAPIWithService(service *authsvc.OAuthService) *OAuthAPI {
	return &OAuthAPI{oauthService: service}
}

// GithubLogin redirects to GitHub OAuth.
func (a *OAuthAPI) GithubLogin(c *gin.Context) {
	url, err := a.oauthService.GetGithubAuthURLContext(c.Request.Context())
	if err != nil {
		writeAuthServiceError(c, "failed to get GitHub auth URL", err)
		return
	}
	c.Redirect(http.StatusFound, url)
}

// GithubCallback handles the GitHub OAuth callback.
func (a *OAuthAPI) GithubCallback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")

	resp, err := a.oauthService.GithubCallbackContext(c.Request.Context(), code, state)
	if err != nil {
		writeAuthServiceError(c, "failed to handle GitHub callback", err)
		return
	}

	if !resp.RequiresTOTP {
		publishLoginSuccess(c, resp.User.ID, resp.User.Username, events.LoginTypeOAuthGithub)
	}
	response.SuccessWithMessage(c, "login success", resp)
}

// WechatLogin redirects to WeChat OAuth.
func (a *OAuthAPI) WechatLogin(c *gin.Context) {
	url, err := a.oauthService.GetWechatAuthURLContext(c.Request.Context())
	if err != nil {
		writeAuthServiceError(c, "failed to get WeChat auth URL", err)
		return
	}
	c.Redirect(http.StatusFound, url)
}

// WechatCallback handles the WeChat OAuth callback.
func (a *OAuthAPI) WechatCallback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")

	resp, err := a.oauthService.WechatCallbackContext(c.Request.Context(), code, state)
	if err != nil {
		writeAuthServiceError(c, "failed to handle WeChat callback", err)
		return
	}

	if !resp.RequiresTOTP {
		publishLoginSuccess(c, resp.User.ID, resp.User.Username, events.LoginTypeOAuthWechat)
	}
	response.SuccessWithMessage(c, "login success", resp)
}

// BindOAuth binds a third-party account.
func (a *OAuthAPI) BindOAuth(c *gin.Context) {
	var req authsvc.BindOAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	userID, ok := currentOAuthUserID(c)
	if !ok {
		response.UnauthorizedWithCode(c, response.ErrorCodeAuthContextMissing, "user not found in context")
		return
	}
	if err := a.oauthService.BindOAuthContext(c.Request.Context(), userID, req); err != nil {
		writeAuthServiceError(c, "failed to bind OAuth account", err)
		return
	}
	response.SuccessWithMessage(c, "bind success", nil)
}

// UnbindOAuth unbinds a third-party account.
func (a *OAuthAPI) UnbindOAuth(c *gin.Context) {
	var req authsvc.UnbindOAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	userID, ok := currentOAuthUserID(c)
	if !ok {
		response.UnauthorizedWithCode(c, response.ErrorCodeAuthContextMissing, "user not found in context")
		return
	}
	if err := a.oauthService.UnbindOAuthContext(c.Request.Context(), userID, req); err != nil {
		writeAuthServiceError(c, "failed to unbind OAuth account", err)
		return
	}
	response.SuccessWithMessage(c, "unbind success", nil)
}

func currentOAuthUserID(c *gin.Context) (uint, bool) {
	value, exists := c.Get("user_id")
	if !exists {
		return 0, false
	}
	userID, ok := value.(uint)
	return userID, ok && userID != 0
}
