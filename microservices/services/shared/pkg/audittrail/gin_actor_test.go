package audittrail

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGinAuditActorPropagatesContextAndTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Nil gin.Context falls back cleanly
	def := GetGinAuditActor(nil)
	if def.ActorType != DefaultAuditActorType || def.ActorID != DefaultAuditActorID {
		t.Fatalf("nil context default = %+v", def)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/test", nil)
	c.Set("tenant_id", uint64(7))

	got := SetGinAuditActor(c, " admin ", " user:42 ")
	if got.ActorType != "admin" || got.ActorID != "user:42" {
		t.Fatalf("SetGinAuditActor = %+v", got)
	}
	if ctxActor, ok := ActorFromContext(c.Request.Context()); !ok || ctxActor.Type != "admin" || ctxActor.ID != "user:42" {
		t.Fatalf("ActorFromContext = %+v ok=%v", ctxActor, ok)
	}
	if tid, ok := TenantIDFromContext(c.Request.Context()); !ok || tid != 7 {
		t.Fatalf("TenantIDFromContext = %d ok=%v", tid, ok)
	}

	// Fallback to request.Context when gin keys are absent
	c2, _ := gin.CreateTestContext(httptest.NewRecorder())
	c2.Request = httptest.NewRequest(http.MethodGet, "/api/v1/test", nil).
		WithContext(WithActor(c.Request.Context(), "service", "cron"))
	fromCtx := GetGinAuditActor(c2)
	if fromCtx.ActorType != "service" || fromCtx.ActorID != "cron" {
		t.Fatalf("GetGinAuditActor fallback = %+v", fromCtx)
	}
}
