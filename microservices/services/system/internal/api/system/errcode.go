package system

import (
	"context"
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/response"
	"github.com/go-admin-kit/services/system/internal/pkg/runtimeconfig"
	systemsvc "github.com/go-admin-kit/services/system/internal/service/system"
)

// ErrCodeAPI 错误码管理接口：CRUD + 全量启用错误码拉取。
type ErrCodeAPI struct {
	errCodeService systemsvc.ErrCodeService
}

// NewErrCodeAPI 用全局回退依赖构建 ErrCodeAPI。
func NewErrCodeAPI() *ErrCodeAPI {
	return &ErrCodeAPI{errCodeService: systemsvc.ErrCodeService{}}
}

// NewErrCodeAPIWithService 用注入的 service 构建 ErrCodeAPI。
func NewErrCodeAPIWithService(errCodeService systemsvc.ErrCodeService) *ErrCodeAPI {
	return &ErrCodeAPI{errCodeService: errCodeService}
}

// GetList 分页查询错误码，支持 keyword/scope/status 筛选。
func (a *ErrCodeAPI) GetList(c *gin.Context) {
	var req systemsvc.ErrCodeListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "invalid query parameters")
		return
	}

	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 10
	}

	codes, total, err := a.errCodeService.GetListContext(c.Request.Context(), req)
	if err != nil {
		internalServerError(c, "failed to get error code list", err)
		return
	}

	response.PageSuccess(c, codes, total, req.Page, req.PageSize)
}

// GetAllEnabled 返回全量启用错误码（供各服务/前端整包拉取做本地缓存）。
func (a *ErrCodeAPI) GetAllEnabled(c *gin.Context) {
	codes, err := a.errCodeService.GetAllEnabledContext(c.Request.Context())
	if err != nil {
		internalServerError(c, "failed to get enabled error codes", err)
		return
	}

	response.Success(c, codes)
}

// Get 按 id 查询单个错误码。
func (a *ErrCodeAPI) Get(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}

	errorCode, err := a.errCodeService.GetByIDContext(c.Request.Context(), uint(id))
	if err != nil {
		writeSystemErrCodeServiceError(c, "failed to get error code", err)
		return
	}

	response.Success(c, errorCode)
}

// Create 新建错误码。
func (a *ErrCodeAPI) Create(c *gin.Context) {
	var req systemsvc.CreateErrCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	errorCode, err := a.errCodeService.CreateContext(c.Request.Context(), req)
	if err != nil {
		writeSystemErrCodeServiceError(c, "failed to create error code", err)
		return
	}

	response.SuccessWithMessage(c, "error code created successfully", errorCode)
}

// Update 更新错误码文案/备注/来源/状态（code 标识不可改，启停也走这里）。
func (a *ErrCodeAPI) Update(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}

	var req systemsvc.UpdateErrCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	errorCode, err := a.errCodeService.UpdateContext(c.Request.Context(), uint(id), req)
	if err != nil {
		writeSystemErrCodeServiceError(c, "failed to update error code", err)
		return
	}

	response.SuccessWithMessage(c, "error code updated successfully", errorCode)
}

// Delete 删除错误码。
func (a *ErrCodeAPI) Delete(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}

	if err := a.errCodeService.DeleteContext(c.Request.Context(), uint(id)); err != nil {
		writeSystemErrCodeServiceError(c, "failed to delete error code", err)
		return
	}

	response.SuccessWithMessage(c, "error code deleted successfully", nil)
}

// writeSystemErrCodeServiceError 将 service 层错误映射为 HTTP 响应。
func writeSystemErrCodeServiceError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, systemsvc.ErrErrorCodeAlreadyExists):
		response.BadRequest(c, systemsvc.ErrErrorCodeAlreadyExists.Error())
	case errors.Is(err, systemsvc.ErrErrorCodeCodeRequired):
		response.BadRequest(c, systemsvc.ErrErrorCodeCodeRequired.Error())
	case errors.Is(err, systemsvc.ErrErrorCodeNotFound):
		response.NotFound(c, systemsvc.ErrErrorCodeNotFound.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		internalServerError(c, operation, err)
	default:
		internalServerError(c, operation, err)
	}
}

// errcodeMessageOr 错误码文案覆盖：命中错误码表中启用的自定义文案则用之，
// 否则回落到代码里的默认文案。读取走 30s TTL 内存缓存，在线修改约 30 秒热生效。
func errcodeMessageOr(c *gin.Context, code response.ErrorCode, fallback string) string {
	if message, ok := runtimeconfig.DefaultErrorCodeReader().GetMessage(c.Request.Context(), string(code)); ok {
		return message
	}
	return fallback
}
