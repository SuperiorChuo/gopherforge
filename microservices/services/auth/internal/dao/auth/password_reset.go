package auth

import (
	"context"
	"time"

	"github.com/go-admin-kit/services/auth/internal/model"
	"gorm.io/gorm"
)

// PasswordResetDAO 读写 password_resets 表。
type PasswordResetDAO struct {
	db *gorm.DB
}

func NewPasswordResetDAO(db *gorm.DB) *PasswordResetDAO {
	return &PasswordResetDAO{db: db}
}

func (d *PasswordResetDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

// CreateContext persists a reset token row (token already hashed by caller).
func (d *PasswordResetDAO) CreateContext(ctx context.Context, reset *model.PasswordReset) error {
	return d.dbWithContext(ctx).Create(reset).Error
}

// GetByTokenHashContext fetches a reset row by its sha256 hash.
func (d *PasswordResetDAO) GetByTokenHashContext(ctx context.Context, tokenHash string) (*model.PasswordReset, error) {
	var reset model.PasswordReset
	result := d.dbWithContext(ctx).Where("token_hash = ?", tokenHash).First(&reset)
	return &reset, result.Error
}

// MarkUsedContext atomically consumes the token: only unused & unexpired rows
// transition to used_at, so concurrent double-use succeeds exactly once.
func (d *PasswordResetDAO) MarkUsedContext(ctx context.Context, id uint) error {
	result := d.dbWithContext(ctx).
		Model(&model.PasswordReset{}).
		Where("id = ? AND used_at IS NULL AND expires_at > ?", id, time.Now()).
		Update("used_at", time.Now())
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// PruneExpiredContext deletes used or expired rows older than the cutoff,
// keeping the table bounded.
func (d *PasswordResetDAO) PruneExpiredContext(ctx context.Context, before time.Time) (int64, error) {
	result := d.dbWithContext(ctx).
		Where("used_at IS NOT NULL OR expires_at < ?", before).
		Delete(&model.PasswordReset{})
	return result.RowsAffected, result.Error
}
