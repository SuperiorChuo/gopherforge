package auth

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestConsoleSessionDAOGetBySessionIDContext(t *testing.T) {
	db, mock := newAuthDAOTestDB(t)
	now := time.Now().UTC()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `console_sessions` WHERE session_id = ? ORDER BY `console_sessions`.`session_id` LIMIT ?")).
		WithArgs("session-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"session_id", "username", "issued_at", "expires_at", "created_at"}).
			AddRow("session-1", "alice", now, now.Add(time.Hour), now))

	record, err := NewConsoleSessionDAO(db).GetBySessionIDContext(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("GetBySessionIDContext() error = %v", err)
	}
	if record.SessionID != "session-1" || record.Username != "alice" {
		t.Fatalf("record = %#v, want session-1/alice", record)
	}
}

func TestConsoleSessionDAOGetBySessionIDContextHonorsCanceledContext(t *testing.T) {
	db, _ := newAuthDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewConsoleSessionDAO(db).GetBySessionIDContext(ctx, "session-1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetBySessionIDContext() error = %v, want context.Canceled", err)
	}
}

func TestConsoleSessionDAOReadyReflectsInjectedDatabase(t *testing.T) {
	db, _ := newAuthDAOTestDB(t)

	if !NewConsoleSessionDAO(db).Ready() {
		t.Fatal("Ready() = false, want true when a database is injected")
	}
}

func TestConsoleSessionDAOUsesInjectedDB(t *testing.T) {
	db, mock := newAuthDAOTestDB(t)
	now := time.Now().UTC()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `console_sessions` WHERE session_id = ? ORDER BY `console_sessions`.`session_id` LIMIT ?")).
		WithArgs("session-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"session_id", "username", "issued_at", "expires_at", "created_at"}).
			AddRow("session-1", "alice", now, now.Add(time.Hour), now))

	record, err := NewConsoleSessionDAO(db).GetBySessionIDContext(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("GetBySessionIDContext() error = %v", err)
	}
	if record.SessionID != "session-1" || record.Username != "alice" {
		t.Fatalf("record = %#v, want session-1/alice", record)
	}
}
