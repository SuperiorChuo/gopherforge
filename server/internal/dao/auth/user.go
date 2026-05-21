package auth

import (
	"context"

	sharedDAO "github.com/go-admin-kit/server/internal/dao"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"gorm.io/gorm"
)

// UserDAO keeps auth-specific user queries while reusing shared user persistence methods.
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

// Deprecated: use UpdateUserProfileContext instead.
func (d *UserDAO) UpdateUserProfile(id uint, updates map[string]any) error {
	return d.UpdateUserProfileContext(context.Background(), id, updates)
}

func (d *UserDAO) UpdateUserProfileContext(ctx context.Context, id uint, updates map[string]any) error {
	return d.dbWithContext(ctx).Model(&model.User{}).Where("id = ?", id).Updates(updates).Error
}

// Deprecated: use GetUserByPhoneContext instead.
func (d *UserDAO) GetUserByPhone(phone string) (*model.User, error) {
	return d.GetUserByPhoneContext(context.Background(), phone)
}

func (d *UserDAO) GetUserByPhoneContext(ctx context.Context, phone string) (*model.User, error) {
	var user model.User
	result := d.dbWithContext(ctx).Where("phone = ?", phone).First(&user)
	return &user, result.Error
}

// Deprecated: use GetUserWithRolesAndPermissionsContext instead.
func (d *UserDAO) GetUserWithRolesAndPermissions(id uint) (*model.User, error) {
	return d.GetUserWithRolesAndPermissionsContext(context.Background(), id)
}

func (d *UserDAO) GetUserWithRolesAndPermissionsContext(ctx context.Context, id uint) (*model.User, error) {
	var user model.User
	result := d.dbWithContext(ctx).
		Preload("Roles.Permissions").
		First(&user, id)
	return &user, result.Error
}
