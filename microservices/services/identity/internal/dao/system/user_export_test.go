package system

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/services/identity/internal/pkg/authz"
	"github.com/go-admin-kit/services/identity/internal/pkg/tenant"
)

const exportSelectColumns = `SELECT users.id,users.username,users.nickname,users.email,users.phone,users.department_id,users.status,users.created_at FROM "users"`

func TestExportUsersPageContextFirstPageHasNoCursorPredicate(t *testing.T) {
	db, mock := setupSystemDAOTestDB(t)
	// No cursor → no keyset predicate, and no OFFSET anywhere.
	mock.ExpectQuery(regexp.QuoteMeta(exportSelectColumns + ` ORDER BY users.created_at DESC, users.id DESC LIMIT $1`)).
		WithArgs(500).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).
			AddRow(uint(3), "carol").
			AddRow(uint(2), "bob"))

	users, err := NewUserDAO(db).ExportUsersPageContext(
		context.Background(),
		ExportUserCursor{},
		500,
		"",
		nil,
		authz.UserDataScope{Scope: authz.DataScopeAll},
	)
	if err != nil {
		t.Fatalf("ExportUsersPageContext() error = %v", err)
	}
	if len(users) != 2 || users[0].ID != 3 {
		t.Fatalf("ExportUsersPageContext() = %+v, want newest-first rows", users)
	}
}

func TestExportUsersPageContextAppliesKeysetCursor(t *testing.T) {
	db, mock := setupSystemDAOTestDB(t)
	cursorAt := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	// Row-value comparison on (created_at, id): ties on created_at cannot skip
	// or repeat a row the way a plain OFFSET over a non-unique key could.
	mock.ExpectQuery(regexp.QuoteMeta(exportSelectColumns+` WHERE (users.created_at, users.id) < ($1, $2) ORDER BY users.created_at DESC, users.id DESC LIMIT $3`)).
		WithArgs(cursorAt, uint(7), 500).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(uint(6), "frank"))

	users, err := NewUserDAO(db).ExportUsersPageContext(
		context.Background(),
		ExportUserCursor{CreatedAt: cursorAt, ID: 7},
		500,
		"",
		nil,
		authz.UserDataScope{Scope: authz.DataScopeAll},
	)
	if err != nil {
		t.Fatalf("ExportUsersPageContext() error = %v", err)
	}
	if len(users) != 1 || users[0].ID != 6 {
		t.Fatalf("ExportUsersPageContext() = %+v, want the row after the cursor", users)
	}
}

func TestExportUsersPageContextKeepsFiltersTenantAndDataScope(t *testing.T) {
	db, mock := setupSystemDAOTestDB(t)
	status := int8(1)
	// Tenant isolation, keyword and status filters, and the data-scope plugin
	// predicate must all survive the rewrite — the export must never widen
	// beyond what the list endpoint shows.
	mock.ExpectQuery(regexp.QuoteMeta(exportSelectColumns+` WHERE users.tenant_id = $1 AND (username LIKE $2 OR nickname LIKE $3 OR email LIKE $4 OR phone LIKE $5) AND status = $6 AND department_id IN ($7,$8) ORDER BY users.created_at DESC, users.id DESC LIMIT $9`)).
		WithArgs(uint(4), "%ali%", "%ali%", "%ali%", "%ali%", status, uint(10), uint(11), 500).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	ctx := tenant.WithContext(context.Background(), 4)
	_, err := NewUserDAO(db).ExportUsersPageContext(
		ctx,
		ExportUserCursor{},
		500,
		"ali",
		&status,
		authz.UserDataScope{Scope: authz.DataScopeDepartment, DepartmentIDs: []uint{10, 11}},
	)
	if err != nil {
		t.Fatalf("ExportUsersPageContext() error = %v", err)
	}
}

func TestExportUsersPageContextHonorsCanceledContext(t *testing.T) {
	db, _ := setupSystemDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewUserDAO(db).ExportUsersPageContext(
		ctx,
		ExportUserCursor{},
		500,
		"",
		nil,
		authz.UserDataScope{Scope: authz.DataScopeAll},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ExportUsersPageContext() error = %v, want context.Canceled", err)
	}
}

func TestExportUserCursorValid(t *testing.T) {
	if (ExportUserCursor{}).Valid() {
		t.Fatal("zero cursor must not be valid")
	}
	if !(ExportUserCursor{ID: 1}).Valid() {
		t.Fatal("cursor with an id must be valid")
	}
}
