package api

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/monitor/internal/api/common"
	"github.com/go-admin-kit/services/monitor/internal/api/monitor"
	sharedapi "github.com/go-admin-kit/services/monitor/internal/api/shared"
	"github.com/go-admin-kit/services/monitor/internal/middleware"
)

// SetupRoutes mounts the slimmed-down monolith API using legacy global
// fallbacks. Auth, identity, system, audit, and file domains have moved to
// dedicated microservices; the monolith only serves monitoring plus
// health/metrics fallbacks.
func SetupRoutes(router *gin.Engine) {
	SetupRoutesWithDeps(router, sharedapi.Dependencies{})
}

// SetupRoutesWithDeps mounts the API with injected infrastructure handles.
func SetupRoutesWithDeps(router *gin.Engine, deps sharedapi.Dependencies) {
	api := router.Group("/api/v1")

	common.RegisterPublicRoutesWithDeps(api, deps)

	protected := api.Group("/")
	protected.Use(middleware.AuthMiddleware(), middleware.OperationLogger())
	{
		monitor.RegisterProtectedRoutesWithDeps(protected, deps)
	}
}
