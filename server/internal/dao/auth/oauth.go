package auth

import (
	"context"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
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
	if d.db != nil {
		return d.db.WithContext(ctx)
	}
	return database.DB.WithContext(ctx)
}

func (d OAuthBindingDAO) GetByProviderUser(provider, providerUserID string) (*model.OAuthBinding, error) {
	return d.GetByProviderUserContext(context.Background(), provider, providerUserID)
}

func (d OAuthBindingDAO) GetByProviderUserContext(ctx context.Context, provider, providerUserID string) (*model.OAuthBinding, error) {
	var binding model.OAuthBinding
	err := d.dbWithContext(ctx).
		Where("provider = ? AND provider_user_id = ?", provider, providerUserID).
		First(&binding).
		Error
	return &binding, err
}

func (d OAuthBindingDAO) Create(binding *model.OAuthBinding) error {
	return d.CreateContext(context.Background(), binding)
}

func (d OAuthBindingDAO) CreateContext(ctx context.Context, binding *model.OAuthBinding) error {
	return d.dbWithContext(ctx).Create(binding).Error
}
