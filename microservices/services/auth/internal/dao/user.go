package dao

import (
	"context"

	"github.com/go-admin-kit/services/auth/internal/model"
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
	return d.GetUserByTenantUsernameContext(ctx, 1, username)
}

// GetUserByUsernameLegacyContext looks up by username only (pre-tenant / backfill).
func (d *UserDAO) GetUserByUsernameLegacyContext(ctx context.Context, username string) (*model.User, error) {
	var user model.User
	result := d.dbWithContext(ctx).Where("username = ?", username).First(&user)
	return &user, result.Error
}

// GetUserByTenantUsernameContext looks up a user inside one tenant.
func (d *UserDAO) GetUserByTenantUsernameContext(ctx context.Context, tenantID uint, username string) (*model.User, error) {
	if tenantID == 0 {
		tenantID = 1
	}
	var user model.User
	result := d.dbWithContext(ctx).Where("tenant_id = ? AND username = ?", tenantID, username).First(&user)
	return &user, result.Error
}

// GetTenantByCodeContext returns an enabled tenant by code.
func (d *UserDAO) GetTenantByCodeContext(ctx context.Context, code string) (*model.Tenant, error) {
	if code == "" {
		code = "default"
	}
	var t model.Tenant
	result := d.dbWithContext(ctx).Where("code = ? AND status = 1", code).First(&t)
	return &t, result.Error
}

// GetTenantByIDContext returns tenant by primary key.
func (d *UserDAO) GetTenantByIDContext(ctx context.Context, id uint) (*model.Tenant, error) {
	var t model.Tenant
	result := d.dbWithContext(ctx).First(&t, id)
	return &t, result.Error
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
