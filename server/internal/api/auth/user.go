package auth

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/middleware"
	"github.com/go-admin-kit/server/internal/pkg/consoleauth"
	"github.com/go-admin-kit/server/internal/pkg/jwt"
	"github.com/go-admin-kit/server/internal/pkg/response"
	"github.com/go-admin-kit/server/internal/service/auth"
	"github.com/go-admin-kit/server/internal/service/system"
)

// UserAPI 用户认证API
type UserAPI struct {
	userService           auth.UserService
	consoleRouteService   auth.ConsoleRouteService
	consoleSessionService auth.ConsoleSessionService
	onlineUserService     system.OnlineUserService
	auditService          system.AuditLogService
}

// NewUserAPI 创建UserAPI实例
func NewUserAPI() *UserAPI {
	return &UserAPI{
		userService:           auth.UserService{},
		consoleRouteService:   auth.ConsoleRouteService{},
		consoleSessionService: auth.ConsoleSessionService{},
		onlineUserService:     system.OnlineUserService{},
		auditService:          system.AuditLogService{},
	}
}

// Login 用户登录
func (a *UserAPI) Login(c *gin.Context) {
	var req auth.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	loginLimitCfg := middleware.LoginLimitConfigFromApp()
	loginIdentifier := middleware.LoginIdentifier(req.Username, c.ClientIP())
	if middleware.LoginLimitEnabled() {
		if locked, ttl := middleware.IsLoginLocked(loginIdentifier, loginLimitCfg); locked {
			response.Error(c, 429, "登录失败次数过多，请稍后再试")
			c.Header("Retry-After", fmt.Sprintf("%.0f", ttl.Seconds()))
			return
		}
	}

	resp, err := a.userService.Login(req)
	if err != nil {
		if middleware.LoginLimitEnabled() {
			middleware.RecordLoginFailure(loginIdentifier, loginLimitCfg)
		}
		response.Unauthorized(c, err.Error())
		return
	}
	if middleware.LoginLimitEnabled() {
		middleware.ClearLoginLimit(loginIdentifier, loginLimitCfg)
	}

	// 提取权限代码
	permissions := a.userService.GetUserPermissions(&resp.User)

	// 记录在线用户到 Redis
	tokenHash := md5.Sum([]byte(resp.AccessToken))
	tokenID := hex.EncodeToString(tokenHash[:8]) // 使用 token 前 8 字节的 hash 作为 ID
	browser, os := system.ParseUserAgent(c.GetHeader("User-Agent"))
	tokenTTL := time.Hour
	var accessTokenExpiresAt time.Time
	if claims, err := jwt.ParseToken(resp.AccessToken); err == nil && claims.ExpiresAt != nil {
		accessTokenExpiresAt = claims.ExpiresAt.Time
		if ttl := time.Until(accessTokenExpiresAt); ttl > 0 {
			tokenTTL = ttl
		}
	}

	onlineUser := system.OnlineUser{
		UserID:               resp.User.ID,
		Username:             resp.User.Username,
		Nickname:             resp.User.Nickname,
		IP:                   c.ClientIP(),
		Location:             "", // 可以通过 IP 解析获取
		Browser:              browser,
		OS:                   os,
		LoginTime:            time.Now(),
		TokenID:              tokenID,
		AccessToken:          resp.AccessToken,
		AccessTokenExpiresAt: accessTokenExpiresAt,
	}
	// 异步记录，不阻塞登录响应
	go a.onlineUserService.SetOnlineUser(onlineUser, tokenTTL)

	// 构建响应数据
	loginResp := gin.H{
		"user":          ConvertUserToResponse(&resp.User, permissions),
		"access_token":  resp.AccessToken,
		"refresh_token": resp.RefreshToken,
	}

	response.SuccessWithMessage(c, "login success", loginResp)
}

// Register 用户注册
func (a *UserAPI) Register(c *gin.Context) {
	var req auth.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	user, err := a.userService.Register(req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "register success", user)
}

// GetCurrentUser 获取当前用户信息
func (a *UserAPI) GetCurrentUser(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "user not found in context")
		return
	}

	// 获取用户及其角色和权限
	user, err := a.userService.GetUserWithRolesAndPermissions(userID.(uint))
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	// 提取权限代码
	permissions := a.userService.GetUserPermissions(user)

	// 构建响应 DTO
	userResp := ConvertUserToResponse(user, permissions)

	response.Success(c, userResp)
}

// UpdateProfile 更新当前用户个人资料
func (a *UserAPI) UpdateProfile(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "user not found in context")
		return
	}

	var req auth.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	user, err := a.userService.UpdateProfile(userID.(uint), req)
	if err != nil {
		var validationErr auth.ProfileValidationError
		switch {
		case errors.As(err, &validationErr):
			response.BadRequest(c, validationErr.Error())
		case errors.Is(err, auth.ErrEmailAlreadyExists), errors.Is(err, auth.ErrPhoneAlreadyExists):
			response.Error(c, http.StatusConflict, err.Error())
		case errors.Is(err, auth.ErrUserNotFound):
			response.NotFound(c, err.Error())
		default:
			response.InternalServerError(c, err.Error())
		}
		return
	}

	permissions := a.userService.GetUserPermissions(user)
	response.Success(c, ConvertUserToResponse(user, permissions))
}

// RefreshToken 刷新AccessToken
func (a *UserAPI) RefreshToken(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// 刷新token并轮换refresh token
	accessToken, refreshToken, err := jwt.RefreshToken(req.RefreshToken)
	if err != nil {
		response.Unauthorized(c, err.Error())
		return
	}
	a.recordOnlineUser(c, accessToken)

	response.Success(c, gin.H{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
}

// Logout 用户退出登录
func (a *UserAPI) Logout(c *gin.Context) {
	accessToken := bearerToken(c)
	if accessToken == "" {
		response.Unauthorized(c, "Authorization header format must be Bearer {token}")
		return
	}

	claims, err := jwt.ParseToken(accessToken)
	if err != nil {
		response.Unauthorized(c, err.Error())
		return
	}
	if err := jwt.RevokeToken(accessToken, claims); err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	tokenHash := md5.Sum([]byte(accessToken))
	tokenID := hex.EncodeToString(tokenHash[:8])
	_ = a.onlineUserService.RemoveOnlineUser(tokenID)

	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if c.Request.ContentLength > 0 {
		_ = c.ShouldBindJSON(&req)
	}
	if req.RefreshToken != "" {
		if refreshClaims, err := jwt.ParseToken(req.RefreshToken); err == nil {
			_ = jwt.RevokeToken(req.RefreshToken, refreshClaims)
		}
	}

	response.SuccessWithMessage(c, "logout success", nil)
}

// ChangePassword 修改密码
func (a *UserAPI) ChangePassword(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "user not found in context")
		return
	}

	var req auth.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if err := a.userService.ChangePassword(userID.(uint), req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "password changed successfully", nil)
}

func bearerToken(c *gin.Context) string {
	return consoleauth.TokenFromGinContext(c)
}

func (a *UserAPI) recordOnlineUser(c *gin.Context, accessToken string) {
	claims, err := jwt.ParseToken(accessToken)
	if err != nil || claims.ExpiresAt == nil {
		return
	}

	user, err := a.userService.GetUserWithRolesAndPermissions(claims.UserID)
	if err != nil {
		return
	}

	tokenHash := md5.Sum([]byte(accessToken))
	tokenID := hex.EncodeToString(tokenHash[:8])
	browser, os := system.ParseUserAgent(c.GetHeader("User-Agent"))
	expiresAt := claims.ExpiresAt.Time
	tokenTTL := time.Until(expiresAt)
	if tokenTTL <= 0 {
		return
	}

	onlineUser := system.OnlineUser{
		UserID:               user.ID,
		Username:             user.Username,
		Nickname:             user.Nickname,
		IP:                   c.ClientIP(),
		Location:             "",
		Browser:              browser,
		OS:                   os,
		LoginTime:            time.Now(),
		TokenID:              tokenID,
		AccessToken:          accessToken,
		AccessTokenExpiresAt: expiresAt,
	}
	go a.onlineUserService.SetOnlineUser(onlineUser, tokenTTL)
}
