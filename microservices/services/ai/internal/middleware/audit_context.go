package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
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
	c.Set(AuditActorTypeKey, actor.ActorType)
	c.Set(AuditActorIDKey, actor.ActorID)
	return actor
}

func GetAuditActor(c *gin.Context) AuditActor {
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

	return AuditActor{
		ActorType: normalizeAuditValue(actorType, DefaultAuditActorType),
		ActorID:   normalizeAuditValue(actorID, DefaultAuditActorID),
	}
}

func normalizeAuditValue(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
