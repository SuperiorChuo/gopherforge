package system

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
)

type DictDAO struct {
	db *gorm.DB
}

func NewDictDAO(db *gorm.DB) *DictDAO {
	return &DictDAO{db: db}
}

// Deprecated: use CreateTypeContext instead.
func (d *DictDAO) CreateType(dictType *model.DictType) error {
	return d.CreateTypeContext(context.Background(), dictType)
}

func (d *DictDAO) CreateTypeContext(ctx context.Context, dictType *model.DictType) error {
	return d.dbWithContext(ctx).Create(dictType).Error
}

// Deprecated: use GetTypeByIDContext instead.
func (d *DictDAO) GetTypeByID(id uint) (*model.DictType, error) {
	return d.GetTypeByIDContext(context.Background(), id)
}

func (d *DictDAO) GetTypeByIDContext(ctx context.Context, id uint) (*model.DictType, error) {
	var dictType model.DictType
	result := d.dbWithContext(ctx).First(&dictType, id)
	return &dictType, result.Error
}

// Deprecated: use GetTypeByCodeContext instead.
func (d *DictDAO) GetTypeByCode(code string) (*model.DictType, error) {
	return d.GetTypeByCodeContext(context.Background(), code)
}

func (d *DictDAO) GetTypeByCodeContext(ctx context.Context, code string) (*model.DictType, error) {
	var dictType model.DictType
	result := d.dbWithContext(ctx).Where("code = ?", code).First(&dictType)
	return &dictType, result.Error
}

// Deprecated: use GetTypeListContext instead.
func (d *DictDAO) GetTypeList(req pagination.PageRequest, keyword string, status *int8) ([]model.DictType, int64, error) {
	return d.GetTypeListContext(context.Background(), req, keyword, status)
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

// Deprecated: use GetAllTypesContext instead.
func (d *DictDAO) GetAllTypes(status *int8) ([]model.DictType, error) {
	return d.GetAllTypesContext(context.Background(), status)
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

// Deprecated: use UpdateTypeContext instead.
func (d *DictDAO) UpdateType(dictType *model.DictType) error {
	return d.UpdateTypeContext(context.Background(), dictType)
}

func (d *DictDAO) UpdateTypeContext(ctx context.Context, dictType *model.DictType) error {
	return d.dbWithContext(ctx).Save(dictType).Error
}

// Deprecated: use DeleteTypeContext instead.
func (d *DictDAO) DeleteType(id uint) error {
	return d.DeleteTypeContext(context.Background(), id)
}

func (d *DictDAO) DeleteTypeContext(ctx context.Context, id uint) error {
	return d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("dict_type_id = ?", id).Delete(&model.DictItem{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.DictType{}, id).Error
	})
}

// Deprecated: use CreateItemContext instead.
func (d *DictDAO) CreateItem(item *model.DictItem) error {
	return d.CreateItemContext(context.Background(), item)
}

func (d *DictDAO) CreateItemContext(ctx context.Context, item *model.DictItem) error {
	return d.dbWithContext(ctx).Create(item).Error
}

// Deprecated: use GetItemByIDContext instead.
func (d *DictDAO) GetItemByID(id uint) (*model.DictItem, error) {
	return d.GetItemByIDContext(context.Background(), id)
}

func (d *DictDAO) GetItemByIDContext(ctx context.Context, id uint) (*model.DictItem, error) {
	var item model.DictItem
	result := d.dbWithContext(ctx).First(&item, id)
	return &item, result.Error
}

// Deprecated: use GetItemsByTypeIDContext instead.
func (d *DictDAO) GetItemsByTypeID(typeID uint, status *int8) ([]model.DictItem, error) {
	return d.GetItemsByTypeIDContext(context.Background(), typeID, status)
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

// Deprecated: use GetItemsByTypeCodeContext instead.
func (d *DictDAO) GetItemsByTypeCode(code string, status *int8) ([]model.DictItem, error) {
	return d.GetItemsByTypeCodeContext(context.Background(), code, status)
}

func (d *DictDAO) GetItemsByTypeCodeContext(ctx context.Context, code string, status *int8) ([]model.DictItem, error) {
	dictType, err := d.GetTypeByCodeContext(ctx, code)
	if err != nil {
		return nil, err
	}
	return d.GetItemsByTypeIDContext(ctx, dictType.ID, status)
}

// Deprecated: use GetItemListContext instead.
func (d *DictDAO) GetItemList(req pagination.PageRequest, typeID uint, keyword string, status *int8) ([]model.DictItem, int64, error) {
	return d.GetItemListContext(context.Background(), req, typeID, keyword, status)
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

// Deprecated: use UpdateItemContext instead.
func (d *DictDAO) UpdateItem(item *model.DictItem) error {
	return d.UpdateItemContext(context.Background(), item)
}

func (d *DictDAO) UpdateItemContext(ctx context.Context, item *model.DictItem) error {
	return d.dbWithContext(ctx).Save(item).Error
}

// Deprecated: use DeleteItemContext instead.
func (d *DictDAO) DeleteItem(id uint) error {
	return d.DeleteItemContext(context.Background(), id)
}

func (d *DictDAO) DeleteItemContext(ctx context.Context, id uint) error {
	return d.dbWithContext(ctx).Delete(&model.DictItem{}, id).Error
}

// Deprecated: use DeleteItemsByTypeIDContext instead.
func (d *DictDAO) DeleteItemsByTypeID(typeID uint) error {
	return d.DeleteItemsByTypeIDContext(context.Background(), typeID)
}

func (d *DictDAO) DeleteItemsByTypeIDContext(ctx context.Context, typeID uint) error {
	return d.dbWithContext(ctx).Where("dict_type_id = ?", typeID).Delete(&model.DictItem{}).Error
}

// Deprecated: use GetTypeWithItemsContext instead.
func (d *DictDAO) GetTypeWithItems(code string) (*model.DictType, error) {
	return d.GetTypeWithItemsContext(context.Background(), code)
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

// Deprecated: use GetAllTypesWithItemsContext instead.
func (d *DictDAO) GetAllTypesWithItems() ([]model.DictType, error) {
	return d.GetAllTypesWithItemsContext(context.Background())
}

func (d *DictDAO) GetAllTypesWithItemsContext(ctx context.Context) ([]model.DictType, error) {
	status := int8(1)
	types, err := d.GetAllTypesContext(ctx, &status)
	if err != nil {
		return nil, err
	}

	for i := range types {
		items, err := d.GetItemsByTypeIDContext(ctx, types[i].ID, &status)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			continue
		}
		types[i].Items = items
	}

	return types, nil
}

func (d *DictDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	if d != nil && d.db != nil {
		return d.db.WithContext(ctx)
	}
	return database.DB.WithContext(ctx)
}
