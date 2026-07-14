package monitor

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	sharedapi "github.com/go-admin-kit/server/internal/api/shared"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestRegisterProtectedRoutes(t *testing.T) {
	db := setupMonitorRouteSQLMock(t)
	routes := registeredMonitorRoutes(func(r *gin.RouterGroup) {
		RegisterProtectedRoutesWithDeps(r, sharedapi.Dependencies{DB: db})
	})

	for _, route := range []string{
		"GET /api/v1/monitor/server",
		"GET /api/v1/monitor/mysql",
		"GET /api/v1/monitor/redis",
		"GET /api/v1/monitor/jobs",
		"GET /api/v1/monitor/jobs/health",
		"POST /api/v1/monitor/jobs",
		"PUT /api/v1/monitor/jobs/:id",
		"DELETE /api/v1/monitor/jobs/:id",
		"POST /api/v1/monitor/jobs/:id/start",
		"POST /api/v1/monitor/jobs/:id/stop",
		"POST /api/v1/monitor/jobs/:id/run",
		"POST /api/v1/monitor/job-logs/cleanup",
	} {
		if _, ok := routes[route]; !ok {
			t.Fatalf("route registration is missing: %s", route)
		}
	}
}

func setupMonitorRouteSQLMock(t *testing.T) *gorm.DB {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock db: %v", err)
	}
	mock.ExpectQuery("SELECT \\* FROM \"scheduled_jobs\" WHERE status = \\$\\d+").
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm sqlmock db: %v", err)
	}

	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet database expectations: %v", err)
		}
		_ = sqlDB.Close()
	})
	return db
}

func registeredMonitorRoutes(register func(*gin.RouterGroup)) map[string]struct{} {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1")
	register(group)

	routes := make(map[string]struct{}, len(router.Routes()))
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	return routes
}
