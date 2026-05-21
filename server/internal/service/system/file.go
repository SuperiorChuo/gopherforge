package system

import (
	"context"
	"errors"
	"mime/multipart"
	"time"

	systemdao "github.com/go-admin-kit/server/internal/dao/system"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
	"github.com/go-admin-kit/server/internal/pkg/upload"
	"gorm.io/gorm"
)

var ErrFileNotFoundOrPermissionDenied = errors.New("file not found or permission denied")

type FileService struct {
	fileDAO  systemdao.FileDAO
	uploader *upload.Uploader
}

func NewFileService() *FileService {
	return &FileService{
		fileDAO:  systemdao.FileDAO{},
		uploader: upload.NewUploader(),
	}
}

type FileListRequest struct {
	pagination.PageRequest
	UserID    *uint               `form:"user_id" json:"user_id"`
	FileType  string              `form:"file_type" json:"file_type"`
	Keyword   string              `form:"keyword" json:"keyword"`
	StartTime *time.Time          `form:"start_time" time_format:"2006-01-02 15:04:05" json:"start_time"`
	EndTime   *time.Time          `form:"end_time" time_format:"2006-01-02 15:04:05" json:"end_time"`
	DataScope authz.UserDataScope `json:"-" form:"-"`
}

// Deprecated: use UploadContext instead.
func (s *FileService) Upload(file *multipart.FileHeader, userID uint) (*model.File, error) {
	return s.UploadContext(context.Background(), file, userID)
}

func (s *FileService) UploadContext(ctx context.Context, file *multipart.FileHeader, userID uint) (*model.File, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	info, err := s.uploader.UploadContext(ctx, file)
	if err != nil {
		return nil, err
	}

	existingFile, err := s.fileDAO.GetByHashContext(ctx, info.Hash)
	if err == nil && existingFile != nil {
		_ = s.uploader.DeleteContext(ctx, info.FilePath)
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
		if err := s.fileDAO.CreateContext(ctx, newFile); err != nil {
			return nil, err
		}
		return newFile, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		_ = s.uploader.DeleteContext(ctx, info.FilePath)
		return nil, err
	}

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

	if err := s.fileDAO.CreateContext(ctx, fileRecord); err != nil {
		_ = s.uploader.DeleteContext(ctx, info.FilePath)
		return nil, err
	}

	return fileRecord, nil
}

// Deprecated: use UploadMultipleContext instead.
func (s *FileService) UploadMultiple(files []*multipart.FileHeader, userID uint) ([]*model.File, []error) {
	return s.UploadMultipleContext(context.Background(), files, userID)
}

func (s *FileService) UploadMultipleContext(ctx context.Context, files []*multipart.FileHeader, userID uint) ([]*model.File, []error) {
	var results []*model.File
	var errs []error

	for _, file := range files {
		record, err := s.UploadContext(ctx, file, userID)
		if err != nil {
			errs = append(errs, err)
		} else {
			results = append(results, record)
		}
	}

	return results, errs
}

// Deprecated: use GetFileByIDContext instead.
func (s *FileService) GetFileByID(id uint) (*model.File, error) {
	return s.GetFileByIDContext(context.Background(), id)
}

func (s *FileService) GetFileByIDContext(ctx context.Context, id uint) (*model.File, error) {
	file, err := s.fileDAO.GetByIDContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrFileNotFoundOrPermissionDenied
		}
		return nil, err
	}
	return file, nil
}

// Deprecated: use GetFileByIDInScopeContext instead.
func (s *FileService) GetFileByIDInScope(id uint, dataScope authz.UserDataScope) (*model.File, error) {
	return s.GetFileByIDInScopeContext(context.Background(), id, dataScope)
}

func (s *FileService) GetFileByIDInScopeContext(ctx context.Context, id uint, dataScope authz.UserDataScope) (*model.File, error) {
	file, err := s.fileDAO.GetByIDInScopeContext(ctx, id, dataScope)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrFileNotFoundOrPermissionDenied
		}
		return nil, err
	}
	return file, nil
}

// Deprecated: use GetFileByHashContext instead.
func (s *FileService) GetFileByHash(hash string, dataScope authz.UserDataScope) (*model.File, error) {
	return s.GetFileByHashContext(context.Background(), hash, dataScope)
}

func (s *FileService) GetFileByHashContext(ctx context.Context, hash string, dataScope authz.UserDataScope) (*model.File, error) {
	file, err := s.fileDAO.GetByHashInScopeContext(ctx, hash, dataScope)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrFileNotFoundOrPermissionDenied
		}
		return nil, err
	}
	return file, nil
}

// Deprecated: use GetFileListContext instead.
func (s *FileService) GetFileList(req FileListRequest) ([]model.File, int64, error) {
	return s.GetFileListContext(context.Background(), req)
}

func (s *FileService) GetFileListContext(ctx context.Context, req FileListRequest) ([]model.File, int64, error) {
	return s.fileDAO.GetListContext(ctx, req.PageRequest, req.UserID, req.FileType, req.Keyword, req.StartTime, req.EndTime, req.DataScope)
}

// Deprecated: use DeleteFileContext instead.
func (s *FileService) DeleteFile(id uint, userID uint, dataScope authz.UserDataScope) error {
	return s.DeleteFileContext(context.Background(), id, userID, dataScope)
}

func (s *FileService) DeleteFileContext(ctx context.Context, id uint, userID uint, dataScope authz.UserDataScope) error {
	if dataScope.UserID == 0 {
		dataScope.UserID = userID
	}

	file, err := s.fileDAO.GetByIDInScopeContext(ctx, id, dataScope)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrFileNotFoundOrPermissionDenied
		}
		return err
	}

	_ = s.uploader.DeleteContext(ctx, file.FilePath)
	return s.fileDAO.DeleteContext(ctx, id)
}

// Deprecated: use DeleteFilesContext instead.
func (s *FileService) DeleteFiles(ids []uint, userID uint, dataScope authz.UserDataScope) error {
	return s.DeleteFilesContext(context.Background(), ids, userID, dataScope)
}

func (s *FileService) DeleteFilesContext(ctx context.Context, ids []uint, userID uint, dataScope authz.UserDataScope) error {
	for _, id := range ids {
		if err := s.DeleteFileContext(ctx, id, userID, dataScope); err != nil {
			return err
		}
	}
	return nil
}

// Deprecated: use GetFileStatsContext instead.
func (s *FileService) GetFileStats(userID *uint, dataScope authz.UserDataScope) (*systemdao.FileStats, error) {
	return s.GetFileStatsContext(context.Background(), userID, dataScope)
}

func (s *FileService) GetFileStatsContext(ctx context.Context, userID *uint, dataScope authz.UserDataScope) (*systemdao.FileStats, error) {
	return s.fileDAO.GetStatsInScopeContext(ctx, userID, dataScope)
}
