package system

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/response"
	"github.com/go-admin-kit/server/internal/service/system"
)

// DictAPI 字典管理 API
type DictAPI struct {
	dictService system.DictService
}

// NewDictAPI 创建 DictAPI 实例
func NewDictAPI() *DictAPI {
	return &DictAPI{
		dictService: system.DictService{},
	}
}

// ========== 字典类型 ==========

// GetTypeList 获取字典类型列表
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

	types, total, err := a.dictService.GetTypeList(req)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.PageSuccess(c, types, total, req.Page, req.PageSize)
}

// GetAllTypes 获取所有字典类型
func (a *DictAPI) GetAllTypes(c *gin.Context) {
	types, err := a.dictService.GetAllTypes()
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.Success(c, types)
}

// GetType 获取字典类型详情
func (a *DictAPI) GetType(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}

	dictType, err := a.dictService.GetTypeByID(uint(id))
	if err != nil {
		response.NotFound(c, "dict type not found")
		return
	}

	response.Success(c, dictType)
}

// CreateType 创建字典类型
func (a *DictAPI) CreateType(c *gin.Context) {
	var req system.CreateDictTypeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	dictType, err := a.dictService.CreateType(req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "dict type created successfully", dictType)
}

// UpdateType 更新字典类型
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

	dictType, err := a.dictService.UpdateType(uint(id), req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "dict type updated successfully", dictType)
}

// DeleteType 删除字典类型
func (a *DictAPI) DeleteType(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}

	if err := a.dictService.DeleteType(uint(id)); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "dict type deleted successfully", nil)
}

// ========== 字典项 ==========

// GetItemList 获取字典项列表
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

	items, total, err := a.dictService.GetItemList(req)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.PageSuccess(c, items, total, req.Page, req.PageSize)
}

// GetItemsByTypeID 根据类型ID获取字典项
func (a *DictAPI) GetItemsByTypeID(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid type id")
		return
	}

	items, err := a.dictService.GetItemsByTypeID(uint(id))
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.Success(c, items)
}

// GetItem 获取字典项详情
func (a *DictAPI) GetItem(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}

	item, err := a.dictService.GetItemByID(uint(id))
	if err != nil {
		response.NotFound(c, "dict item not found")
		return
	}

	response.Success(c, item)
}

// CreateItem 创建字典项
func (a *DictAPI) CreateItem(c *gin.Context) {
	var req system.CreateDictItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	item, err := a.dictService.CreateItem(req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "dict item created successfully", item)
}

// UpdateItem 更新字典项
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

	item, err := a.dictService.UpdateItem(uint(id), req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "dict item updated successfully", item)
}

// DeleteItem 删除字典项
func (a *DictAPI) DeleteItem(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}

	if err := a.dictService.DeleteItem(uint(id)); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "dict item deleted successfully", nil)
}

// ========== 前端使用 ==========

// GetDictData 获取字典数据（根据编码）
func (a *DictAPI) GetDictData(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		response.BadRequest(c, "code is required")
		return
	}

	items, err := a.dictService.GetDictData(code)
	if err != nil {
		response.NotFound(c, "dict not found")
		return
	}

	response.Success(c, items)
}

// GetMultipleDictData 批量获取字典数据
func (a *DictAPI) GetMultipleDictData(c *gin.Context) {
	codesStr := c.Query("codes")
	if codesStr == "" {
		response.BadRequest(c, "codes is required")
		return
	}

	codes := strings.Split(codesStr, ",")
	data, err := a.dictService.GetMultipleDictData(codes)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.Success(c, data)
}

// GetAllDictData 获取所有字典数据
func (a *DictAPI) GetAllDictData(c *gin.Context) {
	data, err := a.dictService.GetAllDictData()
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.Success(c, data)
}
