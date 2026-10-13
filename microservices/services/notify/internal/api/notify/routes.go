package api

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/gatewayauth"
)

func (s *Server) RegisterRoutes(r *gin.Engine) {
	r.GET("/api/v1/health/live", s.HealthLive)
	r.GET("/api/v1/health/ready", s.HealthReady)
	r.GET("/api/v1/notify/health/live", s.HealthLive)
	r.GET("/api/v1/notify/health/ready", s.HealthReady)

	// internal (docker net + token)
	r.POST("/api/v1/notify/internal/send", s.InternalSend)
	r.POST("/api/v1/notify/internal/alerts", s.ReceiveAlerts)

	// user inbox (gateway auth, self-scoped)
	r.GET("/api/v1/notify/messages", s.ListMessages)
	r.GET("/api/v1/notify/messages/unread-count", s.UnreadCount)
	r.POST("/api/v1/notify/messages/:id/read", s.MarkRead)
	r.POST("/api/v1/notify/messages/read-all", s.MarkAllRead)
	r.GET("/api/v1/notify/channels", s.ChannelStatus)
	r.GET("/api/v1/notify/deliveries", s.ListMyDeliveryLogs)

	// 模板与全量投递日志是管理面，动作级 RBAC 由网关注入权限头。
	templateRead := gatewayauth.RequireAnyPermission("notify:template:read")
	templateWrite := gatewayauth.RequireAnyPermission("notify:template:write")
	r.GET("/api/v1/notify/templates", templateRead, s.ListTemplates)
	r.GET("/api/v1/notify/templates/:id", templateRead, s.GetTemplate)
	r.PUT("/api/v1/notify/templates/:id", templateWrite, s.UpdateTemplate)
	r.POST("/api/v1/notify/templates/:id/preview", templateWrite, s.PreviewTemplate)
	r.POST("/api/v1/notify/templates/:id/test-send", templateWrite, s.TestSendTemplate)
	r.GET("/api/v1/notify/deliveries/all", templateRead, s.AdminListDeliveryLogs)
}
