package common

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/health"
	sharedapi "github.com/go-admin-kit/services/shared/pkg/sharedapi"
)

// RegisterPublicRoutes 使用旧的零值装配挂载无需认证的健康路由。
func RegisterPublicRoutes(r gin.IRoutes) {
	RegisterPublicRoutesWithDeps(r, sharedapi.Dependencies{})
}

// RegisterPublicRoutesWithDeps 使用注入的基础设施句柄挂载无需认证的健康路由。
func RegisterPublicRoutesWithDeps(r gin.IRoutes, deps sharedapi.Dependencies) {
	healthAPI := newHealthAPIFromDeps(deps)
	r.GET("/health", healthAPI.Health)
	r.GET("/health/check", healthAPI.HealthCheck)
	r.GET("/health/live", healthAPI.Liveness)
	r.GET("/health/ready", healthAPI.Readiness)
}

func newHealthAPIFromDeps(deps sharedapi.Dependencies) *health.API {
	if deps.DB == nil && deps.Redis == nil {
		return health.New()
	}
	return health.NewWithClients(deps.DB, deps.Redis)
}
