package system

import (
	"errors"

	"github.com/go-admin-kit/server/internal/dao/system"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
)

// DepartmentService 部门服务
type DepartmentService struct {
	deptDAO system.DepartmentDAO
}

// DepartmentListRequest 部门列表请求
type DepartmentListRequest struct {
	pagination.PageRequest
	Keyword string `json:"keyword" form:"keyword"`
	Status  *int8  `json:"status" form:"status"`
}

// CreateDepartmentRequest 创建部门请求
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

// UpdateDepartmentRequest 更新部门请求
type UpdateDepartmentRequest struct {
	Name     string `json:"name"`
	ParentID *uint  `json:"parent_id"`
	Leader   string `json:"leader"`
	Phone    string `json:"phone"`
	Email    string `json:"email"`
	Sort     *int   `json:"sort"`
	Status   *int8  `json:"status"`
}

// GetByID 根据ID获取部门
func (s *DepartmentService) GetByID(id uint) (*model.Department, error) {
	return s.deptDAO.GetByID(id)
}

// GetList 获取部门列表
func (s *DepartmentService) GetList(req DepartmentListRequest) ([]model.Department, int64, error) {
	return s.deptDAO.GetList(req.PageRequest, req.Keyword, req.Status)
}

// GetAll 获取所有部门
func (s *DepartmentService) GetAll(status *int8) ([]model.Department, error) {
	return s.deptDAO.GetAll(status)
}

// GetTree 获取部门树
func (s *DepartmentService) GetTree(status *int8) ([]model.Department, error) {
	return s.deptDAO.GetTree(status)
}

// Create 创建部门
func (s *DepartmentService) Create(req CreateDepartmentRequest) (*model.Department, error) {
	// 检查编码是否已存在
	_, err := s.deptDAO.GetByCode(req.Code)
	if err == nil {
		return nil, errors.New("部门编码已存在")
	}

	// 检查父部门是否存在
	if req.ParentID > 0 {
		_, err := s.deptDAO.GetByID(req.ParentID)
		if err != nil {
			return nil, errors.New("父部门不存在")
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

	// 默认启用状态
	if dept.Status == 0 {
		dept.Status = 1
	}

	if err := s.deptDAO.Create(dept); err != nil {
		return nil, err
	}

	return dept, nil
}

// Update 更新部门
func (s *DepartmentService) Update(id uint, req UpdateDepartmentRequest) (*model.Department, error) {
	dept, err := s.deptDAO.GetByID(id)
	if err != nil {
		return nil, errors.New("部门不存在")
	}

	// 检查父部门是否存在且不能是自己
	if req.ParentID != nil {
		if *req.ParentID == id {
			return nil, errors.New("不能将自己设为父部门")
		}
		if *req.ParentID > 0 {
			_, err := s.deptDAO.GetByID(*req.ParentID)
			if err != nil {
				return nil, errors.New("父部门不存在")
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

	if err := s.deptDAO.Update(dept); err != nil {
		return nil, err
	}

	return dept, nil
}

// Delete 删除部门
func (s *DepartmentService) Delete(id uint) error {
	return s.deptDAO.Delete(id)
}

// GetChildrenIDs 获取所有子部门ID
func (s *DepartmentService) GetChildrenIDs(parentID uint) ([]uint, error) {
	return s.deptDAO.GetChildrenIDs(parentID)
}
