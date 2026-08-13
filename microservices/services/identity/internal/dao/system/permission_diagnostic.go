package system

import (
	"context"

	localmodel "github.com/go-admin-kit/services/identity/internal/model"
	model "github.com/go-admin-kit/services/shared/pkg/model"
	"gorm.io/gorm"
)

// PermissionDiagnosticDAO loads the complete RBAC chain used by permission diagnostics.
type PermissionDiagnosticDAO struct {
	db *gorm.DB
}

func NewPermissionDiagnosticDAO(db *gorm.DB) *PermissionDiagnosticDAO {
	return &PermissionDiagnosticDAO{db: db}
}

func (d *PermissionDiagnosticDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

func (d *PermissionDiagnosticDAO) GetUserContext(ctx context.Context, id uint) (*localmodel.User, error) {
	var user localmodel.User
	err := d.dbWithContext(ctx).First(&user, id).Error
	return &user, err
}

func (d *PermissionDiagnosticDAO) GetRolesContext(ctx context.Context, userID uint) ([]model.Role, error) {
	var roles []model.Role
	err := d.dbWithContext(ctx).
		Model(&model.Role{}).
		Joins("JOIN user_roles ON user_roles.role_id = roles.id").
		Where("user_roles.user_id = ?", userID).
		Preload("DataScopeDepartments").
		Order("roles.id ASC").
		Find(&roles).Error
	if err != nil || len(roles) == 0 {
		return roles, err
	}

	roleByID := make(map[uint]*model.Role, len(roles))
	roleIDs := make([]uint, 0, len(roles))
	for i := range roles {
		roleByID[roles[i].ID] = &roles[i]
		roleIDs = append(roleIDs, roles[i].ID)
	}
	type rolePermissionRow struct {
		RoleID uint
		model.Permission
	}
	var rows []rolePermissionRow
	err = d.dbWithContext(ctx).
		Table("role_permissions rp").
		Select("rp.role_id, permissions.*").
		Joins("JOIN permissions ON permissions.id = rp.permission_id").
		Where("rp.role_id IN ?", roleIDs).
		Order("rp.role_id ASC, permissions.id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for i := range rows {
		roleByID[rows[i].RoleID].Permissions = append(roleByID[rows[i].RoleID].Permissions, rows[i].Permission)
	}
	return roles, nil
}

func (d *PermissionDiagnosticDAO) GetTenantContext(ctx context.Context, id uint) (*localmodel.Tenant, error) {
	var target localmodel.Tenant
	err := d.dbWithContext(ctx).First(&target, id).Error
	return &target, err
}

func (d *PermissionDiagnosticDAO) GetTenantPackageContext(ctx context.Context, id uint) (*localmodel.TenantPackage, error) {
	var target localmodel.TenantPackage
	err := d.dbWithContext(ctx).First(&target, id).Error
	return &target, err
}
