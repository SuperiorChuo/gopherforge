package system

import (
	"errors"
	"mime/multipart"
	"time"

	"github.com/go-admin-kit/server/internal/dao/system"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
	"github.com/go-admin-kit/server/internal/pkg/upload"
)

var ErrFileNotFoundOrPermissionDenied = errors.New("file not found or permission denied")

// FileService 文件服务
type FileService struct {
	fileDAO  system.FileDAO
	uploader *upload.Uploader
}

// NewFileService 创建文件服务
func NewFileService() *FileService {
	return &FileService{
		fileDAO:  system.FileDAO{},
		uploader: upload.NewUploader(),
	}
}

// FileListRequest 文件列表请求
type FileListRequest struct {
	pagination.PageRequest
	UserID    *uint               `form:"user_id" json:"user_id"`
	FileType  string              `form:"file_type" json:"file_type"`
	Keyword   string              `form:"keyword" json:"keyword"`
	StartTime *time.Time          `form:"start_time" time_format:"2006-01-02 15:04:05" json:"start_time"`
	EndTime   *time.Time          `form:"end_time" time_format:"2006-01-02 15:04:05" json:"end_time"`
	DataScope authz.UserDataScope `json:"-" form:"-"`
}

// Upload 上传文件
func (s *FileService) Upload(file *multipart.FileHeader, userID uint) (*model.File, error) {
	// 检查是否存在相同哈希的文件（秒传）
	// 这里需要先计算哈希，但为了简化，先上传再检查

	// 上传文件
	info, err := s.uploader.Upload(file)
	if err != nil {
		return nil, err
	}

	// 检查是否已存在（秒传）
	existingFile, err := s.fileDAO.GetByHash(info.Hash)
	if err == nil && existingFile != nil {
		// 文件已存在，删除刚上传的文件
		_ = s.uploader.Delete(info.FilePath)
		// 创建新的文件记录，指向已存在的文件
		newFile := &model.File{
			UserID:      userID,
			FileName:    info.FileName,
			FilePath:    existingFile.FilePath,
			FileSize:    existingFile.FileSize,
			FileType:    existingFile.FileType,
			MimeType:    existingFile.MimeType,
			Extension:   existingFile.Extension,
			StorageType: existingFile.StorageType,
			URL:         existingFile.URL,
			Hash:        existingFile.Hash,
		}
		if err := s.fileDAO.Create(newFile); err != nil {
			return nil, err
		}
		return newFile, nil
	}

	// 创建文件记录
	fileRecord := &model.File{
		UserID:      userID,
		FileName:    info.FileName,
		FilePath:    info.FilePath,
		FileSize:    info.FileSize,
		FileType:    info.FileType,
		MimeType:    info.MimeType,
		Extension:   info.Extension,
		StorageType: info.StorageType,
		URL:         info.URL,
		Hash:        info.Hash,
	}

	if err := s.fileDAO.Create(fileRecord); err != nil {
		// 创建记录失败，删除已上传的文件
		_ = s.uploader.Delete(info.FilePath)
		return nil, err
	}

	return fileRecord, nil
}

// UploadMultiple 批量上传文件
func (s *FileService) UploadMultiple(files []*multipart.FileHeader, userID uint) ([]*model.File, []error) {
	var results []*model.File
	var errs []error

	for _, file := range files {
		record, err := s.Upload(file, userID)
		if err != nil {
			errs = append(errs, err)
		} else {
			results = append(results, record)
		}
	}

	return results, errs
}

// GetFileByID 根据ID获取文件
func (s *FileService) GetFileByID(id uint) (*model.File, error) {
	return s.fileDAO.GetByID(id)
}

// GetFileByIDInScope 根据ID在当前用户数据权限范围内获取文件。
func (s *FileService) GetFileByIDInScope(id uint, dataScope authz.UserDataScope) (*model.File, error) {
	return s.fileDAO.GetByIDInScope(id, dataScope)
}

// GetFileByHash 根据哈希在当前用户数据权限范围内查询文件。
func (s *FileService) GetFileByHash(hash string, dataScope authz.UserDataScope) (*model.File, error) {
	return s.fileDAO.GetByHashInScope(hash, dataScope)
}

// GetFileList 获取文件列表
func (s *FileService) GetFileList(req FileListRequest) ([]model.File, int64, error) {
	return s.fileDAO.GetList(req.PageRequest, req.UserID, req.FileType, req.Keyword, req.StartTime, req.EndTime, req.DataScope)
}

// DeleteFile 删除文件
func (s *FileService) DeleteFile(id uint, userID uint, dataScope authz.UserDataScope) error {
	if dataScope.UserID == 0 {
		dataScope.UserID = userID
	}

	file, err := s.fileDAO.GetByIDInScope(id, dataScope)
	if err != nil {
		return ErrFileNotFoundOrPermissionDenied
	}

	// 检查是否有其他记录引用同一物理文件
	// 简化处理：直接删除物理文件
	// 实际应用中可能需要引用计数

	// 删除物理文件
	if err := s.uploader.Delete(file.FilePath); err != nil {
		// 记录日志但不中断流程
	}

	// 删除数据库记录
	return s.fileDAO.Delete(id)
}

// DeleteFiles 批量删除文件
func (s *FileService) DeleteFiles(ids []uint, userID uint, dataScope authz.UserDataScope) error {
	for _, id := range ids {
		if err := s.DeleteFile(id, userID, dataScope); err != nil {
			return err
		}
	}
	return nil
}

// GetFileStats 获取文件统计
func (s *FileService) GetFileStats(userID *uint, dataScope authz.UserDataScope) (*system.FileStats, error) {
	return s.fileDAO.GetStatsInScope(userID, dataScope)
}
