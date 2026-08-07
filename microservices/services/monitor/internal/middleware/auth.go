package middleware

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/monitor/internal/pkg/authz"
	"github.com/go-admin-kit/services/monitor/internal/pkg/cache"
	"github.com/go-admin-kit/services/monitor/internal/pkg/jwt"
	"github.com/go-admin-kit/services/monitor/internal/pkg/tenantctx"
	"github.com/go-admin-kit/services/shared/pkg/consoleauth"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

// ctxKey is a private type so context keys cannot collide (SA1029).
type ctxKey string

// TenantIDContextKey stores the authenticated tenant id in context.Context.
// The key itself lives in the leaf tenantctx package so that packages below
// middleware (authz) can read it without an import cycle.
const TenantIDContextKey = tenantctx.Key

// platformAdminContextKey stores the platform-admin flag in context.Context.
const platformAdminContextKey ctxKey = "platform_admin"

var errAuthUserStoreMissing = errors.New("auth: user store is not configured")

// AuthMiddleware validates an access token and stores the actor in the request context.
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		tokenString, tokenSource := consoleauth.TokenFromGinContextWithSource(c)
		if authHeader == "" && tokenString == "" {
			response.UnauthorizedWithCode(c, response.ErrorCodeAuthHeaderMissing, "Authorization header is required")
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if authHeader != "" && (len(parts) != 2 || parts[0] != "Bearer") {
			response.UnauthorizedWithCode(c, response.ErrorCodeAuthHeaderInvalid, "Authorization header format must be Bearer {token}")
			c.Abort()
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
			c.Abort()
			return
		}
		if claims.TokenType != jwt.AccessTokenType {
			response.UnauthorizedWithCode(c, response.ErrorCodeAuthTokenInvalid, "Invalid token type")
			c.Abort()
			return
		}
		if tokenSource == consoleauth.TokenSourceCookie {
			deps := currentAuthDeps()
			if deps.ConsoleSessions == nil || deps.Users == nil {
				response.UnauthorizedWithCode(c, response.ErrorCodeConsoleLoginRequired, "Console login required")
				c.Abort()
				return
			}
			if !consoleSessionAuthorized(c.Request.Context(), deps, claims) {
				response.UnauthorizedWithCode(c, response.ErrorCodeConsoleLoginRequired, "Console login required")
				c.Abort()
				return
			}
		}

		tenantID := jwt.NormalizeTenantID(claims.TenantID)
		// Prefer gateway-propagated tenant when present (ForwardAuth).
		if h := c.GetHeader("X-Auth-Tenant-ID"); h != "" {
			if n, err := strconv.ParseUint(h, 10, 64); err == nil && n > 0 {
				tenantID = uint(n)
			}
		}
		platformAdmin := claims.PlatformAdmin
		if h := c.GetHeader("X-Auth-Platform-Admin"); h == "1" || strings.EqualFold(h, "true") {
			platformAdmin = true
		}
		// Platform operators may act-as another tenant via X-Act-Tenant-ID (M4).
		if platformAdmin {
			if h := c.GetHeader("X-Act-Tenant-ID"); h != "" {
				if n, err := strconv.ParseUint(h, 10, 64); err == nil && n > 0 {
					tenantID = uint(n)
				}
			}
		}
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("tenant_id", tenantID)
		c.Set("platform_admin", platformAdmin)
		// Propagate tenant into request context for DAOs/services.
		ctx := context.WithValue(c.Request.Context(), TenantIDContextKey, tenantID)
		ctx = context.WithValue(ctx, platformAdminContextKey, platformAdmin)
		c.Request = c.Request.WithContext(ctx)
		SetAuditActor(c, DefaultAuditActorType, claims.Username)

		c.Next()
	}
}

// consoleSessionAuthorized validates a cookie-borne console session.
//
// The uncached path costs three SELECTs and one UPDATE per request: the session
// row, the user row with preloaded roles, and the last_seen_at touch. Both facts
// it establishes — the session is live, and the account is enabled — change only
// on explicit administrative action (logout, force-logout, disable/delete), and
// every one of those paths already invalidates this cache. So the result is held
// in Redis for a short TTL and the whole branch collapses to a single GET.
//
// Roles and permissions are deliberately not cached here. They stay on the
// role/permission caches with their own invalidation, so a hit on this cache can
// never hand back a privilege that was revoked.
func consoleSessionAuthorized(ctx context.Context, deps AuthMiddlewareDependencies, claims *jwt.Claims) bool {
	cacheService := cache.NewCacheService()
	if identity, ok := cacheService.GetConsoleSessionContext(ctx, claims.ID); ok {
		// Bind the cached entry to the presented token. Without this a session id
		// collision or a re-issued token could ride another user's cached result.
		if identity.UserID == claims.UserID &&
			(claims.Username == "" || identity.Username == claims.Username) {
			return true
		}
		_ = cacheService.DelConsoleSessionContext(ctx, claims.ID)
	}

	if _, err := deps.ConsoleSessions.ValidateActiveSessionContext(ctx, claims.ID, claims.Username); err != nil {
		return false
	}
	user, err := deps.Users.GetUserWithRolesContext(ctx, claims.UserID)
	if err != nil || user.Status != 1 {
		return false
	}

	// Feed the role cache from the user we just loaded: RoleMiddleware and
	// PermissionMiddleware would otherwise re-read the same row and roles a few
	// microseconds later on this very request.
	codes := make([]string, 0, len(user.Roles))
	for _, role := range user.Roles {
		codes = append(codes, role.Code)
	}
	_ = cacheService.SetUserRolesContext(ctx, claims.UserID, codes)

	_ = cacheService.SetConsoleSessionContext(ctx, claims.ID, cache.ConsoleSessionIdentity{
		UserID:   claims.UserID,
		Username: claims.Username,
		TenantID: jwt.NormalizeTenantID(claims.TenantID),
	})
	return true
}

// RoleMiddleware allows the request when the current user has any required role.
func RoleMiddleware(requiredRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			response.UnauthorizedWithCode(c, response.ErrorCodeAuthContextMissing, "user not found in context")
			c.Abort()
			return
		}

		grantedRoles, err := userRoleCodesContext(c.Request.Context(), userID.(uint))
		if err != nil {
			response.Forbidden(c, "failed to get user roles")
			c.Abort()
			return
		}

		if !containsAnyString(grantedRoles, requiredRoles) {
			response.Forbidden(c, "insufficient permissions")
			c.Abort()
			return
		}

		c.Next()
	}
}

// PermissionMiddleware allows the request when the current user has any required permission.
func PermissionMiddleware(requiredPermissions ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			response.UnauthorizedWithCode(c, response.ErrorCodeAuthContextMissing, "user not found in context")
			c.Abort()
			return
		}

		// Memoize the codes so ResolveUserDataScopeFromContext can reuse them
		// instead of re-reading user+roles from the database. On error we fall
		// through exactly as before, memoizing nothing.
		if codes, err := userRoleCodesContext(c.Request.Context(), userID.(uint)); err == nil {
			c.Set(authz.RoleCodesContextKey, codes)
			if containsAnyString(codes, []string{"super_admin"}) {
				c.Next()
				return
			}
		}

		cacheService := cache.NewCacheService() // shared package-level instance
		permissions, err := cacheService.GetUserPermissionsContext(c.Request.Context(), userID.(uint))
		if err != nil || len(permissions) == 0 {
			store := currentAuthDeps().Permissions
			if store == nil {
				response.Forbidden(c, "failed to get user permissions")
				c.Abort()
				return
			}
			permissions, err = store.GetUserPermissionsContext(c.Request.Context(), userID.(uint))
			if err != nil {
				response.Forbidden(c, "failed to get user permissions")
				c.Abort()
				return
			}

			_ = cacheService.SetUserPermissionsContext(c.Request.Context(), userID.(uint), permissions)
		}

		if !hasAnyRequiredPermission(permissions, requiredPermissions) {
			response.Forbidden(c, "insufficient permissions")
			c.Abort()
			return
		}

		c.Next()
	}
}

func hasAnyRequiredPermission(grantedPermissions []string, requiredPermissions []string) bool {
	for _, granted := range grantedPermissions {
		if granted == "*" || granted == "*:*:*" {
			return true
		}
		for _, required := range requiredPermissions {
			if granted == required {
				return true
			}
		}
	}
	return false
}

func hasRoleContext(ctx context.Context, userID uint, roleCodes ...string) bool {
	granted, err := userRoleCodesContext(ctx, userID)
	if err != nil {
		return false
	}
	return containsAnyString(granted, roleCodes)
}

// userRoleCodesContext 取用户角色码，优先读缓存。
// PermissionMiddleware 对每个受权限保护的请求都要先判一次 super_admin，
// 不缓存的话请求路径必查 users + 预加载 roles——权限本身早已缓存，这道前置
// 判定就成了鉴权链上唯一的必查项（super_admin 命中还会直接返回，缓存好的
// 权限连用都用不上）。失效与权限缓存同点位，见 service/system/cache.go。
func userRoleCodesContext(ctx context.Context, userID uint) ([]string, error) {
	cacheService := cache.NewCacheService()
	if cached, err := cacheService.GetUserRolesContext(ctx, userID); err == nil && len(cached) > 0 {
		return cached, nil
	}

	users := currentAuthDeps().Users
	if users == nil {
		return nil, errAuthUserStoreMissing
	}
	user, err := users.GetUserWithRolesContext(ctx, userID)
	if err != nil {
		return nil, err
	}

	codes := make([]string, 0, len(user.Roles))
	for _, role := range user.Roles {
		codes = append(codes, role.Code)
	}
	_ = cacheService.SetUserRolesContext(ctx, userID, codes)
	return codes, nil
}

func containsAnyString(granted []string, wanted []string) bool {
	for _, code := range granted {
		for _, want := range wanted {
			if code == want {
				return true
			}
		}
	}
	return false
}
