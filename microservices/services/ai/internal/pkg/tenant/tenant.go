// Package tenant provides multi-tenant context helpers for row-level isolation.
package tenant

import "context"

// ContextKey is the request context key for the active tenant id
// (matches middleware.TenantIDContextKey / gin "tenant_id").
const ContextKey = "tenant_id"

// FromContext returns tenant id from context (0 if absent).
func FromContext(ctx context.Context) uint {
	if ctx == nil {
		return 0
	}
	switch v := ctx.Value(ContextKey).(type) {
	case uint:
		return v
	case uint64:
		return uint(v)
	case int:
		if v > 0 {
			return uint(v)
		}
	case int64:
		if v > 0 {
			return uint(v)
		}
	}
	return 0
}

// Normalize maps 0 → 1 (default tenant).
func Normalize(id uint) uint {
	if id == 0 {
		return 1
	}
	return id
}

// FromContextOrDefault returns the tenant id from context, defaulting to 1.
func FromContextOrDefault(ctx context.Context) uint {
	return Normalize(FromContext(ctx))
}
