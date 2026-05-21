package system

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/server/internal/pkg/database"
)

func TestMenuDAOGetMenuTreeContextHonorsCanceledContext(t *testing.T) {
	db, _ := newInjectedDepartmentMenuDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewMenuDAO(db).GetMenuTreeContext(ctx, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetMenuTreeContext() error = %v, want context.Canceled", err)
	}
}

func TestMenuDAOGetMenuTreeUsesInjectedDB(t *testing.T) {
	oldDB := database.DB
	database.DB = nil
	t.Cleanup(func() {
		database.DB = oldDB
	})

	db, mock := newInjectedDepartmentMenuDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `menus` ORDER BY parent_id ASC, sort ASC, created_at ASC")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "title", "parent_id"}).AddRow(9, "dashboard", "Dashboard", 0))

	menus, err := NewMenuDAO(db).GetMenuTree(nil)
	if err != nil {
		t.Fatalf("GetMenuTree() error = %v", err)
	}
	if len(menus) != 1 || menus[0].Name != "dashboard" {
		t.Fatalf("GetMenuTree() menus = %#v, want one injected row", menus)
	}
}
