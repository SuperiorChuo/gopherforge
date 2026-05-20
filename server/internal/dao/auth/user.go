package auth

import (
	"context"

	sharedDAO "github.com/go-admin-kit/server/internal/dao"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
)

// UserDAO keeps auth-specific user queries while reusing shared user persistence methods.
type UserDAO struct {
	sharedDAO.UserDAO
}

func (d *UserDAO) UpdateUserProfile(id uint, updates map[string]any) error {
	return d.UpdateUserProfileContext(context.Background(), id, updates)
}

func (d *UserDAO) UpdateUserProfileContext(ctx context.Context, id uint, updates map[string]any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return database.DB.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).Updates(updates).Error
}

func (d *UserDAO) GetUserByPhone(phone string) (*model.User, error) {
	return d.GetUserByPhoneContext(context.Background(), phone)
}

func (d *UserDAO) GetUserByPhoneContext(ctx context.Context, phone string) (*model.User, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var user model.User
	result := database.DB.WithContext(ctx).Where("phone = ?", phone).First(&user)
	return &user, result.Error
}

func (d *UserDAO) GetUserWithRolesAndPermissions(id uint) (*model.User, error) {
	return d.GetUserWithRolesAndPermissionsContext(context.Background(), id)
}

func (d *UserDAO) GetUserWithRolesAndPermissionsContext(ctx context.Context, id uint) (*model.User, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var user model.User
	result := database.DB.WithContext(ctx).
		Preload("Roles.Permissions").
		First(&user, id)
	return &user, result.Error
}
