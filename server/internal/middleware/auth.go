package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	authDAO "github.com/go-admin-kit/server/internal/dao/auth"
	"github.com/go-admin-kit/server/internal/pkg/cache"
	"github.com/go-admin-kit/server/internal/pkg/consoleauth"
	"github.com/go-admin-kit/server/internal/pkg/jwt"
	"github.com/go-admin-kit/server/internal/pkg/response"
	authSvc "github.com/go-admin-kit/server/internal/service/auth"
)

// AuthMiddleware JWT认证中间件
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从Authorization header中获取token
		authHeader := c.GetHeader("Authorization")
		tokenString, tokenSource := consoleauth.TokenFromGinContextWithSource(c)
		if authHeader == "" && tokenString == "" {
			response.Unauthorized(c, "Authorization header is required")
			c.Abort()
			return
		}

		// 检查token格式
		parts := strings.SplitN(authHeader, " ", 2)
		if authHeader != "" && !(len(parts) == 2 && parts[0] == "Bearer") {
			response.Unauthorized(c, "Authorization header format must be Bearer {token}")
			c.Abort()
			return
		}

		if jwt.IsTokenBlacklisted(tokenString) {
			response.Unauthorized(c, "Token has been revoked")
			c.Abort()
			return
		}

		// 解析token
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
			if _, err := (authSvc.ConsoleSessionService{}).ValidateActiveSession(claims.ID, claims.Username); err != nil {
				response.Unauthorized(c, "Console login required")
				c.Abort()
				return
			}
			userDAO := authDAO.UserDAO{}
			user, err := userDAO.GetUserWithRoles(claims.UserID)
			if err != nil || user.Status != 1 {
				response.Unauthorized(c, "Console login required")
				c.Abort()
				return
			}
		}

		// 将用户信息存储到上下文中
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		SetAuditActor(c, DefaultAuditActorType, claims.Username)

		c.Next()
	}
}

// RoleMiddleware 角色检查中间件
func RoleMiddleware(requiredRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			response.Unauthorized(c, "user not found in context")
			c.Abort()
			return
		}

		// 从缓存或数据库获取用户角色
		userDAO := authDAO.UserDAO{}
		user, err := userDAO.GetUserWithRoles(userID.(uint))
		if err != nil {
			response.Forbidden(c, "failed to get user roles")
			c.Abort()
			return
		}

		// 检查用户是否拥有所需角色之一
		hasRole := false
		for _, role := range user.Roles {
			for _, requiredRole := range requiredRoles {
				if role.Code == requiredRole {
					hasRole = true
					break
				}
			}
			if hasRole {
				break
			}
		}

		if !hasRole {
			response.Forbidden(c, "insufficient permissions")
			c.Abort()
			return
		}

		c.Next()
	}
}

// PermissionMiddleware 权限控制中间件
func PermissionMiddleware(requiredPermission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			response.Unauthorized(c, "user not found in context")
			c.Abort()
			return
		}

		if hasRole(userID.(uint), "super_admin") {
			c.Next()
			return
		}

		// 先从缓存获取权限
		cacheService := cache.NewCacheService()
		permissions, err := cacheService.GetUserPermissions(userID.(uint))
		if err != nil || len(permissions) == 0 {
			// 缓存未命中，从数据库获取
			permissionDAO := authDAO.PermissionDAO{}
			permissions, err = permissionDAO.GetUserPermissions(userID.(uint))
			if err != nil {
				response.Forbidden(c, "failed to get user permissions")
				c.Abort()
				return
			}

			// 更新缓存
			_ = cacheService.SetUserPermissions(userID.(uint), permissions)
		}

		// 检查是否拥有所需权限
		hasPermission := false
		for _, perm := range permissions {
			if perm == requiredPermission || perm == "*" || perm == "*:*:*" {
				hasPermission = true
				break
			}
		}

		if !hasPermission {
			response.Forbidden(c, "insufficient permissions")
			c.Abort()
			return
		}

		c.Next()
	}
}

// PermissionMiddlewareMultiple 多权限检查中间件（满足任一权限即可）
func PermissionMiddlewareMultiple(requiredPermissions ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			response.Unauthorized(c, "user not found in context")
			c.Abort()
			return
		}

		if hasRole(userID.(uint), "super_admin") {
			c.Next()
			return
		}

		// 先从缓存获取权限
		cacheService := cache.NewCacheService()
		permissions, err := cacheService.GetUserPermissions(userID.(uint))
		if err != nil || len(permissions) == 0 {
			// 缓存未命中，从数据库获取
			permissionDAO := authDAO.PermissionDAO{}
			permissions, err = permissionDAO.GetUserPermissions(userID.(uint))
			if err != nil {
				response.Forbidden(c, "failed to get user permissions")
				c.Abort()
				return
			}

			// 更新缓存
			_ = cacheService.SetUserPermissions(userID.(uint), permissions)
		}

		// 检查是否拥有任一所需权限
		hasPermission := false
		for _, perm := range permissions {
			for _, requiredPerm := range requiredPermissions {
				if perm == requiredPerm || perm == "*" || perm == "*:*:*" {
					hasPermission = true
					break
				}
			}
			if hasPermission {
				break
			}
		}

		if !hasPermission {
			response.Forbidden(c, "insufficient permissions")
			c.Abort()
			return
		}

		c.Next()
	}
}

func hasRole(userID uint, roleCodes ...string) bool {
	userDAO := authDAO.UserDAO{}
	user, err := userDAO.GetUserWithRoles(userID)
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
