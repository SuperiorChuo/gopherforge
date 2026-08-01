package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	sharedaudit "github.com/go-admin-kit/services/shared/pkg/audittrail"
)

const (
	AuditActorTypeKey = "audit_actor_type"
	AuditActorIDKey   = "audit_actor_id"

	DefaultAuditActorType = "operator"
	DefaultAuditActorID   = "web-console"
)

type AuditActor struct {
	ActorType string
	ActorID   string
}

func SetAuditActor(c *gin.Context, actorType, actorID string) AuditActor {
	actor := AuditActor{
		ActorType: normalizeAuditValue(actorType, DefaultAuditActorType),
		ActorID:   normalizeAuditValue(actorID, DefaultAuditActorID),
	}
	if c == nil {
		return actor
	}
	c.Set(AuditActorTypeKey, actor.ActorType)
	c.Set(AuditActorIDKey, actor.ActorID)
	if c.Request != nil {
		ctx := sharedaudit.WithActor(c.Request.Context(), actor.ActorType, actor.ActorID)
		if tenantID, ok := auditTenantID(c); ok {
			ctx = sharedaudit.WithTenantID(ctx, tenantID)
		}
		c.Request = c.Request.WithContext(ctx)
	}
	return actor
}

func GetAuditActor(c *gin.Context) AuditActor {
	if c == nil {
		return AuditActor{
			ActorType: DefaultAuditActorType,
			ActorID:   DefaultAuditActorID,
		}
	}
	actorType := ""
	if value, ok := c.Get(AuditActorTypeKey); ok {
		if typed, ok := value.(string); ok {
			actorType = typed
		}
	}

	actorID := ""
	if value, ok := c.Get(AuditActorIDKey); ok {
		if typed, ok := value.(string); ok {
			actorID = typed
		}
	}
	if c.Request != nil && (actorType == "" || actorID == "") {
		if actor, ok := sharedaudit.ActorFromContext(c.Request.Context()); ok {
			actorType = actor.Type
			actorID = actor.ID
		}
	}

	return AuditActor{
		ActorType: normalizeAuditValue(actorType, DefaultAuditActorType),
		ActorID:   normalizeAuditValue(actorID, DefaultAuditActorID),
	}
}

func auditTenantID(c *gin.Context) (uint, bool) {
	value, ok := c.Get("tenant_id")
	if !ok {
		return 0, false
	}

	switch tenantID := value.(type) {
	case uint:
		return tenantID, tenantID > 0
	case uint64:
		if tenantID == 0 || tenantID > uint64(^uint(0)) {
			return 0, false
		}
		return uint(tenantID), true
	case int:
		if tenantID <= 0 {
			return 0, false
		}
		return uint(tenantID), true
	case int64:
		if tenantID <= 0 || uint64(tenantID) > uint64(^uint(0)) {
			return 0, false
		}
		return uint(tenantID), true
	default:
		return 0, false
	}
}

func normalizeAuditValue(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
