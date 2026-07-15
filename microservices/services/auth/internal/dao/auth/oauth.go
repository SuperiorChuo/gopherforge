package auth

import (
	"context"

	"github.com/go-admin-kit/services/auth/internal/model"
	"gorm.io/gorm"
)

type OAuthBindingDAO struct {
	db *gorm.DB
}

func NewOAuthBindingDAO(db *gorm.DB) OAuthBindingDAO {
	return OAuthBindingDAO{db: db}
}

func (d OAuthBindingDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

func (d OAuthBindingDAO) GetByProviderUserContext(ctx context.Context, provider, providerUserID string) (*model.OAuthBinding, error) {
	var binding model.OAuthBinding
	err := d.dbWithContext(ctx).
		Where("provider = ? AND provider_user_id = ?", provider, providerUserID).
		First(&binding).
		Error
	return &binding, err
}

func (d OAuthBindingDAO) GetByUserProviderContext(ctx context.Context, userID uint, provider string) (*model.OAuthBinding, error) {
	var binding model.OAuthBinding
	err := d.dbWithContext(ctx).
		Where("user_id = ? AND provider = ?", userID, provider).
		First(&binding).
		Error
	return &binding, err
}

func (d OAuthBindingDAO) CreateContext(ctx context.Context, binding *model.OAuthBinding) error {
	return d.dbWithContext(ctx).Create(binding).Error
}

func (d OAuthBindingDAO) DeleteByUserProviderContext(ctx context.Context, userID uint, provider string) (int64, error) {
	result := d.dbWithContext(ctx).
		Where("user_id = ? AND provider = ?", userID, provider).
		Delete(&model.OAuthBinding{})
	return result.RowsAffected, result.Error
}
