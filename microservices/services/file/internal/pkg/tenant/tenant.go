// Package tenant provides multi-tenant context helpers for file-service (M3).
package tenant

import (
	"context"

	"gorm.io/gorm"
)

// ContextKey is the request context key for the active tenant id (matches middleware).
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

// WithContext stores tenant id on context.
func WithContext(ctx context.Context, tenantID uint) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ContextKey, tenantID)
}

// Normalize maps 0 → 1 (default tenant).
func Normalize(id uint) uint {
	if id == 0 {
		return 1
	}
	return id
}

// IDFromContext returns the active tenant id, defaulting to 1.
func IDFromContext(ctx context.Context) uint {
	return Normalize(FromContext(ctx))
}

// ApplyFilter constrains a query to the tenant in ctx (default tenant 1).
func ApplyFilter(db *gorm.DB, ctx context.Context) *gorm.DB {
	if db == nil {
		return db
	}
	return db.Where("tenant_id = ?", IDFromContext(ctx))
}
