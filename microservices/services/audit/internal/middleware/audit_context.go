package middleware

import (
	"github.com/gin-gonic/gin"
	sharedaudit "github.com/go-admin-kit/services/shared/pkg/audittrail"
)

const (
	AuditActorTypeKey     = sharedaudit.AuditActorTypeKey
	AuditActorIDKey       = sharedaudit.AuditActorIDKey
	DefaultAuditActorType = sharedaudit.DefaultAuditActorType
	DefaultAuditActorID   = sharedaudit.DefaultAuditActorID
)

type AuditActor = sharedaudit.GinAuditActor

func SetAuditActor(c *gin.Context, actorType, actorID string) AuditActor {
	return sharedaudit.SetGinAuditActor(c, actorType, actorID)
}

func GetAuditActor(c *gin.Context) AuditActor {
	return sharedaudit.GetGinAuditActor(c)
}
