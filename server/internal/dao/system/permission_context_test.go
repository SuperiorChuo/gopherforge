package system

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/server/internal/pkg/database"
)

func TestPermissionManageDAOGetPermissionTreeUsesInjectedDB(t *testing.T) {
	oldDB := database.DB
	database.DB = nil
	t.Cleanup(func() {
		database.DB = oldDB
	})

	db, mock := newInjectedRBACDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `permissions` ORDER BY parent_id ASC, created_at ASC")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code", "type", "parent_id"}).
			AddRow(uint(1), "System", "system", int8(1), uint(0)).
			AddRow(uint(2), "User", "system:user", int8(2), uint(1)))

	permissions, err := NewPermissionManageDAO(db).GetPermissionTreeContext(context.Background())
	if err != nil {
		t.Fatalf("GetPermissionTreeContext() error = %v", err)
	}
	if len(permissions) != 1 || len(permissions[0].Children) != 1 || permissions[0].Children[0].Code != "system:user" {
		t.Fatalf("permissions = %#v, want injected permission tree", permissions)
	}
}

func TestPermissionManageDAOGetPermissionTreeContextHonorsCanceledContext(t *testing.T) {
	setupSystemDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (&PermissionManageDAO{}).GetPermissionTreeContext(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetPermissionTreeContext() error = %v, want context.Canceled", err)
	}
}
