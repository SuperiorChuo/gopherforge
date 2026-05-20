package auth

import (
	"github.com/go-admin-kit/server/internal/pkg/database"
)

// PermissionDAO 权限数据访问对象（用于中间件查询用户权限）
type PermissionDAO struct{}

// GetUserPermissions 获取用户的所有权限代码
func (d *PermissionDAO) GetUserPermissions(userID uint) ([]string, error) {
	var codes []string

	// 通过用户 -> 角色 -> 权限的关联查询
	result := database.DB.
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

// GetUserPermissionsByCode 根据权限代码获取用户权限
func (d *PermissionDAO) GetUserPermissionsByCode(userID uint) (map[string]bool, error) {
	codes, err := d.GetUserPermissions(userID)
	if err != nil {
		return nil, err
	}

	// 转换为map，方便快速查找
	permissionMap := make(map[string]bool)
	for _, code := range codes {
		permissionMap[code] = true
	}

	return permissionMap, nil
}

// HasPermission 检查用户是否拥有指定权限
func (d *PermissionDAO) HasPermission(userID uint, permissionCode string) (bool, error) {
	permissionMap, err := d.GetUserPermissionsByCode(userID)
	if err != nil {
		return false, err
	}

	return permissionMap[permissionCode], nil
}
