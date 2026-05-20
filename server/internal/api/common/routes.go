package common

import "github.com/gin-gonic/gin"

// RegisterPublicRoutes mounts unauthenticated health, metrics, and IP lookup routes.
func RegisterPublicRoutes(r gin.IRoutes) {
	healthAPI := NewHealthAPI()
	r.GET("/health", healthAPI.Health)
	r.GET("/health/check", healthAPI.HealthCheck)
	r.GET("/health/live", healthAPI.Liveness)
	r.GET("/health/ready", healthAPI.Readiness)
	r.GET("/metrics/json", healthAPI.MetricsSnapshot)
	r.GET("/metrics", healthAPI.PrometheusMetrics)

	ipInfoAPI := NewIPInfoAPI()
	r.GET("/ip/info", ipInfoAPI.GetIPInfo)
	r.GET("/ip/location", ipInfoAPI.GetIPLocation)
	r.GET("/ip/me", ipInfoAPI.GetMyIPInfo)
}
