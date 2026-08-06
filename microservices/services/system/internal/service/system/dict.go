package system

import (
	"context"
	"errors"
	"strings"

	systemdao "github.com/go-admin-kit/services/system/internal/dao/system"
	"github.com/go-admin-kit/services/system/internal/model"
	cachepkg "github.com/go-admin-kit/services/system/internal/pkg/cache"
	"github.com/go-admin-kit/services/shared/pkg/pagination"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
)

type DictService struct {
	dictDAO systemdao.DictDAO
	cache   *cachepkg.CacheService
}

// NewDictServiceWithDB builds a DictService backed by an injected database handle.
func NewDictServiceWithDB(db *gorm.DB) DictService {
	return DictService{
		dictDAO: *systemdao.NewDictDAO(db),
		cache:   cachepkg.NewCacheService(),
	}
}

// NewDictServiceWithCache builds a DictService with an explicit cache handle
// (tests inject a dedicated Redis client this way).
func NewDictServiceWithCache(db *gorm.DB, cache *cachepkg.CacheService) DictService {
	return DictService{
		dictDAO: *systemdao.NewDictDAO(db),
		cache:   cache,
	}
}

// cacheService resolves the cache handle, falling back to the shared instance
// so a zero-value DictService still behaves like the injected one.
func (s *DictService) cacheService() *cachepkg.CacheService {
	if s.cache != nil {
		return s.cache
	}
	return cachepkg.NewCacheService()
}

// invalidateDictCache drops every cached dictionary payload. Called after each
// dictionary write; see cache/dict.go for why invalidation is namespace-wide.
func (s *DictService) invalidateDictCache(ctx context.Context) {
	// Best effort: a cache that cannot be reached must not fail the write. The
	// bounded TTL is the backstop.
	_ = s.cacheService().DelAllDictDataContext(ctx)
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

	s.invalidateDictCache(ctx)
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

	s.invalidateDictCache(ctx)
	return dictType, nil
}

func (s *DictService) DeleteTypeContext(ctx context.Context, id uint) error {
	if err := s.dictDAO.DeleteTypeContext(ctx, id); err != nil {
		return err
	}
	s.invalidateDictCache(ctx)
	return nil
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

	s.invalidateDictCache(ctx)
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

	s.invalidateDictCache(ctx)
	return item, nil
}

func (s *DictService) DeleteItemContext(ctx context.Context, id uint) error {
	if err := s.dictDAO.DeleteItemContext(ctx, id); err != nil {
		return err
	}
	s.invalidateDictCache(ctx)
	return nil
}

// GetDictDataContext returns one code's active items, served from cache when
// warm. GET /dicts/:code.
func (s *DictService) GetDictDataContext(ctx context.Context, code string) ([]model.DictItem, error) {
	resolved, err := s.resolveDictCodesContext(ctx, []string{code})
	if err != nil {
		return nil, err
	}
	items, found := resolved[code]
	if !found {
		return nil, ErrDictTypeNotFound
	}
	return items, nil
}

// GetMultipleDictDataContext returns active items for several codes at once.
// GET /dicts?codes=a,b,c. Warm cache costs zero queries; a cold or partial
// cache costs two queries total (types by code, then their items) instead of
// the two-per-code it replaces. Unknown codes are omitted, as before.
func (s *DictService) GetMultipleDictDataContext(ctx context.Context, codes []string) (map[string][]model.DictItem, error) {
	return s.resolveDictCodesContext(ctx, codes)
}

// resolveDictCodesContext is the shared cache-aside path for the by-code
// endpoints. Codes absent from the returned map have no active dict type.
func (s *DictService) resolveDictCodesContext(ctx context.Context, codes []string) (map[string][]model.DictItem, error) {
	wanted := normalizeDictCodes(codes)
	result := make(map[string][]model.DictItem, len(wanted))
	if len(wanted) == 0 {
		return result, nil
	}

	tenantID := tenant.FromContextOrDefault(ctx)
	cache := s.cacheService()

	missing := wanted
	if hits := cache.GetDictCodesContext(ctx, tenantID, wanted); len(hits) > 0 {
		missing = missing[:0:0]
		for _, code := range wanted {
			entry, ok := hits[code]
			if !ok {
				missing = append(missing, code)
				continue
			}
			// A cached miss (Found=false) is an answer too: the code has no
			// active dict type, so it stays out of the result without a query.
			if entry.Found {
				result[code] = entry.Items
			}
		}
	}
	if len(missing) == 0 {
		return result, nil
	}

	fetched, err := s.dictDAO.GetTypesWithItemsByCodesContext(ctx, missing)
	if err != nil {
		return nil, err
	}

	entries := make(map[string]cachepkg.DictEntry, len(missing))
	for _, code := range missing {
		items, found := fetched[code]
		if found {
			result[code] = items
		}
		entries[code] = cachepkg.DictEntry{Found: found, Items: items}
	}
	// Best effort: a cache write failure only costs the next request a query.
	_ = cache.SetDictCodesContext(ctx, tenantID, entries)

	return result, nil
}

// GetAllDictDataContext returns every active code with its active items.
// GET /dicts/all. Two queries on a cold cache (down from 1+N), zero when warm.
func (s *DictService) GetAllDictDataContext(ctx context.Context) (map[string][]model.DictItem, error) {
	tenantID := tenant.FromContextOrDefault(ctx)
	cache := s.cacheService()
	if cached, ok := cache.GetAllDictDataContext(ctx, tenantID); ok {
		return cached, nil
	}

	types, err := s.dictDAO.GetAllTypesWithItemsContext(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[string][]model.DictItem, len(types))
	for _, t := range types {
		result[t.Code] = t.Items
	}

	_ = cache.SetAllDictDataContext(ctx, tenantID, result)
	return result, nil
}

// normalizeDictCodes trims, drops empties and de-duplicates the requested
// codes so a repeated code cannot multiply the cache round trip.
func normalizeDictCodes(codes []string) []string {
	seen := make(map[string]struct{}, len(codes))
	normalized := make([]string, 0, len(codes))
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		if _, dup := seen[code]; dup {
			continue
		}
		seen[code] = struct{}{}
		normalized = append(normalized, code)
	}
	return normalized
}
