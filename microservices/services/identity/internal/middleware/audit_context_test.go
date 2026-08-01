package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	sharedaudit "github.com/go-admin-kit/services/shared/pkg/audittrail"
)

func TestAuditActorDefaultsAndNormalization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)

	actor := GetAuditActor(c)
	if actor.ActorType != DefaultAuditActorType {
		t.Fatalf("default actor type = %q, want %q", actor.ActorType, DefaultAuditActorType)
	}
	if actor.ActorID != DefaultAuditActorID {
		t.Fatalf("default actor id = %q, want %q", actor.ActorID, DefaultAuditActorID)
	}

	actor = SetAuditActor(c, " operator ", " alice ")
	if actor.ActorType != "operator" || actor.ActorID != "alice" {
		t.Fatalf("normalized actor = %#v", actor)
	}

	actor = GetAuditActor(c)
	if actor.ActorType != "operator" || actor.ActorID != "alice" {
		t.Fatalf("context actor = %#v", actor)
	}
	requestActor, ok := sharedaudit.ActorFromContext(c.Request.Context())
	if !ok || requestActor.Type != "operator" || requestActor.ID != "alice" {
		t.Fatalf("request context actor = %#v, found=%v", requestActor, ok)
	}
}

func TestSetAuditActorPropagatesSupportedTenantTypes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name  string
		value any
		want  uint
	}{
		{name: "uint", value: uint(11), want: 11},
		{name: "uint64", value: uint64(12), want: 12},
		{name: "int", value: int(13), want: 13},
		{name: "int64", value: int64(14), want: 14},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", "/", nil)
			c.Set("tenant_id", tt.value)

			SetAuditActor(c, " operator ", " alice ")

			actor, ok := sharedaudit.ActorFromContext(c.Request.Context())
			if !ok || actor.Type != "operator" || actor.ID != "alice" {
				t.Fatalf("request context actor = %#v, found=%v", actor, ok)
			}
			tenantID, ok := sharedaudit.TenantIDFromContext(c.Request.Context())
			if !ok || tenantID != tt.want {
				t.Fatalf("request context tenant = %d, found=%v, want=%d", tenantID, ok, tt.want)
			}
		})
	}
}

func TestSetAuditActorDoesNotInventTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)

	SetAuditActor(c, "operator", "alice")

	if tenantID, ok := sharedaudit.TenantIDFromContext(c.Request.Context()); ok {
		t.Fatalf("request context tenant = %d, want no tenant", tenantID)
	}
}

func TestSetAuditActorHandlesNilContextAndRequest(t *testing.T) {
	actor := SetAuditActor(nil, " operator ", " alice ")
	if actor.ActorType != "operator" || actor.ActorID != "alice" {
		t.Fatalf("nil context actor = %#v", actor)
	}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	actor = SetAuditActor(c, "   ", "")
	if actor.ActorType != DefaultAuditActorType {
		t.Fatalf("blank actor type = %q, want %q", actor.ActorType, DefaultAuditActorType)
	}
	if actor.ActorID != DefaultAuditActorID {
		t.Fatalf("blank actor id = %q, want %q", actor.ActorID, DefaultAuditActorID)
	}
}
