package store

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-admin-kit/services/im/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var retDBSeq atomic.Int64

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dsn := fmt.Sprintf("file:rettest%d?mode=memory&cache=shared", retDBSeq.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	s, err := NewWithDB(db)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func closedConversation(t *testing.T, s *Store, closedAgo time.Duration) *model.Conversation {
	t.Helper()
	v, err := s.UpsertVisitor(1, fmt.Sprintf("ret-guest-%d", retDBSeq.Add(1)), "")
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateConversation(1, 1, v.ID, "h5", "{}", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateMessage(&model.Message{
		ConversationID: c.ID, SenderType: "visitor", MsgType: "text",
		Content: `{"text":"hi"}`,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateMessage(&model.Message{
		ConversationID: c.ID, SenderType: "visitor", MsgType: "file",
		Content: `{"url":"/im/uploads/202607/ret-obj.txt","name":"a.txt"}`,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.CloseConversation(c, CloseOpts{Reason: "test"}); err != nil {
		t.Fatal(err)
	}
	closedAt := time.Now().Add(-closedAgo)
	if err := s.db.Model(c).Update("closed_at", closedAt).Error; err != nil {
		t.Fatal(err)
	}
	return c
}

func TestPurgeExpired(t *testing.T) {
	s := newTestStore(t)
	old := closedConversation(t, s, 200*24*time.Hour)  // beyond retention
	fresh := closedConversation(t, s, 10*24*time.Hour) // within retention
	openConv := func() *model.Conversation {           // open, must never purge
		v, _ := s.UpsertVisitor(1, "ret-open", "")
		c, _ := s.CreateConversation(1, 1, v.ID, "h5", "{}", nil, false)
		return c
	}()

	res, err := s.PurgeExpired(180*24*time.Hour, 500)
	if err != nil {
		t.Fatal(err)
	}
	if res.Conversations != 1 {
		t.Fatalf("purged %d conversations, want 1", res.Conversations)
	}
	if res.Messages == 0 {
		t.Fatal("no messages purged")
	}
	if len(res.AttachmentKeys) != 1 || res.AttachmentKeys[0] != "202607/ret-obj.txt" {
		t.Fatalf("attachment keys %v", res.AttachmentKeys)
	}

	if _, err := s.GetConversationByPublicID(old.PublicID.String()); err == nil {
		t.Fatal("expired conversation still present")
	}
	if _, err := s.GetConversationByPublicID(fresh.PublicID.String()); err != nil {
		t.Fatal("fresh closed conversation was purged")
	}
	if _, err := s.GetConversationByPublicID(openConv.PublicID.String()); err != nil {
		t.Fatal("open conversation was purged")
	}
	var n int64
	s.db.Model(&model.Message{}).Where("conversation_id = ?", old.ID).Count(&n)
	if n != 0 {
		t.Fatalf("%d orphan messages left", n)
	}
}

func TestPurgeDisabledAndIdempotent(t *testing.T) {
	s := newTestStore(t)
	closedConversation(t, s, 200*24*time.Hour)

	// retention 0 → no-op
	res, err := s.PurgeExpired(0, 500)
	if err != nil || res.Conversations != 0 {
		t.Fatalf("disabled purge acted: %+v %v", res, err)
	}

	// first sweep purges, second finds nothing
	if res, _ = s.PurgeExpired(180*24*time.Hour, 500); res.Conversations != 1 {
		t.Fatalf("first sweep %d", res.Conversations)
	}
	if res, _ = s.PurgeExpired(180*24*time.Hour, 500); res.Conversations != 0 {
		t.Fatalf("second sweep purged again: %d", res.Conversations)
	}
}

func TestPurgeBatches(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 5; i++ {
		closedConversation(t, s, 200*24*time.Hour)
	}
	// batch=2 must loop until done
	res, err := s.PurgeExpired(180*24*time.Hour, 2)
	if err != nil {
		t.Fatal(err)
	}
	if res.Conversations != 5 {
		t.Fatalf("purged %d, want 5", res.Conversations)
	}
	if len(res.AttachmentKeys) != 5 {
		t.Fatalf("attachment keys %d, want 5", len(res.AttachmentKeys))
	}
}
