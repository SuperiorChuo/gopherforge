package system

import (
	"context"
	"fmt"

	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"

	"github.com/go-admin-kit/services/identity/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/audittrail"
	"github.com/go-admin-kit/services/shared/pkg/pagination"
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
	return d.db.WithContext(ctx)
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

func (d *RoleDAO) GetRoleByCodeContext(ctx context.Context, code string) (*model.Role, error) {
	var role model.Role
	q := d.dbWithContext(ctx).Where("code = ?", code)
	if tid := tenant.FromContext(ctx); tid > 0 {
		q = q.Where("tenant_id = ?", tid)
	}
	result := q.First(&role)
	return &role, result.Error
}

func (d *RoleDAO) GetRoleListContext(ctx context.Context, req pagination.PageRequest, keyword string) ([]model.Role, int64, error) {
	var roles []model.Role
	var total int64

	query := d.dbWithContext(ctx).Model(&model.Role{})
	if tid := tenant.FromContext(ctx); tid > 0 {
		query = query.Where("roles.tenant_id = ?", tid)
	}
	if keyword != "" {
		query = query.Where("name LIKE ? OR code LIKE ? OR description LIKE ?",
			"%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 列表不预加载 Permissions：列表页只展示角色本身，权限集在详情
	// （GetRoleByIDContext）与分配弹窗按需拉取。50 角色 × 96 权限时列表
	// 会背 4800 行关联数据，前端对列表行的 permissions 字段零消费。
	result := query.Scopes(pagination.Paginate(req)).
		Preload("DataScopeDepartments").
		Order("created_at DESC").
		Find(&roles)

	fillRolesDataScopeDepartmentIDs(roles)
	return roles, total, result.Error
}

func (d *RoleDAO) GetAllRolesContext(ctx context.Context) ([]model.Role, error) {
	var roles []model.Role
	q := d.dbWithContext(ctx).Model(&model.Role{})
	if tid := tenant.FromContext(ctx); tid > 0 {
		q = q.Where("roles.tenant_id = ?", tid)
	}
	result := q.
		Preload("DataScopeDepartments").
		Order("created_at ASC").
		Find(&roles)
	fillRolesDataScopeDepartmentIDs(roles)
	return roles, result.Error
}

func (d *RoleDAO) CreateRoleContext(ctx context.Context, role *model.Role) error {
	return d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(role).Error; err != nil {
			return err
		}
		if err := replaceRoleDataScopeDepartments(ctx, tx, role.ID, role.DataScope, role.DataScopeDepartmentIDs); err != nil {
			return err
		}
		return reloadRoleDataScopeDepartmentIDs(tx, role)
	})
}

func (d *RoleDAO) UpdateRoleContext(ctx context.Context, role *model.Role) error {
	return d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(role).Error; err != nil {
			return err
		}
		if err := replaceRoleDataScopeDepartments(ctx, tx, role.ID, role.DataScope, role.DataScopeDepartmentIDs); err != nil {
			return err
		}
		return reloadRoleDataScopeDepartmentIDs(tx, role)
	})
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

func replaceRoleDataScopeDepartments(ctx context.Context, tx *gorm.DB, roleID uint, dataScope string, departmentIDs []uint) error {
	before := make([]uint, 0)
	if err := tx.Model(&model.RoleDataScopeDepartment{}).Where("role_id = ?", roleID).Pluck("department_id", &before).Error; err != nil {
		return err
	}

	if err := tx.Where("role_id = ?", roleID).Delete(&model.RoleDataScopeDepartment{}).Error; err != nil {
		return err
	}

	after := make([]uint, 0)
	relations := make([]model.RoleDataScopeDepartment, 0, len(departmentIDs))
	if dataScope == "custom" {
		seen := make(map[uint]struct{}, len(departmentIDs))
		for _, departmentID := range departmentIDs {
			if departmentID == 0 {
				continue
			}
			if _, ok := seen[departmentID]; ok {
				continue
			}
			seen[departmentID] = struct{}{}
			after = append(after, departmentID)
			relations = append(relations, model.RoleDataScopeDepartment{
				RoleID:       roleID,
				DepartmentID: departmentID,
			})
		}
	}
	if len(after) > 0 {
		if err := tx.Create(&relations).Error; err != nil {
			return err
		}
	}
	return audittrail.RecordAssociation(ctx, tx, audittrail.RecordAssociationRequest{
		TargetType: "role_data_scope_departments",
		TargetID:   fmt.Sprint(roleID),
		Action:     "update",
		Before:     map[string]any{"department_ids": append([]uint{}, before...)},
		After:      map[string]any{"department_ids": append([]uint{}, after...)},
		Summary:    fmt.Sprintf("update role %d data scope departments", roleID),
	})
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

func (d *RoleDAO) AssignPermissionsContext(ctx context.Context, roleID uint, permissionIDs []uint) error {
	return d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		before := make([]uint, 0)
		if err := tx.Model(&model.RolePermission{}).Where("role_id = ?", roleID).Pluck("permission_id", &before).Error; err != nil {
			return err
		}

		if err := tx.Where("role_id = ?", roleID).Delete(&model.RolePermission{}).Error; err != nil {
			return err
		}

		if len(permissionIDs) > 0 {
			rolePermissions := make([]model.RolePermission, 0, len(permissionIDs))
			for _, permissionID := range permissionIDs {
				rolePermissions = append(rolePermissions, model.RolePermission{
					RoleID:       roleID,
					PermissionID: permissionID,
				})
			}
			if err := tx.Create(&rolePermissions).Error; err != nil {
				return err
			}
		}
		return audittrail.RecordAssociation(ctx, tx, audittrail.RecordAssociationRequest{
			TargetType: "role_permissions",
			TargetID:   fmt.Sprint(roleID),
			Action:     "update",
			Before:     map[string]any{"permission_ids": append([]uint{}, before...)},
			After:      map[string]any{"permission_ids": append([]uint{}, permissionIDs...)},
			Summary:    fmt.Sprintf("update role %d permissions", roleID),
		})
	})
}
