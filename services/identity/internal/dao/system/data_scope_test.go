package system

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/services/identity/internal/pkg/authz"
	"github.com/go-admin-kit/services/identity/internal/pkg/pagination"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestUserDAOGetUserListAppliesDepartmentScope(t *testing.T) {
	db, mock := setupSystemDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "users" WHERE department_id IN ($1,$2)`)).
		WithArgs(uint(10), uint(11)).
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(0))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE department_id IN ($1,$2) ORDER BY created_at DESC LIMIT $3`)).
		WithArgs(uint(10), uint(11), 10).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	users, total, err := NewUserDAO(db).GetUserListContext(
		context.Background(),
		pagination.PageRequest{Page: 1, PageSize: 10},
		"",
		nil,
		authz.UserDataScope{Scope: authz.DataScopeDepartment, DepartmentIDs: []uint{10, 11}},
	)
	if err != nil {
		t.Fatalf("GetUserListContext() error = %v", err)
	}
	if total != 0 || len(users) != 0 {
		t.Fatalf("GetUserListContext() total=%d users=%d, want empty result", total, len(users))
	}
}

func TestUserDAOGetUserListContextHonorsCanceledContext(t *testing.T) {
	db, _ := setupSystemDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := NewUserDAO(db).GetUserListContext(
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

func TestOperationLogDAOGetLogListAppliesSelfScope(t *testing.T) {
	db, mock := setupSystemDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "operation_logs" WHERE user_id = $1`)).
		WithArgs(uint(7)).
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(0))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "operation_logs" WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2`)).
		WithArgs(uint(7), 10).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	logs, total, err := NewOperationLogDAO(db).GetLogListContext(
		context.Background(),
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
		t.Fatalf("GetLogListContext() error = %v", err)
	}
	if total != 0 || len(logs) != 0 {
		t.Fatalf("GetLogListContext() total=%d logs=%d, want empty result", total, len(logs))
	}
}

func TestOperationLogDAOGetLogByIDInScopeAppliesDepartmentScope(t *testing.T) {
	db, mock := setupSystemDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM \"operation_logs\" WHERE id = $1 AND user_id IN (SELECT `id` FROM `users` WHERE department_id IN ($2,$3)) ORDER BY \"operation_logs\".\"id\" LIMIT $4")).
		WithArgs(uint(88), uint(20), uint(21), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, err := NewOperationLogDAO(db).GetLogByIDInScopeContext(
		context.Background(),
		88,
		authz.UserDataScope{Scope: authz.DataScopeDepartment, DepartmentIDs: []uint{20, 21}},
	)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("GetLogByIDInScopeContext() error = %v, want record not found", err)
	}
}

func TestOperationLogDAOGetLogStatsInScopeAppliesDepartmentScope(t *testing.T) {
	db, mock := setupSystemDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM \"operation_logs\" WHERE user_id IN (SELECT `id` FROM `users` WHERE department_id IN ($1,$2))")).
		WithArgs(uint(20), uint(21)).
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(6))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT module, count(*) as count FROM \"operation_logs\" WHERE user_id IN (SELECT `id` FROM `users` WHERE department_id IN ($1,$2)) GROUP BY \"module\"")).
		WithArgs(uint(20), uint(21)).
		WillReturnRows(sqlmock.NewRows([]string{"module", "count"}).AddRow("system", 6))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT method, count(*) as count FROM \"operation_logs\" WHERE user_id IN (SELECT `id` FROM `users` WHERE department_id IN ($1,$2)) GROUP BY \"method\"")).
		WithArgs(uint(20), uint(21)).
		WillReturnRows(sqlmock.NewRows([]string{"method", "count"}).AddRow("GET", 6))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM \"operation_logs\" WHERE status >= 400 AND user_id IN (SELECT `id` FROM `users` WHERE department_id IN ($1,$2))")).
		WithArgs(uint(20), uint(21)).
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(2))

	stats, err := NewOperationLogDAO(db).GetLogStatsInScopeContext(
		context.Background(),
		nil,
		nil,
		authz.UserDataScope{Scope: authz.DataScopeDepartmentTree, DepartmentIDs: []uint{20, 21}},
	)
	if err != nil {
		t.Fatalf("GetLogStatsInScopeContext() error = %v", err)
	}
	if stats.Total != 6 || stats.ErrorCount != 2 || stats.ByModule["system"] != 6 || stats.ByMethod["GET"] != 6 {
		t.Fatalf("GetLogStatsInScopeContext() = %#v, want total=6 error=2 module/system=6 method/GET=6", stats)
	}
}

func TestOperationLogDAODeleteLogsBeforeInScopeAppliesSelfScope(t *testing.T) {
	db, mock := setupSystemDAOTestDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "operation_logs" WHERE created_at < $1 AND user_id = $2`)).
		WithArgs(sqlmock.AnyArg(), uint(7)).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectCommit()

	deleted, err := NewOperationLogDAO(db).DeleteLogsBeforeInScopeContext(
		context.Background(),
		timeNowForOperationLogDeleteTest(),
		authz.UserDataScope{Scope: authz.DataScopeSelf, UserID: 7},
	)
	if err != nil {
		t.Fatalf("DeleteLogsBeforeInScopeContext() error = %v", err)
	}
	if deleted != 3 {
		t.Fatalf("DeleteLogsBeforeInScopeContext() deleted=%d, want 3", deleted)
	}
}

func timeNowForOperationLogDeleteTest() time.Time {
	return time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
}

func setupSystemDAOTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock db: %v", err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm sqlmock db: %v", err)
	}
	if err := authz.RegisterDataScopePlugin(db); err != nil {
		t.Fatalf("register data scope plugin: %v", err)
	}

	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet database expectations: %v", err)
		}
		_ = sqlDB.Close()
	})

	return db, mock
}
