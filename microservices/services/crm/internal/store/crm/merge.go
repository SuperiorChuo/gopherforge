package store

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-admin-kit/services/crm/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/fullread"
	"gorm.io/gorm"
)

// ---------- 客户合并去重 ----------
//
// 重复检测：号码归一后尾 11 位相同，或名称去空格全同。
// 合并：全部子实体（联系人/商机/合同/回款经合同、跟进/流转日志/名片回填/
// IM 访客）迁到主客户，主客户空字段用副客户补齐，副客户物理删除，
// crm_assign_logs 记 merge 留痕。

// DupGroup 一组疑似重复客户。
type DupGroup struct {
	Key       string           `json:"key"`  // phone:<尾11位> | name:<归一名>
	Kind      string           `json:"kind"` // phone | name
	Customers []model.Customer `json:"customers"`
}

// FindDuplicateGroups 检测租户内重复客户组（每组 ≥2 个）。SQL 先找有限重复键，
// 再只加载候选客户，避免把租户全表装入 Go 内存。
func (s *Store) FindDuplicateGroups(tenantID uint64, limit int) ([]DupGroup, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	phoneExpr, phoneDigitsExpr, nameExpr := duplicateKeyExprs(s.db.Dialector.Name())
	phoneKeys, err := s.findDuplicateKeys(tenantID, phoneExpr,
		"LENGTH("+phoneDigitsExpr+") >= ?", []any{6}, nil, limit)
	if err != nil {
		return nil, err
	}
	phoneCustomers, err := s.customersForDuplicateKeys(tenantID, phoneExpr, phoneKeys)
	if err != nil {
		return nil, err
	}
	groups := make([]DupGroup, 0, limit)
	seen := map[uint64]bool{} // 已进电话组的客户不再按名字重复报
	for _, key := range phoneKeys {
		list := phoneCustomers[key]
		groups = append(groups, DupGroup{Key: "phone:" + key, Kind: "phone", Customers: list})
		for _, customer := range list {
			seen[customer.ID] = true
		}
	}
	remaining := limit - len(groups)
	if remaining <= 0 {
		return groups, nil
	}
	seenIDs := make([]uint64, 0, len(seen))
	for id := range seen {
		seenIDs = append(seenIDs, id)
	}
	nameKeys, err := s.findDuplicateKeys(tenantID, nameExpr, nameExpr+" <> ''", nil, seenIDs, remaining)
	if err != nil {
		return nil, err
	}
	nameCustomers, err := s.customersForDuplicateKeys(tenantID, nameExpr, nameKeys)
	if err != nil {
		return nil, err
	}
	for _, key := range nameKeys {
		groups = append(groups, DupGroup{Key: "name:" + key, Kind: "name", Customers: nameCustomers[key]})
	}
	return groups, nil
}

func duplicateKeyExprs(dialect string) (phone, phoneDigits, name string) {
	name = "REPLACE(TRIM(COALESCE(name, '')), ' ', '')"
	if dialect == "postgres" {
		phoneDigits = "REGEXP_REPLACE(COALESCE(phone, ''), '[^0-9]', '', 'g')"
		return "RIGHT(" + phoneDigits + ", 11)", phoneDigits, name
	}
	phoneDigits = "COALESCE(phone, '')"
	for _, old := range []string{" ", "+", "-", "(", ")", "."} {
		phoneDigits = "REPLACE(" + phoneDigits + ", '" + old + "', '')"
	}
	return "SUBSTR(" + phoneDigits + ", -11)", phoneDigits, name
}

func (s *Store) findDuplicateKeys(tenantID uint64, expr, condition string, args []any, excludeIDs []uint64, limit int) ([]string, error) {
	type keyRow struct {
		Key string `gorm:"column:dup_key"`
	}
	q := s.db.Model(&model.Customer{}).Select(expr+" AS dup_key").Where("tenant_id = ?", tenantID)
	if condition != "" {
		q = q.Where(condition, args...)
	}
	if len(excludeIDs) > 0 {
		q = q.Where("id NOT IN ?", excludeIDs)
	}
	var rows []keyRow
	err := q.Group(expr).Having("COUNT(*) >= 2").Order("MIN(id) ASC").Limit(limit).Scan(&rows).Error
	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, row.Key)
	}
	return keys, err
}

func (s *Store) customersForDuplicateKeys(tenantID uint64, expr string, keys []string) (map[string][]model.Customer, error) {
	out := make(map[string][]model.Customer, len(keys))
	if len(keys) == 0 {
		return out, nil
	}
	customers, err := fullread.All[model.Customer](tenantQ(s.db, tenantID).
		Where(expr+" IN ?", keys).Order("id ASC"))
	if err != nil {
		return nil, err
	}
	phoneExpr, _, nameExpr := duplicateKeyExprs(s.db.Dialector.Name())
	for _, customer := range customers {
		key := strings.ReplaceAll(strings.TrimSpace(customer.Name), " ", "")
		if expr == phoneExpr {
			key = normalizePhone(customer.Phone)
			if len(key) > 11 {
				key = key[len(key)-11:]
			}
		} else if expr != nameExpr {
			return nil, errors.New("unsupported duplicate key expression")
		}
		out[key] = append(out[key], customer)
	}
	return out, nil
}

const MaxMergeSourceIDs = 100

// NormalizeMergeSourceIDs validates and deduplicates merge input before any query.
func NormalizeMergeSourceIDs(primaryID uint64, sourceIDs []uint64) ([]uint64, error) {
	if len(sourceIDs) == 0 {
		return nil, errors.New("未选择要合并的客户")
	}
	if len(sourceIDs) > MaxMergeSourceIDs {
		return nil, fmt.Errorf("一次最多合并 %d 个客户", MaxMergeSourceIDs)
	}
	seen := make(map[uint64]struct{}, len(sourceIDs))
	out := make([]uint64, 0, len(sourceIDs))
	for _, id := range sourceIDs {
		if id == 0 {
			return nil, errors.New("被合并客户 ID 不能为空")
		}
		if id == primaryID {
			return nil, errors.New("主客户不能同时是被合并方")
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}

// GetCustomersByIDs loads all merge participants with one tenant-scoped query.
func (s *Store) GetCustomersByIDs(tenantID uint64, ids []uint64) ([]model.Customer, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var customers []model.Customer
	// Unique primary keys bound the result by input cardinality, not an arbitrary cap.
	err := tenantQ(s.db, tenantID).Where("id IN ?", ids).Order("id ASC").Limit(len(ids)).Find(&customers).Error
	return customers, err
}

// MergeCustomers 把 srcIDs 合并进 primaryID（同租户、事务）。
// 返回迁移统计。所有参与客户须先经 handler 层 scope 校验。
func (s *Store) MergeCustomers(tenantID, primaryID uint64, srcIDs []uint64, byUserID uint64) (map[string]int64, error) {
	var err error
	srcIDs, err = NormalizeMergeSourceIDs(primaryID, srcIDs)
	if err != nil {
		return nil, err
	}
	stats := map[string]int64{}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var primary model.Customer
		if err := tenantQ(tx, tenantID).First(&primary, primaryID).Error; err != nil {
			return errors.New("主客户不存在")
		}
		var srcs []model.Customer
		if err := tenantQ(tx, tenantID).Where("id IN ?", srcIDs).Limit(len(srcIDs)).Find(&srcs).Error; err != nil {
			return err
		}
		if len(srcs) != len(srcIDs) {
			return errors.New("部分被合并客户不存在")
		}

		// 子实体迁移（按表逐一 UPDATE customer_id）
		type mig struct {
			table string
			col   string
		}
		for _, m := range []mig{
			{"crm_contacts", "customer_id"},
			{"crm_opportunities", "customer_id"},
			{"crm_contracts", "customer_id"},
		} {
			r := tx.Table(m.table).Where("tenant_id = ? AND "+m.col+" IN ?", tenantID, srcIDs).
				Update(m.col, primaryID)
			if r.Error != nil {
				return r.Error
			}
			stats[m.table] = r.RowsAffected
		}
		// 跟进与流转日志（target_type=customer）
		r := tx.Table("crm_followups").
			Where("tenant_id = ? AND target_type = ? AND target_id IN ?", tenantID, "customer", srcIDs).
			Update("target_id", primaryID)
		if r.Error != nil {
			return r.Error
		}
		stats["crm_followups"] = r.RowsAffected
		r = tx.Table("crm_assign_logs").
			Where("tenant_id = ? AND target_type = ? AND target_id IN ?", tenantID, "customer", srcIDs).
			Update("target_id", primaryID)
		if r.Error != nil {
			return r.Error
		}
		stats["crm_assign_logs"] = r.RowsAffected
		// 名片转化回填指向
		r = tx.Table("crm_leads").
			Where("tenant_id = ? AND customer_id IN ?", tenantID, srcIDs).
			Update("customer_id", primaryID)
		if r.Error != nil {
			return r.Error
		}
		stats["crm_leads"] = r.RowsAffected

		// 主客户空字段用副客户补齐（第一个有值的副客户胜出）
		changed := false
		for _, src := range srcs {
			if primary.Phone == "" && src.Phone != "" {
				primary.Phone, changed = src.Phone, true
			}
			if primary.Industry == "" && src.Industry != "" {
				primary.Industry, changed = src.Industry, true
			}
			if primary.Address == "" && src.Address != "" {
				primary.Address, changed = src.Address, true
			}
			if primary.IMVisitorID == 0 && src.IMVisitorID != 0 {
				primary.IMVisitorID, changed = src.IMVisitorID, true
			}
			if src.Note != "" {
				primary.Note = strings.TrimSpace(primary.Note + "\n[合并自 #" + fmt.Sprint(src.ID) + "] " + src.Note)
				changed = true
			}
		}
		// 主客户在公海而副客户有归属 → 继承第一个归属，避免合并后掉线索
		if primary.OwnerUserID == 0 {
			for _, src := range srcs {
				if src.OwnerUserID > 0 {
					primary.OwnerUserID = src.OwnerUserID
					primary.PooledAt = nil
					changed = true
					break
				}
			}
		}
		if changed {
			if err := tx.Save(&primary).Error; err != nil {
				return err
			}
		}

		// 副客户删除 + merge 留痕
		if err := tx.Where("tenant_id = ? AND id IN ?", tenantID, srcIDs).
			Delete(&model.Customer{}).Error; err != nil {
			return err
		}
		now := time.Now()
		for _, src := range srcs {
			log := model.AssignLog{
				TenantID: tenantID, TargetType: "customer", TargetID: primaryID,
				Action: "merge", FromOwner: src.OwnerUserID, ToOwner: primary.OwnerUserID,
				ByUserID: byUserID,
				Note:     fmt.Sprintf("合并客户 #%d（%s）", src.ID, src.Name),
			}
			log.CreatedAt = now
			if err := tx.Create(&log).Error; err != nil {
				return err
			}
		}
		// 合并算一次跟进活动，刷新回收计时
		if err := tx.Model(&model.Customer{}).
			Where("tenant_id = ? AND id = ?", tenantID, primaryID).
			Update("last_follow_at", now).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return stats, nil
}
