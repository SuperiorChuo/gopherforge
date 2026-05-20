package system

import (
	"errors"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
)

// PermissionManageDAO 权限管理数据访问对象
type PermissionManageDAO struct{}

// GetPermissionByID 根据ID获取权限
func (d *PermissionManageDAO) GetPermissionByID(id uint) (*model.Permission, error) {
	var permission model.Permission
	result := database.DB.First(&permission, id)
	return &permission, result.Error
}

// GetPermissionByCode 根据代码获取权限
func (d *PermissionManageDAO) GetPermissionByCode(code string) (*model.Permission, error) {
	var permission model.Permission
	result := database.DB.Where("code = ?", code).First(&permission)
	return &permission, result.Error
}

// GetPermissionList 获取权限列表（分页）
func (d *PermissionManageDAO) GetPermissionList(req pagination.PageRequest, keyword string, permissionType *int8) ([]model.Permission, int64, error) {
	var permissions []model.Permission
	var total int64

	query := database.DB.Model(&model.Permission{})

	// 关键词搜索
	if keyword != "" {
		query = query.Where("name LIKE ? OR code LIKE ? OR description LIKE ? OR path LIKE ?",
			"%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}

	// 类型筛选
	if permissionType != nil {
		query = query.Where("type = ?", *permissionType)
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	result := query.Scopes(pagination.Paginate(req)).
		Order("parent_id ASC, created_at ASC").
		Find(&permissions)

	return permissions, total, result.Error
}

// GetPermissionTree 获取权限树
func (d *PermissionManageDAO) GetPermissionTree() ([]model.Permission, error) {
	var permissions []model.Permission
	result := database.DB.Order("parent_id ASC, created_at ASC").Find(&permissions)
	if result.Error != nil {
		return nil, result.Error
	}

	// 构建树形结构
	return buildPermissionTree(permissions, 0), nil
}

// buildPermissionTree 构建权限树
func buildPermissionTree(permissions []model.Permission, parentID uint) []model.Permission {
	var tree []model.Permission
	for i := range permissions {
		if permissions[i].ParentID == parentID {
			children := buildPermissionTree(permissions, permissions[i].ID)
			permissions[i].Children = children
			tree = append(tree, permissions[i])
		}
	}
	return tree
}

// CreatePermission 创建权限
func (d *PermissionManageDAO) CreatePermission(permission *model.Permission) error {
	return database.DB.Create(permission).Error
}

// UpdatePermission 更新权限
func (d *PermissionManageDAO) UpdatePermission(permission *model.Permission) error {
	return database.DB.Save(permission).Error
}

// DeletePermission 删除权限
func (d *PermissionManageDAO) DeletePermission(id uint) error {
	// 检查是否有子权限
	var count int64
	database.DB.Model(&model.Permission{}).Where("parent_id = ?", id).Count(&count)
	if count > 0 {
		return errors.New("cannot delete permission with children")
	}

	// 先删除关联关系
	database.DB.Where("permission_id = ?", id).Delete(&model.RolePermission{})
	// 再删除权限
	return database.DB.Delete(&model.Permission{}, id).Error
}
