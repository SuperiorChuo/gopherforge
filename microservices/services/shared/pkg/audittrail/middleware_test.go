package audittrail

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAuditHeaderMiddlewareInjectsActorTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	var gotActor Actor
	var gotTenant uint
	r.Use(AuditHeaderMiddleware())
	r.GET("/t", func(c *gin.Context) {
		gotActor, _ = ActorFromContext(c.Request.Context())
		gotTenant, _ = TenantIDFromContext(c.Request.Context())
		c.Status(200)
	})

	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set(HeaderUserID, "42")
	req.Header.Set(HeaderUsername, "alice")
	req.Header.Set(HeaderTenantID, "7")
	r.ServeHTTP(httptest.NewRecorder(), req)

	if gotActor.Type != "operator" || gotActor.ID != "alice" {
		t.Fatalf("actor = %+v, want operator/alice", gotActor)
	}
	if gotTenant != 7 {
		t.Fatalf("tenant = %d, want 7", gotTenant)
	}
}

func TestAuditHeaderMiddlewareSkipsAnonymous(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	var got bool
	r.Use(AuditHeaderMiddleware())
	r.GET("/t", func(c *gin.Context) {
		_, got = ActorFromContext(c.Request.Context())
		c.Status(200)
	})

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/t", nil))
	if got {
		t.Fatal("anonymous request must not inject actor")
	}
}

func TestAuditHeaderMiddlewareDefaultsTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	var gotTenant uint
	r.Use(AuditHeaderMiddleware())
	r.GET("/t", func(c *gin.Context) {
		gotTenant, _ = TenantIDFromContext(c.Request.Context())
		c.Status(200)
	})

	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set(HeaderUserID, "1")
	r.ServeHTTP(httptest.NewRecorder(), req)
	if gotTenant != 1 {
		t.Fatalf("tenant = %d, want default 1", gotTenant)
	}
}
