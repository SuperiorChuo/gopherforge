package auth

import (
	"github.com/gin-gonic/gin"
	sharedapi "github.com/go-admin-kit/services/auth/internal/api/shared"
	"github.com/go-admin-kit/services/auth/internal/middleware"
	authsvc "github.com/go-admin-kit/services/auth/internal/service/auth"
	systemsvc "github.com/go-admin-kit/services/auth/internal/service/system"
)

// newUserAPIFromDeps assembles a UserAPI from injected handles, falling back
// to the legacy zero-value wiring when no database handle is provided.
func newUserAPIFromDeps(deps sharedapi.Dependencies) *UserAPI {
	if deps.DB == nil {
		return NewUserAPI()
	}
	var onlineUserService onlineUserRecorder = &systemsvc.OnlineUserService{}
	if deps.Redis != nil {
		onlineUserService = systemsvc.NewOnlineUserServiceWithClient(deps.Redis)
	}
	return NewUserAPIWithServices(
		authsvc.NewUserServiceWithDB(deps.DB),
		authsvc.NewConsoleRouteServiceWithDB(deps.DB),
		authsvc.NewConsoleSessionServiceWithDB(deps.DB),
		onlineUserService,
		systemsvc.NewAuditLogServiceWithDB(deps.DB),
	)
}

func newOAuthAPIFromDeps(deps sharedapi.Dependencies) *OAuthAPI {
	if deps.DB == nil {
		return NewOAuthAPI()
	}
	return NewOAuthAPIWithService(authsvc.NewOAuthServiceWithDB(deps.DB))
}

// RegisterPublicRoutes mounts unauthenticated authentication routes using
// legacy global fallbacks.
func RegisterPublicRoutes(r gin.IRoutes) {
	RegisterPublicRoutesWithDeps(r, sharedapi.Dependencies{})
}

// RegisterPublicRoutesWithDeps mounts unauthenticated authentication routes
// with injected infrastructure handles.
func RegisterPublicRoutesWithDeps(r gin.IRoutes, deps sharedapi.Dependencies) {
	userAPI := newUserAPIFromDeps(deps)
	r.POST("/login", userAPI.Login)
	r.POST("/login/2fa/verify", userAPI.VerifyTOTPLogin)
	r.POST("/auth/login", userAPI.LoginConsole)
	r.POST("/auth/login/2fa/verify", userAPI.VerifyConsoleTOTPLogin)
	r.POST("/register", userAPI.Register)
	r.POST("/refresh", userAPI.RefreshToken)

	captchaAPI := NewCaptchaAPI()
	r.GET("/captcha", captchaAPI.GetCaptcha)
	r.POST("/captcha/verify", captchaAPI.VerifyCaptcha)

	oauthAPI := newOAuthAPIFromDeps(deps)
	r.GET("/oauth/github/login", oauthAPI.GithubLogin)
	r.GET("/oauth/github/callback", oauthAPI.GithubCallback)
	r.GET("/oauth/wechat/login", oauthAPI.WechatLogin)
	r.GET("/oauth/wechat/callback", oauthAPI.WechatCallback)

	// OAuth2 authorization-server protocol endpoints. These authenticate by
	// client credentials / opaque bearer, not the console JWT, so they live in
	// the public group.
	RegisterOAuth2PublicRoutes(r, deps)
}

// RegisterProtectedRoutes mounts authenticated console/user authentication
// routes using legacy global fallbacks.
func RegisterProtectedRoutes(r gin.IRoutes) {
	RegisterProtectedRoutesWithDeps(r, sharedapi.Dependencies{})
}

// RegisterProtectedRoutesWithDeps mounts authenticated console/user
// authentication routes with injected infrastructure handles.
func RegisterProtectedRoutesWithDeps(r gin.IRoutes, deps sharedapi.Dependencies) {
	userAPI := newUserAPIFromDeps(deps)
	r.GET("/auth/me", userAPI.GetConsoleSession)
	r.GET("/auth/routes", userAPI.GetConsoleRoutes)
	r.POST("/auth/logout", userAPI.LogoutConsole)
	r.GET("/console-routes", middleware.PermissionMiddleware("settings.write"), userAPI.ListConsoleRoutes)
	r.POST("/console-routes", middleware.PermissionMiddleware("settings.write"), userAPI.CreateConsoleRoute)
	r.GET("/console-routes/:route_key", middleware.PermissionMiddleware("settings.write"), userAPI.GetConsoleRoute)
	r.PUT("/console-routes/:route_key", middleware.PermissionMiddleware("settings.write"), userAPI.UpdateConsoleRoute)
	r.DELETE("/console-routes/:route_key", middleware.PermissionMiddleware("settings.write"), userAPI.DeleteConsoleRoute)
	r.GET("/user/me", userAPI.GetCurrentUser)
	r.PUT("/user/profile", userAPI.UpdateProfile)
	r.PUT("/user/password", userAPI.ChangePassword)
	r.POST("/user/2fa/setup", userAPI.SetupTOTP)
	r.POST("/user/2fa/enable", userAPI.EnableTOTP)
	r.POST("/user/2fa/disable", userAPI.DisableTOTP)
	r.POST("/user/2fa/recovery-codes", userAPI.RegenerateTOTPRecoveryCodes)
	r.POST("/logout", userAPI.Logout)

	oauthAPI := newOAuthAPIFromDeps(deps)
	r.POST("/oauth/bind", oauthAPI.BindOAuth)
	r.POST("/oauth/unbind", oauthAPI.UnbindOAuth)

	// OAuth2 consent (resource owner logs in as a normal user) + tenant-scoped
	// application/token management (behind per-action permission checks).
	RegisterOAuth2AuthorizeRoutes(r, deps)
	RegisterOAuth2AdminRoutes(r, deps)
}
