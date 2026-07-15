package middleware

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/system/internal/pkg/cache"
	"github.com/go-admin-kit/services/system/internal/pkg/consoleauth"
	"github.com/go-admin-kit/services/system/internal/pkg/jwt"
	"github.com/go-admin-kit/services/system/internal/pkg/response"
)

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
			if _, err := deps.ConsoleSessions.ValidateActiveSessionContext(c.Request.Context(), claims.ID, claims.Username); err != nil {
				response.UnauthorizedWithCode(c, response.ErrorCodeConsoleLoginRequired, "Console login required")
				c.Abort()
				return
			}
			user, err := deps.Users.GetUserWithRolesContext(c.Request.Context(), claims.UserID)
			if err != nil || user.Status != 1 {
				response.UnauthorizedWithCode(c, response.ErrorCodeConsoleLoginRequired, "Console login required")
				c.Abort()
				return
			}
		}

		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		SetAuditActor(c, DefaultAuditActorType, claims.Username)

		c.Next()
	}
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

		users := currentAuthDeps().Users
		if users == nil {
			response.Forbidden(c, "failed to get user roles")
			c.Abort()
			return
		}
		user, err := users.GetUserWithRolesContext(c.Request.Context(), userID.(uint))
		if err != nil {
			response.Forbidden(c, "failed to get user roles")
			c.Abort()
			return
		}

		hasRequiredRole := false
		for _, role := range user.Roles {
			for _, requiredRole := range requiredRoles {
				if role.Code == requiredRole {
					hasRequiredRole = true
					break
				}
			}
			if hasRequiredRole {
				break
			}
		}

		if !hasRequiredRole {
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

		if hasRoleContext(c.Request.Context(), userID.(uint), "super_admin") {
			c.Next()
			return
		}

		cacheService := cache.NewCacheService()
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
	users := currentAuthDeps().Users
	if users == nil {
		return false
	}
	user, err := users.GetUserWithRolesContext(ctx, userID)
	if err != nil {
		return false
	}
	for _, role := range user.Roles {
		for _, code := range roleCodes {
			if role.Code == code {
				return true
			}
		}
	}
	return false
}
