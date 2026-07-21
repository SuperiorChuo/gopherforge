package system

import (
	"context"
	"errors"

	systemdao "github.com/go-admin-kit/services/identity/internal/dao/system"
	"github.com/go-admin-kit/services/identity/internal/model"
	"github.com/go-admin-kit/services/identity/internal/pkg/pagination"
	"github.com/go-admin-kit/services/identity/internal/pkg/tenant"
	"gorm.io/gorm"
)

type postDAO interface {
	GetByIDContext(ctx context.Context, id uint) (*model.Post, error)
	GetByCodeContext(ctx context.Context, code string) (*model.Post, error)
	GetListContext(ctx context.Context, req pagination.PageRequest, keyword string, status *int8) ([]model.Post, int64, error)
	GetAllContext(ctx context.Context, status *int8) ([]model.Post, error)
	CreateContext(ctx context.Context, post *model.Post) error
	UpdateContext(ctx context.Context, post *model.Post) error
	DeleteContext(ctx context.Context, id uint) error
}

// PostService manages job positions for the system module.
type PostService struct {
	postDAO postDAO
}

// NewPostServiceWithDB builds a PostService backed by an injected database handle.
func NewPostServiceWithDB(db *gorm.DB) PostService {
	return PostService{postDAO: systemdao.NewPostDAO(db)}
}

func (s *PostService) dao() postDAO {
	if s.postDAO != nil {
		return s.postDAO
	}
	return &systemdao.PostDAO{}
}

type PostListRequest struct {
	pagination.PageRequest
	Keyword string `json:"keyword" form:"keyword"`
	Status  *int8  `json:"status" form:"status"`
}

type CreatePostRequest struct {
	Code   string `json:"code" binding:"required"`
	Name   string `json:"name" binding:"required"`
	Sort   int    `json:"sort"`
	Status int8   `json:"status"`
	Remark string `json:"remark"`
}

type UpdatePostRequest struct {
	Code   string  `json:"code"`
	Name   string  `json:"name"`
	Sort   *int    `json:"sort"`
	Status *int8   `json:"status"`
	Remark *string `json:"remark"`
}

var (
	ErrPostCodeAlreadyExists = errors.New("post code already exists")
	ErrPostNotFound          = errors.New("post does not exist")
	ErrPostHasUsers          = systemdao.ErrPostHasUsers
)

func (s *PostService) GetByIDContext(ctx context.Context, id uint) (*model.Post, error) {
	post, err := s.dao().GetByIDContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPostNotFound
		}
		return nil, err
	}
	return post, nil
}

func (s *PostService) GetListContext(ctx context.Context, req PostListRequest) ([]model.Post, int64, error) {
	return s.dao().GetListContext(ctx, req.PageRequest, req.Keyword, req.Status)
}

func (s *PostService) GetAllContext(ctx context.Context, status *int8) ([]model.Post, error) {
	return s.dao().GetAllContext(ctx, status)
}

func (s *PostService) CreateContext(ctx context.Context, req CreatePostRequest) (*model.Post, error) {
	dao := s.dao()
	if _, err := dao.GetByCodeContext(ctx, req.Code); err == nil {
		return nil, ErrPostCodeAlreadyExists
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	post := &model.Post{
		TenantID: tenant.Normalize(tenant.FromContext(ctx)),
		Code:     req.Code,
		Name:     req.Name,
		Sort:     req.Sort,
		Status:   req.Status,
		Remark:   req.Remark,
	}
	if post.Status == 0 {
		post.Status = 1
	}

	if err := dao.CreateContext(ctx, post); err != nil {
		return nil, err
	}
	return post, nil
}

func (s *PostService) UpdateContext(ctx context.Context, id uint, req UpdatePostRequest) (*model.Post, error) {
	dao := s.dao()
	post, err := dao.GetByIDContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPostNotFound
		}
		return nil, err
	}

	if req.Code != "" && req.Code != post.Code {
		if existing, err := dao.GetByCodeContext(ctx, req.Code); err == nil && existing.ID != id {
			return nil, ErrPostCodeAlreadyExists
		} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		post.Code = req.Code
	}

	if req.Name != "" {
		post.Name = req.Name
	}
	if req.Sort != nil {
		post.Sort = *req.Sort
	}
	if req.Status != nil {
		post.Status = *req.Status
	}
	if req.Remark != nil {
		post.Remark = *req.Remark
	}

	if err := dao.UpdateContext(ctx, post); err != nil {
		return nil, err
	}
	return post, nil
}

func (s *PostService) DeleteContext(ctx context.Context, id uint) error {
	if _, err := s.GetByIDContext(ctx, id); err != nil {
		return err
	}
	return s.dao().DeleteContext(ctx, id)
}
