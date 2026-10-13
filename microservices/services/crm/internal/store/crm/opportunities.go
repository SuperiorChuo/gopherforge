package store

import (
	"errors"
	"github.com/go-admin-kit/services/crm/internal/model"
	"gorm.io/gorm"
	"strings"
	"time"
)

type OppFilter struct {
	Stage      string
	CustomerID uint64
	Owner      uint64
	// Access 数据权限硬隔离（按 owner_user_id）；零值=不限制（内部调用）。
	Access UserScope
	Page
}

func (s *Store) ListOpportunities(tenantID uint64, f OppFilter) ([]model.Opportunity, int64, error) {
	q := tenantQ(s.db.Model(&model.Opportunity{}), tenantID)
	q = applyOwnerAccess(q, f.Access)
	if f.Stage != "" {
		q = q.Where("stage = ?", f.Stage)
	}
	if f.CustomerID > 0 {
		q = q.Where("customer_id = ?", f.CustomerID)
	}
	if f.Owner > 0 {
		q = q.Where("owner_user_id = ?", f.Owner)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset, limit := f.clamp()
	var list []model.Opportunity
	err := q.Order("id DESC").Offset(offset).Limit(limit).Find(&list).Error
	return list, total, err
}

func (s *Store) CreateOpportunity(o *model.Opportunity, byUser uint64) error {
	if strings.TrimSpace(o.Name) == "" {
		return errors.New("商机名称必填")
	}
	if _, err := s.GetCustomer(o.CustomerID, o.TenantID); err != nil {
		return errors.New("客户不存在")
	}
	if o.Stage == "" {
		o.Stage = "initial"
	}
	if !model.ValidStage(o.Stage) {
		return errors.New("无效阶段")
	}
	o.Probability = model.StageProbability[o.Stage]
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(o).Error; err != nil {
			return err
		}
		return tx.Create(&model.OpportunityStageLog{
			OpportunityID: o.ID, ToStage: o.Stage, Note: "创建商机", ByUserID: byUser,
		}).Error
	})
}

func (s *Store) GetOpportunity(id, tenantID uint64) (*model.Opportunity, error) {
	var o model.Opportunity
	err := tenantQ(s.db, tenantID).First(&o, id).Error
	return &o, err
}

func (s *Store) UpdateOpportunity(o *model.Opportunity) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		return tx.Save(o).Error
	})
}

func (s *Store) ListStageLogs(oppID uint64) ([]model.OpportunityStageLog, error) {
	var logs []model.OpportunityStageLog
	//nolint:unbounded-find -- 商机详情需要单份商机的完整阶段变更历史。
	err := s.db.Where("opportunity_id = ?", oppID).Order("id ASC").Find(&logs).Error
	return logs, err
}

// AdvanceStage 推进商机阶段：终态不可再动；won/lost 记时间；全程留痕。
func (s *Store) AdvanceStage(id, tenantID uint64, to, note, lostReason string, byUser uint64) (*model.Opportunity, error) {
	if !model.ValidStage(to) {
		return nil, errors.New("无效阶段")
	}
	var o model.Opportunity
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tenantQ(tx, tenantID).First(&o, id).Error; err != nil {
			return errors.New("商机不存在")
		}
		if o.Stage == "won" || o.Stage == "lost" {
			return errors.New("商机已结束，不能再变更阶段")
		}
		if o.Stage == to {
			return errors.New("已处于该阶段")
		}
		from := o.Stage
		now := time.Now()
		o.Stage = to
		o.Probability = model.StageProbability[to]
		switch to {
		case "won":
			o.WonAt = &now
		case "lost":
			o.LostAt = &now
			o.LostReason = lostReason
		}
		if err := tx.Save(&o).Error; err != nil {
			return err
		}
		return tx.Create(&model.OpportunityStageLog{
			OpportunityID: o.ID, FromStage: from, ToStage: to, Note: note, ByUserID: byUser,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return &o, nil
}
