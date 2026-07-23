package monitor

import (
	"github.com/gin-gonic/gin"
	sharedapi "github.com/go-admin-kit/server/internal/api/shared"
	"github.com/go-admin-kit/server/internal/middleware"
	monitorsvc "github.com/go-admin-kit/server/internal/service/monitor"
)

// RegisterProtectedRoutes mounts authenticated system monitoring routes.
// Deprecated: use RegisterProtectedRoutesWithDeps with an injected DB.
func RegisterProtectedRoutes(r *gin.RouterGroup) {
	RegisterProtectedRoutesWithDeps(r, sharedapi.Dependencies{})
}

// RegisterProtectedRoutesWithDeps mounts authenticated system monitoring
// routes with injected infrastructure handles.
func RegisterProtectedRoutesWithDeps(r *gin.RouterGroup, deps sharedapi.Dependencies) {
	serverAPI := NewServerAPI()

	var mysqlAPI *MySQLAPI
	if deps.DB != nil {
		mysqlAPI = NewMySQLAPIWithService(monitorsvc.NewMySQLServiceWithDB(deps.DB))
	}

	redisAPI := NewRedisAPI()
	if deps.Redis != nil {
		redisAPI = NewRedisAPIWithService(monitorsvc.NewRedisServiceWithClient(deps.Redis))
	}

	var jobAPI *JobAPI
	if deps.DB != nil {
		jobAPI = &JobAPI{service: monitorsvc.InitJobService(deps.DB)}
	} else if svc := monitorsvc.GetJobService(); svc != nil {
		jobAPI = &JobAPI{service: svc}
	}

	monitorGroup := r.Group("/monitor")
	monitorGroup.GET("/server", middleware.PermissionMiddleware("system:monitor:server"), serverAPI.GetServerInfo)
	monitorGroup.GET("/services", middleware.PermissionMiddleware("system:monitor:server"), serverAPI.GetServicesHealth)
	if mysqlAPI != nil {
		monitorGroup.GET("/mysql", middleware.PermissionMiddleware("system:monitor:mysql"), mysqlAPI.GetMySQLInfo)
	}
	monitorGroup.GET("/redis", middleware.PermissionMiddleware("system:monitor:redis"), redisAPI.GetRedisInfo)

	if jobAPI != nil {
		monitorGroup.GET("/jobs", middleware.PermissionMiddleware("system:job:list"), jobAPI.GetJobList)
		monitorGroup.GET("/jobs/health", middleware.PermissionMiddleware("system:job:list"), jobAPI.GetJobHealth)
		monitorGroup.GET("/jobs/heartbeats", middleware.PermissionMiddleware("system:job:list"), jobAPI.GetJobHeartbeats)
		monitorGroup.POST("/jobs", middleware.PermissionMiddleware("system:job:create"), jobAPI.CreateJob)
		monitorGroup.PUT("/jobs/:id", middleware.PermissionMiddleware("system:job:update"), jobAPI.UpdateJob)
		monitorGroup.DELETE("/jobs/:id", middleware.PermissionMiddleware("system:job:delete"), jobAPI.DeleteJob)
		monitorGroup.POST("/jobs/:id/start", middleware.PermissionMiddleware("system:job:run"), jobAPI.StartJob)
		monitorGroup.POST("/jobs/:id/stop", middleware.PermissionMiddleware("system:job:run"), jobAPI.StopJob)
		monitorGroup.POST("/jobs/:id/run", middleware.PermissionMiddleware("system:job:run"), jobAPI.RunJob)
		monitorGroup.POST("/job-logs/cleanup", middleware.PermissionMiddleware("system:job:run"), jobAPI.CleanupJobLogs)
	}
}
