package system

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPermissionCacheDAOFindUserIDsByRoleIDsUsesInjectedDB(t *testing.T) {
	db, mock := newInjectedRBACDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT DISTINCT `user_id` FROM `user_roles` WHERE role_id IN (?)")).
		WithArgs(uint(9)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(uint(42)))

	userIDs, err := NewPermissionCacheDAO(db).FindUserIDsByRoleIDsContext(context.Background(), []uint{9})
	if err != nil {
		t.Fatalf("FindUserIDsByRoleIDsContext() error = %v", err)
	}
	if !reflect.DeepEqual(userIDs, []uint{42}) {
		t.Fatalf("userIDs = %#v, want [42]", userIDs)
	}
}

func TestPermissionCacheDAOFindUserIDsByRoleIDs(t *testing.T) {
	db, mock := setupSystemDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT DISTINCT `user_id` FROM `user_roles` WHERE role_id IN (?,?)")).
		WithArgs(uint(2), uint(3)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).
			AddRow(uint(10)).
			AddRow(uint(11)))

	userIDs, err := NewPermissionCacheDAO(db).FindUserIDsByRoleIDsContext(context.Background(), []uint{2, 3})
	if err != nil {
		t.Fatalf("FindUserIDsByRoleIDsContext() error = %v", err)
	}
	if !reflect.DeepEqual(userIDs, []uint{10, 11}) {
		t.Fatalf("userIDs = %#v, want [10 11]", userIDs)
	}
}

func TestPermissionCacheDAOFindUserIDsByRoleIDsContextHonorsCanceledContext(t *testing.T) {
	db, _ := setupSystemDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewPermissionCacheDAO(db).FindUserIDsByRoleIDsContext(ctx, []uint{2})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("FindUserIDsByRoleIDsContext() error = %v, want context.Canceled", err)
	}
}

func TestPermissionCacheDAOFindRoleIDsByPermissionIDs(t *testing.T) {
	db, mock := setupSystemDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT DISTINCT `role_id` FROM `role_permissions` WHERE permission_id IN (?,?)")).
		WithArgs(uint(5), uint(6)).
		WillReturnRows(sqlmock.NewRows([]string{"role_id"}).
			AddRow(uint(20)).
			AddRow(uint(21)))

	roleIDs, err := NewPermissionCacheDAO(db).FindRoleIDsByPermissionIDsContext(context.Background(), []uint{5, 6})
	if err != nil {
		t.Fatalf("FindRoleIDsByPermissionIDsContext() error = %v", err)
	}
	if !reflect.DeepEqual(roleIDs, []uint{20, 21}) {
		t.Fatalf("roleIDs = %#v, want [20 21]", roleIDs)
	}
}

func TestPermissionCacheDAOEmptyInputsSkipDatabase(t *testing.T) {
	db, _ := setupSystemDAOTestDB(t)
	dao := NewPermissionCacheDAO(db)

	userIDs, err := dao.FindUserIDsByRoleIDsContext(context.Background(), nil)
	if err != nil {
		t.Fatalf("FindUserIDsByRoleIDsContext(nil) error = %v", err)
	}
	if userIDs != nil {
		t.Fatalf("userIDs = %#v, want nil", userIDs)
	}

	roleIDs, err := dao.FindRoleIDsByPermissionIDsContext(context.Background(), nil)
	if err != nil {
		t.Fatalf("FindRoleIDsByPermissionIDsContext(nil) error = %v", err)
	}
	if roleIDs != nil {
		t.Fatalf("roleIDs = %#v, want nil", roleIDs)
	}
}
