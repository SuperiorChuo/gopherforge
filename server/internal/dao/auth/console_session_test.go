package auth

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestConsoleSessionDAOGetBySessionID(t *testing.T) {
	mock := setupAuthDAOTestDB(t)
	now := time.Now().UTC()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `wm_console_session` WHERE session_id = ? ORDER BY `wm_console_session`.`session_id` LIMIT ?")).
		WithArgs("session-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"session_id", "username", "issued_at", "expires_at", "created_at"}).
			AddRow("session-1", "alice", now, now.Add(time.Hour), now))

	record, err := (ConsoleSessionDAO{}).GetBySessionID("session-1")
	if err != nil {
		t.Fatalf("GetBySessionID() error = %v", err)
	}
	if record.SessionID != "session-1" || record.Username != "alice" {
		t.Fatalf("record = %#v, want session-1/alice", record)
	}
}

func TestConsoleSessionDAOGetBySessionIDContextHonorsCanceledContext(t *testing.T) {
	setupAuthDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (ConsoleSessionDAO{}).GetBySessionIDContext(ctx, "session-1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetBySessionIDContext() error = %v, want context.Canceled", err)
	}
}

func TestConsoleSessionDAOReadyReflectsGlobalDatabase(t *testing.T) {
	oldDB := database.DB
	database.DB = nil
	t.Cleanup(func() {
		database.DB = oldDB
	})

	if (ConsoleSessionDAO{}).Ready() {
		t.Fatal("Ready() should be false when database is nil")
	}
}

func TestConsoleSessionDAOUsesInjectedDB(t *testing.T) {
	oldDB := database.DB
	database.DB = nil
	t.Cleanup(func() {
		database.DB = oldDB
	})

	db, mock := newInjectedAuthDAOTestDB(t)
	now := time.Now().UTC()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `wm_console_session` WHERE session_id = ? ORDER BY `wm_console_session`.`session_id` LIMIT ?")).
		WithArgs("session-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"session_id", "username", "issued_at", "expires_at", "created_at"}).
			AddRow("session-1", "alice", now, now.Add(time.Hour), now))

	record, err := NewConsoleSessionDAO(db).GetBySessionID("session-1")
	if err != nil {
		t.Fatalf("GetBySessionID() error = %v", err)
	}
	if record.SessionID != "session-1" || record.Username != "alice" {
		t.Fatalf("record = %#v, want session-1/alice", record)
	}
}

func setupAuthDAOTestDB(t *testing.T) sqlmock.Sqlmock {
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
