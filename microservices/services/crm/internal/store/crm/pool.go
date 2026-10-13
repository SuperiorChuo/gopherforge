package store

// P1 名片分配 + P2 公海规则引擎。
// 组织结构（部门/用户）读同库 system 表，best-effort：测试库无表时返回空。

import (
	"context"
	"errors"
	"time"

	"github.com/go-admin-kit/services/crm/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/fullread"
	"gorm.io/gorm"
)

// ---------- 组织数据 ----------

type OrgUser struct {
	ID           uint64 `json:"id"`
	Name         string `json:"name"`
	DepartmentID uint64 `json:"department_id"`
}

type OrgDept struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
}

// Org 返回本租户的部门与用户清单供分配选择器（同库读 system 表，失败静默为空）。
// Phase 2C：组织架构改走 identity owner API（/internal/org），不再直查 departments/users。
func (s *Store) Org(ctx context.Context, tenantID uint64) ([]OrgDept, []OrgUser) {
	return s.orgData(ctx, tenantID)
}

func (s *Store) logAssign(tx *gorm.DB, l model.AssignLog) {
	_ = tx.Create(&l).Error
}

// ---------- 名片分配（P1） ----------

// AssignLeads 批量分配：给用户（owner>0，可同时落部门）或进部门池
// （dept>0 owner=0）。已转化名片跳过。返回实际分配数。
// 分配目标做本租户归属校验：owner 必须是本租户启用用户、dept 必须是本租户
// 部门（共享 users/departments 表按 tenant_id 过滤），防跨租户注水。
func (s *Store) AssignLeads(ctx context.Context, tenantID uint64, ids []uint64, deptID, ownerID, byUser uint64) (int, error) {
	if len(ids) == 0 {
		return 0, errors.New("请选择名片")
	}
	if deptID == 0 && ownerID == 0 {
		return 0, errors.New("请选择分配目标（部门或成员）")
	}
	// Phase 2C：分配目标校验改走 identity owner API，不再直查共享 users/departments 表。
	if ownerID > 0 && !s.userInTenant(ctx, ownerID, tenantID) {
		return 0, errors.New("目标成员不存在或不属于本租户")
	}
	if deptID > 0 && !s.deptExists(ctx, deptID, tenantID) {
		return 0, errors.New("目标部门不存在或不属于本租户")
	}
	n := 0
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var leads []model.Lead
		if err := tx.Where("tenant_id = ? AND id IN ? AND status <> ?", tenantID, ids, "converted").
			Limit(len(ids)).Find(&leads).Error; err != nil {
			return err
		}
		for i := range leads {
			l := &leads[i]
			from := l.OwnerUserID
			l.OwnerUserID = ownerID
			l.DepartmentID = deptID
			if err := tx.Save(l).Error; err != nil {
				return err
			}
			s.logAssign(tx, model.AssignLog{
				TenantID: tenantID, TargetType: "lead", TargetID: l.ID, Action: "assign",
				FromOwner: from, ToOwner: ownerID, ToDept: deptID, ByUserID: byUser,
			})
			n++
		}
		return nil
	})
	return n, err
}

// ClaimLead 成员从部门池/未分配池领取名片。
func (s *Store) ClaimLead(id, tenantID, byUser uint64) (*model.Lead, error) {
	var l model.Lead
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tenantQ(tx, tenantID).First(&l, id).Error; err != nil {
			return errors.New("名片不存在")
		}
		if l.OwnerUserID != 0 {
			return errors.New("该名片已有归属")
		}
		if l.Status == "converted" {
			return errors.New("该名片已转化")
		}
		from := l.OwnerUserID
		l.OwnerUserID = byUser
		if err := tx.Save(&l).Error; err != nil {
			return err
		}
		s.logAssign(tx, model.AssignLog{
			TenantID: tenantID, TargetType: "lead", TargetID: l.ID, Action: "claim",
			FromOwner: from, ToOwner: byUser, ToDept: l.DepartmentID, ByUserID: byUser,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// ---------- 公海规则引擎（P2） ----------

// GetPoolRules 返回租户全部规则；无配置时补一条默认档（不落库）。
func (s *Store) GetPoolRules(tenantID uint64) ([]model.PoolRule, error) {
	// Read every persisted rule, including legacy levels outside the current write API.
	rules, err := fullread.All[model.PoolRule](tenantQ(s.db, tenantID).Order("level ASC"))
	if err != nil {
		return nil, err
	}
	hasDefault := false
	for _, r := range rules {
		if r.Level == "*" {
			hasDefault = true
		}
	}
	if !hasDefault {
		rules = append([]model.PoolRule{{
			TenantID: tenantID, Level: "*", IdleDays: 30, ProtectActive: true, Enabled: true,
		}}, rules...)
	}
	return rules, nil
}

// SavePoolRule upsert 一档规则（tenant+level 唯一）。
func (s *Store) SavePoolRule(r *model.PoolRule) error {
	if r.Level == "" {
		r.Level = "*"
	}
	if r.Level != "*" && r.Level != "A" && r.Level != "B" && r.Level != "C" {
		return errors.New("无效级别")
	}
	if r.IdleDays < 0 || r.IdleDays > 3650 {
		return errors.New("回收天数需在 0-3650 之间")
	}
	var existing model.PoolRule
	err := s.db.Where("tenant_id = ? AND level = ?", r.TenantID, r.Level).First(&existing).Error
	if err == nil {
		existing.IdleDays = r.IdleDays
		existing.ProtectActive = r.ProtectActive
		existing.Enabled = r.Enabled
		existing.UpdatedBy = r.UpdatedBy
		*r = existing
		return s.db.Save(&existing).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return s.db.Create(r).Error
}

// effectiveIdleDays 计算某级别客户的回收天数：级别档优先，缺省用 * 档；
// 返回 0 表示该级别不回收。
func effectiveIdleDays(rules []model.PoolRule, level string) (int, bool) {
	var def *model.PoolRule
	for i := range rules {
		r := &rules[i]
		if r.Level == level {
			if !r.Enabled {
				return 0, r.ProtectActive
			}
			return r.IdleDays, r.ProtectActive
		}
		if r.Level == "*" {
			def = r
		}
	}
	if def == nil || !def.Enabled {
		return 0, true
	}
	return def.IdleDays, def.ProtectActive
}

// overdueCond 生成"按级别规则已超期未跟进"的查询条件（跨期视图与引擎同口径）。
func (s *Store) overdueCond(rules []model.PoolRule, now time.Time) *gorm.DB {
	cond := s.db.Where("1 = 0")
	levels := []string{"A", "B", "C"}
	for _, lv := range levels {
		days, _ := effectiveIdleDays(rules, lv)
		if days <= 0 {
			continue
		}
		cutoff := now.Add(-time.Duration(days) * 24 * time.Hour)
		cond = cond.Or("level = ? AND COALESCE(last_follow_at, created_at) < ?", lv, cutoff)
	}
	if days, _ := effectiveIdleDays(rules, "__other__"); days > 0 {
		cutoff := now.Add(-time.Duration(days) * 24 * time.Hour)
		cond = cond.Or("level NOT IN ? AND COALESCE(last_follow_at, created_at) < ?", levels, cutoff)
	}
	return cond
}

// RecycledCustomer is a pool-engine recycle result (for notify).
type RecycledCustomer struct {
	ID        uint64
	Name      string
	FromOwner uint64
	TenantID  uint64
}

// RecycleIdleCustomers 引擎扫描：owner>0 且按级别规则超期未跟进的客户回公海。
// 返回回收明细（含原归属人，便于站内信）。
//
// 查询量：超期判定下推 SQL（overdueCond，与跨期视图同口径）1 条 + 保护判定
// 分组聚合 2 条 + 批量 UPDATE 1 条 + 日志批量 INSERT 1 条，与候选数无关；
// 原实现是「拉 600 行 + 每行 2 次 COUNT + 每行 Save + 每行 INSERT」。
func (s *Store) RecycleIdleCustomers(tenantID uint64, limit int) ([]RecycledCustomer, error) {
	plan, err := s.PlanRecycle(tenantID, limit)
	if err != nil {
		return nil, err
	}
	if len(plan) == 0 {
		return nil, nil
	}
	return s.ApplyRecycle(tenantID, plan)
}

// PlanRecycle 只读阶段：算出本轮该回收的客户（超期判定下推 SQL、保护判定
// 一次分组）。手动触发接口用它算出「已受理多少」后再异步落库。
func (s *Store) PlanRecycle(tenantID uint64, limit int) ([]RecycledCustomer, error) {
	rules, err := s.GetPoolRules(tenantID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 200
	}
	now := time.Now()
	// 多取一些候选：保护规则会筛掉一部分，仍尽量凑满 limit（上限兜住内存）
	fetch := limit * 2
	if fetch > 1000 {
		fetch = 1000
	}
	var candidates []model.Customer
	if err := tenantQ(s.db, tenantID).Where("owner_user_id > 0").
		Where(s.overdueCond(rules, now)).
		Order("id ASC").Limit(fetch).Find(&candidates).Error; err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	// 保护判定：只对 protect_active 档的候选做一次分组查询
	protectIDs := make([]uint64, 0, len(candidates))
	protectOf := make(map[uint64]bool, len(candidates))
	for i := range candidates {
		c := &candidates[i]
		if _, protect := effectiveIdleDays(rules, c.Level); protect {
			protectOf[c.ID] = true
			protectIDs = append(protectIDs, c.ID)
		}
	}
	active := s.customersWithActiveBusiness(tenantID, protectIDs)

	recycled := make([]RecycledCustomer, 0, limit)
	for i := range candidates {
		c := &candidates[i]
		if protectOf[c.ID] && active[c.ID] {
			continue
		}
		recycled = append(recycled, RecycledCustomer{
			ID: c.ID, Name: c.Name, FromOwner: c.OwnerUserID, TenantID: tenantID,
		})
		if len(recycled) >= limit {
			break
		}
	}
	return recycled, nil
}

// ApplyRecycle 写阶段：按原归属人分组批量回池 + 批量留痕（查询数 = 归属人数
// + 2，与回收条数无关）。租户条件保留；UPDATE 带 owner_user_id = 计划时的
// 归属人——计划算出后被人领走或已易主的行不满足条件，自动跳过，不会被误清。
// 返回真正落库的客户明细（留痕与站内信都只针对这批）。
func (s *Store) ApplyRecycle(tenantID uint64, plan []RecycledCustomer) ([]RecycledCustomer, error) {
	if len(plan) == 0 {
		return nil, nil
	}
	byOwner := map[uint64][]uint64{}
	for _, r := range plan {
		byOwner[r.FromOwner] = append(byOwner[r.FromOwner], r.ID)
	}
	now := time.Now()
	for owner, ids := range byOwner {
		if err := tenantQ(s.db.Model(&model.Customer{}), tenantID).
			Where("id IN ? AND owner_user_id = ?", ids, owner).
			UpdateColumns(map[string]any{
				"owner_user_id": 0, "pooled_at": now, "updated_at": now,
			}).Error; err != nil {
			return nil, err
		}
	}
	// 实际入池的行（跳过并发易主的）：一条查询确认，留痕与通知都按它走
	var appliedIDs []uint64
	ids := make([]uint64, 0, len(plan))
	for _, r := range plan {
		ids = append(ids, r.ID)
	}
	if err := tenantQ(s.db.Model(&model.Customer{}), tenantID).
		Where("id IN ? AND owner_user_id = 0", ids).
		Pluck("id", &appliedIDs).Error; err != nil {
		return nil, err
	}
	applied := make(map[uint64]bool, len(appliedIDs))
	for _, id := range appliedIDs {
		applied[id] = true
	}
	out := make([]RecycledCustomer, 0, len(appliedIDs))
	logs := make([]model.AssignLog, 0, len(appliedIDs))
	for _, r := range plan {
		if !applied[r.ID] {
			continue
		}
		out = append(out, r)
		logs = append(logs, model.AssignLog{
			TenantID: tenantID, TargetType: "customer", TargetID: r.ID, Action: "recycle",
			FromOwner: r.FromOwner, Note: "超期未跟进自动回收",
		})
	}
	if len(logs) == 0 {
		return nil, nil
	}
	if err := s.db.CreateInBatches(logs, 200).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// customersWithActiveBusiness 批量判定「有进行中商机或生效合同」（保护规则）：
// 两条分组查询替代逐客户 2 次 COUNT。
func (s *Store) customersWithActiveBusiness(tenantID uint64, customerIDs []uint64) map[uint64]bool {
	out := map[uint64]bool{}
	if len(customerIDs) == 0 {
		return out
	}
	var oppIDs []uint64
	tenantQ(s.db.Model(&model.Opportunity{}), tenantID).
		Where("customer_id IN ? AND stage IN ?", customerIDs,
			[]string{"initial", "demand", "proposal", "negotiation"}).
		Distinct("customer_id").Pluck("customer_id", &oppIDs)
	for _, id := range oppIDs {
		out[id] = true
	}
	var ctIDs []uint64
	tenantQ(s.db.Model(&model.Contract{}), tenantID).
		Where("customer_id IN ? AND status = ?", customerIDs, "active").
		Distinct("customer_id").Pluck("customer_id", &ctIDs)
	for _, id := range ctIDs {
		out[id] = true
	}
	return out
}

// ClaimCustomer 从公海领取客户。
func (s *Store) ClaimCustomer(id, tenantID, byUser uint64) (*model.Customer, error) {
	var c model.Customer
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tenantQ(tx, tenantID).First(&c, id).Error; err != nil {
			return errors.New("客户不存在")
		}
		if c.OwnerUserID != 0 {
			return errors.New("该客户已有归属")
		}
		now := time.Now()
		c.OwnerUserID = byUser
		c.LastFollowAt = &now
		c.PooledAt = nil
		if err := tx.Save(&c).Error; err != nil {
			return err
		}
		s.logAssign(tx, model.AssignLog{
			TenantID: tenantID, TargetType: "customer", TargetID: c.ID, Action: "claim",
			ToOwner: byUser, ByUserID: byUser,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ReleaseCustomer 手动释放客户到公海。
func (s *Store) ReleaseCustomer(id, tenantID, byUser uint64, note string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var c model.Customer
		if err := tenantQ(tx, tenantID).First(&c, id).Error; err != nil {
			return errors.New("客户不存在")
		}
		if c.OwnerUserID == 0 {
			return errors.New("客户已在公海")
		}
		from := c.OwnerUserID
		now := time.Now()
		c.OwnerUserID = 0
		c.PooledAt = &now
		if err := tx.Save(&c).Error; err != nil {
			return err
		}
		s.logAssign(tx, model.AssignLog{
			TenantID: tenantID, TargetType: "customer", TargetID: c.ID, Action: "release",
			FromOwner: from, ByUserID: byUser, Note: note,
		})
		return nil
	})
}

// TouchCustomerFollow 跟进落库后刷新客户活跃时间（公海引擎的计时基准）。
func (s *Store) TouchCustomerFollow(customerID uint64) {
	now := time.Now()
	_ = s.db.Model(&model.Customer{}).Where("id = ?", customerID).
		Update("last_follow_at", now).Error
}

// TenantIDs 返回存在客户数据的租户清单（回收循环用）。
func (s *Store) TenantIDs() ([]uint64, error) {
	var ids []uint64
	err := s.db.Model(&model.Customer{}).Distinct("tenant_id").Pluck("tenant_id", &ids).Error
	return ids, err
}

// PoolStats 公海规则页实时态：当前公海客户数、今日已回收数、
// 按现行规则的预估待回收名单（下轮扫描会收走的）。
type PoolStats struct {
	PoolCount     int64            `json:"pool_count"`
	TodayRecycled int64            `json:"today_recycled"`
	PreviewCount  int64            `json:"preview_count"`
	Preview       []PoolPreviewRow `json:"preview"`
}

type PoolPreviewRow struct {
	ID          uint64 `json:"id"`
	Name        string `json:"name"`
	Level       string `json:"level"`
	OwnerUserID uint64 `json:"owner_user_id"`
	Protected   bool   `json:"protected"`
}

// PoolStatsFor 汇总公海实时态；preview 取按规则超期的客户（不含保护判断的
// 归属客户），逐条标注是否会被保护规则拦下。
func (s *Store) PoolStatsFor(tenantID uint64, previewLimit int) (*PoolStats, error) {
	if previewLimit <= 0 || previewLimit > 100 {
		previewLimit = 20
	}
	out := &PoolStats{}
	tenantQ(s.db.Model(&model.Customer{}), tenantID).Where("owner_user_id = 0").Count(&out.PoolCount)

	todayStart := time.Now().Truncate(24 * time.Hour)
	tenantQ(s.db.Model(&model.AssignLog{}), tenantID).
		Where("target_type = ? AND action = ? AND created_at >= ?", "customer", "recycle", todayStart).
		Count(&out.TodayRecycled)

	rules, err := s.GetPoolRules(tenantID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var overdue []model.Customer
	tenantQ(s.db, tenantID).Where("owner_user_id > 0").Where(s.overdueCond(rules, now)).
		Order("COALESCE(last_follow_at, created_at) ASC").Limit(previewLimit).Find(&overdue)
	// 保护标注批量判定（原逐行 2 次 COUNT，20 行 = 40 次往返）
	protectIDs := make([]uint64, 0, len(overdue))
	for i := range overdue {
		if _, protect := effectiveIdleDays(rules, overdue[i].Level); protect {
			protectIDs = append(protectIDs, overdue[i].ID)
		}
	}
	active := s.customersWithActiveBusiness(tenantID, protectIDs)
	for i := range overdue {
		c := &overdue[i]
		_, protect := effectiveIdleDays(rules, c.Level)
		row := PoolPreviewRow{ID: c.ID, Name: c.Name, Level: c.Level, OwnerUserID: c.OwnerUserID}
		if protect && active[c.ID] {
			row.Protected = true
		}
		out.Preview = append(out.Preview, row)
	}
	out.PreviewCount = int64(len(out.Preview))
	return out, nil
}

// ListAssignLogs 目标的归属流转记录。
func (s *Store) ListAssignLogs(tenantID uint64, targetType string, targetID uint64, limit int) ([]model.AssignLog, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var logs []model.AssignLog
	err := tenantQ(s.db, tenantID).
		Where("target_type = ? AND target_id = ?", targetType, targetID).
		Order("id DESC").Limit(limit).Find(&logs).Error
	return logs, err
}
