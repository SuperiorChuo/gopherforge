package auth

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRegisterPublicRoutes(t *testing.T) {
	routes := registeredAuthRoutes(func(r gin.IRoutes) {
		RegisterPublicRoutes(r)
	})

	for _, route := range []string{
		"POST /api/v1/login",
		"POST /api/v1/login/2fa/verify",
		"POST /api/v1/auth/login",
		"POST /api/v1/auth/login/2fa/verify",
		"POST /api/v1/register",
		"POST /api/v1/refresh",
		"GET /api/v1/captcha",
		"POST /api/v1/captcha/verify",
		"GET /api/v1/oauth/github/login",
		"GET /api/v1/oauth/github/callback",
		"GET /api/v1/oauth/wechat/login",
		"GET /api/v1/oauth/wechat/callback",
	} {
		if _, ok := routes[route]; !ok {
			t.Fatalf("route registration is missing: %s", route)
		}
	}
}

func TestRegisterProtectedRoutes(t *testing.T) {
	routes := registeredAuthRoutes(func(r gin.IRoutes) {
		RegisterProtectedRoutes(r)
	})

	for _, route := range []string{
		"GET /api/v1/auth/me",
		"GET /api/v1/auth/routes",
		"POST /api/v1/auth/logout",
		"GET /api/v1/console-routes",
		"POST /api/v1/console-routes",
		"GET /api/v1/console-routes/:route_key",
		"PUT /api/v1/console-routes/:route_key",
		"DELETE /api/v1/console-routes/:route_key",
		"GET /api/v1/user/me",
		"PUT /api/v1/user/profile",
		"PUT /api/v1/user/password",
		"POST /api/v1/user/2fa/setup",
		"POST /api/v1/user/2fa/enable",
		"POST /api/v1/user/2fa/disable",
		"POST /api/v1/user/2fa/recovery-codes",
		"POST /api/v1/logout",
		"POST /api/v1/oauth/bind",
		"POST /api/v1/oauth/unbind",
	} {
		if _, ok := routes[route]; !ok {
			t.Fatalf("route registration is missing: %s", route)
		}
	}
}

func registeredAuthRoutes(register func(gin.IRoutes)) map[string]struct{} {
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
