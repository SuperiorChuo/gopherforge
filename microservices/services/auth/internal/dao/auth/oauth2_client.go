package auth

import (
	"context"

	"github.com/go-admin-kit/services/auth/internal/middleware"
	"github.com/go-admin-kit/services/auth/internal/model"
	"gorm.io/gorm"
)

// tenantFromCtx reads the authenticated tenant id from the request context,
// normalizing the empty/zero case to the default tenant (1). auth-service has
// no GORM tenant plugin, so tenant scoping is applied explicitly per query.
func tenantFromCtx(ctx context.Context) uint {
	if ctx == nil {
		return 1
	}
	if v := ctx.Value(middleware.TenantIDContextKey); v != nil {
		if id, ok := v.(uint); ok && id > 0 {
			return id
		}
	}
	return 1
}

// OAuth2ClientDAO persists registered OAuth2 client applications.
type OAuth2ClientDAO struct {
	db *gorm.DB
}

func NewOAuth2ClientDAO(db *gorm.DB) OAuth2ClientDAO {
	return OAuth2ClientDAO{db: db}
}

func (d OAuth2ClientDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

func (d OAuth2ClientDAO) CreateContext(ctx context.Context, client *model.OAuth2Client) error {
	return d.dbWithContext(ctx).Create(client).Error
}

func (d OAuth2ClientDAO) UpdateContext(ctx context.Context, client *model.OAuth2Client) error {
	return d.dbWithContext(ctx).Save(client).Error
}

// GetByClientIDContext looks up a client globally by client_id, WITHOUT tenant
// scoping. This is the single deliberate exception: the token endpoint has no
// tenant context and must resolve the tenant from the client row itself.
func (d OAuth2ClientDAO) GetByClientIDContext(ctx context.Context, clientID string) (*model.OAuth2Client, error) {
	var client model.OAuth2Client
	err := d.dbWithContext(ctx).Where("client_id = ?", clientID).First(&client).Error
	if err != nil {
		return nil, err
	}
	return &client, nil
}

// GetByIDContext fetches a client scoped to the caller's tenant (management API).
func (d OAuth2ClientDAO) GetByIDContext(ctx context.Context, id uint) (*model.OAuth2Client, error) {
	var client model.OAuth2Client
	err := d.dbWithContext(ctx).
		Where("id = ? AND tenant_id = ?", id, tenantFromCtx(ctx)).
		First(&client).Error
	if err != nil {
		return nil, err
	}
	return &client, nil
}

// ListContext returns tenant-scoped clients with optional keyword filter.
func (d OAuth2ClientDAO) ListContext(ctx context.Context, keyword string, page, pageSize int) ([]model.OAuth2Client, int64, error) {
	query := d.dbWithContext(ctx).Model(&model.OAuth2Client{}).Where("tenant_id = ?", tenantFromCtx(ctx))
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("name LIKE ? OR client_id LIKE ?", like, like)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var clients []model.OAuth2Client
	err := query.Order("id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&clients).Error
	return clients, total, err
}

func (d OAuth2ClientDAO) DeleteContext(ctx context.Context, id uint) (int64, error) {
	result := d.dbWithContext(ctx).
		Where("id = ? AND tenant_id = ?", id, tenantFromCtx(ctx)).
		Delete(&model.OAuth2Client{})
	return result.RowsAffected, result.Error
}
