package api

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	sharedapi "github.com/go-admin-kit/services/shared/pkg/sharedapi"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPermissionDiagnosticRouteIsRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sqlDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock: %v", err)
	}
	defer sqlDB.Close()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}

	router := gin.New()
	SetupRoutesWithDeps(router, sharedapi.Dependencies{DB: db})
	for _, route := range router.Routes() {
		if route.Method == "POST" && route.Path == "/api/v1/permissions/diagnose" {
			return
		}
	}
	t.Fatal("POST /api/v1/permissions/diagnose is not registered")
}
