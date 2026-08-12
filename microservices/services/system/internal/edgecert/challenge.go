package edgecert

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const challengeTTL = 15 * time.Minute

// ChallengeStore 将 HTTP-01 响应持久化，签发请求和回调可落在不同副本。
type ChallengeStore interface {
	Put(ctx context.Context, certificateID uint64, token, keyAuthorization string, expiresAt time.Time) error
	Delete(ctx context.Context, token string) error
}

type DBChallengeStore struct {
	DB  *gorm.DB
	Now func() time.Time
}

func (s DBChallengeStore) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func validateChallengeToken(token string) error {
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 255 || strings.ContainsAny(token, "/\\\r\n\t ") {
		return fmt.Errorf("invalid challenge token")
	}
	return nil
}

func (s DBChallengeStore) Put(ctx context.Context, certificateID uint64, token, keyAuthorization string, expiresAt time.Time) error {
	if s.DB == nil {
		return fmt.Errorf("challenge database unavailable")
	}
	if err := validateChallengeToken(token); err != nil {
		return err
	}
	if certificateID == 0 || strings.TrimSpace(keyAuthorization) == "" {
		return fmt.Errorf("invalid challenge payload")
	}
	if expiresAt.IsZero() {
		expiresAt = s.now().Add(challengeTTL)
	}
	row := Challenge{
		Token: token, CertificateID: certificateID, KeyAuthorization: keyAuthorization,
		ExpiresAt: expiresAt.UTC(), CreatedAt: s.now(),
	}
	return s.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "token"}},
		DoUpdates: clause.AssignmentColumns([]string{"certificate_id", "key_authorization", "expires_at", "created_at"}),
	}).Create(&row).Error
}

func (s DBChallengeStore) Delete(ctx context.Context, token string) error {
	if s.DB == nil {
		return fmt.Errorf("challenge database unavailable")
	}
	return s.DB.WithContext(ctx).Where("token = ?", token).Delete(&Challenge{}).Error
}

// LookupChallenge 只返回未过期的持久化挑战，并顺手清理命中的过期行。
func LookupChallenge(ctx context.Context, db *gorm.DB, token string) (string, bool, error) {
	if db == nil {
		return "", false, fmt.Errorf("challenge database unavailable")
	}
	if err := validateChallengeToken(token); err != nil {
		return "", false, nil
	}
	var row Challenge
	err := db.WithContext(ctx).Where("token = ?", token).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !row.ExpiresAt.After(time.Now().UTC()) {
		_ = db.WithContext(ctx).Where("token = ?", token).Delete(&Challenge{}).Error
		return "", false, nil
	}
	return row.KeyAuthorization, row.KeyAuthorization != "", nil
}

func CleanupExpiredChallenges(ctx context.Context, db *gorm.DB, now time.Time) error {
	if db == nil {
		return fmt.Errorf("challenge database unavailable")
	}
	return db.WithContext(ctx).Where("expires_at <= ?", now.UTC()).Delete(&Challenge{}).Error
}
