package store

// 聚合改写回归：合同回款进度、销售看板漏斗、公海回收引擎由 N+1 改成分组聚合 /
// 批量写后，结果必须与原逐条实现逐字段一致；数据范围（Access UserScope）与
// 租户过滤在改写后仍生效；组合索引 DDL 幂等。

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/crm/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var storeDBSeq atomic.Int64

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dsn := fmt.Sprintf("file:crmstore%d?mode=memory&cache=shared", storeDBSeq.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("sqlite: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	s, err := NewWithDB(db)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	return s
}

// ---------- 参照实现（改写前的逐条逻辑，逐字段比对用） ----------

// naivePaidCents 原实现：分页后每行一条 SUM。
func naivePaidCents(s *Store, contractID uint64) int64 {
	var paid int64
	s.db.Model(&model.Payment{}).
		Where("contract_id = ? AND status = ?", contractID, PaymentConfirmed).
		Select("COALESCE(SUM(amount_cents),0)").Scan(&paid)
	return paid
}

// naiveFunnel 原实现：每阶段 COUNT + SUM 两条查询。
func naiveFunnel(s *Store, tenantID uint64, since time.Time) []StageBucket {
	var out []StageBucket
	for _, st := range model.Stages {
		var b StageBucket
		b.Stage = st
		terminalFilter := func(q *gorm.DB) *gorm.DB {
			switch st {
			case "won":
				return q.Where("won_at >= ?", since)
			case "lost":
				return q.Where("lost_at >= ?", since)
			}
			return q
		}
		terminalFilter(tenantQ(s.db.Model(&model.Opportunity{}), tenantID).
			Where("stage = ?", st)).Count(&b.Count)
		terminalFilter(tenantQ(s.db.Model(&model.Opportunity{}), tenantID).
			Where("stage = ?", st)).
			Select("COALESCE(SUM(amount_cents),0)").Scan(&b.AmountCents)
		out = append(out, b)
	}
	return out
}

// naiveHasActiveBusiness 原实现：每客户两条 COUNT。
func naiveHasActiveBusiness(s *Store, customerID, tenantID uint64) bool {
	var n int64
	tenantQ(s.db.Model(&model.Opportunity{}), tenantID).
		Where("customer_id = ? AND stage IN ?", customerID,
			[]string{"initial", "demand", "proposal", "negotiation"}).Count(&n)
	if n > 0 {
		return true
	}
	tenantQ(s.db.Model(&model.Contract{}), tenantID).
		Where("customer_id = ? AND status = ?", customerID, "active").Count(&n)
	return n > 0
}

// ---------- 1. 合同回款进度：一条分组聚合 == 逐行 SUM ----------

func TestListContractsPaidCentsMatchesPerRowSum(t *testing.T) {
	s := newTestStore(t)
	const tenant uint64 = 1
	cust := model.Customer{TenantID: tenant, Name: "客户A", OwnerUserID: 7}
	if err := s.db.Create(&cust).Error; err != nil {
		t.Fatal(err)
	}
	// 10 份合同，回款分布刻意不均：有的没回款、有的多笔、有的混着非 confirmed
	for i := 0; i < 10; i++ {
		c := model.Contract{
			TenantID: tenant, No: fmt.Sprintf("HT-%03d", i), Title: "合同",
			CustomerID: cust.ID, AmountCents: 1000000, Status: "active", OwnerUserID: 7,
		}
		if err := s.db.Create(&c).Error; err != nil {
			t.Fatal(err)
		}
		for j := 0; j <= i%3; j++ {
			if err := s.db.Create(&model.Payment{
				TenantID: tenant, ContractID: c.ID, AmountCents: int64(1000 * (i + 1)),
				PaidAt: time.Now(), Status: PaymentConfirmed,
			}).Error; err != nil {
				t.Fatal(err)
			}
		}
		if i%4 == 0 {
			// 审批中/驳回不计入进度
			for _, st := range []string{PaymentPendingApproval, PaymentRejected} {
				if err := s.db.Create(&model.Payment{
					TenantID: tenant, ContractID: c.ID, AmountCents: 999999,
					PaidAt: time.Now(), Status: st,
				}).Error; err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	rows, total, err := s.ListContracts(tenant, ContractFilter{Page: Page{Page: 1, PageSize: 100}})
	if err != nil {
		t.Fatal(err)
	}
	if total != 10 || len(rows) != 10 {
		t.Fatalf("total=%d rows=%d, want 10/10", total, len(rows))
	}
	for _, r := range rows {
		if want := naivePaidCents(s, r.ID); r.PaidCents != want {
			t.Fatalf("合同 %d 回款 = %d, 逐行 SUM = %d", r.ID, r.PaidCents, want)
		}
	}
	// 没有任何回款的合同必须是 0 而不是被丢行
	empty := model.Contract{
		TenantID: tenant, No: "HT-EMPTY", Title: "无回款", CustomerID: cust.ID,
		AmountCents: 500, Status: "active", OwnerUserID: 7,
	}
	if err := s.db.Create(&empty).Error; err != nil {
		t.Fatal(err)
	}
	rows, _, err = s.ListContracts(tenant, ContractFilter{Page: Page{Page: 1, PageSize: 100}})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range rows {
		if r.ID == empty.ID {
			found = true
			if r.PaidCents != 0 {
				t.Fatalf("无回款合同 paid = %d, want 0", r.PaidCents)
			}
		}
	}
	if !found {
		t.Fatal("无回款合同被聚合改写吞掉了")
	}
}

// 数据范围过滤 + 租户隔离在聚合改写后仍生效。
func TestListContractsAccessAndTenantStillApply(t *testing.T) {
	s := newTestStore(t)
	mk := func(tenant, owner uint64, no string, paid int64) uint64 {
		c := model.Contract{
			TenantID: tenant, No: no, Title: "合同", CustomerID: 1,
			AmountCents: 100000, Status: "active", OwnerUserID: owner,
		}
		if err := s.db.Create(&c).Error; err != nil {
			t.Fatal(err)
		}
		if err := s.db.Create(&model.Payment{
			TenantID: tenant, ContractID: c.ID, AmountCents: paid,
			PaidAt: time.Now(), Status: PaymentConfirmed,
		}).Error; err != nil {
			t.Fatal(err)
		}
		return c.ID
	}
	mine := mk(1, 7, "T1-MINE", 111)
	peer := mk(1, 8, "T1-PEER", 222)
	other := mk(2, 7, "T2-OTHER", 333)

	// self 档：只看得到自己的合同
	rows, total, err := s.ListContracts(1, ContractFilter{
		Access: UserScope{Kind: ScopeSelf, Me: 7}, Page: Page{Page: 1, PageSize: 100},
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(rows) != 1 || rows[0].ID != mine {
		t.Fatalf("self 档 = %+v (total %d), 只应见 %d", rows, total, mine)
	}
	if rows[0].PaidCents != 111 {
		t.Fatalf("self 档回款 = %d, want 111", rows[0].PaidCents)
	}
	// dept 档：树内成员的都能看
	rows, total, err = s.ListContracts(1, ContractFilter{
		Access: UserScope{Kind: ScopeDept, Me: 7, VisibleUserIDs: []uint64{7, 8}},
		Page:   Page{Page: 1, PageSize: 100},
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("dept 档 total = %d, want 2", total)
	}
	for _, r := range rows {
		if r.ID == other {
			t.Fatal("跨租户合同泄漏进列表")
		}
		if r.ID == peer && r.PaidCents != 222 {
			t.Fatalf("同部门合同回款 = %d, want 222", r.PaidCents)
		}
	}
	// 跨租户回款不能被 IN 聚合串进本租户行
	if err := s.db.Create(&model.Payment{
		TenantID: 2, ContractID: mine, AmountCents: 900000,
		PaidAt: time.Now(), Status: PaymentConfirmed,
	}).Error; err != nil {
		t.Fatal(err)
	}
	rows, _, err = s.ListContracts(1, ContractFilter{
		Access: UserScope{Kind: ScopeSelf, Me: 7}, Page: Page{Page: 1, PageSize: 100},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].PaidCents != 111 {
		t.Fatalf("他租户回款串入本租户合同：paid = %d, want 111", rows[0].PaidCents)
	}
}

// ---------- 2. 销售看板漏斗：一条分组 == 每阶段两条 ----------

func TestStatsFunnelMatchesPerStageQueries(t *testing.T) {
	s := newTestStore(t)
	const tenant uint64 = 1
	since := time.Now().AddDate(0, 0, -30)
	old := time.Now().AddDate(0, 0, -90) // 区间外的终态

	mk := func(stage string, amount int64, won, lost *time.Time) {
		o := model.Opportunity{
			TenantID: tenant, CustomerID: 1, Name: "商机", AmountCents: amount,
			Stage: stage, OwnerUserID: 7, WonAt: won, LostAt: lost,
		}
		if err := s.db.Create(&o).Error; err != nil {
			t.Fatal(err)
		}
	}
	recent := time.Now().AddDate(0, 0, -3)
	for _, st := range []string{"initial", "demand", "proposal", "negotiation"} {
		mk(st, 10000, nil, nil)
		mk(st, 20000, nil, nil)
	}
	mk("won", 500000, &recent, nil) // 区间内
	mk("won", 600000, &recent, nil)
	mk("won", 700000, &old, nil) // 区间外，不计
	mk("lost", 800000, nil, &recent)
	mk("lost", 900000, nil, &old) // 区间外，不计
	// 他租户干扰
	if err := s.db.Create(&model.Opportunity{
		TenantID: 2, CustomerID: 1, Name: "他租户", AmountCents: 12345678,
		Stage: "won", WonAt: &recent,
	}).Error; err != nil {
		t.Fatal(err)
	}

	stats, err := s.Stats(context.Background(), tenant, since)
	if err != nil {
		t.Fatal(err)
	}
	want := naiveFunnel(s, tenant, since)
	if len(stats.Funnel) != len(want) {
		t.Fatalf("漏斗档数 = %d, want %d", len(stats.Funnel), len(want))
	}
	for i, got := range stats.Funnel {
		if got.Stage != want[i].Stage || got.Count != want[i].Count || got.AmountCents != want[i].AmountCents {
			t.Fatalf("漏斗[%d] = %+v, 逐阶段实现 = %+v", i, got, want[i])
		}
	}
	// 派生字段与漏斗自洽
	if stats.WonCount != 2 || stats.WonAmountCents != 1100000 {
		t.Fatalf("赢单 = %d/%d, want 2/1100000", stats.WonCount, stats.WonAmountCents)
	}
	if stats.OpenOpportunity != 8 {
		t.Fatalf("进行中商机 = %d, want 8", stats.OpenOpportunity)
	}
}

// 漏斗里没有任何数据的阶段仍要出现（0 值档不能被 GROUP BY 吃掉）。
func TestStatsFunnelKeepsEmptyStages(t *testing.T) {
	s := newTestStore(t)
	if err := s.db.Create(&model.Opportunity{
		TenantID: 1, CustomerID: 1, Name: "唯一商机", AmountCents: 100, Stage: "initial",
	}).Error; err != nil {
		t.Fatal(err)
	}
	stats, err := s.Stats(context.Background(), 1, time.Now().AddDate(0, 0, -30))
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.Funnel) != len(model.Stages) {
		t.Fatalf("漏斗档数 = %d, want %d", len(stats.Funnel), len(model.Stages))
	}
	for i, b := range stats.Funnel {
		if b.Stage != model.Stages[i] {
			t.Fatalf("漏斗[%d].Stage = %s, want %s", i, b.Stage, model.Stages[i])
		}
	}
}

// ---------- 3. 公海回收：批量实现 == 逐条实现 ----------

func TestPlanRecycleMatchesPerRowLogic(t *testing.T) {
	s := newTestStore(t)
	const tenant uint64 = 1
	if err := s.SavePoolRule(&model.PoolRule{
		TenantID: tenant, Level: "*", IdleDays: 30, ProtectActive: true, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.SavePoolRule(&model.PoolRule{
		TenantID: tenant, Level: "A", IdleDays: 90, ProtectActive: true, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-40 * 24 * time.Hour)
	mk := func(name, level string, owner uint64, follow *time.Time) uint64 {
		c := model.Customer{
			TenantID: tenant, Name: name, Level: level, OwnerUserID: owner, LastFollowAt: follow,
		}
		if err := s.db.Create(&c).Error; err != nil {
			t.Fatal(err)
		}
		return c.ID
	}
	overdueC := mk("C级超期", "C", 7, &old)
	notOverdueA := mk("A级40天", "A", 7, &old) // 90 天档，不到期
	protectedC := mk("C级有商机", "C", 7, &old)  // 保护规则拦下
	fresh := mk("C级刚跟进", "C", 7, nil)        // created_at=now，不到期
	pooled := mk("已在公海", "C", 0, &old)       // owner=0 不参与
	otherTenantC := mk("他租户", "C", 7, &old)  // 下面改租户
	if err := s.db.Model(&model.Customer{}).Where("id = ?", otherTenantC).
		Update("tenant_id", 2).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.db.Create(&model.Opportunity{
		TenantID: tenant, CustomerID: protectedC, Name: "保护商机", Stage: "negotiation",
	}).Error; err != nil {
		t.Fatal(err)
	}

	plan, err := s.PlanRecycle(tenant, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 1 || plan[0].ID != overdueC {
		t.Fatalf("计划回收 = %+v, 只应含 %d", plan, overdueC)
	}
	if plan[0].FromOwner != 7 || plan[0].TenantID != tenant || plan[0].Name != "C级超期" {
		t.Fatalf("回收明细 = %+v", plan[0])
	}
	_ = notOverdueA
	_ = fresh
	_ = pooled

	// 批量保护判定 == 逐客户两条 COUNT
	all := []uint64{overdueC, notOverdueA, protectedC, fresh, pooled}
	batch := s.customersWithActiveBusiness(tenant, all)
	for _, id := range all {
		if got, want := batch[id], naiveHasActiveBusiness(s, id, tenant); got != want {
			t.Fatalf("客户 %d 保护判定 batch=%v naive=%v", id, got, want)
		}
	}

	// 落库：客户回池 + 留痕，且只动计划内的行
	applied, err := s.ApplyRecycle(tenant, plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 1 || applied[0].ID != overdueC {
		t.Fatalf("已回收列表 = %+v, 应只含 %d", applied, overdueC)
	}
	var got model.Customer
	if err := s.db.First(&got, overdueC).Error; err != nil {
		t.Fatal(err)
	}
	if got.OwnerUserID != 0 || got.PooledAt == nil {
		t.Fatalf("回收后客户 = owner %d pooled %v", got.OwnerUserID, got.PooledAt)
	}
	for _, id := range []uint64{notOverdueA, protectedC, fresh} {
		var c model.Customer
		if err := s.db.First(&c, id).Error; err != nil {
			t.Fatal(err)
		}
		if c.OwnerUserID != 7 {
			t.Fatalf("客户 %d 被误回收（owner=%d）", id, c.OwnerUserID)
		}
	}
	var otherT model.Customer
	if err := s.db.First(&otherT, otherTenantC).Error; err != nil {
		t.Fatal(err)
	}
	if otherT.OwnerUserID != 7 {
		t.Fatal("跨租户客户被回收，租户条件丢失")
	}
	var logs []model.AssignLog
	if err := tenantQ(s.db, tenant).Where("target_type = ? AND action = ?", "customer", "recycle").
		Find(&logs).Error; err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].TargetID != overdueC || logs[0].FromOwner != 7 {
		t.Fatalf("回收留痕 = %+v", logs)
	}
}

// 计划算出后被人领走的客户：UPDATE 的 owner_user_id > 0 守卫必须挡住误清。
func TestApplyRecycleSkipsConcurrentlyClaimed(t *testing.T) {
	s := newTestStore(t)
	const tenant uint64 = 1
	c := model.Customer{TenantID: tenant, Name: "并发领取", Level: "C", OwnerUserID: 7}
	if err := s.db.Create(&c).Error; err != nil {
		t.Fatal(err)
	}
	// 计划里有它，但落库前已被释放进公海再被人领走
	if err := s.db.Model(&model.Customer{}).Where("id = ?", c.ID).
		Update("owner_user_id", 0).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimCustomer(c.ID, tenant, 42); err != nil {
		t.Fatal(err)
	}
	plan := []RecycledCustomer{{ID: c.ID, Name: c.Name, FromOwner: 7, TenantID: tenant}}
	applied, err := s.ApplyRecycle(tenant, plan)
	if err != nil {
		t.Fatal(err)
	}
	// 返回值是“真的入池了的行”，通知与留痕都按它走：并发易主的必须不在其中
	if len(applied) != 0 {
		t.Fatalf("并发领取的客户出现在已回收列表里：%+v", applied)
	}
	var got model.Customer
	if err := s.db.First(&got, c.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.OwnerUserID != 42 {
		t.Fatalf("并发领取的客户被回收清空：owner = %d, want 42", got.OwnerUserID)
	}
	var logs int64
	if err := s.db.Model(&model.AssignLog{}).
		Where("target_id = ? AND action = ?", c.ID, "recycle").Count(&logs).Error; err != nil {
		t.Fatal(err)
	}
	if logs != 0 {
		t.Fatalf("并发易主却留下了回收痕迹：%d 条", logs)
	}
}

// 回收批量化后仍遵守 limit，且候选量与查询数无关。
func TestPlanRecycleRespectsLimit(t *testing.T) {
	s := newTestStore(t)
	const tenant uint64 = 1
	old := time.Now().Add(-60 * 24 * time.Hour)
	for i := 0; i < 50; i++ {
		if err := s.db.Create(&model.Customer{
			TenantID: tenant, Name: fmt.Sprintf("超期%d", i), Level: "C",
			OwnerUserID: 7, LastFollowAt: &old,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	plan, err := s.PlanRecycle(tenant, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 10 {
		t.Fatalf("limit=10 计划出 %d 条", len(plan))
	}
	all, err := s.PlanRecycle(tenant, 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 50 {
		t.Fatalf("limit=200 计划出 %d 条, want 50", len(all))
	}
}

// 公海预览的保护标注批量化后与逐条一致。
func TestPoolStatsPreviewProtectedMatchesPerRow(t *testing.T) {
	s := newTestStore(t)
	const tenant uint64 = 1
	old := time.Now().Add(-60 * 24 * time.Hour)
	ids := make([]uint64, 0, 6)
	for i := 0; i < 6; i++ {
		c := model.Customer{
			TenantID: tenant, Name: fmt.Sprintf("客户%d", i), Level: "C",
			OwnerUserID: 7, LastFollowAt: &old,
		}
		if err := s.db.Create(&c).Error; err != nil {
			t.Fatal(err)
		}
		ids = append(ids, c.ID)
		if i%2 == 0 {
			if err := s.db.Create(&model.Opportunity{
				TenantID: tenant, CustomerID: c.ID, Name: "在谈", Stage: "proposal",
			}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	stats, err := s.PoolStatsFor(tenant, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.Preview) != 6 {
		t.Fatalf("preview = %d 行, want 6", len(stats.Preview))
	}
	for _, row := range stats.Preview {
		if want := naiveHasActiveBusiness(s, row.ID, tenant); row.Protected != want {
			t.Fatalf("客户 %d protected=%v, 逐条判定=%v", row.ID, row.Protected, want)
		}
	}
	_ = ids
}

// ---------- 4. 客户详情关联上限 ----------

func TestCustomerDetailCapsAssociations(t *testing.T) {
	s := newTestStore(t)
	const tenant uint64 = 1
	cust := model.Customer{TenantID: tenant, Name: "大客户", OwnerUserID: 7}
	if err := s.db.Create(&cust).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 150; i++ {
		if err := s.db.Create(&model.Contact{
			TenantID: tenant, CustomerID: cust.ID, Name: fmt.Sprintf("联系人%d", i),
		}).Error; err != nil {
			t.Fatal(err)
		}
		if err := s.db.Create(&model.Opportunity{
			TenantID: tenant, CustomerID: cust.ID, Name: fmt.Sprintf("商机%d", i), Stage: "initial",
		}).Error; err != nil {
			t.Fatal(err)
		}
		if err := s.db.Create(&model.Contract{
			TenantID: tenant, No: fmt.Sprintf("BIG-%03d", i), Title: "合同",
			CustomerID: cust.ID, Status: "active",
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	d, err := s.GetCustomerDetail(cust.ID, tenant)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Contacts) != 100 || len(d.Opportunities) != 100 || len(d.Contracts) != 100 {
		t.Fatalf("关联未加上限：contacts=%d opps=%d contracts=%d",
			len(d.Contacts), len(d.Opportunities), len(d.Contracts))
	}
}

// ---------- 5. 索引幂等 ----------

func TestIndexStatementsAreIdempotent(t *testing.T) {
	stmts := indexStatements()
	if len(stmts) == 0 {
		t.Fatal("索引清单为空")
	}
	seen := map[string]bool{}
	for _, stmt := range stmts {
		if !contains(stmt, "IF NOT EXISTS") {
			t.Fatalf("非幂等 DDL: %s", stmt)
		}
		if seen[stmt] {
			t.Fatalf("重复 DDL: %s", stmt)
		}
		seen[stmt] = true
	}
	// sqlite 下 ensureIndexes 是 no-op，连跑两次不得报错（生产 PG 路径靠
	// IF NOT EXISTS 保证同样语义）
	s := newTestStore(t)
	for i := 0; i < 2; i++ {
		if err := s.ensureIndexes(); err != nil {
			t.Fatalf("第 %d 次 ensureIndexes: %v", i+1, err)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
