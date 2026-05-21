package system

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
	"gorm.io/gorm"
)

func TestFileServiceGetFileListContextHonorsCanceledContext(t *testing.T) {
	setupSystemUserServiceContextTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := NewFileService().GetFileListContext(ctx, FileListRequest{
		PageRequest: pagination.PageRequest{Page: 1, PageSize: 10},
		DataScope:   authz.UserDataScope{Scope: authz.DataScopeAll},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetFileListContext() error = %v, want context.Canceled", err)
	}
}

func TestFileServiceGetFileByIDInScopeContextReturnsNotFoundSentinel(t *testing.T) {
	mock := setupSystemUserServiceContextTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `files` WHERE id = ? ORDER BY `files`.`id` LIMIT ?")).
		WithArgs(7, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := NewFileService().GetFileByIDInScopeContext(context.Background(), 7, authz.UserDataScope{
		Scope: authz.DataScopeAll,
	})
	if !errors.Is(err, ErrFileNotFoundOrPermissionDenied) {
		t.Fatalf("GetFileByIDInScopeContext() error = %v, want ErrFileNotFoundOrPermissionDenied", err)
	}
}

func TestFileServiceDeleteFileContextReturnsLookupError(t *testing.T) {
	mock := setupSystemUserServiceContextTestDB(t)
	lookupErr := errors.New("database lookup failed")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `files` WHERE id = ? ORDER BY `files`.`id` LIMIT ?")).
		WithArgs(7, 1).
		WillReturnError(lookupErr)

	err := NewFileService().DeleteFileContext(context.Background(), 7, 1, authz.UserDataScope{
		Scope: authz.DataScopeAll,
	})
	if !errors.Is(err, lookupErr) {
		t.Fatalf("DeleteFileContext() error = %v, want lookup error", err)
	}
}
