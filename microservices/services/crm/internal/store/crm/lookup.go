package store

import (
	"errors"
	"github.com/go-admin-kit/services/crm/internal/model"
	"strings"
)

type PhoneLookup struct {
	Matched  string          `json:"matched"` // customer | lead | none
	Customer *model.Customer `json:"customer,omitempty"`
	Contact  *model.Contact  `json:"contact,omitempty"` // 命中联系人时附带
	Lead     *model.Lead     `json:"lead,omitempty"`
}

// normalizePhone keeps digits only（去空格/横线/+86 前缀差异靠尾号匹配兜底）。
func normalizePhone(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// LookupByPhone 按号码识别来电归属：客户 > 联系人（带出客户）> 线索。
// 匹配口径：原样精确 或 归一化后尾 11 位一致。
func (s *Store) LookupByPhone(tenantID uint64, phone string) (*PhoneLookup, error) {
	out := &PhoneLookup{Matched: "none"}
	digits := normalizePhone(phone)
	if strings.TrimSpace(phone) == "" || digits == "" {
		return nil, errors.New("phone is required")
	}
	tail := digits
	if len(tail) > 11 {
		tail = tail[len(tail)-11:]
	}
	like := "%" + tail

	var cu model.Customer
	if err := tenantQ(s.db, tenantID).
		Where("phone = ? OR phone LIKE ?", phone, like).
		Order("id DESC").First(&cu).Error; err == nil {
		out.Matched = "customer"
		out.Customer = &cu
		return out, nil
	}
	var ct model.Contact
	if err := tenantQ(s.db, tenantID).
		Where("phone = ? OR phone LIKE ?", phone, like).
		Order("id DESC").First(&ct).Error; err == nil {
		if c, err := s.GetCustomer(ct.CustomerID, tenantID); err == nil {
			out.Matched = "customer"
			out.Customer = c
			out.Contact = &ct
			return out, nil
		}
	}
	var l model.Lead
	if err := tenantQ(s.db, tenantID).
		Where("phone = ? OR phone LIKE ?", phone, like).
		Order("id DESC").First(&l).Error; err == nil {
		out.Matched = "lead"
		out.Lead = &l
		return out, nil
	}
	return out, nil
}
