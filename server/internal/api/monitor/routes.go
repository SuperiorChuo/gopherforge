package monitor

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/middleware"
)

// RegisterProtectedRoutes mounts authenticated system monitoring routes.
func RegisterProtectedRoutes(r *gin.RouterGroup) {
	serverAPI := NewServerAPI()
	mysqlAPI := NewMySQLAPI()
	redisAPI := NewRedisAPI()
	jobAPI := NewJobAPI()

	monitorGroup := r.Group("/monitor")
	monitorGroup.GET("/server", middleware.PermissionMiddleware("system:monitor:server"), serverAPI.GetServerInfo)
	monitorGroup.GET("/mysql", middleware.PermissionMiddleware("system:monitor:mysql"), mysqlAPI.GetMySQLInfo)
	monitorGroup.GET("/redis", middleware.PermissionMiddleware("system:monitor:redis"), redisAPI.GetRedisInfo)

	monitorGroup.GET("/jobs", middleware.PermissionMiddleware("system:job:list"), jobAPI.GetJobList)
	monitorGroup.GET("/jobs/health", middleware.PermissionMiddleware("system:job:list"), jobAPI.GetJobHealth)
	monitorGroup.POST("/jobs", middleware.PermissionMiddleware("system:job:create"), jobAPI.CreateJob)
	monitorGroup.PUT("/jobs/:id", middleware.PermissionMiddleware("system:job:update"), jobAPI.UpdateJob)
	monitorGroup.DELETE("/jobs/:id", middleware.PermissionMiddleware("system:job:delete"), jobAPI.DeleteJob)
	monitorGroup.POST("/jobs/:id/start", middleware.PermissionMiddleware("system:job:run"), jobAPI.StartJob)
	monitorGroup.POST("/jobs/:id/stop", middleware.PermissionMiddleware("system:job:run"), jobAPI.StopJob)
	monitorGroup.POST("/jobs/:id/run", middleware.PermissionMiddleware("system:job:run"), jobAPI.RunJob)
	monitorGroup.POST("/job-logs/cleanup", middleware.PermissionMiddleware("system:job:run"), jobAPI.CleanupJobLogs)
}
