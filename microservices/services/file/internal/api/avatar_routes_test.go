package api

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSetupRoutesExposesSelfServiceAvatarEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	SetupRoutes(router)

	routes := make(map[string]struct{}, len(router.Routes()))
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}

	for _, want := range []string{
		"POST /api/v1/files/avatar",
		"POST /api/v1/files/avatar/cleanup",
		"GET /api/v1/files/avatars/:token",
	} {
		if _, ok := routes[want]; !ok {
			t.Fatalf("avatar route registration is missing: %s", want)
		}
	}
}
