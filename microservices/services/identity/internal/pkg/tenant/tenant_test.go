package tenant

import (
	"context"
	"testing"
)

func TestFromContextAndWithContext(t *testing.T) {
	if FromContext(nil) != 0 {
		t.Fatal("nil ctx")
	}
	ctx := WithContext(context.Background(), 42)
	if FromContext(ctx) != 42 {
		t.Fatalf("got %d", FromContext(ctx))
	}
}

func TestNormalize(t *testing.T) {
	if Normalize(0) != 1 || Normalize(3) != 3 {
		t.Fatal("normalize")
	}
}

func TestDisableScope(t *testing.T) {
	ctx := DisableScope(context.Background())
	if !scopeDisabled(ctx) {
		t.Fatal("expected disabled")
	}
	if scopeDisabled(context.Background()) {
		t.Fatal("expected enabled")
	}
}

func TestRequire(t *testing.T) {
	if _, err := Require(context.Background()); err == nil {
		t.Fatal("want error")
	}
	id, err := Require(WithContext(context.Background(), 7))
	if err != nil || id != 7 {
		t.Fatalf("got %d %v", id, err)
	}
}
