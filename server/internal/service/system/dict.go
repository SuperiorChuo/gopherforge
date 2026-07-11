package system

import (
	"context"
	"errors"

	systemdao "github.com/go-admin-kit/server/internal/dao/system"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
	"gorm.io/gorm"
)

type DictService struct {
	dictDAO systemdao.DictDAO
}

// NewDictServiceWithDB builds a DictService backed by an injected database handle.
func NewDictServiceWithDB(db *gorm.DB) DictService {
	return DictService{dictDAO: *systemdao.NewDictDAO(db)}
}

type DictTypeListRequest struct {
	pagination.PageRequest
	Keyword string `form:"keyword" json:"keyword"`
	Status  *int8  `form:"status" json:"status"`
}

type CreateDictTypeRequest struct {
	Name        string `json:"name" binding:"required"`
	Code        string `json:"code" binding:"required"`
	Description string `json:"description"`
	Status      int8   `json:"status"`
}

type UpdateDictTypeRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      *int8  `json:"status"`
}

type DictItemListRequest struct {
	pagination.PageRequest
	TypeID  uint   `form:"type_id" json:"type_id" binding:"required"`
	Keyword string `form:"keyword" json:"keyword"`
	Status  *int8  `form:"status" json:"status"`
}

type CreateDictItemRequest struct {
	DictTypeID uint   `json:"dict_type_id" binding:"required"`
	Label      string `json:"label" binding:"required"`
	Value      string `json:"value" binding:"required"`
	Sort       int    `json:"sort"`
	Status     int8   `json:"status"`
	Remark     string `json:"remark"`
}

type UpdateDictItemRequest struct {
	Label  string `json:"label"`
	Value  string `json:"value"`
	Sort   int    `json:"sort"`
	Status *int8  `json:"status"`
	Remark string `json:"remark"`
}

var (
	ErrDictTypeCodeAlreadyExists = errors.New("dict type code already exists")
	ErrDictTypeNotFound          = errors.New("dict type not found")
	ErrDictItemNotFound          = errors.New("dict item not found")
)

func (s *DictService) CreateTypeContext(ctx context.Context, req CreateDictTypeRequest) (*model.DictType, error) {
	_, err := s.dictDAO.GetTypeByCodeContext(ctx, req.Code)
	if err == nil {
		return nil, ErrDictTypeCodeAlreadyExists
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
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

	if err := s.dictDAO.CreateTypeContext(ctx, dictType); err != nil {
		return nil, err
	}

	return dictType, nil
}

func (s *DictService) GetTypeByIDContext(ctx context.Context, id uint) (*model.DictType, error) {
	dictType, err := s.dictDAO.GetTypeByIDContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDictTypeNotFound
		}
		return nil, err
	}
	return dictType, nil
}

func (s *DictService) GetTypeByCodeContext(ctx context.Context, code string) (*model.DictType, error) {
	dictType, err := s.dictDAO.GetTypeByCodeContext(ctx, code)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDictTypeNotFound
		}
		return nil, err
	}
	return dictType, nil
}

func (s *DictService) GetTypeListContext(ctx context.Context, req DictTypeListRequest) ([]model.DictType, int64, error) {
	return s.dictDAO.GetTypeListContext(ctx, req.PageRequest, req.Keyword, req.Status)
}

func (s *DictService) GetAllTypesContext(ctx context.Context) ([]model.DictType, error) {
	status := int8(1)
	return s.dictDAO.GetAllTypesContext(ctx, &status)
}

func (s *DictService) UpdateTypeContext(ctx context.Context, id uint, req UpdateDictTypeRequest) (*model.DictType, error) {
	dictType, err := s.dictDAO.GetTypeByIDContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDictTypeNotFound
		}
		return nil, err
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

	if err := s.dictDAO.UpdateTypeContext(ctx, dictType); err != nil {
		return nil, err
	}

	return dictType, nil
}

func (s *DictService) DeleteTypeContext(ctx context.Context, id uint) error {
	return s.dictDAO.DeleteTypeContext(ctx, id)
}

func (s *DictService) CreateItemContext(ctx context.Context, req CreateDictItemRequest) (*model.DictItem, error) {
	_, err := s.dictDAO.GetTypeByIDContext(ctx, req.DictTypeID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDictTypeNotFound
		}
		return nil, err
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

	if err := s.dictDAO.CreateItemContext(ctx, item); err != nil {
		return nil, err
	}

	return item, nil
}

func (s *DictService) GetItemByIDContext(ctx context.Context, id uint) (*model.DictItem, error) {
	item, err := s.dictDAO.GetItemByIDContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDictItemNotFound
		}
		return nil, err
	}
	return item, nil
}

func (s *DictService) GetItemsByTypeIDContext(ctx context.Context, typeID uint) ([]model.DictItem, error) {
	status := int8(1)
	return s.dictDAO.GetItemsByTypeIDContext(ctx, typeID, &status)
}

func (s *DictService) GetItemsByTypeCodeContext(ctx context.Context, code string) ([]model.DictItem, error) {
	dictType, err := s.GetTypeByCodeContext(ctx, code)
	if err != nil {
		return nil, err
	}
	status := int8(1)
	return s.dictDAO.GetItemsByTypeIDContext(ctx, dictType.ID, &status)
}

func (s *DictService) GetItemListContext(ctx context.Context, req DictItemListRequest) ([]model.DictItem, int64, error) {
	return s.dictDAO.GetItemListContext(ctx, req.PageRequest, req.TypeID, req.Keyword, req.Status)
}

func (s *DictService) UpdateItemContext(ctx context.Context, id uint, req UpdateDictItemRequest) (*model.DictItem, error) {
	item, err := s.dictDAO.GetItemByIDContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDictItemNotFound
		}
		return nil, err
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

	if err := s.dictDAO.UpdateItemContext(ctx, item); err != nil {
		return nil, err
	}

	return item, nil
}

func (s *DictService) DeleteItemContext(ctx context.Context, id uint) error {
	return s.dictDAO.DeleteItemContext(ctx, id)
}

func (s *DictService) GetDictDataContext(ctx context.Context, code string) ([]model.DictItem, error) {
	return s.GetItemsByTypeCodeContext(ctx, code)
}

func (s *DictService) GetMultipleDictDataContext(ctx context.Context, codes []string) (map[string][]model.DictItem, error) {
	result := make(map[string][]model.DictItem)
	for _, code := range codes {
		items, err := s.GetItemsByTypeCodeContext(ctx, code)
		if err != nil {
			if isContextError(err) {
				return nil, err
			}
			if !errors.Is(err, ErrDictTypeNotFound) {
				return nil, err
			}
			continue
		}
		result[code] = items
	}
	return result, nil
}

func (s *DictService) GetAllDictDataContext(ctx context.Context) (map[string][]model.DictItem, error) {
	types, err := s.dictDAO.GetAllTypesWithItemsContext(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[string][]model.DictItem)
	for _, t := range types {
		result[t.Code] = t.Items
	}

	return result, nil
}
