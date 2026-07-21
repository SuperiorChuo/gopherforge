package system

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/services/system/internal/pkg/pagination"
)

func TestSmsChannelDAOGetListContextHonorsCanceledContext(t *testing.T) {
	db, _ := setupSystemDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := NewSmsChannelDAO(db).GetListContext(ctx, pagination.PageRequest{Page: 1, PageSize: 10}, nil, "", "")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetListContext() error = %v, want context.Canceled", err)
	}
}

func TestSmsTemplateDAOGetByCodeContextHonorsCanceledContext(t *testing.T) {
	db, _ := setupSystemDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewSmsTemplateDAO(db).GetByCodeContext(ctx, "user_register")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetByCodeContext() error = %v, want context.Canceled", err)
	}
}

func TestSmsLogDAOGetListContextHonorsCanceledContext(t *testing.T) {
	db, _ := setupSystemDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := NewSmsLogDAO(db).GetListContext(ctx, pagination.PageRequest{Page: 1, PageSize: 10}, "", "", "")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetListContext() error = %v, want context.Canceled", err)
	}
}

func TestSmsChannelDAOScopesByTenant(t *testing.T) {
	db, mock := newInjectedDictNoticeSeedDAOTestDB(t)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "sms_channels" WHERE tenant_id = $1 AND "sms_channels"."id" = $2 ORDER BY "sms_channels"."id" LIMIT $3`)).
		WithArgs(uint(1), uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "name", "provider", "status"}).
			AddRow(7, 1, "调试渠道", "debug", 1))

	channel, err := NewSmsChannelDAO(db).GetByIDContext(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetByIDContext() error = %v", err)
	}
	if channel.ID != 7 || channel.Provider != "debug" {
		t.Fatalf("channel = %#v, want injected row", channel)
	}
}

func TestSmsTemplateDAOCountByCodeExcludesSelf(t *testing.T) {
	db, mock := newInjectedDictNoticeSeedDAOTestDB(t)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "sms_templates" WHERE tenant_id = $1 AND code = $2 AND id <> $3`)).
		WithArgs(uint(1), "user_register", uint(3)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	count, err := NewSmsTemplateDAO(db).CountByCodeContext(context.Background(), "user_register", 3)
	if err != nil {
		t.Fatalf("CountByCodeContext() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
}

func TestSmsLogDAOUpdateResultContext(t *testing.T) {
	db, mock := newInjectedDictNoticeSeedDAOTestDB(t)

	mock.ExpectBegin()
	// Updates(map) 的 SET 列按字母序：error / provider_msg_id / status / updated_at。
	mock.ExpectExec(`UPDATE "sms_logs" SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := NewSmsLogDAO(db).UpdateResultContext(context.Background(), 5, "success", "biz-1", "")
	if err != nil {
		t.Fatalf("UpdateResultContext() error = %v", err)
	}
}
