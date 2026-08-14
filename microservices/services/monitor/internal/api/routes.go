package api

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/monitor/internal/api/common"
	"github.com/go-admin-kit/services/monitor/internal/api/monitor"
	"github.com/go-admin-kit/services/monitor/internal/config"
	"github.com/go-admin-kit/services/monitor/internal/middleware"
	"github.com/go-admin-kit/services/shared/pkg/jwt"
	sharedapi "github.com/go-admin-kit/services/shared/pkg/sharedapi"
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
	configureRuntimeJWT(deps)
	api := router.Group("/api/v1")

	common.RegisterPublicRoutesWithDeps(api, deps)

	protected := api.Group("/")
	protected.Use(middleware.AuthMiddleware(), middleware.OperationLogger())
	{
		monitor.RegisterProtectedRoutesWithDeps(protected, deps)
	}
}

func configureRuntimeJWT(deps sharedapi.Dependencies) func() {
	cfg := config.Cfg.JWT
	restore := jwt.SetConfig(jwt.JWTConfig{
		Secret: cfg.Secret, Issuer: cfg.Issuer,
		AccessTokenExpire: cfg.AccessTokenExpire, RefreshTokenExpire: cfg.RefreshTokenExpire,
		RefreshTokenRotation: cfg.RefreshTokenRotation,
	})
	if deps.Redis != nil {
		jwt.SetRedis(deps.Redis)
	}
	return restore
}
