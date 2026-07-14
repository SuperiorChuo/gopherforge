package monitor

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestMySQLDAOGetServerStatsContext(t *testing.T) {
	db, mock := newMonitorDAOTestDB(t)
	mock.ExpectQuery("FROM pg_stat_database s").
		WillReturnRows(sqlmock.NewRows([]string{
			"uptime_seconds", "connections", "active_connections", "max_connections",
			"commits", "rollbacks", "rows_returned", "rows_inserted", "rows_updated",
			"rows_deleted", "blocks_read", "blocks_hit", "temp_bytes",
		}).AddRow(120, 7, 2, 100, 200, 40, 900, 30, 20, 10, 55, 4500, 1024))

	stats, err := NewMySQLDAO(db).GetServerStatsContext(context.Background())
	if err != nil {
		t.Fatalf("GetServerStatsContext() error = %v", err)
	}
	want := MySQLServerStats{
		UptimeSeconds:     120,
		Connections:       7,
		ActiveConnections: 2,
		MaxConnections:    100,
		Commits:           200,
		Rollbacks:         40,
		RowsReturned:      900,
		RowsInserted:      30,
		RowsUpdated:       20,
		RowsDeleted:       10,
		BlocksRead:        55,
		BlocksHit:         4500,
		TempBytes:         1024,
	}
	if stats != want {
		t.Fatalf("stats = %#v, want %#v", stats, want)
	}
}

func TestMySQLDAOGetVersionContextHonorsCanceledContext(t *testing.T) {
	db, _ := newMonitorDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewMySQLDAO(db).GetVersionContext(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetVersionContext() error = %v, want context.Canceled", err)
	}
}

func TestMySQLDAOGetTableStatsContext(t *testing.T) {
	db, mock := newMonitorDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("pg_database_size($2) AS database_size")).
		WithArgs("go_admin", "go_admin").
		WillReturnRows(sqlmock.NewRows([]string{"table_count", "database_size"}).
			AddRow(12, 4096))

	stats, err := NewMySQLDAO(db).GetTableStatsContext(context.Background(), "go_admin")
	if err != nil {
		t.Fatalf("GetTableStatsContext() error = %v", err)
	}
	if stats.TableCount != 12 || stats.DatabaseSize != 4096 {
		t.Fatalf("stats = %#v, want table_count=12 database_size=4096", stats)
	}
}

func TestMySQLDAOGetVersionContext(t *testing.T) {
	db, mock := newMonitorDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SHOW server_version")).
		WillReturnRows(sqlmock.NewRows([]string{"server_version"}).AddRow("16.3"))

	version, err := NewMySQLDAO(db).GetVersionContext(context.Background())
	if err != nil {
		t.Fatalf("GetVersionContext() error = %v", err)
	}
	if version != "16.3" {
		t.Fatalf("version = %q, want 16.3", version)
	}
}

func TestMySQLDAOGetCurrentDatabaseContext(t *testing.T) {
	db, mock := newMonitorDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT current_database()")).
		WillReturnRows(sqlmock.NewRows([]string{"current_database"}).AddRow("go_admin"))

	current, err := NewMySQLDAO(db).GetCurrentDatabaseContext(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentDatabaseContext() error = %v", err)
	}
	if current != "go_admin" {
		t.Fatalf("current database = %q, want go_admin", current)
	}
}

func TestMySQLDAOUsesInjectedDB(t *testing.T) {
	db, mock := newMonitorDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SHOW server_version")).
		WillReturnRows(sqlmock.NewRows([]string{"server_version"}).AddRow("16.3"))

	version, err := NewMySQLDAO(db).GetVersionContext(context.Background())
	if err != nil {
		t.Fatalf("GetVersionContext() error = %v", err)
	}
	if version != "16.3" {
		t.Fatalf("version = %q, want 16.3", version)
	}
}

// newMonitorDAOTestDB returns a sqlmock-backed *gorm.DB for constructor
// injection into monitor DAOs. It never touches the global database.DB.
func newMonitorDAOTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock db: %v", err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet database expectations: %v", err)
		}
		_ = sqlDB.Close()
	})
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm sqlmock db: %v", err)
	}
	return db, mock
}
