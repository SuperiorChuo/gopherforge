// Package audittrail provides actor-bound, transaction-coupled change auditing.
package audittrail

import (
	"context"
	"strings"
)

type actorContextKey struct{}
type tenantContextKey struct{}
type disabledContextKey struct{}

const tenantCompatibilityContextKey = "tenant_id"

// Actor identifies the principal responsible for a database mutation.
type Actor struct {
	Type string
	ID   string
}

// WithActor attaches an explicit audit principal to ctx.
func WithActor(ctx context.Context, actorType, actorID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, actorContextKey{}, Actor{
		Type: strings.TrimSpace(actorType),
		ID:   strings.TrimSpace(actorID),
	})
}

// ActorFromContext returns a complete, explicitly attached audit principal.
func ActorFromContext(ctx context.Context) (Actor, bool) {
	if ctx == nil {
		return Actor{}, false
	}
	actor, ok := ctx.Value(actorContextKey{}).(Actor)
	if !ok || actor.Type == "" || actor.ID == "" {
		return Actor{}, false
	}
	return actor, true
}

// WithTenantID attaches the tenant attribution used by audit records.
func WithTenantID(ctx context.Context, tenantID uint) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	// Existing service tenant plugins read the historical string key. Keep it
	// in sync while using the package-private key as the audit contract.
	ctx = context.WithValue(ctx, tenantCompatibilityContextKey, tenantID)
	return context.WithValue(ctx, tenantContextKey{}, tenantID)
}

// TenantIDFromContext returns a positive, explicitly attached tenant id.
func TenantIDFromContext(ctx context.Context) (uint, bool) {
	if ctx == nil {
		return 0, false
	}
	tenantID, ok := ctx.Value(tenantContextKey{}).(uint)
	return tenantID, ok && tenantID > 0
}

func disable(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, disabledContextKey{}, true)
}

func isDisabled(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	disabled, _ := ctx.Value(disabledContextKey{}).(bool)
	return disabled
}

func internalContext(ctx context.Context, tenantID uint) context.Context {
	return disable(WithTenantID(ctx, tenantID))
}
