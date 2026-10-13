package store

import (
	"context"
	"errors"
	"github.com/go-admin-kit/services/crm/internal/model"
	"gorm.io/gorm"
	"strings"
	"time"
)

type CustomerFilter struct {
	Keyword string
	Level   string
	Owner   uint64
	// Scope 客户视图（P2）：all | mine | today(今日成为我的) |
	// overdue(跨期：按规则超期未跟进) | revisit(回访：最新跟进的提醒已到期) |
	// pool(公海)
	Scope string
	Me    uint64
	// Access 数据权限硬隔离；零值=不限制（内部调用）。
	Access UserScope
	Page
}

func (s *Store) ListCustomers(tenantID uint64, f CustomerFilter) ([]model.Customer, int64, error) {
	q := tenantQ(s.db.Model(&model.Customer{}), tenantID)
	q = applyCustomerAccess(q, f.Access)
	if f.Level != "" {
		q = q.Where("level = ?", f.Level)
	}
	if f.Owner > 0 {
		q = q.Where("owner_user_id = ?", f.Owner)
	}
	now := time.Now()
	switch f.Scope {
	case "mine":
		q = q.Where("owner_user_id = ?", f.Me)
	case "pool":
		q = q.Where("owner_user_id = 0")
	case "today":
		todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		q = q.Where("owner_user_id = ?", f.Me).
			Where("created_at >= ? OR id IN (?)", todayStart,
				tenantQ(s.db.Model(&model.AssignLog{}), tenantID).
					Select("target_id").
					Where("target_type = ? AND to_owner = ? AND action IN ? AND created_at >= ?",
						"customer", f.Me, []string{"assign", "claim"}, todayStart))
	case "revisit":
		// 最新一条跟进带 next_at 且已到期（回访提醒）
		q = q.Where("owner_user_id = ?", f.Me).
			Where("id IN (?)", tenantQ(s.db.Model(&model.FollowUp{}), tenantID).
				Select("target_id").
				Where("target_type = ? AND next_at IS NOT NULL AND next_at <= ?", "customer", now).
				Where("id IN (SELECT MAX(f2.id) FROM crm_followups f2 WHERE f2.target_type = 'customer' GROUP BY f2.target_id)"))
	case "overdue":
		// 跨期：按公海规则口径已超期（下轮引擎将回收）
		rules, err := s.GetPoolRules(tenantID)
		if err != nil {
			return nil, 0, err
		}
		q = q.Where("owner_user_id = ?", f.Me).Where(s.overdueCond(rules, now))
	}
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("name LIKE ? OR phone LIKE ?", like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset, limit := f.clamp()
	var list []model.Customer
	err := q.Order("id DESC").Offset(offset).Limit(limit).Find(&list).Error
	return list, total, err
}

// CustomerCard 客户 + 卡片视图聚合指标（商机数/进行中商机/合同数/合同额）。
type CustomerCard struct {
	model.Customer
	OppCount      int64 `json:"opp_count"`
	OpenOppCount  int64 `json:"open_opp_count"`
	ContractCount int64 `json:"contract_count"`
	ContractCents int64 `json:"contract_cents"`
}

// ListCustomerCards 在 ListCustomers 基础上批量补聚合指标（单查询按 IN 聚合，
// 避免逐行 N+1）。
func (s *Store) ListCustomerCards(tenantID uint64, f CustomerFilter) ([]CustomerCard, int64, error) {
	list, total, err := s.ListCustomers(tenantID, f)
	if err != nil {
		return nil, 0, err
	}
	cards := make([]CustomerCard, len(list))
	ids := make([]uint64, len(list))
	idx := map[uint64]int{}
	for i, c := range list {
		cards[i] = CustomerCard{Customer: c}
		ids[i] = c.ID
		idx[c.ID] = i
	}
	if len(ids) == 0 {
		return cards, total, nil
	}
	// 商机聚合：总数 + 进行中数
	type oppAgg struct {
		CustomerID uint64
		Total      int64
		Open       int64
	}
	var oa []oppAgg
	tenantQ(s.db.Model(&model.Opportunity{}), tenantID).
		Where("customer_id IN ?", ids).
		Select("customer_id, COUNT(*) AS total, SUM(CASE WHEN stage IN ? THEN 1 ELSE 0 END) AS open",
			[]string{"initial", "demand", "proposal", "negotiation"}).
		Group("customer_id").Scan(&oa)
	for _, a := range oa {
		if i, ok := idx[a.CustomerID]; ok {
			cards[i].OppCount, cards[i].OpenOppCount = a.Total, a.Open
		}
	}
	// 合同聚合：数量 + 生效合同额
	type ctAgg struct {
		CustomerID uint64
		Cnt        int64
		Cents      int64
	}
	var ca []ctAgg
	tenantQ(s.db.Model(&model.Contract{}), tenantID).
		Where("customer_id IN ?", ids).
		Select("customer_id, COUNT(*) AS cnt, COALESCE(SUM(CASE WHEN status = ? THEN amount_cents ELSE 0 END),0) AS cents", "active").
		Group("customer_id").Scan(&ca)
	for _, a := range ca {
		if i, ok := idx[a.CustomerID]; ok {
			cards[i].ContractCount, cards[i].ContractCents = a.Cnt, a.Cents
		}
	}
	return cards, total, nil
}

func (s *Store) CreateCustomer(ctx context.Context, c *model.Customer) error {
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("客户名称必填")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Create(c).Error
	})
}

func (s *Store) GetCustomer(id, tenantID uint64) (*model.Customer, error) {
	var c model.Customer
	err := tenantQ(s.db, tenantID).First(&c, id).Error
	return &c, err
}

func (s *Store) UpdateCustomer(ctx context.Context, c *model.Customer) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Save(c).Error
	})
}

// CustomerDetail 聚合详情：联系人 + 商机 + 合同 + 最近跟进。
type CustomerDetail struct {
	Customer      model.Customer      `json:"customer"`
	Contacts      []model.Contact     `json:"contacts"`
	Opportunities []model.Opportunity `json:"opportunities"`
	Contracts     []model.Contract    `json:"contracts"`
	FollowUps     []model.FollowUp    `json:"follow_ups"`
	Tags          []model.Tag         `json:"tags"`
}

func (s *Store) GetCustomerDetail(id, tenantID uint64) (*CustomerDetail, error) {
	c, err := s.GetCustomer(id, tenantID)
	if err != nil {
		return nil, err
	}
	d := &CustomerDetail{Customer: *c}
	// 标签（M2M）；Association.Find 由当前客户的关联键约束，并非无条件表扫描。
	//nolint:unbounded-find
	s.db.WithContext(context.Background()).Model(c).Association("Tags").Find(&d.Tags)
	// 关联列表加上限：详情页只展示最近若干条，全量加载会被单个大客户拖垮
	// （跟进已限 20，这里对齐）。需要完整清单走对应列表接口分页。
	const detailAssocLimit = 100
	if err := tenantQ(s.db, tenantID).Where("customer_id = ?", id).
		Order("is_primary DESC, id ASC").Limit(detailAssocLimit).Find(&d.Contacts).Error; err != nil {
		return nil, err
	}
	if err := tenantQ(s.db, tenantID).Where("customer_id = ?", id).
		Order("id DESC").Limit(detailAssocLimit).Find(&d.Opportunities).Error; err != nil {
		return nil, err
	}
	if err := tenantQ(s.db, tenantID).Where("customer_id = ?", id).
		Order("id DESC").Limit(detailAssocLimit).Find(&d.Contracts).Error; err != nil {
		return nil, err
	}
	if err := tenantQ(s.db, tenantID).Where("target_type = ? AND target_id = ?", "customer", id).
		Order("id DESC").Limit(20).Find(&d.FollowUps).Error; err != nil {
		return nil, err
	}
	return d, nil
}

func (s *Store) CreateContact(ctx context.Context, c *model.Contact) error {
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("联系人姓名必填")
	}
	if _, err := s.GetCustomer(c.CustomerID, c.TenantID); err != nil {
		return errors.New("客户不存在")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Create(c).Error
	})
}

func (s *Store) DeleteContact(ctx context.Context, id, tenantID uint64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tenantQ(tx, tenantID).Delete(&model.Contact{}, id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}
