package api

import (
	"github.com/gin-gonic/gin"
	sharedapi "github.com/go-admin-kit/services/system/internal/api/shared"
	"github.com/go-admin-kit/services/system/internal/api/system"
	"github.com/go-admin-kit/services/system/internal/middleware"
)

// registerSmsRoutes 挂载短信管理路由（渠道/模板/发送日志/发送入口）。
// 独立成文件是为了并行开发时把 routes.go 的冲突面压到一行调用。
func registerSmsRoutes(protected *gin.RouterGroup, deps sharedapi.Dependencies) {
	smsAPI := system.NewSmsAPI()
	if deps.DB != nil {
		smsAPI = system.NewSmsAPIWithDB(deps.DB)
	}

	// 渠道
	protected.GET("/sms/channels", middleware.PermissionMiddleware("system:sms-channel:list"), smsAPI.GetChannelList)
	// 启用渠道下拉：模板编辑表单用，跟随模板 list 权限
	protected.GET("/sms/channels/enabled", middleware.PermissionMiddleware("system:sms-template:list"), smsAPI.GetEnabledChannels)
	protected.GET("/sms/channels/:id", middleware.PermissionMiddleware("system:sms-channel:list"), smsAPI.GetChannel)
	protected.POST("/sms/channels", middleware.PermissionMiddleware("system:sms-channel:create"), smsAPI.CreateChannel)
	protected.PUT("/sms/channels/:id", middleware.PermissionMiddleware("system:sms-channel:update"), smsAPI.UpdateChannel)
	protected.PUT("/sms/channels/:id/status", middleware.PermissionMiddleware("system:sms-channel:update"), smsAPI.UpdateChannelStatus)
	protected.DELETE("/sms/channels/:id", middleware.PermissionMiddleware("system:sms-channel:delete"), smsAPI.DeleteChannel)

	// 模板
	protected.GET("/sms/templates", middleware.PermissionMiddleware("system:sms-template:list"), smsAPI.GetTemplateList)
	protected.GET("/sms/templates/:id", middleware.PermissionMiddleware("system:sms-template:list"), smsAPI.GetTemplate)
	protected.POST("/sms/templates", middleware.PermissionMiddleware("system:sms-template:create"), smsAPI.CreateTemplate)
	protected.PUT("/sms/templates/:id", middleware.PermissionMiddleware("system:sms-template:update"), smsAPI.UpdateTemplate)
	protected.PUT("/sms/templates/:id/status", middleware.PermissionMiddleware("system:sms-template:update"), smsAPI.UpdateTemplateStatus)
	protected.DELETE("/sms/templates/:id", middleware.PermissionMiddleware("system:sms-template:delete"), smsAPI.DeleteTemplate)

	// 发送日志
	protected.GET("/sms/logs", middleware.PermissionMiddleware("system:sms-log:list"), smsAPI.GetLogList)

	// 发送入口（业务发送与模板测试发送共用）
	protected.POST("/sms/send", middleware.PermissionMiddleware("system:sms:send"), smsAPI.SendSms)
}
