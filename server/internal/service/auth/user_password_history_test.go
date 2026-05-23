package auth

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/server/internal/config"
	"github.com/go-admin-kit/server/internal/pkg/runtimeconfig"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func TestChangePasswordContextRejectsRecentlyUsedPassword(t *testing.T) {
	mock := setupAuthServiceContextTestDB(t)
	oldCfg := config.Cfg
	config.Cfg.Security.PasswordHistoryCount = 5
	t.Cleanup(func() {
		config.Cfg = oldCfg
	})

	currentHash := mustHashPasswordForTest(t, "CurrentPass1")
	reusedHash := mustHashPasswordForTest(t, "UsedPass1")
	changedAt := time.Date(2026, 5, 20, 8, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `users` WHERE `users`.`id` = ? ORDER BY `users`.`id` LIMIT ?")).
		WithArgs(uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "password", "status", "must_change_password"}).
			AddRow(uint(7), "alice", currentHash, int8(1), true))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `password_history` WHERE user_id = ? ORDER BY changed_at DESC, id DESC LIMIT ?")).
		WithArgs(uint(7), 5).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "password_hash", "changed_at"}).
			AddRow(uint(1), uint(7), reusedHash, changedAt))

	err := (&UserService{}).ChangePasswordContext(context.Background(), 7, ChangePasswordRequest{
		OldPassword: "CurrentPass1",
		NewPassword: "UsedPass1",
	})
	if !errors.Is(err, ErrPasswordRecentlyUsed) {
		t.Fatalf("ChangePasswordContext() error = %v, want ErrPasswordRecentlyUsed", err)
	}
}

func TestChangePasswordContextUpdatesPasswordAndHistoryAtomically(t *testing.T) {
	mock := setupAuthServiceContextTestDB(t)
	oldCfg := config.Cfg
	config.Cfg.Security.PasswordHistoryCount = 5
	t.Cleanup(func() {
		config.Cfg = oldCfg
	})

	currentHash := mustHashPasswordForTest(t, "CurrentPass1")

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `users` WHERE `users`.`id` = ? ORDER BY `users`.`id` LIMIT ?")).
		WithArgs(uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "password", "status", "must_change_password"}).
			AddRow(uint(7), "alice", currentHash, int8(1), true))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `password_history` WHERE user_id = ? ORDER BY changed_at DESC, id DESC LIMIT ?")).
		WithArgs(uint(7), 5).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "password_hash", "changed_at"}))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `users` SET `must_change_password`=?,`password`=?,`password_changed_at`=?,`updated_at`=? WHERE id = ? AND password = ?")).
		WithArgs(false, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), uint(7), currentHash).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `password_history` (`user_id`,`password_hash`,`changed_at`,`created_at`) VALUES (?,?,?,?)")).
		WithArgs(uint(7), currentHash, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := (&UserService{}).ChangePasswordContext(context.Background(), 7, ChangePasswordRequest{
		OldPassword: "CurrentPass1",
		NewPassword: "FreshPass1",
	})
	if err != nil {
		t.Fatalf("ChangePasswordContext() error = %v", err)
	}
}

func TestChangePasswordContextUsesRuntimePasswordHistoryCount(t *testing.T) {
	mock := setupAuthServiceContextTestDB(t)
	oldCfg := config.Cfg
	config.Cfg.Security.PasswordHistoryCount = 5
	t.Cleanup(func() {
		config.Cfg = oldCfg
	})

	currentHash := mustHashPasswordForTest(t, "CurrentPass1")

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `users` WHERE `users`.`id` = ? ORDER BY `users`.`id` LIMIT ?")).
		WithArgs(uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "password", "status", "must_change_password"}).
			AddRow(uint(7), "alice", currentHash, int8(1), true))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `password_history` WHERE user_id = ? ORDER BY changed_at DESC, id DESC LIMIT ?")).
		WithArgs(uint(7), 2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "password_hash", "changed_at"}))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `users` SET `must_change_password`=?,`password`=?,`password_changed_at`=?,`updated_at`=? WHERE id = ? AND password = ?")).
		WithArgs(false, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), uint(7), currentHash).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `password_history` (`user_id`,`password_hash`,`changed_at`,`created_at`) VALUES (?,?,?,?)")).
		WithArgs(uint(7), currentHash, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	service := &UserService{policyReader: stubSecurityPolicyReader{policy: runtimeconfig.SecurityPolicy{PasswordHistoryCount: 2}}}
	err := service.ChangePasswordContext(context.Background(), 7, ChangePasswordRequest{
		OldPassword: "CurrentPass1",
		NewPassword: "FreshPass1",
	})
	if err != nil {
		t.Fatalf("ChangePasswordContext() error = %v", err)
	}
}

func TestChangePasswordContextReturnsOldPasswordErrorWhenConcurrentUpdateWins(t *testing.T) {
	mock := setupAuthServiceContextTestDB(t)
	oldCfg := config.Cfg
	config.Cfg.Security.PasswordHistoryCount = 0
	t.Cleanup(func() {
		config.Cfg = oldCfg
	})

	currentHash := mustHashPasswordForTest(t, "CurrentPass1")

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `users` WHERE `users`.`id` = ? ORDER BY `users`.`id` LIMIT ?")).
		WithArgs(uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "password", "status", "must_change_password"}).
			AddRow(uint(7), "alice", currentHash, int8(1), true))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `users` SET `must_change_password`=?,`password`=?,`password_changed_at`=?,`updated_at`=? WHERE id = ? AND password = ?")).
		WithArgs(false, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), uint(7), currentHash).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	err := (&UserService{}).ChangePasswordContext(context.Background(), 7, ChangePasswordRequest{
		OldPassword: "CurrentPass1",
		NewPassword: "FreshPass1",
	})
	if !errors.Is(err, ErrOldPasswordIncorrect) {
		t.Fatalf("ChangePasswordContext() error = %v, want ErrOldPasswordIncorrect", err)
	}
}

func TestUserServiceLoginPasswordContextReturnsUpdateErrorWhenExpiredFlagCannotPersist(t *testing.T) {
	mock := setupAuthServiceContextTestDB(t)
	oldCfg := config.Cfg
	config.Cfg.Security.PasswordMaxAgeDays = 30
	t.Cleanup(func() {
		config.Cfg = oldCfg
	})

	currentHash := mustHashPasswordForTest(t, "CurrentPass1")
	changedAt := time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC)
	updateErr := errors.New("update failed")

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `users` WHERE username = ? ORDER BY `users`.`id` LIMIT ?")).
		WithArgs("alice", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "password", "status", "must_change_password", "password_changed_at"}).
			AddRow(uint(7), "alice", currentHash, int8(1), false, changedAt))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `users` SET `must_change_password`=?,`updated_at`=? WHERE id = ?")).
		WithArgs(true, sqlmock.AnyArg(), uint(7)).
		WillReturnError(updateErr)
	mock.ExpectRollback()

	_, err := (&UserService{}).LoginPasswordContext(context.Background(), "alice", "CurrentPass1")
	if !errors.Is(err, updateErr) {
		t.Fatalf("LoginPasswordContext() error = %v, want update error", err)
	}
}

func TestUserServiceLoginPasswordContextUsesRuntimePasswordMaxAge(t *testing.T) {
	mock := setupAuthServiceContextTestDB(t)
	oldCfg := config.Cfg
	config.Cfg.Security.PasswordMaxAgeDays = 0
	t.Cleanup(func() {
		config.Cfg = oldCfg
	})

	currentHash := mustHashPasswordForTest(t, "CurrentPass1")
	changedAt := time.Now().AddDate(0, 0, -60)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `users` WHERE username = ? ORDER BY `users`.`id` LIMIT ?")).
		WithArgs("alice", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "password", "status", "must_change_password", "password_changed_at"}).
			AddRow(uint(7), "alice", currentHash, int8(1), false, changedAt))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `users` SET `must_change_password`=?,`updated_at`=? WHERE id = ?")).
		WithArgs(true, sqlmock.AnyArg(), uint(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `users` WHERE `users`.`id` = ? ORDER BY `users`.`id` LIMIT ?")).
		WithArgs(uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "password", "status", "must_change_password"}).
			AddRow(uint(7), "alice", currentHash, int8(1), true))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_roles` WHERE `user_roles`.`user_id` = ?")).
		WithArgs(uint(7)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}))

	service := &UserService{policyReader: stubSecurityPolicyReader{policy: runtimeconfig.SecurityPolicy{PasswordMaxAgeDays: 30}}}
	resp, err := service.LoginPasswordContext(context.Background(), "alice", "CurrentPass1")
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
