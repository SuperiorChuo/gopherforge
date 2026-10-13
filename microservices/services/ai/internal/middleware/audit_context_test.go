package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAuditActorDefaultsAndNormalization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

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
}

func TestAuditActorFallsBackForBlankValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	actor := SetAuditActor(c, "   ", "")
	if actor.ActorType != DefaultAuditActorType {
		t.Fatalf("blank actor type = %q, want %q", actor.ActorType, DefaultAuditActorType)
	}
	if actor.ActorID != DefaultAuditActorID {
		t.Fatalf("blank actor id = %q, want %q", actor.ActorID, DefaultAuditActorID)
	}
}
