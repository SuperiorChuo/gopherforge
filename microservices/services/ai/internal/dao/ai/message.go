package ai

import (
	"context"

	"gorm.io/gorm"

	localmodel "github.com/go-admin-kit/services/ai/internal/model"
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
func (d *MessageDAO) CreateContext(ctx context.Context, message *localmodel.AIMessage) error {
	return d.dbWithContext(ctx).Create(message).Error
}

// Message list bounds. AIMessage.Content is a text column, so an unbounded
// Find returns every turn of a long conversation in one response.
const (
	// DefaultMessagePageSize is the window size used when the caller passes none.
	DefaultMessagePageSize = 50
	// MaxMessagePageSize caps whatever the caller asks for.
	MaxMessagePageSize = 200
)

// NormalizeMessageLimit clamps a caller-supplied page size into
// [1, MaxMessagePageSize], defaulting to DefaultMessagePageSize.
func NormalizeMessageLimit(limit int) int {
	if limit <= 0 {
		return DefaultMessagePageSize
	}
	if limit > MaxMessagePageSize {
		return MaxMessagePageSize
	}
	return limit
}

// ListByConversationContext returns one keyset page of a conversation's
// messages in chronological order (oldest of the window first), newest page
// first. beforeID is the exclusive upper cursor: pass 0 for the newest page,
// then the id of the oldest row you already hold to walk backwards. hasMore
// reports whether older messages remain before the returned window.
//
// Keyset (id < cursor) rather than OFFSET: idx_ai_messages_conv_id
// (monitor/migrations/000015) covers (conversation_id, id), so every page
// costs one index range scan regardless of conversation length.
func (d *MessageDAO) ListByConversationContext(ctx context.Context, conversationID uint, beforeID uint, limit int) ([]localmodel.AIMessage, bool, error) {
	limit = NormalizeMessageLimit(limit)

	query := d.dbWithContext(ctx).Where("conversation_id = ?", conversationID)
	if beforeID > 0 {
		query = query.Where("id < ?", beforeID)
	}

	var messages []localmodel.AIMessage
	// limit+1 probes for a further page without a second COUNT query.
	if err := query.Order("id DESC").Limit(limit + 1).Find(&messages).Error; err != nil {
		return nil, false, err
	}

	hasMore := len(messages) > limit
	if hasMore {
		messages = messages[:limit]
	}
	reverseMessages(messages)
	return messages, hasMore, nil
}

// ListRecentByConversationContext returns the newest limit messages of a
// conversation in chronological order (oldest of the window first).
func (d *MessageDAO) ListRecentByConversationContext(ctx context.Context, conversationID uint, limit int) ([]localmodel.AIMessage, error) {
	var messages []localmodel.AIMessage
	err := d.dbWithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Order("id DESC").
		Limit(limit).
		Find(&messages).Error
	if err != nil {
		return nil, err
	}
	reverseMessages(messages)
	return messages, nil
}

// reverseMessages flips a newest-first slice into chronological order.
func reverseMessages(messages []localmodel.AIMessage) {
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
}
