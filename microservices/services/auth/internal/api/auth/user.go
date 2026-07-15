package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	sharedapi "github.com/go-admin-kit/services/auth/internal/api/shared"
	"github.com/go-admin-kit/services/auth/internal/events"
	"github.com/go-admin-kit/services/auth/internal/middleware"
	"github.com/go-admin-kit/services/auth/internal/pkg/consoleauth"
	"github.com/go-admin-kit/services/auth/internal/pkg/jwt"
	"github.com/go-admin-kit/services/auth/internal/pkg/logger"
	"github.com/go-admin-kit/services/auth/internal/pkg/response"
	"github.com/go-admin-kit/services/auth/internal/pkg/runtimeconfig"
	"github.com/go-admin-kit/services/auth/internal/service/auth"
	"github.com/go-admin-kit/services/auth/internal/service/system"
)

// UserAPI handles user authentication endpoints.
type UserAPI struct {
	userService           auth.UserService
	consoleRouteService   auth.ConsoleRouteService
	consoleSessionService auth.ConsoleSessionService
	onlineUserService     onlineUserRecorder
	auditService          system.AuditLogService
}

const invalidRequestBodyMessage = "invalid request body"
const onlineUserWriteTimeout = 3 * time.Second

type onlineUserRecorder interface {
	SetOnlineUserContext(ctx context.Context, user system.OnlineUser, expiration time.Duration) error
	RemoveOnlineUserContext(ctx context.Context, tokenID string) error
}

// NewUserAPI creates a UserAPI instance.
func NewUserAPI() *UserAPI {
	return &UserAPI{
		userService:           auth.UserService{},
		consoleRouteService:   auth.ConsoleRouteService{},
		consoleSessionService: auth.ConsoleSessionService{},
		onlineUserService:     &system.OnlineUserService{},
		auditService:          system.AuditLogService{},
	}
}

// NewUserAPIWithServices creates a UserAPI instance from injected services.
func NewUserAPIWithServices(
	userService auth.UserService,
	consoleRouteService auth.ConsoleRouteService,
	consoleSessionService auth.ConsoleSessionService,
	onlineUserService onlineUserRecorder,
	auditService system.AuditLogService,
) *UserAPI {
	return &UserAPI{
		userService:           userService,
		consoleRouteService:   consoleRouteService,
		consoleSessionService: consoleSessionService,
		onlineUserService:     onlineUserService,
		auditService:          auditService,
	}
}

// Login authenticates a user.
func (a *UserAPI) Login(c *gin.Context) {
	var req auth.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, invalidRequestBodyMessage)
		return
	}

	policy := runtimeconfig.DefaultSecurityPolicyReader().SecurityPolicy(c.Request.Context())
	loginLimitCfg := middleware.LoginLimitConfigFromPolicy(policy)
	loginIdentifier := middleware.LoginIdentifier(req.Username, c.ClientIP())
	if policy.LoginLimitEnabled {
		if locked, ttl := middleware.IsLoginLockedContext(c.Request.Context(), loginIdentifier, loginLimitCfg); locked {
			response.Error(c, 429, "too many failed login attempts, please try again later")
			c.Header("Retry-After", fmt.Sprintf("%.0f", ttl.Seconds()))
			return
		}
	}

	resp, err := a.userService.LoginContext(c.Request.Context(), req)
	if err != nil {
		if policy.LoginLimitEnabled {
			middleware.RecordLoginFailureContext(c.Request.Context(), loginIdentifier, loginLimitCfg)
		}
		publishLoginFailed(c, req.Username, err.Error())
		writeAuthServiceError(c, "login failed", err)
		return
	}
	if policy.LoginLimitEnabled {
		middleware.ClearLoginLimitContext(c.Request.Context(), loginIdentifier, loginLimitCfg)
	}

	// Extract permission codes for the frontend session payload.
	permissions := a.userService.GetUserPermissions(&resp.User)
	if resp.RequiresTOTP {
		loginResp := LoginResponseData{
			RequiresTOTP:    true,
			TOTPChallengeID: resp.TOTPChallengeID,
		}
		response.SuccessWithMessage(c, "totp verification required", loginResp)
		return
	}

	// Record the online user in Redis.
	tokenID := ""
	browser, os := system.ParseUserAgent(c.GetHeader("User-Agent"))
	tokenTTL := time.Hour
	var accessTokenExpiresAt time.Time
	if claims, err := jwt.ParseTokenContext(c.Request.Context(), resp.AccessToken); err == nil && claims.ExpiresAt != nil {
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
		a.recordOnlineUserAsync(c.Request.Context(), onlineUser, tokenTTL)
	}

	// Build the response payload.
	loginResp := LoginResponseData{
		User:            ConvertUserToResponse(&resp.User, permissions),
		AccessToken:     resp.AccessToken,
		RefreshToken:    resp.RefreshToken,
		RequiresTOTP:    resp.RequiresTOTP,
		TOTPChallengeID: resp.TOTPChallengeID,
	}

	publishLoginSuccess(c, resp.User.ID, resp.User.Username, events.LoginTypeAccount)

	targetUserID := resp.User.ID
	response.SuccessWithMessageMasked(c, "login success", loginResp, sharedapi.ShouldMask(resp.User.ID, &targetUserID, nil))
}

// VerifyTOTPLogin completes a two-factor login challenge.
func (a *UserAPI) VerifyTOTPLogin(c *gin.Context) {
	var req auth.VerifyTOTPLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, invalidRequestBodyMessage)
		return
	}

	policy := runtimeconfig.DefaultSecurityPolicyReader().SecurityPolicy(c.Request.Context())
	loginLimitCfg := middleware.LoginLimitConfigFromPolicy(policy)
	loginLimitCfg.KeyPrefix = "totp_login_limit"
	loginIdentifier := totpLoginIdentifier(req.ChallengeID, c.ClientIP())
	if policy.LoginLimitEnabled {
		if locked, ttl := middleware.IsLoginLockedContext(c.Request.Context(), loginIdentifier, loginLimitCfg); locked {
			response.Error(c, 429, "too many failed two-factor verification attempts, please try again later")
			c.Header("Retry-After", fmt.Sprintf("%.0f", ttl.Seconds()))
			return
		}
	}

	resp, err := a.userService.VerifyTOTPLoginContext(c.Request.Context(), req)
	if err != nil {
		if policy.LoginLimitEnabled {
			middleware.RecordLoginFailureContext(c.Request.Context(), loginIdentifier, loginLimitCfg)
		}
		publishLoginFailed(c, consoleTOTPChallengeUsername(req.ChallengeID), err.Error())
		writeAuthServiceError(c, "failed to verify totp login", err)
		return
	}
	if policy.LoginLimitEnabled {
		middleware.ClearLoginLimitContext(c.Request.Context(), loginIdentifier, loginLimitCfg)
	}

	publishLoginSuccess(c, resp.User.ID, resp.User.Username, events.LoginTypeTOTP)

	permissions := a.userService.GetUserPermissions(&resp.User)
	loginResp := LoginResponseData{
		User:         ConvertUserToResponse(&resp.User, permissions),
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
	}
	targetUserID := resp.User.ID
	response.SuccessWithMessageMasked(c, "login success", loginResp, sharedapi.ShouldMask(resp.User.ID, &targetUserID, nil))
}

func totpLoginIdentifier(challengeID, ip string) string {
	challengeID = strings.TrimSpace(challengeID)
	if claims, err := jwt.ParseTOTPChallenge(challengeID); err == nil && claims.UserID != 0 {
		return middleware.LoginIdentifier(fmt.Sprintf("totp:%d", claims.UserID), ip)
	}
	sum := sha256.Sum256([]byte(challengeID))
	return middleware.LoginIdentifier("totp:"+hex.EncodeToString(sum[:]), ip)
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

	targetUserID := user.ID
	response.SuccessMasked(c, userResp, sharedapi.ShouldMask(userID.(uint), &targetUserID, nil))
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
		case errors.Is(err, auth.ErrEmailAlreadyExists):
			response.Error(c, http.StatusConflict, auth.ErrEmailAlreadyExists.Error())
		case errors.Is(err, auth.ErrPhoneAlreadyExists):
			response.Error(c, http.StatusConflict, auth.ErrPhoneAlreadyExists.Error())
		case errors.Is(err, auth.ErrUserNotFound):
			response.NotFound(c, auth.ErrUserNotFound.Error())
		default:
			internalServerError(c, "failed to update profile", err)
		}
		return
	}

	permissions := a.userService.GetUserPermissions(user)
	targetUserID := user.ID
	response.SuccessMasked(c, ConvertUserToResponse(user, permissions), sharedapi.ShouldMask(userID.(uint), &targetUserID, nil))
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
	accessToken, refreshToken, err := jwt.RefreshTokenContext(c.Request.Context(), req.RefreshToken)
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

	claims, err := jwt.ParseTokenContext(c.Request.Context(), accessToken)
	if err != nil {
		writeJWTUnauthorizedError(c, err)
		return
	}
	if err := jwt.RevokeTokenContext(c.Request.Context(), accessToken, claims); err != nil {
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
		if refreshClaims, err := jwt.ParseTokenContext(c.Request.Context(), req.RefreshToken); err == nil {
			_ = jwt.RevokeTokenContext(c.Request.Context(), req.RefreshToken, refreshClaims)
		}
	}

	publishLogout(c, claims.UserID, claims.Username)

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

func (a *UserAPI) SetupTOTP(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "user not found in context")
		return
	}

	var req auth.TOTPSetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, invalidRequestBodyMessage)
		return
	}

	setup, err := a.userService.GenerateTOTPSetupContext(c.Request.Context(), userID.(uint), req)
	if err != nil {
		writeAuthServiceError(c, "failed to setup totp", err)
		return
	}
	response.Success(c, setup)
}

func (a *UserAPI) EnableTOTP(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "user not found in context")
		return
	}

	var req auth.TOTPVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, invalidRequestBodyMessage)
		return
	}
	recoveryCodes, err := a.userService.EnableTOTPContext(c.Request.Context(), userID.(uint), req)
	if err != nil {
		writeAuthServiceError(c, "failed to enable totp", err)
		return
	}
	response.SuccessWithMessage(c, "totp enabled successfully", recoveryCodes)
}

func (a *UserAPI) DisableTOTP(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "user not found in context")
		return
	}

	var req auth.TOTPVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, invalidRequestBodyMessage)
		return
	}
	if err := a.userService.DisableTOTPContext(c.Request.Context(), userID.(uint), req); err != nil {
		writeAuthServiceError(c, "failed to disable totp", err)
		return
	}
	response.SuccessWithMessage(c, "totp disabled successfully", nil)
}

func (a *UserAPI) RegenerateTOTPRecoveryCodes(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "user not found in context")
		return
	}

	var req auth.TOTPVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, invalidRequestBodyMessage)
		return
	}
	recoveryCodes, err := a.userService.RegenerateTOTPRecoveryCodesContext(c.Request.Context(), userID.(uint), req)
	if err != nil {
		writeAuthServiceError(c, "failed to regenerate totp recovery codes", err)
		return
	}
	response.SuccessWithMessage(c, "totp recovery codes regenerated successfully", recoveryCodes)
}

func bearerToken(c *gin.Context) string {
	return consoleauth.TokenFromGinContext(c)
}

func (a *UserAPI) recordOnlineUser(c *gin.Context, accessToken string) {
	claims, err := jwt.ParseTokenContext(c.Request.Context(), accessToken)
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
	a.recordOnlineUserAsync(c.Request.Context(), onlineUser, tokenTTL)
}

func (a *UserAPI) recordOnlineUserAsync(ctx context.Context, onlineUser system.OnlineUser, tokenTTL time.Duration) {
	go func() {
		writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), onlineUserWriteTimeout)
		defer cancel()

		if err := a.onlineUserService.SetOnlineUserContext(writeCtx, onlineUser, tokenTTL); err != nil {
			logAuthOnlineUserError("failed to record online user", err)
		}
	}()
}

func logAuthOnlineUserError(message string, err error) {
	if logger.Logger == nil {
		return
	}
	logger.Error(message, logger.Err(err))
}
