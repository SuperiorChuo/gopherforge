package system

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	localmodel "github.com/go-admin-kit/services/identity/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/authz"
)

const exportSelectPrefix = `SELECT users.id,users.username,users.nickname,users.email,users.phone,users.department_id,users.status,users.created_at FROM "users"`

// exportRow builds one sqlmock row in the export column shape.
func exportRows(ids ...uint) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"id", "username", "created_at"})
	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	for _, id := range ids {
		// created_at descends with id so the fixture matches the sort order.
		rows.AddRow(id, "user", base.Add(-time.Duration(id)*time.Minute))
	}
	return rows
}

func exportCreatedAt(id uint) time.Time {
	return time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC).Add(-time.Duration(id) * time.Minute)
}

func expectExportFirstPage(mock sqlmock.Sqlmock, limit int, rows *sqlmock.Rows) {
	mock.ExpectQuery(regexp.QuoteMeta(exportSelectPrefix + ` ORDER BY users.created_at DESC, users.id DESC LIMIT $1`)).
		WithArgs(limit).
		WillReturnRows(rows)
}

func expectExportCursorPage(mock sqlmock.Sqlmock, cursorID uint, limit int, rows *sqlmock.Rows) {
	mock.ExpectQuery(regexp.QuoteMeta(exportSelectPrefix+` WHERE (users.created_at, users.id) < ($1, $2) ORDER BY users.created_at DESC, users.id DESC LIMIT $3`)).
		WithArgs(exportCreatedAt(cursorID), cursorID, limit).
		WillReturnRows(rows)
}

func exportTestRequest() UserListRequest {
	return UserListRequest{DataScope: authz.UserDataScope{Scope: authz.DataScopeAll}}
}

func TestExportUsersContextStopsOnShortPage(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	// A page shorter than the page size ends the walk — no COUNT is issued at
	// any point, which is the whole point of the rewrite.
	expectExportFirstPage(mock, userExportPageSize, exportRows(3, 2, 1))

	svc := NewUserServiceWithDB(db)
	users, truncated, err := (&svc).ExportUsersContext(context.Background(), exportTestRequest())
	if err != nil {
		t.Fatalf("ExportUsersContext() error = %v", err)
	}
	if truncated {
		t.Fatal("ExportUsersContext() truncated = true, want false")
	}
	if len(users) != 3 || users[0].ID != 3 || users[2].ID != 1 {
		t.Fatalf("ExportUsersContext() = %+v, want three newest-first rows", users)
	}
}

func TestExportUsersContextWalksPagesWithoutGapsOrDuplicates(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)

	// Two full pages then a short one. Each page's cursor is the previous
	// page's last row, so the concatenation must be strictly descending with
	// no repeats — the property plain OFFSET loses when rows shift underneath.
	first := make([]uint, 0, userExportPageSize)
	for id := uint(1500); id > 1500-uint(userExportPageSize); id-- {
		first = append(first, id)
	}
	second := make([]uint, 0, userExportPageSize)
	for id := uint(1000); id > 1000-uint(userExportPageSize); id-- {
		second = append(second, id)
	}

	expectExportFirstPage(mock, userExportPageSize, exportRows(first...))
	expectExportCursorPage(mock, first[len(first)-1], userExportPageSize, exportRows(second...))
	expectExportCursorPage(mock, second[len(second)-1], userExportPageSize, exportRows(7, 6))

	svc := NewUserServiceWithDB(db)
	users, truncated, err := (&svc).ExportUsersContext(context.Background(), exportTestRequest())
	if err != nil {
		t.Fatalf("ExportUsersContext() error = %v", err)
	}
	if truncated {
		t.Fatal("ExportUsersContext() truncated = true, want false")
	}
	if len(users) != userExportPageSize*2+2 {
		t.Fatalf("ExportUsersContext() len = %d, want %d", len(users), userExportPageSize*2+2)
	}

	seen := make(map[uint]bool, len(users))
	for i, u := range users {
		if seen[u.ID] {
			t.Fatalf("duplicate row id=%d at index %d", u.ID, i)
		}
		seen[u.ID] = true
		if i > 0 && users[i-1].ID <= u.ID {
			t.Fatalf("order broke at index %d: %d follows %d", i, u.ID, users[i-1].ID)
		}
	}
}

func TestExportUsersContextReportsTruncationAtCap(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	svc := NewUserServiceWithDB(db)

	// Walk to one page short of the cap, then let the probe page return one
	// row past the cap: truncated must be true and the slice trimmed exactly.
	fullPages := UserExportCap / userExportPageSize
	nextID := uint(100000)
	var lastID uint
	for page := 0; page < fullPages-1; page++ {
		ids := make([]uint, 0, userExportPageSize)
		for i := 0; i < userExportPageSize; i++ {
			ids = append(ids, nextID)
			nextID--
		}
		if page == 0 {
			expectExportFirstPage(mock, userExportPageSize, exportRows(ids...))
		} else {
			expectExportCursorPage(mock, lastID, userExportPageSize, exportRows(ids...))
		}
		lastID = ids[len(ids)-1]
	}

	// Final page is requested with limit = remaining+1 (the probe).
	probeLimit := userExportPageSize + 1
	probeIDs := make([]uint, 0, probeLimit)
	for i := 0; i < probeLimit; i++ {
		probeIDs = append(probeIDs, nextID)
		nextID--
	}
	expectExportCursorPage(mock, lastID, probeLimit, exportRows(probeIDs...))

	users, truncated, err := (&svc).ExportUsersContext(context.Background(), exportTestRequest())
	if err != nil {
		t.Fatalf("ExportUsersContext() error = %v", err)
	}
	if !truncated {
		t.Fatal("ExportUsersContext() truncated = false, want true past the cap")
	}
	if len(users) != UserExportCap {
		t.Fatalf("ExportUsersContext() len = %d, want exactly UserExportCap", len(users))
	}
}

func TestExportUsersContextExactCapIsNotTruncated(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	svc := NewUserServiceWithDB(db)

	// Exactly UserExportCap rows exist: the probe page comes back one row short
	// of its limit, proving nothing follows. Reporting truncation here would be
	// a lie shown to the user in the sheet's last line.
	fullPages := UserExportCap / userExportPageSize
	nextID := uint(100000)
	var lastID uint
	for page := 0; page < fullPages-1; page++ {
		ids := make([]uint, 0, userExportPageSize)
		for i := 0; i < userExportPageSize; i++ {
			ids = append(ids, nextID)
			nextID--
		}
		if page == 0 {
			expectExportFirstPage(mock, userExportPageSize, exportRows(ids...))
		} else {
			expectExportCursorPage(mock, lastID, userExportPageSize, exportRows(ids...))
		}
		lastID = ids[len(ids)-1]
	}

	probeIDs := make([]uint, 0, userExportPageSize)
	for i := 0; i < userExportPageSize; i++ {
		probeIDs = append(probeIDs, nextID)
		nextID--
	}
	expectExportCursorPage(mock, lastID, userExportPageSize+1, exportRows(probeIDs...))

	users, truncated, err := (&svc).ExportUsersContext(context.Background(), exportTestRequest())
	if err != nil {
		t.Fatalf("ExportUsersContext() error = %v", err)
	}
	if truncated {
		t.Fatal("ExportUsersContext() truncated = true at exactly the cap, want false")
	}
	if len(users) != UserExportCap {
		t.Fatalf("ExportUsersContext() len = %d, want UserExportCap", len(users))
	}
}

func TestStreamExportUsersContextEmitsPerPageAndPropagatesEmitError(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	expectExportFirstPage(mock, userExportPageSize, exportRows(3, 2, 1))

	svc := NewUserServiceWithDB(db)
	var pages int
	emitErr := errors.New("write failed")
	// A failing sheet write must abort the export instead of silently
	// producing a short file.
	_, err := (&svc).StreamExportUsersContext(context.Background(), exportTestRequest(),
		func(batch []localmodel.User) error {
			pages++
			return emitErr
		})
	if !errors.Is(err, emitErr) {
		t.Fatalf("StreamExportUsersContext() error = %v, want the emit error", err)
	}
	if pages != 1 {
		t.Fatalf("emit called %d times, want 1", pages)
	}
}

func TestStreamExportUsersContextEmptyResultEmitsNothing(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	expectExportFirstPage(mock, userExportPageSize, exportRows())

	svc := NewUserServiceWithDB(db)
	calls := 0
	truncated, err := (&svc).StreamExportUsersContext(context.Background(), exportTestRequest(),
		func(batch []localmodel.User) error {
			calls++
			return nil
		})
	if err != nil {
		t.Fatalf("StreamExportUsersContext() error = %v", err)
	}
	if truncated || calls != 0 {
		t.Fatalf("StreamExportUsersContext() truncated=%v calls=%d, want false/0", truncated, calls)
	}
}
