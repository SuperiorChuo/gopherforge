package system

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestUserDAOGetUserListUsesInjectedDB(t *testing.T) {
	db, mock := newInjectedSystemUserTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `users`")).
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `users` ORDER BY created_at DESC LIMIT ?")).
		WithArgs(10).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(42, "alice"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_roles` WHERE `user_roles`.`user_id` = ?")).
		WithArgs(uint(42)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}))

	users, total, err := NewUserDAO(db).GetUserListContext(
		context.Background(),
		pagination.PageRequest{Page: 1, PageSize: 10},
		"",
		nil,
		authz.UserDataScope{Scope: authz.DataScopeAll},
	)
	if err != nil {
		t.Fatalf("GetUserListContext() error = %v", err)
	}
	if total != 1 || len(users) != 1 || users[0].Username != "alice" {
		t.Fatalf("GetUserListContext() total=%d users=%#v, want one alice", total, users)
	}
}

func newInjectedSystemUserTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock db: %v", err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet database expectations: %v", err)
		}
		_ = sqlDB.Close()
	})
	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm sqlmock db: %v", err)
	}
	return db, mock
}
