package system

import (
	"context"
	"time"

	localmodel "github.com/go-admin-kit/services/file/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/authz"
	"github.com/go-admin-kit/services/shared/pkg/pagination"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
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

func (d *FileDAO) CreateContext(ctx context.Context, file *localmodel.File) error {
	if file != nil && file.TenantID == 0 {
		file.TenantID = tenant.IDFromContext(ctx)
	}
	return d.dbWithContext(ctx).Create(file).Error
}

func (d *FileDAO) GetByIDContext(ctx context.Context, id uint) (*localmodel.File, error) {
	var file localmodel.File
	result := tenant.ApplyFilter(d.dbWithContext(authz.DisableDataScope(ctx)), ctx).First(&file, id)
	return &file, result.Error
}

func (d *FileDAO) GetByIDInScopeContext(ctx context.Context, id uint, dataScope authz.UserDataScope) (*localmodel.File, error) {
	var file localmodel.File
	query := tenant.ApplyFilter(d.dbWithContext(authz.EnableDataScope(ctx, dataScope)).Model(&localmodel.File{}), ctx)
	result := query.Where("id = ?", id).First(&file)
	return &file, result.Error
}

func (d *FileDAO) GetAvatarByPublicTokenContext(ctx context.Context, token string) (*localmodel.File, error) {
	var file localmodel.File
	result := d.dbWithContext(tenant.DisableScope(authz.DisableDataScope(ctx))).
		Where("public_token = ? AND purpose = ?", token, "avatar").
		First(&file)
	return &file, result.Error
}

func (d *FileDAO) CreateAvatarContext(ctx context.Context, file *localmodel.File, publicToken string) error {
	if file != nil && file.TenantID == 0 {
		file.TenantID = tenant.IDFromContext(ctx)
	}
	now := time.Now()
	return d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := tx.Raw(`
			INSERT INTO files (
				tenant_id, user_id, file_name, file_path, file_size, image_width, image_height,
				thumbnail_path, thumbnail_url, thumbnail_width, thumbnail_height, file_type,
				mime_type, extension, storage_type, url, hash, purpose, public_token, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'avatar', ?, ?, ?)
			RETURNING id`,
			file.TenantID, file.UserID, file.FileName, file.FilePath, file.FileSize, file.ImageWidth, file.ImageHeight,
			file.ThumbnailPath, file.ThumbnailURL, file.ThumbnailWidth, file.ThumbnailHeight, file.FileType,
			file.MimeType, file.Extension, file.StorageType, file.URL, file.Hash, publicToken, now, now,
		).Row()
		if err := row.Scan(&file.ID); err != nil {
			return err
		}
		file.CreatedAt = now
		file.UpdatedAt = now
		return nil
	})
}

func (d *FileDAO) GetByHashContext(ctx context.Context, hash string) (*localmodel.File, error) {
	var file localmodel.File
	result := tenant.ApplyFilter(d.dbWithContext(authz.DisableDataScope(ctx)), ctx).Where("hash = ?", hash).First(&file)
	return &file, result.Error
}

func (d *FileDAO) GetByHashInScopeContext(ctx context.Context, hash string, dataScope authz.UserDataScope) (*localmodel.File, error) {
	var file localmodel.File
	query := tenant.ApplyFilter(d.dbWithContext(authz.EnableDataScope(ctx, dataScope)).Model(&localmodel.File{}), ctx)
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
) ([]localmodel.File, int64, error) {
	var files []localmodel.File
	var total int64

	query := tenant.ApplyFilter(d.dbWithContext(authz.EnableDataScope(ctx, dataScope)).Model(&localmodel.File{}), ctx)

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
	return tenant.ApplyFilter(d.dbWithContext(ctx), ctx).Delete(&localmodel.File{}, id).Error
}

func (d *FileDAO) DeleteByIDsContext(ctx context.Context, ids []uint) error {
	return tenant.ApplyFilter(d.dbWithContext(ctx), ctx).Delete(&localmodel.File{}, ids).Error
}

func (d *FileDAO) ListOtherAvatarsContext(ctx context.Context, userID uint, keepTokens []string) ([]localmodel.File, error) {
	var files []localmodel.File
	query := tenant.ApplyFilter(d.dbWithContext(ctx).Model(&localmodel.File{}), ctx).
		Where("user_id = ? AND purpose = ?", userID, "avatar")
	if len(keepTokens) > 0 {
		query = query.Where("public_token NOT IN ?", keepTokens)
	}
	if err := query.Find(&files).Error; err != nil {
		return nil, err
	}
	return files, nil
}

// CountByFilePathExcludingIDContext counts storage references without tenant filter so
// physical cleanup remains correct if paths are ever shared (tenant.DisableScope
// 显式绕过 GORM 租户兜底插件，保持跨租户引用计数语义).
func (d *FileDAO) CountByFilePathExcludingIDContext(ctx context.Context, storageType, filePath string, excludedID uint) (int64, error) {
	var count int64
	err := d.dbWithContext(tenant.DisableScope(authz.DisableDataScope(ctx))).
		Model(&localmodel.File{}).
		Where("storage_type = ? AND file_path = ? AND id <> ?", storageType, filePath, excludedID).
		Count(&count).Error
	return count, err
}

func (d *FileDAO) CountByThumbnailPathExcludingIDContext(ctx context.Context, storageType, thumbnailPath string, excludedID uint) (int64, error) {
	var count int64
	err := d.dbWithContext(tenant.DisableScope(authz.DisableDataScope(ctx))).
		Model(&localmodel.File{}).
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

	query := tenant.ApplyFilter(d.dbWithContext(ctx).Model(&localmodel.File{}), ctx)
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
	query2 := tenant.ApplyFilter(d.dbWithContext(ctx).Model(&localmodel.File{}), ctx)
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
	// StorageQuotaMB 当前租户存储配额（0=不限），文件页用量展示用。
	// nil = 查询失败（前端显示错误态）；非 nil 且值 0 = 不限。
	StorageQuotaMB *int64 `json:"storage_quota_mb"`
}

type TypeStat struct {
	Count int64 `json:"count"`
	Size  int64 `json:"size"`
}

// StorageQuota holds the tenant's storage limit and current usage.
type StorageQuota struct {
	QuotaMB  int64 `json:"quota_mb"`
	UsedByte int64 `json:"used_bytes"`
}

// GetTenantStorageQuotaContext returns the current tenant's storage quota
// (MB, 0 = unlimited) and their used bytes. The quota column lives on
// tenant_packages (0 = no limit).
func (d *FileDAO) GetTenantStorageQuotaContext(ctx context.Context, tenantID uint) (StorageQuota, error) {
	var quotaMB int64
	err := d.dbWithContext(ctx).Raw(
		"SELECT COALESCE(storage_quota_mb, 0) FROM tenant_packages WHERE id = ?", tenantID,
	).Scan(&quotaMB).Error
	if err != nil {
		return StorageQuota{}, err
	}
	var used int64
	err = d.dbWithContext(ctx).Model(&localmodel.File{}).
		Where("tenant_id = ?", tenantID).
		Select("COALESCE(SUM(file_size), 0)").Scan(&used).Error
	if err != nil {
		return StorageQuota{}, err
	}
	return StorageQuota{QuotaMB: quotaMB, UsedByte: used}, nil
}
