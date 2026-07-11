package system

import (
	"context"
	"errors"
	"time"

	systemdao "github.com/go-admin-kit/server/internal/dao/system"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
	"gorm.io/gorm"
)

type NoticeService struct {
	noticeDAO systemdao.NoticeDAO
}

// NewNoticeServiceWithDB builds a NoticeService backed by an injected database handle.
func NewNoticeServiceWithDB(db *gorm.DB) NoticeService {
	return NoticeService{noticeDAO: *systemdao.NewNoticeDAO(db)}
}

type NoticeListRequest struct {
	pagination.PageRequest
	Type    *int8  `json:"type" form:"type"`
	Status  *int8  `json:"status" form:"status"`
	Keyword string `json:"keyword" form:"keyword"`
}

type CreateNoticeRequest struct {
	Title     string     `json:"title" binding:"required"`
	Content   string     `json:"content" binding:"required"`
	Type      int8       `json:"type"`
	Status    int8       `json:"status"`
	StartTime *time.Time `json:"start_time"`
	EndTime   *time.Time `json:"end_time"`
}

type UpdateNoticeRequest struct {
	Title     string     `json:"title"`
	Content   string     `json:"content"`
	Type      int8       `json:"type"`
	Status    int8       `json:"status"`
	StartTime *time.Time `json:"start_time"`
	EndTime   *time.Time `json:"end_time"`
}

var ErrNoticeNotFound = errors.New("notice not found")

func (s *NoticeService) GetByIDContext(ctx context.Context, id uint) (*model.Notice, error) {
	notice, err := s.noticeDAO.GetByIDContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNoticeNotFound
		}
		return nil, err
	}
	return notice, nil
}

func (s *NoticeService) GetListContext(ctx context.Context, req NoticeListRequest) ([]model.Notice, int64, error) {
	return s.noticeDAO.GetListContext(ctx, req.PageRequest, req.Type, req.Status, req.Keyword)
}

func (s *NoticeService) GetActiveListContext(ctx context.Context, noticeType *int8) ([]model.Notice, error) {
	return s.noticeDAO.GetActiveListContext(ctx, noticeType)
}

func (s *NoticeService) CreateContext(ctx context.Context, req CreateNoticeRequest, creatorID uint, creatorName string) (*model.Notice, error) {
	if req.Status == 0 {
		req.Status = 1
	}
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

	if err := s.noticeDAO.CreateContext(ctx, notice); err != nil {
		return nil, err
	}

	return notice, nil
}

func (s *NoticeService) UpdateContext(ctx context.Context, id uint, req UpdateNoticeRequest) (*model.Notice, error) {
	notice, err := s.noticeDAO.GetByIDContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNoticeNotFound
		}
		return nil, err
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

	if err := s.noticeDAO.UpdateContext(ctx, notice); err != nil {
		return nil, err
	}

	return notice, nil
}

func (s *NoticeService) DeleteContext(ctx context.Context, id uint) error {
	return s.noticeDAO.DeleteContext(ctx, id)
}

func (s *NoticeService) UpdateStatusContext(ctx context.Context, id uint, status int8) error {
	return s.noticeDAO.UpdateStatusContext(ctx, id, status)
}
