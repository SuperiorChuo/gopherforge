// Package api wires the identity service HTTP surface. The /api/v1 layout
// matches the monolith exactly for every extracted route so the gateway can
// switch traffic over without any client change.
package api

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/identity/internal/api/common"
	sharedapi "github.com/go-admin-kit/services/identity/internal/api/shared"
	"github.com/go-admin-kit/services/identity/internal/api/system"
	"github.com/go-admin-kit/services/identity/internal/middleware"
	systemsvc "github.com/go-admin-kit/services/identity/internal/service/system"
)

// SetupRoutes mounts the identity service API using legacy global fallbacks.
func SetupRoutes(router *gin.Engine) {
	SetupRoutesWithDeps(router, sharedapi.Dependencies{})
}

// SetupRoutesWithDeps mounts the API with injected infrastructure handles.
func SetupRoutesWithDeps(router *gin.Engine, deps sharedapi.Dependencies) {
	api := router.Group("/api/v1")

	common.RegisterPublicRoutesWithDeps(api, deps)

	userMgmtAPI := system.NewUserManagementAPI()
	roleMgmtAPI := system.NewRoleManagementAPI()
	permissionMgmtAPI := system.NewPermissionManagementAPI()
	departmentAPI := system.NewDepartmentAPI()
	postAPI := system.NewPostAPI()
	var tenantAPI *system.TenantAPI
	var tenantPackageAPI *system.TenantPackageAPI
	if deps.DB != nil {
		userMgmtAPI = system.NewUserManagementAPIWithService(systemsvc.NewUserServiceWithDB(deps.DB))
		roleMgmtAPI = system.NewRoleManagementAPIWithService(systemsvc.NewRoleServiceWithDB(deps.DB))
		permissionMgmtAPI = system.NewPermissionManagementAPIWithService(systemsvc.NewPermissionServiceWithDB(deps.DB))
		departmentAPI = system.NewDepartmentAPIWithService(systemsvc.NewDepartmentServiceWithDB(deps.DB))
		postAPI = system.NewPostAPIWithService(systemsvc.NewPostServiceWithDB(deps.DB))
		tenantAPI = system.NewTenantAPIWithService(systemsvc.NewTenantServiceWithDB(deps.DB))
		tenantPackageAPI = system.NewTenantPackageAPIWithService(systemsvc.NewTenantPackageServiceWithDB(deps.DB))
	}

	protected := api.Group("/")
	protected.Use(middleware.AuthMiddleware(), middleware.OperationLogger())
	{
		if tenantAPI != nil {
			protected.GET("/tenants", middleware.PermissionMiddleware("system:tenant:list"), tenantAPI.GetTenantList)
			protected.POST("/tenants", middleware.PermissionMiddleware("system:tenant:create"), tenantAPI.CreateTenant)
			protected.GET("/tenants/:id", middleware.PermissionMiddleware("system:tenant:detail"), tenantAPI.GetTenant)
			protected.PUT("/tenants/:id", middleware.PermissionMiddleware("system:tenant:update"), tenantAPI.UpdateTenant)
		}

		if tenantPackageAPI != nil {
			// 租户套餐（权限包）：/all 供租户管理页下拉，放宽到 system:tenant:list 亦可访问。
			protected.GET("/tenant-packages", middleware.PermissionMiddleware("system:tenant-package:list"), tenantPackageAPI.GetTenantPackageList)
			protected.GET("/tenant-packages/all", middleware.PermissionMiddleware("system:tenant-package:list", "system:tenant:list"), tenantPackageAPI.GetAllTenantPackages)
			protected.GET("/tenant-packages/:id", middleware.PermissionMiddleware("system:tenant-package:list"), tenantPackageAPI.GetTenantPackage)
			protected.POST("/tenant-packages", middleware.PermissionMiddleware("system:tenant-package:create"), tenantPackageAPI.CreateTenantPackage)
			protected.PUT("/tenant-packages/:id", middleware.PermissionMiddleware("system:tenant-package:update"), tenantPackageAPI.UpdateTenantPackage)
			protected.DELETE("/tenant-packages/:id", middleware.PermissionMiddleware("system:tenant-package:delete"), tenantPackageAPI.DeleteTenantPackage)
		}

		protected.GET("/users", middleware.PermissionMiddleware("system:user:list"), userMgmtAPI.GetUserList)
		protected.POST("/users", middleware.PermissionMiddleware("system:user:create"), userMgmtAPI.CreateUser)
		protected.GET("/users/:id", middleware.PermissionMiddleware("system:user:detail"), userMgmtAPI.GetUser)
		protected.PUT("/users/:id", middleware.PermissionMiddleware("system:user:update"), userMgmtAPI.UpdateUser)
		protected.DELETE("/users/:id", middleware.PermissionMiddleware("system:user:delete"), userMgmtAPI.DeleteUser)
		protected.PUT("/users/:id/status", middleware.PermissionMiddleware("system:user:update"), userMgmtAPI.UpdateUserStatus)
		protected.POST("/users/:id/roles", middleware.PermissionMiddleware("system:user:update"), userMgmtAPI.AssignRoles)

		protected.GET("/roles", middleware.PermissionMiddleware("system:role:list"), roleMgmtAPI.GetRoleList)
		protected.GET("/roles/all", middleware.PermissionMiddleware("system:role:list"), roleMgmtAPI.GetAllRoles)
		protected.GET("/roles/:id", middleware.PermissionMiddleware("system:role:list"), roleMgmtAPI.GetRole)
		protected.POST("/roles", middleware.PermissionMiddleware("system:role:create"), roleMgmtAPI.CreateRole)
		protected.PUT("/roles/:id", middleware.PermissionMiddleware("system:role:update"), roleMgmtAPI.UpdateRole)
		protected.DELETE("/roles/:id", middleware.PermissionMiddleware("system:role:delete"), roleMgmtAPI.DeleteRole)
		protected.POST("/roles/:id/permissions", middleware.PermissionMiddleware("system:role:update"), roleMgmtAPI.AssignPermissions)

		protected.GET("/permissions", middleware.PermissionMiddleware("system:permission:list"), permissionMgmtAPI.GetPermissionList)
		protected.GET("/permissions/tree", middleware.PermissionMiddleware("system:permission:list"), permissionMgmtAPI.GetPermissionTree)
		protected.GET("/permissions/:id", middleware.PermissionMiddleware("system:permission:list"), permissionMgmtAPI.GetPermission)
		protected.POST("/permissions", middleware.PermissionMiddleware("system:permission:create"), permissionMgmtAPI.CreatePermission)
		protected.PUT("/permissions/:id", middleware.PermissionMiddleware("system:permission:update"), permissionMgmtAPI.UpdatePermission)
		protected.DELETE("/permissions/:id", middleware.PermissionMiddleware("system:permission:delete"), permissionMgmtAPI.DeletePermission)

		protected.GET("/departments", middleware.PermissionMiddleware("system:department:list"), departmentAPI.GetDepartmentList)
		protected.GET("/departments/tree", middleware.PermissionMiddleware("system:department:list"), departmentAPI.GetDepartmentTree)
		protected.GET("/departments/all", middleware.PermissionMiddleware("system:department:list"), departmentAPI.GetAllDepartments)
		protected.GET("/departments/:id", middleware.PermissionMiddleware("system:department:list"), departmentAPI.GetDepartment)
		protected.POST("/departments", middleware.PermissionMiddleware("system:department:create"), departmentAPI.CreateDepartment)
		protected.PUT("/departments/:id", middleware.PermissionMiddleware("system:department:update"), departmentAPI.UpdateDepartment)
		protected.DELETE("/departments/:id", middleware.PermissionMiddleware("system:department:delete"), departmentAPI.DeleteDepartment)

		protected.GET("/posts", middleware.PermissionMiddleware("system:post:list"), postAPI.GetPostList)
		protected.GET("/posts/all", middleware.PermissionMiddleware("system:post:list"), postAPI.GetAllPosts)
		protected.GET("/posts/:id", middleware.PermissionMiddleware("system:post:list"), postAPI.GetPost)
		protected.POST("/posts", middleware.PermissionMiddleware("system:post:create"), postAPI.CreatePost)
		protected.PUT("/posts/:id", middleware.PermissionMiddleware("system:post:update"), postAPI.UpdatePost)
		protected.DELETE("/posts/:id", middleware.PermissionMiddleware("system:post:delete"), postAPI.DeletePost)
	}
}
