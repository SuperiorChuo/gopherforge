package middleware

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/consoleauth"
	"github.com/go-admin-kit/services/shared/pkg/response"

	"github.com/go-admin-kit/services/shared/pkg/jwt"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"github.com/go-admin-kit/services/system/internal/pkg/authz"
	"github.com/go-admin-kit/services/system/internal/pkg/cache"
)

// TenantIDContextKey 在 context.Context 中保存已认证的租户 ID
// （与 shared/pkg/tenant 同一 typed key，保证 middleware 写入与
// tenant.FromContext 读取一致）。
const TenantIDContextKey = tenant.ContextKey

var errAuthUserStoreMissing = errors.New("auth: user store is not configured")

// AuthMiddleware 校验访问令牌，并将操作者信息存入请求上下文。
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
		// 优先使用网关透传的租户（ForwardAuth 场景下）。
		if h := c.GetHeader("X-Auth-Tenant-ID"); h != "" {
			if n, err := strconv.ParseUint(h, 10, 64); err == nil && n > 0 {
				tenantID = uint(n)
			}
		}
		// Platform administrator authority is security-sensitive and must come
		// from the verified JWT. X-Auth-* headers are transport metadata from
		// ForwardAuth, but they are also client-controlled on any direct service
		// path and therefore must never be allowed to elevate this claim.
		platformAdmin := claims.PlatformAdmin
		// 平台运维人员可通过 X-Act-Tenant-ID 以其他租户身份操作（M4）。
		if platformAdmin {
			if h := c.GetHeader("X-Act-Tenant-ID"); h != "" {
				if n, err := strconv.ParseUint(h, 10, 64); err == nil && n > 0 {
					tenantID = uint(n)
				}
			}
		}
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("session_id", claims.ID)
		c.Set("tenant_id", tenantID)
		c.Set("platform_admin", platformAdmin)
		// 将租户写入请求上下文，供 DAO/服务使用。
		ctx := context.WithValue(c.Request.Context(), TenantIDContextKey, tenantID)
		ctx = context.WithValue(ctx, tenant.PlatformAdminContextKey, platformAdmin)
		c.Request = c.Request.WithContext(ctx)
		SetAuditActor(c, DefaultAuditActorType, claims.Username)

		c.Next()
	}
}

// consoleSessionAuthorized 校验基于 cookie 的控制台会话。
//
// 未走缓存时，每次请求要付出三次 SELECT 和一次 UPDATE：会话行、带预加载
// 角色的用户行，以及 last_seen_at 的刷新。它确立的两个事实——会话仍然存活、
// 账号处于启用状态——只会在显式的管理操作（登出、强制登出、禁用/删除）时
// 改变，而所有这些路径都会使该缓存失效。因此结果会以较短的 TTL 存进 Redis，
// 整个分支退化为一次 GET。
//
// 角色与权限有意不在这里缓存。它们各自放在带独立失效机制的角色/权限缓存中，
// 这样命中本缓存绝不会返回已撤销的权限。
func consoleSessionAuthorized(ctx context.Context, deps AuthMiddlewareDependencies, claims *jwt.Claims) bool {
	cacheService := cache.NewCacheService()
	if identity, ok := cacheService.GetConsoleSessionContext(ctx, claims.ID); ok {
		// 将缓存条目绑定到当前出示的令牌。若不这样做，会话 ID 冲突或
		// 重新签发的令牌可能会误用其他用户的缓存结果。
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

	// 用刚加载的用户回填角色缓存：否则 RoleMiddleware 和
	// PermissionMiddleware 会在本次请求的几微秒后重新读取同一行用户及其角色。
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

// RoleMiddleware 在当前用户拥有任一所需角色时放行请求。
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

// PlatformAdminMiddleware 将路由限制为仅平台运维人员可访问。
// 系统设置含 AI/SMTP 密钥等平台级配置，租户管理员不得读写。
func PlatformAdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if v, ok := c.Get("platform_admin"); !ok || v != true {
			response.Forbidden(c, "platform administrator required")
			c.Abort()
			return
		}
		c.Next()
	}
}

// PermissionMiddleware 在当前用户拥有任一所需权限时放行请求。
func PermissionMiddleware(requiredPermissions ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			response.UnauthorizedWithCode(c, response.ErrorCodeAuthContextMissing, "user not found in context")
			c.Abort()
			return
		}

		// 将角色码暂存起来，使 ResolveUserDataScopeFromContext 可以直接复用，
		// 不必再从数据库重新读取用户+角色。出错时行为与之前完全一致，不缓存任何内容。
		if codes, err := userRoleCodesContext(c.Request.Context(), userID.(uint)); err == nil {
			c.Set(authz.RoleCodesContextKey, codes)
			if containsAnyString(codes, []string{"super_admin"}) {
				c.Next()
				return
			}
		}

		cacheService := cache.NewCacheService() // 包级共享实例
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
