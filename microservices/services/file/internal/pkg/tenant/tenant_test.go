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

func TestNormalizeAndIDFromContext(t *testing.T) {
	if Normalize(0) != 1 || Normalize(3) != 3 {
		t.Fatal("normalize")
	}
	if IDFromContext(context.Background()) != 1 {
		t.Fatal("default tenant")
	}
	if IDFromContext(WithContext(context.Background(), 9)) != 9 {
		t.Fatal("tenant from ctx")
	}
}
