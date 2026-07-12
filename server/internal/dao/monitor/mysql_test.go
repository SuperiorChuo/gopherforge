package monitor

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestMySQLDAOGetNameValuesContext(t *testing.T) {
	db, mock := newMonitorDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SHOW GLOBAL STATUS")).
		WillReturnRows(sqlmock.NewRows([]string{"Variable_name", "Value"}).
			AddRow("Uptime", "120").
			AddRow("Questions", "240"))

	values, err := NewMySQLDAO(db).GetNameValuesContext(context.Background(), "SHOW GLOBAL STATUS")
	if err != nil {
		t.Fatalf("GetNameValuesContext() error = %v", err)
	}
	want := map[string]string{"Uptime": "120", "Questions": "240"}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("values = %#v, want %#v", values, want)
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
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) AS table_count, COALESCE(SUM(data_length + index_length), 0) AS database_size
		 FROM information_schema.tables
		 WHERE table_schema = ?`)).
		WithArgs("go_admin").
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
	mock.ExpectQuery(regexp.QuoteMeta("SELECT VERSION()")).
		WillReturnRows(sqlmock.NewRows([]string{"VERSION()"}).AddRow("8.0.36"))

	version, err := NewMySQLDAO(db).GetVersionContext(context.Background())
	if err != nil {
		t.Fatalf("GetVersionContext() error = %v", err)
	}
	if version != "8.0.36" {
		t.Fatalf("version = %q, want 8.0.36", version)
	}
}

func TestMySQLDAOUsesInjectedDB(t *testing.T) {
	db, mock := newMonitorDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT VERSION()")).
		WillReturnRows(sqlmock.NewRows([]string{"VERSION()"}).AddRow("8.0.36"))

	version, err := NewMySQLDAO(db).GetVersionContext(context.Background())
	if err != nil {
		t.Fatalf("GetVersionContext() error = %v", err)
	}
	if version != "8.0.36" {
		t.Fatalf("version = %q, want 8.0.36", version)
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
	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm sqlmock db: %v", err)
	}
	return db, mock
}
