package system

import (
	"context"
	"errors"
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
