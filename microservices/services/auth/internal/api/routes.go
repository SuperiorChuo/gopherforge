// Package api wires the auth service HTTP surface. The /api/v1 layout matches
// the monolith exactly for every extracted route; /internal/verify is new and
// deliberately outside /api so it stays unreachable through the gateway's
// /api routing rules.
package api

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/auth/internal/api/auth"
	"github.com/go-admin-kit/services/auth/internal/api/common"
	sharedapi "github.com/go-admin-kit/services/auth/internal/api/shared"
	"github.com/go-admin-kit/services/auth/internal/api/verify"
	authDAO "github.com/go-admin-kit/services/auth/internal/dao/auth"
	"github.com/go-admin-kit/services/auth/internal/middleware"
	authsvc "github.com/go-admin-kit/services/auth/internal/service/auth"
)

// SetupRoutes mounts the auth service API using legacy global fallbacks.
func SetupRoutes(router *gin.Engine) {
	SetupRoutesWithDeps(router, sharedapi.Dependencies{})
}

// SetupRoutesWithDeps mounts the API with injected infrastructure handles.
func SetupRoutesWithDeps(router *gin.Engine, deps sharedapi.Dependencies) {
	api := router.Group("/api/v1")

	common.RegisterPublicRoutesWithDeps(api, deps)

	public := api.Group("/")
	{
		auth.RegisterPublicRoutesWithDeps(public, deps)
	}

	protected := api.Group("/")
	protected.Use(middleware.AuthMiddleware())
	{
		auth.RegisterProtectedRoutesWithDeps(protected, deps)
	}

	router.GET("/internal/verify", newVerifyHandlerFromDeps(deps).Verify)
}

// newVerifyHandlerFromDeps assembles the forwardAuth handler from injected
// handles. Without a database handle the cookie branch reports "console login
// required", mirroring middleware.AuthMiddleware without dependencies.
func newVerifyHandlerFromDeps(deps sharedapi.Dependencies) *verify.Handler {
	if deps.DB == nil {
		return verify.NewHandler(nil, nil)
	}
	sessions := authsvc.NewConsoleSessionServiceWithDB(deps.DB)
	return verify.NewHandler(&sessions, authDAO.NewUserDAO(deps.DB))
}
