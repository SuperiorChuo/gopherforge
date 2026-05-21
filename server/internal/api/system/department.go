package system

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/response"
	"github.com/go-admin-kit/server/internal/service/system"
)

// DepartmentAPI handles department endpoints.
type DepartmentAPI struct {
	deptService system.DepartmentService
}

// NewDepartmentAPI creates a DepartmentAPI instance.
func NewDepartmentAPI() *DepartmentAPI {
	return &DepartmentAPI{
		deptService: system.DepartmentService{},
	}
}

// GetDepartmentList returns paginated departments.
func (a *DepartmentAPI) GetDepartmentList(c *gin.Context) {
	var req system.DepartmentListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "invalid query parameters")
		return
	}

	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 10
	}

	depts, total, err := a.deptService.GetListContext(c.Request.Context(), req)
	if err != nil {
		internalServerError(c, "failed to get department list", err)
		return
	}

	response.PageSuccess(c, depts, total, req.Page, req.PageSize)
}

// GetDepartmentTree returns a department tree.
func (a *DepartmentAPI) GetDepartmentTree(c *gin.Context) {
	var status *int8
	if statusStr := c.Query("status"); statusStr != "" {
		statusVal, err := strconv.ParseInt(statusStr, 10, 8)
		if err == nil {
			statusInt8 := int8(statusVal)
			status = &statusInt8
		}
	}

	depts, err := a.deptService.GetTreeContext(c.Request.Context(), status)
	if err != nil {
		internalServerError(c, "failed to get department tree", err)
		return
	}

	response.Success(c, depts)
}

// GetAllDepartments returns all departments.
func (a *DepartmentAPI) GetAllDepartments(c *gin.Context) {
	var status *int8
	if statusStr := c.Query("status"); statusStr != "" {
		statusVal, err := strconv.ParseInt(statusStr, 10, 8)
		if err == nil {
			statusInt8 := int8(statusVal)
			status = &statusInt8
		}
	}

	depts, err := a.deptService.GetAllContext(c.Request.Context(), status)
	if err != nil {
		internalServerError(c, "failed to get departments", err)
		return
	}

	response.Success(c, depts)
}

// GetDepartment returns a department by id.
func (a *DepartmentAPI) GetDepartment(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid department id")
		return
	}

	dept, err := a.deptService.GetByIDContext(c.Request.Context(), uint(id))
	if err != nil {
		writeSystemDepartmentServiceError(c, "failed to get department", err)
		return
	}

	response.Success(c, dept)
}

// CreateDepartment creates a department.
func (a *DepartmentAPI) CreateDepartment(c *gin.Context) {
	var req system.CreateDepartmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	dept, err := a.deptService.CreateContext(c.Request.Context(), req)
	if err != nil {
		writeSystemDepartmentServiceError(c, "failed to create department", err)
		return
	}

	response.SuccessWithMessage(c, "department created", dept)
}

// UpdateDepartment updates a department.
func (a *DepartmentAPI) UpdateDepartment(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid department id")
		return
	}

	var req system.UpdateDepartmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	dept, err := a.deptService.UpdateContext(c.Request.Context(), uint(id), req)
	if err != nil {
		writeSystemDepartmentServiceError(c, "failed to update department", err)
		return
	}

	response.SuccessWithMessage(c, "department updated", dept)
}

// DeleteDepartment deletes a department.
func (a *DepartmentAPI) DeleteDepartment(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid department id")
		return
	}

	if err := a.deptService.DeleteContext(c.Request.Context(), uint(id)); err != nil {
		writeSystemDepartmentServiceError(c, "failed to delete department", err)
		return
	}

	response.SuccessWithMessage(c, "department deleted", nil)
}
