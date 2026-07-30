package auth

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// last_seen_at used to be written on every authenticated request. The throttle
// must push the staleness bound into the WHERE clause, so a request arriving
// inside the window updates zero rows instead of taking the row lock.
func TestConsoleSessionDAOTouchThrottlesByLastSeenAge(t *testing.T) {
	db, mock := newAuthDAOTestDB(t)
	now := time.Now().UTC()
	staleAfter := 60 * time.Second

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "console_sessions" SET "last_seen_at"=$1 WHERE session_id = $2 AND (last_seen_at IS NULL OR last_seen_at <= $3)`)).
		WithArgs(now, "session-1", now.Add(-staleAfter)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	if err := NewConsoleSessionDAO(db).TouchContext(context.Background(), "session-1", now, staleAfter); err != nil {
		t.Fatalf("TouchContext() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// staleAfter <= 0 restores the unconditional write, so the throttle can be
// turned off by configuration without changing behavior in any other way.
func TestConsoleSessionDAOTouchWritesUnconditionallyWhenThrottleDisabled(t *testing.T) {
	db, mock := newAuthDAOTestDB(t)
	now := time.Now().UTC()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "console_sessions" SET "last_seen_at"=$1 WHERE session_id = $2`)).
		WithArgs(now, "session-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := NewConsoleSessionDAO(db).TouchContext(context.Background(), "session-1", now, 0); err != nil {
		t.Fatalf("TouchContext() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
