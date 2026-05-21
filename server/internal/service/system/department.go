package system

import (
	"context"
	"errors"
	"time"

	systemdao "github.com/go-admin-kit/server/internal/dao/system"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/logger"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
	"gorm.io/gorm"
)

type departmentDAO interface {
	GetByIDContext(ctx context.Context, id uint) (*model.Department, error)
	GetByCodeContext(ctx context.Context, code string) (*model.Department, error)
	GetListContext(ctx context.Context, req pagination.PageRequest, keyword string, status *int8) ([]model.Department, int64, error)
	GetAllContext(ctx context.Context, status *int8) ([]model.Department, error)
	GetTreeContext(ctx context.Context, status *int8) ([]model.Department, error)
	CreateContext(ctx context.Context, dept *model.Department) error
	UpdateContext(ctx context.Context, dept *model.Department) error
	DeleteContext(ctx context.Context, id uint) error
	GetChildrenIDsContext(ctx context.Context, parentID uint) ([]uint, error)
}

type DepartmentService struct {
	deptDAO departmentDAO
}

const departmentTreeInvalidationTimeout = 2 * time.Second

func warnDepartmentTreeInvalidation(err error) {
	if err == nil || logger.Logger == nil {
		return
	}
	logger.Warn("department tree cache invalidation failed", logger.Err(err))
}

func departmentTreeInvalidationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	return context.WithTimeout(base, departmentTreeInvalidationTimeout)
}

func (s *DepartmentService) dao() departmentDAO {
	if s.deptDAO != nil {
		return s.deptDAO
	}
	return &systemdao.DepartmentDAO{}
}

type DepartmentListRequest struct {
	pagination.PageRequest
	Keyword string `json:"keyword" form:"keyword"`
	Status  *int8  `json:"status" form:"status"`
}

type CreateDepartmentRequest struct {
	Name     string `json:"name" binding:"required"`
	Code     string `json:"code" binding:"required"`
	ParentID uint   `json:"parent_id"`
	Leader   string `json:"leader"`
	Phone    string `json:"phone"`
	Email    string `json:"email"`
	Sort     int    `json:"sort"`
	Status   int8   `json:"status"`
}

type UpdateDepartmentRequest struct {
	Name     string `json:"name"`
	ParentID *uint  `json:"parent_id"`
	Leader   string `json:"leader"`
	Phone    string `json:"phone"`
	Email    string `json:"email"`
	Sort     *int   `json:"sort"`
	Status   *int8  `json:"status"`
}

var (
	ErrDepartmentCodeAlreadyExists = errors.New("department code already exists")
	ErrDepartmentNotFound          = errors.New("department does not exist")
	ErrParentDepartmentNotFound    = errors.New("parent department does not exist")
	ErrDepartmentSelfParent        = errors.New("department cannot be its own parent")
	ErrDepartmentHasChildren       = systemdao.ErrDepartmentHasChildren
	ErrDepartmentHasUsers          = systemdao.ErrDepartmentHasUsers
)

// Deprecated: use GetByIDContext instead.
func (s *DepartmentService) GetByID(id uint) (*model.Department, error) {
	return s.GetByIDContext(context.Background(), id)
}

func (s *DepartmentService) GetByIDContext(ctx context.Context, id uint) (*model.Department, error) {
	dept, err := s.dao().GetByIDContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDepartmentNotFound
		}
		return nil, err
	}
	return dept, nil
}

// Deprecated: use GetListContext instead.
func (s *DepartmentService) GetList(req DepartmentListRequest) ([]model.Department, int64, error) {
	return s.GetListContext(context.Background(), req)
}

func (s *DepartmentService) GetListContext(ctx context.Context, req DepartmentListRequest) ([]model.Department, int64, error) {
	return s.dao().GetListContext(ctx, req.PageRequest, req.Keyword, req.Status)
}

// Deprecated: use GetAllContext instead.
func (s *DepartmentService) GetAll(status *int8) ([]model.Department, error) {
	return s.GetAllContext(context.Background(), status)
}

func (s *DepartmentService) GetAllContext(ctx context.Context, status *int8) ([]model.Department, error) {
	return s.dao().GetAllContext(ctx, status)
}

// Deprecated: use GetTreeContext instead.
func (s *DepartmentService) GetTree(status *int8) ([]model.Department, error) {
	return s.GetTreeContext(context.Background(), status)
}

func (s *DepartmentService) GetTreeContext(ctx context.Context, status *int8) ([]model.Department, error) {
	return s.dao().GetTreeContext(ctx, status)
}

// Deprecated: use CreateContext instead.
func (s *DepartmentService) Create(req CreateDepartmentRequest) (*model.Department, error) {
	return s.CreateContext(context.Background(), req)
}

func (s *DepartmentService) CreateContext(ctx context.Context, req CreateDepartmentRequest) (*model.Department, error) {
	dao := s.dao()
	if _, err := dao.GetByCodeContext(ctx, req.Code); err == nil {
		return nil, ErrDepartmentCodeAlreadyExists
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	if req.ParentID > 0 {
		if _, err := dao.GetByIDContext(ctx, req.ParentID); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrParentDepartmentNotFound
			}
			return nil, err
		}
	}

	dept := &model.Department{
		Name:     req.Name,
		Code:     req.Code,
		ParentID: req.ParentID,
		Leader:   req.Leader,
		Phone:    req.Phone,
		Email:    req.Email,
		Sort:     req.Sort,
		Status:   req.Status,
	}
	if dept.Status == 0 {
		dept.Status = 1
	}

	if err := dao.CreateContext(ctx, dept); err != nil {
		return nil, err
	}
	invalidateCtx, cancel := departmentTreeInvalidationContext(ctx)
	defer cancel()
	warnDepartmentTreeInvalidation(authz.InvalidateDepartmentTreeCacheContext(invalidateCtx))
	return dept, nil
}

// Deprecated: use UpdateContext instead.
func (s *DepartmentService) Update(id uint, req UpdateDepartmentRequest) (*model.Department, error) {
	return s.UpdateContext(context.Background(), id, req)
}

func (s *DepartmentService) UpdateContext(ctx context.Context, id uint, req UpdateDepartmentRequest) (*model.Department, error) {
	dao := s.dao()
	dept, err := dao.GetByIDContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDepartmentNotFound
		}
		return nil, err
	}

	if req.ParentID != nil {
		if *req.ParentID == id {
			return nil, ErrDepartmentSelfParent
		}
		if *req.ParentID > 0 {
			if _, err := dao.GetByIDContext(ctx, *req.ParentID); err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, ErrParentDepartmentNotFound
				}
				return nil, err
			}
		}
		dept.ParentID = *req.ParentID
	}

	if req.Name != "" {
		dept.Name = req.Name
	}
	if req.Leader != "" {
		dept.Leader = req.Leader
	}
	if req.Phone != "" {
		dept.Phone = req.Phone
	}
	if req.Email != "" {
		dept.Email = req.Email
	}
	if req.Sort != nil {
		dept.Sort = *req.Sort
	}
	if req.Status != nil {
		dept.Status = *req.Status
	}

	if err := dao.UpdateContext(ctx, dept); err != nil {
		return nil, err
	}
	invalidateCtx, cancel := departmentTreeInvalidationContext(ctx)
	defer cancel()
	warnDepartmentTreeInvalidation(authz.InvalidateDepartmentTreeCacheContext(invalidateCtx))
	return dept, nil
}

// Deprecated: use DeleteContext instead.
func (s *DepartmentService) Delete(id uint) error {
	return s.DeleteContext(context.Background(), id)
}

func (s *DepartmentService) DeleteContext(ctx context.Context, id uint) error {
	if err := s.dao().DeleteContext(ctx, id); err != nil {
		return err
	}
	invalidateCtx, cancel := departmentTreeInvalidationContext(ctx)
	defer cancel()
	warnDepartmentTreeInvalidation(authz.InvalidateDepartmentTreeCacheContext(invalidateCtx))
	return nil
}

// Deprecated: use GetChildrenIDsContext instead.
func (s *DepartmentService) GetChildrenIDs(parentID uint) ([]uint, error) {
	return s.GetChildrenIDsContext(context.Background(), parentID)
}

func (s *DepartmentService) GetChildrenIDsContext(ctx context.Context, parentID uint) ([]uint, error) {
	return s.dao().GetChildrenIDsContext(ctx, parentID)
}
