package system

import (
	"context"

	"gorm.io/gorm"

	"github.com/go-admin-kit/services/system/internal/model"
	"github.com/go-admin-kit/services/system/internal/pkg/pagination"
)

type DictDAO struct {
	db *gorm.DB
}

func NewDictDAO(db *gorm.DB) *DictDAO {
	return &DictDAO{db: db}
}

func (d *DictDAO) CreateTypeContext(ctx context.Context, dictType *model.DictType) error {
	return d.dbWithContext(ctx).Create(dictType).Error
}

func (d *DictDAO) GetTypeByIDContext(ctx context.Context, id uint) (*model.DictType, error) {
	var dictType model.DictType
	result := d.dbWithContext(ctx).First(&dictType, id)
	return &dictType, result.Error
}

func (d *DictDAO) GetTypeByCodeContext(ctx context.Context, code string) (*model.DictType, error) {
	var dictType model.DictType
	result := d.dbWithContext(ctx).Where("code = ?", code).First(&dictType)
	return &dictType, result.Error
}

func (d *DictDAO) GetTypeListContext(ctx context.Context, req pagination.PageRequest, keyword string, status *int8) ([]model.DictType, int64, error) {
	var types []model.DictType
	var total int64

	query := d.dbWithContext(ctx).Model(&model.DictType{})
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

func (d *DictDAO) GetAllTypesContext(ctx context.Context, status *int8) ([]model.DictType, error) {
	var types []model.DictType
	query := d.dbWithContext(ctx).Model(&model.DictType{})
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	result := query.Order("created_at DESC").Find(&types)
	return types, result.Error
}

func (d *DictDAO) UpdateTypeContext(ctx context.Context, dictType *model.DictType) error {
	return d.dbWithContext(ctx).Save(dictType).Error
}

func (d *DictDAO) DeleteTypeContext(ctx context.Context, id uint) error {
	return d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("dict_type_id = ?", id).Delete(&model.DictItem{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.DictType{}, id).Error
	})
}

func (d *DictDAO) CreateItemContext(ctx context.Context, item *model.DictItem) error {
	return d.dbWithContext(ctx).Create(item).Error
}

func (d *DictDAO) GetItemByIDContext(ctx context.Context, id uint) (*model.DictItem, error) {
	var item model.DictItem
	result := d.dbWithContext(ctx).First(&item, id)
	return &item, result.Error
}

func (d *DictDAO) GetItemsByTypeIDContext(ctx context.Context, typeID uint, status *int8) ([]model.DictItem, error) {
	var items []model.DictItem
	query := d.dbWithContext(ctx).Where("dict_type_id = ?", typeID)
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	result := query.Order("sort ASC, created_at ASC").Find(&items)
	return items, result.Error
}

func (d *DictDAO) GetItemsByTypeCodeContext(ctx context.Context, code string, status *int8) ([]model.DictItem, error) {
	dictType, err := d.GetTypeByCodeContext(ctx, code)
	if err != nil {
		return nil, err
	}
	return d.GetItemsByTypeIDContext(ctx, dictType.ID, status)
}

// GetItemsByTypeIDsContext loads active items for many dict types in one
// query, grouped by type id. Replaces the per-type loop that made /dicts/all
// and /dicts?codes=… cost one query per code.
func (d *DictDAO) GetItemsByTypeIDsContext(ctx context.Context, typeIDs []uint, status *int8) (map[uint][]model.DictItem, error) {
	grouped := make(map[uint][]model.DictItem, len(typeIDs))
	if len(typeIDs) == 0 {
		return grouped, nil
	}

	var items []model.DictItem
	query := d.dbWithContext(ctx).Where("dict_type_id IN ?", typeIDs)
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	// Ordered by type first so each group comes out in the same
	// (sort, created_at) order the single-type query produces.
	if err := query.Order("dict_type_id ASC, sort ASC, created_at ASC").Find(&items).Error; err != nil {
		return nil, err
	}

	for i := range items {
		grouped[items[i].DictTypeID] = append(grouped[items[i].DictTypeID], items[i])
	}
	return grouped, nil
}

// GetTypesByCodesContext loads dict types for many codes in one query.
func (d *DictDAO) GetTypesByCodesContext(ctx context.Context, codes []string, status *int8) ([]model.DictType, error) {
	if len(codes) == 0 {
		return nil, nil
	}
	var types []model.DictType
	query := d.dbWithContext(ctx).Model(&model.DictType{}).Where("code IN ?", codes)
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	err := query.Find(&types).Error
	return types, err
}

// GetTypesWithItemsByCodesContext resolves code→active items for many codes in
// exactly two queries (types by code, then all their items), regardless of how
// many codes are asked for. Codes with no matching type are absent from the
// result; the caller distinguishes "unknown code" from "known code, no items".
func (d *DictDAO) GetTypesWithItemsByCodesContext(ctx context.Context, codes []string) (map[string][]model.DictItem, error) {
	status := int8(1)
	types, err := d.GetTypesByCodesContext(ctx, codes, &status)
	if err != nil {
		return nil, err
	}
	if len(types) == 0 {
		return map[string][]model.DictItem{}, nil
	}

	typeIDs := make([]uint, 0, len(types))
	for i := range types {
		typeIDs = append(typeIDs, types[i].ID)
	}
	grouped, err := d.GetItemsByTypeIDsContext(ctx, typeIDs, &status)
	if err != nil {
		return nil, err
	}

	result := make(map[string][]model.DictItem, len(types))
	for i := range types {
		items := grouped[types[i].ID]
		if items == nil {
			items = []model.DictItem{}
		}
		result[types[i].Code] = items
	}
	return result, nil
}

func (d *DictDAO) GetItemListContext(ctx context.Context, req pagination.PageRequest, typeID uint, keyword string, status *int8) ([]model.DictItem, int64, error) {
	var items []model.DictItem
	var total int64

	query := d.dbWithContext(ctx).Model(&model.DictItem{}).Where("dict_type_id = ?", typeID)
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

func (d *DictDAO) UpdateItemContext(ctx context.Context, item *model.DictItem) error {
	return d.dbWithContext(ctx).Save(item).Error
}

func (d *DictDAO) DeleteItemContext(ctx context.Context, id uint) error {
	return d.dbWithContext(ctx).Delete(&model.DictItem{}, id).Error
}

func (d *DictDAO) DeleteItemsByTypeIDContext(ctx context.Context, typeID uint) error {
	return d.dbWithContext(ctx).Where("dict_type_id = ?", typeID).Delete(&model.DictItem{}).Error
}

func (d *DictDAO) GetTypeWithItemsContext(ctx context.Context, code string) (*model.DictType, error) {
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

// GetAllTypesWithItemsContext returns every active type with its active items
// in two queries (all types, then all their items) instead of the 1+N loop it
// replaces.
func (d *DictDAO) GetAllTypesWithItemsContext(ctx context.Context) ([]model.DictType, error) {
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
