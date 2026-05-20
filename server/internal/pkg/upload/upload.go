package upload

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-admin-kit/server/internal/config"
)

var (
	ErrFileEmpty          = errors.New("file is empty")
	ErrFileTooLarge       = errors.New("file too large")
	ErrFileTypeNotAllowed = errors.New("file type not allowed")
)

// FileInfo describes an uploaded file.
type FileInfo struct {
	FileName    string `json:"file_name"`
	FilePath    string `json:"file_path"`
	FileSize    int64  `json:"file_size"`
	FileType    string `json:"file_type"`
	MimeType    string `json:"mime_type"`
	Extension   string `json:"extension"`
	StorageType string `json:"storage_type"`
	URL         string `json:"url"`
	Hash        string `json:"hash"`
}

// Uploader uploads files through the configured storage provider.
type Uploader struct {
	config      config.UploadConfig
	provider    StorageProvider
	providerErr error
}

// NewUploader creates an uploader.
func NewUploader() *Uploader {
	return NewUploaderWithConfig(config.Cfg.Upload)
}

func NewUploaderWithConfig(cfg config.UploadConfig) *Uploader {
	provider, err := NewStorageProvider(cfg)
	return &Uploader{
		config:      cfg,
		provider:    provider,
		providerErr: err,
	}
}

// Upload uploads one file.
func (u *Uploader) Upload(file *multipart.FileHeader) (*FileInfo, error) {
	return u.UploadContext(context.Background(), file)
}

func (u *Uploader) UploadContext(ctx context.Context, file *multipart.FileHeader) (*FileInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := u.ensureProvider(); err != nil {
		return nil, err
	}

	// Check file size.
	maxSize := int64(u.config.MaxSize) * 1024 * 1024
	if file.Size > maxSize {
		return nil, ErrFileTooLarge
	}

	// Resolve file extension.
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext == "" {
		return nil, ErrFileTypeNotAllowed
	}

	// Check allowed file type.
	if !u.isAllowedType(ext) {
		return nil, ErrFileTypeNotAllowed
	}

	// Open the source file.
	src, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer src.Close()

	// Sniff the real MIME type from the file header instead of trusting the extension.
	mimeType, err := detectContentType(src)
	if err != nil {
		return nil, fmt.Errorf("failed to detect content type: %w", err)
	}
	if !isCompatibleContentType(ext, mimeType) {
		return nil, ErrFileTypeNotAllowed
	}

	if _, err := src.Seek(0, 0); err != nil {
		return nil, fmt.Errorf("failed to reset file pointer: %w", err)
	}

	// Calculate the file hash.
	hash, err := u.calculateHash(src)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate hash: %w", err)
	}

	// Reset the file pointer.
	if _, err := src.Seek(0, 0); err != nil {
		return nil, fmt.Errorf("failed to reset file pointer: %w", err)
	}

	// Generate the object key and store the file.
	objectKey := u.generateObjectKey(ext)
	stored, err := u.provider.Store(ctx, objectKey, src)
	if err != nil {
		return nil, err
	}

	// Resolve file type.
	fileType := u.getFileType(ext)

	// Build file metadata.
	info := &FileInfo{
		FileName:    safeFileName(file.Filename),
		FilePath:    stored.FilePath,
		FileSize:    file.Size,
		FileType:    fileType,
		MimeType:    mimeType,
		Extension:   ext,
		StorageType: stored.StorageType,
		URL:         stored.URL,
		Hash:        hash,
	}

	return info, nil
}

// UploadMultiple uploads multiple files.
func (u *Uploader) UploadMultiple(files []*multipart.FileHeader) ([]*FileInfo, []error) {
	return u.UploadMultipleContext(context.Background(), files)
}

func (u *Uploader) UploadMultipleContext(ctx context.Context, files []*multipart.FileHeader) ([]*FileInfo, []error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var results []*FileInfo
	var errs []error

	for _, file := range files {
		info, err := u.UploadContext(ctx, file)
		if err != nil {
			errs = append(errs, err)
		} else {
			results = append(results, info)
		}
	}

	return results, errs
}

// Delete deletes a file from storage.
func (u *Uploader) Delete(filePath string) error {
	return u.DeleteContext(context.Background(), filePath)
}

func (u *Uploader) DeleteContext(ctx context.Context, filePath string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := u.ensureProvider(); err != nil {
		return err
	}
	return u.provider.Delete(ctx, filePath)
}

// isAllowedType reports whether an extension is allowed.
func (u *Uploader) isAllowedType(ext string) bool {
	for _, allowed := range u.config.AllowedTypes {
		if strings.EqualFold(ext, allowed) {
			return true
		}
	}
	return false
}

// calculateHash calculates an MD5 hash.
func (u *Uploader) calculateHash(file io.Reader) (string, error) {
	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// generateObjectKey creates an object storage key.
func (u *Uploader) generateObjectKey(ext string) string {
	// Partition objects by date.
	now := time.Now()
	dateDir := now.Format("2006/01/02")
	// Generate a unique filename.
	fileName := fmt.Sprintf("%d%s", now.UnixNano(), ext)

	return path.Join(dateDir, fileName)
}

func (u *Uploader) ensureProvider() error {
	if u.providerErr != nil {
		return u.providerErr
	}
	if u.provider == nil {
		return ErrStorageProviderNotConfigured
	}
	return nil
}

// getFileType classifies a file by extension.
func (u *Uploader) getFileType(ext string) string {
	ext = strings.ToLower(ext)

	imageExts := []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".svg"}
	videoExts := []string{".mp4", ".avi", ".mov", ".wmv", ".flv", ".mkv"}
	audioExts := []string{".mp3", ".wav", ".ogg", ".flac", ".aac"}
	docExts := []string{".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".txt"}

	for _, e := range imageExts {
		if ext == e {
			return "image"
		}
	}
	for _, e := range videoExts {
		if ext == e {
			return "video"
		}
	}
	for _, e := range audioExts {
		if ext == e {
			return "audio"
		}
	}
	for _, e := range docExts {
		if ext == e {
			return "document"
		}
	}

	return "other"
}

// GetFileByHash is reserved for instant-upload lookup by hash.
func (u *Uploader) GetFileByHash(hash string) string {
	// This feature needs database support.
	// Keep the method as a placeholder for future integration.
	return ""
}

func detectContentType(file multipart.File) (string, error) {
	head := make([]byte, 512)
	n, err := file.Read(head)
	if err != nil && err != io.EOF {
		return "", err
	}
	if n == 0 {
		return "", ErrFileEmpty
	}
	return http.DetectContentType(head[:n]), nil
}

func isCompatibleContentType(ext string, contentType string) bool {
	ext = strings.ToLower(ext)
	contentType = strings.ToLower(strings.Split(contentType, ";")[0])

	compatible := map[string][]string{
		".jpg":  {"image/jpeg"},
		".jpeg": {"image/jpeg"},
		".png":  {"image/png"},
		".gif":  {"image/gif"},
		".webp": {"image/webp"},
		".pdf":  {"application/pdf"},
		".zip":  {"application/zip", "application/x-zip-compressed"},
		".rar":  {"application/vnd.rar", "application/x-rar-compressed", "application/octet-stream"},
		".doc":  {"application/msword", "application/octet-stream"},
		".docx": {"application/vnd.openxmlformats-officedocument.wordprocessingml.document", "application/zip", "application/octet-stream"},
		".xls":  {"application/vnd.ms-excel", "application/octet-stream"},
		".xlsx": {"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "application/zip", "application/octet-stream"},
	}

	allowed, ok := compatible[ext]
	if !ok {
		return false
	}
	for _, item := range allowed {
		if contentType == item {
			return true
		}
	}
	return false
}

func safeFileName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	base := filepath.Base(name)
	base = strings.ReplaceAll(base, "\x00", "")
	if base == "." || base == string(filepath.Separator) {
		return "upload"
	}
	return base
}
