package system

import (
	"errors"

	"github.com/go-admin-kit/server/internal/dao/system"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
)

// DictService 字典服务
type DictService struct {
	dictDAO system.DictDAO
}

// DictTypeListRequest 字典类型列表请求
type DictTypeListRequest struct {
	pagination.PageRequest
	Keyword string `form:"keyword" json:"keyword"`
	Status  *int8  `form:"status" json:"status"`
}

// CreateDictTypeRequest 创建字典类型请求
type CreateDictTypeRequest struct {
	Name        string `json:"name" binding:"required"`
	Code        string `json:"code" binding:"required"`
	Description string `json:"description"`
	Status      int8   `json:"status"`
}

// UpdateDictTypeRequest 更新字典类型请求
type UpdateDictTypeRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      *int8  `json:"status"`
}

// DictItemListRequest 字典项列表请求
type DictItemListRequest struct {
	pagination.PageRequest
	TypeID  uint   `form:"type_id" json:"type_id" binding:"required"`
	Keyword string `form:"keyword" json:"keyword"`
	Status  *int8  `form:"status" json:"status"`
}

// CreateDictItemRequest 创建字典项请求
type CreateDictItemRequest struct {
	DictTypeID uint   `json:"dict_type_id" binding:"required"`
	Label      string `json:"label" binding:"required"`
	Value      string `json:"value" binding:"required"`
	Sort       int    `json:"sort"`
	Status     int8   `json:"status"`
	Remark     string `json:"remark"`
}

// UpdateDictItemRequest 更新字典项请求
type UpdateDictItemRequest struct {
	Label  string `json:"label"`
	Value  string `json:"value"`
	Sort   int    `json:"sort"`
	Status *int8  `json:"status"`
	Remark string `json:"remark"`
}

// ========== 字典类型 ==========

// CreateType 创建字典类型
func (s *DictService) CreateType(req CreateDictTypeRequest) (*model.DictType, error) {
	// 检查编码是否已存在
	_, err := s.dictDAO.GetTypeByCode(req.Code)
	if err == nil {
		return nil, errors.New("dict type code already exists")
	}

	dictType := &model.DictType{
		Name:        req.Name,
		Code:        req.Code,
		Description: req.Description,
		Status:      req.Status,
	}

	if dictType.Status == 0 {
		dictType.Status = 1
	}

	if err := s.dictDAO.CreateType(dictType); err != nil {
		return nil, err
	}

	return dictType, nil
}

// GetTypeByID 根据ID获取字典类型
func (s *DictService) GetTypeByID(id uint) (*model.DictType, error) {
	return s.dictDAO.GetTypeByID(id)
}

// GetTypeByCode 根据编码获取字典类型
func (s *DictService) GetTypeByCode(code string) (*model.DictType, error) {
	return s.dictDAO.GetTypeByCode(code)
}

// GetTypeList 获取字典类型列表
func (s *DictService) GetTypeList(req DictTypeListRequest) ([]model.DictType, int64, error) {
	return s.dictDAO.GetTypeList(req.PageRequest, req.Keyword, req.Status)
}

// GetAllTypes 获取所有字典类型
func (s *DictService) GetAllTypes() ([]model.DictType, error) {
	status := int8(1)
	return s.dictDAO.GetAllTypes(&status)
}

// UpdateType 更新字典类型
func (s *DictService) UpdateType(id uint, req UpdateDictTypeRequest) (*model.DictType, error) {
	dictType, err := s.dictDAO.GetTypeByID(id)
	if err != nil {
		return nil, errors.New("dict type not found")
	}

	if req.Name != "" {
		dictType.Name = req.Name
	}
	if req.Description != "" {
		dictType.Description = req.Description
	}
	if req.Status != nil {
		dictType.Status = *req.Status
	}

	if err := s.dictDAO.UpdateType(dictType); err != nil {
		return nil, err
	}

	return dictType, nil
}

// DeleteType 删除字典类型
func (s *DictService) DeleteType(id uint) error {
	return s.dictDAO.DeleteType(id)
}

// ========== 字典项 ==========

// CreateItem 创建字典项
func (s *DictService) CreateItem(req CreateDictItemRequest) (*model.DictItem, error) {
	// 检查字典类型是否存在
	_, err := s.dictDAO.GetTypeByID(req.DictTypeID)
	if err != nil {
		return nil, errors.New("dict type not found")
	}

	item := &model.DictItem{
		DictTypeID: req.DictTypeID,
		Label:      req.Label,
		Value:      req.Value,
		Sort:       req.Sort,
		Status:     req.Status,
		Remark:     req.Remark,
	}

	if item.Status == 0 {
		item.Status = 1
	}

	if err := s.dictDAO.CreateItem(item); err != nil {
		return nil, err
	}

	return item, nil
}

// GetItemByID 根据ID获取字典项
func (s *DictService) GetItemByID(id uint) (*model.DictItem, error) {
	return s.dictDAO.GetItemByID(id)
}

// GetItemsByTypeID 根据类型ID获取字典项列表
func (s *DictService) GetItemsByTypeID(typeID uint) ([]model.DictItem, error) {
	status := int8(1)
	return s.dictDAO.GetItemsByTypeID(typeID, &status)
}

// GetItemsByTypeCode 根据类型编码获取字典项列表
func (s *DictService) GetItemsByTypeCode(code string) ([]model.DictItem, error) {
	status := int8(1)
	return s.dictDAO.GetItemsByTypeCode(code, &status)
}

// GetItemList 获取字典项列表（分页）
func (s *DictService) GetItemList(req DictItemListRequest) ([]model.DictItem, int64, error) {
	return s.dictDAO.GetItemList(req.PageRequest, req.TypeID, req.Keyword, req.Status)
}

// UpdateItem 更新字典项
func (s *DictService) UpdateItem(id uint, req UpdateDictItemRequest) (*model.DictItem, error) {
	item, err := s.dictDAO.GetItemByID(id)
	if err != nil {
		return nil, errors.New("dict item not found")
	}

	if req.Label != "" {
		item.Label = req.Label
	}
	if req.Value != "" {
		item.Value = req.Value
	}
	if req.Sort > 0 {
		item.Sort = req.Sort
	}
	if req.Status != nil {
		item.Status = *req.Status
	}
	if req.Remark != "" {
		item.Remark = req.Remark
	}

	if err := s.dictDAO.UpdateItem(item); err != nil {
		return nil, err
	}

	return item, nil
}

// DeleteItem 删除字典项
func (s *DictService) DeleteItem(id uint) error {
	return s.dictDAO.DeleteItem(id)
}

// ========== 前端使用 ==========

// GetDictData 获取字典数据（根据编码，用于前端下拉选项）
func (s *DictService) GetDictData(code string) ([]model.DictItem, error) {
	return s.GetItemsByTypeCode(code)
}

// GetMultipleDictData 批量获取字典数据
func (s *DictService) GetMultipleDictData(codes []string) (map[string][]model.DictItem, error) {
	result := make(map[string][]model.DictItem)

	for _, code := range codes {
		items, err := s.GetItemsByTypeCode(code)
		if err != nil {
			continue
		}
		result[code] = items
	}

	return result, nil
}

// GetAllDictData 获取所有字典数据（用于前端缓存）
func (s *DictService) GetAllDictData() (map[string][]model.DictItem, error) {
	types, err := s.dictDAO.GetAllTypesWithItems()
	if err != nil {
		return nil, err
	}

	result := make(map[string][]model.DictItem)
	for _, t := range types {
		result[t.Code] = t.Items
	}

	return result, nil
}
