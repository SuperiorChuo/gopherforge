package middleware

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	authDAO "github.com/go-admin-kit/server/internal/dao/auth"
	"github.com/go-admin-kit/server/internal/pkg/cache"
	"github.com/go-admin-kit/server/internal/pkg/consoleauth"
	"github.com/go-admin-kit/server/internal/pkg/jwt"
	"github.com/go-admin-kit/server/internal/pkg/response"
	authSvc "github.com/go-admin-kit/server/internal/service/auth"
)

// AuthMiddleware validates an access token and stores the actor in the request context.
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		tokenString, tokenSource := consoleauth.TokenFromGinContextWithSource(c)
		if authHeader == "" && tokenString == "" {
			response.Unauthorized(c, "Authorization header is required")
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if authHeader != "" && (len(parts) != 2 || parts[0] != "Bearer") {
			response.Unauthorized(c, "Authorization header format must be Bearer {token}")
			c.Abort()
			return
		}

		if jwt.IsTokenBlacklisted(tokenString) {
			response.Unauthorized(c, "Token has been revoked")
			c.Abort()
			return
		}

		claims, err := jwt.ParseToken(tokenString)
		if err != nil {
			var message string
			switch err {
			case jwt.ErrExpiredToken:
				message = "Token has expired"
			case jwt.ErrInvalidToken:
				message = "Invalid token"
			case jwt.ErrRevokedToken:
				message = "Token has been revoked"
			default:
				message = "Unauthorized"
			}
			response.Unauthorized(c, message)
			c.Abort()
			return
		}
		if claims.TokenType != jwt.AccessTokenType {
			response.Unauthorized(c, "Invalid token type")
			c.Abort()
			return
		}
		if tokenSource == consoleauth.TokenSourceCookie {
			if _, err := (authSvc.ConsoleSessionService{}).ValidateActiveSessionContext(c.Request.Context(), claims.ID, claims.Username); err != nil {
				response.Unauthorized(c, "Console login required")
				c.Abort()
				return
			}
			userDAO := authDAO.UserDAO{}
			user, err := userDAO.GetUserWithRolesContext(c.Request.Context(), claims.UserID)
			if err != nil || user.Status != 1 {
				response.Unauthorized(c, "Console login required")
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
			response.Unauthorized(c, "user not found in context")
			c.Abort()
			return
		}

		userDAO := authDAO.UserDAO{}
		user, err := userDAO.GetUserWithRolesContext(c.Request.Context(), userID.(uint))
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
			response.Unauthorized(c, "user not found in context")
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
			permissionDAO := authDAO.PermissionDAO{}
			permissions, err = permissionDAO.GetUserPermissionsContext(c.Request.Context(), userID.(uint))
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
	userDAO := authDAO.UserDAO{}
	user, err := userDAO.GetUserWithRolesContext(ctx, userID)
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
