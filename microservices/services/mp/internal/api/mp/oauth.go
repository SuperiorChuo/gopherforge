package api

// 网页授权：前端/H5 拿到微信 code 后调本接口换 openid。服务号专属能力。

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// OAuthExchange POST /api/v1/mp/accounts/:id/oauth/exchange body {code}。
// 用 code 换 openid（appsecret 直换，不走中控 token）。
func (s *Server) OAuthExchange(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	if s.WX == nil || s.Secrets == nil {
		fail(c, http.StatusServiceUnavailable, "微信客户端/密钥未配置")
		return
	}
	acc, ok2 := s.wxAccount(c, u.TenantID)
	if !ok2 {
		return
	}
	if !requireService(c, acc) {
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Code == "" {
		fail(c, http.StatusBadRequest, "code 必填")
		return
	}
	appID, secret, err := s.Secrets.ResolveSecret(c.Request.Context(), acc)
	if err != nil {
		fail(c, http.StatusInternalServerError, "appsecret 未配置")
		return
	}
	res, err := s.WX.ExchangeCodeForOpenID(c.Request.Context(), appID, secret, body.Code)
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	ok(c, gin.H{"open_id": res.OpenID, "scope": res.Scope, "union_id": res.UnionID})
}
