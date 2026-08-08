package system

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"strings"
	"time"

	systemdao "github.com/go-admin-kit/services/file/internal/dao/system"
	"github.com/go-admin-kit/services/file/internal/model"
	"github.com/go-admin-kit/services/file/internal/pkg/authz"
	"github.com/go-admin-kit/services/shared/pkg/pagination"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"github.com/go-admin-kit/services/file/internal/pkg/upload"
	"gorm.io/gorm"
)

var ErrFileNotFoundOrPermissionDenied = errors.New("file not found or permission denied")

// UploadSessionStore is the persistence surface for chunked uploads, so tests
// can substitute an in-memory implementation.
type UploadSessionStore interface {
	CreateContext(ctx context.Context, s *model.UploadSession) error
	GetByIDContext(ctx context.Context, id uint) (*model.UploadSession, error)
	GetPendingByHashContext(ctx context.Context, hash string, tenantID uint) (*model.UploadSession, error)
	MarkChunkReceivedContext(ctx context.Context, id uint, bitmap string, count int) error
	DeleteContext(ctx context.Context, id uint) error
	PruneExpiredContext(ctx context.Context, before time.Time) (int64, error)
}

type FileService struct {
	fileDAO    systemdao.FileDAO
	sessionDAO UploadSessionStore
	uploader   *upload.Uploader
}

type FileContent struct {
	FileName    string
	ContentType string
	Size        int64
	Body        io.ReadCloser
}

func NewFileService() *FileService {
	return &FileService{
		fileDAO:    systemdao.FileDAO{},
		sessionDAO: &systemdao.UploadSessionDAO{},
		uploader:   upload.NewUploader(),
	}
}

// NewFileServiceWithDB builds a FileService backed by an injected database
// handle. The uploader keeps its default implementation.
func NewFileServiceWithDB(db *gorm.DB) *FileService {
	return &FileService{
		fileDAO:    *systemdao.NewFileDAO(db),
		sessionDAO: systemdao.NewUploadSessionDAO(db),
		uploader:   upload.NewUploader(),
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

func (s *FileService) UploadContext(ctx context.Context, file *multipart.FileHeader, userID uint) (*model.File, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if err := s.enforceStorageQuota(ctx, file.Size); err != nil {
		return nil, err
	}

	info, err := s.uploader.UploadContext(ctx, file)
	if err != nil {
		return nil, err
	}

	tenantID := tenant.IDFromContext(ctx)

	existingFile, err := s.fileDAO.GetByHashContext(ctx, info.Hash)
	if err == nil && existingFile != nil {
		_ = s.uploader.DeleteContext(ctx, info.FilePath)
		if info.ThumbnailPath != "" {
			_ = s.uploader.DeleteContext(ctx, info.ThumbnailPath)
		}
		newFile := &model.File{
			TenantID:        tenantID,
			UserID:          userID,
			FileName:        info.FileName,
			FilePath:        existingFile.FilePath,
			FileSize:        existingFile.FileSize,
			ImageWidth:      existingFile.ImageWidth,
			ImageHeight:     existingFile.ImageHeight,
			ThumbnailPath:   existingFile.ThumbnailPath,
			ThumbnailURL:    existingFile.ThumbnailURL,
			ThumbnailWidth:  existingFile.ThumbnailWidth,
			ThumbnailHeight: existingFile.ThumbnailHeight,
			FileType:        existingFile.FileType,
			MimeType:        existingFile.MimeType,
			Extension:       existingFile.Extension,
			StorageType:     existingFile.StorageType,
			URL:             existingFile.URL,
			Hash:            existingFile.Hash,
		}
		if err := s.fileDAO.CreateContext(ctx, newFile); err != nil {
			return nil, err
		}
		return newFile, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		_ = s.uploader.DeleteContext(ctx, info.FilePath)
		if info.ThumbnailPath != "" {
			_ = s.uploader.DeleteContext(ctx, info.ThumbnailPath)
		}
		return nil, err
	}

	fileRecord := &model.File{
		TenantID:        tenantID,
		UserID:          userID,
		FileName:        info.FileName,
		FilePath:        info.FilePath,
		FileSize:        info.FileSize,
		ImageWidth:      info.ImageWidth,
		ImageHeight:     info.ImageHeight,
		FileType:        info.FileType,
		MimeType:        info.MimeType,
		Extension:       info.Extension,
		StorageType:     info.StorageType,
		URL:             info.URL,
		Hash:            info.Hash,
		ThumbnailPath:   info.ThumbnailPath,
		ThumbnailURL:    info.ThumbnailURL,
		ThumbnailWidth:  info.ThumbnailWidth,
		ThumbnailHeight: info.ThumbnailHeight,
	}

	if err := s.fileDAO.CreateContext(ctx, fileRecord); err != nil {
		_ = s.uploader.DeleteContext(ctx, info.FilePath)
		if info.ThumbnailPath != "" {
			_ = s.uploader.DeleteContext(ctx, info.ThumbnailPath)
		}
		return nil, err
	}

	return fileRecord, nil
}

// enforceStorageQuota rejects an upload that would push the tenant past its
// storage quota. quota 0 (default) means unlimited. Runs before the object is
// stored so a rejected file is never written.
func (s *FileService) enforceStorageQuota(ctx context.Context, size int64) error {
	tenantID := tenant.IDFromContext(ctx)
	if tenantID == 0 {
		tenantID = 1
	}
	quota, err := s.fileDAO.GetTenantStorageQuotaContext(ctx, tenantID)
	if err != nil {
		// tenant_packages may predate the quota column in some deployments;
		// a lookup failure must not break uploads, so fail open (unlimited).
		return nil
	}
	if quota.QuotaMB <= 0 {
		return nil
	}
	limit := quota.QuotaMB * 1024 * 1024
	if quota.UsedByte+size > limit {
		return upload.ErrStorageQuotaExceeded
	}
	return nil
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

func (s *FileService) GetFileListContext(ctx context.Context, req FileListRequest) ([]model.File, int64, error) {
	return s.fileDAO.GetListContext(ctx, req.PageRequest, req.UserID, req.FileType, req.Keyword, req.StartTime, req.EndTime, req.DataScope)
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

	filePathReferences, err := s.fileDAO.CountByFilePathExcludingIDContext(ctx, file.StorageType, file.FilePath, file.ID)
	if err != nil {
		return err
	}
	if file.ThumbnailPath != "" {
		thumbnailPathReferences, err := s.fileDAO.CountByThumbnailPathExcludingIDContext(ctx, file.StorageType, file.ThumbnailPath, file.ID)
		if err != nil {
			return err
		}
		if thumbnailPathReferences == 0 {
			if err := s.uploader.DeleteForStorageTypeContext(ctx, file.StorageType, file.ThumbnailPath); err != nil {
				return fmt.Errorf("delete stored thumbnail: %w", err)
			}
		}
	}
	if filePathReferences == 0 {
		if err := s.uploader.DeleteForStorageTypeContext(ctx, file.StorageType, file.FilePath); err != nil {
			return fmt.Errorf("delete stored file: %w", err)
		}
	}
	return s.fileDAO.DeleteContext(ctx, id)
}

func (s *FileService) DeleteFilesContext(ctx context.Context, ids []uint, userID uint, dataScope authz.UserDataScope) error {
	for _, id := range ids {
		if err := s.DeleteFileContext(ctx, id, userID, dataScope); err != nil {
			return err
		}
	}
	return nil
}

func (s *FileService) GetFileStatsContext(ctx context.Context, userID *uint, dataScope authz.UserDataScope) (*systemdao.FileStats, error) {
	stats, err := s.fileDAO.GetStatsInScopeContext(ctx, userID, dataScope)
	if err != nil {
		return nil, err
	}
	tenantID := tenant.IDFromContext(ctx)
	if tenantID == 0 {
		tenantID = 1
	}
	quota, quotaErr := s.fileDAO.GetTenantStorageQuotaContext(ctx, tenantID)
	if quotaErr == nil {
		stats.StorageQuotaMB = &quota.QuotaMB
	}
	// quotaErr != nil → StorageQuotaMB 保持 nil（前端据此显示"配额不可用"）
	return stats, nil
}

func (s *FileService) OpenFileContentContext(ctx context.Context, file *model.File) (*FileContent, error) {
	if file == nil {
		return nil, ErrFileNotFoundOrPermissionDenied
	}
	if ctx == nil {
		ctx = context.Background()
	}
	opened, err := s.uploader.OpenForStorageTypeContext(ctx, file.StorageType, file.FilePath)
	if err != nil {
		return nil, err
	}
	contentType := strings.TrimSpace(file.MimeType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return &FileContent{
		FileName:    file.FileName,
		ContentType: contentType,
		Size:        opened.Size,
		Body:        opened.Body,
	}, nil
}
