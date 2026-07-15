package upload

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-admin-kit/services/file/internal/config"
	"github.com/go-admin-kit/services/file/internal/pkg/logger"
	xdraw "golang.org/x/image/draw"
)

var (
	ErrFileEmpty          = errors.New("file is empty")
	ErrFileTooLarge       = errors.New("file too large")
	ErrFileTypeNotAllowed = errors.New("file type not allowed")
)

// FileInfo describes an uploaded file.
type FileInfo struct {
	FileName        string `json:"file_name"`
	FilePath        string `json:"file_path"`
	FileSize        int64  `json:"file_size"`
	FileType        string `json:"file_type"`
	MimeType        string `json:"mime_type"`
	Extension       string `json:"extension"`
	StorageType     string `json:"storage_type"`
	URL             string `json:"url"`
	Hash            string `json:"hash"`
	ImageWidth      int    `json:"image_width"`
	ImageHeight     int    `json:"image_height"`
	ThumbnailPath   string `json:"thumbnail_path"`
	ThumbnailURL    string `json:"thumbnail_url"`
	ThumbnailWidth  int    `json:"thumbnail_width"`
	ThumbnailHeight int    `json:"thumbnail_height"`
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

	imageWidth, imageHeight, err := imageDimensions(src, ext)
	if err != nil {
		return nil, err
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

	var thumbnailPath string
	var thumbnailURL string
	var thumbnailWidth int
	var thumbnailHeight int
	if _, err := src.Seek(0, 0); err == nil {
		thumbnail, err := u.generateThumbnail(ctx, src, ext, objectKey, imageWidth, imageHeight)
		if err == nil && thumbnail != nil {
			thumbnailPath = thumbnail.FilePath
			thumbnailURL = thumbnail.URL
			thumbnailWidth = thumbnail.Width
			thumbnailHeight = thumbnail.Height
		} else if err != nil && logger.Logger != nil {
			logger.Warn("thumbnail generation failed",
				logger.String("file_name", safeFileName(file.Filename)),
				logger.String("extension", ext),
				logger.Err(err),
			)
		}
	}

	// Resolve file type.
	fileType := u.getFileType(ext)

	// Build file metadata.
	info := &FileInfo{
		FileName:        safeFileName(file.Filename),
		FilePath:        stored.FilePath,
		FileSize:        file.Size,
		FileType:        fileType,
		MimeType:        mimeType,
		Extension:       ext,
		StorageType:     stored.StorageType,
		URL:             stored.URL,
		Hash:            hash,
		ImageWidth:      imageWidth,
		ImageHeight:     imageHeight,
		ThumbnailPath:   thumbnailPath,
		ThumbnailURL:    thumbnailURL,
		ThumbnailWidth:  thumbnailWidth,
		ThumbnailHeight: thumbnailHeight,
	}

	return info, nil
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

func (u *Uploader) DeleteContext(ctx context.Context, filePath string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := u.ensureProvider(); err != nil {
		return err
	}
	return u.provider.Delete(ctx, filePath)
}

func (u *Uploader) DeleteForStorageTypeContext(ctx context.Context, storageType, filePath string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	provider, err := u.providerForStorageType(storageType)
	if err != nil {
		return err
	}
	return provider.Delete(ctx, filePath)
}

func (u *Uploader) OpenContext(ctx context.Context, filePath string) (*StoredObjectReader, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := u.ensureProvider(); err != nil {
		return nil, err
	}
	return u.provider.Open(ctx, filePath)
}

func (u *Uploader) OpenForStorageTypeContext(ctx context.Context, storageType, filePath string) (*StoredObjectReader, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	provider, err := u.providerForStorageType(storageType)
	if err != nil {
		return nil, err
	}
	return provider.Open(ctx, filePath)
}

func (u *Uploader) providerForStorageType(storageType string) (StorageProvider, error) {
	normalized := strings.ToLower(strings.TrimSpace(storageType))
	if normalized == "" || normalized == u.config.EffectiveStorageType() {
		if err := u.ensureProvider(); err != nil {
			return nil, err
		}
		return u.provider, nil
	}

	cfg := u.config
	cfg.StorageType = normalized
	provider, err := NewStorageProvider(cfg)
	if err != nil {
		return nil, err
	}
	return provider, nil
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

type thumbnailInfo struct {
	FilePath string
	URL      string
	Width    int
	Height   int
}

func (u *Uploader) generateThumbnail(ctx context.Context, src io.Reader, ext, objectKey string, imageWidth, imageHeight int) (*thumbnailInfo, error) {
	if !supportsThumbnail(ext) || imageWidth <= 0 || imageHeight <= 0 {
		return nil, nil
	}

	targetWidth, targetHeight := thumbnailBounds(u.config.Image)
	width, height := fitDimensions(imageWidth, imageHeight, targetWidth, targetHeight)
	if width <= 0 || height <= 0 {
		return nil, nil
	}

	img, _, err := image.Decode(src)
	if err != nil {
		return nil, err
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), xdraw.Over, nil)

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, err
	}

	base := strings.TrimSuffix(path.Base(objectKey), path.Ext(objectKey))
	thumbnailKey := path.Join(path.Dir(objectKey), "thumbs", fmt.Sprintf("%s_%dx%d.png", base, width, height))
	stored, err := u.provider.Store(ctx, thumbnailKey, &buf)
	if err != nil {
		return nil, err
	}
	return &thumbnailInfo{
		FilePath: stored.FilePath,
		URL:      stored.URL,
		Width:    width,
		Height:   height,
	}, nil
}

func supportsThumbnail(ext string) bool {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg", ".png", ".gif":
		return true
	default:
		return false
	}
}

func thumbnailBounds(imageCfg config.ImageConfig) (int, int) {
	width := imageCfg.ThumbnailWidth
	height := imageCfg.ThumbnailHeight
	if width <= 0 {
		width = 200
	}
	if height <= 0 {
		height = 200
	}
	return width, height
}

func fitDimensions(width, height, maxWidth, maxHeight int) (int, int) {
	if width <= maxWidth && height <= maxHeight {
		return width, height
	}
	ratioW := float64(maxWidth) / float64(width)
	ratioH := float64(maxHeight) / float64(height)
	ratio := ratioW
	if ratioH < ratio {
		ratio = ratioH
	}
	scaledWidth := int(float64(width) * ratio)
	scaledHeight := int(float64(height) * ratio)
	if scaledWidth < 1 {
		scaledWidth = 1
	}
	if scaledHeight < 1 {
		scaledHeight = 1
	}
	return scaledWidth, scaledHeight
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

func imageDimensions(file multipart.File, ext string) (int, int, error) {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg", ".png", ".gif":
	default:
		return 0, 0, nil
	}

	cfg, _, err := image.DecodeConfig(file)
	if err != nil {
		return 0, 0, ErrFileTypeNotAllowed
	}
	return cfg.Width, cfg.Height, nil
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
