package auth

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/services/auth/internal/config"
	"github.com/go-admin-kit/services/auth/internal/pkg/runtimeconfig"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func TestChangePasswordContextRejectsRecentlyUsedPassword(t *testing.T) {
	db, mock := setupAuthServiceContextTestDB(t)
	svc := NewUserServiceWithDB(db)
	oldCfg := config.Cfg
	config.Cfg.Security.PasswordHistoryCount = 5
	t.Cleanup(func() {
		config.Cfg = oldCfg
	})

	currentHash := mustHashPasswordForTest(t, "CurrentPass1")
	reusedHash := mustHashPasswordForTest(t, "UsedPass1")
	changedAt := time.Date(2026, 5, 20, 8, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE "users"."id" = $1 ORDER BY "users"."id" LIMIT $2`)).
		WithArgs(uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "password", "status", "must_change_password"}).
			AddRow(uint(7), "alice", currentHash, int8(1), true))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "password_history" WHERE user_id = $1 ORDER BY changed_at DESC, id DESC LIMIT $2`)).
		WithArgs(uint(7), 5).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "password_hash", "changed_at"}).
			AddRow(uint(1), uint(7), reusedHash, changedAt))

	err := (&svc).ChangePasswordContext(context.Background(), 7, ChangePasswordRequest{
		OldPassword: "CurrentPass1",
		NewPassword: "UsedPass1",
	})
	if !errors.Is(err, ErrPasswordRecentlyUsed) {
		t.Fatalf("ChangePasswordContext() error = %v, want ErrPasswordRecentlyUsed", err)
	}
}

func TestChangePasswordContextUpdatesPasswordAndHistoryAtomically(t *testing.T) {
	db, mock := setupAuthServiceContextTestDB(t)
	svc := NewUserServiceWithDB(db)
	oldCfg := config.Cfg
	config.Cfg.Security.PasswordHistoryCount = 5
	t.Cleanup(func() {
		config.Cfg = oldCfg
	})

	currentHash := mustHashPasswordForTest(t, "CurrentPass1")

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE "users"."id" = $1 ORDER BY "users"."id" LIMIT $2`)).
		WithArgs(uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "password", "status", "must_change_password"}).
			AddRow(uint(7), "alice", currentHash, int8(1), true))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "password_history" WHERE user_id = $1 ORDER BY changed_at DESC, id DESC LIMIT $2`)).
		WithArgs(uint(7), 5).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "password_hash", "changed_at"}))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "users" SET "must_change_password"=$1,"password"=$2,"password_changed_at"=$3,"updated_at"=$4 WHERE id = $5 AND password = $6`)).
		WithArgs(false, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), uint(7), currentHash).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "password_history" ("user_id","password_hash","changed_at","created_at") VALUES ($1,$2,$3,$4) RETURNING "id"`)).
		WithArgs(uint(7), currentHash, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()

	err := (&svc).ChangePasswordContext(context.Background(), 7, ChangePasswordRequest{
		OldPassword: "CurrentPass1",
		NewPassword: "FreshPass1",
	})
	if err != nil {
		t.Fatalf("ChangePasswordContext() error = %v", err)
	}
}

func TestChangePasswordContextUsesRuntimePasswordHistoryCount(t *testing.T) {
	db, mock := setupAuthServiceContextTestDB(t)
	svc := NewUserServiceWithDB(db)
	oldCfg := config.Cfg
	config.Cfg.Security.PasswordHistoryCount = 5
	t.Cleanup(func() {
		config.Cfg = oldCfg
	})

	currentHash := mustHashPasswordForTest(t, "CurrentPass1")

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE "users"."id" = $1 ORDER BY "users"."id" LIMIT $2`)).
		WithArgs(uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "password", "status", "must_change_password"}).
			AddRow(uint(7), "alice", currentHash, int8(1), true))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "password_history" WHERE user_id = $1 ORDER BY changed_at DESC, id DESC LIMIT $2`)).
		WithArgs(uint(7), 2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "password_hash", "changed_at"}))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "users" SET "must_change_password"=$1,"password"=$2,"password_changed_at"=$3,"updated_at"=$4 WHERE id = $5 AND password = $6`)).
		WithArgs(false, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), uint(7), currentHash).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "password_history" ("user_id","password_hash","changed_at","created_at") VALUES ($1,$2,$3,$4) RETURNING "id"`)).
		WithArgs(uint(7), currentHash, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()

	svc.policyReader = stubSecurityPolicyReader{policy: runtimeconfig.SecurityPolicy{PasswordHistoryCount: 2}}
	err := (&svc).ChangePasswordContext(context.Background(), 7, ChangePasswordRequest{
		OldPassword: "CurrentPass1",
		NewPassword: "FreshPass1",
	})
	if err != nil {
		t.Fatalf("ChangePasswordContext() error = %v", err)
	}
}

func TestChangePasswordContextReturnsOldPasswordErrorWhenConcurrentUpdateWins(t *testing.T) {
	db, mock := setupAuthServiceContextTestDB(t)
	svc := NewUserServiceWithDB(db)
	oldCfg := config.Cfg
	config.Cfg.Security.PasswordHistoryCount = 0
	t.Cleanup(func() {
		config.Cfg = oldCfg
	})

	currentHash := mustHashPasswordForTest(t, "CurrentPass1")

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE "users"."id" = $1 ORDER BY "users"."id" LIMIT $2`)).
		WithArgs(uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "password", "status", "must_change_password"}).
			AddRow(uint(7), "alice", currentHash, int8(1), true))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "users" SET "must_change_password"=$1,"password"=$2,"password_changed_at"=$3,"updated_at"=$4 WHERE id = $5 AND password = $6`)).
		WithArgs(false, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), uint(7), currentHash).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	err := (&svc).ChangePasswordContext(context.Background(), 7, ChangePasswordRequest{
		OldPassword: "CurrentPass1",
		NewPassword: "FreshPass1",
	})
	if !errors.Is(err, ErrOldPasswordIncorrect) {
		t.Fatalf("ChangePasswordContext() error = %v, want ErrOldPasswordIncorrect", err)
	}
}

func TestUserServiceLoginPasswordContextReturnsUpdateErrorWhenExpiredFlagCannotPersist(t *testing.T) {
	db, mock := setupAuthServiceContextTestDB(t)
	svc := NewUserServiceWithDB(db)
	oldCfg := config.Cfg
	config.Cfg.Security.PasswordMaxAgeDays = 30
	t.Cleanup(func() {
		config.Cfg = oldCfg
	})

	currentHash := mustHashPasswordForTest(t, "CurrentPass1")
	changedAt := time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC)
	updateErr := errors.New("update failed")

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE username = $1 ORDER BY "users"."id" LIMIT $2`)).
		WithArgs("alice", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "password", "status", "must_change_password", "password_changed_at"}).
			AddRow(uint(7), "alice", currentHash, int8(1), false, changedAt))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "users" SET "must_change_password"=$1,"updated_at"=$2 WHERE id = $3`)).
		WithArgs(true, sqlmock.AnyArg(), uint(7)).
		WillReturnError(updateErr)
	mock.ExpectRollback()

	_, err := (&svc).LoginPasswordContext(context.Background(), "alice", "CurrentPass1")
	if !errors.Is(err, updateErr) {
		t.Fatalf("LoginPasswordContext() error = %v, want update error", err)
	}
}

func TestUserServiceLoginPasswordContextUsesRuntimePasswordMaxAge(t *testing.T) {
	db, mock := setupAuthServiceContextTestDB(t)
	svc := NewUserServiceWithDB(db)
	oldCfg := config.Cfg
	config.Cfg.Security.PasswordMaxAgeDays = 0
	t.Cleanup(func() {
		config.Cfg = oldCfg
	})

	currentHash := mustHashPasswordForTest(t, "CurrentPass1")
	changedAt := time.Now().AddDate(0, 0, -60)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE username = $1 ORDER BY "users"."id" LIMIT $2`)).
		WithArgs("alice", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "password", "status", "must_change_password", "password_changed_at"}).
			AddRow(uint(7), "alice", currentHash, int8(1), false, changedAt))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "users" SET "must_change_password"=$1,"updated_at"=$2 WHERE id = $3`)).
		WithArgs(true, sqlmock.AnyArg(), uint(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE "users"."id" = $1 ORDER BY "users"."id" LIMIT $2`)).
		WithArgs(uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "password", "status", "must_change_password"}).
			AddRow(uint(7), "alice", currentHash, int8(1), true))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "user_roles" WHERE "user_roles"."user_id" = $1`)).
		WithArgs(uint(7)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}))

	svc.policyReader = stubSecurityPolicyReader{policy: runtimeconfig.SecurityPolicy{PasswordMaxAgeDays: 30}}
	resp, err := (&svc).LoginPasswordContext(context.Background(), "alice", "CurrentPass1")
	if err != nil {
		t.Fatalf("LoginPasswordContext() error = %v", err)
	}
	if !resp.User.MustChangePassword {
		t.Fatal("runtime password max age should mark user as must_change_password")
	}
}

func mustHashPasswordForTest(t *testing.T, password string) string {
	t.Helper()
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	return string(hashed)
}

func TestUserServiceMapsConditionalPasswordUpdateMissToOldPasswordError(t *testing.T) {
	if !errors.Is(mapPasswordUpdateError(gorm.ErrRecordNotFound), ErrOldPasswordIncorrect) {
		t.Fatal("expected conditional update miss to map to ErrOldPasswordIncorrect")
	}
}

type stubSecurityPolicyReader struct {
	policy runtimeconfig.SecurityPolicy
}

func (s stubSecurityPolicyReader) SecurityPolicy(ctx context.Context) runtimeconfig.SecurityPolicy {
	return s.policy
}
