package upload

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-admin-kit/server/internal/config"
)

func TestUploaderLocalUploadAndDelete(t *testing.T) {
	dir := t.TempDir()
	uploader := NewUploaderWithConfig(config.UploadConfig{
		StorageType:   "local",
		LocalPath:     dir,
		PublicBaseURL: "/files",
		MaxSize:       1,
		AllowedTypes:  []string{".png"},
	})

	info, err := uploader.Upload(newMultipartFileHeader(t, "avatar.png", tinyPNG()))
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}

	if info.StorageType != "local" {
		t.Fatalf("storage type = %q, want local", info.StorageType)
	}
	if info.FileName != "avatar.png" {
		t.Fatalf("file name = %q, want avatar.png", info.FileName)
	}
	if info.MimeType != "image/png" {
		t.Fatalf("mime type = %q, want image/png", info.MimeType)
	}
	if !strings.HasPrefix(info.URL, "/files/") {
		t.Fatalf("url = %q, want /files/ prefix", info.URL)
	}
	if !strings.HasPrefix(info.FilePath, filepath.Clean(dir)+string(filepath.Separator)) {
		t.Fatalf("file path = %q, want inside %q", info.FilePath, dir)
	}
	if _, err := os.Stat(info.FilePath); err != nil {
		t.Fatalf("uploaded file does not exist: %v", err)
	}

	if err := uploader.Delete(info.FilePath); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if _, err := os.Stat(info.FilePath); !os.IsNotExist(err) {
		t.Fatalf("uploaded file still exists after delete, stat err: %v", err)
	}
}

func TestUploaderContextMethodsPassCanceledContextToProvider(t *testing.T) {
	uploader := &Uploader{
		config: config.UploadConfig{
			MaxSize:      1,
			AllowedTypes: []string{".png"},
		},
		provider: contextAwareStorageProvider{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := uploader.UploadContext(ctx, newMultipartFileHeader(t, "avatar.png", tinyPNG()))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("UploadContext error = %v, want context.Canceled", err)
	}

	if err := uploader.DeleteContext(ctx, "avatar.png"); !errors.Is(err, context.Canceled) {
		t.Fatalf("DeleteContext error = %v, want context.Canceled", err)
	}
}

func TestUploaderKeepsLegacyLocalConfig(t *testing.T) {
	dir := t.TempDir()
	uploader := NewUploaderWithConfig(config.UploadConfig{
		StorageType: "local",
		Local: config.LocalStorageConfig{
			Path:      dir,
			URLPrefix: "/uploads",
		},
		MaxSize:      1,
		AllowedTypes: []string{".png"},
	})

	info, err := uploader.Upload(newMultipartFileHeader(t, "legacy.png", tinyPNG()))
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	if !strings.HasPrefix(info.URL, "/uploads/") {
		t.Fatalf("url = %q, want legacy /uploads/ prefix", info.URL)
	}
}

type contextAwareStorageProvider struct{}

func (contextAwareStorageProvider) Type() string {
	return "test"
}

func (contextAwareStorageProvider) Store(ctx context.Context, _ string, _ io.Reader) (*StoredObject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &StoredObject{
		FilePath:    "avatar.png",
		URL:         "/uploads/avatar.png",
		StorageType: "test",
	}, nil
}

func (contextAwareStorageProvider) Open(ctx context.Context, _ string) (*StoredObjectReader, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, ErrStorageReadNotImplemented
}

func (contextAwareStorageProvider) Delete(ctx context.Context, _ string) error {
	return ctx.Err()
}

func (contextAwareStorageProvider) PublicURL(filePath string) (string, error) {
	return filePath, nil
}

func TestLocalStorageProviderOpenStreamsFile(t *testing.T) {
	dir := t.TempDir()
	provider := NewLocalStorageProvider(config.UploadConfig{
		StorageType: "local",
		LocalPath:   dir,
	})
	key := "artifacts/task-1/report.txt"
	targetPath := filepath.Join(dir, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("streamed artifact"), 0600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	opened, err := provider.Open(context.TODO(), key)
	if err != nil {
		t.Fatalf("open failed: %v", err)
	}
	defer opened.Body.Close()
	content, err := io.ReadAll(opened.Body)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if string(content) != "streamed artifact" {
		t.Fatalf("content = %q, want streamed artifact", string(content))
	}
	if opened.Key != key {
		t.Fatalf("key = %q, want %q", opened.Key, key)
	}
	if opened.Size != int64(len("streamed artifact")) {
		t.Fatalf("size = %d, want %d", opened.Size, len("streamed artifact"))
	}
}

func TestLocalStorageProviderOpenRejectsSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outsideDir := t.TempDir()
	provider := NewLocalStorageProvider(config.UploadConfig{
		StorageType: "local",
		LocalPath:   dir,
	})
	outsidePath := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outsidePath, []byte("secret"), 0600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	linkPath := filepath.Join(dir, "linked-secret.txt")
	if err := os.Symlink(outsidePath, linkPath); err != nil {
		t.Skipf("symlink is not available in this environment: %v", err)
	}

	_, err := provider.Open(context.TODO(), "linked-secret.txt")
	if !errors.Is(err, ErrStorageProviderUnavailable) {
		t.Fatalf("err = %v, want ErrStorageProviderUnavailable", err)
	}
	if err == nil || !strings.Contains(err.Error(), "resolves outside storage root") {
		t.Fatalf("err = %v, want symlink escape detail", err)
	}
}

func TestUploaderS3MissingConfigIsExplicit(t *testing.T) {
	uploader := NewUploaderWithConfig(config.UploadConfig{
		StorageType:  "s3",
		MaxSize:      1,
		AllowedTypes: []string{".png"},
	})

	_, err := uploader.Upload(newMultipartFileHeader(t, "avatar.png", tinyPNG()))
	if !errors.Is(err, ErrStorageProviderNotConfigured) {
		t.Fatalf("err = %v, want ErrStorageProviderNotConfigured", err)
	}
	if !strings.Contains(err.Error(), "upload.s3 missing") {
		t.Fatalf("err = %q, want missing s3 config details", err.Error())
	}
}

func TestS3StorageProviderStoreUploadsObject(t *testing.T) {
	const key = "uploads/2026/05/22/avatar.png"
	content := tinyPNG()
	var received []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method = %q, want PUT", r.Method)
		}
		if r.URL.Path != "/go-admin-kit/"+key {
			t.Fatalf("path = %q, want /go-admin-kit/%s", r.URL.Path, key)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		received = body
		w.Header().Set("ETag", `"uploaded-etag"`)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider, err := NewStorageProvider(config.UploadConfig{
		StorageType:   "s3",
		PublicBaseURL: "https://cdn.example.test/uploads",
		S3: config.ObjectStorageConfig{
			Endpoint:  server.URL,
			Bucket:    "go-admin-kit",
			Region:    "us-east-1",
			AccessKey: "access",
			SecretKey: "secret",
		},
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}

	stored, err := provider.Store(context.Background(), key, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("store failed: %v", err)
	}
	if !bytes.Contains(received, content) {
		t.Fatalf("uploaded body does not contain object content, length = %d, want content length %d", len(received), len(content))
	}
	if stored.Key != key {
		t.Fatalf("key = %q, want %q", stored.Key, key)
	}
	if stored.FilePath != key {
		t.Fatalf("file path = %q, want object key", stored.FilePath)
	}
	if stored.StorageType != "s3" {
		t.Fatalf("storage type = %q, want s3", stored.StorageType)
	}
	if stored.URL != "https://cdn.example.test/uploads/"+key {
		t.Fatalf("url = %q, want CDN URL", stored.URL)
	}
}

func TestS3StorageProviderOpenStreamsObject(t *testing.T) {
	const key = "artifacts/task-1/report.txt"
	const content = "streamed s3 artifact"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/go-admin-kit/"+key {
			t.Fatalf("path = %q, want /go-admin-kit/%s", r.URL.Path, key)
		}
		switch r.Method {
		case http.MethodHead:
			w.Header().Set("Content-Length", "20")
			w.Header().Set("Last-Modified", "Sat, 09 May 2026 00:00:00 GMT")
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			w.Header().Set("Content-Length", "20")
			w.Header().Set("Last-Modified", "Sat, 09 May 2026 00:00:00 GMT")
			_, _ = w.Write([]byte(content))
		default:
			t.Fatalf("method = %q, want HEAD or GET", r.Method)
		}
	}))
	defer server.Close()

	provider, err := NewStorageProvider(config.UploadConfig{
		StorageType:   "s3",
		PublicBaseURL: "https://cdn.example.test/uploads",
		S3: config.ObjectStorageConfig{
			Endpoint:  server.URL,
			Bucket:    "go-admin-kit",
			Region:    "us-east-1",
			AccessKey: "access",
			SecretKey: "secret",
		},
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}
	opened, err := provider.Open(context.TODO(), key)
	if err != nil {
		t.Fatalf("open failed: %v", err)
	}
	defer opened.Body.Close()
	body, err := io.ReadAll(opened.Body)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if string(body) != content {
		t.Fatalf("content = %q, want %q", string(body), content)
	}
	if opened.Key != key {
		t.Fatalf("key = %q, want %q", opened.Key, key)
	}
	if opened.FilePath != key {
		t.Fatalf("file path = %q, want object key", opened.FilePath)
	}
	if opened.StorageType != "s3" {
		t.Fatalf("storage type = %q, want s3", opened.StorageType)
	}
	if opened.Size != int64(len(content)) {
		t.Fatalf("size = %d, want %d", opened.Size, len(content))
	}
}

func TestMinIOStorageProviderOpenWrapsSDKError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "backend unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	provider, err := NewStorageProvider(config.UploadConfig{
		StorageType:   "minio",
		PublicBaseURL: "http://127.0.0.1:9000/go-admin-kit",
		MinIO: config.ObjectStorageConfig{
			Endpoint:  server.URL,
			Bucket:    "go-admin-kit",
			Region:    "us-east-1",
			AccessKey: "minioadmin",
			SecretKey: "minioadmin",
			UseSSL:    false,
		},
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}

	_, err = provider.Open(context.TODO(), "artifacts/task-1/missing.txt")
	if !errors.Is(err, ErrStorageProviderUnavailable) {
		t.Fatalf("err = %v, want ErrStorageProviderUnavailable", err)
	}
	if err == nil || !strings.Contains(err.Error(), "minio stat") {
		t.Fatalf("err = %v, want minio stat detail", err)
	}
}

func TestMinIOStorageProviderDeleteRemovesObject(t *testing.T) {
	const key = "uploads/2026/05/22/avatar.png"
	deleted := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("method = %q, want DELETE", r.Method)
		}
		if r.URL.Path != "/go-admin-kit/"+key {
			t.Fatalf("path = %q, want /go-admin-kit/%s", r.URL.Path, key)
		}
		deleted = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	provider, err := NewStorageProvider(config.UploadConfig{
		StorageType:   "minio",
		PublicBaseURL: "http://127.0.0.1:9000/go-admin-kit",
		MinIO: config.ObjectStorageConfig{
			Endpoint:  server.URL,
			Bucket:    "go-admin-kit",
			Region:    "us-east-1",
			AccessKey: "minio-access",
			SecretKey: "minio-secret",
			UseSSL:    false,
		},
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}

	if err := provider.Delete(context.Background(), key); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if !deleted {
		t.Fatal("delete request was not sent")
	}
}

func TestObjectStorageProviderOpenRejectsRawURL(t *testing.T) {
	provider, err := NewStorageProvider(config.UploadConfig{
		StorageType:   "s3",
		PublicBaseURL: "https://cdn.example.test/uploads",
		S3: config.ObjectStorageConfig{
			Endpoint:  "https://s3.example.test",
			Bucket:    "go-admin-kit",
			Region:    "us-east-1",
			AccessKey: "access",
			SecretKey: "secret",
		},
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}

	_, err = provider.Open(context.TODO(), "https://evil.example.test/artifacts/task-1/report.txt")
	if !errors.Is(err, ErrStorageProviderNotConfigured) {
		t.Fatalf("err = %v, want ErrStorageProviderNotConfigured", err)
	}
	if err == nil || !strings.Contains(err.Error(), "object key must not be a URL") {
		t.Fatalf("err = %v, want raw URL detail", err)
	}
}

func TestUploaderMinIOConfiguredUploadsObject(t *testing.T) {
	var received []byte
	var objectPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method = %q, want PUT", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/go-admin-kit/") {
			t.Fatalf("path = %q, want /go-admin-kit/ prefix", r.URL.Path)
		}
		objectPath = strings.TrimPrefix(r.URL.Path, "/go-admin-kit/")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		received = body
		w.Header().Set("ETag", `"uploaded-etag"`)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	uploader := NewUploaderWithConfig(config.UploadConfig{
		StorageType:   "minio",
		PublicBaseURL: "http://cdn.example.test/go-admin-kit",
		MinIO: config.ObjectStorageConfig{
			Endpoint:  server.URL,
			Bucket:    "go-admin-kit",
			Region:    "us-east-1",
			AccessKey: "minio-access",
			SecretKey: "minio-secret",
			UseSSL:    false,
		},
		MaxSize:      1,
		AllowedTypes: []string{".png"},
	})

	info, err := uploader.Upload(newMultipartFileHeader(t, "avatar.png", tinyPNG()))
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	if len(received) == 0 {
		t.Fatal("upload request body was empty")
	}
	if info.StorageType != "minio" {
		t.Fatalf("storage type = %q, want minio", info.StorageType)
	}
	if info.FilePath == "" || strings.HasPrefix(info.FilePath, "http") {
		t.Fatalf("file path = %q, want object key", info.FilePath)
	}
	if info.FilePath != objectPath {
		t.Fatalf("file path = %q, want uploaded path %q", info.FilePath, objectPath)
	}
	if info.URL != "http://cdn.example.test/go-admin-kit/"+objectPath {
		t.Fatalf("url = %q, want CDN object URL", info.URL)
	}
}

func newMultipartFileHeader(t *testing.T, filename string, content []byte) *multipart.FileHeader {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	reader := multipart.NewReader(&body, writer.Boundary())
	form, err := reader.ReadForm(int64(body.Len()))
	if err != nil {
		t.Fatalf("read form: %v", err)
	}
	t.Cleanup(func() {
		_ = form.RemoveAll()
	})
	return form.File["file"][0]
}

func tinyPNG() []byte {
	return []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0a, 0x49, 0x44, 0x41,
		0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00,
		0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
		0x42, 0x60, 0x82,
	}
}
