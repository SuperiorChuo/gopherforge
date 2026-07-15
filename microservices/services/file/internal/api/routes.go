// Package api wires the file service HTTP surface. The /api/v1 layout
// matches the monolith exactly for every extracted route so the gateway can
// switch traffic over without any client change.
package api

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/file/internal/api/common"
	sharedapi "github.com/go-admin-kit/services/file/internal/api/shared"
	"github.com/go-admin-kit/services/file/internal/api/system"
	"github.com/go-admin-kit/services/file/internal/middleware"
	systemsvc "github.com/go-admin-kit/services/file/internal/service/system"
)

// SetupRoutes mounts the file service API using legacy global fallbacks.
func SetupRoutes(router *gin.Engine) {
	SetupRoutesWithDeps(router, sharedapi.Dependencies{})
}

// SetupRoutesWithDeps mounts the API with injected infrastructure handles.
func SetupRoutesWithDeps(router *gin.Engine, deps sharedapi.Dependencies) {
	api := router.Group("/api/v1")

	common.RegisterPublicRoutesWithDeps(api, deps)

	fileAPI := system.NewFileAPI()
	if deps.DB != nil {
		fileAPI = system.NewFileAPIWithService(systemsvc.NewFileServiceWithDB(deps.DB))
	}

	protected := api.Group("/")
	protected.Use(middleware.AuthMiddleware(), middleware.OperationLogger())
	{
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
	}

	system.ServeStaticFiles(router)
}
