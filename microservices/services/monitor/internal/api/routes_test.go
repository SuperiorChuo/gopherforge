package api

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSetupRoutesIncludesDependencyBackedMonitorRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	SetupRoutes(router)

	routes := make(map[string]struct{}, len(router.Routes()))
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}

	for _, route := range []string{
		"GET /api/v1/monitor/mysql",
		"GET /api/v1/monitor/jobs",
		"GET /api/v1/monitor/jobs/health",
		"GET /api/v1/monitor/jobs/heartbeats",
		"POST /api/v1/monitor/jobs",
		"PUT /api/v1/monitor/jobs/:id",
		"DELETE /api/v1/monitor/jobs/:id",
		"POST /api/v1/monitor/jobs/:id/start",
		"POST /api/v1/monitor/jobs/:id/stop",
		"POST /api/v1/monitor/jobs/:id/run",
		"POST /api/v1/monitor/job-logs/cleanup",
	} {
		if _, ok := routes[route]; !ok {
			t.Fatalf("route registration is missing: %s", route)
		}
	}
}
