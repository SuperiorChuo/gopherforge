package system

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
	"gorm.io/gorm"
)

func TestLoginLogServiceGetLogListContextHonorsCanceledContext(t *testing.T) {
	db, _ := setupSystemUserServiceContextTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc := NewLoginLogServiceWithDB(db)
	_, _, err := (&svc).GetLogListContext(ctx, LoginLogListRequest{
		PageRequest: pagination.PageRequest{Page: 1, PageSize: 10},
		DataScope:   authz.UserDataScope{Scope: authz.DataScopeAll},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetLogListContext() error = %v, want context.Canceled", err)
	}
}

func TestLoginLogServiceGetUserLastLoginContextReturnsNotFoundSentinel(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `login_logs` WHERE user_id = ? AND status = 1 ORDER BY created_at DESC,`login_logs`.`id` LIMIT ?")).
		WithArgs(7, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	svc := NewLoginLogServiceWithDB(db)
	_, err := (&svc).GetUserLastLoginContext(context.Background(), 7)
	if !errors.Is(err, ErrLoginLogNotFound) {
		t.Fatalf("GetUserLastLoginContext() error = %v, want ErrLoginLogNotFound", err)
	}
}
