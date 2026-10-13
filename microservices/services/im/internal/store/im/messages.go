package store

import (
	"encoding/json"
	"errors"
	"github.com/go-admin-kit/services/im/internal/model"
	"gorm.io/gorm"
	"strings"
	"time"
)

type MessageHit struct {
	model.Message
	ConversationPublicID string `json:"conversation_public_id"`
	ConversationStatus   string `json:"conversation_status"`
}

// SearchMessages 对文本消息做租户范围内的子串检索。
// content 是 JSON 串，LIKE 命中即返回；轻量场景够用，重检索再上 tsvector。
func (s *Store) SearchMessages(tenantID uint64, keyword string, limit int) ([]MessageHit, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, errors.New("keyword is required")
	}
	if tenantID == 0 {
		tenantID = 1
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	var hits []MessageHit
	err := s.db.Table("im_messages AS m").
		Select("m.*, c.public_id AS conversation_public_id, c.status AS conversation_status").
		Joins("JOIN im_conversations c ON c.id = m.conversation_id").
		Where("c.tenant_id = ? AND m.msg_type = ? AND m.content LIKE ?", tenantID, "text", "%"+keyword+"%").
		Order("m.id DESC").Limit(limit).
		Scan(&hits).Error
	return hits, err
}

// seqAllocRetries 覆盖滚动发布窗口：旧实例仍按 MAX(seq)+1 插入且不动 last_seq，
// 与新实例的计数器可能撞同一个 seq。撞了就把 last_seq 与实际 MAX(seq) 对齐后重试。
const seqAllocRetries = 5

// CreateMessage 落库一条消息并刷新会话预览。
//
// seq 分配由 `UPDATE ... SET last_seq = last_seq + 1 ... RETURNING last_seq`
// 一条语句完成：行级写锁只在这一条语句内部持有，同一会话的并发写串行排队但不再
// 跨多次往返握着显式 FOR UPDATE，也不再需要 SELECT MAX(seq)。同一 last_seq 不会
// 发给两个事务（读改写在单语句内原子完成），计数器只 +1 所以不跳号；
// ux_im_messages_conv_seq 唯一索引仍在，重号会被拒并触发重试兜底。
// 往返数：4（SELECT FOR UPDATE + MAX(seq) + INSERT + UPDATE）→ 2（UPDATE
// RETURNING + INSERT），带 client_msg_id 的幂等写多一次去重 SELECT。
func (s *Store) CreateMessage(m *model.Message) error {
	// 幂等：widget 重试/断线重发同一 client_msg_id 时返回首次落库的消息，
	// 不再自增 seq、不再刷会话预览
	if prev, err := s.findReplayedMessage(m); err != nil {
		return err
	} else if prev != nil {
		*m = *prev
		m.Replayed = true
		return nil
	}
	preview := MessagePreview(m)

	var lastErr error
	for attempt := 0; attempt < seqAllocRetries; attempt++ {
		now := time.Now()
		seq, err := s.bumpConversationSeq(m.ConversationID, preview, now)
		if err != nil {
			return err
		}
		m.ID = 0
		m.Seq = seq
		m.CreatedAt = now
		if err := s.db.Create(m).Error; err == nil {
			return nil
		} else {
			lastErr = err
		}
		// 冲突兜底：可能是并发同 client_msg_id 抢先落库，也可能是滚动发布期旧实例
		// 占了这个 seq。前者回读返回首条，后者对齐计数器后重试。
		if prev, ferr := s.findReplayedMessage(m); ferr == nil && prev != nil {
			*m = *prev
			m.Replayed = true
			return nil
		}
		if err := s.resyncConversationLastSeq(m.ConversationID); err != nil {
			return err
		}
	}
	return lastErr
}

// findReplayedMessage 查同一会话 + 同发送方 + 同 client_msg_id 的已落库消息；
// 无 client_msg_id 的消息（系统事件、坐席普通发言）直接跳过，不产生查询。
func (s *Store) findReplayedMessage(m *model.Message) (*model.Message, error) {
	if m.ClientMsgID == nil || *m.ClientMsgID == "" {
		return nil, nil
	}
	var prev model.Message
	err := s.db.Where("conversation_id = ? AND sender_type = ? AND client_msg_id = ?",
		m.ConversationID, m.SenderType, *m.ClientMsgID).First(&prev).Error
	if err == nil {
		return &prev, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return nil, err
}

// bumpConversationSeq 原子自增会话 seq 计数器并同时刷新预览，返回本条消息的 seq。
// 会话不存在时返回 gorm.ErrRecordNotFound（与旧的 First 行为一致）。
func (s *Store) bumpConversationSeq(conversationID uint64, preview string, now time.Time) (int64, error) {
	var seqs []int64
	err := s.db.Raw(`UPDATE im_conversations
		SET last_seq = last_seq + 1, last_message_at = ?, last_message_preview = ?, updated_at = ?
		WHERE id = ? RETURNING last_seq`, now, preview, now, conversationID).Scan(&seqs).Error
	if err != nil {
		return 0, err
	}
	if len(seqs) == 0 {
		return 0, gorm.ErrRecordNotFound
	}
	return seqs[0], nil
}

// resyncConversationLastSeq 把单个会话的 last_seq 抬到实际 MAX(seq)，用于插入
// 撞唯一索引后的自愈；幂等，落后才写。
func (s *Store) resyncConversationLastSeq(conversationID uint64) error {
	return s.db.Exec(`UPDATE im_conversations SET last_seq = (
			SELECT COALESCE(MAX(seq), 0) FROM im_messages WHERE conversation_id = ?
		) WHERE id = ? AND last_seq < (
			SELECT COALESCE(MAX(seq), 0) FROM im_messages WHERE conversation_id = ?
		)`, conversationID, conversationID, conversationID).Error
}

// MessagePreview 返回这条消息写进 im_conversations.last_message_preview 的值。
// 导出给 ws 发送路径复用：写完消息后本地更新会话字段，省掉一次会话重读。
// 系统事件仍会轻量更新最后一条预览。
func MessagePreview(m *model.Message) string {
	if m.MsgType == "event" {
		return "[系统] " + eventName(m.Content)
	}
	return previewFromContent(m.Content)
}

func eventName(content string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(content), &m); err != nil {
		return "事件"
	}
	if e, ok := m["event"].(string); ok {
		return e
	}
	return "事件"
}

func previewFromContent(content string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(content), &m); err != nil {
		return truncate(content, 200)
	}
	if t, ok := m["text"].(string); ok {
		return truncate(t, 200)
	}
	if u, ok := m["url"].(string); ok {
		name, _ := m["name"].(string)
		if isImageURL(u) {
			return "[图片]"
		}
		if name != "" {
			return "[文件] " + truncate(name, 60)
		}
		return "[文件]"
	}
	return "[消息]"
}

func isImageURL(u string) bool {
	u = strings.ToLower(u)
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".gif", ".webp"} {
		if strings.HasSuffix(u, ext) {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n])
}

// MarkRead 单调递增地推进已读游标。reader 为 "agent" 或
// "visitor"；seq <= 0 表示“读到最新一条消息”。返回
// 调用后的有效游标。
func (s *Store) MarkRead(c *model.Conversation, reader string, seq int64) (int64, error) {
	var col string
	var cur int64
	switch reader {
	case "agent":
		col, cur = "agent_last_read_seq", c.AgentLastReadSeq
	case "visitor":
		col, cur = "visitor_last_read_seq", c.VisitorLastReadSeq
	default:
		return 0, errors.New("reader must be agent|visitor")
	}
	if seq <= 0 {
		var maxSeq *int64
		if err := s.db.Model(&model.Message{}).Select("MAX(seq)").
			Where("conversation_id = ?", c.ID).Scan(&maxSeq).Error; err != nil {
			return 0, err
		}
		if maxSeq == nil {
			return cur, nil
		}
		seq = *maxSeq
	}
	if seq <= cur {
		return cur, nil
	}
	// WHERE 条件保证并发标记下的单调性
	if err := s.db.Model(&model.Conversation{}).
		Where("id = ? AND "+col+" < ?", c.ID, seq).
		Update(col, seq).Error; err != nil {
		return 0, err
	}
	if reader == "agent" {
		c.AgentLastReadSeq = seq
	} else {
		c.VisitorLastReadSeq = seq
	}
	return seq, nil
}

// AgentUnreadCounts 返回每个会话中、超过坐席已读游标的访客 text/media
// 消息数量（不含 events）。
func (s *Store) AgentUnreadCounts(conversationIDs []uint64) (map[uint64]int64, error) {
	out := make(map[uint64]int64, len(conversationIDs))
	if len(conversationIDs) == 0 {
		return out, nil
	}
	type row struct {
		ConversationID uint64
		N              int64
	}
	var rows []row
	err := s.db.Model(&model.Message{}).
		Select("im_messages.conversation_id AS conversation_id, COUNT(*) AS n").
		Joins("JOIN im_conversations c ON c.id = im_messages.conversation_id").
		Where("im_messages.conversation_id IN ?", conversationIDs).
		Where("im_messages.sender_type = ? AND im_messages.msg_type <> ?", "visitor", "event").
		Where("im_messages.seq > c.agent_last_read_seq").
		Group("im_messages.conversation_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.ConversationID] = r.N
	}
	return out, nil
}

func (s *Store) ListMessages(conversationID uint64, afterSeq int64, limit int) ([]model.Message, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := s.db.Where("conversation_id = ?", conversationID)
	if afterSeq > 0 {
		q = q.Where("seq > ?", afterSeq)
	}
	var list []model.Message
	err := q.Order("seq ASC").Limit(limit).Find(&list).Error
	return list, err
}
