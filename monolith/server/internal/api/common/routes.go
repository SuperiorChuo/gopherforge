package common

import (
	"github.com/gin-gonic/gin"
	sharedapi "github.com/go-admin-kit/server/internal/api/shared"
)

// RegisterPublicRoutes mounts unauthenticated health, metrics, and IP lookup
// routes using legacy global fallbacks.
func RegisterPublicRoutes(r gin.IRoutes) {
	RegisterPublicRoutesWithDeps(r, sharedapi.Dependencies{})
}

// RegisterPublicRoutesWithDeps mounts unauthenticated health, metrics, and IP
// lookup routes with injected infrastructure handles.
func RegisterPublicRoutesWithDeps(r gin.IRoutes, deps sharedapi.Dependencies) {
	healthAPI := newHealthAPIFromDeps(deps)
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

// newHealthAPIFromDeps assembles a HealthAPI from injected handles, falling
// back to the legacy zero-value wiring when no handles are provided. The nil
// guards keep typed-nil pointers out of the client interfaces.
func newHealthAPIFromDeps(deps sharedapi.Dependencies) *HealthAPI {
	var databaseClient DatabaseClient
	if deps.DB != nil {
		databaseClient = deps.DB
	}
	var redisClient RedisPingClient
	if deps.Redis != nil {
		redisClient = deps.Redis
	}
	if databaseClient == nil && redisClient == nil {
		return NewHealthAPI()
	}
	return NewHealthAPIWithClients(databaseClient, redisClient)
}
