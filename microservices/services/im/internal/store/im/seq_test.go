package store

import (
	"fmt"
	"sync"
	"testing"

	"github.com/go-admin-kit/services/im/internal/model"
)

// newConversation 建一条可写消息的会话。
func newConversation(t *testing.T, s *Store) *model.Conversation {
	t.Helper()
	v, err := s.UpsertVisitor(1, fmt.Sprintf("seq-guest-%d", retDBSeq.Add(1)), "")
	if err != nil {
		t.Fatal(err)
	}
	conv, err := s.EnsureOpenConversation(1, 1, v.ID, "h5", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	return conv
}

func sendText(t *testing.T, s *Store, convID uint64, body string) *model.Message {
	t.Helper()
	m := &model.Message{
		ConversationID: convID,
		SenderType:     "visitor",
		MsgType:        "text",
		Content:        JSONText(map[string]any{"text": body}),
	}
	if err := s.CreateMessage(m); err != nil {
		t.Fatal(err)
	}
	return m
}

// seq 必须从 1 开始连续递增，且写进会话计数器。
func TestCreateMessageSeqIsContiguous(t *testing.T) {
	s := newTestStore(t)
	conv := newConversation(t, s)

	for i := 1; i <= 20; i++ {
		m := sendText(t, s, conv.ID, fmt.Sprintf("m%d", i))
		if m.Seq != int64(i) {
			t.Fatalf("message %d: seq = %d, want %d", i, m.Seq, i)
		}
	}
	var got model.Conversation
	if err := s.db.First(&got, conv.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.LastSeq != 20 {
		t.Fatalf("conversation last_seq = %d, want 20", got.LastSeq)
	}
	if got.LastMessagePreview != "m20" {
		t.Fatalf("last_message_preview = %q, want %q", got.LastMessagePreview, "m20")
	}
	if got.LastMessageAt == nil {
		t.Fatal("last_message_at not set")
	}
}

// 并发写同一会话：seq 不能重号、不能跳号——客户端按 seq > cursor 拉取，
// 重号会丢消息，跳号会让增量拉取出现永久空洞。
func TestCreateMessageSeqNoDupNoGapUnderConcurrency(t *testing.T) {
	s := newTestStore(t)
	conv := newConversation(t, s)

	const writers, perWriter = 8, 15
	const total = writers * perWriter

	var wg sync.WaitGroup
	var mu sync.Mutex
	seqs := make([]int64, 0, total)
	errs := make([]error, 0)

	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				m := &model.Message{
					ConversationID: conv.ID,
					SenderType:     "visitor",
					MsgType:        "text",
					Content:        JSONText(map[string]any{"text": fmt.Sprintf("w%d-%d", w, i)}),
				}
				err := s.CreateMessage(m)
				mu.Lock()
				if err != nil {
					errs = append(errs, err)
				} else {
					seqs = append(seqs, m.Seq)
				}
				mu.Unlock()
			}
		}(w)
	}
	wg.Wait()

	for _, err := range errs {
		t.Fatalf("concurrent CreateMessage: %v", err)
	}
	if len(seqs) != total {
		t.Fatalf("wrote %d messages, want %d", len(seqs), total)
	}
	// 1..total 每个 seq 恰好出现一次 = 既不重号也不跳号
	seen := make(map[int64]int, total)
	for _, sq := range seqs {
		seen[sq]++
	}
	for want := int64(1); want <= total; want++ {
		switch n := seen[want]; {
		case n == 0:
			t.Fatalf("seq %d missing (gap): client incremental pull would stall here", want)
		case n > 1:
			t.Fatalf("seq %d assigned %d times (duplicate)", want, n)
		}
	}
	// 落库行数与唯一索引口径一致
	var rows int64
	if err := s.db.Model(&model.Message{}).Where("conversation_id = ?", conv.ID).Count(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if rows != total {
		t.Fatalf("persisted %d rows, want %d", rows, total)
	}
	var got model.Conversation
	if err := s.db.First(&got, conv.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.LastSeq != total {
		t.Fatalf("conversation last_seq = %d, want %d", got.LastSeq, total)
	}
}

// client_msg_id 幂等：重发不产生新行、不推进 seq，且回原消息。
func TestCreateMessageClientMsgIDIdempotent(t *testing.T) {
	s := newTestStore(t)
	conv := newConversation(t, s)
	sendText(t, s, conv.ID, "first")

	cid := "retry-1"
	mk := func() *model.Message {
		return &model.Message{
			ConversationID: conv.ID,
			ClientMsgID:    &cid,
			SenderType:     "visitor",
			MsgType:        "text",
			Content:        JSONText(map[string]any{"text": "hello"}),
		}
	}
	a := mk()
	if err := s.CreateMessage(a); err != nil {
		t.Fatal(err)
	}
	if a.Replayed {
		t.Fatal("first write must not be marked replayed")
	}
	b := mk()
	if err := s.CreateMessage(b); err != nil {
		t.Fatal(err)
	}
	if !b.Replayed {
		t.Fatal("resend of same client_msg_id must be marked replayed")
	}
	if b.Seq != a.Seq || b.ID != a.ID {
		t.Fatalf("replay returned seq=%d id=%d, want seq=%d id=%d", b.Seq, b.ID, a.Seq, a.ID)
	}
	var rows int64
	if err := s.db.Model(&model.Message{}).Where("conversation_id = ?", conv.ID).Count(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if rows != 2 {
		t.Fatalf("persisted %d rows, want 2 (replay must not insert)", rows)
	}
	var got model.Conversation
	if err := s.db.First(&got, conv.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.LastSeq != a.Seq {
		t.Fatalf("last_seq = %d, want %d (replay must not bump counter)", got.LastSeq, a.Seq)
	}
}

// 存量数据迁移：last_seq 落后于实际 MAX(seq) 时（新列刚加、默认 0），
// 回填必须把计数器抬上去，否则下一条消息会撞唯一索引 / 重号。
func TestBackfillConversationLastSeq(t *testing.T) {
	s := newTestStore(t)
	conv := newConversation(t, s)
	for i := 0; i < 3; i++ {
		sendText(t, s, conv.ID, fmt.Sprintf("old%d", i))
	}
	// 模拟「列刚加上、还没回填」的状态
	if err := s.db.Model(&model.Conversation{}).Where("id = ?", conv.ID).
		Update("last_seq", 0).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.backfillConversationLastSeq(); err != nil {
		t.Fatal(err)
	}
	var got model.Conversation
	if err := s.db.First(&got, conv.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.LastSeq != 3 {
		t.Fatalf("after backfill last_seq = %d, want 3", got.LastSeq)
	}
	// 幂等：再跑一次不变
	if err := s.backfillConversationLastSeq(); err != nil {
		t.Fatal(err)
	}
	if err := s.db.First(&got, conv.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.LastSeq != 3 {
		t.Fatalf("second backfill changed last_seq to %d, want 3 (must be idempotent)", got.LastSeq)
	}
	// 回填后继续写，seq 从 4 起，不重号
	if m := sendText(t, s, conv.ID, "new"); m.Seq != 4 {
		t.Fatalf("next seq after backfill = %d, want 4", m.Seq)
	}
}

// 插入撞唯一索引（滚动发布期旧实例按 MAX(seq)+1 抢占了 seq）时，
// CreateMessage 必须对齐计数器后重试成功，而不是报错。
func TestCreateMessageRecoversFromStaleCounter(t *testing.T) {
	s := newTestStore(t)
	conv := newConversation(t, s)
	sendText(t, s, conv.ID, "m1")

	// 旧实例风格写入：直接占用 seq=2，不动 last_seq
	if err := s.db.Create(&model.Message{
		ConversationID: conv.ID,
		SenderType:     "agent",
		MsgType:        "text",
		Content:        JSONText(map[string]any{"text": "legacy"}),
		Seq:            2,
	}).Error; err != nil {
		t.Fatal(err)
	}
	// 计数器仍是 1，下一条会先撞 seq=2，自愈后应落到 3
	m := sendText(t, s, conv.ID, "m3")
	if m.Seq != 3 {
		t.Fatalf("seq after stale-counter recovery = %d, want 3", m.Seq)
	}
	var rows int64
	if err := s.db.Model(&model.Message{}).Where("conversation_id = ?", conv.ID).Count(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if rows != 3 {
		t.Fatalf("persisted %d rows, want 3", rows)
	}
}
