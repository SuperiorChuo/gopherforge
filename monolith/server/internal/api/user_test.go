package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/api/auth"
	"github.com/go-admin-kit/server/internal/middleware"
	"github.com/go-admin-kit/server/internal/pkg/database"
)

func requireTestDatabase(t *testing.T) {
	t.Helper()
	if database.DB == nil {
		t.Skip("requires initialized test database")
	}
}

func TestUserAPI_Register(t *testing.T) {
	requireTestDatabase(t)

	// Set Gin to test mode.
	gin.SetMode(gin.TestMode)

	// Create the test router.
	router := gin.New()
	api := router.Group("/api/v1")
	{
		userAPI := auth.NewUserAPI()
		api.POST("/register", userAPI.Register)
	}

	// Test data.
	registerData := map[string]string{
		"username": "testuser",
		"password": "testpass123",
		"email":    "test@example.com",
	}
	jsonData, _ := json.Marshal(registerData)

	// Create the request.
	req, _ := http.NewRequest("POST", "/api/v1/register", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Execute the request.
	router.ServeHTTP(w, req)

	// Assert the response.
	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
		t.Errorf("Expected status code %d or %d, got %d", http.StatusOK, http.StatusBadRequest, w.Code)
	}
}

func TestUserAPI_Login(t *testing.T) {
	requireTestDatabase(t)

	// Set Gin to test mode.
	gin.SetMode(gin.TestMode)

	// Create the test router.
	router := gin.New()
	api := router.Group("/api/v1")
	{
		userAPI := auth.NewUserAPI()
		api.POST("/login", userAPI.Login)
	}

	// Test data.
	loginData := map[string]string{
		"username": "testuser",
		"password": "testpass123",
	}
	jsonData, _ := json.Marshal(loginData)

	// Create the request.
	req, _ := http.NewRequest("POST", "/api/v1/login", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Execute the request.
	router.ServeHTTP(w, req)

	// The user may not exist in a sparse test database.
	if w.Code != http.StatusOK && w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status code %d or %d, got %d", http.StatusOK, http.StatusUnauthorized, w.Code)
	}
}

func TestUserAPI_UpdateProfileRequiresAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	api := router.Group("/api/v1")
	protected := api.Group("/")
	protected.Use(middleware.AuthMiddleware())
	{
		userAPI := auth.NewUserAPI()
		protected.PUT("/user/profile", userAPI.UpdateProfile)
	}

	req, _ := http.NewRequest("PUT", "/api/v1/user/profile", bytes.NewBufferString(`{"nickname":"tester"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("Expected status code %d, got %d", http.StatusUnauthorized, w.Code)
	}
}
