package system

import (
	"context"
	"time"

	"github.com/go-admin-kit/services/file/internal/model"
	"github.com/go-admin-kit/services/file/internal/pkg/authz"
	"github.com/go-admin-kit/services/file/internal/pkg/pagination"
	"github.com/go-admin-kit/services/file/internal/pkg/tenant"
	"gorm.io/gorm"
)

type FileDAO struct {
	db *gorm.DB
}

func NewFileDAO(db *gorm.DB) *FileDAO {
	return &FileDAO{db: db}
}

func (d *FileDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

func (d *FileDAO) CreateContext(ctx context.Context, file *model.File) error {
	if file != nil && file.TenantID == 0 {
		file.TenantID = tenant.IDFromContext(ctx)
	}
	return d.dbWithContext(ctx).Create(file).Error
}

func (d *FileDAO) GetByIDContext(ctx context.Context, id uint) (*model.File, error) {
	var file model.File
	result := tenant.ApplyFilter(d.dbWithContext(authz.DisableDataScope(ctx)), ctx).First(&file, id)
	return &file, result.Error
}

func (d *FileDAO) GetByIDInScopeContext(ctx context.Context, id uint, dataScope authz.UserDataScope) (*model.File, error) {
	var file model.File
	query := tenant.ApplyFilter(d.dbWithContext(authz.EnableDataScope(ctx, dataScope)).Model(&model.File{}), ctx)
	result := query.Where("id = ?", id).First(&file)
	return &file, result.Error
}

func (d *FileDAO) GetByHashContext(ctx context.Context, hash string) (*model.File, error) {
	var file model.File
	result := tenant.ApplyFilter(d.dbWithContext(authz.DisableDataScope(ctx)), ctx).Where("hash = ?", hash).First(&file)
	return &file, result.Error
}

func (d *FileDAO) GetByHashInScopeContext(ctx context.Context, hash string, dataScope authz.UserDataScope) (*model.File, error) {
	var file model.File
	query := tenant.ApplyFilter(d.dbWithContext(authz.EnableDataScope(ctx, dataScope)).Model(&model.File{}), ctx)
	result := query.Where("hash = ?", hash).First(&file)
	return &file, result.Error
}

func (d *FileDAO) GetListContext(
	ctx context.Context,
	req pagination.PageRequest,
	userID *uint,
	fileType, keyword string,
	startTime, endTime *time.Time,
	dataScope authz.UserDataScope,
) ([]model.File, int64, error) {
	var files []model.File
	var total int64

	query := tenant.ApplyFilter(d.dbWithContext(authz.EnableDataScope(ctx, dataScope)).Model(&model.File{}), ctx)

	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}
	if fileType != "" {
		query = query.Where("file_type = ?", fileType)
	}
	if keyword != "" {
		query = query.Where("file_name LIKE ?", "%"+keyword+"%")
	}
	if startTime != nil {
		query = query.Where("created_at >= ?", *startTime)
	}
	if endTime != nil {
		query = query.Where("created_at <= ?", *endTime)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	result := query.
		Scopes(pagination.Paginate(req)).
		Order("created_at DESC").
		Find(&files)

	return files, total, result.Error
}

func (d *FileDAO) DeleteContext(ctx context.Context, id uint) error {
	return tenant.ApplyFilter(d.dbWithContext(ctx), ctx).Delete(&model.File{}, id).Error
}

func (d *FileDAO) DeleteByIDsContext(ctx context.Context, ids []uint) error {
	return tenant.ApplyFilter(d.dbWithContext(ctx), ctx).Delete(&model.File{}, ids).Error
}

// CountByFilePathExcludingIDContext counts storage references without tenant filter so
// physical cleanup remains correct if paths are ever shared.
func (d *FileDAO) CountByFilePathExcludingIDContext(ctx context.Context, storageType, filePath string, excludedID uint) (int64, error) {
	var count int64
	err := d.dbWithContext(authz.DisableDataScope(ctx)).
		Model(&model.File{}).
		Where("storage_type = ? AND file_path = ? AND id <> ?", storageType, filePath, excludedID).
		Count(&count).Error
	return count, err
}

func (d *FileDAO) CountByThumbnailPathExcludingIDContext(ctx context.Context, storageType, thumbnailPath string, excludedID uint) (int64, error) {
	var count int64
	err := d.dbWithContext(authz.DisableDataScope(ctx)).
		Model(&model.File{}).
		Where("storage_type = ? AND thumbnail_path = ? AND id <> ?", storageType, thumbnailPath, excludedID).
		Count(&count).Error
	return count, err
}

func (d *FileDAO) GetStatsContext(ctx context.Context, userID *uint) (*FileStats, error) {
	return d.getStatsContext(authz.DisableDataScope(ctx), userID)
}

func (d *FileDAO) GetStatsInScopeContext(ctx context.Context, userID *uint, dataScope authz.UserDataScope) (*FileStats, error) {
	return d.getStatsContext(authz.EnableDataScope(ctx, dataScope), userID)
}

func (d *FileDAO) getStatsContext(ctx context.Context, userID *uint) (*FileStats, error) {
	stats := &FileStats{}

	query := tenant.ApplyFilter(d.dbWithContext(ctx).Model(&model.File{}), ctx)
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}

	var result struct {
		Count     int64 `json:"count"`
		TotalSize int64 `json:"total_size"`
	}
	if err := query.Select("COUNT(*) as count, COALESCE(SUM(file_size), 0) as total_size").Scan(&result).Error; err != nil {
		return nil, err
	}
	stats.Total = result.Count
	stats.TotalSize = result.TotalSize

	var typeStats []struct {
		FileType string `json:"file_type"`
		Count    int64  `json:"count"`
		Size     int64  `json:"size"`
	}
	query2 := tenant.ApplyFilter(d.dbWithContext(ctx).Model(&model.File{}), ctx)
	if userID != nil {
		query2 = query2.Where("user_id = ?", *userID)
	}
	if err := query2.Select("file_type, COUNT(*) as count, COALESCE(SUM(file_size), 0) as size").
		Group("file_type").
		Find(&typeStats).Error; err != nil {
		return nil, err
	}

	stats.ByType = make(map[string]TypeStat)
	for _, s := range typeStats {
		stats.ByType[s.FileType] = TypeStat{Count: s.Count, Size: s.Size}
	}

	return stats, nil
}

type FileStats struct {
	Total     int64               `json:"total"`
	TotalSize int64               `json:"total_size"`
	ByType    map[string]TypeStat `json:"by_type"`
}

type TypeStat struct {
	Count int64 `json:"count"`
	Size  int64 `json:"size"`
}
