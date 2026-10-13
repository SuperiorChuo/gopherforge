// 包 storage 隐藏 IM 附件的实际存储位置。key 形如
// "<yyyymm>/<uuid><ext>"；公开 URL 始终是 /im/uploads/<key>，因此
// 切换后端不影响客户端。
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var ErrNotFound = errors.New("attachment not found")

type Store interface {
	Type() string
	Save(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	// Open 返回对象流及其大小和内容类型。
	Open(ctx context.Context, key string) (io.ReadCloser, int64, string, error)
	// Delete 删除对象；对象不存在不算错误
	// （保留期清理是幂等的）。
	Delete(ctx context.Context, key string) error
}

// CleanKey 拒绝路径穿越与绝对路径 key。
func CleanKey(key string) (string, error) {
	key = strings.TrimSpace(strings.ReplaceAll(key, "\\", "/"))
	key = strings.TrimPrefix(key, "/")
	clean := strings.TrimPrefix(path.Clean("/"+key), "/")
	if clean == "" || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("invalid attachment key %q", key)
	}
	return clean, nil
}

func contentTypeFor(key, fallback string) string {
	if fallback != "" {
		return fallback
	}
	if ct := mime.TypeByExtension(strings.ToLower(filepath.Ext(key))); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// ---------- 本地磁盘（dev / 兜底） ----------

type Local struct {
	Root string
}

func NewLocal(root string) *Local { return &Local{Root: root} }

func (l *Local) Type() string { return "local" }

func (l *Local) Save(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	key, err := CleanKey(key)
	if err != nil {
		return err
	}
	dst := filepath.Join(l.Root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}

func (l *Local) Delete(_ context.Context, key string) error {
	key, err := CleanKey(key)
	if err != nil {
		return err
	}
	err = os.Remove(filepath.Join(l.Root, filepath.FromSlash(key)))
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	return err
}

func (l *Local) Open(_ context.Context, key string) (io.ReadCloser, int64, string, error) {
	key, err := CleanKey(key)
	if err != nil {
		return nil, 0, "", err
	}
	p := filepath.Join(l.Root, filepath.FromSlash(key))
	info, err := os.Stat(p)
	if err != nil || info.IsDir() {
		return nil, 0, "", ErrNotFound
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, 0, "", err
	}
	return f, info.Size(), contentTypeFor(key, ""), nil
}

// ---------- MinIO / S3 兼容 ----------

type MinIOConfig struct {
	Endpoint  string // 容器内 host:port
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
	// Prefix 在共享 bucket 内为 IM 对象划分命名空间。
	Prefix string
}

type MinIO struct {
	client *minio.Client
	bucket string
	prefix string
}

func NewMinIO(ctx context.Context, cfg MinIOConfig) (*MinIO, error) {
	if cfg.Endpoint == "" || cfg.Bucket == "" {
		return nil, errors.New("minio endpoint and bucket required")
	}
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, err
	}
	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("minio unreachable: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("create bucket %s: %w", cfg.Bucket, err)
		}
	}
	prefix := strings.Trim(cfg.Prefix, "/")
	if prefix == "" {
		prefix = "im"
	}
	return &MinIO{client: client, bucket: cfg.Bucket, prefix: prefix}, nil
}

func (m *MinIO) Type() string { return "minio" }

func (m *MinIO) objectName(key string) (string, error) {
	key, err := CleanKey(key)
	if err != nil {
		return "", err
	}
	return m.prefix + "/" + key, nil
}

func (m *MinIO) Save(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	name, err := m.objectName(key)
	if err != nil {
		return err
	}
	_, err = m.client.PutObject(ctx, m.bucket, name, r, size, minio.PutObjectOptions{
		ContentType: contentTypeFor(key, contentType),
	})
	return err
}

func (m *MinIO) Delete(ctx context.Context, key string) error {
	name, err := m.objectName(key)
	if err != nil {
		return err
	}
	// 对不存在的 key 执行 RemoveObject 也会成功（S3 语义）——幂等
	return m.client.RemoveObject(ctx, m.bucket, name, minio.RemoveObjectOptions{})
}

func (m *MinIO) Open(ctx context.Context, key string) (io.ReadCloser, int64, string, error) {
	name, err := m.objectName(key)
	if err != nil {
		return nil, 0, "", err
	}
	stat, err := m.client.StatObject(ctx, m.bucket, name, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return nil, 0, "", ErrNotFound
		}
		return nil, 0, "", err
	}
	obj, err := m.client.GetObject(ctx, m.bucket, name, minio.GetObjectOptions{})
	if err != nil {
		return nil, 0, "", err
	}
	return obj, stat.Size, contentTypeFor(key, stat.ContentType), nil
}
