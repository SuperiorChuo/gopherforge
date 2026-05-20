package system

import (
	"context"
	"errors"
	"testing"
)

func TestDictServiceCreateTypeContextHonorsCanceledContext(t *testing.T) {
	setupSystemUserServiceContextTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (&DictService{}).CreateTypeContext(ctx, CreateDictTypeRequest{
		Name: "Gender",
		Code: "gender",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CreateTypeContext() error = %v, want context.Canceled", err)
	}
}
