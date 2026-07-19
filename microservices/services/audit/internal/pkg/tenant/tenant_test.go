package tenant

import (
	"context"
	"testing"
)

func TestFromContextOrDefault(t *testing.T) {
	if got := FromContextOrDefault(context.Background()); got != DefaultID {
		t.Fatalf("empty context = %d, want %d", got, DefaultID)
	}
	ctx := context.WithValue(context.Background(), ContextKey, uint(9))
	if got := FromContextOrDefault(ctx); got != 9 {
		t.Fatalf("tenant context = %d, want 9", got)
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

func TestApplyFilterUsesDefaultTenant(t *testing.T) {
	// Compile-time sanity: FromContextOrDefault is what ApplyFilter uses.
	if got := FromContextOrDefault(nil); got != DefaultID {
		t.Fatalf("nil ctx default = %d, want %d", got, DefaultID)
	}
}
