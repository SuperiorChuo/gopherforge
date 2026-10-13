package store

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-admin-kit/services/crm/internal/model"
	"gorm.io/gorm"
	"strings"
	"time"
)

type ContractFilter struct {
	Status     string
	CustomerID uint64
	Keyword    string
	// Access 数据权限硬隔离（按合同 owner_user_id，即签约负责人）；
	// 零值=不限制（内部调用）。
	Access UserScope
	Page
}

// ContractRow 附带回款进度。
type ContractRow struct {
	model.Contract
	PaidCents int64 `json:"paid_cents"`
}

func (s *Store) ListContracts(tenantID uint64, f ContractFilter) ([]ContractRow, int64, error) {
	q := tenantQ(s.db.Model(&model.Contract{}), tenantID)
	q = applyOwnerAccess(q, f.Access)
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.CustomerID > 0 {
		q = q.Where("customer_id = ?", f.CustomerID)
	}
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("no LIKE ? OR title LIKE ?", like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset, limit := f.clamp()
	var contracts []model.Contract
	if err := q.Order("id DESC").Offset(offset).Limit(limit).Find(&contracts).Error; err != nil {
		return nil, 0, err
	}
	rows := make([]ContractRow, 0, len(contracts))
	if len(contracts) == 0 {
		return rows, total, nil
	}
	ids := make([]uint64, 0, len(contracts))
	for _, c := range contracts {
		ids = append(ids, c.ID)
	}
	// 回款进度一次分组聚合（原逐行 SUM 是 N+1）；只计已确认（审批中/驳回不计，
	// BPM M2 口径）。合同集合已经过租户 + 数据范围过滤，按 id IN 取回款即安全。
	type paidAgg struct {
		ContractID uint64
		Cents      int64
	}
	var pa []paidAgg
	if err := tenantQ(s.db.Model(&model.Payment{}), tenantID).
		Where("contract_id IN ? AND status = ?", ids, PaymentConfirmed).
		Select("contract_id, COALESCE(SUM(amount_cents),0) AS cents").
		Group("contract_id").Scan(&pa).Error; err != nil {
		return nil, 0, err
	}
	paidByContract := make(map[uint64]int64, len(pa))
	for _, a := range pa {
		paidByContract[a.ContractID] = a.Cents
	}
	for _, c := range contracts {
		rows = append(rows, ContractRow{Contract: c, PaidCents: paidByContract[c.ID]})
	}
	return rows, total, nil
}

// CreateContract 合同编号租户内唯一；关联商机时校验归属并默认带出金额。
func (s *Store) CreateContract(ctx context.Context, c *model.Contract) error {
	if strings.TrimSpace(c.No) == "" || strings.TrimSpace(c.Title) == "" {
		return errors.New("合同编号与标题必填")
	}
	if _, err := s.GetCustomer(c.CustomerID, c.TenantID); err != nil {
		return errors.New("客户不存在")
	}
	if c.OpportunityID != nil && *c.OpportunityID > 0 {
		opp, err := s.GetOpportunity(*c.OpportunityID, c.TenantID)
		if err != nil || opp.CustomerID != c.CustomerID {
			return errors.New("商机不存在或不属于该客户")
		}
		if c.AmountCents == 0 {
			c.AmountCents = opp.AmountCents
		}
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(c).Error; err != nil {
			if strings.Contains(err.Error(), "ux_crm_contract_no") || strings.Contains(strings.ToLower(err.Error()), "unique") {
				return fmt.Errorf("合同编号 %s 已存在", c.No)
			}
			return err
		}
		return nil
	})
}

func (s *Store) GetContract(id, tenantID uint64) (*model.Contract, error) {
	var c model.Contract
	err := tenantQ(s.db, tenantID).First(&c, id).Error
	return &c, err
}

func (s *Store) UpdateContract(ctx context.Context, c *model.Contract) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Save(c).Error
	})
}

// AddPayment 登记回款：仅生效合同可回款，累计不得超合同额。
func (s *Store) AddPayment(ctx context.Context, p *model.Payment) error {
	if p.AmountCents <= 0 {
		return errors.New("回款金额必须大于 0")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var c model.Contract
		if err := tenantQ(tx, p.TenantID).First(&c, p.ContractID).Error; err != nil {
			return errors.New("合同不存在")
		}
		if c.Status != "active" {
			return errors.New("仅生效中的合同可登记回款")
		}
		var paid int64
		// 超额校验计入审批中的回款（否则审批期间可重复登记超出合同额），
		// 仅排除已驳回；统计口径（只计 confirmed）见 Stats/ownerStats。
		tx.Model(&model.Payment{}).
			Where("contract_id = ? AND status <> ?", c.ID, PaymentRejected).
			Select("COALESCE(SUM(amount_cents),0)").Scan(&paid)
		if paid+p.AmountCents > c.AmountCents {
			return fmt.Errorf("回款累计将超出合同额（已回 %.2f / 合同 %.2f 元）",
				float64(paid)/100, float64(c.AmountCents)/100)
		}
		if p.PaidAt.IsZero() {
			p.PaidAt = time.Now()
		}
		return tx.Create(p).Error
	})
}

func (s *Store) ListPayments(contractID, tenantID uint64) ([]model.Payment, error) {
	var list []model.Payment
	//nolint:unbounded-find -- 合同详情需要该租户单份合同的完整回款历史。
	err := tenantQ(s.db, tenantID).Where("contract_id = ?", contractID).Order("paid_at DESC, id DESC").Find(&list).Error
	return list, err
}
