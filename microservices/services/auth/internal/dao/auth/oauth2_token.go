package auth

import (
	"context"
	"time"

	"github.com/go-admin-kit/services/auth/internal/model"
	"gorm.io/gorm"
)

// OAuth2TokenDAO persists opaque access/refresh tokens (SHA-256 hashes).
type OAuth2TokenDAO struct {
	db *gorm.DB
}

func NewOAuth2TokenDAO(db *gorm.DB) OAuth2TokenDAO {
	return OAuth2TokenDAO{db: db}
}

func (d OAuth2TokenDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

func (d OAuth2TokenDAO) CreateAccessContext(ctx context.Context, token *model.OAuth2AccessToken) error {
	return d.dbWithContext(ctx).Create(token).Error
}

func (d OAuth2TokenDAO) CreateRefreshContext(ctx context.Context, token *model.OAuth2RefreshToken) error {
	return d.dbWithContext(ctx).Create(token).Error
}

// GetAccessByHashContext looks up an access token globally by hash (resource
// servers calling introspect/userinfo carry no tenant context).
func (d OAuth2TokenDAO) GetAccessByHashContext(ctx context.Context, hash string) (*model.OAuth2AccessToken, error) {
	var token model.OAuth2AccessToken
	err := d.dbWithContext(ctx).Where("token_hash = ?", hash).First(&token).Error
	if err != nil {
		return nil, err
	}
	return &token, nil
}

func (d OAuth2TokenDAO) GetRefreshByHashContext(ctx context.Context, hash string) (*model.OAuth2RefreshToken, error) {
	var token model.OAuth2RefreshToken
	err := d.dbWithContext(ctx).Where("token_hash = ?", hash).First(&token).Error
	if err != nil {
		return nil, err
	}
	return &token, nil
}

// RevokeAccessByHashContext marks a single access token revoked. Returns rows affected.
func (d OAuth2TokenDAO) RevokeAccessByHashContext(ctx context.Context, hash string) (int64, error) {
	now := time.Now()
	result := d.dbWithContext(ctx).Model(&model.OAuth2AccessToken{}).
		Where("token_hash = ? AND revoked_at IS NULL", hash).
		Update("revoked_at", now)
	return result.RowsAffected, result.Error
}

func (d OAuth2TokenDAO) RevokeRefreshByHashContext(ctx context.Context, hash string) (int64, error) {
	now := time.Now()
	result := d.dbWithContext(ctx).Model(&model.OAuth2RefreshToken{}).
		Where("token_hash = ? AND revoked_at IS NULL", hash).
		Update("revoked_at", now)
	return result.RowsAffected, result.Error
}

// RevokeAccessByRefreshTokenIDContext cascades: revoking a refresh token also
// revokes the access token it minted (refresh rotation / logout).
func (d OAuth2TokenDAO) RevokeAccessByRefreshTokenIDContext(ctx context.Context, refreshID uint) error {
	now := time.Now()
	return d.dbWithContext(ctx).Model(&model.OAuth2AccessToken{}).
		Where("refresh_token_id = ? AND revoked_at IS NULL", refreshID).
		Update("revoked_at", now).Error
}

// RevokeAccessByIDContext revokes one access token by id, tenant-scoped (management API).
func (d OAuth2TokenDAO) RevokeAccessByIDContext(ctx context.Context, id uint) (int64, error) {
	now := time.Now()
	result := d.dbWithContext(ctx).Model(&model.OAuth2AccessToken{}).
		Where("id = ? AND tenant_id = ? AND revoked_at IS NULL", id, tenantFromCtx(ctx)).
		Update("revoked_at", now)
	return result.RowsAffected, result.Error
}

// RevokeAllByClientUserContext revokes every live token pair for a (client, user)
// combination. userID=nil revokes client_credentials tokens (no user).
func (d OAuth2TokenDAO) RevokeAllByClientUserContext(ctx context.Context, clientID string, userID *uint) error {
	now := time.Now()
	db := d.dbWithContext(ctx)
	access := db.Model(&model.OAuth2AccessToken{}).Where("client_id = ? AND revoked_at IS NULL", clientID)
	refresh := db.Model(&model.OAuth2RefreshToken{}).Where("client_id = ? AND revoked_at IS NULL", clientID)
	if userID != nil {
		access = access.Where("user_id = ?", *userID)
		refresh = refresh.Where("user_id = ?", *userID)
	}
	if err := access.Update("revoked_at", now).Error; err != nil {
		return err
	}
	return refresh.Update("revoked_at", now).Error
}

// RevokeAllByClientContext revokes every live token for a client (client disabled/deleted).
func (d OAuth2TokenDAO) RevokeAllByClientContext(ctx context.Context, clientID string) error {
	now := time.Now()
	db := d.dbWithContext(ctx)
	if err := db.Model(&model.OAuth2AccessToken{}).
		Where("client_id = ? AND revoked_at IS NULL", clientID).Update("revoked_at", now).Error; err != nil {
		return err
	}
	return db.Model(&model.OAuth2RefreshToken{}).
		Where("client_id = ? AND revoked_at IS NULL", clientID).Update("revoked_at", now).Error
}

// ListAccessContext returns tenant-scoped access tokens for the management view.
func (d OAuth2TokenDAO) ListAccessContext(ctx context.Context, clientID string, page, pageSize int) ([]model.OAuth2AccessToken, int64, error) {
	query := d.dbWithContext(ctx).Model(&model.OAuth2AccessToken{}).Where("tenant_id = ?", tenantFromCtx(ctx))
	if clientID != "" {
		query = query.Where("client_id = ?", clientID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var tokens []model.OAuth2AccessToken
	err := query.Order("id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&tokens).Error
	return tokens, total, err
}
