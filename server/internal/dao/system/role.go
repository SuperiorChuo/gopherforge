package system

import (
	"context"

	"gorm.io/gorm"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
)

type RoleDAO struct {
	db *gorm.DB
}

func NewRoleDAO(db *gorm.DB) *RoleDAO {
	return &RoleDAO{db: db}
}

func (d *RoleDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	if d != nil && d.db != nil {
		return d.db.WithContext(ctx)
	}
	return database.DB.WithContext(ctx)
}

// Deprecated: use GetRoleByIDContext instead.
func (d *RoleDAO) GetRoleByID(id uint) (*model.Role, error) {
	return d.GetRoleByIDContext(context.Background(), id)
}

func (d *RoleDAO) GetRoleByIDContext(ctx context.Context, id uint) (*model.Role, error) {
	var role model.Role
	result := d.dbWithContext(ctx).
		Preload("Permissions").
		Preload("DataScopeDepartments").
		First(&role, id)
	if result.Error != nil {
		return &role, result.Error
	}
	fillRoleDataScopeDepartmentIDs(&role)
	return &role, nil
}

// Deprecated: use GetRoleByCodeContext instead.
func (d *RoleDAO) GetRoleByCode(code string) (*model.Role, error) {
	return d.GetRoleByCodeContext(context.Background(), code)
}

func (d *RoleDAO) GetRoleByCodeContext(ctx context.Context, code string) (*model.Role, error) {
	var role model.Role
	result := d.dbWithContext(ctx).Where("code = ?", code).First(&role)
	return &role, result.Error
}

// Deprecated: use GetRoleListContext instead.
func (d *RoleDAO) GetRoleList(req pagination.PageRequest, keyword string) ([]model.Role, int64, error) {
	return d.GetRoleListContext(context.Background(), req, keyword)
}

func (d *RoleDAO) GetRoleListContext(ctx context.Context, req pagination.PageRequest, keyword string) ([]model.Role, int64, error) {
	var roles []model.Role
	var total int64

	query := d.dbWithContext(ctx).Model(&model.Role{})
	if keyword != "" {
		query = query.Where("name LIKE ? OR code LIKE ? OR description LIKE ?",
			"%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	result := query.Scopes(pagination.Paginate(req)).
		Preload("Permissions").
		Preload("DataScopeDepartments").
		Order("created_at DESC").
		Find(&roles)

	fillRolesDataScopeDepartmentIDs(roles)
	return roles, total, result.Error
}

// Deprecated: use GetAllRolesContext instead.
func (d *RoleDAO) GetAllRoles() ([]model.Role, error) {
	return d.GetAllRolesContext(context.Background())
}

func (d *RoleDAO) GetAllRolesContext(ctx context.Context) ([]model.Role, error) {
	var roles []model.Role
	result := d.dbWithContext(ctx).
		Preload("DataScopeDepartments").
		Order("created_at ASC").
		Find(&roles)
	fillRolesDataScopeDepartmentIDs(roles)
	return roles, result.Error
}

// Deprecated: use CreateRoleContext instead.
func (d *RoleDAO) CreateRole(role *model.Role) error {
	return d.CreateRoleContext(context.Background(), role)
}

func (d *RoleDAO) CreateRoleContext(ctx context.Context, role *model.Role) error {
	return d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(role).Error; err != nil {
			return err
		}
		if err := replaceRoleDataScopeDepartments(tx, role.ID, role.DataScope, role.DataScopeDepartmentIDs); err != nil {
			return err
		}
		return reloadRoleDataScopeDepartmentIDs(tx, role)
	})
}

// Deprecated: use UpdateRoleContext instead.
func (d *RoleDAO) UpdateRole(role *model.Role) error {
	return d.UpdateRoleContext(context.Background(), role)
}

func (d *RoleDAO) UpdateRoleContext(ctx context.Context, role *model.Role) error {
	return d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(role).Error; err != nil {
			return err
		}
		if err := replaceRoleDataScopeDepartments(tx, role.ID, role.DataScope, role.DataScopeDepartmentIDs); err != nil {
			return err
		}
		return reloadRoleDataScopeDepartmentIDs(tx, role)
	})
}

// Deprecated: use DeleteRoleContext instead.
func (d *RoleDAO) DeleteRole(id uint) error {
	return d.DeleteRoleContext(context.Background(), id)
}

func (d *RoleDAO) DeleteRoleContext(ctx context.Context, id uint) error {
	return d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("role_id = ?", id).Delete(&model.RolePermission{}).Error; err != nil {
			return err
		}
		if err := tx.Where("role_id = ?", id).Delete(&model.UserRole{}).Error; err != nil {
			return err
		}
		if err := tx.Where("role_id = ?", id).Delete(&model.RoleDataScopeDepartment{}).Error; err != nil {
			return err
		}
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

// Deprecated: use AssignPermissionsContext instead.
func (d *RoleDAO) AssignPermissions(roleID uint, permissionIDs []uint) error {
	return d.AssignPermissionsContext(context.Background(), roleID, permissionIDs)
}

func (d *RoleDAO) AssignPermissionsContext(ctx context.Context, roleID uint, permissionIDs []uint) error {
	return d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("role_id = ?", roleID).Delete(&model.RolePermission{}).Error; err != nil {
			return err
		}

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
