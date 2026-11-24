package authdao

import (
	"context"
	"time"

	model "github.com/go-admin-kit/services/shared/pkg/model"
	"gorm.io/gorm"
)

// BaseUserDAO provides standard User persistence queries backed by shared/pkg/model.User.
type BaseUserDAO struct {
	db *gorm.DB
}

func NewBaseUserDAO(db *gorm.DB) *BaseUserDAO {
	return &BaseUserDAO{db: db}
}

func (d *BaseUserDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

func (d *BaseUserDAO) GetUserByUsernameContext(ctx context.Context, username string) (*model.User, error) {
	var user model.User
	result := d.dbWithContext(ctx).Where("username = ?", username).First(&user)
	return &user, result.Error
}

func (d *BaseUserDAO) GetUserByIDContext(ctx context.Context, id uint) (*model.User, error) {
	var user model.User
	result := d.dbWithContext(ctx).First(&user, id)
	return &user, result.Error
}

func (d *BaseUserDAO) GetUserWithRolesContext(ctx context.Context, id uint) (*model.User, error) {
	var user model.User
	result := d.dbWithContext(ctx).Preload("Roles").First(&user, id)
	return &user, result.Error
}

func (d *BaseUserDAO) GetUserByEmailContext(ctx context.Context, email string) (*model.User, error) {
	var user model.User
	result := d.dbWithContext(ctx).Where("email = ?", email).First(&user)
	return &user, result.Error
}

func (d *BaseUserDAO) CreateUserContext(ctx context.Context, user *model.User) error {
	return d.dbWithContext(ctx).Create(user).Error
}

func (d *BaseUserDAO) UpdateUserContext(ctx context.Context, user *model.User) error {
	return d.dbWithContext(ctx).Save(user).Error
}

// AuthUserOps encapsulates auth-specific user mutations (profile, password history,
// TOTP setup/recovery codes, phone & role-permission lookup) parameterized by the
// service's User model type U.
type AuthUserOps[U any] struct {
	db *gorm.DB
}

func NewAuthUserOps[U any](db *gorm.DB) *AuthUserOps[U] {
	return &AuthUserOps[U]{db: db}
}

func (d *AuthUserOps[U]) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

func (d *AuthUserOps[U]) UpdateUserProfileContext(ctx context.Context, id uint, updates map[string]any) error {
	var u U
	return d.dbWithContext(ctx).Model(&u).Where("id = ?", id).Updates(updates).Error
}

func (d *AuthUserOps[U]) ListRecentPasswordHistoryContext(ctx context.Context, userID uint, limit int) ([]model.PasswordHistory, error) {
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

func (d *AuthUserOps[U]) CreatePasswordHistoryContext(ctx context.Context, history *model.PasswordHistory) error {
	return d.dbWithContext(ctx).Create(history).Error
}

func (d *AuthUserOps[U]) MarkPasswordChangeRequiredContext(ctx context.Context, userID uint) error {
	var u U
	return d.dbWithContext(ctx).
		Model(&u).
		Where("id = ?", userID).
		Update("must_change_password", true).Error
}

func (d *AuthUserOps[U]) UpdatePasswordWithHistoryContext(
	ctx context.Context,
	userID uint,
	previousHash string,
	newHash string,
	changedAt time.Time,
	historyCount int,
) error {
	return d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var u U
		result := tx.Model(&u).
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

func (d *AuthUserOps[U]) UpdateTOTPSetupContext(ctx context.Context, userID uint, secret string) error {
	var u U
	return d.dbWithContext(ctx).
		Model(&u).
		Where("id = ?", userID).
		Updates(map[string]any{
			"totp_secret":  secret,
			"totp_enabled": false,
		}).Error
}

func (d *AuthUserOps[U]) EnableTOTPWithRecoveryCodesContext(ctx context.Context, userID uint, codeHashes []string, createdAt time.Time) error {
	return d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var u U
		result := tx.Model(&u).
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

func (d *AuthUserOps[U]) ReplaceTOTPRecoveryCodesContext(ctx context.Context, userID uint, codeHashes []string, createdAt time.Time) error {
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

func (d *AuthUserOps[U]) ListUnusedTOTPRecoveryCodesContext(ctx context.Context, userID uint) ([]model.TOTPRecoveryCode, error) {
	var codes []model.TOTPRecoveryCode
	//nolint:unbounded-find // per-user TOTP recovery codes are bounded (8-16 rows)
	err := d.dbWithContext(ctx).
		Where("user_id = ? AND used_at IS NULL", userID).
		Order("id ASC").
		Find(&codes).Error
	return codes, err
}

func (d *AuthUserOps[U]) MarkTOTPRecoveryCodeUsedContext(ctx context.Context, userID uint, codeID uint, usedAt time.Time) error {
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

func (d *AuthUserOps[U]) DisableTOTPContext(ctx context.Context, userID uint) error {
	return d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var u U
		result := tx.Model(&u).
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

func (d *AuthUserOps[U]) GetUserByPhoneContext(ctx context.Context, phone string) (*U, error) {
	var user U
	result := d.dbWithContext(ctx).Where("phone = ?", phone).First(&user)
	return &user, result.Error
}

func (d *AuthUserOps[U]) GetUserWithRolesAndPermissionsContext(ctx context.Context, id uint) (*U, error) {
	var user U
	result := d.dbWithContext(ctx).
		Preload("Roles.Permissions").
		First(&user, id)
	return &user, result.Error
}
