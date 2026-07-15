package common

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRegisterPublicRoutes(t *testing.T) {
	routes := registeredCommonRoutes(func(r gin.IRoutes) {
		RegisterPublicRoutes(r)
	})

	for _, route := range []string{
		"GET /api/v1/health",
		"GET /api/v1/health/check",
		"GET /api/v1/health/live",
		"GET /api/v1/health/ready",
		"GET /api/v1/metrics/json",
		"GET /api/v1/metrics",
		"GET /api/v1/ip/info",
		"GET /api/v1/ip/location",
		"GET /api/v1/ip/me",
	} {
		if _, ok := routes[route]; !ok {
			t.Fatalf("route registration is missing: %s", route)
		}
	}
}

func TestHealthIncludesRuntimeInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := NewHealthAPI()
	router.GET("/api/v1/health", api.Health)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	var body struct {
		Data struct {
			Runtime struct {
				GoVersion string `json:"go_version"`
			} `json:"runtime"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.Runtime.GoVersion != runtime.Version() {
		t.Fatalf("go version = %q, want %q", body.Data.Runtime.GoVersion, runtime.Version())
	}
}

func registeredCommonRoutes(register func(gin.IRoutes)) map[string]struct{} {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1")
	register(group)

	routes := make(map[string]struct{}, len(router.Routes()))
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	return routes
}
