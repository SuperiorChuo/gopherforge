package system

import (
	"context"

	"gorm.io/gorm"

	"github.com/go-admin-kit/services/shared/pkg/pagination"
	localmodel "github.com/go-admin-kit/services/system/internal/model"
)

type DictDAO struct {
	db *gorm.DB
}

func NewDictDAO(db *gorm.DB) *DictDAO {
	return &DictDAO{db: db}
}

func (d *DictDAO) CreateTypeContext(ctx context.Context, dictType *localmodel.DictType) error {
	return d.dbWithContext(ctx).Create(dictType).Error
}

func (d *DictDAO) GetTypeByIDContext(ctx context.Context, id uint) (*localmodel.DictType, error) {
	var dictType localmodel.DictType
	result := d.dbWithContext(ctx).First(&dictType, id)
	return &dictType, result.Error
}

func (d *DictDAO) GetTypeByCodeContext(ctx context.Context, code string) (*localmodel.DictType, error) {
	var dictType localmodel.DictType
	result := d.dbWithContext(ctx).Where("code = ?", code).First(&dictType)
	return &dictType, result.Error
}

func (d *DictDAO) GetTypeListContext(ctx context.Context, req pagination.PageRequest, keyword string, status *int8) ([]localmodel.DictType, int64, error) {
	var types []localmodel.DictType
	var total int64

	query := d.dbWithContext(ctx).Model(&localmodel.DictType{})
	if keyword != "" {
		query = query.Where("name LIKE ? OR code LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	result := query.
		Scopes(pagination.Paginate(req)).
		Order("created_at DESC").
		Find(&types)

	return types, total, result.Error
}

func (d *DictDAO) GetAllTypesContext(ctx context.Context, status *int8) ([]localmodel.DictType, error) {
	var types []localmodel.DictType
	query := d.dbWithContext(ctx).Model(&localmodel.DictType{})
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	result := query.Order("created_at DESC").Find(&types)
	return types, result.Error
}

func (d *DictDAO) UpdateTypeContext(ctx context.Context, dictType *localmodel.DictType) error {
	return d.dbWithContext(ctx).Save(dictType).Error
}

func (d *DictDAO) DeleteTypeContext(ctx context.Context, id uint) error {
	return d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("dict_type_id = ?", id).Delete(&localmodel.DictItem{}).Error; err != nil {
			return err
		}
		return tx.Delete(&localmodel.DictType{}, id).Error
	})
}

func (d *DictDAO) CreateItemContext(ctx context.Context, item *localmodel.DictItem) error {
	return d.dbWithContext(ctx).Create(item).Error
}

func (d *DictDAO) GetItemByIDContext(ctx context.Context, id uint) (*localmodel.DictItem, error) {
	var item localmodel.DictItem
	result := d.dbWithContext(ctx).First(&item, id)
	return &item, result.Error
}

func (d *DictDAO) GetItemsByTypeIDContext(ctx context.Context, typeID uint, status *int8) ([]localmodel.DictItem, error) {
	var items []localmodel.DictItem
	query := d.dbWithContext(ctx).Where("dict_type_id = ?", typeID)
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	result := query.Order("sort ASC, created_at ASC").Find(&items)
	return items, result.Error
}

func (d *DictDAO) GetItemsByTypeCodeContext(ctx context.Context, code string, status *int8) ([]localmodel.DictItem, error) {
	dictType, err := d.GetTypeByCodeContext(ctx, code)
	if err != nil {
		return nil, err
	}
	return d.GetItemsByTypeIDContext(ctx, dictType.ID, status)
}

// GetItemsByTypeIDsContext 用一次查询加载多个字典类型的启用条目，并按类型 ID
// 分组。取代了原先按类型循环、导致 /dicts/all 与 /dicts?codes=… 每个 code 都要
// 一次查询的做法。
func (d *DictDAO) GetItemsByTypeIDsContext(ctx context.Context, typeIDs []uint, status *int8) (map[uint][]localmodel.DictItem, error) {
	grouped := make(map[uint][]localmodel.DictItem, len(typeIDs))
	if len(typeIDs) == 0 {
		return grouped, nil
	}

	var items []localmodel.DictItem
	query := d.dbWithContext(ctx).Where("dict_type_id IN ?", typeIDs)
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	// 先按类型排序，使每个分组保持与单类型查询一致的 (sort, created_at) 顺序。
	if err := query.Order("dict_type_id ASC, sort ASC, created_at ASC").Find(&items).Error; err != nil {
		return nil, err
	}

	for i := range items {
		grouped[items[i].DictTypeID] = append(grouped[items[i].DictTypeID], items[i])
	}
	return grouped, nil
}

// GetTypesByCodesContext 用一次查询加载多个 code 对应的字典类型。
func (d *DictDAO) GetTypesByCodesContext(ctx context.Context, codes []string, status *int8) ([]localmodel.DictType, error) {
	if len(codes) == 0 {
		return nil, nil
	}
	var types []localmodel.DictType
	query := d.dbWithContext(ctx).Model(&localmodel.DictType{}).Where("code IN ?", codes)
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	err := query.Find(&types).Error
	return types, err
}

// GetTypesWithItemsByCodesContext 用恰好两次查询（先按 code 查类型，再查其全部
// 条目）解析多个 code 对应的启用条目，与请求的 code 数量无关。没有匹配类型的 code
// 不会出现在结果中；调用方借此区分"未知 code"与"已知 code 但无条目"。
func (d *DictDAO) GetTypesWithItemsByCodesContext(ctx context.Context, codes []string) (map[string][]localmodel.DictItem, error) {
	status := int8(1)
	types, err := d.GetTypesByCodesContext(ctx, codes, &status)
	if err != nil {
		return nil, err
	}
	if len(types) == 0 {
		return map[string][]localmodel.DictItem{}, nil
	}

	typeIDs := make([]uint, 0, len(types))
	for i := range types {
		typeIDs = append(typeIDs, types[i].ID)
	}
	grouped, err := d.GetItemsByTypeIDsContext(ctx, typeIDs, &status)
	if err != nil {
		return nil, err
	}

	result := make(map[string][]localmodel.DictItem, len(types))
	for i := range types {
		items := grouped[types[i].ID]
		if items == nil {
			items = []localmodel.DictItem{}
		}
		result[types[i].Code] = items
	}
	return result, nil
}

func (d *DictDAO) GetItemListContext(ctx context.Context, req pagination.PageRequest, typeID uint, keyword string, status *int8) ([]localmodel.DictItem, int64, error) {
	var items []localmodel.DictItem
	var total int64

	query := d.dbWithContext(ctx).Model(&localmodel.DictItem{}).Where("dict_type_id = ?", typeID)
	if keyword != "" {
		query = query.Where("label LIKE ? OR value LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	result := query.
		Scopes(pagination.Paginate(req)).
		Order("sort ASC, created_at ASC").
		Find(&items)

	return items, total, result.Error
}

func (d *DictDAO) UpdateItemContext(ctx context.Context, item *localmodel.DictItem) error {
	return d.dbWithContext(ctx).Save(item).Error
}

func (d *DictDAO) DeleteItemContext(ctx context.Context, id uint) error {
	return d.dbWithContext(ctx).Delete(&localmodel.DictItem{}, id).Error
}

func (d *DictDAO) DeleteItemsByTypeIDContext(ctx context.Context, typeID uint) error {
	return d.dbWithContext(ctx).Where("dict_type_id = ?", typeID).Delete(&localmodel.DictItem{}).Error
}

func (d *DictDAO) GetTypeWithItemsContext(ctx context.Context, code string) (*localmodel.DictType, error) {
	dictType, err := d.GetTypeByCodeContext(ctx, code)
	if err != nil {
		return nil, err
	}

	status := int8(1)
	items, err := d.GetItemsByTypeIDContext(ctx, dictType.ID, &status)
	if err != nil {
		return nil, err
	}

	dictType.Items = items
	return dictType, nil
}

// GetAllTypesWithItemsContext 用两次查询（先查全部类型，再查其全部条目）返回
// 所有启用类型及其启用条目，取代原先的 1+N 循环。
func (d *DictDAO) GetAllTypesWithItemsContext(ctx context.Context) ([]localmodel.DictType, error) {
	status := int8(1)
	types, err := d.GetAllTypesContext(ctx, &status)
	if err != nil {
		return nil, err
	}
	if len(types) == 0 {
		return types, nil
	}

	typeIDs := make([]uint, 0, len(types))
	for i := range types {
		typeIDs = append(typeIDs, types[i].ID)
	}
	grouped, err := d.GetItemsByTypeIDsContext(ctx, typeIDs, &status)
	if err != nil {
		return nil, err
	}

	for i := range types {
		types[i].Items = grouped[types[i].ID]
	}
	return types, nil
}

func (d *DictDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}
