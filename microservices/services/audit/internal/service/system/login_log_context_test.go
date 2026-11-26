package system

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/services/shared/pkg/authz"
	"github.com/go-admin-kit/services/shared/pkg/pagination"
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
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "login_logs" WHERE tenant_id = $1 AND (user_id = $2 AND status = 1) ORDER BY created_at DESC,"login_logs"."id" LIMIT $3`)).
		WithArgs(sqlmock.AnyArg(), 7, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	svc := NewLoginLogServiceWithDB(db)
	_, err := (&svc).GetUserLastLoginContext(context.Background(), 7)
	if !errors.Is(err, ErrLoginLogNotFound) {
		t.Fatalf("GetUserLastLoginContext() error = %v, want ErrLoginLogNotFound", err)
	}
}

func TestTruncateStringPreservesUTF8AndCapsRunes(t *testing.T) {
	sqli := "testuserX' AND (SELECT CASE WHEN 1=1 THEN (SELECT 1 FROM (SELECT SLEEP(0))a) ELSE 1 END)=''"
	got := truncateString(sqli, 50)
	if len([]rune(got)) != 50 {
		t.Fatalf("len(runes) = %d, want 50", len([]rune(got)))
	}
	cjk := "管理员测试账号名称超长字符截断验证一二三四五六七八九十"
	gotCJK := truncateString(cjk, 10)
	if gotCJK != "管理员测试账号名称超" {
		t.Fatalf("gotCJK = %q, want 10 runes without broken UTF-8", gotCJK)
	}
}
