package system

import (
	"context"
	"time"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
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
	if d != nil && d.db != nil {
		return d.db.WithContext(ctx)
	}
	return database.DB.WithContext(ctx)
}

// Deprecated: use CreateContext instead.
func (d *FileDAO) Create(file *model.File) error {
	return d.CreateContext(context.Background(), file)
}

func (d *FileDAO) CreateContext(ctx context.Context, file *model.File) error {
	return d.dbWithContext(ctx).Create(file).Error
}

// Deprecated: use GetByIDContext instead.
func (d *FileDAO) GetByID(id uint) (*model.File, error) {
	return d.GetByIDContext(context.Background(), id)
}

func (d *FileDAO) GetByIDContext(ctx context.Context, id uint) (*model.File, error) {
	var file model.File
	result := d.dbWithContext(authz.DisableDataScope(ctx)).First(&file, id)
	return &file, result.Error
}

// Deprecated: use GetByIDInScopeContext instead.
func (d *FileDAO) GetByIDInScope(id uint, dataScope authz.UserDataScope) (*model.File, error) {
	return d.GetByIDInScopeContext(context.Background(), id, dataScope)
}

func (d *FileDAO) GetByIDInScopeContext(ctx context.Context, id uint, dataScope authz.UserDataScope) (*model.File, error) {
	var file model.File
	query := d.dbWithContext(authz.EnableDataScope(ctx, dataScope)).Model(&model.File{})
	result := query.Where("id = ?", id).First(&file)
	return &file, result.Error
}

// Deprecated: use GetByHashContext instead.
func (d *FileDAO) GetByHash(hash string) (*model.File, error) {
	return d.GetByHashContext(context.Background(), hash)
}

func (d *FileDAO) GetByHashContext(ctx context.Context, hash string) (*model.File, error) {
	var file model.File
	result := d.dbWithContext(authz.DisableDataScope(ctx)).Where("hash = ?", hash).First(&file)
	return &file, result.Error
}

// Deprecated: use GetByHashInScopeContext instead.
func (d *FileDAO) GetByHashInScope(hash string, dataScope authz.UserDataScope) (*model.File, error) {
	return d.GetByHashInScopeContext(context.Background(), hash, dataScope)
}

func (d *FileDAO) GetByHashInScopeContext(ctx context.Context, hash string, dataScope authz.UserDataScope) (*model.File, error) {
	var file model.File
	query := d.dbWithContext(authz.EnableDataScope(ctx, dataScope)).Model(&model.File{})
	result := query.Where("hash = ?", hash).First(&file)
	return &file, result.Error
}

// Deprecated: use GetListContext instead.
func (d *FileDAO) GetList(
	req pagination.PageRequest,
	userID *uint,
	fileType, keyword string,
	startTime, endTime *time.Time,
	dataScope authz.UserDataScope,
) ([]model.File, int64, error) {
	return d.GetListContext(context.Background(), req, userID, fileType, keyword, startTime, endTime, dataScope)
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

	query := d.dbWithContext(authz.EnableDataScope(ctx, dataScope)).Model(&model.File{})

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

// Deprecated: use DeleteContext instead.
func (d *FileDAO) Delete(id uint) error {
	return d.DeleteContext(context.Background(), id)
}

func (d *FileDAO) DeleteContext(ctx context.Context, id uint) error {
	return d.dbWithContext(ctx).Delete(&model.File{}, id).Error
}

// Deprecated: use DeleteByIDsContext instead.
func (d *FileDAO) DeleteByIDs(ids []uint) error {
	return d.DeleteByIDsContext(context.Background(), ids)
}

func (d *FileDAO) DeleteByIDsContext(ctx context.Context, ids []uint) error {
	return d.dbWithContext(ctx).Delete(&model.File{}, ids).Error
}

// Deprecated: use GetStatsContext instead.
func (d *FileDAO) GetStats(userID *uint) (*FileStats, error) {
	return d.GetStatsContext(context.Background(), userID)
}

func (d *FileDAO) GetStatsContext(ctx context.Context, userID *uint) (*FileStats, error) {
	return d.getStatsContext(authz.DisableDataScope(ctx), userID)
}

// Deprecated: use GetStatsInScopeContext instead.
func (d *FileDAO) GetStatsInScope(userID *uint, dataScope authz.UserDataScope) (*FileStats, error) {
	return d.GetStatsInScopeContext(context.Background(), userID, dataScope)
}

func (d *FileDAO) GetStatsInScopeContext(ctx context.Context, userID *uint, dataScope authz.UserDataScope) (*FileStats, error) {
	return d.getStatsContext(authz.EnableDataScope(ctx, dataScope), userID)
}

func (d *FileDAO) getStatsContext(ctx context.Context, userID *uint) (*FileStats, error) {
	stats := &FileStats{}

	query := d.dbWithContext(ctx).Model(&model.File{})
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
	query2 := d.dbWithContext(ctx).Model(&model.File{})
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
