package dao

import (
	"context"
	"errors"
	"time"

	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
)

var (
	ErrHermesModelTokenInvalid = errors.New("hermes model token is invalid")
	ErrHermesModelTokenExpired = errors.New("hermes model token has expired")
)

type HermesModelTokenDAO struct {
	db *gorm.DB
}

func NewHermesModelTokenDAO(db *gorm.DB) *HermesModelTokenDAO {
	return &HermesModelTokenDAO{db: db}
}

func (d *HermesModelTokenDAO) Create(ctx context.Context, token *localmodel.HermesModelToken) error {
	return d.db.WithContext(ctx).Create(token).Error
}

// ResolveActive 用哈希查回未吊销、未过期的 token。
//
// 查询刻意绕开租户 scope：调用方此时还不知道自己是哪个租户——租户身份正是
// 从这条记录得出的。token 哈希本身是高熵且全局唯一的，不存在跨租户猜测风险。
func (d *HermesModelTokenDAO) ResolveActive(ctx context.Context, tokenHash string, now time.Time) (*localmodel.HermesModelToken, error) {
	var token localmodel.HermesModelToken
	err := d.db.WithContext(tenant.DisableScope(ctx)).
		Where("token_hash = ?", tokenHash).First(&token).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrHermesModelTokenInvalid
	}
	if err != nil {
		return nil, err
	}
	if token.RevokedAt != nil {
		return nil, ErrHermesModelTokenInvalid
	}
	if !token.ExpiresAt.After(now) {
		return nil, ErrHermesModelTokenExpired
	}
	return &token, nil
}

// TouchLastUsed 记录最近使用时间。刻意不在请求路径上等待结果——
// 这只是可观测字段，写失败不该让模型调用失败。
func (d *HermesModelTokenDAO) TouchLastUsed(ctx context.Context, id uint64, now time.Time) {
	_ = d.db.WithContext(tenant.DisableScope(ctx)).Model(&localmodel.HermesModelToken{}).
		Where("id = ?", id).Update("last_used_at", now).Error
}

// RevokeByDevice 吊销某设备的全部 token。设备被吊销或员工被停用时调用，
// 使模型访问立即失效——这是「停用可强制」的落点。
func (d *HermesModelTokenDAO) RevokeByDevice(ctx context.Context, deviceID string, now time.Time) error {
	return d.db.WithContext(ctx).Model(&localmodel.HermesModelToken{}).
		Where("device_id = ? AND revoked_at IS NULL", deviceID).
		Update("revoked_at", now).Error
}

// RevokeByUser 吊销某员工全部设备的 token。
func (d *HermesModelTokenDAO) RevokeByUser(ctx context.Context, userID uint, now time.Time) error {
	return d.db.WithContext(ctx).Model(&localmodel.HermesModelToken{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", now).Error
}

// DeleteExpired 清理过期记录，避免表无界增长。
func (d *HermesModelTokenDAO) DeleteExpired(ctx context.Context, before time.Time) error {
	return d.db.WithContext(tenant.DisableScope(ctx)).
		Where("expires_at < ?", before).Delete(&localmodel.HermesModelToken{}).Error
}
