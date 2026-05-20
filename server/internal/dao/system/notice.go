package system

import (
	"context"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
)

type NoticeDAO struct{}

func (d *NoticeDAO) GetByID(id uint) (*model.Notice, error) {
	return d.GetByIDContext(context.Background(), id)
}

func (d *NoticeDAO) GetByIDContext(ctx context.Context, id uint) (*model.Notice, error) {
	var notice model.Notice
	result := dbWithContext(ctx).First(&notice, id)
	return &notice, result.Error
}

func (d *NoticeDAO) GetList(req pagination.PageRequest, noticeType *int8, status *int8, keyword string) ([]model.Notice, int64, error) {
	return d.GetListContext(context.Background(), req, noticeType, status, keyword)
}

func (d *NoticeDAO) GetListContext(ctx context.Context, req pagination.PageRequest, noticeType *int8, status *int8, keyword string) ([]model.Notice, int64, error) {
	var notices []model.Notice
	var total int64

	query := dbWithContext(ctx).Model(&model.Notice{})
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

func (d *NoticeDAO) GetActiveList(noticeType *int8) ([]model.Notice, error) {
	return d.GetActiveListContext(context.Background(), noticeType)
}

func (d *NoticeDAO) GetActiveListContext(ctx context.Context, noticeType *int8) ([]model.Notice, error) {
	var notices []model.Notice

	query := dbWithContext(ctx).Model(&model.Notice{}).
		Where("status = 1").
		Where("(start_time IS NULL OR start_time <= NOW())").
		Where("(end_time IS NULL OR end_time >= NOW())")

	if noticeType != nil {
		query = query.Where("type = ?", *noticeType)
	}

	result := query.Order("created_at DESC").Find(&notices)
	return notices, result.Error
}

func (d *NoticeDAO) Create(notice *model.Notice) error {
	return d.CreateContext(context.Background(), notice)
}

func (d *NoticeDAO) CreateContext(ctx context.Context, notice *model.Notice) error {
	return dbWithContext(ctx).Create(notice).Error
}

func (d *NoticeDAO) Update(notice *model.Notice) error {
	return d.UpdateContext(context.Background(), notice)
}

func (d *NoticeDAO) UpdateContext(ctx context.Context, notice *model.Notice) error {
	return dbWithContext(ctx).Save(notice).Error
}

func (d *NoticeDAO) Delete(id uint) error {
	return d.DeleteContext(context.Background(), id)
}

func (d *NoticeDAO) DeleteContext(ctx context.Context, id uint) error {
	return dbWithContext(ctx).Delete(&model.Notice{}, id).Error
}

func (d *NoticeDAO) UpdateStatus(id uint, status int8) error {
	return d.UpdateStatusContext(context.Background(), id, status)
}

func (d *NoticeDAO) UpdateStatusContext(ctx context.Context, id uint, status int8) error {
	return dbWithContext(ctx).Model(&model.Notice{}).Where("id = ?", id).Update("status", status).Error
}
