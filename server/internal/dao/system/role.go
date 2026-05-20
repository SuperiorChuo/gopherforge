package system

import (
	"gorm.io/gorm"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
)

// RoleDAO 角色数据访问对象
type RoleDAO struct{}

// GetRoleByID 根据ID获取角色
func (d *RoleDAO) GetRoleByID(id uint) (*model.Role, error) {
	var role model.Role
	result := database.DB.
		Preload("Permissions").
		Preload("DataScopeDepartments").
		First(&role, id)
	if result.Error != nil {
		return &role, result.Error
	}
	fillRoleDataScopeDepartmentIDs(&role)
	return &role, nil
}

// GetRoleByCode 根据代码获取角色
func (d *RoleDAO) GetRoleByCode(code string) (*model.Role, error) {
	var role model.Role
	result := database.DB.Where("code = ?", code).First(&role)
	return &role, result.Error
}

// GetRoleList 获取角色列表（分页）
func (d *RoleDAO) GetRoleList(req pagination.PageRequest, keyword string) ([]model.Role, int64, error) {
	var roles []model.Role
	var total int64

	query := database.DB.Model(&model.Role{})

	// 关键词搜索
	if keyword != "" {
		query = query.Where("name LIKE ? OR code LIKE ? OR description LIKE ?",
			"%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	result := query.Scopes(pagination.Paginate(req)).
		Preload("Permissions").
		Preload("DataScopeDepartments").
		Order("created_at DESC").
		Find(&roles)

	fillRolesDataScopeDepartmentIDs(roles)
	return roles, total, result.Error
}

// GetAllRoles 获取所有角色
func (d *RoleDAO) GetAllRoles() ([]model.Role, error) {
	var roles []model.Role
	result := database.DB.
		Preload("DataScopeDepartments").
		Order("created_at ASC").
		Find(&roles)
	fillRolesDataScopeDepartmentIDs(roles)
	return roles, result.Error
}

// CreateRole 创建角色
func (d *RoleDAO) CreateRole(role *model.Role) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(role).Error; err != nil {
			return err
		}
		if err := replaceRoleDataScopeDepartments(tx, role.ID, role.DataScope, role.DataScopeDepartmentIDs); err != nil {
			return err
		}
		return reloadRoleDataScopeDepartmentIDs(tx, role)
	})
}

// UpdateRole 更新角色
func (d *RoleDAO) UpdateRole(role *model.Role) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(role).Error; err != nil {
			return err
		}
		if err := replaceRoleDataScopeDepartments(tx, role.ID, role.DataScope, role.DataScopeDepartmentIDs); err != nil {
			return err
		}
		return reloadRoleDataScopeDepartmentIDs(tx, role)
	})
}

// DeleteRole 删除角色
func (d *RoleDAO) DeleteRole(id uint) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		// 先删除关联关系
		if err := tx.Where("role_id = ?", id).Delete(&model.RolePermission{}).Error; err != nil {
			return err
		}
		if err := tx.Where("role_id = ?", id).Delete(&model.UserRole{}).Error; err != nil {
			return err
		}
		if err := tx.Where("role_id = ?", id).Delete(&model.RoleDataScopeDepartment{}).Error; err != nil {
			return err
		}
		// 再删除角色
		if err := tx.Delete(&model.Role{}, id).Error; err != nil {
			return err
		}
		return nil
	})
}

func replaceRoleDataScopeDepartments(tx *gorm.DB, roleID uint, dataScope string, departmentIDs []uint) error {
	if err := tx.Where("role_id = ?", roleID).Delete(&model.RoleDataScopeDepartment{}).Error; err != nil {
		return err
	}

	if dataScope != "custom" || len(departmentIDs) == 0 {
		return nil
	}

	relations := make([]model.RoleDataScopeDepartment, 0, len(departmentIDs))
	seen := make(map[uint]struct{}, len(departmentIDs))
	for _, departmentID := range departmentIDs {
		if departmentID == 0 {
			continue
		}
		if _, ok := seen[departmentID]; ok {
			continue
		}
		seen[departmentID] = struct{}{}
		relations = append(relations, model.RoleDataScopeDepartment{
			RoleID:       roleID,
			DepartmentID: departmentID,
		})
	}
	if len(relations) == 0 {
		return nil
	}

	return tx.Create(&relations).Error
}

func fillRolesDataScopeDepartmentIDs(roles []model.Role) {
	for i := range roles {
		fillRoleDataScopeDepartmentIDs(&roles[i])
	}
}

func fillRoleDataScopeDepartmentIDs(role *model.Role) {
	ids := make([]uint, 0, len(role.DataScopeDepartments))
	for _, relation := range role.DataScopeDepartments {
		ids = append(ids, relation.DepartmentID)
	}
	role.DataScopeDepartmentIDs = ids
}

func reloadRoleDataScopeDepartmentIDs(tx *gorm.DB, role *model.Role) error {
	var relations []model.RoleDataScopeDepartment
	if err := tx.Where("role_id = ?", role.ID).Find(&relations).Error; err != nil {
		return err
	}
	role.DataScopeDepartments = relations
	fillRoleDataScopeDepartmentIDs(role)
	return nil
}

// AssignPermissions 分配权限给角色
func (d *RoleDAO) AssignPermissions(roleID uint, permissionIDs []uint) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		// 先删除原有权限
		if err := tx.Where("role_id = ?", roleID).Delete(&model.RolePermission{}).Error; err != nil {
			return err
		}

		// 添加新权限
		for _, permissionID := range permissionIDs {
			rolePermission := model.RolePermission{
				RoleID:       roleID,
				PermissionID: permissionID,
			}
			if err := tx.Create(&rolePermission).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
