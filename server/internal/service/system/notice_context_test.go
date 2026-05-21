package system

import (
	"context"
	"errors"
	"regexp"
	"testing"
)

func TestNoticeServiceCreateContextHonorsCanceledContext(t *testing.T) {
	setupSystemUserServiceContextTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (&NoticeService{}).CreateContext(ctx, CreateNoticeRequest{
		Title:   "Maintenance",
		Content: "Tonight",
	}, 1, "admin")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CreateContext() error = %v, want context.Canceled", err)
	}
}

func TestNoticeServiceUpdateContextReturnsLookupError(t *testing.T) {
	mock := setupSystemUserServiceContextTestDB(t)
	lookupErr := errors.New("database lookup failed")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `notices` WHERE `notices`.`id` = ? ORDER BY `notices`.`id` LIMIT ?")).
		WithArgs(7, 1).
		WillReturnError(lookupErr)

	_, err := (&NoticeService{}).UpdateContext(context.Background(), 7, UpdateNoticeRequest{
		Title: "Maintenance",
	})
	if !errors.Is(err, lookupErr) {
		t.Fatalf("UpdateContext() error = %v, want lookup error", err)
	}
}
