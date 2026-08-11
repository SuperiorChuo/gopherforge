package system

import (
	"context"
	"time"

	localmodel "github.com/go-admin-kit/services/file/internal/model"
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

func (d *UploadSessionDAO) CreateContext(ctx context.Context, s *localmodel.UploadSession) error {
	return d.dbWithContext(ctx).Create(s).Error
}

func (d *UploadSessionDAO) GetByIDContext(ctx context.Context, id uint) (*localmodel.UploadSession, error) {
	var s localmodel.UploadSession
	err := d.dbWithContext(ctx).Where("id = ?", id).First(&s).Error
	return &s, err
}

// GetPendingByHashContext returns an unfinished session for the same hash +
// tenant + user (断点续传复用）。无则返回 gorm.ErrRecordNotFound。
func (d *UploadSessionDAO) GetPendingByHashContext(ctx context.Context, hash string, tenantID, userID uint) (*localmodel.UploadSession, error) {
	var s localmodel.UploadSession
	err := d.dbWithContext(ctx).
		Where("hash = ? AND tenant_id = ? AND user_id = ? AND status = ?", hash, tenantID, userID, "pending").
		Order("id DESC").First(&s).Error
	return &s, err
}

// UpdateFileNameContext rewrites the session's file name (resume with a
// renamed local file keeps the latest name instead of the stale original).
func (d *UploadSessionDAO) UpdateFileNameContext(ctx context.Context, id uint, fileName string) error {
	return d.dbWithContext(ctx).Model(&localmodel.UploadSession{}).
		Where("id = ?", id).
		Update("file_name", fileName).Error
}

// MarkChunkReceivedContext atomically records one received chunk: appends the
// part number to the bitmap and bumps the count. Returns gorm.ErrRecordNotFound
// for missing/aborted sessions.
func (d *UploadSessionDAO) MarkChunkReceivedContext(ctx context.Context, id uint, bitmap string, count int) error {
	result := d.dbWithContext(ctx).
		Model(&localmodel.UploadSession{}).
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
	return d.dbWithContext(ctx).Delete(&localmodel.UploadSession{}, id).Error
}

// PruneExpiredContext deletes sessions expired before the cutoff (lazy
// cleanup on init). Returns rows removed.
func (d *UploadSessionDAO) PruneExpiredContext(ctx context.Context, before time.Time) (int64, error) {
	if d == nil || d.db == nil {
		return 0, nil
	}
	result := d.dbWithContext(ctx).
		Where("expires_at < ?", before).
		Delete(&localmodel.UploadSession{})
	return result.RowsAffected, result.Error
}
