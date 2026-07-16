package system

import (
	"context"
	"strings"

	"github.com/go-admin-kit/services/identity/internal/model"
	"github.com/go-admin-kit/services/identity/internal/pkg/pagination"
	"gorm.io/gorm"
)

// TenantDAO persists SaaS tenants.
type TenantDAO struct {
	db *gorm.DB
}

func NewTenantDAO(db *gorm.DB) *TenantDAO {
	return &TenantDAO{db: db}
}

func (d *TenantDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

func (d *TenantDAO) ListContext(ctx context.Context, req pagination.PageRequest, keyword string, status *int8) ([]model.Tenant, int64, error) {
	q := d.dbWithContext(ctx).Model(&model.Tenant{})
	if keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("code ILIKE ? OR name ILIKE ?", like, like)
	}
	if status != nil {
		q = q.Where("status = ?", *status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.Tenant
	err := q.Scopes(pagination.Paginate(req)).Order("id ASC").Find(&list).Error
	return list, total, err
}

func (d *TenantDAO) GetByIDContext(ctx context.Context, id uint) (*model.Tenant, error) {
	var t model.Tenant
	err := d.dbWithContext(ctx).First(&t, id).Error
	return &t, err
}

func (d *TenantDAO) GetByCodeContext(ctx context.Context, code string) (*model.Tenant, error) {
	var t model.Tenant
	err := d.dbWithContext(ctx).Where("code = ?", strings.TrimSpace(code)).First(&t).Error
	return &t, err
}

func (d *TenantDAO) CreateContext(ctx context.Context, t *model.Tenant) error {
	return d.dbWithContext(ctx).Create(t).Error
}

func (d *TenantDAO) UpdateContext(ctx context.Context, t *model.Tenant) error {
	return d.dbWithContext(ctx).Save(t).Error
}

func (d *TenantDAO) CountUsersContext(ctx context.Context, tenantID uint) (int64, error) {
	var n int64
	err := d.dbWithContext(ctx).Model(&model.User{}).Where("tenant_id = ?", tenantID).Count(&n).Error
	return n, err
}
