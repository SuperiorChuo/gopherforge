package system

import (
	"context"
	"strings"

	"github.com/go-admin-kit/services/identity/internal/model"
	"github.com/go-admin-kit/services/identity/internal/pkg/pagination"
	"gorm.io/gorm"
)

// TenantPackageDAO 持久化租户套餐（权限包），平台级目录表。
type TenantPackageDAO struct {
	db *gorm.DB
}

func NewTenantPackageDAO(db *gorm.DB) *TenantPackageDAO {
	return &TenantPackageDAO{db: db}
}

func (d *TenantPackageDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

func (d *TenantPackageDAO) ListContext(ctx context.Context, req pagination.PageRequest, keyword string, status *int8) ([]model.TenantPackage, int64, error) {
	q := d.dbWithContext(ctx).Model(&model.TenantPackage{})
	if keyword != "" {
		q = q.Where("name ILIKE ?", "%"+strings.TrimSpace(keyword)+"%")
	}
	if status != nil {
		q = q.Where("status = ?", *status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.TenantPackage
	err := q.Scopes(pagination.Paginate(req)).Order("id ASC").Find(&list).Error
	return list, total, err
}

// GetAllContext 返回全部套餐（含停用，供下拉与回显；前端按 status 标注）。
func (d *TenantPackageDAO) GetAllContext(ctx context.Context) ([]model.TenantPackage, error) {
	var list []model.TenantPackage
	err := d.dbWithContext(ctx).Order("id ASC").Find(&list).Error
	return list, err
}

func (d *TenantPackageDAO) GetByIDContext(ctx context.Context, id uint) (*model.TenantPackage, error) {
	var p model.TenantPackage
	err := d.dbWithContext(ctx).First(&p, id).Error
	return &p, err
}

func (d *TenantPackageDAO) GetByNameContext(ctx context.Context, name string) (*model.TenantPackage, error) {
	var p model.TenantPackage
	err := d.dbWithContext(ctx).Where("name = ?", strings.TrimSpace(name)).First(&p).Error
	return &p, err
}

func (d *TenantPackageDAO) CreateContext(ctx context.Context, p *model.TenantPackage) error {
	return d.dbWithContext(ctx).Create(p).Error
}

func (d *TenantPackageDAO) UpdateContext(ctx context.Context, p *model.TenantPackage) error {
	return d.dbWithContext(ctx).Save(p).Error
}

func (d *TenantPackageDAO) DeleteContext(ctx context.Context, id uint) error {
	return d.dbWithContext(ctx).Delete(&model.TenantPackage{}, id).Error
}

// CountTenantsByPackageContext 统计绑定该套餐的租户数（删除守卫用）。
func (d *TenantPackageDAO) CountTenantsByPackageContext(ctx context.Context, packageID uint) (int64, error) {
	var n int64
	err := d.dbWithContext(ctx).Model(&model.Tenant{}).Where("package_id = ?", packageID).Count(&n).Error
	return n, err
}
