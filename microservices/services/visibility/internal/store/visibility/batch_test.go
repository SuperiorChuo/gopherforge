package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/go-admin-kit/services/visibility/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type batchCase struct {
	table string
	call  func(*Store, context.Context, uint64) (int, int, error)
}

func batchCases() []batchCase {
	return []batchCase{
		{"vis_brands", func(s *Store, c context.Context, t uint64) (int, int, error) {
			return s.BatchCreateBrands(c, t, []model.Brand{{Name: "Alpha"}, {Name: "Beta"}})
		}},
		{"vis_keywords", func(s *Store, c context.Context, t uint64) (int, int, error) {
			return s.BatchCreateKeywords(c, t, []model.Keyword{{Keyword: "Alpha"}, {Keyword: "Beta"}})
		}},
		{"vis_geo_prompts", func(s *Store, c context.Context, t uint64) (int, int, error) {
			return s.BatchCreateGeoPrompts(c, t, []model.GeoPrompt{{Question: "Alpha"}, {Question: "Beta"}})
		}},
	}
}
func batchStore(t *testing.T) *Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "batch.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	st, err := NewWithDB(db)
	if err != nil {
		t.Fatal(err)
	}
	return st
}
func assertRows(t *testing.T, st *Store, table string, want int64) {
	t.Helper()
	var n int64
	if err := st.db.Table(table).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != want {
		t.Fatalf("partial batch persisted: table=%s rows=%d want=%d", table, n, want)
	}
}
func assertBatchWriteRollback(t *testing.T, st *Store, tc batchCase, ctx context.Context) {
	t.Helper()
	hits := 0
	sentinel := errors.New("injected second batch row failure")
	if err := st.db.Callback().Create().Before("gorm:create").Register("test:fail_second", func(tx *gorm.DB) {
		if tx.Statement.Table == tc.table {
			hits++
			if hits == 2 {
				tx.AddError(sentinel)
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := st.db.Callback().Create().Remove("test:fail_second"); err != nil {
			t.Error(err)
		}
	}()
	n, k, err := tc.call(st, ctx, 2)
	if !errors.Is(err, sentinel) || hits != 2 || n != 0 || k != 0 {
		t.Fatalf("batch failure not exercised: hits=%d created=%d skipped=%d err=%v", hits, n, k, err)
	}
	assertRows(t, st, tc.table, 0)
}
func TestBatchStorageFailuresAndCancellation(t *testing.T) {
	for _, tc := range batchCases() {
		for _, stage := range []string{"write failure", "read failure", "read cancel", "second write cancel", "already canceled"} {
			t.Run(tc.table+"/"+stage, func(t *testing.T) {
				st := batchStore(t)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if stage == "write failure" {
					assertBatchWriteRollback(t, st, tc, ctx)
					return
				}
				hits := 0
				var queryErr error
				sentinel := errors.New("dedupe lookup unavailable")
				if err := st.db.Callback().Query().Before("gorm:query").Register("test:read", func(tx *gorm.DB) {
					if tx.Statement.Table == tc.table {
						if stage == "read failure" {
							hits++
							tx.AddError(sentinel)
						}
						if stage == "read cancel" {
							hits++
							cancel()
						}
					}
				}); err != nil {
					t.Fatal(err)
				}
				if err := st.db.Callback().Query().After("gorm:query").Register("test:query_error", func(tx *gorm.DB) {
					if tx.Statement.Table == tc.table {
						queryErr = tx.Error
					}
				}); err != nil {
					t.Fatal(err)
				}
				if err := st.db.Callback().Create().Before("gorm:create").Register("test:cancel_write", func(tx *gorm.DB) {
					if tx.Statement.Table == tc.table && stage == "second write cancel" {
						hits++
						if hits == 2 {
							cancel()
						}
					}
				}); err != nil {
					t.Fatal(err)
				}
				want := error(context.Canceled)
				wantHits := 1
				if stage == "read failure" {
					want = sentinel
				}
				if stage == "second write cancel" {
					wantHits = 2
				}
				if stage == "already canceled" {
					cancel()
					wantHits = 0
				}
				n, k, err := tc.call(st, ctx, 2)
				if !errors.Is(err, want) || hits != wantHits || n != 0 || k != 0 {
					t.Fatalf("stage=%s hits=%d created=%d skipped=%d err=%v", stage, hits, n, k, err)
				}
				if stage == "read cancel" && !errors.Is(queryErr, context.Canceled) {
					t.Fatalf("dedupe query ignored cancellation: %v", queryErr)
				}
				if err := st.db.Callback().Query().Remove("test:read"); err != nil {
					t.Fatal(err)
				}
				assertRows(t, st, tc.table, 0)
			})
		}
	}
}

func TestTaskUpdatesRejectMissingOrForeignRows(t *testing.T) {
	st := batchStore(t)
	run := &model.GeoRun{TenantID: 1, Status: "running"}
	if err := st.CreateGeoRun(run); err != nil {
		t.Fatal(err)
	}
	task := &model.SerpTask{TenantID: 1, Status: "running"}
	if err := st.CreateSerpTask(task); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		id     uint64
		update func(uint64, uint64, map[string]any) error
	}{{"geo", run.ID, st.UpdateGeoRun}, {"serp", task.ID, st.UpdateSerpTask}} {
		for _, key := range [][2]uint64{{2, tc.id}, {1, 99999}} {
			if err := tc.update(key[0], key[1], map[string]any{"status": "done"}); !errors.Is(err, gorm.ErrRecordNotFound) {
				t.Errorf("missing %s task falsely updated: %v", tc.name, err)
			}
		}
		if err := tc.update(1, tc.id, map[string]any{"status": "done"}); err != nil {
			t.Fatal(err)
		}
	}
}
