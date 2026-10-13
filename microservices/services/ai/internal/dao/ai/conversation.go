// Package ai contains the persistence layer for AI conversations, messages,
// and knowledge-base documents.
package ai

import (
	"context"
	"time"

	"gorm.io/gorm"

	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/pagination"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
)

// ConversationDAO persists chat conversations.
type ConversationDAO struct {
	db *gorm.DB
}

// NewConversationDAO builds a ConversationDAO backed by an injected handle.
func NewConversationDAO(db *gorm.DB) *ConversationDAO {
	return &ConversationDAO{db: db}
}

func (d *ConversationDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

// CreateContext inserts a conversation.
func (d *ConversationDAO) CreateContext(ctx context.Context, conversation *localmodel.AIConversation) error {
	return d.dbWithContext(ctx).Create(conversation).Error
}

// GetForUserContext loads one conversation owned by userID within the tenant.
func (d *ConversationDAO) GetForUserContext(ctx context.Context, id, userID uint) (*localmodel.AIConversation, error) {
	var conversation localmodel.AIConversation
	result := d.dbWithContext(ctx).
		Where("id = ? AND user_id = ? AND tenant_id = ?", id, userID, tenant.FromContextOrDefault(ctx)).
		First(&conversation)
	return &conversation, result.Error
}

// ListForUserContext returns one page of conversations owned by userID
// within the tenant, newest first.
func (d *ConversationDAO) ListForUserContext(ctx context.Context, userID uint, req pagination.PageRequest) ([]localmodel.AIConversation, int64, error) {
	var conversations []localmodel.AIConversation
	var total int64

	query := d.dbWithContext(ctx).Model(&localmodel.AIConversation{}).
		Where("user_id = ? AND tenant_id = ?", userID, tenant.FromContextOrDefault(ctx))
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	result := query.
		Scopes(pagination.Paginate(req)).
		Order("updated_at DESC").
		Find(&conversations)
	return conversations, total, result.Error
}

// DeleteForUserContext deletes one conversation owned by userID within the
// tenant. Messages cascade at the database level. gorm.ErrRecordNotFound is
// returned when the conversation does not exist, belongs to another user, or
// belongs to another tenant.
func (d *ConversationDAO) DeleteForUserContext(ctx context.Context, id, userID uint) error {
	result := d.dbWithContext(ctx).
		Where("id = ? AND user_id = ? AND tenant_id = ?", id, userID, tenant.FromContextOrDefault(ctx)).
		Delete(&localmodel.AIConversation{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// TouchContext bumps a conversation's updated_at within the tenant.
func (d *ConversationDAO) TouchContext(ctx context.Context, id uint, at time.Time) error {
	return d.dbWithContext(ctx).
		Model(&localmodel.AIConversation{}).
		Where("id = ? AND tenant_id = ?", id, tenant.FromContextOrDefault(ctx)).
		Update("updated_at", at).Error
}
