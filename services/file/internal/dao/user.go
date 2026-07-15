package dao

import (
	"context"

	"github.com/go-admin-kit/services/file/internal/model"
	"gorm.io/gorm"
)

type UserDAO struct {
	db *gorm.DB
}

func NewUserDAO(db *gorm.DB) *UserDAO {
	return &UserDAO{db: db}
}

func (d *UserDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

func (d *UserDAO) GetUserByUsernameContext(ctx context.Context, username string) (*model.User, error) {
	var user model.User
	result := d.dbWithContext(ctx).Where("username = ?", username).First(&user)
	return &user, result.Error
}

func (d *UserDAO) GetUserByIDContext(ctx context.Context, id uint) (*model.User, error) {
	var user model.User
	result := d.dbWithContext(ctx).First(&user, id)
	return &user, result.Error
}

func (d *UserDAO) GetUserWithRolesContext(ctx context.Context, id uint) (*model.User, error) {
	var user model.User
	result := d.dbWithContext(ctx).Preload("Roles").First(&user, id)
	return &user, result.Error
}

func (d *UserDAO) GetUserByEmailContext(ctx context.Context, email string) (*model.User, error) {
	var user model.User
	result := d.dbWithContext(ctx).Where("email = ?", email).First(&user)
	return &user, result.Error
}

func (d *UserDAO) CreateUserContext(ctx context.Context, user *model.User) error {
	return d.dbWithContext(ctx).Create(user).Error
}

func (d *UserDAO) UpdateUserContext(ctx context.Context, user *model.User) error {
	return d.dbWithContext(ctx).Save(user).Error
}
