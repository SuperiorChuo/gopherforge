package authz

import (
	"context"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/dao/auth"
)

// UserHasPermission checks role bypasses and explicit permission codes for a user.
// Deprecated: use UserHasPermissionContext instead.
func UserHasPermission(userID uint, requiredPermission string) (bool, error) {
	return UserHasPermissionContext(context.Background(), userID, requiredPermission)
}

func UserHasPermissionContext(ctx context.Context, userID uint, requiredPermission string) (bool, error) {
	requiredPermission = strings.TrimSpace(requiredPermission)
	if requiredPermission == "" {
		return true, nil
	}
	if userID == 0 {
		return false, nil
	}

	userDAO := auth.UserDAO{}
	user, err := userDAO.GetUserWithRolesContext(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, role := range user.Roles {
		if role.Code == "super_admin" {
			return true, nil
		}
	}

	permissionDAO := auth.PermissionDAO{}
	permissions, err := permissionDAO.GetUserPermissionsContext(ctx, userID)
	if err != nil {
		return false, err
	}
	return MatchesPermission(permissions, requiredPermission), nil
}

// UserHasPermissionFromContext reads user_id from Gin context and checks a permission.
func UserHasPermissionFromContext(c *gin.Context, requiredPermission string) (bool, error) {
	userID, exists := c.Get("user_id")
	if !exists {
		return false, fmt.Errorf("user not found in context")
	}
	uid, ok := userID.(uint)
	if !ok {
		return false, fmt.Errorf("invalid user id in context")
	}
	ctx := context.Background()
	if c.Request != nil {
		ctx = c.Request.Context()
	}
	return UserHasPermissionContext(ctx, uid, requiredPermission)
}

// MatchesPermission applies the same wildcard rules used by the permission middleware.
func MatchesPermission(permissions []string, requiredPermission string) bool {
	requiredPermission = strings.TrimSpace(requiredPermission)
	for _, permission := range permissions {
		switch strings.TrimSpace(permission) {
		case requiredPermission, "*", "*:*:*":
			return true
		}
	}
	return false
}
