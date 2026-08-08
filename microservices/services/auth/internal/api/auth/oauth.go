package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/auth/internal/events"
	authsvc "github.com/go-admin-kit/services/auth/internal/service/auth"
	"github.com/go-admin-kit/services/shared/pkg/response"
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

// oauthDeviceCookie stashes the X-Device-ID header as a cookie when OAuth
// login is initiated — the callback comes back via a browser redirect from
// the provider, which cannot carry a custom header, so we read it from cookie.
const oauthDeviceCookie = "oauth_device_id"

func oauthDeviceIDFromRequest(c *gin.Context) string {
	if id := strings.TrimSpace(c.GetHeader("X-Device-ID")); id != "" {
		return id
	}
	if id, err := c.Cookie(oauthDeviceCookie); err == nil {
		return strings.TrimSpace(id)
	}
	return ""
}

func stashOAuthDeviceID(c *gin.Context) {
	if id := strings.TrimSpace(c.GetHeader("X-Device-ID")); id != "" {
		c.SetCookie(oauthDeviceCookie, id, 10*60, "/", "", false, true) // 10 min, same-origin, HttpOnly
	}
}

// GithubLogin redirects to GitHub OAuth.
func (a *OAuthAPI) GithubLogin(c *gin.Context) {
	stashOAuthDeviceID(c)
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
		publishLoginSuccess(c, resp.User.ID, resp.User.Username, events.LoginTypeOAuthGithub, resp.User.TenantID, oauthDeviceIDFromRequest(c))
	}
	response.SuccessWithMessage(c, "login success", resp)
}

// WechatLogin redirects to WeChat OAuth.
func (a *OAuthAPI) WechatLogin(c *gin.Context) {
	stashOAuthDeviceID(c)
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
		publishLoginSuccess(c, resp.User.ID, resp.User.Username, events.LoginTypeOAuthWechat, resp.User.TenantID, oauthDeviceIDFromRequest(c))
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
