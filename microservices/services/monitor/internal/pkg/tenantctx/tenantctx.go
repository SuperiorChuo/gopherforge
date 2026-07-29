// Package tenantctx carries the authenticated tenant id across context
// boundaries. It exists as a leaf package so that both middleware (which writes
// the value) and lower-level packages such as authz (which read it) can share
// one context key without an import cycle.
package tenantctx

import "context"

type ctxKey string

// Key is the context key holding the active tenant id.
const Key ctxKey = "tenant_id"

// WithContext returns a context carrying the tenant id.
func WithContext(ctx context.Context, tenantID uint) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, Key, tenantID)
}

// FromContext returns the active tenant id, or 0 when absent.
func FromContext(ctx context.Context) uint {
	if ctx == nil {
		return 0
	}
	switch v := ctx.Value(Key).(type) {
	case uint:
		return v
	case uint64:
		return uint(v)
	case int:
		if v > 0 {
			return uint(v)
		}
	}
	return 0
}
