package auth

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPermissionDAOGetUserPermissionsContextHonorsCanceledContext(t *testing.T) {
	db, _ := newAuthDAOTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewPermissionDAO(db).GetUserPermissionsContext(ctx, 7)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetUserPermissionsContext() error = %v, want context.Canceled", err)
	}
}

func TestPermissionDAOUsesInjectedDB(t *testing.T) {
	db, mock := newAuthDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT permissions.code FROM "users" JOIN user_roles ON users.id = user_roles.user_id JOIN roles ON user_roles.role_id = roles.id JOIN role_permissions ON roles.id = role_permissions.role_id JOIN permissions ON role_permissions.permission_id = permissions.id WHERE users.id = $1`)).
		WithArgs(uint(7)).
		WillReturnRows(sqlmock.NewRows([]string{"code"}).AddRow("dashboard.view"))

	codes, err := NewPermissionDAO(db).GetUserPermissionsContext(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetUserPermissionsContext() error = %v", err)
	}
	if len(codes) != 1 || codes[0] != "dashboard.view" {
		t.Fatalf("codes = %v, want dashboard.view", codes)
	}
}
