package api

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/api/auth"
	"github.com/go-admin-kit/server/internal/api/common"
	"github.com/go-admin-kit/server/internal/api/monitor"
	sharedapi "github.com/go-admin-kit/server/internal/api/shared"
	"github.com/go-admin-kit/server/internal/api/system"
	"github.com/go-admin-kit/server/internal/middleware"
)

// SetupRoutes mounts the clean Go Admin Kit API using legacy global fallbacks.
func SetupRoutes(router *gin.Engine) {
	SetupRoutesWithDeps(router, sharedapi.Dependencies{})
}

// SetupRoutesWithDeps mounts the API with injected infrastructure handles.
func SetupRoutesWithDeps(router *gin.Engine, deps sharedapi.Dependencies) {
	api := router.Group("/api/v1")

	common.RegisterPublicRoutes(api)

	notificationAPI := system.NewNotificationAPI()
	public := api.Group("/")
	{
		auth.RegisterPublicRoutesWithDeps(public, deps)
		public.GET("/ws/notifications", notificationAPI.Connect)
	}

	protected := api.Group("/")
	protected.Use(middleware.AuthMiddleware(), middleware.OperationLogger())
	{
		auth.RegisterProtectedRoutesWithDeps(protected, deps)
		protected.POST("/ws/notifications/ticket", notificationAPI.CreateTicket)

		userMgmtAPI := system.NewUserManagementAPI()
		protected.GET("/users", middleware.PermissionMiddleware("system:user:list"), userMgmtAPI.GetUserList)
		protected.POST("/users", middleware.PermissionMiddleware("system:user:create"), userMgmtAPI.CreateUser)
		protected.GET("/users/:id", middleware.PermissionMiddleware("system:user:detail"), userMgmtAPI.GetUser)
		protected.PUT("/users/:id", middleware.PermissionMiddleware("system:user:update"), userMgmtAPI.UpdateUser)
		protected.DELETE("/users/:id", middleware.PermissionMiddleware("system:user:delete"), userMgmtAPI.DeleteUser)
		protected.PUT("/users/:id/status", middleware.PermissionMiddleware("system:user:update"), userMgmtAPI.UpdateUserStatus)
		protected.POST("/users/:id/roles", middleware.PermissionMiddleware("system:user:update"), userMgmtAPI.AssignRoles)

		roleMgmtAPI := system.NewRoleManagementAPI()
		protected.GET("/roles", middleware.PermissionMiddleware("system:role:list"), roleMgmtAPI.GetRoleList)
		protected.GET("/roles/all", middleware.PermissionMiddleware("system:role:list"), roleMgmtAPI.GetAllRoles)
		protected.GET("/roles/:id", middleware.PermissionMiddleware("system:role:list"), roleMgmtAPI.GetRole)
		protected.POST("/roles", middleware.PermissionMiddleware("system:role:create"), roleMgmtAPI.CreateRole)
		protected.PUT("/roles/:id", middleware.PermissionMiddleware("system:role:update"), roleMgmtAPI.UpdateRole)
		protected.DELETE("/roles/:id", middleware.PermissionMiddleware("system:role:delete"), roleMgmtAPI.DeleteRole)
		protected.POST("/roles/:id/permissions", middleware.PermissionMiddleware("system:role:update"), roleMgmtAPI.AssignPermissions)

		permissionMgmtAPI := system.NewPermissionManagementAPI()
		protected.GET("/permissions", middleware.PermissionMiddleware("system:permission:list"), permissionMgmtAPI.GetPermissionList)
		protected.GET("/permissions/tree", middleware.PermissionMiddleware("system:permission:list"), permissionMgmtAPI.GetPermissionTree)
		protected.GET("/permissions/:id", middleware.PermissionMiddleware("system:permission:list"), permissionMgmtAPI.GetPermission)
		protected.POST("/permissions", middleware.PermissionMiddleware("system:permission:create"), permissionMgmtAPI.CreatePermission)
		protected.PUT("/permissions/:id", middleware.PermissionMiddleware("system:permission:update"), permissionMgmtAPI.UpdatePermission)
		protected.DELETE("/permissions/:id", middleware.PermissionMiddleware("system:permission:delete"), permissionMgmtAPI.DeletePermission)

		menuMgmtAPI := system.NewMenuManagementAPI()
		protected.GET("/menus", middleware.PermissionMiddleware("system:menu:list"), menuMgmtAPI.GetMenuList)
		protected.GET("/menus/tree", middleware.PermissionMiddleware("system:menu:list"), menuMgmtAPI.GetMenuTree)
		protected.GET("/menus/:id", middleware.PermissionMiddleware("system:menu:list"), menuMgmtAPI.GetMenu)
		protected.POST("/menus", middleware.PermissionMiddleware("system:menu:create"), menuMgmtAPI.CreateMenu)
		protected.PUT("/menus/:id", middleware.PermissionMiddleware("system:menu:update"), menuMgmtAPI.UpdateMenu)
		protected.DELETE("/menus/:id", middleware.PermissionMiddleware("system:menu:delete"), menuMgmtAPI.DeleteMenu)

		deptAPI := system.NewDepartmentAPI()
		protected.GET("/departments", middleware.PermissionMiddleware("system:department:list"), deptAPI.GetDepartmentList)
		protected.GET("/departments/tree", middleware.PermissionMiddleware("system:department:list"), deptAPI.GetDepartmentTree)
		protected.GET("/departments/all", middleware.PermissionMiddleware("system:department:list"), deptAPI.GetAllDepartments)
		protected.GET("/departments/:id", middleware.PermissionMiddleware("system:department:list"), deptAPI.GetDepartment)
		protected.POST("/departments", middleware.PermissionMiddleware("system:department:create"), deptAPI.CreateDepartment)
		protected.PUT("/departments/:id", middleware.PermissionMiddleware("system:department:update"), deptAPI.UpdateDepartment)
		protected.DELETE("/departments/:id", middleware.PermissionMiddleware("system:department:delete"), deptAPI.DeleteDepartment)

		opLogAPI := system.NewOperationLogAPI()
		auditLogAPI := system.NewAuditLogAPI()
		protected.GET("/operation-logs", middleware.PermissionMiddleware("system:log:operation"), opLogAPI.GetOperationLogs)
		protected.GET("/operation-logs/stats", middleware.PermissionMiddleware("system:log:operation"), opLogAPI.GetOperationLogStats)
		protected.GET("/operation-logs/export", middleware.PermissionMiddleware("system:log:operation"), opLogAPI.ExportOperationLogs)
		protected.GET("/operation-logs/:id", middleware.PermissionMiddleware("system:log:operation"), opLogAPI.GetOperationLogDetail)
		protected.DELETE("/operation-logs/clear", middleware.PermissionMiddleware("system:log:operation:clear"), opLogAPI.ClearOperationLogs)
		protected.GET("/logs/audit", middleware.PermissionMiddleware("system:log:audit"), auditLogAPI.GetAuditLogs)

		onlineUserAPI := system.NewOnlineUserAPI()
		protected.GET("/online-users", middleware.PermissionMiddleware("system:online-user:list"), onlineUserAPI.GetOnlineUsers)
		protected.GET("/online-users/count", middleware.PermissionMiddleware("system:online-user:list"), onlineUserAPI.GetOnlineUserCount)
		protected.DELETE("/online-users/:token_id", middleware.PermissionMiddleware("system:online-user:kick"), onlineUserAPI.ForceLogout)

		noticeAPI := system.NewNoticeAPI()
		protected.GET("/notices", middleware.PermissionMiddleware("system:notice:list"), noticeAPI.GetNoticeList)
		protected.GET("/notices/active", noticeAPI.GetActiveNotices)
		protected.GET("/notices/:id", middleware.PermissionMiddleware("system:notice:list"), noticeAPI.GetNotice)
		protected.POST("/notices", middleware.PermissionMiddleware("system:notice:create"), noticeAPI.CreateNotice)
		protected.PUT("/notices/:id", middleware.PermissionMiddleware("system:notice:update"), noticeAPI.UpdateNotice)
		protected.DELETE("/notices/:id", middleware.PermissionMiddleware("system:notice:delete"), noticeAPI.DeleteNotice)
		protected.PUT("/notices/:id/status", middleware.PermissionMiddleware("system:notice:update"), noticeAPI.UpdateNoticeStatus)

		fileAPI := system.NewFileAPI()
		protected.POST("/files/upload", middleware.PermissionMiddleware("system:file:upload"), fileAPI.Upload)
		protected.POST("/files/upload/multiple", middleware.PermissionMiddleware("system:file:upload"), fileAPI.UploadMultiple)
		protected.GET("/files", middleware.PermissionMiddleware("system:file:list"), fileAPI.GetFileList)
		protected.GET("/files/my", fileAPI.GetMyFiles)
		protected.GET("/files/stats", middleware.PermissionMiddleware("system:file:list"), fileAPI.GetFileStats)
		protected.GET("/files/hash/check", middleware.PermissionMiddleware("system:file:list"), fileAPI.CheckHash)
		protected.GET("/files/:id", middleware.PermissionMiddleware("system:file:list"), fileAPI.GetFile)
		protected.GET("/files/:id/download", middleware.PermissionMiddleware("system:file:list"), fileAPI.Download)
		protected.GET("/files/:id/preview", middleware.PermissionMiddleware("system:file:list"), fileAPI.Preview)
		protected.DELETE("/files/:id", middleware.PermissionMiddleware("system:file:delete"), fileAPI.DeleteFile)
		protected.DELETE("/files/batch", middleware.PermissionMiddleware("system:file:delete"), fileAPI.DeleteFiles)

		loginLogAPI := system.NewLoginLogAPI()
		protected.GET("/login-logs", middleware.PermissionMiddleware("system:log:login"), loginLogAPI.GetLoginLogs)
		protected.GET("/login-logs/my", loginLogAPI.GetMyLoginLogs)
		protected.GET("/login-logs/stats", middleware.PermissionMiddleware("system:log:login"), loginLogAPI.GetLoginStats)
		protected.GET("/login-logs/trend", middleware.PermissionMiddleware("system:log:login"), loginLogAPI.GetLoginTrend)
		protected.GET("/login-logs/last", loginLogAPI.GetLastLogin)
		protected.GET("/login-logs/user/:user_id", middleware.PermissionMiddleware("system:log:login"), loginLogAPI.GetUserLoginHistory)
		protected.DELETE("/login-logs/clear", middleware.PermissionMiddleware("system:log:login"), loginLogAPI.ClearLoginLogs)

		dictAPI := system.NewDictAPI()
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

		settingAPI := system.NewSettingAPI()
		protected.GET("/system-settings", middleware.PermissionMiddleware("system:setting:list"), settingAPI.GetSettings)
		protected.POST("/system-settings/batch", middleware.PermissionMiddleware("system:setting:update"), settingAPI.BatchUpsertSettings)
		protected.GET("/system-settings/:key", middleware.PermissionMiddleware("system:setting:list"), settingAPI.GetSetting)
		protected.PUT("/system-settings/:key", middleware.PermissionMiddleware("system:setting:update"), settingAPI.UpsertSetting)
		protected.DELETE("/system-settings/:key", middleware.PermissionMiddleware("system:setting:delete"), settingAPI.DeleteSetting)

		monitor.RegisterProtectedRoutesWithDeps(protected, deps)
	}

	system.ServeStaticFiles(router)
}
