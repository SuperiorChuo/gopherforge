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
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `files` WHERE id = ? AND user_id IN (SELECT `id` FROM `users` WHERE department_id IN (?,?)) ORDER BY `files`.`id` LIMIT ?")).
		WithArgs(uint(99), uint(20), uint(21), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, err := (&FileDAO{}).GetByIDInScope(
		99,
		authz.UserDataScope{Scope: authz.DataScopeCustom, DepartmentIDs: []uint{20, 21}},
	)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("GetByIDInScope() error = %v, want record not found", err)
	}
}

func TestFileDAOGetByHashInScopeAppliesSelfScope(t *testing.T) {
	mock := setupSystemDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `files` WHERE hash = ? AND user_id = ? ORDER BY `files`.`id` LIMIT ?")).
		WithArgs("abc123", uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, err := (&FileDAO{}).GetByHashInScope(
		"abc123",
		authz.UserDataScope{Scope: authz.DataScopeSelf, UserID: 7},
	)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("GetByHashInScope() error = %v, want record not found", err)
	}
}

func TestFileDAOGetListAppliesNoScope(t *testing.T) {
	mock := setupSystemDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `files` WHERE 1 = 0")).
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `files` WHERE 1 = 0 ORDER BY created_at DESC LIMIT ?")).
		WithArgs(10).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	files, total, err := (&FileDAO{}).GetList(
		pagination.PageRequest{Page: 1, PageSize: 10},
		nil,
		"",
		"",
		nil,
		nil,
		authz.UserDataScope{Scope: authz.DataScopeNone},
	)
	if err != nil {
		t.Fatalf("GetList() error = %v", err)
	}
	if total != 0 || len(files) != 0 {
		t.Fatalf("GetList() total=%d files=%d, want empty result", total, len(files))
	}
}

func TestFileDAOGetStatsInScopeAppliesOwnerDepartmentScopeToBothAggregates(t *testing.T) {
	mock := setupSystemDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) as count, COALESCE(SUM(file_size), 0) as total_size FROM `files` WHERE user_id IN (SELECT `id` FROM `users` WHERE department_id IN (?,?))")).
		WithArgs(uint(20), uint(21)).
		WillReturnRows(sqlmock.NewRows([]string{"count", "total_size"}).AddRow(3, 2048))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT file_type, COUNT(*) as count, COALESCE(SUM(file_size), 0) as size FROM `files` WHERE user_id IN (SELECT `id` FROM `users` WHERE department_id IN (?,?)) GROUP BY `file_type`")).
		WithArgs(uint(20), uint(21)).
		WillReturnRows(sqlmock.NewRows([]string{"file_type", "count", "size"}).AddRow("image", 2, 1024).AddRow("doc", 1, 1024))

	stats, err := (&FileDAO{}).GetStatsInScopeContext(
		context.Background(),
		nil,
		authz.UserDataScope{Scope: authz.DataScopeDepartment, DepartmentIDs: []uint{20, 21}},
	)
	if err != nil {
		t.Fatalf("GetStatsInScopeContext() error = %v", err)
	}
	if stats.Total != 3 || stats.TotalSize != 2048 {
		t.Fatalf("GetStatsInScopeContext() total=%d size=%d, want total=3 size=2048", stats.Total, stats.TotalSize)
	}
	if len(stats.ByType) != 2 {
		t.Fatalf("GetStatsInScopeContext() type stats len=%d, want 2", len(stats.ByType))
	}
}

func TestLoginLogDAOGetListAppliesDepartmentScopeAndFilters(t *testing.T) {
	mock := setupSystemDAOTestDB(t)
	status := int8(1)
	loginType := int8(2)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `login_logs` WHERE username LIKE ? AND ip LIKE ? AND status = ? AND login_type = ? AND user_id IN (SELECT `id` FROM `users` WHERE department_id IN (?,?))")).
		WithArgs("%alice%", "%10.%", status, loginType, uint(20), uint(21)).
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `login_logs` WHERE username LIKE ? AND ip LIKE ? AND status = ? AND login_type = ? AND user_id IN (SELECT `id` FROM `users` WHERE department_id IN (?,?)) ORDER BY created_at DESC LIMIT ?")).
		WithArgs("%alice%", "%10.%", status, loginType, uint(20), uint(21), 10).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	logs, total, err := (&LoginLogDAO{}).GetList(
		pagination.PageRequest{Page: 1, PageSize: 10},
		nil,
		"alice",
		"10.",
		&status,
		&loginType,
		nil,
		nil,
		authz.UserDataScope{Scope: authz.DataScopeDepartment, DepartmentIDs: []uint{20, 21}},
	)
	if err != nil {
		t.Fatalf("GetList() error = %v", err)
	}
	if total != 0 || len(logs) != 0 {
		t.Fatalf("GetList() total=%d logs=%d, want empty result", total, len(logs))
	}
}

func TestLoginLogDAOGetStatsInScopeAppliesDepartmentScope(t *testing.T) {
	mock := setupSystemDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `login_logs` WHERE user_id IN (SELECT `id` FROM `users` WHERE department_id IN (?,?))")).
		WithArgs(uint(20), uint(21)).
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(5))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `login_logs` WHERE status = 1 AND user_id IN (SELECT `id` FROM `users` WHERE department_id IN (?,?))")).
		WithArgs(uint(20), uint(21)).
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(4))
	mock.ExpectQuery("(?i)^SELECT COUNT\\(DISTINCT\\(`user_id`\\)\\) FROM `login_logs` WHERE \\(status = 1 AND created_at >= \\?\\) AND user_id IN \\(SELECT `id` FROM `users` WHERE department_id IN \\(\\?,\\?\\)\\)$").
		WithArgs(sqlmock.AnyArg(), uint(20), uint(21)).
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(3))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT device, COUNT(*) as count FROM `login_logs` WHERE status = 1 AND user_id IN (SELECT `id` FROM `users` WHERE department_id IN (?,?)) GROUP BY `device`")).
		WithArgs(uint(20), uint(21)).
		WillReturnRows(sqlmock.NewRows([]string{"device", "count"}).AddRow("Desktop", 4))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT browser, COUNT(*) as count FROM `login_logs` WHERE status = 1 AND user_id IN (SELECT `id` FROM `users` WHERE department_id IN (?,?)) GROUP BY `browser`")).
		WithArgs(uint(20), uint(21)).
		WillReturnRows(sqlmock.NewRows([]string{"browser", "count"}).AddRow("Chrome", 4))

	stats, err := (&LoginLogDAO{}).GetStatsInScopeContext(
		context.Background(),
		nil,
		nil,
		authz.UserDataScope{Scope: authz.DataScopeDepartment, DepartmentIDs: []uint{20, 21}},
	)
	if err != nil {
		t.Fatalf("GetStatsInScopeContext() error = %v", err)
	}
	if stats.Total != 5 || stats.Success != 4 || stats.Failed != 1 || stats.TodayUsers != 3 {
		t.Fatalf("GetStatsInScopeContext() = %#v, want total=5 success=4 failed=1 today_users=3", stats)
	}
}

func TestLoginLogDAOGetLoginTrendInScopeAppliesDepartmentScope(t *testing.T) {
	mock := setupSystemDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `login_logs` WHERE (created_at >= ? AND created_at < ?) AND user_id IN (SELECT `id` FROM `users` WHERE department_id IN (?,?))")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), uint(20), uint(21)).
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(5))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `login_logs` WHERE (created_at >= ? AND created_at < ? AND status = 1) AND user_id IN (SELECT `id` FROM `users` WHERE department_id IN (?,?))")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), uint(20), uint(21)).
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(4))

	trend, err := (&LoginLogDAO{}).GetLoginTrendInScopeContext(
		context.Background(),
		1,
		authz.UserDataScope{Scope: authz.DataScopeDepartment, DepartmentIDs: []uint{20, 21}},
	)
	if err != nil {
		t.Fatalf("GetLoginTrendInScopeContext() error = %v", err)
	}
	if len(trend) != 1 || trend[0].Count != 5 || trend[0].Success != 4 || trend[0].Failed != 1 {
		t.Fatalf("GetLoginTrendInScopeContext() = %#v, want single day 5/4/1", trend)
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

func TestOperationLogDAOGetLogByIDInScopeAppliesDepartmentScope(t *testing.T) {
	mock := setupSystemDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `operation_logs` WHERE id = ? AND user_id IN (SELECT `id` FROM `users` WHERE department_id IN (?,?)) ORDER BY `operation_logs`.`id` LIMIT ?")).
		WithArgs(uint(88), uint(20), uint(21), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, err := (&OperationLogDAO{}).GetLogByIDInScopeContext(
		context.Background(),
		88,
		authz.UserDataScope{Scope: authz.DataScopeDepartment, DepartmentIDs: []uint{20, 21}},
	)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("GetLogByIDInScopeContext() error = %v, want record not found", err)
	}
}

func TestOperationLogDAOGetLogStatsInScopeAppliesDepartmentScope(t *testing.T) {
	mock := setupSystemDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `operation_logs` WHERE user_id IN (SELECT `id` FROM `users` WHERE department_id IN (?,?))")).
		WithArgs(uint(20), uint(21)).
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(6))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT module, count(*) as count FROM `operation_logs` WHERE user_id IN (SELECT `id` FROM `users` WHERE department_id IN (?,?)) GROUP BY `module`")).
		WithArgs(uint(20), uint(21)).
		WillReturnRows(sqlmock.NewRows([]string{"module", "count"}).AddRow("system", 6))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT method, count(*) as count FROM `operation_logs` WHERE user_id IN (SELECT `id` FROM `users` WHERE department_id IN (?,?)) GROUP BY `method`")).
		WithArgs(uint(20), uint(21)).
		WillReturnRows(sqlmock.NewRows([]string{"method", "count"}).AddRow("GET", 6))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `operation_logs` WHERE status >= 400 AND user_id IN (SELECT `id` FROM `users` WHERE department_id IN (?,?))")).
		WithArgs(uint(20), uint(21)).
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(2))

	stats, err := (&OperationLogDAO{}).GetLogStatsInScopeContext(
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
	if err := authz.RegisterDataScopePlugin(db); err != nil {
		t.Fatalf("register data scope plugin: %v", err)
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
