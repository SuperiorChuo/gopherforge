package monitor

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestRegisterProtectedRoutes(t *testing.T) {
	setupMonitorRouteSQLMock(t)
	routes := registeredMonitorRoutes(func(r *gin.RouterGroup) {
		RegisterProtectedRoutes(r)
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

func setupMonitorRouteSQLMock(t *testing.T) {
	t.Helper()

	oldDB := database.DB
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock db: %v", err)
	}
	mock.ExpectQuery("SELECT \\* FROM `scheduled_jobs` WHERE status = \\?").
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm sqlmock db: %v", err)
	}

	database.DB = db
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet database expectations: %v", err)
		}
		_ = sqlDB.Close()
		database.DB = oldDB
	})
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
