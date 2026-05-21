package dao

import (
	"context"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
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
	if d != nil && d.db != nil {
		return d.db.WithContext(ctx)
	}
	return database.DB.WithContext(ctx)
}

// Deprecated: use GetUserByUsernameContext instead.
func (d *UserDAO) GetUserByUsername(username string) (*model.User, error) {
	return d.GetUserByUsernameContext(context.Background(), username)
}

func (d *UserDAO) GetUserByUsernameContext(ctx context.Context, username string) (*model.User, error) {
	var user model.User
	result := d.dbWithContext(ctx).Where("username = ?", username).First(&user)
	return &user, result.Error
}

// Deprecated: use GetUserByIDContext instead.
func (d *UserDAO) GetUserByID(id uint) (*model.User, error) {
	return d.GetUserByIDContext(context.Background(), id)
}

func (d *UserDAO) GetUserByIDContext(ctx context.Context, id uint) (*model.User, error) {
	var user model.User
	result := d.dbWithContext(ctx).First(&user, id)
	return &user, result.Error
}

// Deprecated: use GetUserWithRolesContext instead.
func (d *UserDAO) GetUserWithRoles(id uint) (*model.User, error) {
	return d.GetUserWithRolesContext(context.Background(), id)
}

func (d *UserDAO) GetUserWithRolesContext(ctx context.Context, id uint) (*model.User, error) {
	var user model.User
	result := d.dbWithContext(ctx).Preload("Roles").First(&user, id)
	return &user, result.Error
}

// Deprecated: use GetUserByEmailContext instead.
func (d *UserDAO) GetUserByEmail(email string) (*model.User, error) {
	return d.GetUserByEmailContext(context.Background(), email)
}

func (d *UserDAO) GetUserByEmailContext(ctx context.Context, email string) (*model.User, error) {
	var user model.User
	result := d.dbWithContext(ctx).Where("email = ?", email).First(&user)
	return &user, result.Error
}

// Deprecated: use CreateUserContext instead.
func (d *UserDAO) CreateUser(user *model.User) error {
	return d.CreateUserContext(context.Background(), user)
}

func (d *UserDAO) CreateUserContext(ctx context.Context, user *model.User) error {
	return d.dbWithContext(ctx).Create(user).Error
}

// Deprecated: use UpdateUserContext instead.
func (d *UserDAO) UpdateUser(user *model.User) error {
	return d.UpdateUserContext(context.Background(), user)
}

func (d *UserDAO) UpdateUserContext(ctx context.Context, user *model.User) error {
	return d.dbWithContext(ctx).Save(user).Error
}
