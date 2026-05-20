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

// FileInfo 文件信息
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

// Uploader 文件上传器
type Uploader struct {
	config      config.UploadConfig
	provider    StorageProvider
	providerErr error
}

// NewUploader 创建上传器
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

// Upload 上传文件
func (u *Uploader) Upload(file *multipart.FileHeader) (*FileInfo, error) {
	if err := u.ensureProvider(); err != nil {
		return nil, err
	}

	// 检查文件大小
	maxSize := int64(u.config.MaxSize) * 1024 * 1024
	if file.Size > maxSize {
		return nil, ErrFileTooLarge
	}

	// 获取文件扩展名
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext == "" {
		return nil, ErrFileTypeNotAllowed
	}

	// 检查文件类型
	if !u.isAllowedType(ext) {
		return nil, ErrFileTypeNotAllowed
	}

	// 打开文件
	src, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer src.Close()

	// 用文件头嗅探真实 MIME，避免只凭扩展名放行伪装文件。
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

	// 计算文件哈希
	hash, err := u.calculateHash(src)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate hash: %w", err)
	}

	// 重置文件指针
	if _, err := src.Seek(0, 0); err != nil {
		return nil, fmt.Errorf("failed to reset file pointer: %w", err)
	}

	// 生成对象键并写入配置的存储后端。
	objectKey := u.generateObjectKey(ext)
	stored, err := u.provider.Store(context.Background(), objectKey, src)
	if err != nil {
		return nil, err
	}

	// 获取文件类型
	fileType := u.getFileType(ext)

	// 构建文件信息
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

// UploadMultiple 批量上传文件
func (u *Uploader) UploadMultiple(files []*multipart.FileHeader) ([]*FileInfo, []error) {
	var results []*FileInfo
	var errs []error

	for _, file := range files {
		info, err := u.Upload(file)
		if err != nil {
			errs = append(errs, err)
		} else {
			results = append(results, info)
		}
	}

	return results, errs
}

// Delete 删除文件
func (u *Uploader) Delete(filePath string) error {
	if err := u.ensureProvider(); err != nil {
		return err
	}
	return u.provider.Delete(context.Background(), filePath)
}

// isAllowedType 检查文件类型是否允许
func (u *Uploader) isAllowedType(ext string) bool {
	for _, allowed := range u.config.AllowedTypes {
		if strings.EqualFold(ext, allowed) {
			return true
		}
	}
	return false
}

// calculateHash 计算文件MD5哈希
func (u *Uploader) calculateHash(file io.Reader) (string, error) {
	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// generateObjectKey 生成对象存储键
func (u *Uploader) generateObjectKey(ext string) string {
	// 按日期分目录
	now := time.Now()
	dateDir := now.Format("2006/01/02")
	// 生成唯一文件名
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

// getFileType 根据扩展名判断文件类型
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

// GetFileByHash 通过哈希查找文件（用于秒传）
func (u *Uploader) GetFileByHash(hash string) string {
	// 这个功能需要配合数据库使用
	// 这里只是预留接口
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
