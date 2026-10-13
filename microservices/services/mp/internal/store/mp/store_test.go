package store

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type legacyMenu struct {
	ID        uint64 `gorm:"primaryKey"`
	TenantID  uint64
	AccountID uint64
	Buttons   string
	Published bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (legacyMenu) TableName() string { return "mp_menus" }

type legacyMessage struct {
	ID        uint64 `gorm:"primaryKey"`
	TenantID  uint64 `gorm:"not null;default:1"`
	AccountID uint64 `gorm:"not null"`
	OpenID    string
	Direction string `gorm:"not null"`
	MsgType   string `gorm:"not null"`
	Content   string
	MsgID     string
	Event     string
	RawBody   string
	CreatedAt time.Time
}

func (legacyMessage) TableName() string { return "mp_messages" }

func TestNewWithDBDeduplicatesLegacyMenusBeforeUniqueIndex(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:legacy-menu?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&legacyMenu{}); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	older := time.Now().Add(-time.Hour)
	newer := time.Now()
	rows := []legacyMenu{
		{TenantID: 1, AccountID: 9, Buttons: `[{"name":"旧"}]`, UpdatedAt: older},
		{TenantID: 1, AccountID: 9, Buttons: `[{"name":"新"}]`, UpdatedAt: newer},
		{TenantID: 2, AccountID: 9, Buttons: `[{"name":"其他租户"}]`, UpdatedAt: newer},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("seed legacy rows: %v", err)
	}

	st, err := NewWithDB(db)
	if err != nil {
		t.Fatalf("migrate store: %v", err)
	}
	menu, err := st.GetMenu(1, 9)
	if err != nil {
		t.Fatalf("get retained menu: %v", err)
	}
	if menu.Buttons != `[{"name":"新"}]` {
		t.Fatalf("retained buttons = %s", menu.Buttons)
	}
	var count int64
	if err := db.Model(&MpMenu{}).Where("account_id = ?", 9).Count(&count).Error; err != nil {
		t.Fatalf("count migrated rows: %v", err)
	}
	if count != 2 {
		t.Fatalf("migrated rows = %d, want one per tenant", count)
	}
	if err := db.Create(&MpMenu{TenantID: 1, AccountID: 9, Buttons: "[]"}).Error; err == nil {
		t.Fatal("expected tenant/account unique index to reject duplicate")
	}
}

func TestMarkMenuPublishedUsesDraftCompareAndSwap(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:menu-cas?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	st, err := NewWithDB(db)
	if err != nil {
		t.Fatalf("migrate store: %v", err)
	}
	if err := st.UpsertMenu(1, 7, `[{"name":"草稿B"}]`, false); err != nil {
		t.Fatalf("seed draft: %v", err)
	}
	updated, err := st.MarkMenuPublished(1, 7, `[{"name":"草稿A"}]`, `[{"name":"草稿A"}]`)
	if err != nil {
		t.Fatalf("mark stale draft: %v", err)
	}
	if updated {
		t.Fatal("stale draft must not be marked published")
	}
	menu, err := st.GetMenu(1, 7)
	if err != nil || menu.Buttons != `[{"name":"草稿B"}]` || menu.Published {
		t.Fatalf("draft after stale publish = %+v err=%v", menu, err)
	}
	updated, err = st.MarkMenuPublished(1, 7, menu.Buttons, `[{"name":"规范草稿B"}]`)
	if err != nil || !updated {
		t.Fatalf("mark current draft updated=%v err=%v", updated, err)
	}
	menu, err = st.GetMenu(1, 7)
	if err != nil || menu.Buttons != `[{"name":"规范草稿B"}]` || !menu.Published {
		t.Fatalf("normalized published draft = %+v err=%v", menu, err)
	}
}

func TestNewWithDBDeduplicatesInboundMessagesBeforeUniqueIndex(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&legacyMessage{}); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	rows := []legacyMessage{
		{TenantID: 1, AccountID: 7, Direction: "in", MsgType: "text", MsgID: "duplicate-1"},
		{TenantID: 1, AccountID: 7, Direction: "in", MsgType: "text", MsgID: "duplicate-1"},
		{TenantID: 2, AccountID: 7, Direction: "in", MsgType: "text", MsgID: "duplicate-1"},
		{TenantID: 1, AccountID: 7, Direction: "out", MsgType: "text", MsgID: "duplicate-1"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("seed legacy rows: %v", err)
	}

	st, err := NewWithDB(db)
	if err != nil {
		t.Fatalf("migrate store: %v", err)
	}
	var inboundCount int64
	if err := db.Model(&MpMessage{}).
		Where("tenant_id = ? AND account_id = ? AND direction = ? AND msg_id = ?", 1, 7, "in", "duplicate-1").
		Count(&inboundCount).Error; err != nil {
		t.Fatalf("count retained inbound rows: %v", err)
	}
	if inboundCount != 1 {
		t.Fatalf("retained inbound rows=%d, want 1", inboundCount)
	}
	inserted, err := st.ArchiveInboundMessage(&MpMessage{
		TenantID: 1, AccountID: 7, MsgID: "duplicate-1", MsgType: "text",
	})
	if err != nil {
		t.Fatalf("archive duplicate: %v", err)
	}
	if inserted {
		t.Fatal("duplicate inbound message must not insert")
	}
	if err := st.LogMessage(&MpMessage{
		TenantID: 1, AccountID: 7, Direction: "out", MsgID: "", MsgType: "text",
	}); err != nil {
		t.Fatalf("archive first outbound: %v", err)
	}
	if err := st.LogMessage(&MpMessage{
		TenantID: 1, AccountID: 7, Direction: "out", MsgID: "", MsgType: "text",
	}); err != nil {
		t.Fatalf("archive second outbound: %v", err)
	}
}

func TestPublishResultJobLeaseRetryAndRevive(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	st, err := NewWithDB(db)
	if err != nil {
		t.Fatalf("migrate store: %v", err)
	}
	job, err := st.UpsertPublishResultJob(&MpPublishResultJob{
		TenantID: 1, AccountID: 8, PublishID: "publish-1", PublishStatus: "0",
	})
	if err != nil {
		t.Fatalf("upsert job: %v", err)
	}
	claimed, err := st.ClaimPublishResultJobs(10, time.Minute)
	if err != nil || len(claimed) != 1 || claimed[0].ID != job.ID {
		t.Fatalf("first claim=%+v err=%v", claimed, err)
	}
	claimedAgain, err := st.ClaimPublishResultJobs(10, time.Minute)
	if err != nil || len(claimedAgain) != 0 {
		t.Fatalf("leased claim=%+v err=%v, want empty", claimedAgain, err)
	}
	if err := st.RetryPublishResultJob(job.ID, 2, 2, "not found"); err != nil {
		t.Fatalf("exhaust job: %v", err)
	}
	var dead MpPublishResultJob
	if err := db.First(&dead, job.ID).Error; err != nil {
		t.Fatalf("load dead job: %v", err)
	}
	if dead.Status != "dead" || dead.Attempts != 2 {
		t.Fatalf("dead job=%+v", dead)
	}
	revived, err := st.UpsertPublishResultJob(&MpPublishResultJob{
		TenantID: 1, AccountID: 8, PublishID: "publish-1", PublishStatus: "2",
	})
	if err != nil {
		t.Fatalf("revive job: %v", err)
	}
	if revived.ID != job.ID || revived.Status != "pending" || revived.Attempts != 0 || revived.PublishStatus != "2" {
		t.Fatalf("revived job=%+v", revived)
	}
}
