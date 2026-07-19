package auth

import (
	"context"
	"time"

	sharedDAO "github.com/go-admin-kit/services/identity/internal/dao"
	"github.com/go-admin-kit/services/identity/internal/model"
	"gorm.io/gorm"
)

// UserDAO keeps auth-specific user queries while reusing shared user persistence methods.
type UserDAO struct {
	sharedDAO.UserDAO
	db *gorm.DB
}

func NewUserDAO(db *gorm.DB) *UserDAO {
	shared := sharedDAO.NewUserDAO(db)
	return &UserDAO{
		UserDAO: *shared,
		db:      db,
	}
}

func (d *UserDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

func (d *UserDAO) UpdateUserProfileContext(ctx context.Context, id uint, updates map[string]any) error {
	return d.dbWithContext(ctx).Model(&model.User{}).Where("id = ?", id).Updates(updates).Error
}

func (d *UserDAO) ListRecentPasswordHistoryContext(ctx context.Context, userID uint, limit int) ([]model.PasswordHistory, error) {
	if limit <= 0 {
		return nil, nil
	}

	var history []model.PasswordHistory
	err := d.dbWithContext(ctx).
		Where("user_id = ?", userID).
		Order("changed_at DESC, id DESC").
		Limit(limit).
		Find(&history).Error
	return history, err
}

func (d *UserDAO) CreatePasswordHistoryContext(ctx context.Context, history *model.PasswordHistory) error {
	return d.dbWithContext(ctx).Create(history).Error
}

func (d *UserDAO) MarkPasswordChangeRequiredContext(ctx context.Context, userID uint) error {
	return d.dbWithContext(ctx).
		Model(&model.User{}).
		Where("id = ?", userID).
		Update("must_change_password", true).Error
}

func (d *UserDAO) UpdatePasswordWithHistoryContext(
	ctx context.Context,
	userID uint,
	previousHash string,
	newHash string,
	changedAt time.Time,
	historyCount int,
) error {
	return d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.User{}).
			Where("id = ? AND password = ?", userID, previousHash).
			Updates(map[string]any{
				"password":             newHash,
				"must_change_password": false,
				"password_changed_at":  changedAt,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}

		if historyCount <= 0 {
			return nil
		}
		return tx.Create(&model.PasswordHistory{
			UserID:       userID,
			PasswordHash: previousHash,
			ChangedAt:    changedAt,
		}).Error
	})
}

func (d *UserDAO) UpdateTOTPSetupContext(ctx context.Context, userID uint, secret string) error {
	return d.dbWithContext(ctx).
		Model(&model.User{}).
		Where("id = ?", userID).
		Updates(map[string]any{
			"totp_secret":  secret,
			"totp_enabled": false,
		}).Error
}

func (d *UserDAO) EnableTOTPWithRecoveryCodesContext(ctx context.Context, userID uint, codeHashes []string, createdAt time.Time) error {
	return d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.User{}).
			Where("id = ?", userID).
			Update("totp_enabled", true)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}

		if err := tx.Where("user_id = ?", userID).Delete(&model.TOTPRecoveryCode{}).Error; err != nil {
			return err
		}
		if len(codeHashes) == 0 {
			return nil
		}

		codes := make([]model.TOTPRecoveryCode, 0, len(codeHashes))
		for _, hash := range codeHashes {
			codes = append(codes, model.TOTPRecoveryCode{
				UserID:    userID,
				CodeHash:  hash,
				CreatedAt: createdAt,
				UpdatedAt: createdAt,
			})
		}
		return tx.Create(&codes).Error
	})
}

func (d *UserDAO) ReplaceTOTPRecoveryCodesContext(ctx context.Context, userID uint, codeHashes []string, createdAt time.Time) error {
	return d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&model.TOTPRecoveryCode{}).Error; err != nil {
			return err
		}
		if len(codeHashes) == 0 {
			return nil
		}
		codes := make([]model.TOTPRecoveryCode, 0, len(codeHashes))
		for _, hash := range codeHashes {
			codes = append(codes, model.TOTPRecoveryCode{
				UserID:    userID,
				CodeHash:  hash,
				CreatedAt: createdAt,
				UpdatedAt: createdAt,
			})
		}
		return tx.Create(&codes).Error
	})
}

func (d *UserDAO) ListUnusedTOTPRecoveryCodesContext(ctx context.Context, userID uint) ([]model.TOTPRecoveryCode, error) {
	var codes []model.TOTPRecoveryCode
	err := d.dbWithContext(ctx).
		Where("user_id = ? AND used_at IS NULL", userID).
		Order("id ASC").
		Find(&codes).Error
	return codes, err
}

func (d *UserDAO) MarkTOTPRecoveryCodeUsedContext(ctx context.Context, userID uint, codeID uint, usedAt time.Time) error {
	result := d.dbWithContext(ctx).
		Model(&model.TOTPRecoveryCode{}).
		Where("user_id = ? AND id = ? AND used_at IS NULL", userID, codeID).
		Updates(map[string]any{"used_at": usedAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (d *UserDAO) DisableTOTPContext(ctx context.Context, userID uint) error {
	return d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.User{}).
			Where("id = ?", userID).
			Updates(map[string]any{
				"totp_secret":  "",
				"totp_enabled": false,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return tx.Where("user_id = ?", userID).Delete(&model.TOTPRecoveryCode{}).Error
	})
}

func (d *UserDAO) GetUserByPhoneContext(ctx context.Context, phone string) (*model.User, error) {
	var user model.User
	result := d.dbWithContext(ctx).Where("phone = ?", phone).First(&user)
	return &user, result.Error
}

func (d *UserDAO) GetUserWithRolesAndPermissionsContext(ctx context.Context, id uint) (*model.User, error) {
	var user model.User
	result := d.dbWithContext(ctx).
		Preload("Roles.Permissions").
		First(&user, id)
	return &user, result.Error
}
