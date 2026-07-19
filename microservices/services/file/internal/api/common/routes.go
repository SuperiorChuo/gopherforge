package common

import (
	"github.com/gin-gonic/gin"
	sharedapi "github.com/go-admin-kit/services/file/internal/api/shared"
)

// RegisterPublicRoutes mounts unauthenticated health routes using legacy
// global fallbacks.
func RegisterPublicRoutes(r gin.IRoutes) {
	RegisterPublicRoutesWithDeps(r, sharedapi.Dependencies{})
}

// RegisterPublicRoutesWithDeps mounts unauthenticated health routes with
// injected infrastructure handles. The metrics and IP lookup routes from the
// monolith are not part of the auth service surface.
func RegisterPublicRoutesWithDeps(r gin.IRoutes, deps sharedapi.Dependencies) {
	healthAPI := newHealthAPIFromDeps(deps)
	r.GET("/health", healthAPI.Health)
	r.GET("/health/check", healthAPI.HealthCheck)
	r.GET("/health/live", healthAPI.Liveness)
	r.GET("/health/ready", healthAPI.Readiness)
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
