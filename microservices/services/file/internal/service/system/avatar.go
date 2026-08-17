package system

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"mime/multipart"
	"path/filepath"
	"strings"

	localmodel "github.com/go-admin-kit/services/file/internal/model"
	"github.com/go-admin-kit/services/file/internal/pkg/upload"
	"github.com/go-admin-kit/services/shared/pkg/authz"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
)

const (
	MaxAvatarBytes  int64 = 2 * 1024 * 1024
	MinAvatarPixels       = 32
	MaxAvatarPixels       = 4096
)

// UploadAvatarContext stores a self-service avatar without requiring the file
// management permission. Avatar validation is stricter than generic uploads:
// only decoded JPG/PNG images up to 2 MiB and bounded dimensions are accepted.
func (s *FileService) UploadAvatarContext(ctx context.Context, file *multipart.FileHeader, userID uint) (*localmodel.File, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if file == nil || file.Size <= 0 {
		return nil, upload.ErrFileEmpty
	}
	if file.Size > MaxAvatarBytes {
		return nil, upload.ErrAvatarTooLarge
	}
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		return nil, upload.ErrAvatarTypeNotAllowed
	}
	if err := s.enforceStorageQuota(ctx, file.Size); err != nil {
		return nil, err
	}

	info, err := s.uploader.UploadContext(ctx, file)
	if err != nil {
		if errors.Is(err, upload.ErrFileTypeNotAllowed) {
			return nil, upload.ErrAvatarTypeNotAllowed
		}
		return nil, err
	}
	cleanupStored := func() {
		_ = s.uploader.DeleteForStorageTypeContext(ctx, info.StorageType, info.FilePath)
		if info.ThumbnailPath != "" {
			_ = s.uploader.DeleteForStorageTypeContext(ctx, info.StorageType, info.ThumbnailPath)
		}
	}
	if info.FileType != "image" || (info.MimeType != "image/jpeg" && info.MimeType != "image/png") {
		cleanupStored()
		return nil, upload.ErrAvatarTypeNotAllowed
	}
	if info.ImageWidth < MinAvatarPixels || info.ImageHeight < MinAvatarPixels ||
		info.ImageWidth > MaxAvatarPixels || info.ImageHeight > MaxAvatarPixels {
		cleanupStored()
		return nil, upload.ErrAvatarDimensions
	}

	token, err := newAvatarPublicToken()
	if err != nil {
		cleanupStored()
		return nil, err
	}
	tenantID := tenant.IDFromContext(ctx)
	if tenantID == 0 {
		tenantID = 1
	}
	record := &localmodel.File{
		TenantID:        tenantID,
		UserID:          userID,
		FileName:        info.FileName,
		FilePath:        info.FilePath,
		FileSize:        info.FileSize,
		ImageWidth:      info.ImageWidth,
		ImageHeight:     info.ImageHeight,
		ThumbnailPath:   info.ThumbnailPath,
		ThumbnailURL:    info.ThumbnailURL,
		ThumbnailWidth:  info.ThumbnailWidth,
		ThumbnailHeight: info.ThumbnailHeight,
		FileType:        info.FileType,
		MimeType:        info.MimeType,
		Extension:       info.Extension,
		StorageType:     info.StorageType,
		URL:             info.URL,
		Hash:            info.Hash,
		Purpose:         "avatar",
		PublicToken:     &token,
	}
	if err := s.fileDAO.CreateAvatarContext(ctx, record, token); err != nil {
		cleanupStored()
		return nil, err
	}
	return record, nil
}

// DeleteOtherAvatarsContext is called only after auth-service has persisted
// the newly issued avatar URL. This prevents an auth update failure from
// deleting the user's still-active avatar.
func (s *FileService) DeleteOtherAvatarsContext(ctx context.Context, userID uint, keepTokens []string) error {
	files, err := s.fileDAO.ListOtherAvatarsContext(ctx, userID, keepTokens)
	if err != nil {
		return err
	}
	dataScope := authz.UserDataScope{Scope: authz.DataScopeSelf, UserID: userID}
	for i := range files {
		if err := s.DeleteFileContext(ctx, files[i].ID, userID, dataScope); err != nil {
			return err
		}
	}
	return nil
}

func (s *FileService) OpenAvatarByTokenContext(ctx context.Context, token string) (*FileContent, error) {
	if !ValidAvatarPublicToken(token) {
		return nil, ErrFileNotFoundOrPermissionDenied
	}
	file, err := s.fileDAO.GetAvatarByPublicTokenContext(ctx, token)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrFileNotFoundOrPermissionDenied
		}
		return nil, err
	}
	return s.OpenFileContentContext(ctx, file)
}

func newAvatarPublicToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func ValidAvatarPublicToken(token string) bool {
	if len(token) != 43 {
		return false
	}
	for _, r := range token {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}
