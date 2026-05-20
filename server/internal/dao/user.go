package dao

import (
	"context"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
)

type UserDAO struct{}

func (d *UserDAO) GetUserByUsername(username string) (*model.User, error) {
	return d.GetUserByUsernameContext(context.Background(), username)
}

func (d *UserDAO) GetUserByUsernameContext(ctx context.Context, username string) (*model.User, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var user model.User
	result := database.DB.WithContext(ctx).Where("username = ?", username).First(&user)
	return &user, result.Error
}

func (d *UserDAO) GetUserByID(id uint) (*model.User, error) {
	return d.GetUserByIDContext(context.Background(), id)
}

func (d *UserDAO) GetUserByIDContext(ctx context.Context, id uint) (*model.User, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var user model.User
	result := database.DB.WithContext(ctx).First(&user, id)
	return &user, result.Error
}

func (d *UserDAO) GetUserWithRoles(id uint) (*model.User, error) {
	return d.GetUserWithRolesContext(context.Background(), id)
}

func (d *UserDAO) GetUserWithRolesContext(ctx context.Context, id uint) (*model.User, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var user model.User
	result := database.DB.WithContext(ctx).Preload("Roles").First(&user, id)
	return &user, result.Error
}

func (d *UserDAO) GetUserByEmail(email string) (*model.User, error) {
	return d.GetUserByEmailContext(context.Background(), email)
}

func (d *UserDAO) GetUserByEmailContext(ctx context.Context, email string) (*model.User, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var user model.User
	result := database.DB.WithContext(ctx).Where("email = ?", email).First(&user)
	return &user, result.Error
}

func (d *UserDAO) CreateUser(user *model.User) error {
	return d.CreateUserContext(context.Background(), user)
}

func (d *UserDAO) CreateUserContext(ctx context.Context, user *model.User) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return database.DB.WithContext(ctx).Create(user).Error
}

func (d *UserDAO) UpdateUser(user *model.User) error {
	return d.UpdateUserContext(context.Background(), user)
}

func (d *UserDAO) UpdateUserContext(ctx context.Context, user *model.User) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return database.DB.WithContext(ctx).Save(user).Error
}
