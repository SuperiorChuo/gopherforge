package store

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-admin-kit/services/crm/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/idempotency"
	"gorm.io/gorm"
	"strconv"
	"strings"
	"time"
)

var followTargets = map[string]bool{"lead": true, "customer": true, "opportunity": true}

// CreateFollowUp 写跟进（无幂等键，兼容用户面与旧内部调用）。
func (s *Store) CreateFollowUp(ctx context.Context, f *model.FollowUp) error {
	return s.CreateFollowUpIdempotent(ctx, f, "")
}

// CreateFollowUpIdempotent 写跟进；idemKey 非空时同键重试返回首次结果（不双写）。
func (s *Store) CreateFollowUpIdempotent(ctx context.Context, f *model.FollowUp, idemKey string) error {
	if !followTargets[f.TargetType] {
		return errors.New("无效跟进目标类型")
	}
	if strings.TrimSpace(f.Content) == "" {
		return errors.New("跟进内容必填")
	}
	// 目标必须存在于同租户
	var err error
	switch f.TargetType {
	case "lead":
		_, err = s.GetLead(f.TargetID, f.TenantID)
	case "customer":
		_, err = s.GetCustomer(f.TargetID, f.TenantID)
	case "opportunity":
		_, err = s.GetOpportunity(f.TargetID, f.TenantID)
	}
	if err != nil {
		return errors.New("跟进目标不存在")
	}

	reqHash := idempotency.Hash(struct {
		TenantID   uint64 `json:"tenant_id"`
		TargetType string `json:"target_type"`
		TargetID   uint64 `json:"target_id"`
		Content    string `json:"content"`
		ByUserID   uint64 `json:"by_user_id"`
	}{
		TenantID: f.TenantID, TargetType: f.TargetType, TargetID: f.TargetID,
		Content: strings.TrimSpace(f.Content), ByUserID: f.ByUserID,
	})

	var reused bool
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		id, wasReused, err := idempotency.Run(tx, crmIdempotencyTable, f.TenantID, idemKey, reqHash,
			func(tx *gorm.DB) (uint64, error) {
				if err := tx.Create(f).Error; err != nil {
					return 0, err
				}
				return f.ID, nil
			})
		if err != nil {
			return err
		}
		reused = wasReused
		if wasReused {
			var existing model.FollowUp
			if err := tx.First(&existing, id).Error; err != nil {
				return fmt.Errorf("幂等记录对应的跟进不存在: %w", err)
			}
			*f = existing
		}
		return nil
	}); err != nil {
		return err
	}

	// 首次写入才刷新公海计时；重放不重复 Touch
	if !reused {
		switch f.TargetType {
		case "customer":
			s.TouchCustomerFollow(f.TargetID)
		case "opportunity":
			if o, err := s.GetOpportunity(f.TargetID, f.TenantID); err == nil {
				s.TouchCustomerFollow(o.CustomerID)
			}
		}
	}
	return nil
}

// PurgeIdempotencyKeys 清理超过 maxAge 的幂等键（默认 24h）。
func (s *Store) PurgeIdempotencyKeys(maxAge time.Duration) (int64, error) {
	return idempotency.PurgeOlderThan(s.db, crmIdempotencyTable, maxAge)
}

func (s *Store) ListFollowUps(tenantID uint64, targetType string, targetID uint64, limit int) ([]model.FollowUp, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var list []model.FollowUp
	err := tenantQ(s.db, tenantID).Where("target_type = ? AND target_id = ?", targetType, targetID).
		Order("id DESC").Limit(limit).Find(&list).Error
	return list, err
}

// DueFollowUp is a past-next_at follow-up with a display name for inbox.
type DueFollowUp struct {
	model.FollowUp
	TargetName string `json:"target_name"`
}

// ListDueFollowUps returns follow-ups whose next_at has passed (回访到期提醒).
func (s *Store) ListDueFollowUps(limit int) ([]DueFollowUp, error) {
	if limit <= 0 {
		limit = 100
	}
	now := time.Now()
	var rows []model.FollowUp
	// only "recently" due (last 14d) so ancient next_at doesn't keep firing after dedupe window
	since := now.Add(-14 * 24 * time.Hour)
	if err := s.db.Where("next_at IS NOT NULL AND next_at <= ? AND next_at >= ?", now, since).
		Order("next_at ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]DueFollowUp, 0, len(rows))
	for i := range rows {
		f := rows[i]
		name := s.resolveFollowTargetName(f.TargetType, f.TargetID, f.TenantID)
		out = append(out, DueFollowUp{FollowUp: f, TargetName: name})
	}
	return out, nil
}

func (s *Store) resolveFollowTargetName(targetType string, targetID, tenantID uint64) string {
	switch targetType {
	case "customer":
		if c, err := s.GetCustomer(targetID, tenantID); err == nil {
			return c.Name
		}
	case "lead":
		if l, err := s.GetLead(targetID, tenantID); err == nil {
			return l.Name
		}
	case "opportunity":
		if o, err := s.GetOpportunity(targetID, tenantID); err == nil {
			return o.Name
		}
	}
	return targetType + "#" + strconv.FormatUint(targetID, 10)
}
