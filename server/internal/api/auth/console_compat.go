package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/config"
	"github.com/go-admin-kit/server/internal/middleware"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/consoleauth"
	"github.com/go-admin-kit/server/internal/pkg/jwt"
	"github.com/go-admin-kit/server/internal/pkg/response"
	authSvc "github.com/go-admin-kit/server/internal/service/auth"
	systemSvc "github.com/go-admin-kit/server/internal/service/system"
)

type consoleLoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type consoleSessionUser struct {
	ID                 uint     `json:"id"`
	Username           string   `json:"username"`
	DisplayName        string   `json:"display_name"`
	Role               string   `json:"role"`
	Roles              []string `json:"roles"`
	Permissions        []string `json:"permissions"`
	ActorType          string   `json:"actor_type"`
	ActorID            string   `json:"actor_id"`
	Nickname           string   `json:"nickname"`
	Avatar             string   `json:"avatar"`
	MustChangePassword bool     `json:"must_change_password"`
}

type consoleSessionResponse struct {
	Authenticated bool               `json:"authenticated"`
	AuthEnabled   bool               `json:"auth_enabled"`
	User          consoleSessionUser `json:"user"`
	ExpiresAt     string             `json:"expires_at"`
	TTLSec        int                `json:"ttl_sec"`
	AccessToken   string             `json:"access_token,omitempty"`
	RefreshToken  string             `json:"refresh_token,omitempty"`
}

type consoleSecurityPolicy struct {
	SessionTTLMinutes       int
	LoginMaxAttemptsPerHour int
	LockoutMinutes          int
}

func (a *UserAPI) LoginConsole(c *gin.Context) {
	var req consoleLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	policy := defaultConsoleSecurityPolicy()
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
		a.recordConsoleAuthAudit(c, "auth.login.failed", req.Username, nil, consoleAuthAttemptSnapshot(c, req.Username, "FAILED", "invalid_credentials"))
		response.Unauthorized(c, "Invalid console username or password")
		return
	}

	sessionRecord, err := a.consoleSessionService.CreateFromTokenContext(c.Request.Context(), loginResp.AccessToken, c.ClientIP(), c.GetHeader("User-Agent"))
	if err != nil {
		internalServerError(c, "failed to create console session", err)
		return
	}

	permissions := consolePermissionsForUser(c.Request.Context(), &loginResp.User, a.userService.GetUserPermissions(&loginResp.User))
	session := buildConsoleSession(&loginResp.User, permissions, loginResp.AccessToken, loginResp.RefreshToken)
	setConsoleSessionCookie(c, loginResp.AccessToken, session.TTLSec)
	a.recordOnlineUser(c, loginResp.AccessToken)
	a.recordConsoleAuthAudit(c, "auth.login.success", loginResp.User.Username, nil, consoleLoginSuccessSnapshot(c, sessionRecord, session.TTLSec))
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
	permissions := consolePermissionsForUser(c.Request.Context(), user, a.userService.GetUserPermissions(user))
	response.Success(c, buildConsoleSession(user, permissions, token, ""))
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

	permissions := consolePermissionsForUser(c.Request.Context(), user, a.userService.GetUserPermissions(user))
	roles := consoleRoleCodes(user.Roles)
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
		if claims, err := jwt.ParseToken(token); err == nil {
			username = claims.Username
			if record, revokeErr := a.consoleSessionService.RevokeByTokenContext(c.Request.Context(), token); revokeErr == nil {
				before = authSvc.ConsoleSessionSnapshot(record)
			}
			_ = jwt.RevokeToken(token, claims)
			_ = a.onlineUserService.RemoveOnlineUserContext(c.Request.Context(), claims.ID)
		}
	}
	a.recordConsoleAuthAudit(c, "auth.logout", auditTarget(username, "unknown"), before, consoleAuthAttemptSnapshot(c, username, "LOGOUT", ""))
	clearConsoleSessionCookie(c)
	response.Success(c, gin.H{"authenticated": false})
}

func defaultConsoleSecurityPolicy() consoleSecurityPolicy {
	sessionTTL := config.Cfg.JWT.AccessTokenExpire
	if sessionTTL <= 0 {
		sessionTTL = 480
	}
	maxAttempts := config.Cfg.Security.LoginLimit.MaxFailures
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	lockoutMinutes := config.Cfg.Security.LoginLimit.LockMinutes
	if lockoutMinutes <= 0 {
		lockoutMinutes = 15
	}
	return consoleSecurityPolicy{
		SessionTTLMinutes:       sessionTTL,
		LoginMaxAttemptsPerHour: maxAttempts,
		LockoutMinutes:          lockoutMinutes,
	}
}

func consoleLoginLimitConfig(policy consoleSecurityPolicy) middleware.LoginLimitConfig {
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
		TargetID:   auditTarget(targetID, "unknown"),
		Before:     before,
		After:      after,
		Summary:    consoleAuthAuditSummary(action, targetID),
	})
}

func consoleAuthAttemptSnapshot(c *gin.Context, username, result, reason string) map[string]any {
	snapshot := map[string]any{
		"username":   strings.TrimSpace(username),
		"ip":         c.ClientIP(),
		"user_agent": c.GetHeader("User-Agent"),
		"origin":     c.GetHeader("Origin"),
		"referer":    c.GetHeader("Referer"),
		"result":     result,
	}
	if reason != "" {
		snapshot["reason"] = reason
	}
	return snapshot
}

func consoleLoginSuccessSnapshot(c *gin.Context, record *model.ConsoleSession, ttlSec int) map[string]any {
	snapshot := consoleAuthAttemptSnapshot(c, record.Username, "SUCCESS", "")
	snapshot["session_id"] = record.SessionID
	snapshot["expires_at"] = record.ExpiresAt
	snapshot["ttl_sec"] = ttlSec
	return snapshot
}

func consoleAuthAuditSummary(action, targetID string) string {
	switch action {
	case "auth.login.success":
		return fmt.Sprintf("Console login succeeded for %s", auditTarget(targetID, "unknown"))
	case "auth.login.failed":
		return fmt.Sprintf("Console login failed for %s", auditTarget(targetID, "unknown"))
	case "auth.logout":
		return fmt.Sprintf("Console logout for %s", auditTarget(targetID, "unknown"))
	default:
		return fmt.Sprintf("Console auth event for %s", auditTarget(targetID, "unknown"))
	}
}

func auditTarget(value, fallback string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return fallback
}

func buildConsoleSession(user *model.User, permissions []string, accessToken, refreshToken string) consoleSessionResponse {
	expiresAt := time.Now().UTC().Add(time.Hour)
	if accessToken != "" {
		if claims, err := jwt.ParseToken(accessToken); err == nil && claims.ExpiresAt != nil {
			expiresAt = claims.ExpiresAt.UTC()
		}
	}
	ttl := int(time.Until(expiresAt).Seconds())
	if ttl < 0 {
		ttl = 0
	}
	return consoleSessionResponse{
		Authenticated: true,
		AuthEnabled:   true,
		User:          buildConsoleSessionUser(user, permissions),
		ExpiresAt:     expiresAt.Format(time.RFC3339),
		TTLSec:        ttl,
		AccessToken:   accessToken,
		RefreshToken:  refreshToken,
	}
}

func buildConsoleSessionUser(user *model.User, permissions []string) consoleSessionUser {
	roles := consoleRoleCodes(user.Roles)
	role := "operator"
	if len(roles) > 0 {
		role = roles[0]
	}
	displayName := strings.TrimSpace(user.Nickname)
	if displayName == "" {
		displayName = user.Username
	}
	return consoleSessionUser{
		ID:                 user.ID,
		Username:           user.Username,
		DisplayName:        displayName,
		Role:               role,
		Roles:              roles,
		Permissions:        permissions,
		ActorType:          "operator",
		ActorID:            user.Username,
		Nickname:           user.Nickname,
		Avatar:             user.Avatar,
		MustChangePassword: user.MustChangePassword,
	}
}

func consoleRoleCodes(roles []model.Role) []string {
	values := make([]string, 0, len(roles))
	for _, role := range roles {
		code := strings.TrimSpace(role.Code)
		if code != "" {
			values = append(values, code)
		}
	}
	return authSvc.UniqueSortedConsoleStrings(values)
}

func consolePermissionsForUser(ctx context.Context, user *model.User, base []string) []string {
	values := append([]string{}, base...)
	values = append(values, consolePermissionAliases(base)...)
	if consoleHasRole(user, "super_admin") {
		routePermissions, err := authSvc.ConsoleRouteService{}.AllRoutePermissionsContext(ctx)
		if err != nil {
			routePermissions = authSvc.AllConsoleRoutePermissions()
		}
		values = append(values, routePermissions...)
		values = append(values,
			"dashboard.view",
			"logs.read",
			"settings.read",
			"settings.write",
			"rbac.read",
			"rbac.write",
		)
	}
	return authSvc.UniqueSortedConsoleStrings(values)
}

func consolePermissionAliases(base []string) []string {
	aliasMap := map[string][]string{
		"system:log:audit":         {"logs.read"},
		"system:log:operation":     {"logs.read"},
		"system:user:list":         {"rbac.read"},
		"system:role:list":         {"rbac.read"},
		"system:permission:list":   {"rbac.read"},
		"system:department:list":   {"rbac.read"},
		"system:user:update":       {"rbac.write"},
		"system:role:update":       {"rbac.write"},
		"system:permission:update": {"rbac.write"},
		"system:department:update": {"rbac.write"},
		"system:monitor":           {"dashboard.view"},
	}
	values := []string{}
	for _, permission := range base {
		values = append(values, aliasMap[permission]...)
	}
	return values
}

func consoleHasRole(user *model.User, roleCode string) bool {
	for _, role := range user.Roles {
		if strings.TrimSpace(role.Code) == roleCode {
			return true
		}
	}
	return false
}

func setConsoleSessionCookie(c *gin.Context, token string, ttlSec int) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(consoleauth.SessionCookieName, token, ttlSec, "/", "", secureConsoleCookie(), true)
}

func clearConsoleSessionCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(consoleauth.SessionCookieName, "", -1, "/", "", secureConsoleCookie(), true)
}

func secureConsoleCookie() bool {
	return strings.EqualFold(config.Cfg.App.Env, "production") || config.Cfg.Security.Headers.HSTS
}
