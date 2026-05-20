package auth

import (
	"context"

	"github.com/go-admin-kit/server/internal/pkg/database"
)

type PermissionDAO struct{}

func (d *PermissionDAO) GetUserPermissions(userID uint) ([]string, error) {
	return d.GetUserPermissionsContext(context.Background(), userID)
}

func (d *PermissionDAO) GetUserPermissionsContext(ctx context.Context, userID uint) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	var codes []string
	result := database.DB.
		WithContext(ctx).
		Table("users").
		Select("permissions.code").
		Joins("JOIN user_roles ON users.id = user_roles.user_id").
		Joins("JOIN roles ON user_roles.role_id = roles.id").
		Joins("JOIN role_permissions ON roles.id = role_permissions.role_id").
		Joins("JOIN permissions ON role_permissions.permission_id = permissions.id").
		Where("users.id = ?", userID).
		Pluck("permissions.code", &codes)
	if result.Error != nil {
		return nil, result.Error
	}
	return codes, nil
}

func (d *PermissionDAO) GetUserPermissionsByCode(userID uint) (map[string]bool, error) {
	return d.GetUserPermissionsByCodeContext(context.Background(), userID)
}

func (d *PermissionDAO) GetUserPermissionsByCodeContext(ctx context.Context, userID uint) (map[string]bool, error) {
	codes, err := d.GetUserPermissionsContext(ctx, userID)
	if err != nil {
		return nil, err
	}

	permissionMap := make(map[string]bool)
	for _, code := range codes {
		permissionMap[code] = true
	}
	return permissionMap, nil
}

func (d *PermissionDAO) HasPermission(userID uint, permissionCode string) (bool, error) {
	return d.HasPermissionContext(context.Background(), userID, permissionCode)
}

func (d *PermissionDAO) HasPermissionContext(ctx context.Context, userID uint, permissionCode string) (bool, error) {
	permissionMap, err := d.GetUserPermissionsByCodeContext(ctx, userID)
	if err != nil {
		return false, err
	}
	return permissionMap[permissionCode], nil
}
