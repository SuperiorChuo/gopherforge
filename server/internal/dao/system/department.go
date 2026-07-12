package system

import (
	"context"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
	"gorm.io/gorm"
)

type DepartmentDAO struct {
	db *gorm.DB
}

func NewDepartmentDAO(db *gorm.DB) *DepartmentDAO {
	return &DepartmentDAO{db: db}
}

func (d *DepartmentDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

func (d *DepartmentDAO) GetByIDContext(ctx context.Context, id uint) (*model.Department, error) {
	var dept model.Department
	result := d.dbWithContext(ctx).First(&dept, id)
	return &dept, result.Error
}

func (d *DepartmentDAO) GetByCodeContext(ctx context.Context, code string) (*model.Department, error) {
	var dept model.Department
	result := d.dbWithContext(ctx).Where("code = ?", code).First(&dept)
	return &dept, result.Error
}

func (d *DepartmentDAO) GetListContext(ctx context.Context, req pagination.PageRequest, keyword string, status *int8) ([]model.Department, int64, error) {
	var depts []model.Department
	var total int64

	query := d.dbWithContext(ctx).Model(&model.Department{})
	if keyword != "" {
		query = query.Where("name LIKE ? OR code LIKE ? OR leader LIKE ?",
			"%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	result := query.Scopes(pagination.Paginate(req)).
		Order("parent_id ASC, sort ASC, created_at ASC").
		Find(&depts)

	return depts, total, result.Error
}

func (d *DepartmentDAO) GetAllContext(ctx context.Context, status *int8) ([]model.Department, error) {
	var depts []model.Department
	query := d.dbWithContext(ctx).Model(&model.Department{})
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	result := query.Order("parent_id ASC, sort ASC, created_at ASC").Find(&depts)
	return depts, result.Error
}

func (d *DepartmentDAO) GetTreeContext(ctx context.Context, status *int8) ([]model.Department, error) {
	depts, err := d.GetAllContext(ctx, status)
	if err != nil {
		return nil, err
	}
	return buildDepartmentTree(depts, 0), nil
}

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

func (d *DepartmentDAO) CreateContext(ctx context.Context, dept *model.Department) error {
	return d.dbWithContext(ctx).Create(dept).Error
}

func (d *DepartmentDAO) UpdateContext(ctx context.Context, dept *model.Department) error {
	return d.dbWithContext(ctx).Save(dept).Error
}

func (d *DepartmentDAO) DeleteContext(ctx context.Context, id uint) error {
	db := d.dbWithContext(ctx)

	var count int64
	if err := db.Model(&model.Department{}).Where("parent_id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrDepartmentHasChildren
	}

	if err := db.Model(&model.User{}).Where("department_id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrDepartmentHasUsers
	}

	return db.Delete(&model.Department{}, id).Error
}

func (d *DepartmentDAO) GetChildrenIDsContext(ctx context.Context, parentID uint) ([]uint, error) {
	var ids []uint
	depts, err := d.GetAllContext(ctx, nil)
	if err != nil {
		return nil, err
	}
	collectChildrenIDs(depts, parentID, &ids)
	return ids, nil
}

func collectChildrenIDs(depts []model.Department, parentID uint, ids *[]uint) {
	for _, dept := range depts {
		if dept.ParentID == parentID {
			*ids = append(*ids, dept.ID)
			collectChildrenIDs(depts, dept.ID, ids)
		}
	}
}

type departmentError string

func (e departmentError) Error() string { return string(e) }

const (
	ErrDepartmentHasChildren departmentError = "department has child departments"
	ErrDepartmentHasUsers    departmentError = "department has users"
)
