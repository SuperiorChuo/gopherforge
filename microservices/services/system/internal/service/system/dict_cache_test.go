package system

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	miniredis "github.com/alicebob/miniredis/v2"
	cachepkg "github.com/go-admin-kit/services/system/internal/pkg/cache"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	goredis "github.com/redis/go-redis/v9"
)

// setupDictCacheTestService wires a DictService onto a sqlmock database and a
// dedicated miniredis, so cache behaviour is observable without touching the
// package-global Redis handle.
func setupDictCacheTestService(t *testing.T) (DictService, sqlmock.Sqlmock, *miniredis.Miniredis) {
	t.Helper()

	db, mock := setupSystemUserServiceContextTestDB(t)
	store, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	client := goredis.NewClient(&goredis.Options{Addr: store.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		store.Close()
	})

	svc := NewDictServiceWithCache(db, cachepkg.NewCacheServiceWithClient(client))
	return svc, mock, store
}

func expectDictCodeQueries(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "dict_types" WHERE code IN ($1) AND status = $2`)).
		WithArgs("gender", int8(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "status"}).AddRow(uint(1), "gender", int8(1)))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "dict_items" WHERE dict_type_id IN ($1) AND status = $2 ORDER BY dict_type_id ASC, sort ASC, created_at ASC`)).
		WithArgs(uint(1), int8(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "dict_type_id", "label", "value"}).
			AddRow(uint(11), uint(1), "男", "1"))
}

func TestDictServiceGetMultipleDictDataContextServesSecondCallFromCache(t *testing.T) {
	svc, mock, _ := setupDictCacheTestService(t)
	// Exactly one pair of queries is expected; sqlmock's ExpectationsWereMet
	// plus ordered matching means a second database round trip would fail.
	expectDictCodeQueries(mock)

	ctx := context.Background()
	first, err := (&svc).GetMultipleDictDataContext(ctx, []string{"gender"})
	if err != nil {
		t.Fatalf("first GetMultipleDictDataContext(): %v", err)
	}
	second, err := (&svc).GetMultipleDictDataContext(ctx, []string{"gender"})
	if err != nil {
		t.Fatalf("second GetMultipleDictDataContext(): %v", err)
	}

	if len(first["gender"]) != 1 || len(second["gender"]) != 1 {
		t.Fatalf("cached read = %+v, want same single item as the cold read %+v", second, first)
	}
	if second["gender"][0].Label != first["gender"][0].Label {
		t.Fatalf("cached label = %q, want %q", second["gender"][0].Label, first["gender"][0].Label)
	}
}

func TestDictServiceGetAllDictDataContextServesSecondCallFromCache(t *testing.T) {
	svc, mock, _ := setupDictCacheTestService(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "dict_types" WHERE status = $1 ORDER BY created_at DESC`)).
		WithArgs(int8(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "status"}).AddRow(uint(1), "gender", int8(1)))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "dict_items" WHERE dict_type_id IN ($1) AND status = $2 ORDER BY dict_type_id ASC, sort ASC, created_at ASC`)).
		WithArgs(uint(1), int8(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "dict_type_id", "label"}).AddRow(uint(11), uint(1), "男"))

	ctx := context.Background()
	if _, err := (&svc).GetAllDictDataContext(ctx); err != nil {
		t.Fatalf("first GetAllDictDataContext(): %v", err)
	}
	cached, err := (&svc).GetAllDictDataContext(ctx)
	if err != nil {
		t.Fatalf("second GetAllDictDataContext(): %v", err)
	}
	if len(cached["gender"]) != 1 || cached["gender"][0].Label != "男" {
		t.Fatalf("cached /dicts/all = %+v, want the stored item", cached)
	}
}

func TestDictServiceCachesUnknownCodeAsNotFound(t *testing.T) {
	svc, mock, _ := setupDictCacheTestService(t)
	// One query pair only: a bogus code must not re-hit the database.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "dict_types" WHERE code IN ($1) AND status = $2`)).
		WithArgs("nope", int8(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "status"}))

	ctx := context.Background()
	for i := 0; i < 2; i++ {
		data, err := (&svc).GetMultipleDictDataContext(ctx, []string{"nope"})
		if err != nil {
			t.Fatalf("GetMultipleDictDataContext() call %d: %v", i+1, err)
		}
		if len(data) != 0 {
			t.Fatalf("GetMultipleDictDataContext() = %+v, want unknown code omitted", data)
		}
	}
}

func TestDictServiceIsolatesTenantsInCache(t *testing.T) {
	svc, mock, _ := setupDictCacheTestService(t)
	// Tenant 1 warms its own key; tenant 2 must still query.
	expectDictCodeQueries(mock)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "dict_types" WHERE code IN ($1) AND status = $2`)).
		WithArgs("gender", int8(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "status"}).AddRow(uint(9), "gender", int8(1)))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "dict_items" WHERE dict_type_id IN ($1) AND status = $2 ORDER BY dict_type_id ASC, sort ASC, created_at ASC`)).
		WithArgs(uint(9), int8(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "dict_type_id", "label", "value"}).
			AddRow(uint(91), uint(9), "other-tenant", "1"))

	tenantOne := tenant.WithContext(context.Background(), 1)
	tenantTwo := tenant.WithContext(context.Background(), 2)

	one, err := (&svc).GetMultipleDictDataContext(tenantOne, []string{"gender"})
	if err != nil {
		t.Fatalf("tenant 1 read: %v", err)
	}
	two, err := (&svc).GetMultipleDictDataContext(tenantTwo, []string{"gender"})
	if err != nil {
		t.Fatalf("tenant 2 read: %v", err)
	}

	if one["gender"][0].Label != "男" {
		t.Fatalf("tenant 1 = %+v, want its own row", one["gender"])
	}
	if two["gender"][0].Label != "other-tenant" {
		t.Fatalf("tenant 2 = %+v, want its own row rather than tenant 1's cache entry", two["gender"])
	}
}

// The write paths below each assert that the cached read is dropped, so a
// dictionary edit is visible on the next request. One case per write endpoint:
// missing any of them is exactly the "changed it but nothing happened" bug.
func TestDictServiceWritePathsInvalidateCache(t *testing.T) {
	cases := []struct {
		name        string
		expectWrite func(sqlmock.Sqlmock)
		write       func(*testing.T, *DictService, context.Context)
	}{
		{
			name: "create type",
			expectWrite: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "dict_types" WHERE code = $1 ORDER BY "dict_types"."id" LIMIT $2`)).
					WithArgs("new-code", 1).
					WillReturnRows(sqlmock.NewRows([]string{"id"}))
				mock.ExpectBegin()
				mock.ExpectQuery(`INSERT INTO "dict_types"`).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uint(5)))
				mock.ExpectCommit()
			},
			write: func(t *testing.T, svc *DictService, ctx context.Context) {
				if _, err := svc.CreateTypeContext(ctx, CreateDictTypeRequest{Name: "New", Code: "new-code"}); err != nil {
					t.Fatalf("CreateTypeContext(): %v", err)
				}
			},
		},
		{
			name: "update type",
			expectWrite: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "dict_types" WHERE "dict_types"."id" = $1 ORDER BY "dict_types"."id" LIMIT $2`)).
					WithArgs(uint(5), 1).
					WillReturnRows(sqlmock.NewRows([]string{"id", "code"}).AddRow(uint(5), "gender"))
				mock.ExpectBegin()
				mock.ExpectExec(`UPDATE "dict_types" SET`).
					WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			},
			write: func(t *testing.T, svc *DictService, ctx context.Context) {
				if _, err := svc.UpdateTypeContext(ctx, 5, UpdateDictTypeRequest{Name: "Renamed"}); err != nil {
					t.Fatalf("UpdateTypeContext(): %v", err)
				}
			},
		},
		{
			name: "delete type",
			expectWrite: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "dict_items" WHERE dict_type_id = $1`)).
					WithArgs(uint(5)).
					WillReturnResult(sqlmock.NewResult(0, 2))
				mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "dict_types" WHERE "dict_types"."id" = $1`)).
					WithArgs(uint(5)).
					WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			},
			write: func(t *testing.T, svc *DictService, ctx context.Context) {
				if err := svc.DeleteTypeContext(ctx, 5); err != nil {
					t.Fatalf("DeleteTypeContext(): %v", err)
				}
			},
		},
		{
			name: "create item",
			expectWrite: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "dict_types" WHERE "dict_types"."id" = $1 ORDER BY "dict_types"."id" LIMIT $2`)).
					WithArgs(uint(1), 1).
					WillReturnRows(sqlmock.NewRows([]string{"id", "code"}).AddRow(uint(1), "gender"))
				mock.ExpectBegin()
				mock.ExpectQuery(`INSERT INTO "dict_items"`).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uint(12)))
				mock.ExpectCommit()
			},
			write: func(t *testing.T, svc *DictService, ctx context.Context) {
				if _, err := svc.CreateItemContext(ctx, CreateDictItemRequest{
					DictTypeID: 1, Label: "未知", Value: "0",
				}); err != nil {
					t.Fatalf("CreateItemContext(): %v", err)
				}
			},
		},
		{
			name: "update item",
			expectWrite: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "dict_items" WHERE "dict_items"."id" = $1 ORDER BY "dict_items"."id" LIMIT $2`)).
					WithArgs(uint(11), 1).
					WillReturnRows(sqlmock.NewRows([]string{"id", "dict_type_id", "label"}).AddRow(uint(11), uint(1), "男"))
				mock.ExpectBegin()
				mock.ExpectExec(`UPDATE "dict_items" SET`).
					WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			},
			write: func(t *testing.T, svc *DictService, ctx context.Context) {
				if _, err := svc.UpdateItemContext(ctx, 11, UpdateDictItemRequest{Label: "男性"}); err != nil {
					t.Fatalf("UpdateItemContext(): %v", err)
				}
			},
		},
		{
			name: "delete item",
			expectWrite: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "dict_items" WHERE "dict_items"."id" = $1`)).
					WithArgs(uint(11)).
					WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			},
			write: func(t *testing.T, svc *DictService, ctx context.Context) {
				if err := svc.DeleteItemContext(ctx, 11); err != nil {
					t.Fatalf("DeleteItemContext(): %v", err)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, mock, store := setupDictCacheTestService(t)
			ctx := context.Background()

			// Warm the cache, then perform the write, then read again: the
			// second read must re-query, which is why a second query pair is
			// expected below.
			expectDictCodeQueries(mock)
			if _, err := (&svc).GetMultipleDictDataContext(ctx, []string{"gender"}); err != nil {
				t.Fatalf("warm read: %v", err)
			}
			if len(store.Keys()) == 0 {
				t.Fatal("warm read wrote nothing to the cache")
			}

			tc.expectWrite(mock)
			tc.write(t, &svc, ctx)

			for _, key := range store.Keys() {
				if key == cachepkg.KeyDictIndex {
					t.Fatalf("dict cache index survived %s", tc.name)
				}
			}
			if store.Exists(dictCacheKeyForTest(1, "gender")) {
				t.Fatalf("cached gender entry survived %s", tc.name)
			}

			expectDictCodeQueries(mock)
			again, err := (&svc).GetMultipleDictDataContext(ctx, []string{"gender"})
			if err != nil {
				t.Fatalf("post-write read: %v", err)
			}
			if len(again["gender"]) != 1 {
				t.Fatalf("post-write read = %+v, want a fresh database result", again)
			}
		})
	}
}

func dictCacheKeyForTest(tenantID uint, code string) string {
	return fmt.Sprintf(cachepkg.KeyDictData, tenantID, code)
}
