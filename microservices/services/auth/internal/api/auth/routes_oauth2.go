package auth

import (
	"github.com/gin-gonic/gin"
	sharedapi "github.com/go-admin-kit/services/auth/internal/api/shared"
	"github.com/go-admin-kit/services/auth/internal/config"
	"github.com/go-admin-kit/services/auth/internal/middleware"
	authsvc "github.com/go-admin-kit/services/auth/internal/service/auth"
	systemsvc "github.com/go-admin-kit/services/auth/internal/service/system"
)

func newOAuth2ServerAPIFromDeps(deps sharedapi.Dependencies) *OAuth2ServerAPI {
	if deps.DB == nil {
		return nil
	}
	oidc := authsvc.NewOIDCService(deps.DB, config.Cfg.JWT.OIDCIssuerURL)
	return NewOAuth2ServerAPI(authsvc.NewOAuth2ServerServiceWithDB(deps.DB, deps.Redis, oidc))
}

func newOAuth2AdminAPIFromDeps(deps sharedapi.Dependencies) *OAuth2AdminAPI {
	if deps.DB == nil {
		return nil
	}
	return NewOAuth2AdminAPI(
		authsvc.NewOAuth2ClientServiceWithDB(deps.DB),
		systemsvc.NewAuditLogServiceWithDB(deps.DB),
	)
}

// RegisterOAuth2PublicRoutes mounts the protocol endpoints that authenticate by
// their own means (client credentials / opaque bearer), NOT the console JWT.
func RegisterOAuth2PublicRoutes(r gin.IRoutes, deps sharedapi.Dependencies) {
	server := newOAuth2ServerAPIFromDeps(deps)
	if server == nil {
		return
	}
	r.POST("/oauth2/token", server.PostToken)
	r.POST("/oauth2/introspect", server.PostIntrospect)
	r.POST("/oauth2/revoke", server.PostRevoke)
	r.GET("/oauth2/userinfo", middleware.OAuth2BearerMiddleware(), server.GetUserInfo)
	// OIDC discovery + JWKS. Path-scoped issuer keeps these under the already
	// gateway-routed /api/v1/oauth2/ prefix.
	r.GET("/oauth2/.well-known/openid-configuration", server.GetOpenIDConfiguration)
	r.GET("/oauth2/jwks", server.GetJWKS)
}

// RegisterOAuth2AuthorizeRoutes mounts the consent endpoints (require console
// login — the resource owner authenticates as a normal user).
func RegisterOAuth2AuthorizeRoutes(r gin.IRoutes, deps sharedapi.Dependencies) {
	server := newOAuth2ServerAPIFromDeps(deps)
	if server == nil {
		return
	}
	r.GET("/oauth2/authorize", server.GetAuthorize)
	r.POST("/oauth2/authorize", server.PostAuthorize)
}

// RegisterOAuth2AdminRoutes mounts the tenant-scoped management endpoints
// behind per-action permission checks.
func RegisterOAuth2AdminRoutes(r gin.IRoutes, deps sharedapi.Dependencies) {
	admin := newOAuth2AdminAPIFromDeps(deps)
	if admin == nil {
		return
	}
	r.GET("/oauth2/catalog", middleware.PermissionMiddleware("system:oauth2-client:list"), admin.GetCatalog)
	r.GET("/oauth2/clients", middleware.PermissionMiddleware("system:oauth2-client:list"), admin.ListClients)
	r.GET("/oauth2/clients/:id", middleware.PermissionMiddleware("system:oauth2-client:list"), admin.GetClient)
	r.POST("/oauth2/clients", middleware.PermissionMiddleware("system:oauth2-client:create"), admin.CreateClient)
	r.PUT("/oauth2/clients/:id", middleware.PermissionMiddleware("system:oauth2-client:update"), admin.UpdateClient)
	r.POST("/oauth2/clients/:id/reset-secret", middleware.PermissionMiddleware("system:oauth2-client:reset-secret"), admin.ResetSecret)
	r.DELETE("/oauth2/clients/:id", middleware.PermissionMiddleware("system:oauth2-client:delete"), admin.DeleteClient)
	r.GET("/oauth2/tokens", middleware.PermissionMiddleware("system:oauth2-token:list"), admin.ListTokens)
	r.DELETE("/oauth2/tokens/:id", middleware.PermissionMiddleware("system:oauth2-token:delete"), admin.RevokeToken)
}
