// Package testdb supplies isolated databases for complete-set query regressions.
// It is imported only by tests, never by service entry points.
package testdb

import (
	"crypto/rand"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/shared/pkg/fullread"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const IsolatedDSN = "host=127.0.0.1 port=5432 user=postgres password=isolated-test-only dbname=release_fullread sslmode=disable"

// Open normally uses SQLite. PostgreSQL requires BOTH an exact disposable DSN
// and the explicit release harness confirmation; each test owns a random schema.
func Open(t testing.TB, models ...any) *gorm.DB {
	t.Helper()
	config := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), PrepareStmt: true}
	var db *gorm.DB
	var err error
	dsn := os.Getenv("GAK_FULLREAD_TEST_DSN")
	if dsn == "" {
		db, err = gorm.Open(sqlite.Open(":memory:"), config)
	} else {
		if dsn != IsolatedDSN || os.Getenv("GAK_RELEASE_TEST_CONFIRM") != "isolated-release-database" {
			t.Fatal("requires the explicit isolated fullread database harness")
		}
		admin, openErr := gorm.Open(postgres.Open(dsn), config)
		if openErr != nil {
			t.Fatal(openErr)
		}
		pool, _ := admin.DB()
		t.Cleanup(func() { pool.Close() })
		var nonce [12]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			t.Fatal(err)
		}
		schema := fmt.Sprintf("release_fullread_%x", nonce)
		if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error; err != nil {
				t.Error(err)
			}
		})
		db, err = gorm.Open(postgres.Open(dsn+" search_path="+schema), config)
	}
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if dsn == "" {
		pool.SetMaxOpenConns(1)
	} else {
		pool.SetMaxOpenConns(4)
	}
	t.Cleanup(func() { pool.Close() })
	cache, ok := db.Statement.ConnPool.(*gorm.PreparedStmtDB)
	if !ok {
		t.Fatal("complete-set fixture must use the production prepared-statement setting")
	}
	t.Cleanup(func() {
		defer cache.Close()
		for _, sql := range cache.Stmts.Keys() {
			if strings.HasPrefix(sql, "DECLARE gak_fullread_") || strings.HasPrefix(sql, "FETCH FORWARD ") || strings.HasPrefix(sql, "CLOSE gak_fullread_") {
				t.Error("one-shot cursor SQL leaked into the shared prepared cache")
			}
		}
	})
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatal(err)
	}
	t.Logf("complete-set fixture dialect=%s", db.Dialector.Name())
	return db
}

var callbackID atomic.Uint64

// Batches requires a multi-fetch read, and catches a regression to one large
// Find even when its final result happens to be complete on the fixture.
func Batches(t testing.TB, db *gorm.DB, read func()) {
	t.Helper()
	name := fmt.Sprintf("test:complete_batches_%d", callbackID.Add(1))
	fetches := 0
	if err := db.Callback().Query().After("gorm:after_query").Register(name, func(q *gorm.DB) {
		if q.DryRun || q.Error != nil {
			return
		}
		sql := q.Statement.SQL.String()
		if strings.HasPrefix(sql, "FETCH FORWARD ") || strings.HasPrefix(sql, "SELECT ") {
			fetches++
			if q.RowsAffected > fullread.BatchSize {
				t.Errorf("unbounded fetch: %d rows", q.RowsAffected)
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Query().Remove(name)
	read()
	if fetches < 3 {
		t.Fatalf("expected at least three bounded fetches, got %d", fetches)
	}
}
