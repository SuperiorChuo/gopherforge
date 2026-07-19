// Package tenant provides multi-tenant context helpers for audit-service (M3).
package tenant

import (
	"context"

	"gorm.io/gorm"
)

// ContextKey matches middleware.TenantIDContextKey / JWT propagation.
const ContextKey = "tenant_id"

// DefaultID is the platform default tenant used when context has no tenant.
const DefaultID uint = 1

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

// Normalize maps 0 → DefaultID.
func Normalize(id uint) uint {
	if id == 0 {
		return DefaultID
	}
	return id
}

// FromContextOrDefault returns tenant from context, or DefaultID when missing.
func FromContextOrDefault(ctx context.Context) uint {
	return Normalize(FromContext(ctx))
}

// EnsureID keeps a positive existing tenant id; otherwise resolves from context
// (defaulting to DefaultID). Used on create/write paths so async or event-driven
// writers can stamp TenantID before context is lost.
func EnsureID(ctx context.Context, existing uint) uint {
	if existing > 0 {
		return existing
	}
	return FromContextOrDefault(ctx)
}

// ApplyFilter constrains a query to the actor tenant in ctx (default tenant 1).
// Authenticated requests always carry tenant_id via middleware; missing context
// falls back to the platform default so unscoped cross-tenant reads cannot leak.
func ApplyFilter(query *gorm.DB, ctx context.Context) *gorm.DB {
	if query == nil {
		return query
	}
	return query.Where("tenant_id = ?", FromContextOrDefault(ctx))
}
