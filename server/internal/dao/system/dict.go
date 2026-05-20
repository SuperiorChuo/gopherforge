package system

import (
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
)

// DictDAO 字典数据访问对象
type DictDAO struct{}

// ========== 字典类型 ==========

// CreateType 创建字典类型
func (d *DictDAO) CreateType(dictType *model.DictType) error {
	return database.DB.Create(dictType).Error
}

// GetTypeByID 根据ID获取字典类型
func (d *DictDAO) GetTypeByID(id uint) (*model.DictType, error) {
	var dictType model.DictType
	result := database.DB.First(&dictType, id)
	return &dictType, result.Error
}

// GetTypeByCode 根据编码获取字典类型
func (d *DictDAO) GetTypeByCode(code string) (*model.DictType, error) {
	var dictType model.DictType
	result := database.DB.Where("code = ?", code).First(&dictType)
	return &dictType, result.Error
}

// GetTypeList 获取字典类型列表
func (d *DictDAO) GetTypeList(req pagination.PageRequest, keyword string, status *int8) ([]model.DictType, int64, error) {
	var types []model.DictType
	var total int64

	query := database.DB.Model(&model.DictType{})

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

// GetAllTypes 获取所有字典类型
func (d *DictDAO) GetAllTypes(status *int8) ([]model.DictType, error) {
	var types []model.DictType
	query := database.DB.Model(&model.DictType{})
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	result := query.Order("created_at DESC").Find(&types)
	return types, result.Error
}

// UpdateType 更新字典类型
func (d *DictDAO) UpdateType(dictType *model.DictType) error {
	return database.DB.Save(dictType).Error
}

// DeleteType 删除字典类型
func (d *DictDAO) DeleteType(id uint) error {
	// 先删除关联的字典项
	database.DB.Where("dict_type_id = ?", id).Delete(&model.DictItem{})
	// 再删除字典类型
	return database.DB.Delete(&model.DictType{}, id).Error
}

// ========== 字典项 ==========

// CreateItem 创建字典项
func (d *DictDAO) CreateItem(item *model.DictItem) error {
	return database.DB.Create(item).Error
}

// GetItemByID 根据ID获取字典项
func (d *DictDAO) GetItemByID(id uint) (*model.DictItem, error) {
	var item model.DictItem
	result := database.DB.First(&item, id)
	return &item, result.Error
}

// GetItemsByTypeID 根据类型ID获取字典项列表
func (d *DictDAO) GetItemsByTypeID(typeID uint, status *int8) ([]model.DictItem, error) {
	var items []model.DictItem
	query := database.DB.Where("dict_type_id = ?", typeID)
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	result := query.Order("sort ASC, created_at ASC").Find(&items)
	return items, result.Error
}

// GetItemsByTypeCode 根据类型编码获取字典项列表
func (d *DictDAO) GetItemsByTypeCode(code string, status *int8) ([]model.DictItem, error) {
	// 先获取类型
	dictType, err := d.GetTypeByCode(code)
	if err != nil {
		return nil, err
	}
	return d.GetItemsByTypeID(dictType.ID, status)
}

// GetItemList 获取字典项列表（分页）
func (d *DictDAO) GetItemList(req pagination.PageRequest, typeID uint, keyword string, status *int8) ([]model.DictItem, int64, error) {
	var items []model.DictItem
	var total int64

	query := database.DB.Model(&model.DictItem{}).Where("dict_type_id = ?", typeID)

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

// UpdateItem 更新字典项
func (d *DictDAO) UpdateItem(item *model.DictItem) error {
	return database.DB.Save(item).Error
}

// DeleteItem 删除字典项
func (d *DictDAO) DeleteItem(id uint) error {
	return database.DB.Delete(&model.DictItem{}, id).Error
}

// DeleteItemsByTypeID 删除类型下的所有字典项
func (d *DictDAO) DeleteItemsByTypeID(typeID uint) error {
	return database.DB.Where("dict_type_id = ?", typeID).Delete(&model.DictItem{}).Error
}

// GetTypeWithItems 获取字典类型及其字典项
func (d *DictDAO) GetTypeWithItems(code string) (*model.DictType, error) {
	dictType, err := d.GetTypeByCode(code)
	if err != nil {
		return nil, err
	}

	status := int8(1)
	items, err := d.GetItemsByTypeID(dictType.ID, &status)
	if err != nil {
		return nil, err
	}

	dictType.Items = items
	return dictType, nil
}

// GetAllTypesWithItems 获取所有字典类型及其字典项
func (d *DictDAO) GetAllTypesWithItems() ([]model.DictType, error) {
	status := int8(1)
	types, err := d.GetAllTypes(&status)
	if err != nil {
		return nil, err
	}

	for i := range types {
		items, err := d.GetItemsByTypeID(types[i].ID, &status)
		if err != nil {
			continue
		}
		types[i].Items = items
	}

	return types, nil
}
