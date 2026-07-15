package upload

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-admin-kit/services/file/internal/config"
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

	info, err := uploader.UploadContext(context.Background(), newMultipartFileHeader(t, "avatar.png", tinyPNG()))
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

	if err := uploader.DeleteContext(context.Background(), info.FilePath); err != nil {
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

	if _, err := uploader.OpenContext(ctx, "avatar.png"); !errors.Is(err, context.Canceled) {
		t.Fatalf("OpenContext error = %v, want context.Canceled", err)
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

	info, err := uploader.UploadContext(context.Background(), newMultipartFileHeader(t, "legacy.png", tinyPNG()))
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	if !strings.HasPrefix(info.URL, "/uploads/") {
		t.Fatalf("url = %q, want legacy /uploads/ prefix", info.URL)
	}
}

func TestUploaderReportsStandardImageDimensions(t *testing.T) {
	tests := []struct {
		name       string
		filename   string
		content    []byte
		wantWidth  int
		wantHeight int
	}{
		{
			name:       "png",
			filename:   "avatar.png",
			content:    fixtureBase64(t, "iVBORw0KGgoAAAANSUhEUgAAAAIAAAADCAYAAAC56t6BAAAAAXNSR0IArs4c6QAAAARnQU1BAACxjwv8YQUAAAAJcEhZcwAADsMAAA7DAcdvqGQAAAAiSURBVBhXY9CImvbfJmrafwaNvGn/bfJAjKZp/22apv0HAKfrDT/onk38AAAAAElFTkSuQmCC"),
			wantWidth:  2,
			wantHeight: 3,
		},
		{
			name:       "jpeg",
			filename:   "avatar.jpg",
			content:    fixtureBase64(t, "/9j/4AAQSkZJRgABAQEAYABgAAD/2wBDAAMCAgMCAgMDAwMEAwMEBQgFBQQEBQoHBwYIDAoMDAsKCwsNDhIQDQ4RDgsLEBYQERMUFRUVDA8XGBYUGBIUFRT/2wBDAQMEBAUEBQkFBQkUDQsNFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBT/wAARCAACAAMDASIAAhEBAxEB/8QAHwAAAQUBAQEBAQEAAAAAAAAAAAECAwQFBgcICQoL/8QAtRAAAgEDAwIEAwUFBAQAAAF9AQIDAAQRBRIhMUEGE1FhByJxFDKBkaEII0KxwRVS0fAkM2JyggkKFhcYGRolJicoKSo0NTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqDhIWGh4iJipKTlJWWl5iZmqKjpKWmp6ipqrKztLW2t7i5usLDxMXGx8jJytLT1NXW19jZ2uHi4+Tl5ufo6erx8vP09fb3+Pn6/8QAHwEAAwEBAQEBAQEBAQAAAAAAAAECAwQFBgcICQoL/8QAtREAAgECBAQDBAcFBAQAAQJ3AAECAxEEBSExBhJBUQdhcRMiMoEIFEKRobHBCSMzUvAVYnLRChYkNOEl8RcYGRomJygpKjU2Nzg5OkNERUZHSElKU1RVVldYWVpjZGVmZ2hpanN0dXZ3eHl6goOEhYaHiImKkpOUlZaXmJmaoqOkpaanqKmqsrO0tba3uLm6wsPExcbHyMnK0tPU1dbX2Nna4uPk5ebn6Onq8vP09fb3+Pn6/9oADAMBAAIRAxEAPwDzqzt4vssf7pOn90UUUV+nS3Z8rS/hx9Ef/9k="),
			wantWidth:  3,
			wantHeight: 2,
		},
		{
			name:       "gif",
			filename:   "avatar.gif",
			content:    fixtureBase64(t, "R0lGODlhBAAFAPcAAAAAAAAAMwAAZgAAmQAAzAAA/wArAAArMwArZgArmQArzAAr/wBVAABVMwBVZgBVmQBVzABV/wCAAACAMwCAZgCAmQCAzACA/wCqAACqMwCqZgCqmQCqzACq/wDVAADVMwDVZgDVmQDVzADV/wD/AAD/MwD/ZgD/mQD/zAD//zMAADMAMzMAZjMAmTMAzDMA/zMrADMrMzMrZjMrmTMrzDMr/zNVADNVMzNVZjNVmTNVzDNV/zOAADOAMzOAZjOAmTOAzDOA/zOqADOqMzOqZjOqmTOqzDOq/zPVADPVMzPVZjPVmTPVzDPV/zP/ADP/MzP/ZjP/mTP/zDP//2YAAGYAM2YAZmYAmWYAzGYA/2YrAGYrM2YrZmYrmWYrzGYr/2ZVAGZVM2ZVZmZVmWZVzGZV/2aAAGaAM2aAZmaAmWaAzGaA/2aqAGaqM2aqZmaqmWaqzGaq/2bVAGbVM2bVZmbVmWbVzGbV/2b/AGb/M2b/Zmb/mWb/zGb//5kAAJkAM5kAZpkAmZkAzJkA/5krAJkrM5krZpkrmZkrzJkr/5lVAJlVM5lVZplVmZlVzJlV/5mAAJmAM5mAZpmAmZmAzJmA/5mqAJmqM5mqZpmqmZmqzJmq/5nVAJnVM5nVZpnVmZnVzJnV/5n/AJn/M5n/Zpn/mZn/zJn//8wAAMwAM8wAZswAmcwAzMwA/8wrAMwrM8wrZswrmcwrzMwr/8xVAMxVM8xVZsxVmcxVzMxV/8yAAMyAM8yAZsyAmcyAzMyA/8yqAMyqM8yqZsyqmcyqzMyq/8zVAMzVM8zVZszVmczVzMzV/8z/AMz/M8z/Zsz/mcz/zMz///8AAP8AM/8AZv8Amf8AzP8A//8rAP8rM/8rZv8rmf8rzP8r//9VAP9VM/9VZv9Vmf9VzP9V//+AAP+AM/+AZv+Amf+AzP+A//+qAP+qM/+qZv+qmf+qzP+q///VAP/VM//VZv/Vmf/VzP/V////AP//M///Zv//mf//zP///wAAAAAAAAAAAAAAACH5BAEAAPwALAAAAAAEAAUAAAgVAHMIHPMjRxqCP36kKVLkzUKGbwICADs="),
			wantWidth:  4,
			wantHeight: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uploader := newLocalTestUploader(t, ".png", ".jpg", ".gif")

			info, err := uploader.UploadContext(context.Background(), newMultipartFileHeader(t, tt.filename, tt.content))
			if err != nil {
				t.Fatalf("upload failed: %v", err)
			}

			if info.ImageWidth != tt.wantWidth {
				t.Fatalf("image width = %d, want %d", info.ImageWidth, tt.wantWidth)
			}
			if info.ImageHeight != tt.wantHeight {
				t.Fatalf("image height = %d, want %d", info.ImageHeight, tt.wantHeight)
			}
		})
	}
}

func TestUploaderResetsReaderAfterReadingImageDimensions(t *testing.T) {
	content := fixtureBase64(t, "iVBORw0KGgoAAAANSUhEUgAAAAIAAAADCAYAAAC56t6BAAAAAXNSR0IArs4c6QAAAARnQU1BAACxjwv8YQUAAAAJcEhZcwAADsMAAA7DAcdvqGQAAAAiSURBVBhXY9CImvbfJmrafwaNvGn/bfJAjKZp/22apv0HAKfrDT/onk38AAAAAElFTkSuQmCC")
	uploader := newLocalTestUploader(t, ".png")

	info, err := uploader.UploadContext(context.Background(), newMultipartFileHeader(t, "avatar.png", content))
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}

	stored, err := os.ReadFile(info.FilePath)
	if err != nil {
		t.Fatalf("read stored file: %v", err)
	}
	if !bytes.Equal(stored, content) {
		t.Fatalf("stored content length = %d, want original length %d", len(stored), len(content))
	}
	if info.Hash != md5Hex(content) {
		t.Fatalf("hash = %q, want %q", info.Hash, md5Hex(content))
	}
}

func TestUploaderUsesZeroDimensionsForUnsupportedImageAndNonImage(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		content  []byte
		allowed  []string
	}{
		{
			name:     "webp",
			filename: "avatar.webp",
			content:  []byte("RIFF\x0c\x00\x00\x00WEBPVP8 \x00\x00\x00\x00"),
			allowed:  []string{".webp"},
		},
		{
			name:     "pdf",
			filename: "report.pdf",
			content:  []byte("%PDF-1.4\n%test pdf\n1 0 obj\n<<>>\nendobj\n"),
			allowed:  []string{".pdf"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uploader := newLocalTestUploader(t, tt.allowed...)

			info, err := uploader.UploadContext(context.Background(), newMultipartFileHeader(t, tt.filename, tt.content))
			if err != nil {
				t.Fatalf("upload failed: %v", err)
			}

			if info.ImageWidth != 0 {
				t.Fatalf("image width = %d, want 0", info.ImageWidth)
			}
			if info.ImageHeight != 0 {
				t.Fatalf("image height = %d, want 0", info.ImageHeight)
			}
		})
	}
}

func TestUploaderGeneratesPNGThumbnailWithinConfiguredBounds(t *testing.T) {
	content := testPNG(t, 400, 200)
	uploader := NewUploaderWithConfig(config.UploadConfig{
		StorageType:   "local",
		LocalPath:     t.TempDir(),
		PublicBaseURL: "/files",
		MaxSize:       1,
		AllowedTypes:  []string{".png"},
		Image: config.ImageConfig{
			ThumbnailWidth:  100,
			ThumbnailHeight: 100,
		},
	})

	info, err := uploader.UploadContext(context.Background(), newMultipartFileHeader(t, "banner.png", content))
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}

	if info.ThumbnailPath == "" {
		t.Fatal("thumbnail path is empty")
	}
	if !strings.Contains(filepath.ToSlash(info.ThumbnailPath), "/thumbs/") {
		t.Fatalf("thumbnail path = %q, want thumbs directory", info.ThumbnailPath)
	}
	if !strings.HasSuffix(info.ThumbnailPath, "_100x50.png") {
		t.Fatalf("thumbnail path = %q, want _100x50.png suffix", info.ThumbnailPath)
	}
	if info.ThumbnailURL == "" {
		t.Fatal("thumbnail URL is empty")
	}
	if info.ThumbnailWidth != 100 {
		t.Fatalf("thumbnail width = %d, want 100", info.ThumbnailWidth)
	}
	if info.ThumbnailHeight != 50 {
		t.Fatalf("thumbnail height = %d, want 50", info.ThumbnailHeight)
	}

	thumbFile, err := os.Open(info.ThumbnailPath)
	if err != nil {
		t.Fatalf("open thumbnail: %v", err)
	}
	defer thumbFile.Close()
	cfg, format, err := image.DecodeConfig(thumbFile)
	if err != nil {
		t.Fatalf("decode thumbnail config: %v", err)
	}
	if format != "png" {
		t.Fatalf("thumbnail format = %q, want png", format)
	}
	if cfg.Width != 100 || cfg.Height != 50 {
		t.Fatalf("thumbnail dimensions = %dx%d, want 100x50", cfg.Width, cfg.Height)
	}
}

func TestUploaderThumbnailUsesDefaultsAndDoesNotUpscale(t *testing.T) {
	content := testPNG(t, 80, 40)
	uploader := newLocalTestUploader(t, ".png")

	info, err := uploader.UploadContext(context.Background(), newMultipartFileHeader(t, "small.png", content))
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}

	if info.ThumbnailWidth != 80 {
		t.Fatalf("thumbnail width = %d, want original width 80", info.ThumbnailWidth)
	}
	if info.ThumbnailHeight != 40 {
		t.Fatalf("thumbnail height = %d, want original height 40", info.ThumbnailHeight)
	}
	if !strings.HasSuffix(info.ThumbnailPath, "_80x40.png") {
		t.Fatalf("thumbnail path = %q, want _80x40.png suffix", info.ThumbnailPath)
	}
}

func TestUploaderThumbnailSkipsWebPAndNonImage(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		content  []byte
		allowed  []string
	}{
		{
			name:     "webp",
			filename: "avatar.webp",
			content:  []byte("RIFF\x0c\x00\x00\x00WEBPVP8 \x00\x00\x00\x00"),
			allowed:  []string{".webp"},
		},
		{
			name:     "pdf",
			filename: "report.pdf",
			content:  []byte("%PDF-1.4\n%test pdf\n1 0 obj\n<<>>\nendobj\n"),
			allowed:  []string{".pdf"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uploader := newLocalTestUploader(t, tt.allowed...)

			info, err := uploader.UploadContext(context.Background(), newMultipartFileHeader(t, tt.filename, tt.content))
			if err != nil {
				t.Fatalf("upload failed: %v", err)
			}

			if info.ThumbnailPath != "" {
				t.Fatalf("thumbnail path = %q, want empty", info.ThumbnailPath)
			}
			if info.ThumbnailURL != "" {
				t.Fatalf("thumbnail URL = %q, want empty", info.ThumbnailURL)
			}
			if info.ThumbnailWidth != 0 || info.ThumbnailHeight != 0 {
				t.Fatalf("thumbnail dimensions = %dx%d, want 0x0", info.ThumbnailWidth, info.ThumbnailHeight)
			}
		})
	}
}

func TestUploaderThumbnailStoreFailureDoesNotBlockUpload(t *testing.T) {
	uploader := &Uploader{
		config: config.UploadConfig{
			MaxSize:      1,
			AllowedTypes: []string{".png"},
			Image: config.ImageConfig{
				ThumbnailWidth:  20,
				ThumbnailHeight: 20,
			},
		},
		provider: thumbnailFailingStorageProvider{},
	}

	info, err := uploader.UploadContext(context.Background(), newMultipartFileHeader(t, "avatar.png", testPNG(t, 40, 40)))
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}

	if info.FilePath == "" {
		t.Fatal("file path is empty, want original upload to succeed")
	}
	if info.ThumbnailPath != "" || info.ThumbnailURL != "" || info.ThumbnailWidth != 0 || info.ThumbnailHeight != 0 {
		t.Fatalf("thumbnail = %q %q %dx%d, want empty/0 after store failure", info.ThumbnailPath, info.ThumbnailURL, info.ThumbnailWidth, info.ThumbnailHeight)
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

type thumbnailFailingStorageProvider struct{}

func (thumbnailFailingStorageProvider) Type() string {
	return "test"
}

func (thumbnailFailingStorageProvider) Store(_ context.Context, objectKey string, body io.Reader) (*StoredObject, error) {
	if strings.Contains(objectKey, "/thumbs/") {
		return nil, errors.New("thumbnail store failed")
	}
	if _, err := io.Copy(io.Discard, body); err != nil {
		return nil, err
	}
	return &StoredObject{
		FilePath:    objectKey,
		URL:         "/uploads/" + objectKey,
		StorageType: "test",
	}, nil
}

func (thumbnailFailingStorageProvider) Open(context.Context, string) (*StoredObjectReader, error) {
	return nil, ErrStorageReadNotImplemented
}

func (thumbnailFailingStorageProvider) Delete(context.Context, string) error {
	return nil
}

func (thumbnailFailingStorageProvider) PublicURL(filePath string) (string, error) {
	return "/uploads/" + filePath, nil
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

	_, err := uploader.UploadContext(context.Background(), newMultipartFileHeader(t, "avatar.png", tinyPNG()))
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

func TestMinIOStorageProviderOpenMapsMissingObject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`<Error><Code>NoSuchKey</Code><Message>not found</Message></Error>`))
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
	if !errors.Is(err, ErrStoredObjectNotFound) {
		t.Fatalf("err = %v, want ErrStoredObjectNotFound", err)
	}
}

func TestLocalStorageProviderOpenMapsMissingFile(t *testing.T) {
	provider := NewLocalStorageProvider(config.UploadConfig{
		StorageType: "local",
		LocalPath:   t.TempDir(),
	})

	_, err := provider.Open(context.TODO(), "missing.txt")
	if !errors.Is(err, ErrStoredObjectNotFound) {
		t.Fatalf("err = %v, want ErrStoredObjectNotFound", err)
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
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if !strings.Contains(r.URL.Path, "/thumbs/") {
			objectPath = strings.TrimPrefix(r.URL.Path, "/go-admin-kit/")
			received = body
		}
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

	info, err := uploader.UploadContext(context.Background(), newMultipartFileHeader(t, "avatar.png", tinyPNG()))
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

func newLocalTestUploader(t *testing.T, allowedTypes ...string) *Uploader {
	t.Helper()

	return NewUploaderWithConfig(config.UploadConfig{
		StorageType:   "local",
		LocalPath:     t.TempDir(),
		PublicBaseURL: "/files",
		MaxSize:       1,
		AllowedTypes:  allowedTypes,
	})
}

func fixtureBase64(t *testing.T, value string) []byte {
	t.Helper()

	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return decoded
}

func md5Hex(content []byte) string {
	sum := md5.Sum(content)
	return hex.EncodeToString(sum[:])
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

func testPNG(t *testing.T, width, height int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 120, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}
