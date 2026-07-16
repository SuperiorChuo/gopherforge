package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	redisstore "github.com/go-admin-kit/services/auth/internal/pkg/redis"
	"github.com/go-admin-kit/services/shared/pkg/response"
	goredis "github.com/redis/go-redis/v9"
)

func TestVerifyCaptchaUsesEnglishFailureMessage(t *testing.T) {
	setupCaptchaAPITestRedis(t)
	gin.SetMode(gin.TestMode)

	router := gin.New()
	api := NewCaptchaAPI()
	router.POST("/captcha/verify", api.VerifyCaptcha)

	body := bytes.NewBufferString(`{"key":"missing","code":"A7K9"}`)
	req := httptest.NewRequest(http.MethodPost, "/captcha/verify", body)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	var payload response.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Message != "captcha verification failed" {
		t.Fatalf("message = %q, want %q", payload.Message, "captcha verification failed")
	}
}

func setupCaptchaAPITestRedis(t *testing.T) {
	t.Helper()

	store, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}

	oldClient := redisstore.Client
	client := goredis.NewClient(&goredis.Options{Addr: store.Addr()})
	redisstore.Client = client

	t.Cleanup(func() {
		_ = client.Close()
		redisstore.Client = oldClient
		store.Close()
	})
}
