package system

import (
	"context"
	"errors"
	"testing"

	"github.com/go-admin-kit/services/file/internal/config"
	"github.com/go-admin-kit/services/file/internal/pkg/upload"
)

func TestValidAvatarPublicToken(t *testing.T) {
	token, err := newAvatarPublicToken()
	if err != nil {
		t.Fatalf("newAvatarPublicToken() error = %v", err)
	}
	if !ValidAvatarPublicToken(token) {
		t.Fatalf("generated avatar token %q is invalid", token)
	}
	for _, invalid := range []string{"", "short", token + "/", token[:42]} {
		if ValidAvatarPublicToken(invalid) {
			t.Fatalf("validAvatarPublicToken(%q) = true, want false", invalid)
		}
	}
}

func TestUploadAvatarRejectsImageBelowMinimumDimensions(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	mock.ExpectQuery(`SELECT COALESCE\(storage_quota_mb, 0\) FROM tenant_packages WHERE id = \$1`).
		WithArgs(uint(1)).
		WillReturnError(errors.New("quota unavailable"))
	service := newLocalUploadFileService(t, db, ".png")

	_, err := service.UploadAvatarContext(context.Background(), systemMultipartFileHeader(t, "avatar.png", systemPNG(t, 31, 32)), 42)
	if !errors.Is(err, upload.ErrAvatarDimensions) {
		t.Fatalf("UploadAvatarContext() error = %v, want ErrAvatarDimensions", err)
	}
}

func TestUploadAvatarRejectsUnsupportedImageType(t *testing.T) {
	service := &FileService{
		uploader: upload.NewUploaderWithConfig(config.UploadConfig{
			StorageType:   "local",
			LocalPath:     t.TempDir(),
			PublicBaseURL: "/uploads",
			MaxSize:       10,
			AllowedTypes:  []string{".jpg", ".jpeg", ".png", ".gif", ".webp"},
		}),
	}

	_, err := service.UploadAvatarContext(context.Background(), systemMultipartFileHeader(t, "avatar.gif", systemFixtureBase64(t, "R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==")), 42)
	if !errors.Is(err, upload.ErrAvatarTypeNotAllowed) {
		t.Fatalf("UploadAvatarContext() error = %v, want ErrAvatarTypeNotAllowed", err)
	}
}

func TestUploadAvatarRejectsFileLargerThanTwoMiB(t *testing.T) {
	service := &FileService{
		uploader: upload.NewUploaderWithConfig(config.UploadConfig{
			StorageType:   "local",
			LocalPath:     t.TempDir(),
			PublicBaseURL: "/uploads",
			MaxSize:       10,
			AllowedTypes:  []string{".png"},
		}),
	}
	file := systemMultipartFileHeader(t, "avatar.png", systemPNG(t, 1, 1))
	file.Size = MaxAvatarBytes + 1

	_, err := service.UploadAvatarContext(context.Background(), file, 42)
	if !errors.Is(err, upload.ErrAvatarTooLarge) {
		t.Fatalf("UploadAvatarContext() error = %v, want ErrAvatarTooLarge", err)
	}
}
