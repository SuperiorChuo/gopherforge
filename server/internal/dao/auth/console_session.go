package auth

import (
	"context"
	"time"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"gorm.io/gorm"
)

type ConsoleSessionDAO struct {
	db *gorm.DB
}

func NewConsoleSessionDAO(db *gorm.DB) ConsoleSessionDAO {
	return ConsoleSessionDAO{db: db}
}

func (d ConsoleSessionDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	if d.db != nil {
		return d.db.WithContext(ctx)
	}
	return database.DB.WithContext(ctx)
}

func (d ConsoleSessionDAO) Ready() bool {
	return d.db != nil || database.DB != nil
}

// Deprecated: use CreateContext instead.
func (d ConsoleSessionDAO) Create(record *model.ConsoleSession) error {
	return d.CreateContext(context.Background(), record)
}

func (d ConsoleSessionDAO) CreateContext(ctx context.Context, record *model.ConsoleSession) error {
	return d.dbWithContext(ctx).Create(record).Error
}

// Deprecated: use GetBySessionIDContext instead.
func (d ConsoleSessionDAO) GetBySessionID(sessionID string) (*model.ConsoleSession, error) {
	return d.GetBySessionIDContext(context.Background(), sessionID)
}

func (d ConsoleSessionDAO) GetBySessionIDContext(ctx context.Context, sessionID string) (*model.ConsoleSession, error) {
	var record model.ConsoleSession
	err := d.dbWithContext(ctx).First(&record, "session_id = ?", sessionID).Error
	return &record, err
}

// Deprecated: use TouchContext instead.
func (d ConsoleSessionDAO) Touch(sessionID string, seenAt time.Time) error {
	return d.TouchContext(context.Background(), sessionID, seenAt)
}

func (d ConsoleSessionDAO) TouchContext(ctx context.Context, sessionID string, seenAt time.Time) error {
	return d.dbWithContext(ctx).Model(&model.ConsoleSession{}).
		Where("session_id = ?", sessionID).
		Update("last_seen_at", seenAt).
		Error
}

// Deprecated: use RevokeContext instead.
func (d ConsoleSessionDAO) Revoke(record *model.ConsoleSession, revokedAt time.Time) error {
	return d.RevokeContext(context.Background(), record, revokedAt)
}

func (d ConsoleSessionDAO) RevokeContext(ctx context.Context, record *model.ConsoleSession, revokedAt time.Time) error {
	return d.dbWithContext(ctx).Model(record).Update("revoked_at", revokedAt).Error
}
