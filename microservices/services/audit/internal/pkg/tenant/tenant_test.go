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
	if Normalize(0) != DefaultID || Normalize(3) != 3 {
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

func TestFromContextOrDefault(t *testing.T) {
	if got := FromContextOrDefault(context.Background()); got != DefaultID {
		t.Fatalf("empty context = %d, want %d", got, DefaultID)
	}
	if got := FromContextOrDefault(nil); got != DefaultID {
		t.Fatalf("nil ctx default = %d, want %d", got, DefaultID)
	}
	ctx := context.WithValue(context.Background(), ContextKey, uint(9))
	if got := FromContextOrDefault(ctx); got != 9 {
		t.Fatalf("tenant context = %d, want 9", got)
	}
	if IDFromContext(ctx) != 9 || IDFromContext(context.Background()) != DefaultID {
		t.Fatal("IDFromContext should mirror FromContextOrDefault")
	}
}

func TestEnsureID(t *testing.T) {
	ctx := context.WithValue(context.Background(), ContextKey, uint(2))
	if got := EnsureID(ctx, 5); got != 5 {
		t.Fatalf("existing wins: got %d, want 5", got)
	}
	if got := EnsureID(ctx, 0); got != 2 {
		t.Fatalf("context fill: got %d, want 2", got)
	}
	if got := EnsureID(context.Background(), 0); got != DefaultID {
		t.Fatalf("default fill: got %d, want %d", got, DefaultID)
	}
}
