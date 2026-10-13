package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthReadyChecksDatabase(t *testing.T) {
	router, server := newTestRouterWithServer(t)

	healthy := httptest.NewRecorder()
	router.ServeHTTP(healthy, httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil))
	if healthy.Code != http.StatusOK {
		t.Fatalf("healthy readiness status = %d, want 200", healthy.Code)
	}

	sqlDB, err := server.Store.DB().DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close sql db: %v", err)
	}

	unavailable := httptest.NewRecorder()
	router.ServeHTTP(unavailable, httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil))
	if unavailable.Code != http.StatusServiceUnavailable {
		t.Fatalf("closed database readiness status = %d, want 503; body=%s", unavailable.Code, unavailable.Body.String())
	}
}
