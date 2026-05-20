package auth

import (
	"context"
	"time"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
)

type ConsoleSessionDAO struct{}

func (d ConsoleSessionDAO) Ready() bool {
	return database.DB != nil
}

func (d ConsoleSessionDAO) Create(record *model.ConsoleSession) error {
	return d.CreateContext(context.Background(), record)
}

func (d ConsoleSessionDAO) CreateContext(ctx context.Context, record *model.ConsoleSession) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return database.DB.WithContext(ctx).Create(record).Error
}

func (d ConsoleSessionDAO) GetBySessionID(sessionID string) (*model.ConsoleSession, error) {
	return d.GetBySessionIDContext(context.Background(), sessionID)
}

func (d ConsoleSessionDAO) GetBySessionIDContext(ctx context.Context, sessionID string) (*model.ConsoleSession, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var record model.ConsoleSession
	err := database.DB.WithContext(ctx).First(&record, "session_id = ?", sessionID).Error
	return &record, err
}

func (d ConsoleSessionDAO) Touch(sessionID string, seenAt time.Time) error {
	return d.TouchContext(context.Background(), sessionID, seenAt)
}

func (d ConsoleSessionDAO) TouchContext(ctx context.Context, sessionID string, seenAt time.Time) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return database.DB.WithContext(ctx).Model(&model.ConsoleSession{}).
		Where("session_id = ?", sessionID).
		Update("last_seen_at", seenAt).
		Error
}

func (d ConsoleSessionDAO) Revoke(record *model.ConsoleSession, revokedAt time.Time) error {
	return d.RevokeContext(context.Background(), record, revokedAt)
}

func (d ConsoleSessionDAO) RevokeContext(ctx context.Context, record *model.ConsoleSession, revokedAt time.Time) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return database.DB.WithContext(ctx).Model(record).Update("revoked_at", revokedAt).Error
}
