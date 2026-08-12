package outbox

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:outbox-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("sqlite: %v", err)
	}
	if err := EnsureTable(db); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	return db
}

func TestInsertAndProcessSuccess(t *testing.T) {
	db := openDB(t)
	err := db.Transaction(func(tx *gorm.DB) error {
		return Insert(tx, 1, "audit.log.create", []byte(`{"id":1}`))
	})
	if err != nil {
		t.Fatal(err)
	}
	var published atomic.Int32
	pub := PublisherFunc(func(ctx context.Context, subject string, payload []byte) error {
		if subject != "audit.log.create" {
			t.Errorf("subject=%s", subject)
		}
		published.Add(1)
		return nil
	})
	if err := ProcessOnce(context.Background(), db, pub, Options{BatchSize: 10}); err != nil {
		t.Fatal(err)
	}
	if published.Load() != 1 {
		t.Fatalf("published=%d", published.Load())
	}
	n, err := CountByStatus(db, StatusSent)
	if err != nil || n != 1 {
		t.Fatalf("sent count=%d err=%v", n, err)
	}
}

func TestProcessRetryThenFail(t *testing.T) {
	db := openDB(t)
	_ = db.Transaction(func(tx *gorm.DB) error {
		return Insert(tx, 1, "audit.log.x", []byte(`x`))
	})
	pub := PublisherFunc(func(ctx context.Context, subject string, payload []byte) error {
		return errors.New("nats down")
	})
	// MaxAttempts=2：第一次 claim attempts=1 → pending；第二次 attempts=2 → failed
	for i := 0; i < 2; i++ {
		// 确保 next_at 已到期
		_ = db.Table(tableRef(db)).Where("status = ?", StatusPending).
			Update("next_at", time.Now().Add(-time.Second)).Error
		if err := ProcessOnce(context.Background(), db, pub, Options{MaxAttempts: 2, BatchSize: 5}); err != nil {
			t.Fatal(err)
		}
	}
	nFailed, _ := CountByStatus(db, StatusFailed)
	nPending, _ := CountByStatus(db, StatusPending)
	if nFailed != 1 {
		t.Fatalf("failed=%d pending=%d, want failed=1", nFailed, nPending)
	}
}

func TestInsertValidation(t *testing.T) {
	db := openDB(t)
	if err := Insert(db, 1, "", []byte("x")); err == nil {
		t.Fatal("want subject error")
	}
	if err := Insert(db, 1, "s", nil); err == nil {
		t.Fatal("want payload error")
	}
}

func TestTransactionalFlag(t *testing.T) {
	DisableTransactional()
	if TransactionalEnabled() {
		t.Fatal("want disabled")
	}
	EnableTransactional()
	if !TransactionalEnabled() {
		t.Fatal("want enabled")
	}
	DisableTransactional()
}

func TestWorkerStop(t *testing.T) {
	db := openDB(t)
	_ = db.Transaction(func(tx *gorm.DB) error {
		return Insert(tx, 1, "s", []byte("p"))
	})
	var n atomic.Int32
	stop := StartWorker(context.Background(), db, PublisherFunc(func(ctx context.Context, subject string, payload []byte) error {
		n.Add(1)
		return nil
	}), Options{PollInterval: 50 * time.Millisecond, BatchSize: 5, Logger: func(string, ...any) {}})
	time.Sleep(200 * time.Millisecond)
	stop()
	if n.Load() < 1 {
		t.Fatalf("worker published %d, want >=1", n.Load())
	}
}
