package system

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	model "github.com/go-admin-kit/services/shared/pkg/model"
)

func expectMenuSeedInventory(mock sqlmock.Sqlmock, rows *sqlmock.Rows) {
	mock.ExpectExec("LOCK TABLE menus IN SHARE ROW EXCLUSIVE MODE").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT "id","name","path" FROM "menus" LIMIT`).WithArgs(10001).WillReturnRows(rows)
}

func expectMenuSequenceAdvance(mock sqlmock.Sqlmock) {
	mock.ExpectExec(`SELECT setval\(pg_get_serial_sequence\('menus', 'id'\), GREATEST\(MAX\(id\), pg_sequence_last_value`).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func TestMenuSeedDAOPreservesPublicIDsAndRemapsNewChildren(t *testing.T) {
	db, mock := setupSystemDAOTestDB(t)
	mock.ExpectBegin()
	expectMenuSeedInventory(mock, sqlmock.NewRows([]string{"id", "name", "path"}).
		AddRow(35, "bpm", "/bpm").AddRow(122, "custom-menu", "/custom"))
	expectMenuSequenceAdvance(mock)
	now := time.Now()
	// No explicit ID is inserted. The parent is the public ID, not source seed 122.
	mock.ExpectQuery(`INSERT INTO "menus" \("name","title","icon","path","component","parent_id"`).
		WithArgs("bpm-new", "New", "", "/bpm/new", "", 35, 0, 1, 0, "", now, now).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(200002))
	mock.ExpectCommit()
	count, err := NewMenuSeedDAO(db).BootstrapDefaultMenusContext(context.Background(), []model.Menu{
		{ID: 123, Name: "bpm-new", Title: "New", Path: "/bpm/new", ParentID: 122}, // child first
		{ID: 122, Name: "bpm", Path: "/bpm"},
	}, now)
	if err != nil || count != 1 {
		t.Fatalf("bootstrap = (%d, %v), want (1, nil)", count, err)
	}
}

func TestMenuSeedDAOPublicIdentitiesAreIdempotentWithoutUpdates(t *testing.T) {
	db, mock := setupSystemDAOTestDB(t)
	mock.ExpectBegin()
	expectMenuSeedInventory(mock, sqlmock.NewRows([]string{"id", "name", "path"}).
		AddRow(35, "bpm", "/bpm").AddRow(900, "ai", "/ai").AddRow(901, "ai-assistant", "/ai/assistant"))
	mock.ExpectCommit() // No UPDATE, INSERT or sequence write is permitted.
	count, err := NewMenuSeedDAO(db).BootstrapDefaultMenusContext(context.Background(), []model.Menu{
		{ID: 122, Name: "bpm", Path: "/bpm"},
		{ID: 50, Name: "ai", Path: "/ai"},
		{ID: 51, Name: "ai-assistant", Path: "/ai/assistant", ParentID: 50},
	}, time.Now())
	if err != nil || count != 0 {
		t.Fatalf("bootstrap = (%d, %v), want (0, nil)", count, err)
	}
}

func TestMenuSeedDAODoesNotStealCustomIDs(t *testing.T) {
	db, mock := setupSystemDAOTestDB(t)
	mock.ExpectBegin()
	expectMenuSeedInventory(mock, sqlmock.NewRows([]string{"id", "name", "path"}).AddRow(70, "custom", "/custom"))
	expectMenuSequenceAdvance(mock)
	now := time.Now()
	mock.ExpectQuery(`INSERT INTO "menus" \("name","title","icon","path","component","parent_id"`).
		WithArgs("cc", "CC", "", "/cc", "", 0, 0, 1, 0, "", now, now).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(99002))
	mock.ExpectQuery(`INSERT INTO "menus" \("name","title","icon","path","component","parent_id"`).
		WithArgs("cc-new", "New", "", "/cc/new", "", 99002, 0, 1, 0, "", now, now).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(99003))
	mock.ExpectCommit()
	count, err := NewMenuSeedDAO(db).BootstrapDefaultMenusContext(context.Background(), []model.Menu{
		{ID: 71, Name: "cc-new", Title: "New", Path: "/cc/new", ParentID: 70},
		{ID: 70, Name: "cc", Title: "CC", Path: "/cc"},
	}, now)
	if err != nil || count != 2 {
		t.Fatalf("bootstrap = (%d, %v), want (2, nil)", count, err)
	}
}

func TestMenuSeedDAORefusesAmbiguousIdentitiesBeforeWriting(t *testing.T) {
	db, mock := setupSystemDAOTestDB(t)
	mock.ExpectBegin()
	expectMenuSeedInventory(mock, sqlmock.NewRows([]string{"id", "name", "path"}).
		AddRow(35, "bpm", "/old-bpm").AddRow(122, "custom", "/bpm"))
	mock.ExpectRollback()
	count, err := NewMenuSeedDAO(db).BootstrapDefaultMenusContext(context.Background(), []model.Menu{
		{ID: 1, Name: "new", Path: "/new"},
		{ID: 122, Name: "bpm", Path: "/bpm"},
	}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "ambiguous menu identity") || count != 0 {
		t.Fatalf("bootstrap = (%d, %v), want ambiguity refusal without writes", count, err)
	}
}

func TestMenuSeedDAORefusesCrossSeedIdentityReuseBeforeWriting(t *testing.T) {
	for _, exact := range []bool{false, true} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("exact=%t/reverse=%t", exact, reverse), func(t *testing.T) {
				db, mock := setupSystemDAOTestDB(t)
				mock.ExpectBegin()
				expectMenuSeedInventory(mock, sqlmock.NewRows([]string{"id", "name", "path"}).
					AddRow(35, "crm", "/bpm"))
				mock.ExpectRollback() // No INSERT, UPDATE or setval is allowed.
				seed := []model.Menu{
					{ID: 122, Name: "bpm", Path: "/bpm"},
					{ID: 80, Name: "crm", Path: "/crm"},
				}
				if exact {
					seed[1].Path = "/bpm" // A full match must not share a row with a fallback.
				}
				if reverse {
					seed[0], seed[1] = seed[1], seed[0]
				}
				// Even an earlier missing seed must not be inserted before validation.
				seed = append([]model.Menu{{ID: 1, Name: "new", Path: "/new"}}, seed...)
				count, err := NewMenuSeedDAO(db).BootstrapDefaultMenusContext(context.Background(), seed, time.Now())
				if err == nil || !strings.Contains(err.Error(), "share persisted ID 35") || count != 0 {
					t.Fatalf("bootstrap = (%d, %v), want cross-seed refusal without writes", count, err)
				}
			})
		}
	}
}

func TestMenuSeedDAOPrefersUniqueFullPublicIdentity(t *testing.T) {
	db, mock := setupSystemDAOTestDB(t)
	mock.ExpectBegin()
	expectMenuSeedInventory(mock, sqlmock.NewRows([]string{"id", "name", "path"}).
		AddRow(42, "Tenant", "/system/tenant").AddRow(24, "tenant", "/system/tenant"))
	expectMenuSequenceAdvance(mock)
	now := time.Now()
	mock.ExpectQuery(`INSERT INTO "menus" \("name","title","icon","path","component","parent_id"`).
		WithArgs("tenant-child", "New", "", "/system/tenant/child", "", 24, 0, 1, 0, "", now, now).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(300002))
	mock.ExpectCommit() // Both old rows remain untouched; no UPDATE or DELETE.
	count, err := NewMenuSeedDAO(db).BootstrapDefaultMenusContext(context.Background(), []model.Menu{
		{ID: 1, Name: "tenant", Path: "/system/tenant"},
		{ID: 2, Name: "tenant-child", Title: "New", Path: "/system/tenant/child", ParentID: 1},
	}, now)
	if err != nil || count != 1 {
		t.Fatalf("bootstrap = (%d, %v), want unique complete identity", count, err)
	}
}

func TestMenuSeedDAOPreservesOnlyKnownPublicSettingsPair(t *testing.T) {
	for _, tc := range []struct {
		name string
		ids  []uint
		fail bool
	}{
		{"public-pair", []uint{22, 23}, false},
		{"reverse-order", []uint{23, 22}, false},
		{"different-pair", []uint{22, 24}, true},
		{"extra-duplicate", []uint{22, 23, 99001}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := setupSystemDAOTestDB(t)
			mock.ExpectBegin()
			rows := sqlmock.NewRows([]string{"id", "name", "path"})
			for _, id := range tc.ids {
				rows.AddRow(id, "setting", "/system/setting")
			}
			expectMenuSeedInventory(mock, rows)
			now := time.Now()
			if tc.fail {
				mock.ExpectRollback()
			} else {
				expectMenuSequenceAdvance(mock)
				mock.ExpectQuery(`INSERT INTO "menus" \("name","title","icon","path","component","parent_id"`).
					WithArgs("setting-child", "New", "", "/system/setting/child", "", 23, 0, 1, 0, "", now, now).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(300002))
				mock.ExpectCommit() // Old 22 and 23 must not be modified or removed.
			}
			count, err := NewMenuSeedDAO(db).BootstrapDefaultMenusContext(context.Background(), []model.Menu{
				{ID: 1, Name: "setting", Path: "/system/setting"},
				{ID: 2, Name: "setting-child", Title: "New", Path: "/system/setting/child", ParentID: 1},
			}, now)
			if tc.fail {
				if err == nil || !strings.Contains(err.Error(), "ambiguous menu identity") || count != 0 {
					t.Fatalf("bootstrap = (%d, %v), want unknown duplicate refusal", count, err)
				}
			} else if err != nil || count != 1 {
				t.Fatalf("bootstrap = (%d, %v), want preserved public pair", count, err)
			}
		})
	}
}

func TestMenuSeedDAORefusesDuplicateFullIdentities(t *testing.T) {
	db, mock := setupSystemDAOTestDB(t)
	mock.ExpectBegin()
	expectMenuSeedInventory(mock, sqlmock.NewRows([]string{"id", "name", "path"}).
		AddRow(35, "bpm", "/bpm").AddRow(122, "bpm", "/bpm"))
	mock.ExpectRollback()
	count, err := NewMenuSeedDAO(db).BootstrapDefaultMenusContext(context.Background(), []model.Menu{
		{ID: 1, Name: "new", Path: "/new"}, {ID: 2, Name: "bpm", Path: "/bpm"},
	}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "ambiguous menu identity") || count != 0 {
		t.Fatalf("bootstrap = (%d, %v), want full-identity ambiguity refusal", count, err)
	}
}

func TestMenuSeedDAORefusesInvalidTree(t *testing.T) {
	for _, tc := range []struct {
		name string
		seed []model.Menu
	}{
		{"cycle", []model.Menu{{ID: 1, ParentID: 2}, {ID: 2, ParentID: 1}}},
		{"missing-parent", []model.Menu{{ID: 1, ParentID: 2}}},
		{"duplicate-id", []model.Menu{{ID: 1}, {ID: 1}}},
		{"zero-id", []model.Menu{{ID: 0}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := setupSystemDAOTestDB(t)
			mock.ExpectBegin()
			expectMenuSeedInventory(mock, sqlmock.NewRows([]string{"id", "name", "path"}))
			mock.ExpectRollback()
			count, err := NewMenuSeedDAO(db).BootstrapDefaultMenusContext(context.Background(), tc.seed, time.Now())
			if err == nil || count != 0 {
				t.Fatalf("bootstrap = (%d, %v), want invalid tree refusal", count, err)
			}
		})
	}
}

func TestMenuSeedDAORollbackDoesNotReportCreatedRows(t *testing.T) {
	db, mock := setupSystemDAOTestDB(t)
	mock.ExpectBegin()
	expectMenuSeedInventory(mock, sqlmock.NewRows([]string{"id", "name", "path"}))
	expectMenuSequenceAdvance(mock)
	mock.ExpectQuery(`INSERT INTO "menus"`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery(`INSERT INTO "menus"`).WillReturnError(errors.New("insert failed"))
	mock.ExpectRollback()
	count, err := NewMenuSeedDAO(db).BootstrapDefaultMenusContext(context.Background(), []model.Menu{
		{ID: 1, Name: "one"}, {ID: 2, Name: "two"},
	}, time.Now())
	if err == nil || count != 0 {
		t.Fatalf("bootstrap = (%d, %v), want rolled back count 0", count, err)
	}
}
