package auth

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/middleware"
)

// RegisterPublicRoutes mounts unauthenticated authentication routes.
func RegisterPublicRoutes(r gin.IRoutes) {
	userAPI := NewUserAPI()
	r.POST("/login", userAPI.Login)
	r.POST("/auth/login", userAPI.LoginConsole)
	r.POST("/register", userAPI.Register)
	r.POST("/refresh", userAPI.RefreshToken)

	captchaAPI := NewCaptchaAPI()
	r.GET("/captcha", captchaAPI.GetCaptcha)
	r.POST("/captcha/verify", captchaAPI.VerifyCaptcha)

	oauthAPI := NewOAuthAPI()
	r.GET("/oauth/github/login", oauthAPI.GithubLogin)
	r.GET("/oauth/github/callback", oauthAPI.GithubCallback)
	r.GET("/oauth/wechat/login", oauthAPI.WechatLogin)
	r.GET("/oauth/wechat/callback", oauthAPI.WechatCallback)
}

// RegisterProtectedRoutes mounts authenticated console/user authentication routes.
func RegisterProtectedRoutes(r gin.IRoutes) {
	userAPI := NewUserAPI()
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
	r.POST("/logout", userAPI.Logout)

	menuAPI := NewMenuAPI()
	r.GET("/user/menus", menuAPI.GetUserMenus)

	oauthAPI := NewOAuthAPI()
	r.POST("/oauth/bind", oauthAPI.BindOAuth)
	r.POST("/oauth/unbind", oauthAPI.UnbindOAuth)
}
