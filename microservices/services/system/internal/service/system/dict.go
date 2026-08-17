package system

import (
	"context"
	"errors"
	"strings"

	cachepkg "github.com/go-admin-kit/services/shared/pkg/cache"
	"github.com/go-admin-kit/services/shared/pkg/pagination"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	systemdao "github.com/go-admin-kit/services/system/internal/dao/system"
	localmodel "github.com/go-admin-kit/services/system/internal/model"
	"gorm.io/gorm"
)

type DictService struct {
	dictDAO systemdao.DictDAO
	cache   *cachepkg.CacheService
}

// NewDictServiceWithDB 构建一个基于注入的数据库句柄的 DictService。
func NewDictServiceWithDB(db *gorm.DB) DictService {
	return DictService{
		dictDAO: *systemdao.NewDictDAO(db),
		cache:   cachepkg.NewCacheService(),
	}
}

// NewDictServiceWithCache 构建带显式缓存句柄的 DictService
// （测试借此注入专用 Redis 客户端）。
func NewDictServiceWithCache(db *gorm.DB, cache *cachepkg.CacheService) DictService {
	return DictService{
		dictDAO: *systemdao.NewDictDAO(db),
		cache:   cache,
	}
}

// cacheService 解析缓存句柄，回退到共享实例，
// 使零值 DictService 也能表现得与注入实例一致。
func (s *DictService) cacheService() *cachepkg.CacheService {
	if s.cache != nil {
		return s.cache
	}
	return cachepkg.NewCacheService()
}

// invalidateDictCache 删除所有缓存的字典负载。每次字典写入后调用；
// 关于失效为何按命名空间整体进行，见 cache/dict.go。
func (s *DictService) invalidateDictCache(ctx context.Context) {
	// 尽力而为：缓存不可达不能导致写入失败。有界 TTL 是兜底。
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

func (s *DictService) CreateTypeContext(ctx context.Context, req CreateDictTypeRequest) (*localmodel.DictType, error) {
	_, err := s.dictDAO.GetTypeByCodeContext(ctx, req.Code)
	if err == nil {
		return nil, ErrDictTypeCodeAlreadyExists
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	dictType := &localmodel.DictType{
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

func (s *DictService) GetTypeByIDContext(ctx context.Context, id uint) (*localmodel.DictType, error) {
	dictType, err := s.dictDAO.GetTypeByIDContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDictTypeNotFound
		}
		return nil, err
	}
	return dictType, nil
}

func (s *DictService) GetTypeByCodeContext(ctx context.Context, code string) (*localmodel.DictType, error) {
	dictType, err := s.dictDAO.GetTypeByCodeContext(ctx, code)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDictTypeNotFound
		}
		return nil, err
	}
	return dictType, nil
}

func (s *DictService) GetTypeListContext(ctx context.Context, req DictTypeListRequest) ([]localmodel.DictType, int64, error) {
	return s.dictDAO.GetTypeListContext(ctx, req.PageRequest, req.Keyword, req.Status)
}

func (s *DictService) GetAllTypesContext(ctx context.Context) ([]localmodel.DictType, error) {
	status := int8(1)
	return s.dictDAO.GetAllTypesContext(ctx, &status)
}

func (s *DictService) UpdateTypeContext(ctx context.Context, id uint, req UpdateDictTypeRequest) (*localmodel.DictType, error) {
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

func (s *DictService) CreateItemContext(ctx context.Context, req CreateDictItemRequest) (*localmodel.DictItem, error) {
	_, err := s.dictDAO.GetTypeByIDContext(ctx, req.DictTypeID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDictTypeNotFound
		}
		return nil, err
	}

	item := &localmodel.DictItem{
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

func (s *DictService) GetItemByIDContext(ctx context.Context, id uint) (*localmodel.DictItem, error) {
	item, err := s.dictDAO.GetItemByIDContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDictItemNotFound
		}
		return nil, err
	}
	return item, nil
}

func (s *DictService) GetItemsByTypeIDContext(ctx context.Context, typeID uint) ([]localmodel.DictItem, error) {
	status := int8(1)
	return s.dictDAO.GetItemsByTypeIDContext(ctx, typeID, &status)
}

func (s *DictService) GetItemsByTypeCodeContext(ctx context.Context, code string) ([]localmodel.DictItem, error) {
	dictType, err := s.GetTypeByCodeContext(ctx, code)
	if err != nil {
		return nil, err
	}
	status := int8(1)
	return s.dictDAO.GetItemsByTypeIDContext(ctx, dictType.ID, &status)
}

func (s *DictService) GetItemListContext(ctx context.Context, req DictItemListRequest) ([]localmodel.DictItem, int64, error) {
	return s.dictDAO.GetItemListContext(ctx, req.PageRequest, req.TypeID, req.Keyword, req.Status)
}

func (s *DictService) UpdateItemContext(ctx context.Context, id uint, req UpdateDictItemRequest) (*localmodel.DictItem, error) {
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

// GetDictDataContext 返回单个 code 的启用条目，缓存热时直接命中。
// GET /dicts/:code。
func (s *DictService) GetDictDataContext(ctx context.Context, code string) ([]localmodel.DictItem, error) {
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

// GetMultipleDictDataContext 一次返回多个 code 的启用条目。
// GET /dicts?codes=a,b,c。缓存热时零查询；缓存冷或部分命中时总共两次查询
// （先按 code 查类型，再查其条目），取代原先每个 code 两次查询的做法。
// 未知 code 同样会被省略。
func (s *DictService) GetMultipleDictDataContext(ctx context.Context, codes []string) (map[string][]localmodel.DictItem, error) {
	return s.resolveDictCodesContext(ctx, codes)
}

// resolveDictCodesContext 是按 code 查询各端点的共享旁路缓存路径。
// 返回的 map 中缺失的 code 表示没有启用的字典类型。
func (s *DictService) resolveDictCodesContext(ctx context.Context, codes []string) (map[string][]localmodel.DictItem, error) {
	wanted := normalizeDictCodes(codes)
	result := make(map[string][]localmodel.DictItem, len(wanted))
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
			// 缓存的未命中（Found=false）同样是答案：该 code 没有
			// 启用的字典类型，因此无需查询即可从结果中排除。
			if entry.Found {
				result[code] = dictItemsFromCache(entry.Items)
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
		entries[code] = cachepkg.DictEntry{Found: found, Items: dictItemsToCache(items)}
	}
	// 尽力而为：缓存写入失败只会让下一个请求多一次查询。
	_ = cache.SetDictCodesContext(ctx, tenantID, entries)

	return result, nil
}

// GetAllDictDataContext 返回所有启用 code 及其启用条目。
// GET /dicts/all。缓存冷时两次查询（由 1+N 降下来），热时零查询。
func (s *DictService) GetAllDictDataContext(ctx context.Context) (map[string][]localmodel.DictItem, error) {
	tenantID := tenant.FromContextOrDefault(ctx)
	cache := s.cacheService()
	if cached, ok := cache.GetAllDictDataContext(ctx, tenantID); ok {
		return dictDataFromCache(cached), nil
	}

	types, err := s.dictDAO.GetAllTypesWithItemsContext(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[string][]localmodel.DictItem, len(types))
	for _, t := range types {
		result[t.Code] = t.Items
	}

	_ = cache.SetAllDictDataContext(ctx, tenantID, dictDataToCache(result))
	return result, nil
}

// normalizeDictCodes 对请求的 codes 做去空格、去空值与去重，
// 避免重复 code 成倍增加缓存往返。
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

func dictItemsToCache(items []localmodel.DictItem) []cachepkg.DictItem {
	out := make([]cachepkg.DictItem, len(items))
	for i, item := range items {
		out[i] = cachepkg.DictItem{
			ID: item.ID, DictTypeID: item.DictTypeID, Label: item.Label,
			Value: item.Value, Sort: item.Sort, Status: item.Status, Remark: item.Remark,
		}
	}
	return out
}

func dictItemsFromCache(items []cachepkg.DictItem) []localmodel.DictItem {
	out := make([]localmodel.DictItem, len(items))
	for i, item := range items {
		out[i] = localmodel.DictItem{
			ID: item.ID, DictTypeID: item.DictTypeID, Label: item.Label,
			Value: item.Value, Sort: item.Sort, Status: item.Status, Remark: item.Remark,
		}
	}
	return out
}

func dictDataToCache(data map[string][]localmodel.DictItem) map[string][]cachepkg.DictItem {
	out := make(map[string][]cachepkg.DictItem, len(data))
	for code, items := range data {
		out[code] = dictItemsToCache(items)
	}
	return out
}

func dictDataFromCache(data map[string][]cachepkg.DictItem) map[string][]localmodel.DictItem {
	out := make(map[string][]localmodel.DictItem, len(data))
	for code, items := range data {
		out[code] = dictItemsFromCache(items)
	}
	return out
}
