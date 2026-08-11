package dao

import (
	"context"

	localmodel "github.com/go-admin-kit/services/identity/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
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

func (d *UserDAO) GetUserByUsernameContext(ctx context.Context, username string) (*localmodel.User, error) {
	q := d.dbWithContext(ctx).Where("username = ?", username)
	if tid := tenant.FromContext(ctx); tid > 0 {
		q = q.Where("tenant_id = ?", tid)
	}
	var user localmodel.User
	result := q.First(&user)
	return &user, result.Error
}

func (d *UserDAO) GetUserByIDContext(ctx context.Context, id uint) (*localmodel.User, error) {
	var user localmodel.User
	result := d.dbWithContext(ctx).First(&user, id)
	return &user, result.Error
}

func (d *UserDAO) GetUserWithRolesContext(ctx context.Context, id uint) (*localmodel.User, error) {
	var user localmodel.User
	result := d.dbWithContext(ctx).Preload("Roles").First(&user, id)
	return &user, result.Error
}

func (d *UserDAO) GetUserByEmailContext(ctx context.Context, email string) (*localmodel.User, error) {
	q := d.dbWithContext(ctx).Where("email = ?", email)
	if tid := tenant.FromContext(ctx); tid > 0 {
		q = q.Where("tenant_id = ?", tid)
	}
	var user localmodel.User
	result := q.First(&user)
	return &user, result.Error
}

func (d *UserDAO) CreateUserContext(ctx context.Context, user *localmodel.User) error {
	return d.dbWithContext(ctx).Create(user).Error
}

func (d *UserDAO) UpdateUserContext(ctx context.Context, user *localmodel.User) error {
	return d.dbWithContext(ctx).Save(user).Error
}
