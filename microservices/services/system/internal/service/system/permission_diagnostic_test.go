package system

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPermissionMenuDiagnosticReturnsDirectAndRelationBindings(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT DISTINCT menus.* FROM "menus" LEFT JOIN menu_permissions mp ON mp.menu_id = menus.id LEFT JOIN permissions p ON p.id = mp.permission_id WHERE menus.permission = $1 OR p.code = $2 ORDER BY menus.parent_id ASC, menus.sort ASC, menus.id ASC`)).
		WithArgs("system:user:list", "system:user:list").
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "path", "component", "parent_id", "status", "hidden", "permission"}).
			AddRow(11, "Users", "/system/users", "system/user/index", 10, 1, 0, "system:user:list"))

	svc := NewPermissionMenuDiagnosticServiceWithDB(db)
	result, err := svc.DiagnoseContext(context.Background(), " system:user:list ")
	if err != nil {
		t.Fatalf("DiagnoseContext() error = %v", err)
	}
	if result.Permission != "system:user:list" || len(result.Menus) != 1 || result.Menus[0].Path != "/system/users" {
		t.Fatalf("result = %#v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}
