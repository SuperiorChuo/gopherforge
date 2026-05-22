package upload

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/go-admin-kit/server/internal/config"
)

var (
	ErrStorageProviderNotConfigured = errors.New("storage provider not configured")
	ErrStorageProviderUnavailable   = errors.New("storage provider unavailable")
	ErrStorageReadNotImplemented    = errors.New("storage provider read not implemented")
)

// StorageProvider hides the physical storage backend behind the uploader.
type StorageProvider interface {
	Type() string
	Store(ctx context.Context, objectKey string, body io.Reader) (*StoredObject, error)
	Open(ctx context.Context, filePath string) (*StoredObjectReader, error)
	Delete(ctx context.Context, filePath string) error
	PublicURL(filePath string) (string, error)
}

type StoredObject struct {
	Key         string
	FilePath    string
	URL         string
	StorageType string
}

type StoredObjectReader struct {
	Key         string
	FilePath    string
	StorageType string
	Size        int64
	Body        io.ReadCloser
}

func NewStorageProvider(cfg config.UploadConfig) (StorageProvider, error) {
	switch cfg.EffectiveStorageType() {
	case "local":
		return NewLocalStorageProvider(cfg), nil
	case "s3":
		return newReservedObjectStorageProvider("s3", cfg.S3, strings.TrimSpace(cfg.PublicBaseURL))
	case "minio":
		return newReservedObjectStorageProvider("minio", cfg.MinIO, strings.TrimSpace(cfg.PublicBaseURL))
	default:
		return nil, fmt.Errorf("%w: unsupported upload storage_type %q", ErrStorageProviderNotConfigured, cfg.StorageType)
	}
}

type LocalStorageProvider struct {
	root          string
	publicBaseURL string
}

func NewLocalStorageProvider(cfg config.UploadConfig) *LocalStorageProvider {
	return &LocalStorageProvider{
		root:          cfg.EffectiveLocalPath(),
		publicBaseURL: cfg.EffectivePublicBaseURL(),
	}
}

func (p *LocalStorageProvider) Type() string {
	return "local"
}

func (p *LocalStorageProvider) Store(_ context.Context, objectKey string, body io.Reader) (*StoredObject, error) {
	key, err := cleanObjectKey(objectKey)
	if err != nil {
		return nil, err
	}

	targetPath := filepath.Join(p.root, filepath.FromSlash(key))
	if err := ensureWithinBase(p.root, targetPath); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create upload directory: %w", err)
	}

	dst, err := os.Create(targetPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create upload file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, body); err != nil {
		return nil, fmt.Errorf("failed to write upload file: %w", err)
	}

	publicURL, err := p.PublicURL(targetPath)
	if err != nil {
		return nil, err
	}

	return &StoredObject{
		Key:         key,
		FilePath:    targetPath,
		URL:         publicURL,
		StorageType: p.Type(),
	}, nil
}

func (p *LocalStorageProvider) Delete(_ context.Context, filePath string) error {
	targetPath, err := p.resolvePath(filePath)
	if err != nil {
		return err
	}
	return os.Remove(targetPath)
}

func (p *LocalStorageProvider) Open(_ context.Context, filePath string) (*StoredObjectReader, error) {
	targetPath, err := p.resolveExistingPath(filePath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(targetPath)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%w: file path is a directory", ErrStorageProviderUnavailable)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: file path is not a regular file", ErrStorageProviderUnavailable)
	}
	body, err := os.Open(targetPath)
	if err != nil {
		return nil, err
	}
	key, err := p.objectKeyFromPath(filePath)
	if err != nil {
		_ = body.Close()
		return nil, err
	}
	return &StoredObjectReader{
		Key:         key,
		FilePath:    targetPath,
		StorageType: p.Type(),
		Size:        info.Size(),
		Body:        body,
	}, nil
}

func (p *LocalStorageProvider) PublicURL(filePath string) (string, error) {
	key, err := p.objectKeyFromPath(filePath)
	if err != nil {
		return "", err
	}
	return joinPublicURL(p.publicBaseURL, key), nil
}

func (p *LocalStorageProvider) resolvePath(filePath string) (string, error) {
	if strings.TrimSpace(filePath) == "" {
		return "", fmt.Errorf("%w: file path is empty", ErrStorageProviderNotConfigured)
	}

	cleanPath := filepath.Clean(filePath)
	root := filepath.Clean(p.root)
	targetPath := cleanPath
	if !filepath.IsAbs(cleanPath) && cleanPath != root && !strings.HasPrefix(cleanPath, root+string(filepath.Separator)) {
		key, err := cleanObjectKey(filePath)
		if err != nil {
			return "", err
		}
		targetPath = filepath.Join(p.root, filepath.FromSlash(key))
	}

	if err := ensureWithinBase(p.root, targetPath); err != nil {
		return "", err
	}
	return targetPath, nil
}

func (p *LocalStorageProvider) resolveExistingPath(filePath string) (string, error) {
	targetPath, err := p.resolvePath(filePath)
	if err != nil {
		return "", err
	}
	evalRoot, err := filepath.EvalSymlinks(p.root)
	if err != nil {
		return "", err
	}
	evalTarget, err := filepath.EvalSymlinks(filepath.Clean(targetPath))
	if err != nil {
		return "", err
	}
	if err := ensureWithinBase(evalRoot, evalTarget); err != nil {
		return "", fmt.Errorf("%w: file path resolves outside storage root", ErrStorageProviderUnavailable)
	}
	return evalTarget, nil
}

func (p *LocalStorageProvider) objectKeyFromPath(filePath string) (string, error) {
	if strings.TrimSpace(filePath) == "" {
		return "", fmt.Errorf("%w: file path is empty", ErrStorageProviderNotConfigured)
	}

	cleanPath := filepath.Clean(filePath)
	root := filepath.Clean(p.root)
	var relPath string
	if filepath.IsAbs(cleanPath) {
		absRoot, err := filepath.Abs(p.root)
		if err != nil {
			return "", err
		}
		if err := ensureWithinBase(absRoot, cleanPath); err != nil {
			return "", err
		}
		relPath, err = filepath.Rel(absRoot, cleanPath)
		if err != nil {
			return "", err
		}
	} else if cleanPath == root || strings.HasPrefix(cleanPath, root+string(filepath.Separator)) {
		var err error
		relPath, err = filepath.Rel(root, cleanPath)
		if err != nil {
			return "", err
		}
	} else {
		relPath = cleanPath
	}

	return cleanObjectKey(filepath.ToSlash(relPath))
}

type reservedObjectStorageProvider struct {
	storageType   string
	cfg           config.ObjectStorageConfig
	publicBaseURL string
	configErr     error
}

func newReservedObjectStorageProvider(storageType string, cfg config.ObjectStorageConfig, publicBaseURL string) (StorageProvider, error) {
	provider := &reservedObjectStorageProvider{
		storageType:   storageType,
		cfg:           cfg,
		publicBaseURL: publicBaseURL,
	}
	provider.configErr = validateObjectStorageConfig(storageType, cfg)
	return provider, provider.configErr
}

func (p *reservedObjectStorageProvider) Type() string {
	return p.storageType
}

func (p *reservedObjectStorageProvider) Store(ctx context.Context, objectKey string, body io.Reader) (*StoredObject, error) {
	if p.configErr != nil {
		return nil, p.configErr
	}
	key, err := cleanObjectKey(objectKey)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	client, err := newObjectStorageClient(p.storageType, p.cfg)
	if err != nil {
		return nil, err
	}
	if _, err := client.PutObject(ctx, p.cfg.Bucket, key, body, objectSize(body), minio.PutObjectOptions{}); err != nil {
		return nil, fmt.Errorf("%w: %s put %q failed: %v", ErrStorageProviderUnavailable, p.storageType, key, err)
	}
	publicURL, err := p.PublicURL(key)
	if err != nil {
		return nil, err
	}
	return &StoredObject{
		Key:         key,
		FilePath:    key,
		URL:         publicURL,
		StorageType: p.Type(),
	}, nil
}

func (p *reservedObjectStorageProvider) Open(ctx context.Context, filePath string) (*StoredObjectReader, error) {
	if p.configErr != nil {
		return nil, p.configErr
	}
	key, err := cleanObjectKey(filePath)
	if err != nil {
		return nil, err
	}
	client, err := newObjectStorageClient(p.storageType, p.cfg)
	if err != nil {
		return nil, err
	}

	if ctx == nil {
		ctx = context.Background()
	}
	stat, err := client.StatObject(ctx, p.cfg.Bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("%w: %s stat %q failed: %v", ErrStorageProviderUnavailable, p.storageType, key, err)
	}
	body, err := client.GetObject(ctx, p.cfg.Bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("%w: %s get %q failed: %v", ErrStorageProviderUnavailable, p.storageType, key, err)
	}
	return &StoredObjectReader{
		Key:         key,
		FilePath:    key,
		StorageType: p.Type(),
		Size:        stat.Size,
		Body:        body,
	}, nil
}

func (p *reservedObjectStorageProvider) Delete(ctx context.Context, filePath string) error {
	if p.configErr != nil {
		return p.configErr
	}
	key, err := cleanObjectKey(filePath)
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	client, err := newObjectStorageClient(p.storageType, p.cfg)
	if err != nil {
		return err
	}
	if err := client.RemoveObject(ctx, p.cfg.Bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("%w: %s delete %q failed: %v", ErrStorageProviderUnavailable, p.storageType, key, err)
	}
	return nil
}

func (p *reservedObjectStorageProvider) PublicURL(filePath string) (string, error) {
	if p.configErr != nil {
		return "", p.configErr
	}
	if strings.TrimSpace(p.publicBaseURL) == "" {
		return "", fmt.Errorf("%w: upload.public_base_url is required for %s public URLs", ErrStorageProviderNotConfigured, p.storageType)
	}
	key, err := cleanObjectKey(filePath)
	if err != nil {
		return "", err
	}
	return joinPublicURL(p.publicBaseURL, key), nil
}

func validateObjectStorageConfig(storageType string, cfg config.ObjectStorageConfig) error {
	var missing []string
	if strings.TrimSpace(cfg.Endpoint) == "" {
		missing = append(missing, "endpoint")
	}
	if strings.TrimSpace(cfg.Bucket) == "" {
		missing = append(missing, "bucket")
	}
	if storageType == "s3" && strings.TrimSpace(cfg.Region) == "" {
		missing = append(missing, "region")
	}
	if strings.TrimSpace(cfg.AccessKey) == "" {
		missing = append(missing, "access_key")
	}
	if strings.TrimSpace(cfg.SecretKey) == "" {
		missing = append(missing, "secret_key")
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: upload.%s missing %s", ErrStorageProviderNotConfigured, storageType, strings.Join(missing, ", "))
	}
	return nil
}

func newObjectStorageClient(storageType string, cfg config.ObjectStorageConfig) (*minio.Client, error) {
	endpoint, secure, err := objectStorageEndpoint(cfg)
	if err != nil {
		return nil, err
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(strings.TrimSpace(cfg.AccessKey), strings.TrimSpace(cfg.SecretKey), ""),
		Secure: secure,
		Region: strings.TrimSpace(cfg.Region),
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %s client init failed: %v", ErrStorageProviderUnavailable, storageType, err)
	}
	return client, nil
}

func objectSize(reader io.Reader) int64 {
	switch r := reader.(type) {
	case interface{ Len() int }:
		return int64(r.Len())
	case io.Seeker:
		current, err := r.Seek(0, io.SeekCurrent)
		if err != nil {
			return -1
		}
		end, err := r.Seek(0, io.SeekEnd)
		if err != nil {
			_, _ = r.Seek(current, io.SeekStart)
			return -1
		}
		_, _ = r.Seek(current, io.SeekStart)
		return end - current
	default:
		return -1
	}
}

func objectStorageEndpoint(cfg config.ObjectStorageConfig) (string, bool, error) {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	secure := cfg.UseSSL
	if strings.Contains(endpoint, "://") {
		parsed, err := url.Parse(endpoint)
		if err != nil || parsed.Host == "" {
			return "", false, fmt.Errorf("%w: invalid object storage endpoint", ErrStorageProviderNotConfigured)
		}
		switch parsed.Scheme {
		case "http":
			secure = false
		case "https":
			secure = true
		default:
			return "", false, fmt.Errorf("%w: unsupported object storage endpoint scheme %q", ErrStorageProviderNotConfigured, parsed.Scheme)
		}
		if strings.Trim(parsed.Path, "/") != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return "", false, fmt.Errorf("%w: object storage endpoint must not include path, query, or fragment", ErrStorageProviderNotConfigured)
		}
		endpoint = parsed.Host
	}
	return endpoint, secure, nil
}

func cleanObjectKey(objectKey string) (string, error) {
	objectKey = strings.ReplaceAll(strings.TrimSpace(objectKey), "\\", "/")
	if objectKey == "" {
		return "", fmt.Errorf("%w: object key is empty", ErrStorageProviderNotConfigured)
	}
	if parsed, err := url.Parse(objectKey); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return "", fmt.Errorf("%w: object key must not be a URL", ErrStorageProviderNotConfigured)
	}
	if strings.HasPrefix(objectKey, "/") {
		return "", fmt.Errorf("%w: object key must be relative", ErrStorageProviderNotConfigured)
	}
	cleanKey := strings.TrimPrefix(path.Clean("/"+objectKey), "/")
	if cleanKey == "." || cleanKey == "" || strings.HasPrefix(cleanKey, "../") || cleanKey == ".." {
		return "", fmt.Errorf("%w: invalid object key", ErrStorageProviderNotConfigured)
	}
	return cleanKey, nil
}

func ensureWithinBase(basePath, targetPath string) error {
	absBase, err := filepath.Abs(basePath)
	if err != nil {
		return err
	}
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(absBase, absTarget)
	if err != nil {
		return err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return fmt.Errorf("%w: file path escapes storage root", ErrStorageProviderNotConfigured)
	}
	return nil
}

func joinPublicURL(baseURL, objectKey string) string {
	key := strings.TrimLeft(path.Clean("/"+strings.ReplaceAll(objectKey, "\\", "/")), "/")
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "/" + key
	}

	parsed, err := url.Parse(baseURL)
	if err == nil && parsed.Scheme != "" && parsed.Host != "" {
		parsed.Path = strings.TrimRight(parsed.Path, "/") + "/" + key
		return parsed.String()
	}
	return strings.TrimRight(baseURL, "/") + "/" + key
}
