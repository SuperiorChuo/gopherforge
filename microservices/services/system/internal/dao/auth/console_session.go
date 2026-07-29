package auth

import (
	"context"
	"time"

	"github.com/go-admin-kit/services/system/internal/model"
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
	return d.db.WithContext(ctx)
}

func (d ConsoleSessionDAO) Ready() bool {
	return d.db != nil
}

func (d ConsoleSessionDAO) CreateContext(ctx context.Context, record *model.ConsoleSession) error {
	return d.dbWithContext(ctx).Create(record).Error
}

func (d ConsoleSessionDAO) GetBySessionIDContext(ctx context.Context, sessionID string) (*model.ConsoleSession, error) {
	var record model.ConsoleSession
	err := d.dbWithContext(ctx).First(&record, "session_id = ?", sessionID).Error
	return &record, err
}

// TouchContext refreshes last_seen_at, but only when the stored value is already
// older than staleAfter. last_seen_at is a liveness hint, not an audit record, so
// writing it on every authenticated request bought second-resolution freshness at
// the cost of a row lock, a WAL record and an index write per request. The
// WHERE clause does the throttling server-side, so concurrent requests on the
// same session collapse into at most one write per window without a read first.
func (d ConsoleSessionDAO) TouchContext(ctx context.Context, sessionID string, seenAt time.Time, staleAfter time.Duration) error {
	query := d.dbWithContext(ctx).Model(&model.ConsoleSession{}).
		Where("session_id = ?", sessionID)
	if staleAfter > 0 {
		query = query.Where("last_seen_at IS NULL OR last_seen_at <= ?", seenAt.Add(-staleAfter))
	}
	return query.Update("last_seen_at", seenAt).Error
}

func (d ConsoleSessionDAO) RevokeContext(ctx context.Context, record *model.ConsoleSession, revokedAt time.Time) error {
	return d.dbWithContext(ctx).Model(record).Update("revoked_at", revokedAt).Error
}
