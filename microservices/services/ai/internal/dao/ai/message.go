package ai

import (
	"context"

	"gorm.io/gorm"

	"github.com/go-admin-kit/services/ai/internal/model"
)

// MessageDAO persists chat messages.
type MessageDAO struct {
	db *gorm.DB
}

// NewMessageDAO builds a MessageDAO backed by an injected handle.
func NewMessageDAO(db *gorm.DB) *MessageDAO {
	return &MessageDAO{db: db}
}

func (d *MessageDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

// CreateContext inserts a message.
func (d *MessageDAO) CreateContext(ctx context.Context, message *model.AIMessage) error {
	return d.dbWithContext(ctx).Create(message).Error
}

// ListByConversationContext returns all messages of a conversation in
// chronological order.
func (d *MessageDAO) ListByConversationContext(ctx context.Context, conversationID uint) ([]model.AIMessage, error) {
	var messages []model.AIMessage
	err := d.dbWithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Order("id ASC").
		Find(&messages).Error
	return messages, err
}

// ListRecentByConversationContext returns the newest limit messages of a
// conversation in chronological order (oldest of the window first).
func (d *MessageDAO) ListRecentByConversationContext(ctx context.Context, conversationID uint, limit int) ([]model.AIMessage, error) {
	var messages []model.AIMessage
	err := d.dbWithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Order("id DESC").
		Limit(limit).
		Find(&messages).Error
	if err != nil {
		return nil, err
	}
	// Reverse into chronological order.
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
}
