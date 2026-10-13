package api

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/pay/internal/channel"
	sharedaudit "github.com/go-admin-kit/services/shared/pkg/audittrail"
	"github.com/go-admin-kit/services/shared/pkg/gatewayauth"
)

// RegisterRoutes mounts all pay routes.
// Gateway auth-verify intercepts管理面 at Traefik; requireUser is bare-connect fallback.
// pay-open router 放行：notify（M2 起）/cashier/health。
func (s *Server) RegisterRoutes(r *gin.Engine) {
	r.GET("/api/v1/health/live", s.HealthLive)
	r.GET("/api/v1/health/ready", s.HealthReady)
	r.GET("/api/v1/pay/health/live", s.HealthLive)
	r.GET("/api/v1/pay/health/ready", s.HealthReady)

	// 内部端点（X-Internal-Token）与匿名收银台先注册，不进入控制台 RBAC。
	r.POST("/api/v1/pay/internal/orders", s.InternalCreateOrder)
	r.POST("/api/v1/pay/internal/refunds", s.InternalCreateRefund)
	r.GET("/api/v1/pay/internal/orders/by-biz", s.InternalOrderByBiz)
	r.GET("/api/v1/pay/cashier/:token", s.CashierGet)
	r.POST("/api/v1/pay/cashier/:token/pay", s.CashierPay)
	if s.Registry.Has(channel.MockCode) {
		r.POST("/api/v1/pay/cashier/:token/mock", s.CashierMock)
	}

	r.Use(
		gatewayauth.RequireReadWritePermissions("pay:read", "pay:write"),
		sharedaudit.AuditHeaderMiddleware(),
	)

	// 管理面（ForwardAuth）
	r.GET("/api/v1/pay/stats", s.StatsHandler)
	r.GET("/api/v1/pay/channel-codes", s.ChannelCodes)
	r.GET("/api/v1/pay/channels", s.ListChannels)
	r.POST("/api/v1/pay/channels", s.CreateChannel)
	r.PUT("/api/v1/pay/channels/:id", s.UpdateChannel)
	r.DELETE("/api/v1/pay/channels/:id", s.DeleteChannel)
	r.GET("/api/v1/pay/orders", s.ListOrders)
	r.GET("/api/v1/pay/orders/:orderNo", s.GetOrder)
	r.POST("/api/v1/pay/orders/:orderNo/close", s.CloseOrder)
	r.GET("/api/v1/pay/refunds", s.ListRefunds)
	r.POST("/api/v1/pay/refunds", s.CreateRefund)
	r.GET("/api/v1/pay/callback-jobs", s.ListCallbackJobs)
	r.POST("/api/v1/pay/callback-jobs/:id/replay", s.ReplayCallbackJob)
	r.GET("/api/v1/pay/notify-logs", s.ListNotifyLogs)

}
