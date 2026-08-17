package tenantctx

import (
	"context"
	"testing"
)

func TestWithFromContextRoundTrip(t *testing.T) {
	ctx := WithContext(context.Background(), 42)
	if got := FromContext(ctx); got != 42 {
		t.Fatalf("FromContext = %d, want 42", got)
	}
}

func TestFromContextMissing(t *testing.T) {
	if got := FromContext(context.Background()); got != 0 {
		t.Fatalf("FromContext(empty) = %d, want 0", got)
	}
	if got := FromContext(nil); got != 0 {
		t.Fatalf("FromContext(nil) = %d, want 0", got)
	}
}

func TestFromContextTypeVariants(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  uint
	}{
		{"uint", uint(7), 7},
		{"uint64", uint64(8), 8},
		{"int positive", 9, 9},
		{"int zero", 0, 0},
		{"int negative", -3, 0},
		{"int64 positive", int64(11), 11},
		{"int64 zero", int64(0), 0},
		{"string", "13", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), Key, tc.value)
			if got := FromContext(ctx); got != tc.want {
				t.Fatalf("FromContext = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestWithContextPreservesValues 确认 WithContext 不丢失既有 context 值。
func TestWithContextPreservesValues(t *testing.T) {
	type marker struct{}
	base := context.WithValue(context.Background(), marker{}, "keep")
	ctx := WithContext(base, 3)
	if got := ctx.Value(marker{}); got != "keep" {
		t.Fatalf("marker lost: %v", got)
	}
	if got := FromContext(ctx); got != 3 {
		t.Fatalf("FromContext = %d, want 3", got)
	}
}
