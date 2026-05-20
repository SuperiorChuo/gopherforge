package system

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/response"
	"github.com/go-admin-kit/server/internal/service/system"
)

// DepartmentAPI 部门管理API
type DepartmentAPI struct {
	deptService system.DepartmentService
}

// NewDepartmentAPI 创建DepartmentAPI实例
func NewDepartmentAPI() *DepartmentAPI {
	return &DepartmentAPI{
		deptService: system.DepartmentService{},
	}
}

// GetDepartmentList 获取部门列表
func (a *DepartmentAPI) GetDepartmentList(c *gin.Context) {
	var req system.DepartmentListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// 设置默认分页参数
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 10
	}

	depts, total, err := a.deptService.GetList(req)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.PageSuccess(c, depts, total, req.Page, req.PageSize)
}

// GetDepartmentTree 获取部门树
func (a *DepartmentAPI) GetDepartmentTree(c *gin.Context) {
	var status *int8
	if statusStr := c.Query("status"); statusStr != "" {
		statusVal, err := strconv.ParseInt(statusStr, 10, 8)
		if err == nil {
			statusInt8 := int8(statusVal)
			status = &statusInt8
		}
	}

	depts, err := a.deptService.GetTree(status)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.Success(c, depts)
}

// GetAllDepartments 获取所有部门
func (a *DepartmentAPI) GetAllDepartments(c *gin.Context) {
	var status *int8
	if statusStr := c.Query("status"); statusStr != "" {
		statusVal, err := strconv.ParseInt(statusStr, 10, 8)
		if err == nil {
			statusInt8 := int8(statusVal)
			status = &statusInt8
		}
	}

	depts, err := a.deptService.GetAll(status)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.Success(c, depts)
}

// GetDepartment 获取部门详情
func (a *DepartmentAPI) GetDepartment(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "无效的部门ID")
		return
	}

	dept, err := a.deptService.GetByID(uint(id))
	if err != nil {
		response.NotFound(c, "部门不存在")
		return
	}

	response.Success(c, dept)
}

// CreateDepartment 创建部门
func (a *DepartmentAPI) CreateDepartment(c *gin.Context) {
	var req system.CreateDepartmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	dept, err := a.deptService.Create(req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "部门创建成功", dept)
}

// UpdateDepartment 更新部门
func (a *DepartmentAPI) UpdateDepartment(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "无效的部门ID")
		return
	}

	var req system.UpdateDepartmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	dept, err := a.deptService.Update(uint(id), req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "部门更新成功", dept)
}

// DeleteDepartment 删除部门
func (a *DepartmentAPI) DeleteDepartment(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "无效的部门ID")
		return
	}

	if err := a.deptService.Delete(uint(id)); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "部门删除成功", nil)
}
