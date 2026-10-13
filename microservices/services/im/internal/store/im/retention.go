package store

import (
	"encoding/json"
	"errors"
	"github.com/go-admin-kit/services/im/internal/model"
	"gorm.io/gorm"
	"strings"
	"time"
)

// RecallMessage 将消息标记为已撤回：仅发送者可撤回，2 分钟窗口。
// Content 会被就地替换，历史读取者看到的是墓碑（tombstone）。
func (s *Store) RecallMessage(conversationID uint64, messageID uint64, senderType string, senderID uint64) (*model.Message, error) {
	var m model.Message
	if err := s.db.Where("id = ? AND conversation_id = ?", messageID, conversationID).First(&m).Error; err != nil {
		return nil, err
	}
	if m.SenderType != senderType || m.SenderID == nil || *m.SenderID != senderID {
		return nil, errors.New("only the sender can recall")
	}
	if m.MsgType == "event" {
		return nil, errors.New("cannot recall system events")
	}
	if time.Since(m.CreatedAt) > 2*time.Minute {
		return nil, errors.New("recall window (2 minutes) has passed")
	}
	m.MsgType = "event"
	m.Content = JSONText(map[string]any{"event": "recalled", "by": senderType})
	if err := s.db.Model(&model.Message{}).Where("id = ?", m.ID).
		Updates(map[string]any{"msg_type": m.MsgType, "content": m.Content}).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// PurgeResult 报告一次留存清理的结果。
type PurgeResult struct {
	Conversations  int64
	Messages       int64
	AttachmentKeys []string
}

// PurgeExpired 硬删除关闭时间超过 retention 的会话及其消息。
// 附件对象键（在消息内容中找到的 /im/uploads/<key> URL）会被收集，
// 供调用方从存储中删除。
// 分批处理，避免大量积压时单个事务占用数分钟。
func (s *Store) PurgeExpired(retention time.Duration, batch int) (*PurgeResult, error) {
	if retention <= 0 {
		return &PurgeResult{}, nil
	}
	if batch <= 0 {
		batch = 500
	}
	cutoff := time.Now().Add(-retention)
	res := &PurgeResult{}

	for {
		var convs []model.Conversation
		//nolint:unbounded-find -- 每轮查询由清理批大小 batch 显式约束。
		err := s.db.Select("id").
			Where("status = ? AND closed_at IS NOT NULL AND closed_at < ?", "closed", cutoff).
			Limit(batch).Find(&convs).Error
		if err != nil {
			return res, err
		}
		if len(convs) == 0 {
			return res, nil
		}
		ids := make([]uint64, len(convs))
		for i, c := range convs {
			ids[i] = c.ID
		}

		// 在行消失前收集附件键
		var contents []string
		if err := s.db.Model(&model.Message{}).
			Where("conversation_id IN ? AND msg_type IN ?", ids, []string{"image", "file"}).
			Pluck("content", &contents).Error; err != nil {
			return res, err
		}
		for _, raw := range contents {
			if key := attachmentKeyFromContent(raw); key != "" {
				res.AttachmentKeys = append(res.AttachmentKeys, key)
			}
		}

		err = s.db.Transaction(func(tx *gorm.DB) error {
			del := tx.Where("conversation_id IN ?", ids).Delete(&model.Message{})
			if del.Error != nil {
				return del.Error
			}
			res.Messages += del.RowsAffected
			del = tx.Where("id IN ?", ids).Delete(&model.Conversation{})
			if del.Error != nil {
				return del.Error
			}
			res.Conversations += del.RowsAffected
			return nil
		})
		if err != nil {
			return res, err
		}
		if len(convs) < batch {
			return res, nil
		}
	}
}

// attachmentKeyFromContent 从附件消息内容中提取存储键
// （{"url":"/im/uploads/<key>", ...}）。
func attachmentKeyFromContent(raw string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return ""
	}
	u, _ := m["url"].(string)
	const prefix = "/im/uploads/"
	if !strings.HasPrefix(u, prefix) {
		return ""
	}
	return strings.TrimPrefix(u, prefix)
}
