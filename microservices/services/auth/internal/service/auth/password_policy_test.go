package auth

import (
	"testing"
	"time"

	"github.com/go-admin-kit/services/auth/internal/model"
)

func TestPasswordExpiredWhenChangedAtOlderThanMaxAge(t *testing.T) {
	now := time.Date(2026, 5, 22, 8, 0, 0, 0, time.UTC)
	changedAt := now.AddDate(0, 0, -91)
	user := &model.User{PasswordChangedAt: &changedAt}

	if !isPasswordExpired(user, now, 90) {
		t.Fatal("expected password to be expired")
	}
}

func TestPasswordNotExpiredWhenMaxAgeDisabled(t *testing.T) {
	now := time.Date(2026, 5, 22, 8, 0, 0, 0, time.UTC)
	changedAt := now.AddDate(-10, 0, 0)
	user := &model.User{PasswordChangedAt: &changedAt}

	if isPasswordExpired(user, now, 0) {
		t.Fatal("password should not expire when max age is disabled")
	}
}

func TestPasswordExpiredWhenTimestampMissing(t *testing.T) {
	now := time.Date(2026, 5, 22, 8, 0, 0, 0, time.UTC)

	if !isPasswordExpired(&model.User{}, now, 90) {
		t.Fatal("expected missing password_changed_at to be expired when policy is enabled")
	}
}
