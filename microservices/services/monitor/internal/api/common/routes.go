package common

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/monitor/internal/middleware"
	"github.com/go-admin-kit/services/shared/pkg/health"
	"github.com/go-admin-kit/services/shared/pkg/response"
	sharedapi "github.com/go-admin-kit/services/shared/pkg/sharedapi"
	"net/http"
)

// RegisterPublicRoutes mounts unauthenticated health, metrics, and IP lookup
// routes using the zero-value wiring.
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
	r.GET("/metrics/json", func(c *gin.Context) {
		response.Success(c, middleware.MetricsSnapshot())
	})
	r.GET("/metrics", func(c *gin.Context) {
		c.String(http.StatusOK, middleware.PrometheusMetrics())
	})

	ipInfoAPI := NewIPInfoAPI()
	r.GET("/ip/info", ipInfoAPI.GetIPInfo)
	r.GET("/ip/location", ipInfoAPI.GetIPLocation)
	r.GET("/ip/me", ipInfoAPI.GetMyIPInfo)
}

func newHealthAPIFromDeps(deps sharedapi.Dependencies) *health.API {
	if deps.DB == nil && deps.Redis == nil {
		return health.New()
	}
	return health.NewWithClients(deps.DB, deps.Redis)
}
