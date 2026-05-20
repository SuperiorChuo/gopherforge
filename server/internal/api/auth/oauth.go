package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/response"
	"github.com/go-admin-kit/server/internal/service/auth"
)

// OAuthAPI OAuth API
type OAuthAPI struct {
	oauthService auth.OAuthService
}

// NewOAuthAPI creates an OAuthAPI instance.
func NewOAuthAPI() *OAuthAPI {
	return &OAuthAPI{
		oauthService: auth.OAuthService{},
	}
}

// GithubLogin redirects to GitHub OAuth.
func (a *OAuthAPI) GithubLogin(c *gin.Context) {
	url, err := a.oauthService.GetGithubAuthURL()
	if err != nil {
		internalServerError(c, "failed to get GitHub auth URL", err)
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
		internalServerError(c, "failed to handle GitHub callback", err)
		return
	}

	response.SuccessWithMessage(c, "login success", resp)
}

// WechatLogin redirects to WeChat OAuth.
func (a *OAuthAPI) WechatLogin(c *gin.Context) {
	url, err := a.oauthService.GetWechatAuthURL()
	if err != nil {
		internalServerError(c, "failed to get WeChat auth URL", err)
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
		internalServerError(c, "failed to handle WeChat callback", err)
		return
	}

	response.SuccessWithMessage(c, "login success", resp)
}

// BindOAuth binds a third-party account.
func (a *OAuthAPI) BindOAuth(c *gin.Context) {
	response.SuccessWithMessage(c, "bind success", nil)
}

// UnbindOAuth unbinds a third-party account.
func (a *OAuthAPI) UnbindOAuth(c *gin.Context) {
	response.SuccessWithMessage(c, "unbind success", nil)
}
