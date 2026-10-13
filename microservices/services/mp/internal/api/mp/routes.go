package api

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/gatewayauth"
)

// RegisterRoutes mounts mp-service routes. 回调（callback）走 mp-open router
// 绕网关鉴权，自己做微信签名校验；其余后台接口经网关 auth-verify。
func (s *Server) RegisterRoutes(r *gin.Engine) {
	// health（无鉴权）
	r.GET("/api/v1/health/live", s.HealthLive)
	r.GET("/api/v1/health/ready", s.HealthReady)
	r.GET("/api/v1/mp/health/live", s.HealthLive)
	r.GET("/api/v1/mp/health/ready", s.HealthReady)

	// 微信回调（公开，微信签名校验）：GET 验签 / POST 收消息
	r.GET("/api/v1/mp/callback/:appid", s.CallbackVerify)
	r.POST("/api/v1/mp/callback/:appid", s.CallbackMessage)

	r.Use(gatewayauth.RequireReadWritePermissions("social:read", "social:write"))

	// 后台管理（网关 auth-verify，服务内 requireUser 兜底）
	// 账号 CRUD
	r.GET("/api/v1/mp/accounts", s.ListAccounts)
	r.POST("/api/v1/mp/accounts", s.CreateAccount)
	r.GET("/api/v1/mp/accounts/:id", s.GetAccount)
	r.PUT("/api/v1/mp/accounts/:id", s.UpdateAccount)
	r.DELETE("/api/v1/mp/accounts/:id", s.DeleteAccount)

	// 消息记录 / 粉丝列表
	r.GET("/api/v1/mp/messages", s.ListMessages)
	r.POST("/api/v1/mp/messages/:id/reply", s.ReplyMessage)
	r.GET("/api/v1/mp/fans", s.ListFans)

	// M4 微信主动能力（需 s.WX；未配置则 handler 判空降级）
	r.GET("/api/v1/mp/accounts/:id/menu", s.GetMenu)
	r.PUT("/api/v1/mp/accounts/:id/menu", s.SaveMenu)
	r.POST("/api/v1/mp/accounts/:id/menu", s.PublishMenu)
	r.POST("/api/v1/mp/accounts/:id/menu/publish", s.PublishSavedMenu)
	r.DELETE("/api/v1/mp/accounts/:id/menu", s.DeleteMenu)
	r.POST("/api/v1/mp/accounts/:id/customer-message", s.SendCustomerMessage)
	r.POST("/api/v1/mp/accounts/:id/customer-media", s.UploadCustomerMedia)
	r.GET("/api/v1/mp/accounts/:id/fans/sync", s.GetFanSync)
	r.POST("/api/v1/mp/accounts/:id/fans/sync", s.SyncFans)
	r.POST("/api/v1/mp/accounts/:id/oauth/exchange", s.OAuthExchange)

	// M5 AI 图文：生成 → 封面 → 编辑 → 发布
	r.GET("/api/v1/mp/articles", s.ListArticles)
	r.POST("/api/v1/mp/articles/generate", s.GenerateArticle)
	r.GET("/api/v1/mp/articles/:id", s.GetArticle)
	r.PUT("/api/v1/mp/articles/:id", s.UpdateArticle)
	r.DELETE("/api/v1/mp/articles/:id", s.DeleteArticle)
	r.POST("/api/v1/mp/articles/:id/cover", s.GenerateCover)
	r.POST("/api/v1/mp/articles/:id/publish", s.PublishArticle)

	// 社媒内容中台（阶段B）：平台无关内容 + 多渠道发布/导出
	r.GET("/api/v1/social/channels", s.ListChannels)
	r.GET("/api/v1/social/channel-accounts", s.ListChannelAccounts)
	r.GET("/api/v1/social/posts", s.ListPosts)
	r.POST("/api/v1/social/posts", s.CreatePost)
	r.POST("/api/v1/social/posts/generate", s.GeneratePost)
	r.GET("/api/v1/social/posts/:id", s.GetPost)
	r.PUT("/api/v1/social/posts/:id", s.UpdatePost)
	r.DELETE("/api/v1/social/posts/:id", s.DeletePost)
	r.POST("/api/v1/social/posts/:id/publish", s.PublishPost)
	r.POST("/api/v1/social/posts/:id/export", s.ExportPost)
}
