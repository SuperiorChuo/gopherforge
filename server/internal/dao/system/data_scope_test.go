package system

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestUserDAOGetUserListAppliesDepartmentScope(t *testing.T) {
	mock := setupSystemDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `users` WHERE department_id IN (?,?)")).
		WithArgs(uint(10), uint(11)).
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `users` WHERE department_id IN (?,?) ORDER BY created_at DESC LIMIT ?")).
		WithArgs(uint(10), uint(11), 10).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	users, total, err := (&UserDAO{}).GetUserList(
		pagination.PageRequest{Page: 1, PageSize: 10},
		"",
		nil,
		authz.UserDataScope{Scope: authz.DataScopeDepartment, DepartmentIDs: []uint{10, 11}},
	)
	if err != nil {
		t.Fatalf("GetUserList() error = %v", err)
	}
	if total != 0 || len(users) != 0 {
		t.Fatalf("GetUserList() total=%d users=%d, want empty result", total, len(users))
	}
}

func TestUserDAOGetUserListContextHonorsCanceledContext(t *testing.T) {
	setupSystemDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := (&UserDAO{}).GetUserListContext(
		ctx,
		pagination.PageRequest{Page: 1, PageSize: 10},
		"",
		nil,
		authz.UserDataScope{Scope: authz.DataScopeAll},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetUserListContext() error = %v, want context.Canceled", err)
	}
}

func TestFileDAOGetByIDInScopeAppliesOwnerDepartmentScope(t *testing.T) {
	mock := setupSystemDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `files` WHERE user_id IN (SELECT `id` FROM `users` WHERE department_id IN (?,?)) AND id = ? ORDER BY `files`.`id` LIMIT ?")).
		WithArgs(uint(20), uint(21), uint(99), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, err := (&FileDAO{}).GetByIDInScope(
		99,
		authz.UserDataScope{Scope: authz.DataScopeCustom, DepartmentIDs: []uint{20, 21}},
	)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("GetByIDInScope() error = %v, want record not found", err)
	}
}

func TestOperationLogDAOGetLogListAppliesSelfScope(t *testing.T) {
	mock := setupSystemDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `operation_logs` WHERE user_id = ?")).
		WithArgs(uint(7)).
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `operation_logs` WHERE user_id = ? ORDER BY created_at DESC LIMIT ?")).
		WithArgs(uint(7), 10).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	logs, total, err := (&OperationLogDAO{}).GetLogList(
		pagination.PageRequest{Page: 1, PageSize: 10},
		nil,
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		nil,
		nil,
		nil,
		authz.UserDataScope{Scope: authz.DataScopeSelf, UserID: 7},
	)
	if err != nil {
		t.Fatalf("GetLogList() error = %v", err)
	}
	if total != 0 || len(logs) != 0 {
		t.Fatalf("GetLogList() total=%d logs=%d, want empty result", total, len(logs))
	}
}

func setupSystemDAOTestDB(t *testing.T) sqlmock.Sqlmock {
	t.Helper()

	oldDB := database.DB
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock db: %v", err)
	}
	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm sqlmock db: %v", err)
	}

	database.DB = db
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet database expectations: %v", err)
		}
		_ = sqlDB.Close()
		database.DB = oldDB
	})

	return mock
}
