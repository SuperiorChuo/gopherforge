package store

import (
	"context"
	"errors"
	"github.com/go-admin-kit/services/crm/internal/model"
	"gorm.io/gorm"
	"strings"
	"time"
)

type LeadFilter struct {
	Status  string
	Keyword string
	Owner   uint64
	// Scope 名片视图（P1）：all | unassigned | dept(部门池) | mine
	Scope  string
	DeptID uint64
	Me     uint64
	// Access 数据权限硬隔离（按角色 data_scope 解析）；零值=不限制（内部调用）。
	Access UserScope
	Page
}

func (s *Store) ListLeads(tenantID uint64, f LeadFilter) ([]model.Lead, int64, error) {
	q := tenantQ(s.db.Model(&model.Lead{}), tenantID)
	q = applyLeadAccess(q, f.Access)
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Owner > 0 {
		q = q.Where("owner_user_id = ?", f.Owner)
	}
	switch f.Scope {
	case "unassigned":
		q = q.Where("owner_user_id = 0 AND department_id = 0")
	case "dept":
		q = q.Where("owner_user_id = 0 AND department_id > 0")
		if f.DeptID > 0 {
			q = q.Where("department_id = ?", f.DeptID)
		}
	case "mine":
		q = q.Where("owner_user_id = ?", f.Me)
	}
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("name LIKE ? OR company LIKE ? OR phone LIKE ?", like, like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset, limit := f.clamp()
	var list []model.Lead
	err := q.Order("id DESC").Offset(offset).Limit(limit).Find(&list).Error
	return list, total, err
}

func (s *Store) CreateLead(ctx context.Context, l *model.Lead) error {
	if strings.TrimSpace(l.Name) == "" {
		return errors.New("线索名称必填")
	}
	if l.Status == "" {
		l.Status = "new"
	}
	l.RecalcScore()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Create(l).Error
	})
}

func (s *Store) GetLead(id, tenantID uint64) (*model.Lead, error) {
	var l model.Lead
	err := tenantQ(s.db, tenantID).First(&l, id).Error
	return &l, err
}

func (s *Store) UpdateLead(ctx context.Context, l *model.Lead) error {
	l.RecalcScore()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Save(l).Error
	})
}

// ConvertLead 线索转化：建客户（+主联系人），可顺带建商机；线索置 converted。
func (s *Store) ConvertLead(id, tenantID, byUser uint64, oppName string, oppAmountCents int64) (*model.Customer, *model.Opportunity, error) {
	var customer *model.Customer
	var opp *model.Opportunity
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var l model.Lead
		if err := tenantQ(tx, tenantID).First(&l, id).Error; err != nil {
			return errors.New("线索不存在")
		}
		if l.Status == "converted" {
			return errors.New("线索已转化")
		}
		now := time.Now()
		c := model.Customer{
			TenantID:     tenantID,
			Name:         firstNonEmpty(l.Company, l.Name),
			Phone:        l.Phone,
			Source:       l.Source,
			OwnerUserID:  firstNonZero(l.OwnerUserID, byUser),
			IMVisitorID:  l.IMVisitorID,
			LastFollowAt: &now,
			Note:         l.Note,
		}
		if err := tx.Create(&c).Error; err != nil {
			return err
		}
		contact := model.Contact{
			TenantID: tenantID, CustomerID: c.ID,
			Name: l.Name, Phone: l.Phone, Email: l.Email, IsPrimary: true,
		}
		if err := tx.Create(&contact).Error; err != nil {
			return err
		}
		if strings.TrimSpace(oppName) != "" {
			o := model.Opportunity{
				TenantID: tenantID, CustomerID: c.ID, Name: oppName,
				AmountCents: oppAmountCents, Stage: "initial",
				Probability: model.StageProbability["initial"],
				OwnerUserID: firstNonZero(l.OwnerUserID, byUser),
			}
			if err := tx.Create(&o).Error; err != nil {
				return err
			}
			if err := tx.Create(&model.OpportunityStageLog{
				OpportunityID: o.ID, ToStage: "initial", Note: "线索转化创建", ByUserID: byUser,
			}).Error; err != nil {
				return err
			}
			opp = &o
		}
		l.Status = "converted"
		l.CustomerID = &c.ID
		if err := tx.Save(&l).Error; err != nil {
			return err
		}
		customer = &c
		return nil
	})
	return customer, opp, err
}
