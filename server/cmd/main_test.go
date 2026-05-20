package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/config"
)

func TestSetupCORSAllowsLocalDevelopmentFrontendPorts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalConfig := config.Cfg
	t.Cleanup(func() {
		config.Cfg = originalConfig
	})

	config.Cfg = config.Config{
		App: config.AppCfg{Env: "development"},
		CORS: config.CORSConfig{
			AllowOrigins:     []string{"http://127.0.0.1:3002"},
			AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodOptions},
			AllowHeaders:     []string{"Origin", "Content-Type"},
			AllowCredentials: true,
			MaxAge:           12,
		},
	}

	router := gin.New()
	setupCORS(router)
	router.GET("/api/v1/captcha", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/captcha", nil)
	req.Header.Set("Origin", "http://127.0.0.1:5173")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:5173" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, "http://127.0.0.1:5173")
	}
}
