package system

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/response"
	"github.com/go-admin-kit/server/internal/service/system"
)

// DictAPI handles dictionary management endpoints.
type DictAPI struct {
	dictService system.DictService
}

// NewDictAPI creates a DictAPI instance.
func NewDictAPI() *DictAPI {
	return &DictAPI{
		dictService: system.DictService{},
	}
}

// ========== Dict types ==========

// GetTypeList returns paginated dictionary types.
func (a *DictAPI) GetTypeList(c *gin.Context) {
	var req system.DictTypeListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 10
	}

	types, total, err := a.dictService.GetTypeListContext(c.Request.Context(), req)
	if err != nil {
		internalServerError(c, "failed to get dictionary type list", err)
		return
	}

	response.PageSuccess(c, types, total, req.Page, req.PageSize)
}

// GetAllTypes returns all dictionary types.
func (a *DictAPI) GetAllTypes(c *gin.Context) {
	types, err := a.dictService.GetAllTypesContext(c.Request.Context())
	if err != nil {
		internalServerError(c, "failed to get dictionary types", err)
		return
	}

	response.Success(c, types)
}

// GetType returns a dictionary type by id.
func (a *DictAPI) GetType(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}

	dictType, err := a.dictService.GetTypeByIDContext(c.Request.Context(), uint(id))
	if err != nil {
		writeSystemDictServiceError(c, "failed to get dictionary type", err)
		return
	}

	response.Success(c, dictType)
}

// CreateType creates a dictionary type.
func (a *DictAPI) CreateType(c *gin.Context) {
	var req system.CreateDictTypeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	dictType, err := a.dictService.CreateTypeContext(c.Request.Context(), req)
	if err != nil {
		writeSystemDictServiceError(c, "failed to create dictionary type", err)
		return
	}

	response.SuccessWithMessage(c, "dict type created successfully", dictType)
}

// UpdateType updates a dictionary type.
func (a *DictAPI) UpdateType(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}

	var req system.UpdateDictTypeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	dictType, err := a.dictService.UpdateTypeContext(c.Request.Context(), uint(id), req)
	if err != nil {
		writeSystemDictServiceError(c, "failed to update dictionary type", err)
		return
	}

	response.SuccessWithMessage(c, "dict type updated successfully", dictType)
}

// DeleteType deletes a dictionary type.
func (a *DictAPI) DeleteType(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}

	if err := a.dictService.DeleteTypeContext(c.Request.Context(), uint(id)); err != nil {
		writeSystemDictServiceError(c, "failed to delete dictionary type", err)
		return
	}

	response.SuccessWithMessage(c, "dict type deleted successfully", nil)
}

// ========== Dict items ==========

// GetItemList returns paginated dictionary items.
func (a *DictAPI) GetItemList(c *gin.Context) {
	var req system.DictItemListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 10
	}

	items, total, err := a.dictService.GetItemListContext(c.Request.Context(), req)
	if err != nil {
		internalServerError(c, "failed to get dictionary item list", err)
		return
	}

	response.PageSuccess(c, items, total, req.Page, req.PageSize)
}

// GetItemsByTypeID returns dictionary items by type id.
func (a *DictAPI) GetItemsByTypeID(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid type id")
		return
	}

	items, err := a.dictService.GetItemsByTypeIDContext(c.Request.Context(), uint(id))
	if err != nil {
		internalServerError(c, "failed to get dictionary items by type", err)
		return
	}

	response.Success(c, items)
}

// GetItem returns a dictionary item by id.
func (a *DictAPI) GetItem(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}

	item, err := a.dictService.GetItemByIDContext(c.Request.Context(), uint(id))
	if err != nil {
		writeSystemDictServiceError(c, "failed to get dictionary item", err)
		return
	}

	response.Success(c, item)
}

// CreateItem creates a dictionary item.
func (a *DictAPI) CreateItem(c *gin.Context) {
	var req system.CreateDictItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	item, err := a.dictService.CreateItemContext(c.Request.Context(), req)
	if err != nil {
		writeSystemDictServiceError(c, "failed to create dictionary item", err)
		return
	}

	response.SuccessWithMessage(c, "dict item created successfully", item)
}

// UpdateItem updates a dictionary item.
func (a *DictAPI) UpdateItem(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}

	var req system.UpdateDictItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	item, err := a.dictService.UpdateItemContext(c.Request.Context(), uint(id), req)
	if err != nil {
		writeSystemDictServiceError(c, "failed to update dictionary item", err)
		return
	}

	response.SuccessWithMessage(c, "dict item updated successfully", item)
}

// DeleteItem deletes a dictionary item.
func (a *DictAPI) DeleteItem(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}

	if err := a.dictService.DeleteItemContext(c.Request.Context(), uint(id)); err != nil {
		writeSystemDictServiceError(c, "failed to delete dictionary item", err)
		return
	}

	response.SuccessWithMessage(c, "dict item deleted successfully", nil)
}

// ========== Frontend dictionary data ==========

// GetDictData returns dictionary data by code.
func (a *DictAPI) GetDictData(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		response.BadRequest(c, "code is required")
		return
	}

	items, err := a.dictService.GetDictDataContext(c.Request.Context(), code)
	if err != nil {
		writeSystemDictServiceError(c, "failed to get dictionary data", err)
		return
	}

	response.Success(c, items)
}

// GetMultipleDictData returns dictionary data for multiple codes.
func (a *DictAPI) GetMultipleDictData(c *gin.Context) {
	codesStr := c.Query("codes")
	if codesStr == "" {
		response.BadRequest(c, "codes is required")
		return
	}

	codes := strings.Split(codesStr, ",")
	data, err := a.dictService.GetMultipleDictDataContext(c.Request.Context(), codes)
	if err != nil {
		internalServerError(c, "failed to get dictionary data", err)
		return
	}

	response.Success(c, data)
}

// GetAllDictData returns all dictionary data.
func (a *DictAPI) GetAllDictData(c *gin.Context) {
	data, err := a.dictService.GetAllDictDataContext(c.Request.Context())
	if err != nil {
		internalServerError(c, "failed to get all dictionary data", err)
		return
	}

	response.Success(c, data)
}
