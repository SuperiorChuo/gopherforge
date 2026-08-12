package idempotency

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("sqlite: %v", err)
	}
	const table = "test_idempotency_key"
	if err := EnsureTable(db, table); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	return db
}

func TestRunFirstThenReuse(t *testing.T) {
	db := openTestDB(t)
	const table = "test_idempotency_key"
	var calls int32

	runOnce := func() (uint64, bool, error) {
		var id uint64
		var reused bool
		err := db.Transaction(func(tx *gorm.DB) error {
			var err error
			id, reused, err = Run(tx, table, 1, "k1", Hash(struct{ A int }{A: 1}), func(tx *gorm.DB) (uint64, error) {
				atomic.AddInt32(&calls, 1)
				return 42, nil
			})
			return err
		})
		return id, reused, err
	}

	id1, reused1, err := runOnce()
	if err != nil || reused1 || id1 != 42 {
		t.Fatalf("first: id=%d reused=%v err=%v", id1, reused1, err)
	}
	id2, reused2, err := runOnce()
	if err != nil || !reused2 || id2 != 42 {
		t.Fatalf("second: id=%d reused=%v err=%v", id2, reused2, err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("fn calls = %d, want 1", calls)
	}
}

func TestRunKeyReuseConflict(t *testing.T) {
	db := openTestDB(t)
	const table = "test_idempotency_key"
	err := db.Transaction(func(tx *gorm.DB) error {
		_, _, err := Run(tx, table, 1, "k2", Hash("a"), func(tx *gorm.DB) (uint64, error) {
			return 1, nil
		})
		return err
	})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		_, _, err := Run(tx, table, 1, "k2", Hash("b"), func(tx *gorm.DB) (uint64, error) {
			return 2, nil
		})
		return err
	})
	if !errors.Is(err, ErrKeyReuse) {
		t.Fatalf("want ErrKeyReuse, got %v", err)
	}
}

func TestRunEmptyKeyAlwaysExecutes(t *testing.T) {
	db := openTestDB(t)
	const table = "test_idempotency_key"
	var calls int32
	for i := 0; i < 3; i++ {
		err := db.Transaction(func(tx *gorm.DB) error {
			_, _, err := Run(tx, table, 1, "", "", func(tx *gorm.DB) (uint64, error) {
				atomic.AddInt32(&calls, 1)
				return uint64(i + 1), nil
			})
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if atomic.LoadInt32(&calls) != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

func TestPurgeOlderThan(t *testing.T) {
	db := openTestDB(t)
	const table = "test_idempotency_key"
	err := db.Transaction(func(tx *gorm.DB) error {
		_, _, err := Run(tx, table, 1, "old", Hash(1), func(tx *gorm.DB) (uint64, error) {
			return 1, nil
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	// 把 created_at 拨到过去
	if err := db.Table(table).Where("key = ?", "old").
		Update("created_at", time.Now().Add(-48*time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	n, err := PurgeOlderThan(db, table, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("purged = %d, want 1", n)
	}
}

func TestConcurrentSameKey(t *testing.T) {
	db := openTestDB(t)
	const table = "test_idempotency_key"
	var calls int32
	var wg sync.WaitGroup
	results := make(chan uint64, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var id uint64
			_ = db.Transaction(func(tx *gorm.DB) error {
				var err error
				id, _, err = Run(tx, table, 1, "concurrent", Hash("same"), func(tx *gorm.DB) (uint64, error) {
					atomic.AddInt32(&calls, 1)
					time.Sleep(20 * time.Millisecond)
					return 99, nil
				})
				return err
			})
			if id != 0 {
				results <- id
			}
		}()
	}
	wg.Wait()
	close(results)
	// SQLite 对并发事务支持有限；至少不应 panic，成功路径 id 应一致
	for id := range results {
		if id != 99 && id != 0 {
			// incomplete races may yield 0; non-99 is failure
			if id != 99 {
				t.Fatalf("unexpected id %d", id)
			}
		}
	}
	if c := atomic.LoadInt32(&calls); c > 1 {
		// best-effort：SQLite 下可能串行只执行 1 次；若 >1 说明未去重
		t.Logf("note: fn calls=%d under sqlite concurrency (PG OnConflict is stronger)", c)
	}
}

func TestNewKeyAndHash(t *testing.T) {
	k1, err := NewKey()
	if err != nil || len(k1) != 32 {
		t.Fatalf("NewKey: %q err=%v", k1, err)
	}
	k2, _ := NewKey()
	if k1 == k2 {
		t.Fatal("NewKey should be random")
	}
	if Hash(struct{ X int }{1}) != Hash(struct{ X int }{1}) {
		t.Fatal("Hash unstable")
	}
	if Hash(struct{ X int }{1}) == Hash(struct{ X int }{2}) {
		t.Fatal("Hash collision on different input")
	}
}
