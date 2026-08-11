package system

import (
	"context"

	"gorm.io/gorm"

	localmodel "github.com/go-admin-kit/services/system/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/pagination"
)

// ErrCodeDAO 错误码表数据访问层。
type ErrCodeDAO struct {
	db *gorm.DB
}

func NewErrCodeDAO(db *gorm.DB) *ErrCodeDAO {
	return &ErrCodeDAO{db: db}
}

func (d *ErrCodeDAO) CreateContext(ctx context.Context, errorCode *localmodel.ErrorCode) error {
	return d.dbWithContext(ctx).Create(errorCode).Error
}

func (d *ErrCodeDAO) GetByIDContext(ctx context.Context, id uint) (*localmodel.ErrorCode, error) {
	var errorCode localmodel.ErrorCode
	result := d.dbWithContext(ctx).First(&errorCode, id)
	return &errorCode, result.Error
}

func (d *ErrCodeDAO) GetByCodeContext(ctx context.Context, code string) (*localmodel.ErrorCode, error) {
	var errorCode localmodel.ErrorCode
	result := d.dbWithContext(ctx).Where("code = ?", code).First(&errorCode)
	return &errorCode, result.Error
}

// GetListContext 分页查询错误码，支持 code/文案/备注关键词、状态、来源筛选。
func (d *ErrCodeDAO) GetListContext(ctx context.Context, req pagination.PageRequest, keyword, scope string, status *int8) ([]localmodel.ErrorCode, int64, error) {
	var codes []localmodel.ErrorCode
	var total int64

	query := d.dbWithContext(ctx).Model(&localmodel.ErrorCode{})
	if keyword != "" {
		query = query.Where("code LIKE ? OR message LIKE ? OR memo LIKE ?", "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}
	if scope != "" {
		query = query.Where("scope = ?", scope)
	}
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	result := query.
		Scopes(pagination.Paginate(req)).
		Order("code ASC").
		Find(&codes)

	return codes, total, result.Error
}

// GetAllEnabledContext 返回全量启用的错误码（供各服务/前端整包拉取做本地缓存）。
func (d *ErrCodeDAO) GetAllEnabledContext(ctx context.Context) ([]localmodel.ErrorCode, error) {
	var codes []localmodel.ErrorCode
	result := d.dbWithContext(ctx).
		Where("status = ?", int8(1)).
		Order("code ASC").
		Find(&codes)
	return codes, result.Error
}

func (d *ErrCodeDAO) UpdateContext(ctx context.Context, errorCode *localmodel.ErrorCode) error {
	return d.dbWithContext(ctx).Save(errorCode).Error
}

func (d *ErrCodeDAO) DeleteContext(ctx context.Context, id uint) error {
	return d.dbWithContext(ctx).Delete(&localmodel.ErrorCode{}, id).Error
}

func (d *ErrCodeDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}
