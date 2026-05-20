package system

import (
	"errors"
	"time"

	"github.com/go-admin-kit/server/internal/dao/system"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
)

// NoticeService 通知公告服务
type NoticeService struct {
	noticeDAO system.NoticeDAO
}

// NoticeListRequest 公告列表请求
type NoticeListRequest struct {
	pagination.PageRequest
	Type    *int8  `json:"type" form:"type"`       // 类型 1:通知 2:公告
	Status  *int8  `json:"status" form:"status"`   // 状态 1:正常 0:关闭
	Keyword string `json:"keyword" form:"keyword"` // 关键词
}

// CreateNoticeRequest 创建公告请求
type CreateNoticeRequest struct {
	Title     string     `json:"title" binding:"required"`
	Content   string     `json:"content" binding:"required"`
	Type      int8       `json:"type"`
	Status    int8       `json:"status"`
	StartTime *time.Time `json:"start_time"`
	EndTime   *time.Time `json:"end_time"`
}

// UpdateNoticeRequest 更新公告请求
type UpdateNoticeRequest struct {
	Title     string     `json:"title"`
	Content   string     `json:"content"`
	Type      int8       `json:"type"`
	Status    int8       `json:"status"`
	StartTime *time.Time `json:"start_time"`
	EndTime   *time.Time `json:"end_time"`
}

// GetByID 根据ID获取公告
func (s *NoticeService) GetByID(id uint) (*model.Notice, error) {
	return s.noticeDAO.GetByID(id)
}

// GetList 获取公告列表
func (s *NoticeService) GetList(req NoticeListRequest) ([]model.Notice, int64, error) {
	return s.noticeDAO.GetList(req.PageRequest, req.Type, req.Status, req.Keyword)
}

// GetActiveList 获取有效的公告列表
func (s *NoticeService) GetActiveList(noticeType *int8) ([]model.Notice, error) {
	return s.noticeDAO.GetActiveList(noticeType)
}

// Create 创建公告
func (s *NoticeService) Create(req CreateNoticeRequest, creatorID uint, creatorName string) (*model.Notice, error) {
	// 默认状态为正常
	if req.Status == 0 {
		req.Status = 1
	}
	// 默认类型为通知
	if req.Type == 0 {
		req.Type = 1
	}

	notice := &model.Notice{
		Title:     req.Title,
		Content:   req.Content,
		Type:      req.Type,
		Status:    req.Status,
		CreatorID: creatorID,
		Creator:   creatorName,
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
	}

	if err := s.noticeDAO.Create(notice); err != nil {
		return nil, err
	}

	return notice, nil
}

// Update 更新公告
func (s *NoticeService) Update(id uint, req UpdateNoticeRequest) (*model.Notice, error) {
	notice, err := s.noticeDAO.GetByID(id)
	if err != nil {
		return nil, errors.New("公告不存在")
	}

	if req.Title != "" {
		notice.Title = req.Title
	}
	if req.Content != "" {
		notice.Content = req.Content
	}
	if req.Type != 0 {
		notice.Type = req.Type
	}
	notice.Status = req.Status
	notice.StartTime = req.StartTime
	notice.EndTime = req.EndTime

	if err := s.noticeDAO.Update(notice); err != nil {
		return nil, err
	}

	return notice, nil
}

// Delete 删除公告
func (s *NoticeService) Delete(id uint) error {
	return s.noticeDAO.Delete(id)
}

// UpdateStatus 更新公告状态
func (s *NoticeService) UpdateStatus(id uint, status int8) error {
	return s.noticeDAO.UpdateStatus(id, status)
}
