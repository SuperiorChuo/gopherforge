package auth

import (
	"context"
	"errors"
	"time"

	"github.com/go-admin-kit/services/auth/internal/model"
	"gorm.io/gorm"
)

// OAuth2ApprovalDAO persists user consent records (skip repeat consent screens).
type OAuth2ApprovalDAO struct {
	db *gorm.DB
}

func NewOAuth2ApprovalDAO(db *gorm.DB) OAuth2ApprovalDAO {
	return OAuth2ApprovalDAO{db: db}
}

func (d OAuth2ApprovalDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

// GetContext returns the live (non-expired) approval for a (user, client) pair,
// or nil when none exists.
func (d OAuth2ApprovalDAO) GetContext(ctx context.Context, userID uint, clientID string) (*model.OAuth2Approval, error) {
	var approval model.OAuth2Approval
	err := d.dbWithContext(ctx).
		Where("user_id = ? AND client_id = ? AND expires_at > ?", userID, clientID, time.Now()).
		First(&approval).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &approval, nil
}

// UpsertContext stores or refreshes the consent, unioning newly granted scopes
// onto any previously approved set.
func (d OAuth2ApprovalDAO) UpsertContext(ctx context.Context, tenantID, userID uint, clientID string, scopes []string, expiresAt time.Time) error {
	db := d.dbWithContext(ctx)
	var existing model.OAuth2Approval
	err := db.Where("user_id = ? AND client_id = ?", userID, clientID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(&model.OAuth2Approval{
			TenantID: tenantID, UserID: userID, ClientID: clientID,
			Scopes: scopes, ExpiresAt: expiresAt,
		}).Error
	}
	if err != nil {
		return err
	}
	existing.Scopes = unionScopes(existing.Scopes, scopes)
	existing.ExpiresAt = expiresAt
	return db.Save(&existing).Error
}

// DeleteByClientContext removes consent records for a client (secret reset / disable).
func (d OAuth2ApprovalDAO) DeleteByClientContext(ctx context.Context, clientID string) error {
	return d.dbWithContext(ctx).
		Where("client_id = ?", clientID).
		Delete(&model.OAuth2Approval{}).Error
}

func unionScopes(existing, incoming []string) []string {
	seen := make(map[string]bool, len(existing))
	result := make([]string, 0, len(existing)+len(incoming))
	for _, s := range existing {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	for _, s := range incoming {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}
