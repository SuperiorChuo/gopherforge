package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Contact 是来显归属（mod_cidlookup）的客户名库：号码 → 客户名/备注。
// 入方向来电时 cc-service 的 /api/v1/cc/cid 端点查这张表，命中即把名字回给
// FreeSWITCH 改写 caller_id_name（弹屏/话机显名）。号码按租户唯一。
type Contact struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	TenantID  uint64    `gorm:"not null;default:1;uniqueIndex:ux_cc_contacts_tenant_num" json:"tenant_id"`
	Number    string    `gorm:"size:64;not null;uniqueIndex:ux_cc_contacts_tenant_num" json:"number"`
	Name      string    `gorm:"size:128;not null" json:"name"`
	Remark    string    `gorm:"size:256" json:"remark"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Contact) TableName() string { return "cc_contacts" }

// normalizeNumber 只保留数字与 + 号（cidlookup 侧号码可能带格式），空格/横线去掉，
// 保证话单页维护的号码与来电号可匹配。
func normalizeNumber(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= '0' && r <= '9') || r == '+' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (s *Store) ListContacts(ctx context.Context, tenantID uint64, q string, page, pageSize int) ([]Contact, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}
	db := s.db.WithContext(ctx).Model(&Contact{}).Where("tenant_id = ?", EnsureTenant(tenantID))
	if q != "" {
		like := "%" + q + "%"
		db = db.Where("number LIKE ? OR name LIKE ?", like, like)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []Contact
	err := db.Order("updated_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error
	return list, total, err
}

// UpsertContact 按 tenant+number 幂等：话单页"备注姓名"按钮反复点也只更新一行。
func (s *Store) UpsertContact(ctx context.Context, in *Contact) (*Contact, error) {
	in.TenantID = EnsureTenant(in.TenantID)
	in.Number = normalizeNumber(in.Number)
	in.Name = strings.TrimSpace(in.Name)
	if in.Number == "" || in.Name == "" {
		return nil, errors.New("number and name required")
	}
	var existing Contact
	err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND number = ?", in.TenantID, in.Number).
		First(&existing).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		if err := s.db.WithContext(ctx).Create(in).Error; err != nil {
			return nil, err
		}
		return in, nil
	case err != nil:
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&Contact{}).Where("id = ?", existing.ID).Updates(map[string]any{
		"name":       in.Name,
		"remark":     in.Remark,
		"updated_at": time.Now(),
	}).Error; err != nil {
		return nil, err
	}
	var out Contact
	if err := s.db.WithContext(ctx).First(&out, existing.ID).Error; err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Store) DeleteContact(ctx context.Context, tenantID uint64, number string) (bool, error) {
	// 先查主键再按 id 删——audittrail 插件要求删除谓词含主键。
	var existing Contact
	err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND number = ?", EnsureTenant(tenantID), normalizeNumber(number)).
		First(&existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	res := s.db.WithContext(ctx).
		Where("id = ?", existing.ID).
		Delete(&Contact{})
	return res.RowsAffected > 0, res.Error
}

// LookupContactName 供开放 cid 端点：号码 → 客户名（未命中返回空串）。
// cid 端点不带租户上下文（mod_cidlookup 无鉴权），默认租户 1 查询。
func (s *Store) LookupContactName(ctx context.Context, tenantID uint64, number string) (string, error) {
	num := normalizeNumber(number)
	if num == "" {
		return "", nil
	}
	var c Contact
	err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND number = ?", EnsureTenant(tenantID), num).
		First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return c.Name, nil
}
