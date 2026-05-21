package system

import (
	"context"

	"gorm.io/gorm"

	sharedDAO "github.com/go-admin-kit/server/internal/dao"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
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
	if d != nil && d.db != nil {
		return d.db.WithContext(ctx)
	}
	return database.DB.WithContext(ctx)
}

// Deprecated: use GetUserListContext instead.
func (d *UserDAO) GetUserList(req pagination.PageRequest, keyword string, status *int8, dataScope authz.UserDataScope) ([]model.User, int64, error) {
	return d.GetUserListContext(context.Background(), req, keyword, status, dataScope)
}

func (d *UserDAO) GetUserListContext(ctx context.Context, req pagination.PageRequest, keyword string, status *int8, dataScope authz.UserDataScope) ([]model.User, int64, error) {
	var users []model.User
	var total int64

	query := d.dbWithContext(ctx).Model(&model.User{})
	query = authz.ApplyUserEntityScope(query, dataScope, "id", "department_id")

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

// Deprecated: use DeleteUserContext instead.
func (d *UserDAO) DeleteUser(id uint) error {
	return d.DeleteUserContext(context.Background(), id)
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

// Deprecated: use UpdateUserStatusContext instead.
func (d *UserDAO) UpdateUserStatus(id uint, status int8) error {
	return d.UpdateUserStatusContext(context.Background(), id, status)
}

func (d *UserDAO) UpdateUserStatusContext(ctx context.Context, id uint, status int8) error {
	return d.dbWithContext(ctx).Model(&model.User{}).Where("id = ?", id).Update("status", status).Error
}

// Deprecated: use AssignRolesContext instead.
func (d *UserDAO) AssignRoles(userID uint, roleIDs []uint) error {
	return d.AssignRolesContext(context.Background(), userID, roleIDs)
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
