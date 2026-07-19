// Package api wires the audit service HTTP surface. The /api/v1 layout
// matches the monolith exactly for every extracted route so the gateway can
// switch traffic over without any client change.
package api

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/audit/internal/api/common"
	sharedapi "github.com/go-admin-kit/services/audit/internal/api/shared"
	"github.com/go-admin-kit/services/audit/internal/api/system"
	"github.com/go-admin-kit/services/audit/internal/middleware"
	systemsvc "github.com/go-admin-kit/services/audit/internal/service/system"
)

// SetupRoutes mounts the audit service API using legacy global fallbacks.
func SetupRoutes(router *gin.Engine) {
	SetupRoutesWithDeps(router, sharedapi.Dependencies{})
}

// SetupRoutesWithDeps mounts the API with injected infrastructure handles.
func SetupRoutesWithDeps(router *gin.Engine, deps sharedapi.Dependencies) {
	api := router.Group("/api/v1")

	common.RegisterPublicRoutesWithDeps(api, deps)

	loginLogAPI := system.NewLoginLogAPI()
	opLogAPI := system.NewOperationLogAPI()
	auditLogAPI := system.NewAuditLogAPI()
	if deps.DB != nil {
		loginLogAPI = system.NewLoginLogAPIWithService(systemsvc.NewLoginLogServiceWithDB(deps.DB))
		opLogAPI = system.NewOperationLogAPIWithService(systemsvc.NewOperationLogServiceWithDB(deps.DB))
		auditLogAPI = system.NewAuditLogAPIWithService(systemsvc.NewAuditLogServiceWithDB(deps.DB))
	}

	protected := api.Group("/")
	protected.Use(middleware.AuthMiddleware(), middleware.OperationLogger())
	{
		protected.GET("/login-logs", middleware.PermissionMiddleware("system:log:login"), loginLogAPI.GetLoginLogs)
		protected.GET("/login-logs/my", loginLogAPI.GetMyLoginLogs)
		protected.GET("/login-logs/stats", middleware.PermissionMiddleware("system:log:login"), loginLogAPI.GetLoginStats)
		protected.GET("/login-logs/trend", middleware.PermissionMiddleware("system:log:login"), loginLogAPI.GetLoginTrend)
		protected.GET("/login-logs/last", loginLogAPI.GetLastLogin)
		protected.GET("/login-logs/user/:user_id", middleware.PermissionMiddleware("system:log:login"), loginLogAPI.GetUserLoginHistory)
		protected.DELETE("/login-logs/clear", middleware.PermissionMiddleware("system:log:login"), loginLogAPI.ClearLoginLogs)

		protected.GET("/operation-logs", middleware.PermissionMiddleware("system:log:operation"), opLogAPI.GetOperationLogs)
		protected.GET("/operation-logs/stats", middleware.PermissionMiddleware("system:log:operation"), opLogAPI.GetOperationLogStats)
		protected.GET("/operation-logs/export", middleware.PermissionMiddleware("system:log:operation"), opLogAPI.ExportOperationLogs)
		protected.GET("/operation-logs/:id", middleware.PermissionMiddleware("system:log:operation"), opLogAPI.GetOperationLogDetail)
		protected.DELETE("/operation-logs/clear", middleware.PermissionMiddleware("system:log:operation:clear"), opLogAPI.ClearOperationLogs)

		protected.GET("/logs/audit", middleware.PermissionMiddleware("system:log:audit"), auditLogAPI.GetAuditLogs)
	}
}
