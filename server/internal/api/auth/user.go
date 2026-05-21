package auth

import (
	"context"
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

// UserAPI handles user authentication endpoints.
type UserAPI struct {
	userService           auth.UserService
	consoleRouteService   auth.ConsoleRouteService
	consoleSessionService auth.ConsoleSessionService
	onlineUserService     system.OnlineUserService
	auditService          system.AuditLogService
}

const invalidRequestBodyMessage = "invalid request body"

// NewUserAPI creates a UserAPI instance.
func NewUserAPI() *UserAPI {
	return &UserAPI{
		userService:           auth.UserService{},
		consoleRouteService:   auth.ConsoleRouteService{},
		consoleSessionService: auth.ConsoleSessionService{},
		onlineUserService:     system.OnlineUserService{},
		auditService:          system.AuditLogService{},
	}
}

// Login authenticates a user.
func (a *UserAPI) Login(c *gin.Context) {
	var req auth.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, invalidRequestBodyMessage)
		return
	}

	loginLimitCfg := middleware.LoginLimitConfigFromApp()
	loginIdentifier := middleware.LoginIdentifier(req.Username, c.ClientIP())
	if middleware.LoginLimitEnabled() {
		if locked, ttl := middleware.IsLoginLockedContext(c.Request.Context(), loginIdentifier, loginLimitCfg); locked {
			response.Error(c, 429, "too many failed login attempts, please try again later")
			c.Header("Retry-After", fmt.Sprintf("%.0f", ttl.Seconds()))
			return
		}
	}

	resp, err := a.userService.LoginContext(c.Request.Context(), req)
	if err != nil {
		if middleware.LoginLimitEnabled() {
			middleware.RecordLoginFailureContext(c.Request.Context(), loginIdentifier, loginLimitCfg)
		}
		writeAuthServiceError(c, "login failed", err)
		return
	}
	if middleware.LoginLimitEnabled() {
		middleware.ClearLoginLimitContext(c.Request.Context(), loginIdentifier, loginLimitCfg)
	}

	// Extract permission codes for the frontend session payload.
	permissions := a.userService.GetUserPermissions(&resp.User)

	// Record the online user in Redis.
	tokenID := ""
	browser, os := system.ParseUserAgent(c.GetHeader("User-Agent"))
	tokenTTL := time.Hour
	var accessTokenExpiresAt time.Time
	if claims, err := jwt.ParseToken(resp.AccessToken); err == nil && claims.ExpiresAt != nil {
		tokenID = claims.ID
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
		Location:             "",
		Browser:              browser,
		OS:                   os,
		LoginTime:            time.Now(),
		TokenID:              tokenID,
		AccessTokenExpiresAt: accessTokenExpiresAt,
	}
	// Write asynchronously so the login response is not blocked.
	if tokenID != "" {
		go a.onlineUserService.SetOnlineUserContext(context.WithoutCancel(c.Request.Context()), onlineUser, tokenTTL)
	}

	// Build the response payload.
	loginResp := gin.H{
		"user":          ConvertUserToResponse(&resp.User, permissions),
		"access_token":  resp.AccessToken,
		"refresh_token": resp.RefreshToken,
	}

	response.SuccessWithMessage(c, "login success", loginResp)
}

// Register creates a new user account.
func (a *UserAPI) Register(c *gin.Context) {
	var req auth.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, invalidRequestBodyMessage)
		return
	}

	user, err := a.userService.RegisterContext(c.Request.Context(), req)
	if err != nil {
		writeAuthServiceError(c, "failed to register user", err)
		return
	}

	response.SuccessWithMessage(c, "register success", user)
}

// GetCurrentUser returns the authenticated user's profile.
func (a *UserAPI) GetCurrentUser(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "user not found in context")
		return
	}

	// Load the user with roles and permissions.
	user, err := a.userService.GetUserWithRolesAndPermissionsContext(c.Request.Context(), userID.(uint))
	if err != nil {
		internalServerError(c, "failed to get current user", err)
		return
	}

	// Extract permission codes.
	permissions := a.userService.GetUserPermissions(user)

	// Build the response DTO.
	userResp := ConvertUserToResponse(user, permissions)

	response.Success(c, userResp)
}

// UpdateProfile updates the authenticated user's profile.
func (a *UserAPI) UpdateProfile(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "user not found in context")
		return
	}

	var req auth.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, invalidRequestBodyMessage)
		return
	}

	user, err := a.userService.UpdateProfileContext(c.Request.Context(), userID.(uint), req)
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
			internalServerError(c, "failed to update profile", err)
		}
		return
	}

	permissions := a.userService.GetUserPermissions(user)
	response.Success(c, ConvertUserToResponse(user, permissions))
}

// RefreshToken refreshes the access token and rotates the refresh token.
func (a *UserAPI) RefreshToken(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, invalidRequestBodyMessage)
		return
	}

	// Refresh the access token and rotate the refresh token.
	accessToken, refreshToken, err := jwt.RefreshToken(req.RefreshToken)
	if err != nil {
		writeJWTUnauthorizedError(c, err)
		return
	}
	a.recordOnlineUser(c, accessToken)

	response.Success(c, gin.H{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
}

// Logout revokes the current access token.
func (a *UserAPI) Logout(c *gin.Context) {
	accessToken := bearerToken(c)
	if accessToken == "" {
		response.Unauthorized(c, "Authorization header format must be Bearer {token}")
		return
	}

	claims, err := jwt.ParseToken(accessToken)
	if err != nil {
		writeJWTUnauthorizedError(c, err)
		return
	}
	if err := jwt.RevokeToken(accessToken, claims); err != nil {
		internalServerError(c, "failed to revoke access token", err)
		return
	}

	_ = a.onlineUserService.RemoveOnlineUserContext(c.Request.Context(), claims.ID)

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

// ChangePassword changes the authenticated user's password.
func (a *UserAPI) ChangePassword(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "user not found in context")
		return
	}

	var req auth.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, invalidRequestBodyMessage)
		return
	}

	if err := a.userService.ChangePasswordContext(c.Request.Context(), userID.(uint), req); err != nil {
		writeAuthServiceError(c, "failed to change password", err)
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

	user, err := a.userService.GetUserWithRolesAndPermissionsContext(c.Request.Context(), claims.UserID)
	if err != nil {
		return
	}

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
		TokenID:              claims.ID,
		AccessTokenExpiresAt: expiresAt,
	}
	go a.onlineUserService.SetOnlineUserContext(context.WithoutCancel(c.Request.Context()), onlineUser, tokenTTL)
}
