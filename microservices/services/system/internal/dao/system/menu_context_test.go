package system

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
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
	db, mock := newInjectedDepartmentMenuDAOTestDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(`SAVEPOINT sp[0-9]+`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`DECLARE gak_fullread_[0-9]+ NO SCROLL CURSOR WITHOUT HOLD FOR ` + regexp.QuoteMeta(`SELECT * FROM "menus" ORDER BY parent_id ASC, sort ASC, created_at ASC`)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`FETCH FORWARD 256 FROM gak_fullread_[0-9]+`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "title", "parent_id"}).AddRow(9, "dashboard", "Dashboard", 0))
	mock.ExpectExec(`CLOSE gak_fullread_[0-9]+`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "menus" WHERE id IN ($1) LIMIT $2`)).WithArgs(9, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "title", "parent_id"}).AddRow(9, "dashboard", "Dashboard", 0))
	// 67b58597（权限按菜单归类）起 Menu 带 many2many:menu_permissions Preload，
	// GetMenuTree 会多查一次 menu_permissions；补期望覆盖，menu 9 无关联权限返回空。
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "menu_permissions" WHERE "menu_permissions"."menu_id" = $1`)).
		WithArgs(9).
		WillReturnRows(sqlmock.NewRows([]string{"menu_id", "permission_id"}))
	mock.ExpectCommit()

	menus, err := NewMenuDAO(db).GetMenuTreeContext(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetMenuTreeContext() error = %v", err)
	}
	if len(menus) != 1 || menus[0].Name != "dashboard" {
		t.Fatalf("GetMenuTree() menus = %#v, want one injected row", menus)
	}
}
