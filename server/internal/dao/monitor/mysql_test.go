package monitor

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestMySQLDAOGetNameValues(t *testing.T) {
	mock := setupMonitorDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SHOW GLOBAL STATUS")).
		WillReturnRows(sqlmock.NewRows([]string{"Variable_name", "Value"}).
			AddRow("Uptime", "120").
			AddRow("Questions", "240"))

	values, err := NewMySQLDAO().GetNameValues("SHOW GLOBAL STATUS")
	if err != nil {
		t.Fatalf("GetNameValues() error = %v", err)
	}
	want := map[string]string{"Uptime": "120", "Questions": "240"}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("values = %#v, want %#v", values, want)
	}
}

func TestMySQLDAOGetVersionContextHonorsCanceledContext(t *testing.T) {
	setupMonitorDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewMySQLDAO().GetVersionContext(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetVersionContext() error = %v, want context.Canceled", err)
	}
}

func TestMySQLDAOGetTableStats(t *testing.T) {
	mock := setupMonitorDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) AS table_count, COALESCE(SUM(data_length + index_length), 0) AS database_size
		 FROM information_schema.tables
		 WHERE table_schema = ?`)).
		WithArgs("go_admin").
		WillReturnRows(sqlmock.NewRows([]string{"table_count", "database_size"}).
			AddRow(12, 4096))

	stats, err := NewMySQLDAO().GetTableStats("go_admin")
	if err != nil {
		t.Fatalf("GetTableStats() error = %v", err)
	}
	if stats.TableCount != 12 || stats.DatabaseSize != 4096 {
		t.Fatalf("stats = %#v, want table_count=12 database_size=4096", stats)
	}
}

func TestMySQLDAOGetVersion(t *testing.T) {
	mock := setupMonitorDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT VERSION()")).
		WillReturnRows(sqlmock.NewRows([]string{"VERSION()"}).AddRow("8.0.36"))

	version, err := NewMySQLDAO().GetVersion()
	if err != nil {
		t.Fatalf("GetVersion() error = %v", err)
	}
	if version != "8.0.36" {
		t.Fatalf("version = %q, want 8.0.36", version)
	}
}

func TestMySQLDAOUsesInjectedDB(t *testing.T) {
	oldDB := database.DB
	database.DB = nil
	t.Cleanup(func() {
		database.DB = oldDB
	})

	db, mock := newInjectedMonitorDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT VERSION()")).
		WillReturnRows(sqlmock.NewRows([]string{"VERSION()"}).AddRow("8.0.36"))

	version, err := NewMySQLDAO(db).GetVersion()
	if err != nil {
		t.Fatalf("GetVersion() error = %v", err)
	}
	if version != "8.0.36" {
		t.Fatalf("version = %q, want 8.0.36", version)
	}
}

func setupMonitorDAOTestDB(t *testing.T) sqlmock.Sqlmock {
	t.Helper()

	oldDB := database.DB
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock db: %v", err)
	}
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

	return mock
}

func newInjectedMonitorDAOTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open injected sqlmock db: %v", err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet injected database expectations: %v", err)
		}
		_ = sqlDB.Close()
	})
	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open injected gorm sqlmock db: %v", err)
	}
	return db, mock
}
