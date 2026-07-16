package system

import (
	"context"
	"errors"

	"gorm.io/gorm"

	sharedDAO "github.com/go-admin-kit/services/identity/internal/dao"
	"github.com/go-admin-kit/services/identity/internal/model"
	"github.com/go-admin-kit/services/identity/internal/pkg/authz"
	"github.com/go-admin-kit/services/identity/internal/pkg/pagination"
)

// UserDAO keeps system-management user queries while reusing shared user persistence methods.
type UserDAO struct {
	sharedDAO.UserDAO
	db *gorm.DB
}

func NewUserDAO(db *gorm.DB) *UserDAO {
	shared := sharedDAO.NewUserDAO(db)
	return &UserDAO{
		UserDAO: *shared,
		db:      db,
	}
}

func (d *UserDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

func (d *UserDAO) GetUserListContext(ctx context.Context, req pagination.PageRequest, keyword string, status *int8, dataScope authz.UserDataScope) ([]model.User, int64, error) {
	var users []model.User
	var total int64

	query := d.dbWithContext(authz.EnableDataScope(ctx, dataScope)).Model(&model.User{})

	// Multi-tenant isolation (explicit; GORM tenant plugin also applies when registered).
	if tid, ok := ctx.Value("tenant_id").(uint); ok && tid > 0 {
		query = query.Where("users.tenant_id = ?", tid)
	}

	if keyword != "" {
		query = query.Where("username LIKE ? OR nickname LIKE ? OR email LIKE ? OR phone LIKE ?",
			"%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}

	if status != nil {
		query = query.Where("status = ?", *status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	result := query.Scopes(pagination.Paginate(req)).
		Preload("Roles").
		Order("created_at DESC").
		Find(&users)

	return users, total, result.Error
}

func (d *UserDAO) DeleteUserContext(ctx context.Context, id uint) error {
	return d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", id).Delete(&model.UserRole{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&model.User{}, id).Error; err != nil {
			return err
		}
		return nil
	})
}

func (d *UserDAO) UpdateUserStatusContext(ctx context.Context, id uint, status int8) error {
	return d.dbWithContext(ctx).Model(&model.User{}).Where("id = ?", id).Update("status", status).Error
}

func (d *UserDAO) AssignRolesContext(ctx context.Context, userID uint, roleIDs []uint) error {
	return d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&model.UserRole{}).Error; err != nil {
			return err
		}

		if len(roleIDs) == 0 {
			return nil
		}

		userRoles := make([]model.UserRole, 0, len(roleIDs))
		for _, roleID := range roleIDs {
			userRoles = append(userRoles, model.UserRole{
				UserID: userID,
				RoleID: roleID,
			})
		}

		if err := tx.Create(&userRoles).Error; err != nil {
			return err
		}
		return nil
	})
}

// AssertRolesInTenantContext ensures every role id belongs to tenantID.
func (d *UserDAO) AssertRolesInTenantContext(ctx context.Context, roleIDs []uint, tenantID uint) error {
	if len(roleIDs) == 0 {
		return nil
	}
	if tenantID == 0 {
		tenantID = 1
	}
	seen := make(map[uint]struct{}, len(roleIDs))
	uniq := make([]uint, 0, len(roleIDs))
	for _, id := range roleIDs {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	if len(uniq) == 0 {
		return nil
	}
	var count int64
	if err := d.dbWithContext(ctx).Model(&model.Role{}).
		Where("tenant_id = ? AND id IN ?", tenantID, uniq).
		Count(&count).Error; err != nil {
		return err
	}
	if int(count) != len(uniq) {
		return ErrRoleNotInTenant
	}
	return nil
}

// AssertDepartmentInTenantContext ensures department belongs to tenant.
func (d *UserDAO) AssertDepartmentInTenantContext(ctx context.Context, departmentID, tenantID uint) error {
	if departmentID == 0 {
		return nil
	}
	if tenantID == 0 {
		tenantID = 1
	}
	var count int64
	if err := d.dbWithContext(ctx).Model(&model.Department{}).
		Where("id = ? AND tenant_id = ?", departmentID, tenantID).
		Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return ErrDepartmentNotInTenant
	}
	return nil
}

// Tenant boundary errors (M2).
var (
	ErrRoleNotInTenant       = errors.New("role does not belong to current tenant")
	ErrDepartmentNotInTenant = errors.New("department does not belong to current tenant")
)
