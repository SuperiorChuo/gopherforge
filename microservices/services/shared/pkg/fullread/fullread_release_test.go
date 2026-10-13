//go:build release_integration

package fullread_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/go-admin-kit/services/internal/testdb"
	"github.com/go-admin-kit/services/shared/pkg/fullread"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
)

type snapshotRecord struct {
	ID        uint64 `gorm:"primaryKey"`
	TenantID  uint
	Sort      *int
	Name      string
	Data      map[string]string `gorm:"serializer:json;type:json"`
	DeletedAt gorm.DeletedAt
	Hooked    bool `gorm:"-"`
}

func (r *snapshotRecord) AfterFind(*gorm.DB) error { r.Hooked = true; return nil }

func TestReleaseFullreadRealBatchBoundaries(t *testing.T) {
	if os.Getenv("GAK_FULLREAD_TEST_DSN") != testdb.IsolatedDSN {
		t.Fatal("requires the explicit isolated fullread database harness")
	}
	for _, count := range []int{0, 1, 255, 256, 257, 512, 513} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			db := testdb.Open(t, &snapshotRecord{})
			rows := make([]snapshotRecord, count)
			for i := range rows {
				rows[i] = snapshotRecord{Name: fmt.Sprint(i)}
			}
			if count > 0 {
				if err := db.CreateInBatches(&rows, fullread.BatchSize).Error; err != nil {
					t.Fatal(err)
				}
			}
			fetches := 0
			if err := db.Callback().Query().After("gorm:after_query").Register("test:real_boundaries", func(q *gorm.DB) {
				if strings.HasPrefix(q.Statement.SQL.String(), "FETCH FORWARD ") {
					fetches++
					if q.RowsAffected > fullread.BatchSize {
						t.Errorf("oversized fetch=%d", q.RowsAffected)
					}
				}
			}); err != nil {
				t.Fatal(err)
			}
			got, err := fullread.All[*snapshotRecord](db.Order("id DESC"))
			if err != nil || got == nil || len(got) != count || fetches != count/fullread.BatchSize+1 {
				t.Fatalf("rows=%d fetches=%d err=%v", len(got), fetches, err)
			}
			for i, row := range got {
				if row.ID != uint64(count-i) || !row.Hooked {
					t.Fatalf("row %d: %+v", i, row)
				}
			}
		})
	}
}

func TestReleaseFullreadSnapshotAndCursorLifecycle(t *testing.T) {
	if os.Getenv("GAK_FULLREAD_TEST_DSN") != testdb.IsolatedDSN {
		t.Fatal("requires the explicit isolated fullread database harness")
	}
	db := testdb.Open(t, &snapshotRecord{})
	if err := tenant.Register(db); err != nil {
		t.Fatal(err)
	}
	ctx := tenant.WithContext(context.Background(), 7)
	const count = fullread.BatchSize*2 + 1
	rows := make([]snapshotRecord, 0, count)
	for i := 1; i <= count; i++ {
		sort := (count - i) % 7
		r := snapshotRecord{ID: uint64(i), TenantID: 7, Sort: &sort, Name: fmt.Sprintf("bound ' %d", i), Data: map[string]string{"value": "original"}}
		if i%5 == 0 {
			r.Sort = nil
		}
		rows = append(rows, r)
	}
	if err := db.CreateInBatches(&rows, fullread.BatchSize).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&snapshotRecord{ID: 9000, TenantID: 8, Name: "bound ' other"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&snapshotRecord{ID: 9001, TenantID: 7, Name: "bound ' deleted"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&snapshotRecord{}, 9001).Error; err != nil {
		t.Fatal(err)
	}
	query := db.WithContext(ctx).Table("snapshot_records AS r").Select("r.*").Where("r.name LIKE ?", "bound ' %").Order("r.sort ASC NULLS FIRST, r.id DESC")
	var expected []snapshotRecord
	if err := query.Session(&gorm.Session{}).Find(&expected).Error; err != nil {
		t.Fatal(err)
	}
	if len(expected) != count {
		t.Fatalf("reference count=%d", len(expected))
	}
	for _, row := range expected[:fullread.BatchSize] {
		if row.ID == 4 || row.ID == 6 {
			t.Fatal("mutation fixture must target unfetched rows")
		}
	}
	changed := false
	if err := db.Callback().Query().After("gorm:after_query").Register("test:mutate_after_first_fetch", func(q *gorm.DB) {
		if q.DryRun || q.Error != nil || changed || !strings.HasPrefix(q.Statement.SQL.String(), "FETCH FORWARD ") {
			return
		}
		changed = true
		// A separate connection commits changes to still-unfetched rows.
		for _, sql := range []string{
			"UPDATE snapshot_records SET name='changed', sort=-100 WHERE id=4",
			"DELETE FROM snapshot_records WHERE id=6",
			"INSERT INTO snapshot_records (id,tenant_id,name) VALUES (9999,7,'bound '' inserted')",
		} {
			if err := db.Exec(sql).Error; err != nil {
				q.AddError(err)
				return
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	got, err := fullread.All[snapshotRecord](query)
	db.Callback().Query().Remove("test:mutate_after_first_fetch")
	if err != nil || !changed || !reflect.DeepEqual(got, expected) {
		t.Fatalf("snapshot changed or decoding/hooks lost: got=%d want=%d changed=%t err=%v", len(got), len(expected), changed, err)
	}
	for _, r := range got {
		if !r.Hooked || r.TenantID != 7 || r.Data["value"] != "original" {
			t.Fatalf("decoded row=%+v", r)
		}
	}
	// Reusing the caller query sees the newly committed state, not a closed tx.
	var later []snapshotRecord
	if err := query.Find(&later).Error; err != nil || reflect.DeepEqual(later, expected) {
		t.Fatalf("query reuse: %v", err)
	}

	rollback := errors.New("test-only outer rollback")
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&snapshotRecord{ID: 10000, TenantID: 7, Name: "uncommitted"}).Error; err != nil {
			return err
		}
		inside, err := fullread.All[snapshotRecord](tx.Where("id = ?", 10000))
		if err != nil || len(inside) != 1 {
			return fmt.Errorf("nested transaction lost own writes: %d %v", len(inside), err)
		}
		var cursors int64
		if err := tx.Raw("SELECT count(*) FROM pg_cursors WHERE name LIKE 'gak_fullread_%'").Scan(&cursors).Error; err != nil || cursors != 0 {
			return fmt.Errorf("leaked cursors: %d %v", cursors, err)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	var persisted int64
	if err := db.Model(&snapshotRecord{}).Where("id = ?", 10000).Count(&persisted).Error; err != nil || persisted != 0 {
		t.Fatalf("nested read committed outer transaction: %d %v", persisted, err)
	}

	// Force a real PostgreSQL error on the second FETCH, after a full first page.
	fetches := 0
	if err := db.Callback().Query().Before("gorm:query").Register("test:break_second_fetch", func(q *gorm.DB) {
		if strings.HasPrefix(q.Statement.SQL.String(), "FETCH FORWARD ") {
			fetches++
			if fetches == 2 {
				q.Statement.SQL.Reset()
				q.Statement.SQL.WriteString("FETCH FORWARD 256 FROM test_missing_cursor")
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	got, err = fullread.All[snapshotRecord](db.WithContext(ctx).Order("sort ASC NULLS FIRST"))
	db.Callback().Query().Remove("test:break_second_fetch")
	if err == nil || got != nil || fetches != 2 {
		t.Fatalf("real fetch failure returned partial set: %d %d %v", len(got), fetches, err)
	}
	pool, _ := db.DB()
	if n := pool.Stats().InUse; n != 0 {
		t.Fatalf("connections still in use: %d", n)
	}
	// Two concurrent cursors on independent transactions have independent names.
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			rows, err := fullread.All[snapshotRecord](db.WithContext(ctx).Order("sort ASC NULLS FIRST"))
			if err == nil && len(rows) != count {
				err = fmt.Errorf("concurrent rows=%d", len(rows))
			}
			errs <- err
		}()
	}
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if rows, err := fullread.All[snapshotRecord](db.WithContext(cancelled)); err == nil || rows != nil {
		t.Fatalf("cancelled query=%d %v", len(rows), err)
	}
	t.Log("PASS: original snapshot with concurrent commits, nullable/tied sorting, bound parameters, tenant/soft-delete filters, JSON/hooks, nested transaction ownership, real late-fetch error, cancellation and connection release")
}
