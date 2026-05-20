package system

import (
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
)

// DepartmentDAO 部门数据访问对象
type DepartmentDAO struct{}

// GetByID 根据ID获取部门
func (d *DepartmentDAO) GetByID(id uint) (*model.Department, error) {
	var dept model.Department
	result := database.DB.First(&dept, id)
	return &dept, result.Error
}

// GetByCode 根据编码获取部门
func (d *DepartmentDAO) GetByCode(code string) (*model.Department, error) {
	var dept model.Department
	result := database.DB.Where("code = ?", code).First(&dept)
	return &dept, result.Error
}

// GetList 获取部门列表（分页）
func (d *DepartmentDAO) GetList(req pagination.PageRequest, keyword string, status *int8) ([]model.Department, int64, error) {
	var depts []model.Department
	var total int64

	query := database.DB.Model(&model.Department{})

	// 关键词搜索
	if keyword != "" {
		query = query.Where("name LIKE ? OR code LIKE ? OR leader LIKE ?",
			"%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}
	// 状态筛选
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	result := query.Scopes(pagination.Paginate(req)).
		Order("parent_id ASC, sort ASC, created_at ASC").
		Find(&depts)

	return depts, total, result.Error
}

// GetAll 获取所有部门
func (d *DepartmentDAO) GetAll(status *int8) ([]model.Department, error) {
	var depts []model.Department
	query := database.DB.Model(&model.Department{})
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	result := query.Order("parent_id ASC, sort ASC, created_at ASC").Find(&depts)
	return depts, result.Error
}

// GetTree 获取部门树
func (d *DepartmentDAO) GetTree(status *int8) ([]model.Department, error) {
	depts, err := d.GetAll(status)
	if err != nil {
		return nil, err
	}
	return buildDepartmentTree(depts, 0), nil
}

// buildDepartmentTree 构建部门树
func buildDepartmentTree(depts []model.Department, parentID uint) []model.Department {
	var tree []model.Department
	for i := range depts {
		if depts[i].ParentID == parentID {
			children := buildDepartmentTree(depts, depts[i].ID)
			if children == nil {
				depts[i].Children = []model.Department{}
			} else {
				depts[i].Children = children
			}
			tree = append(tree, depts[i])
		}
	}
	return tree
}

// Create 创建部门
func (d *DepartmentDAO) Create(dept *model.Department) error {
	return database.DB.Create(dept).Error
}

// Update 更新部门
func (d *DepartmentDAO) Update(dept *model.Department) error {
	return database.DB.Save(dept).Error
}

// Delete 删除部门
func (d *DepartmentDAO) Delete(id uint) error {
	// 检查是否有子部门
	var count int64
	database.DB.Model(&model.Department{}).Where("parent_id = ?", id).Count(&count)
	if count > 0 {
		return ErrDepartmentHasChildren
	}

	// 检查是否有关联用户
	database.DB.Model(&model.User{}).Where("department_id = ?", id).Count(&count)
	if count > 0 {
		return ErrDepartmentHasUsers
	}

	return database.DB.Delete(&model.Department{}, id).Error
}

// GetChildrenIDs 获取所有子部门ID（递归）
func (d *DepartmentDAO) GetChildrenIDs(parentID uint) ([]uint, error) {
	var ids []uint
	depts, err := d.GetAll(nil)
	if err != nil {
		return nil, err
	}
	collectChildrenIDs(depts, parentID, &ids)
	return ids, nil
}

// collectChildrenIDs 递归收集子部门ID
func collectChildrenIDs(depts []model.Department, parentID uint, ids *[]uint) {
	for _, dept := range depts {
		if dept.ParentID == parentID {
			*ids = append(*ids, dept.ID)
			collectChildrenIDs(depts, dept.ID, ids)
		}
	}
}

// 错误定义
type departmentError string

func (e departmentError) Error() string { return string(e) }

const (
	ErrDepartmentHasChildren departmentError = "部门下存在子部门，无法删除"
	ErrDepartmentHasUsers    departmentError = "部门下存在用户，无法删除"
)
