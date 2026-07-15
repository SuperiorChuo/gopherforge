package api

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSetupRoutesRegistersNotificationWebSocketRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	SetupRoutes(router)

	foundWebSocket := false
	foundTicket := false
	for _, route := range router.Routes() {
		if route.Method == "GET" && route.Path == "/api/v1/ws/notifications" {
			foundWebSocket = true
		}
		if route.Method == "POST" && route.Path == "/api/v1/ws/notifications/ticket" {
			foundTicket = true
		}
	}
	if !foundWebSocket {
		t.Fatal("GET /api/v1/ws/notifications route is missing")
	}
	if !foundTicket {
		t.Fatal("POST /api/v1/ws/notifications/ticket route is missing")
	}
}
