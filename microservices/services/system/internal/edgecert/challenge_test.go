package edgecert

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestChallengeStoreIsDurableAcrossInstancesAndHonorsTTL(t *testing.T) {
	db := openEdgeCertTestDB(t)
	ctx := context.Background()
	// LookupChallenge 用墙钟判断过期，Put 的 expiresAt 必须相对真实 now 仍有效；
	// Now 钩子只固定 CreatedAt，不能把 expires 钉死在过去的墙钟时间点。
	frozenCreated := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	writer := DBChallengeStore{DB: db, Now: func() time.Time { return frozenCreated }}
	expiresAt := time.Now().UTC().Add(2 * time.Minute)
	if err := writer.Put(ctx, 42, "token-one", "token-one.authorization", expiresAt); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	// A separately constructed store/router instance sees the same durable row.
	value, ok, err := LookupChallenge(ctx, db.Session(&gorm.Session{NewDB: true}), "token-one")
	if err != nil || !ok || value != "token-one.authorization" {
		t.Fatalf("LookupChallenge() = %q/%v/%v", value, ok, err)
	}

	if err := db.Model(&Challenge{}).Where("token = ?", "token-one").Update("expires_at", time.Now().UTC().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	if value, ok, err = LookupChallenge(ctx, db, "token-one"); err != nil || ok || value != "" {
		t.Fatalf("expired LookupChallenge() = %q/%v/%v", value, ok, err)
	}
	var count int64
	if err := db.Model(&Challenge{}).Where("token = ?", "token-one").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("expired challenge not cleaned: count=%d err=%v", count, err)
	}
}

func TestChallengeStoreRejectsPathTokens(t *testing.T) {
	db := openEdgeCertTestDB(t)
	err := (DBChallengeStore{DB: db}).Put(context.Background(), 1, "../escape", "auth", time.Now().Add(time.Minute))
	if err == nil {
		t.Fatal("Put() accepted a path-like challenge token")
	}
}

func openEdgeCertTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "edgecert.sqlite")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(&Certificate{}, &Task{}, &Challenge{}); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	return db
}
