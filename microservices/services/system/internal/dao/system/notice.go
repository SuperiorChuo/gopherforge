package system

import (
	"context"

	"github.com/go-admin-kit/services/system/internal/model"
	"github.com/go-admin-kit/services/system/internal/pkg/pagination"
	"github.com/go-admin-kit/services/system/internal/pkg/tenant"
	"gorm.io/gorm"
)

type NoticeDAO struct {
	db *gorm.DB
}

func NewNoticeDAO(db *gorm.DB) *NoticeDAO {
	return &NoticeDAO{db: db}
}

func (d *NoticeDAO) GetByIDContext(ctx context.Context, id uint) (*model.Notice, error) {
	var notice model.Notice
	result := d.dbWithContext(ctx).
		Where("tenant_id = ?", tenant.FromContextOrDefault(ctx)).
		First(&notice, id)
	return &notice, result.Error
}

func (d *NoticeDAO) GetListContext(ctx context.Context, req pagination.PageRequest, noticeType *int8, status *int8, keyword string) ([]model.Notice, int64, error) {
	var notices []model.Notice
	var total int64

	query := d.dbWithContext(ctx).Model(&model.Notice{}).
		Where("tenant_id = ?", tenant.FromContextOrDefault(ctx))
	if noticeType != nil {
		query = query.Where("type = ?", *noticeType)
	}
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	if keyword != "" {
		query = query.Where("title LIKE ? OR content LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	result := query.Scopes(pagination.Paginate(req)).
		Order("created_at DESC").
		Find(&notices)

	return notices, total, result.Error
}

func (d *NoticeDAO) GetActiveListContext(ctx context.Context, noticeType *int8) ([]model.Notice, error) {
	var notices []model.Notice

	query := d.dbWithContext(ctx).Model(&model.Notice{}).
		Where("tenant_id = ?", tenant.FromContextOrDefault(ctx)).
		Where("status = 1").
		Where("(start_time IS NULL OR start_time <= NOW())").
		Where("(end_time IS NULL OR end_time >= NOW())")

	if noticeType != nil {
		query = query.Where("type = ?", *noticeType)
	}

	result := query.Order("created_at DESC").Find(&notices)
	return notices, result.Error
}

func (d *NoticeDAO) CreateContext(ctx context.Context, notice *model.Notice) error {
	if notice.TenantID == 0 {
		notice.TenantID = tenant.FromContextOrDefault(ctx)
	}
	return d.dbWithContext(ctx).Create(notice).Error
}

func (d *NoticeDAO) UpdateContext(ctx context.Context, notice *model.Notice) error {
	return d.dbWithContext(ctx).Save(notice).Error
}

func (d *NoticeDAO) DeleteContext(ctx context.Context, id uint) error {
	return d.dbWithContext(ctx).
		Where("tenant_id = ?", tenant.FromContextOrDefault(ctx)).
		Delete(&model.Notice{}, id).Error
}

func (d *NoticeDAO) UpdateStatusContext(ctx context.Context, id uint, status int8) error {
	return d.dbWithContext(ctx).Model(&model.Notice{}).
		Where("id = ? AND tenant_id = ?", id, tenant.FromContextOrDefault(ctx)).
		Update("status", status).Error
}

func (d *NoticeDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}
