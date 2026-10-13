package store

import (
	"errors"
	"fmt"
	"github.com/go-admin-kit/services/im/internal/model"
	"gorm.io/gorm"
	"strings"
	"time"
)

func (s *Store) UpsertVisitor(siteID uint64, guestKey, displayName string) (*model.Visitor, error) {
	var v model.Visitor
	err := s.db.Where("site_id = ? AND guest_key = ?", siteID, guestKey).First(&v).Error
	now := time.Now()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		v = model.Visitor{
			SiteID:      siteID,
			GuestKey:    guestKey,
			DisplayName: displayName,
			LastSeenAt:  now,
		}
		if v.DisplayName == "" {
			v.DisplayName = "访客"
		}
		if err := s.db.Create(&v).Error; err != nil {
			return nil, err
		}
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	v.LastSeenAt = now
	if displayName != "" {
		v.DisplayName = displayName
	}
	if err := s.db.Save(&v).Error; err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *Store) GetVisitor(id uint64) (*model.Visitor, error) {
	var v model.Visitor
	if err := s.db.First(&v, id).Error; err != nil {
		return nil, err
	}
	return &v, nil
}

// UpdateVisitorNote 设置由坐席维护的备注（M5）。
func (s *Store) UpdateVisitorNote(id uint64, note string) error {
	return s.db.Model(&model.Visitor{}).Where("id = ?", id).Update("note", note).Error
}

// VisitorProfilePatch 是访客联系字段的部分更新。
// 非 nil 指针会被应用；使用单独的 Clear* 时，空字符串用于清空字段。
type VisitorProfilePatch struct {
	DisplayName *string
	Phone       *string
	Email       *string
	Wechat      *string
}

// UpdateVisitorProfile 更新访客或坐席留下的联系字段。
// 使用旧的 2 参路径时，displayName/phone 中的空字符串会被忽略。
// 多字段更新优先使用 UpdateVisitorProfilePatch。
func (s *Store) UpdateVisitorProfile(id uint64, displayName, phone string) error {
	p := VisitorProfilePatch{}
	if strings.TrimSpace(displayName) != "" {
		dn := strings.TrimSpace(displayName)
		p.DisplayName = &dn
	}
	if strings.TrimSpace(phone) != "" {
		ph := strings.TrimSpace(phone)
		p.Phone = &ph
	}
	return s.UpdateVisitorProfilePatch(id, p)
}

// UpdateVisitorProfilePatch 应用所有非 nil 的联系字段。
func (s *Store) UpdateVisitorProfilePatch(id uint64, p VisitorProfilePatch) error {
	updates := map[string]any{"last_seen_at": time.Now()}
	if p.DisplayName != nil {
		dn := strings.TrimSpace(*p.DisplayName)
		if len(dn) > 128 {
			dn = dn[:128]
		}
		if dn != "" {
			updates["display_name"] = dn
		}
	}
	if p.Phone != nil {
		ph := strings.TrimSpace(*p.Phone)
		if len(ph) > 32 {
			ph = ph[:32]
		}
		updates["phone"] = ph
	}
	if p.Email != nil {
		em := strings.TrimSpace(*p.Email)
		if len(em) > 128 {
			em = em[:128]
		}
		updates["email"] = em
	}
	if p.Wechat != nil {
		wx := strings.TrimSpace(*p.Wechat)
		if len(wx) > 64 {
			wx = wx[:64]
		}
		updates["wechat"] = wx
	}
	return s.db.Model(&model.Visitor{}).Where("id = ?", id).Updates(updates).Error
}

// ClearVisitorPhone 清除已保存的电话（仅坐席）。
func (s *Store) ClearVisitorPhone(id uint64) error {
	return s.db.Model(&model.Visitor{}).Where("id = ?", id).Update("phone", "").Error
}

// ClearVisitorField 清除 phone|email|wechat 中的某一个。
func (s *Store) ClearVisitorField(id uint64, field string) error {
	switch field {
	case "phone", "email", "wechat":
		return s.db.Model(&model.Visitor{}).Where("id = ?", id).Update(field, "").Error
	default:
		return fmt.Errorf("unknown field %q", field)
	}
}

// UpdateConversationContext 覆盖会话上下文 JSON（来源页面、IP 等）。
func (s *Store) UpdateConversationContext(id uint64, contextJSON string) error {
	if contextJSON == "" {
		return nil
	}
	return s.db.Model(&model.Conversation{}).Where("id = ?", id).Update("context", contextJSON).Error
}

// SetVisitorBanned 切换访客黑名单标记（M6）。
func (s *Store) SetVisitorBanned(id uint64, banned bool) error {
	return s.db.Model(&model.Visitor{}).Where("id = ?", id).Update("banned", banned).Error
}

// CountVisitorConversations 返回该访客已开启的会话总数。
func (s *Store) CountVisitorConversations(visitorID uint64) (int64, error) {
	var n int64
	err := s.db.Model(&model.Conversation{}).Where("visitor_id = ?", visitorID).Count(&n).Error
	return n, err
}
