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

// NewOAuthAPI 创建OAuthAPI实例
func NewOAuthAPI() *OAuthAPI {
	return &OAuthAPI{
		oauthService: auth.OAuthService{},
	}
}

// GithubLogin GitHub登录
func (a *OAuthAPI) GithubLogin(c *gin.Context) {
	url, err := a.oauthService.GetGithubAuthURL()
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}
	c.Redirect(http.StatusFound, url)
}

// GithubCallback GitHub登录回调
func (a *OAuthAPI) GithubCallback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")

	resp, err := a.oauthService.GithubCallback(code, state)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "login success", resp)
}

// WechatLogin 微信登录
func (a *OAuthAPI) WechatLogin(c *gin.Context) {
	url, err := a.oauthService.GetWechatAuthURL()
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}
	c.Redirect(http.StatusFound, url)
}

// WechatCallback 微信登录回调
func (a *OAuthAPI) WechatCallback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")

	resp, err := a.oauthService.WechatCallback(code, state)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "login success", resp)
}

// BindOAuth 绑定第三方账号
func (a *OAuthAPI) BindOAuth(c *gin.Context) {
	// 实现绑定第三方账号逻辑
	response.SuccessWithMessage(c, "bind success", nil)
}

// UnbindOAuth 解绑第三方账号
func (a *OAuthAPI) UnbindOAuth(c *gin.Context) {
	// 实现解绑第三方账号逻辑
	response.SuccessWithMessage(c, "unbind success", nil)
}
