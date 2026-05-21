package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/response"
)

func TestBindErrorsDoNotExposeDecoderMessages(t *testing.T) {
	files := []string{
		"user.go",
		"console_routes.go",
		"console_compat.go",
		"captcha.go",
	}
	assertNoBindErrorPassthrough(t, files)
}

func TestLoginInvalidJSONReturnsGenericBodyMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := NewUserAPI()
	router.POST("/login", api.Login)

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"username":`))
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
	if payload.Message != "invalid request body" {
		t.Fatalf("message = %q, want %q", payload.Message, "invalid request body")
	}
}

func assertNoBindErrorPassthrough(t *testing.T, files []string) {
	t.Helper()

	pattern := regexp.MustCompile(`(?s)ShouldBind(?:JSON|Query)\([^)]*\); err != nil[^{]*\{\s*response\.BadRequest\(c,\s*err\.Error\(\)\)`)
	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			content, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read %s: %v", file, err)
			}
			if pattern.Match(content) {
				t.Fatalf("%s exposes raw bind error through response.BadRequest", file)
			}
		})
	}
}
