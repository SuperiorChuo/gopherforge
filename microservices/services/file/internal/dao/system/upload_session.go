package system

import (
	"context"
	"time"

	"github.com/go-admin-kit/services/file/internal/model"
	"gorm.io/gorm"
)

// UploadSessionDAO persists chunked-upload sessions.
type UploadSessionDAO struct {
	db *gorm.DB
}

func NewUploadSessionDAO(db *gorm.DB) *UploadSessionDAO {
	return &UploadSessionDAO{db: db}
}

func (d *UploadSessionDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

func (d *UploadSessionDAO) CreateContext(ctx context.Context, s *model.UploadSession) error {
	return d.dbWithContext(ctx).Create(s).Error
}

func (d *UploadSessionDAO) GetByIDContext(ctx context.Context, id uint) (*model.UploadSession, error) {
	var s model.UploadSession
	err := d.dbWithContext(ctx).Where("id = ?", id).First(&s).Error
	return &s, err
}

// GetPendingByHashContext returns an unfinished session for the same hash +
// tenant (断点续传复用）。无则返回 gorm.ErrRecordNotFound。
func (d *UploadSessionDAO) GetPendingByHashContext(ctx context.Context, hash string, tenantID uint) (*model.UploadSession, error) {
	var s model.UploadSession
	err := d.dbWithContext(ctx).
		Where("hash = ? AND tenant_id = ? AND status = ?", hash, tenantID, "pending").
		Order("id DESC").First(&s).Error
	return &s, err
}

// MarkChunkReceivedContext atomically records one received chunk: appends the
// part number to the bitmap and bumps the count. Returns gorm.ErrRecordNotFound
// for missing/aborted sessions.
func (d *UploadSessionDAO) MarkChunkReceivedContext(ctx context.Context, id uint, bitmap string, count int) error {
	result := d.dbWithContext(ctx).
		Model(&model.UploadSession{}).
		Where("id = ? AND status = ?", id, "pending").
		Updates(map[string]any{
			"received_bitmap": bitmap,
			"received_count":  count,
			"updated_at":      time.Now(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (d *UploadSessionDAO) DeleteContext(ctx context.Context, id uint) error {
	return d.dbWithContext(ctx).Delete(&model.UploadSession{}, id).Error
}

// PruneExpiredContext deletes sessions expired before the cutoff (lazy
// cleanup on init). Returns rows removed.
func (d *UploadSessionDAO) PruneExpiredContext(ctx context.Context, before time.Time) (int64, error) {
	if d == nil || d.db == nil {
		return 0, nil
	}
	result := d.dbWithContext(ctx).
		Where("expires_at < ?", before).
		Delete(&model.UploadSession{})
	return result.RowsAffected, result.Error
}
