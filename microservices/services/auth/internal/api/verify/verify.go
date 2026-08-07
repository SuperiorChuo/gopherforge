// Package verify implements the Traefik forwardAuth endpoint. Traefik calls
// GET /internal/verify for gateway-routed requests and copies the returned
// X-Auth-* headers upstream.
package verify

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/auth/internal/middleware"
	"github.com/go-admin-kit/services/auth/internal/pkg/cache"
	"github.com/go-admin-kit/services/auth/internal/pkg/jwt"
	"github.com/go-admin-kit/services/shared/pkg/consoleauth"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

// Response headers copied upstream by Traefik on successful verification.
const (
	HeaderUserID        = "X-Auth-User-ID"
	HeaderUsername      = "X-Auth-Username"
	HeaderTenantID      = "X-Auth-Tenant-ID"
	HeaderPlatformAdmin = "X-Auth-Platform-Admin"
	HeaderPermissions   = "X-Auth-Permissions"
)

const maxPermissionsHeaderBytes = 16 * 1024

// Handler verifies bearer tokens and console session cookies with the exact
// semantics of middleware.AuthMiddleware, except that a request carrying no
// credentials at all passes through anonymously (the monolith still enforces
// authentication on its protected routes).
type Handler struct {
	consoleSessions middleware.ConsoleSessionValidator
	users           middleware.AuthUserStore
	permissions     middleware.AuthPermissionStore
}

// NewHandler creates a forwardAuth verification handler.
func NewHandler(consoleSessions middleware.ConsoleSessionValidator, users middleware.AuthUserStore, permissions middleware.AuthPermissionStore) *Handler {
	return &Handler{consoleSessions: consoleSessions, users: users, permissions: permissions}
}

// Verify handles GET /internal/verify.
func (h *Handler) Verify(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	tokenString, tokenSource := consoleauth.TokenFromGinContextWithSource(c)

	// Anonymous pass-through: no bearer token and no console session cookie.
	if authHeader == "" && tokenString == "" {
		c.Status(http.StatusOK)
		return
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if authHeader != "" && (len(parts) != 2 || parts[0] != "Bearer") {
		response.UnauthorizedWithCode(c, response.ErrorCodeAuthHeaderInvalid, "Authorization header format must be Bearer {token}")
		return
	}

	claims, err := jwt.ParseTokenContext(c.Request.Context(), tokenString)
	if err != nil {
		var message string
		errorCode := response.ErrorCodeAuthTokenInvalid
		switch err {
		case jwt.ErrExpiredToken:
			message = "Token has expired"
			errorCode = response.ErrorCodeAuthTokenExpired
		case jwt.ErrInvalidToken:
			message = "Invalid token"
		case jwt.ErrRevokedToken:
			message = "Token has been revoked"
			errorCode = response.ErrorCodeAuthTokenRevoked
		default:
			message = "Unauthorized"
		}
		response.UnauthorizedWithCode(c, errorCode, message)
		return
	}
	if claims.TokenType != jwt.AccessTokenType {
		response.UnauthorizedWithCode(c, response.ErrorCodeAuthTokenInvalid, "Invalid token type")
		return
	}
	if tokenSource == consoleauth.TokenSourceCookie {
		if h == nil || h.consoleSessions == nil || h.users == nil {
			response.UnauthorizedWithCode(c, response.ErrorCodeConsoleLoginRequired, "Console login required")
			return
		}
		// Ride AuthMiddleware's session-validation cache (short TTL, invalidated
		// immediately by administrative actions): on a hit the whole cookie
		// branch collapses to one Redis GET; only a miss falls back to the
		// session SELECT + user SELECT and refills the cache.
		deps := middleware.AuthMiddlewareDependencies{
			ConsoleSessions: h.consoleSessions,
			Users:           h.users,
		}
		if !middleware.ConsoleSessionAuthorized(c.Request.Context(), deps, claims) {
			response.UnauthorizedWithCode(c, response.ErrorCodeConsoleLoginRequired, "Console login required")
			return
		}
	}

	c.Header(HeaderUserID, strconv.FormatUint(uint64(claims.UserID), 10))
	c.Header(HeaderUsername, claims.Username)
	c.Header(HeaderTenantID, strconv.FormatUint(uint64(jwt.NormalizeTenantID(claims.TenantID)), 10))
	if claims.PlatformAdmin {
		c.Header(HeaderPlatformAdmin, "1")
	} else {
		c.Header(HeaderPlatformAdmin, "0")
	}
	permissionsHeader, err := h.resolvePermissionsHeader(c, claims.UserID)
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "failed to resolve user permissions")
		return
	}
	if permissionsHeader != "" {
		c.Header(HeaderPermissions, permissionsHeader)
	}
	c.Status(http.StatusOK)
}

func (h *Handler) resolvePermissionsHeader(c *gin.Context, userID uint) (string, error) {
	if h == nil || h.users == nil || h.permissions == nil {
		return "", nil
	}

	cacheService := cache.NewCacheService()
	// Normalized-header cache: a single GET covers the hot path. A string key
	// can hold an empty value, so zero-role/zero-permission users hit it too
	// (the SET-based roles/permissions caches cannot store an empty set, which
	// used to send such users to the database on every request). Invalidated in
	// the same funnel as the roles/permissions caches (identity
	// InvalidatePermissionCacheForUsersContext).
	if header, ok := cacheService.GetUserPermHeaderContext(c.Request.Context(), userID); ok {
		return header, nil
	}

	roleCodes, err := cacheService.GetUserRolesContext(c.Request.Context(), userID)
	if err != nil || len(roleCodes) == 0 {
		user, loadErr := h.users.GetUserWithRolesContext(c.Request.Context(), userID)
		if loadErr != nil {
			return "", loadErr
		}
		roleCodes = make([]string, 0, len(user.Roles))
		for _, role := range user.Roles {
			roleCodes = append(roleCodes, role.Code)
		}
		_ = cacheService.SetUserRolesContext(c.Request.Context(), userID, roleCodes)
	}
	for _, code := range roleCodes {
		if code == "super_admin" {
			_ = cacheService.SetUserPermHeaderContext(c.Request.Context(), userID, "*")
			return "*", nil
		}
	}

	permissionCodes, err := cacheService.GetUserPermissionsContext(c.Request.Context(), userID)
	if err != nil || len(permissionCodes) == 0 {
		permissionCodes, err = h.permissions.GetUserPermissionsContext(c.Request.Context(), userID)
		if err != nil {
			return "", err
		}
		_ = cacheService.SetUserPermissionsContext(c.Request.Context(), userID, permissionCodes)
	}

	permissionCodes = normalizedPermissionCodes(permissionCodes)
	header := strings.Join(permissionCodes, ",")
	if len(header) > maxPermissionsHeaderBytes {
		return "", errPermissionsHeaderTooLarge
	}
	_ = cacheService.SetUserPermHeaderContext(c.Request.Context(), userID, header)
	return header, nil
}

var errPermissionsHeaderTooLarge = &permissionsHeaderError{}

type permissionsHeaderError struct{}

func (*permissionsHeaderError) Error() string { return "permissions header exceeds safe limit" }

func normalizedPermissionCodes(codes []string) []string {
	unique := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code == "" || strings.ContainsAny(code, ",\r\n") {
			continue
		}
		unique[code] = struct{}{}
	}
	normalized := make([]string, 0, len(unique))
	for code := range unique {
		normalized = append(normalized, code)
	}
	sort.Strings(normalized)
	return normalized
}
