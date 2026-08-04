package audittrail

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

// 网关 forwardAuth 注入的可信头（与 gatewayauth 一致）。
const (
	HeaderUserID   = "X-Auth-User-ID"
	HeaderUsername = "X-Auth-Username"
	HeaderTenantID = "X-Auth-Tenant-ID"
)

// AuditHeaderMiddleware 从网关注入的 X-Auth-* 头把 actor/tenant 设置进请求 ctx，
// 供 GORM 审计插件记录业务写操作。业务服务没有 JWT 解析中间件（网关 forwardAuth
// 已验证并注入这些头），这是 actor 的唯一来源。
//
// 匿名 / 无 user id 的请求不注入（插件在无 actor 时静默跳过）——公开注册、
// 后台定时循环等不产生审计行，语义正确。
func AuditHeaderMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.GetHeader(HeaderUserID)
		if uid == "" {
			c.Next()
			return
		}
		if _, err := strconv.ParseUint(uid, 10, 64); err != nil {
			c.Next()
			return
		}
		actorID := uid
		if username := c.GetHeader(HeaderUsername); username != "" {
			actorID = username
		}
		tenantID := uint(1)
		if tid := c.GetHeader(HeaderTenantID); tid != "" {
			if n, err := strconv.ParseUint(tid, 10, 64); err == nil && n > 0 {
				tenantID = uint(n)
			}
		}
		ctx := WithActor(c.Request.Context(), "operator", actorID)
		ctx = WithTenantID(ctx, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
