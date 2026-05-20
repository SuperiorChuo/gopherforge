package auth

import (
	"context"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
)

type OAuthBindingDAO struct{}

func (d OAuthBindingDAO) GetByProviderUser(provider, providerUserID string) (*model.OAuthBinding, error) {
	return d.GetByProviderUserContext(context.Background(), provider, providerUserID)
}

func (d OAuthBindingDAO) GetByProviderUserContext(ctx context.Context, provider, providerUserID string) (*model.OAuthBinding, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var binding model.OAuthBinding
	err := database.DB.WithContext(ctx).
		Where("provider = ? AND provider_user_id = ?", provider, providerUserID).
		First(&binding).
		Error
	return &binding, err
}

func (d OAuthBindingDAO) Create(binding *model.OAuthBinding) error {
	return d.CreateContext(context.Background(), binding)
}

func (d OAuthBindingDAO) CreateContext(ctx context.Context, binding *model.OAuthBinding) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return database.DB.WithContext(ctx).Create(binding).Error
}
