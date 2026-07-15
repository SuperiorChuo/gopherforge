// Package api wires the system service HTTP surface. The /api/v1 layout
// matches the monolith exactly for every extracted route so the gateway can
// switch traffic over without any client change.
package api

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/system/internal/api/common"
	sharedapi "github.com/go-admin-kit/services/system/internal/api/shared"
	"github.com/go-admin-kit/services/system/internal/api/system"
	"github.com/go-admin-kit/services/system/internal/middleware"
	authsvc "github.com/go-admin-kit/services/system/internal/service/auth"
	systemsvc "github.com/go-admin-kit/services/system/internal/service/system"
)

// SetupRoutes mounts the system service API using legacy global fallbacks.
func SetupRoutes(router *gin.Engine) {
	SetupRoutesWithDeps(router, sharedapi.Dependencies{})
}

// SetupRoutesWithDeps mounts the API with injected infrastructure handles.
func SetupRoutesWithDeps(router *gin.Engine, deps sharedapi.Dependencies) {
	api := router.Group("/api/v1")

	common.RegisterPublicRoutesWithDeps(api, deps)

	menuMgmtAPI := system.NewMenuManagementAPI()
	menuUserAPI := system.NewMenuAPI()
	dictAPI := system.NewDictAPI()
	noticeAPI := system.NewNoticeAPI()
	settingAPI := system.NewSettingAPI()
	onlineUserAPI := system.NewOnlineUserAPI()
	notificationAPI := system.NewNotificationAPI()
	if deps.DB != nil {
		menuMgmtAPI = system.NewMenuManagementAPIWithService(systemsvc.NewMenuServiceWithDB(deps.DB))
		menuUserAPI = system.NewMenuAPIWithService(systemsvc.NewMenuUserServiceWithDB(deps.DB))
		dictAPI = system.NewDictAPIWithService(systemsvc.NewDictServiceWithDB(deps.DB))
		noticeAPI = system.NewNoticeAPIWithService(systemsvc.NewNoticeServiceWithDB(deps.DB))
		settingAPI = system.NewSettingAPIWithService(systemsvc.NewSettingServiceWithDB(deps.DB))
		notificationAPI = system.NewNotificationAPIWithService(systemsvc.NewNoticeServiceWithDB(deps.DB))

		onlineUserService := &systemsvc.OnlineUserService{}
		if deps.Redis != nil {
			onlineUserService = systemsvc.NewOnlineUserServiceWithClient(deps.Redis)
		}
		onlineUserAPI = system.NewOnlineUserAPIWithServices(onlineUserService, authsvc.NewUserServiceWithDB(deps.DB))
	}

	public := api.Group("/")
	{
		// WebSocket upgrade authenticates via one-shot ticket, not header.
		public.GET("/ws/notifications", notificationAPI.Connect)
	}

	protected := api.Group("/")
	protected.Use(middleware.AuthMiddleware(), middleware.OperationLogger())
	{
		protected.POST("/ws/notifications/ticket", notificationAPI.CreateTicket)

		protected.GET("/user/menus", menuUserAPI.GetUserMenus)

		protected.GET("/menus", middleware.PermissionMiddleware("system:menu:list"), menuMgmtAPI.GetMenuList)
		protected.GET("/menus/tree", middleware.PermissionMiddleware("system:menu:list"), menuMgmtAPI.GetMenuTree)
		protected.GET("/menus/:id", middleware.PermissionMiddleware("system:menu:list"), menuMgmtAPI.GetMenu)
		protected.POST("/menus", middleware.PermissionMiddleware("system:menu:create"), menuMgmtAPI.CreateMenu)
		protected.PUT("/menus/:id", middleware.PermissionMiddleware("system:menu:update"), menuMgmtAPI.UpdateMenu)
		protected.DELETE("/menus/:id", middleware.PermissionMiddleware("system:menu:delete"), menuMgmtAPI.DeleteMenu)

		protected.GET("/dict-types", middleware.PermissionMiddleware("system:dict:list"), dictAPI.GetTypeList)
		protected.GET("/dict-types/all", middleware.PermissionMiddleware("system:dict:list"), dictAPI.GetAllTypes)
		protected.GET("/dict-types/:id", middleware.PermissionMiddleware("system:dict:list"), dictAPI.GetType)
		protected.GET("/dict-types/:id/items", middleware.PermissionMiddleware("system:dict:list"), dictAPI.GetItemsByTypeID)
		protected.POST("/dict-types", middleware.PermissionMiddleware("system:dict:create"), dictAPI.CreateType)
		protected.PUT("/dict-types/:id", middleware.PermissionMiddleware("system:dict:update"), dictAPI.UpdateType)
		protected.DELETE("/dict-types/:id", middleware.PermissionMiddleware("system:dict:delete"), dictAPI.DeleteType)

		protected.GET("/dict-items", middleware.PermissionMiddleware("system:dict:list"), dictAPI.GetItemList)
		protected.GET("/dict-items/:id", middleware.PermissionMiddleware("system:dict:list"), dictAPI.GetItem)
		protected.POST("/dict-items", middleware.PermissionMiddleware("system:dict:create"), dictAPI.CreateItem)
		protected.PUT("/dict-items/:id", middleware.PermissionMiddleware("system:dict:update"), dictAPI.UpdateItem)
		protected.DELETE("/dict-items/:id", middleware.PermissionMiddleware("system:dict:delete"), dictAPI.DeleteItem)

		protected.GET("/dicts/:code", dictAPI.GetDictData)
		protected.GET("/dicts", dictAPI.GetMultipleDictData)
		protected.GET("/dicts/all", dictAPI.GetAllDictData)

		protected.GET("/notices", middleware.PermissionMiddleware("system:notice:list"), noticeAPI.GetNoticeList)
		protected.GET("/notices/active", noticeAPI.GetActiveNotices)
		protected.GET("/notices/:id", middleware.PermissionMiddleware("system:notice:list"), noticeAPI.GetNotice)
		protected.POST("/notices", middleware.PermissionMiddleware("system:notice:create"), noticeAPI.CreateNotice)
		protected.PUT("/notices/:id", middleware.PermissionMiddleware("system:notice:update"), noticeAPI.UpdateNotice)
		protected.DELETE("/notices/:id", middleware.PermissionMiddleware("system:notice:delete"), noticeAPI.DeleteNotice)
		protected.PUT("/notices/:id/status", middleware.PermissionMiddleware("system:notice:update"), noticeAPI.UpdateNoticeStatus)

		protected.GET("/system-settings", middleware.PermissionMiddleware("system:setting:list"), settingAPI.GetSettings)
		protected.POST("/system-settings/batch", middleware.PermissionMiddleware("system:setting:update"), settingAPI.BatchUpsertSettings)
		protected.GET("/system-settings/:key", middleware.PermissionMiddleware("system:setting:list"), settingAPI.GetSetting)
		protected.PUT("/system-settings/:key", middleware.PermissionMiddleware("system:setting:update"), settingAPI.UpsertSetting)
		protected.DELETE("/system-settings/:key", middleware.PermissionMiddleware("system:setting:delete"), settingAPI.DeleteSetting)

		protected.GET("/online-users", middleware.PermissionMiddleware("system:online-user:list"), onlineUserAPI.GetOnlineUsers)
		protected.GET("/online-users/count", middleware.PermissionMiddleware("system:online-user:list"), onlineUserAPI.GetOnlineUserCount)
		protected.DELETE("/online-users/:token_id", middleware.PermissionMiddleware("system:online-user:kick"), onlineUserAPI.ForceLogout)
	}
}
