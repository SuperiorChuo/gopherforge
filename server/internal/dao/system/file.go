package system

import (
	"time"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
)

// FileDAO 文件数据访问对象
type FileDAO struct{}

// Create 创建文件记录
func (d *FileDAO) Create(file *model.File) error {
	return database.DB.Create(file).Error
}

// GetByID 根据ID获取文件
func (d *FileDAO) GetByID(id uint) (*model.File, error) {
	var file model.File
	result := database.DB.First(&file, id)
	return &file, result.Error
}

// GetByIDInScope 根据ID在数据权限范围内获取文件。
func (d *FileDAO) GetByIDInScope(id uint, dataScope authz.UserDataScope) (*model.File, error) {
	var file model.File
	query := database.DB.Model(&model.File{})
	query = authz.ApplyOwnerScope(query, dataScope, "user_id")
	result := query.Where("id = ?", id).First(&file)
	return &file, result.Error
}

// GetByHash 根据哈希获取文件（用于秒传）
func (d *FileDAO) GetByHash(hash string) (*model.File, error) {
	var file model.File
	result := database.DB.Where("hash = ?", hash).First(&file)
	return &file, result.Error
}

// GetByHashInScope 根据哈希在数据权限范围内获取文件。
func (d *FileDAO) GetByHashInScope(hash string, dataScope authz.UserDataScope) (*model.File, error) {
	var file model.File
	query := database.DB.Model(&model.File{})
	query = authz.ApplyOwnerScope(query, dataScope, "user_id")
	result := query.Where("hash = ?", hash).First(&file)
	return &file, result.Error
}

// GetList 获取文件列表
func (d *FileDAO) GetList(
	req pagination.PageRequest,
	userID *uint,
	fileType, keyword string,
	startTime, endTime *time.Time,
	dataScope authz.UserDataScope,
) ([]model.File, int64, error) {
	var files []model.File
	var total int64

	query := database.DB.Model(&model.File{})
	query = authz.ApplyOwnerScope(query, dataScope, "user_id")

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

// Delete 删除文件记录
func (d *FileDAO) Delete(id uint) error {
	return database.DB.Delete(&model.File{}, id).Error
}

// DeleteByIDs 批量删除文件记录
func (d *FileDAO) DeleteByIDs(ids []uint) error {
	return database.DB.Delete(&model.File{}, ids).Error
}

// GetStats 获取文件统计
func (d *FileDAO) GetStats(userID *uint) (*FileStats, error) {
	return d.getStats(userID, authz.UserDataScope{Scope: authz.DataScopeAll})
}

// GetStatsInScope 获取数据权限范围内的文件统计
func (d *FileDAO) GetStatsInScope(userID *uint, dataScope authz.UserDataScope) (*FileStats, error) {
	return d.getStats(userID, dataScope)
}

func (d *FileDAO) getStats(userID *uint, dataScope authz.UserDataScope) (*FileStats, error) {
	stats := &FileStats{}

	query := database.DB.Model(&model.File{})
	query = authz.ApplyOwnerScope(query, dataScope, "user_id")
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}

	// 总数和总大小
	var result struct {
		Count     int64 `json:"count"`
		TotalSize int64 `json:"total_size"`
	}
	if err := query.Select("COUNT(*) as count, COALESCE(SUM(file_size), 0) as total_size").Scan(&result).Error; err != nil {
		return nil, err
	}
	stats.Total = result.Count
	stats.TotalSize = result.TotalSize

	// 按类型统计
	var typeStats []struct {
		FileType string `json:"file_type"`
		Count    int64  `json:"count"`
		Size     int64  `json:"size"`
	}
	query2 := database.DB.Model(&model.File{})
	query2 = authz.ApplyOwnerScope(query2, dataScope, "user_id")
	if userID != nil {
		query2 = query2.Where("user_id = ?", *userID)
	}
	if err := query2.Select("file_type, COUNT(*) as count, COALESCE(SUM(file_size), 0) as size").
		Group("file_type").
		Find(&typeStats).Error; err == nil {
		stats.ByType = make(map[string]TypeStat)
		for _, s := range typeStats {
			stats.ByType[s.FileType] = TypeStat{Count: s.Count, Size: s.Size}
		}
	}

	return stats, nil
}

// FileStats 文件统计信息
type FileStats struct {
	Total     int64               `json:"total"`
	TotalSize int64               `json:"total_size"`
	ByType    map[string]TypeStat `json:"by_type"`
}

// TypeStat 类型统计
type TypeStat struct {
	Count int64 `json:"count"`
	Size  int64 `json:"size"`
}
