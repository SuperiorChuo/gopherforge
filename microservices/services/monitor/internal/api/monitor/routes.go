package monitor

import (
	"net/http"

	"github.com/gin-gonic/gin"
	sharedapi "github.com/go-admin-kit/services/monitor/internal/api/shared"
	monitordao "github.com/go-admin-kit/services/monitor/internal/dao/monitor"
	"github.com/go-admin-kit/services/monitor/internal/middleware"
	monitorsvc "github.com/go-admin-kit/services/monitor/internal/service/monitor"
	"github.com/go-admin-kit/services/shared/pkg/response"
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

	mysqlHandler := unavailableMonitorHandler
	if deps.DB != nil {
		mysqlHandler = NewMySQLAPIWithService(monitorsvc.NewMySQLServiceWithDB(deps.DB)).GetMySQLInfo
	}

	trendsHandler := unavailableMonitorHandler
	if deps.DB != nil {
		trendsHandler = NewTrendsAPIWithService(monitorsvc.NewMetricTrendService(monitordao.NewMetricSampleDAO(deps.DB))).GetTrends
	}

	redisAPI := NewRedisAPI()
	if deps.Redis != nil {
		redisAPI = NewRedisAPIWithService(monitorsvc.NewRedisServiceWithClient(deps.Redis))
	}
	alertAPI := NewAlertAPIWithService(monitorsvc.NewAlertService(deps.DB, deps.Redis))

	var jobAPI *JobAPI
	if deps.DB != nil {
		jobAPI = &JobAPI{service: monitorsvc.InitJobService(deps.DB)}
	} else if svc := monitorsvc.GetJobService(); svc != nil {
		jobAPI = &JobAPI{service: svc}
	}

	monitorGroup := r.Group("/monitor")
	monitorGroup.GET("/server", middleware.PermissionMiddleware("system:monitor:server"), serverAPI.GetServerInfo)
	monitorGroup.GET("/services", middleware.PermissionMiddleware("system:monitor:server"), serverAPI.GetServicesHealth)
	monitorGroup.GET("/mysql", middleware.PermissionMiddleware("system:monitor:mysql"), mysqlHandler)
	monitorGroup.GET("/redis", middleware.PermissionMiddleware("system:monitor:redis"), redisAPI.GetRedisInfo)
	monitorGroup.GET("/metrics/trends", middleware.PermissionMiddleware("system:monitor:server"), trendsHandler)
	monitorGroup.GET("/alert-metrics", middleware.PermissionMiddleware("system:alert:list"), alertAPI.GetMetrics)
	monitorGroup.GET("/alert-summary", middleware.PermissionMiddleware("system:alert:list"), alertAPI.GetSummary)
	monitorGroup.GET("/alert-rules", middleware.PermissionMiddleware("system:alert:list"), alertAPI.GetRules)
	monitorGroup.POST("/alert-rules", middleware.PermissionMiddleware("system:alert:create"), alertAPI.CreateRule)
	monitorGroup.PUT("/alert-rules/:id", middleware.PermissionMiddleware("system:alert:update"), alertAPI.UpdateRule)
	monitorGroup.DELETE("/alert-rules/:id", middleware.PermissionMiddleware("system:alert:delete"), alertAPI.DeleteRule)
	monitorGroup.POST("/alert-rules/:id/evaluate", middleware.PermissionMiddleware("system:alert:evaluate"), alertAPI.EvaluateRule)
	monitorGroup.GET("/alert-events", middleware.PermissionMiddleware("system:alert:list"), alertAPI.GetEvents)
	registerJobRoutes(monitorGroup, jobAPI)
}

func unavailableMonitorHandler(c *gin.Context) {
	response.Error(c, http.StatusServiceUnavailable, "monitor dependency is unavailable")
}

func registerJobRoutes(group *gin.RouterGroup, jobAPI *JobAPI) {
	getJobs := unavailableMonitorHandler
	getJobHealth := unavailableMonitorHandler
	getJobHeartbeats := unavailableMonitorHandler
	createJob := unavailableMonitorHandler
	updateJob := unavailableMonitorHandler
	deleteJob := unavailableMonitorHandler
	startJob := unavailableMonitorHandler
	stopJob := unavailableMonitorHandler
	runJob := unavailableMonitorHandler
	getJobTargets := unavailableMonitorHandler
	getJobLogs := unavailableMonitorHandler
	cleanupJobLogs := unavailableMonitorHandler
	if jobAPI != nil {
		getJobs = jobAPI.GetJobList
		getJobHealth = jobAPI.GetJobHealth
		getJobHeartbeats = jobAPI.GetJobHeartbeats
		createJob = jobAPI.CreateJob
		updateJob = jobAPI.UpdateJob
		deleteJob = jobAPI.DeleteJob
		startJob = jobAPI.StartJob
		stopJob = jobAPI.StopJob
		runJob = jobAPI.RunJob
		getJobTargets = jobAPI.GetJobTargets
		getJobLogs = jobAPI.GetJobLogList
		cleanupJobLogs = jobAPI.CleanupJobLogs
	}

	group.GET("/jobs", middleware.PermissionMiddleware("system:job:list"), getJobs)
	group.GET("/jobs/health", middleware.PermissionMiddleware("system:job:list"), getJobHealth)
	group.GET("/jobs/heartbeats", middleware.PermissionMiddleware("system:job:list"), getJobHeartbeats)
	group.POST("/jobs", middleware.PermissionMiddleware("system:job:create"), createJob)
	group.PUT("/jobs/:id", middleware.PermissionMiddleware("system:job:update"), updateJob)
	group.DELETE("/jobs/:id", middleware.PermissionMiddleware("system:job:delete"), deleteJob)
	group.POST("/jobs/:id/start", middleware.PermissionMiddleware("system:job:run"), startJob)
	group.POST("/jobs/:id/stop", middleware.PermissionMiddleware("system:job:run"), stopJob)
	group.POST("/jobs/:id/run", middleware.PermissionMiddleware("system:job:run"), runJob)
	group.GET("/jobs/targets", middleware.PermissionMiddleware("system:job:list"), getJobTargets)
	group.GET("/job-logs", middleware.PermissionMiddleware("system:job:list"), getJobLogs)
	group.POST("/job-logs/cleanup", middleware.PermissionMiddleware("system:job:run"), cleanupJobLogs)
}
