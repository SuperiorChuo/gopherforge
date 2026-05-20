package system

import (
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
)

// NoticeDAO 通知公告数据访问对象
type NoticeDAO struct{}

// GetByID 根据ID获取公告
func (d *NoticeDAO) GetByID(id uint) (*model.Notice, error) {
	var notice model.Notice
	result := database.DB.First(&notice, id)
	return &notice, result.Error
}

// GetList 获取公告列表（分页）
func (d *NoticeDAO) GetList(req pagination.PageRequest, noticeType *int8, status *int8, keyword string) ([]model.Notice, int64, error) {
	var notices []model.Notice
	var total int64

	query := database.DB.Model(&model.Notice{})

	// 类型筛选
	if noticeType != nil {
		query = query.Where("type = ?", *noticeType)
	}

	// 状态筛选
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	// 关键词搜索
	if keyword != "" {
		query = query.Where("title LIKE ? OR content LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	result := query.Scopes(pagination.Paginate(req)).
		Order("created_at DESC").
		Find(&notices)

	return notices, total, result.Error
}

// GetActiveList 获取有效的公告列表（已发布且在有效期内）
func (d *NoticeDAO) GetActiveList(noticeType *int8) ([]model.Notice, error) {
	var notices []model.Notice

	query := database.DB.Model(&model.Notice{}).
		Where("status = 1").
		Where("(start_time IS NULL OR start_time <= NOW())").
		Where("(end_time IS NULL OR end_time >= NOW())")

	if noticeType != nil {
		query = query.Where("type = ?", *noticeType)
	}

	result := query.Order("created_at DESC").Find(&notices)
	return notices, result.Error
}

// Create 创建公告
func (d *NoticeDAO) Create(notice *model.Notice) error {
	return database.DB.Create(notice).Error
}

// Update 更新公告
func (d *NoticeDAO) Update(notice *model.Notice) error {
	return database.DB.Save(notice).Error
}

// Delete 删除公告
func (d *NoticeDAO) Delete(id uint) error {
	return database.DB.Delete(&model.Notice{}, id).Error
}

// UpdateStatus 更新公告状态
func (d *NoticeDAO) UpdateStatus(id uint, status int8) error {
	return database.DB.Model(&model.Notice{}).Where("id = ?", id).Update("status", status).Error
}
