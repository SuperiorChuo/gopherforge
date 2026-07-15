package system

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
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/services/file/internal/config"
	systemdao "github.com/go-admin-kit/services/file/internal/dao/system"
	"github.com/go-admin-kit/services/file/internal/model"
	"github.com/go-admin-kit/services/file/internal/pkg/authz"
	"github.com/go-admin-kit/services/file/internal/pkg/pagination"
	"github.com/go-admin-kit/services/file/internal/pkg/upload"
	"gorm.io/gorm"
)

func TestFileServiceUploadContextPersistsImageDimensions(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	content := systemFixtureBase64(t, "iVBORw0KGgoAAAANSUhEUgAAAAIAAAADCAYAAAC56t6BAAAAAXNSR0IArs4c6QAAAARnQU1BAACxjwv8YQUAAAAJcEhZcwAADsMAAA7DAcdvqGQAAAAiSURBVBhXY9CImvbfJmrafwaNvGn/bfJAjKZp/22apv0HAKfrDT/onk38AAAAAElFTkSuQmCC")
	hash := systemMD5Hex(content)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "files" WHERE hash = $1 ORDER BY "files"."id" LIMIT $2`)).
		WithArgs(hash, 1).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "files" ("user_id","file_name","file_path","file_size","image_width","image_height","thumbnail_path","thumbnail_url","thumbnail_width","thumbnail_height","file_type","mime_type","extension","storage_type","url","hash","created_at","updated_at") VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18) RETURNING "id"`)).
		WithArgs(uint(42), "avatar.png", sqlmock.AnyArg(), int64(len(content)), 2, 3, sqlmock.AnyArg(), sqlmock.AnyArg(), 2, 3, "image", "image/png", ".png", "local", sqlmock.AnyArg(), hash, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectCommit()

	service := newLocalUploadFileService(t, db, ".png")
	file, err := service.UploadContext(context.Background(), systemMultipartFileHeader(t, "avatar.png", content), 42)
	if err != nil {
		t.Fatalf("UploadContext() error = %v", err)
	}
	if file.ImageWidth != 2 {
		t.Fatalf("image width = %d, want 2", file.ImageWidth)
	}
	if file.ImageHeight != 3 {
		t.Fatalf("image height = %d, want 3", file.ImageHeight)
	}
}

func TestFileServiceUploadContextReusesExistingImageDimensionsForDuplicateHash(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	content := systemFixtureBase64(t, "iVBORw0KGgoAAAANSUhEUgAAAAIAAAADCAYAAAC56t6BAAAAAXNSR0IArs4c6QAAAARnQU1BAACxjwv8YQUAAAAJcEhZcwAADsMAAA7DAcdvqGQAAAAiSURBVBhXY9CImvbfJmrafwaNvGn/bfJAjKZp/22apv0HAKfrDT/onk38AAAAAElFTkSuQmCC")
	hash := systemMD5Hex(content)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "files" WHERE hash = $1 ORDER BY "files"."id" LIMIT $2`)).
		WithArgs(hash, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "file_name", "file_path", "file_size", "image_width", "image_height",
			"file_type", "mime_type", "extension", "storage_type", "url", "hash",
		}).AddRow(uint(7), uint(9), "stored.png", "2026/05/23/stored.png", int64(123), 640, 480, "image", "image/png", ".png", "local", "/files/stored.png", hash))
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "files" ("user_id","file_name","file_path","file_size","image_width","image_height","thumbnail_path","thumbnail_url","thumbnail_width","thumbnail_height","file_type","mime_type","extension","storage_type","url","hash","created_at","updated_at") VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18) RETURNING "id"`)).
		WithArgs(uint(42), "avatar.png", "2026/05/23/stored.png", int64(123), 640, 480, "", "", 0, 0, "image", "image/png", ".png", "local", "/files/stored.png", hash, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(8))
	mock.ExpectCommit()

	service := newLocalUploadFileService(t, db, ".png")
	file, err := service.UploadContext(context.Background(), systemMultipartFileHeader(t, "avatar.png", content), 42)
	if err != nil {
		t.Fatalf("UploadContext() error = %v", err)
	}
	if file.ImageWidth != 640 {
		t.Fatalf("image width = %d, want 640", file.ImageWidth)
	}
	if file.ImageHeight != 480 {
		t.Fatalf("image height = %d, want 480", file.ImageHeight)
	}
}

func TestFileServiceUploadContextPersistsThumbnailFields(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	content := systemPNG(t, 400, 200)
	hash := systemMD5Hex(content)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "files" WHERE hash = $1 ORDER BY "files"."id" LIMIT $2`)).
		WithArgs(hash, 1).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "files" ("user_id","file_name","file_path","file_size","image_width","image_height","thumbnail_path","thumbnail_url","thumbnail_width","thumbnail_height","file_type","mime_type","extension","storage_type","url","hash","created_at","updated_at") VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18) RETURNING "id"`)).
		WithArgs(uint(42), "avatar.png", sqlmock.AnyArg(), int64(len(content)), 400, 200, sqlmock.AnyArg(), sqlmock.AnyArg(), 100, 50, "image", "image/png", ".png", "local", sqlmock.AnyArg(), hash, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectCommit()

	service := newLocalUploadFileServiceWithImage(t, db, config.ImageConfig{ThumbnailWidth: 100, ThumbnailHeight: 100}, ".png")
	file, err := service.UploadContext(context.Background(), systemMultipartFileHeader(t, "avatar.png", content), 42)
	if err != nil {
		t.Fatalf("UploadContext() error = %v", err)
	}
	if file.ThumbnailPath == "" {
		t.Fatal("thumbnail path is empty")
	}
	if file.ThumbnailURL == "" {
		t.Fatal("thumbnail URL is empty")
	}
	if file.ThumbnailWidth != 100 {
		t.Fatalf("thumbnail width = %d, want 100", file.ThumbnailWidth)
	}
	if file.ThumbnailHeight != 50 {
		t.Fatalf("thumbnail height = %d, want 50", file.ThumbnailHeight)
	}
}

func TestFileServiceUploadContextReusesExistingThumbnailFieldsForDuplicateHash(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	content := systemPNG(t, 400, 200)
	hash := systemMD5Hex(content)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "files" WHERE hash = $1 ORDER BY "files"."id" LIMIT $2`)).
		WithArgs(hash, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "file_name", "file_path", "file_size", "image_width", "image_height",
			"thumbnail_path", "thumbnail_url", "thumbnail_width", "thumbnail_height",
			"file_type", "mime_type", "extension", "storage_type", "url", "hash",
		}).AddRow(uint(7), uint(9), "stored.png", "2026/05/23/stored.png", int64(123), 640, 480,
			"2026/05/23/thumbs/stored_120x90.png", "/files/2026/05/23/thumbs/stored_120x90.png", 120, 90,
			"image", "image/png", ".png", "local", "/files/stored.png", hash))
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "files" ("user_id","file_name","file_path","file_size","image_width","image_height","thumbnail_path","thumbnail_url","thumbnail_width","thumbnail_height","file_type","mime_type","extension","storage_type","url","hash","created_at","updated_at") VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18) RETURNING "id"`)).
		WithArgs(uint(42), "avatar.png", "2026/05/23/stored.png", int64(123), 640, 480,
			"2026/05/23/thumbs/stored_120x90.png", "/files/2026/05/23/thumbs/stored_120x90.png", 120, 90,
			"image", "image/png", ".png", "local", "/files/stored.png", hash, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(8))
	mock.ExpectCommit()

	service := newLocalUploadFileServiceWithImage(t, db, config.ImageConfig{ThumbnailWidth: 100, ThumbnailHeight: 100}, ".png")
	file, err := service.UploadContext(context.Background(), systemMultipartFileHeader(t, "avatar.png", content), 42)
	if err != nil {
		t.Fatalf("UploadContext() error = %v", err)
	}
	if file.ThumbnailPath != "2026/05/23/thumbs/stored_120x90.png" {
		t.Fatalf("thumbnail path = %q, want existing thumbnail path", file.ThumbnailPath)
	}
	if file.ThumbnailWidth != 120 || file.ThumbnailHeight != 90 {
		t.Fatalf("thumbnail dimensions = %dx%d, want 120x90", file.ThumbnailWidth, file.ThumbnailHeight)
	}
}

func TestFileServiceDeleteFileContextDeletesThumbnailBestEffort(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	dir := t.TempDir()
	originalPath := dir + "/avatar.png"
	thumbnailPath := dir + "/thumbs/avatar_20x20.png"
	if err := os.MkdirAll(dir+"/thumbs", 0o755); err != nil {
		t.Fatalf("mkdir thumbs: %v", err)
	}
	if err := os.WriteFile(originalPath, []byte("original"), 0o600); err != nil {
		t.Fatalf("write original: %v", err)
	}
	if err := os.WriteFile(thumbnailPath, []byte("thumbnail"), 0o600); err != nil {
		t.Fatalf("write thumbnail: %v", err)
	}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "files" WHERE id = $1 ORDER BY "files"."id" LIMIT $2`)).
		WithArgs(7, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "file_name", "file_path", "thumbnail_path", "storage_type",
		}).AddRow(uint(7), uint(42), "avatar.png", originalPath, thumbnailPath, "local"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "files" WHERE storage_type = $1 AND file_path = $2 AND id <> $3`)).
		WithArgs("local", originalPath, uint(7)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "files" WHERE storage_type = $1 AND thumbnail_path = $2 AND id <> $3`)).
		WithArgs("local", thumbnailPath, uint(7)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "files" WHERE "files"."id" = $1`)).
		WithArgs(uint(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	service := &FileService{
		fileDAO: *systemdao.NewFileDAO(db),
		uploader: upload.NewUploaderWithConfig(config.UploadConfig{
			StorageType: "local",
			LocalPath:   dir,
		}),
	}
	err := service.DeleteFileContext(context.Background(), 7, 42, authz.UserDataScope{Scope: authz.DataScopeAll})
	if err != nil {
		t.Fatalf("DeleteFileContext() error = %v", err)
	}
	if _, err := os.Stat(thumbnailPath); !os.IsNotExist(err) {
		t.Fatalf("thumbnail still exists after delete, stat err: %v", err)
	}
}

func TestFileServiceDeleteFileContextKeepsSharedOriginalAndThumbnail(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	dir := t.TempDir()
	originalPath := dir + "/avatar.png"
	thumbnailPath := dir + "/thumbs/avatar_20x20.png"
	if err := os.MkdirAll(dir+"/thumbs", 0o755); err != nil {
		t.Fatalf("mkdir thumbs: %v", err)
	}
	if err := os.WriteFile(originalPath, []byte("original"), 0o600); err != nil {
		t.Fatalf("write original: %v", err)
	}
	if err := os.WriteFile(thumbnailPath, []byte("thumbnail"), 0o600); err != nil {
		t.Fatalf("write thumbnail: %v", err)
	}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "files" WHERE id = $1 ORDER BY "files"."id" LIMIT $2`)).
		WithArgs(7, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "file_name", "file_path", "thumbnail_path", "storage_type",
		}).AddRow(uint(7), uint(42), "avatar.png", originalPath, thumbnailPath, "local"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "files" WHERE storage_type = $1 AND file_path = $2 AND id <> $3`)).
		WithArgs("local", originalPath, uint(7)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "files" WHERE storage_type = $1 AND thumbnail_path = $2 AND id <> $3`)).
		WithArgs("local", thumbnailPath, uint(7)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "files" WHERE "files"."id" = $1`)).
		WithArgs(uint(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	service := &FileService{
		fileDAO: *systemdao.NewFileDAO(db),
		uploader: upload.NewUploaderWithConfig(config.UploadConfig{
			StorageType: "local",
			LocalPath:   dir,
		}),
	}
	err := service.DeleteFileContext(context.Background(), 7, 42, authz.UserDataScope{Scope: authz.DataScopeAll})
	if err != nil {
		t.Fatalf("DeleteFileContext() error = %v", err)
	}
	if _, err := os.Stat(originalPath); err != nil {
		t.Fatalf("shared original stat err: %v", err)
	}
	if _, err := os.Stat(thumbnailPath); err != nil {
		t.Fatalf("shared thumbnail stat err: %v", err)
	}
}

func TestFileServiceGetFileListContextHonorsCanceledContext(t *testing.T) {
	db, _ := setupSystemUserServiceContextTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := NewFileServiceWithDB(db).GetFileListContext(ctx, FileListRequest{
		PageRequest: pagination.PageRequest{Page: 1, PageSize: 10},
		DataScope:   authz.UserDataScope{Scope: authz.DataScopeAll},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetFileListContext() error = %v, want context.Canceled", err)
	}
}

func TestFileServiceGetFileByIDInScopeContextReturnsNotFoundSentinel(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "files" WHERE id = $1 ORDER BY "files"."id" LIMIT $2`)).
		WithArgs(7, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := NewFileServiceWithDB(db).GetFileByIDInScopeContext(context.Background(), 7, authz.UserDataScope{
		Scope: authz.DataScopeAll,
	})
	if !errors.Is(err, ErrFileNotFoundOrPermissionDenied) {
		t.Fatalf("GetFileByIDInScopeContext() error = %v, want ErrFileNotFoundOrPermissionDenied", err)
	}
}

func TestFileServiceDeleteFileContextReturnsLookupError(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	lookupErr := errors.New("database lookup failed")
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "files" WHERE id = $1 ORDER BY "files"."id" LIMIT $2`)).
		WithArgs(7, 1).
		WillReturnError(lookupErr)

	err := NewFileServiceWithDB(db).DeleteFileContext(context.Background(), 7, 1, authz.UserDataScope{
		Scope: authz.DataScopeAll,
	})
	if !errors.Is(err, lookupErr) {
		t.Fatalf("DeleteFileContext() error = %v, want lookup error", err)
	}
}

func TestFileServiceOpenFileContentContextStreamsObjectStorageKey(t *testing.T) {
	const key = "artifacts/task-7/report.txt"
	const content = "object storage content"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/go-admin-kit/"+key {
			t.Fatalf("path = %q, want /go-admin-kit/%s", r.URL.Path, key)
		}
		switch r.Method {
		case http.MethodHead:
			w.Header().Set("Content-Length", "22")
			w.Header().Set("Last-Modified", "Sat, 09 May 2026 00:00:00 GMT")
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			w.Header().Set("Content-Length", "22")
			w.Header().Set("Last-Modified", "Sat, 09 May 2026 00:00:00 GMT")
			_, _ = w.Write([]byte(content))
		default:
			t.Fatalf("method = %q, want HEAD or GET", r.Method)
		}
	}))
	defer server.Close()

	service := &FileService{
		uploader: upload.NewUploaderWithConfig(config.UploadConfig{
			StorageType: "s3",
			S3: config.ObjectStorageConfig{
				Endpoint:  server.URL,
				Bucket:    "go-admin-kit",
				Region:    "us-east-1",
				AccessKey: "access",
				SecretKey: "secret",
			},
		}),
	}

	opened, err := service.OpenFileContentContext(context.Background(), &model.File{
		FileName: "report.txt",
		FilePath: key,
		MimeType: "text/plain",
	})
	if err != nil {
		t.Fatalf("OpenFileContentContext() error = %v", err)
	}
	defer opened.Body.Close()

	body, err := io.ReadAll(opened.Body)
	if err != nil {
		t.Fatalf("read opened body: %v", err)
	}
	if string(body) != content {
		t.Fatalf("content = %q, want %q", string(body), content)
	}
	if opened.FileName != "report.txt" {
		t.Fatalf("file name = %q, want report.txt", opened.FileName)
	}
	if opened.ContentType != "text/plain" {
		t.Fatalf("content type = %q, want text/plain", opened.ContentType)
	}
	if opened.Size != int64(len(content)) {
		t.Fatalf("size = %d, want %d", opened.Size, len(content))
	}
}

func TestFileServiceOpenFileContentContextUsesRecordStorageType(t *testing.T) {
	const key = "artifacts/task-8/report.txt"
	const content = "mixed storage content"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/go-admin-kit/"+key {
			t.Fatalf("path = %q, want /go-admin-kit/%s", r.URL.Path, key)
		}
		switch r.Method {
		case http.MethodHead:
			w.Header().Set("Content-Length", "21")
			w.Header().Set("Last-Modified", "Sat, 09 May 2026 00:00:00 GMT")
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			w.Header().Set("Content-Length", "21")
			w.Header().Set("Last-Modified", "Sat, 09 May 2026 00:00:00 GMT")
			_, _ = w.Write([]byte(content))
		default:
			t.Fatalf("method = %q, want HEAD or GET", r.Method)
		}
	}))
	defer server.Close()

	service := &FileService{
		uploader: upload.NewUploaderWithConfig(config.UploadConfig{
			StorageType: "local",
			LocalPath:   t.TempDir(),
			S3: config.ObjectStorageConfig{
				Endpoint:  server.URL,
				Bucket:    "go-admin-kit",
				Region:    "us-east-1",
				AccessKey: "access",
				SecretKey: "secret",
			},
		}),
	}

	opened, err := service.OpenFileContentContext(context.Background(), &model.File{
		FileName:    "report.txt",
		FilePath:    key,
		MimeType:    "text/plain",
		StorageType: "s3",
	})
	if err != nil {
		t.Fatalf("OpenFileContentContext() error = %v", err)
	}
	defer opened.Body.Close()

	body, err := io.ReadAll(opened.Body)
	if err != nil {
		t.Fatalf("read opened body: %v", err)
	}
	if string(body) != content {
		t.Fatalf("content = %q, want %q", string(body), content)
	}
}

func TestFileServiceOpenFileContentContextDefaultsContentType(t *testing.T) {
	path := "legacy-report.txt"
	dir := t.TempDir()
	service := &FileService{
		uploader: upload.NewUploaderWithConfig(config.UploadConfig{
			StorageType: "local",
			LocalPath:   dir,
		}),
	}
	if err := os.WriteFile(dir+"/"+path, []byte("legacy"), 0o600); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	opened, err := service.OpenFileContentContext(context.Background(), &model.File{
		FileName: "legacy-report.txt",
		FilePath: path,
	})
	if err != nil {
		t.Fatalf("OpenFileContentContext() error = %v", err)
	}
	defer opened.Body.Close()

	if opened.ContentType != "application/octet-stream" {
		t.Fatalf("content type = %q, want application/octet-stream", opened.ContentType)
	}
}

func newLocalUploadFileService(t *testing.T, db *gorm.DB, allowedTypes ...string) *FileService {
	t.Helper()

	return newLocalUploadFileServiceWithImage(t, db, config.ImageConfig{}, allowedTypes...)
}

func newLocalUploadFileServiceWithImage(t *testing.T, db *gorm.DB, imageCfg config.ImageConfig, allowedTypes ...string) *FileService {
	t.Helper()

	return &FileService{
		fileDAO: *systemdao.NewFileDAO(db),
		uploader: upload.NewUploaderWithConfig(config.UploadConfig{
			StorageType:   "local",
			LocalPath:     t.TempDir(),
			PublicBaseURL: "/files",
			MaxSize:       1,
			AllowedTypes:  allowedTypes,
			Image:         imageCfg,
		}),
	}
}

func systemMultipartFileHeader(t *testing.T, filename string, content []byte) *multipart.FileHeader {
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

func systemFixtureBase64(t *testing.T, value string) []byte {
	t.Helper()

	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return decoded
}

func systemMD5Hex(content []byte) string {
	sum := md5.Sum(content)
	return hex.EncodeToString(sum[:])
}

func systemPNG(t *testing.T, width, height int) []byte {
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
