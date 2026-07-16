package auth

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/auth/internal/events"
	"github.com/go-admin-kit/services/auth/internal/middleware"
	"github.com/go-admin-kit/services/auth/internal/pkg/consoleauth"
	"github.com/go-admin-kit/services/auth/internal/pkg/jwt"
	"github.com/go-admin-kit/services/auth/internal/pkg/response"
	authSvc "github.com/go-admin-kit/services/auth/internal/service/auth"
	systemSvc "github.com/go-admin-kit/services/auth/internal/service/system"
)

type consoleLoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (a *UserAPI) LoginConsole(c *gin.Context) {
	var req consoleLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, invalidRequestBodyMessage)
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	policy := authSvc.DefaultConsoleSecurityPolicyContext(c.Request.Context())
	loginLimitCfg := consoleLoginLimitConfig(policy)
	loginIdentifier := middleware.LoginIdentifier(req.Username, c.ClientIP())
	if locked, ttl := middleware.IsLoginLockedContext(c.Request.Context(), loginIdentifier, loginLimitCfg); locked {
		response.Error(c, http.StatusTooManyRequests, "login attempts exceeded; please retry later")
		c.Header("Retry-After", fmt.Sprintf("%.0f", ttl.Seconds()))
		return
	}

	loginResp, err := a.userService.LoginPasswordWithAccessTTLContext(c.Request.Context(), req.Username, req.Password, time.Duration(policy.SessionTTLMinutes)*time.Minute)
	if err != nil {
		middleware.RecordLoginFailureContext(c.Request.Context(), loginIdentifier, loginLimitCfg)
		a.recordConsoleAuthAudit(c, "auth.login.failed", req.Username, nil, authSvc.ConsoleAuthAttemptSnapshot(consoleAuthRequestMetadata(c), req.Username, "FAILED", "invalid_credentials"))
		publishLoginFailed(c, req.Username, "invalid_credentials", 1)
		response.Unauthorized(c, "Invalid console username or password")
		return
	}
	if loginResp.RequiresTOTP {
		response.Success(c, gin.H{
			"requires_totp":     true,
			"totp_challenge_id": loginResp.TOTPChallengeID,
		})
		return
	}

	a.writeConsoleLoginSession(c, loginResp)
}

func (a *UserAPI) VerifyConsoleTOTPLogin(c *gin.Context) {
	var req authSvc.VerifyTOTPLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, invalidRequestBodyMessage)
		return
	}

	policy := authSvc.DefaultConsoleSecurityPolicyContext(c.Request.Context())
	loginLimitCfg := consoleLoginLimitConfig(policy)
	loginLimitCfg.KeyPrefix = "console_totp_login_limit"
	loginIdentifier := totpLoginIdentifier(req.ChallengeID, c.ClientIP())
	if locked, ttl := middleware.IsLoginLockedContext(c.Request.Context(), loginIdentifier, loginLimitCfg); locked {
		response.Error(c, http.StatusTooManyRequests, "two-factor verification attempts exceeded; please retry later")
		c.Header("Retry-After", fmt.Sprintf("%.0f", ttl.Seconds()))
		return
	}

	loginResp, err := a.userService.VerifyTOTPLoginWithAccessTTLContext(c.Request.Context(), req, time.Duration(policy.SessionTTLMinutes)*time.Minute)
	if err != nil {
		middleware.RecordLoginFailureContext(c.Request.Context(), loginIdentifier, loginLimitCfg)
		a.recordConsoleAuthAudit(c, "auth.login.failed", consoleTOTPChallengeUsername(req.ChallengeID), nil, authSvc.ConsoleAuthAttemptSnapshot(consoleAuthRequestMetadata(c), consoleTOTPChallengeUsername(req.ChallengeID), "FAILED", "invalid_totp"))
		publishLoginFailed(c, consoleTOTPChallengeUsername(req.ChallengeID), "invalid_totp", 1)
		writeAuthServiceError(c, "failed to verify console totp login", err)
		return
	}
	middleware.ClearLoginLimitContext(c.Request.Context(), loginIdentifier, loginLimitCfg)

	a.writeConsoleLoginSession(c, loginResp)
}

func (a *UserAPI) writeConsoleLoginSession(c *gin.Context, loginResp *authSvc.LoginResponse) {
	sessionRecord, err := a.consoleSessionService.CreateFromTokenContext(c.Request.Context(), loginResp.AccessToken, c.ClientIP(), c.GetHeader("User-Agent"))
	if err != nil {
		internalServerError(c, "failed to create console session", err)
		return
	}

	permissions := authSvc.ConsolePermissionsForUser(c.Request.Context(), a.consoleRouteService, &loginResp.User, a.userService.GetUserPermissions(&loginResp.User))
	session := authSvc.BuildConsoleSession(c.Request.Context(), &loginResp.User, permissions, loginResp.AccessToken, loginResp.RefreshToken)
	setConsoleSessionCookie(c, loginResp.AccessToken, session.TTLSec)
	a.recordOnlineUser(c, loginResp.AccessToken)
	a.recordConsoleAuthAudit(c, "auth.login.success", loginResp.User.Username, nil, authSvc.ConsoleLoginSuccessSnapshot(consoleAuthRequestMetadata(c), sessionRecord, session.TTLSec))
	publishLoginSuccess(c, loginResp.User.ID, loginResp.User.Username, events.LoginTypeConsole, loginResp.User.TenantID)
	response.Success(c, session)
}

func (a *UserAPI) GetConsoleSession(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "user not found in context")
		return
	}

	user, err := a.userService.GetUserWithRolesAndPermissionsContext(c.Request.Context(), userID.(uint))
	if err != nil {
		internalServerError(c, "failed to get console session user", err)
		return
	}
	if user.Status != 1 {
		if token := consoleauth.TokenFromGinContext(c); token != "" {
			_, _ = a.consoleSessionService.RevokeByTokenContext(c.Request.Context(), token)
		}
		clearConsoleSessionCookie(c)
		response.Unauthorized(c, "Console login required")
		return
	}

	token := consoleauth.TokenFromGinContext(c)
	permissions := authSvc.ConsolePermissionsForUser(c.Request.Context(), a.consoleRouteService, user, a.userService.GetUserPermissions(user))
	response.Success(c, authSvc.BuildConsoleSession(c.Request.Context(), user, permissions, token, ""))
}

func (a *UserAPI) GetConsoleRoutes(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "user not found in context")
		return
	}

	user, err := a.userService.GetUserWithRolesAndPermissionsContext(c.Request.Context(), userID.(uint))
	if err != nil {
		internalServerError(c, "failed to get console route user", err)
		return
	}

	permissions := authSvc.ConsolePermissionsForUser(c.Request.Context(), a.consoleRouteService, user, a.userService.GetUserPermissions(user))
	roles := authSvc.ConsoleRoleCodes(user.Roles)
	routes, err := a.consoleRouteService.ListAccessibleRoutesContext(c.Request.Context(), permissions, roles)
	if err != nil {
		internalServerError(c, "failed to get console routes", err)
		return
	}
	response.Success(c, gin.H{"items": routes})
}

func (a *UserAPI) LogoutConsole(c *gin.Context) {
	token := consoleauth.TokenFromGinContext(c)
	var username string
	var before map[string]any
	if token != "" {
		if claims, err := jwt.ParseTokenContext(c.Request.Context(), token); err == nil {
			username = claims.Username
			if record, revokeErr := a.consoleSessionService.RevokeByTokenContext(c.Request.Context(), token); revokeErr == nil {
				before = authSvc.ConsoleSessionSnapshot(record)
			}
			_ = jwt.RevokeTokenContext(c.Request.Context(), token, claims)
			_ = a.onlineUserService.RemoveOnlineUserContext(c.Request.Context(), claims.ID)
			publishLogout(c, claims.UserID, claims.Username)
		}
	}
	a.recordConsoleAuthAudit(c, "auth.logout", authSvc.ConsoleAuditTarget(username, "unknown"), before, authSvc.ConsoleAuthAttemptSnapshot(consoleAuthRequestMetadata(c), username, "LOGOUT", ""))
	clearConsoleSessionCookie(c)
	response.Success(c, gin.H{"authenticated": false})
}

func consoleLoginLimitConfig(policy authSvc.ConsoleSecurityPolicy) middleware.LoginLimitConfig {
	return middleware.LoginLimitConfig{
		Window:       time.Hour,
		MaxFailures:  policy.LoginMaxAttemptsPerHour,
		LockDuration: time.Duration(policy.LockoutMinutes) * time.Minute,
		KeyPrefix:    "console_login_limit",
	}
}

func (a *UserAPI) recordConsoleAuthAudit(c *gin.Context, action, targetID string, before, after map[string]any) {
	_ = a.auditService.Record(c, systemSvc.AuditRecordRequest{
		Action:     action,
		TargetType: "console_session",
		TargetID:   authSvc.ConsoleAuditTarget(targetID, "unknown"),
		Before:     before,
		After:      after,
		Summary:    authSvc.ConsoleAuthAuditSummary(action, targetID),
	})
}

func consoleAuthRequestMetadata(c *gin.Context) authSvc.ConsoleAuthRequestMetadata {
	if c == nil {
		return authSvc.ConsoleAuthRequestMetadata{}
	}
	return authSvc.ConsoleAuthRequestMetadata{
		IP:        c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
		Origin:    c.GetHeader("Origin"),
		Referer:   c.GetHeader("Referer"),
	}
}

func consoleTOTPChallengeUsername(challengeID string) string {
	claims, err := jwt.ParseTOTPChallenge(strings.TrimSpace(challengeID))
	if err != nil {
		return ""
	}
	return claims.Username
}

func setConsoleSessionCookie(c *gin.Context, token string, ttlSec int) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(consoleauth.SessionCookieName, token, ttlSec, "/", "", authSvc.SecureConsoleCookie(), true)
}

func clearConsoleSessionCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(consoleauth.SessionCookieName, "", -1, "/", "", authSvc.SecureConsoleCookie(), true)
}
