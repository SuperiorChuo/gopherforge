package api

// P1 名片分配 + P2 公海引擎测试。

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestLeadAssignAndClaim(t *testing.T) {
	r, srv := newTestRouterWithServer(t)
	// 分配目标须属于本租户：预置部门 5 与成员 77
	for _, ddl := range []string{
		`INSERT INTO departments (id, name, status) VALUES (5, '销售一部', 1)`,
		`INSERT INTO users (id, status) VALUES (77, 1)`,
	} {
		if err := srv.Store.DB().Exec(ddl).Error; err != nil {
			t.Fatalf("seed org: %v", err)
		}
	}

	// 建两张未分配名片（owner 显式清零模拟池子——CreateLead 默认 owner=创建人，
	// 分配场景由管理员导入后先归 0）
	var ids []uint64
	for i := 0; i < 2; i++ {
		_, env := call(t, r, http.MethodPost, "/api/v1/crm/leads",
			map[string]any{"name": fmt.Sprintf("名片%d", i)}, "")
		var l struct {
			ID uint64 `json:"id"`
		}
		decode(t, env.Data, &l)
		ids = append(ids, l.ID)
	}

	// 批量分到部门池（dept=5，owner=0）
	w, env := call(t, r, http.MethodPost, "/api/v1/crm/leads/assign",
		map[string]any{"ids": ids, "department_id": 5}, "")
	if w.Code != http.StatusOK {
		t.Fatalf("assign dept: %d %s", w.Code, w.Body.String())
	}
	var ares struct {
		Assigned int `json:"assigned"`
	}
	decode(t, env.Data, &ares)
	if ares.Assigned != 2 {
		t.Fatalf("assigned = %d", ares.Assigned)
	}

	// 部门池视图可见
	w, env = call(t, r, http.MethodGet, "/api/v1/crm/leads?scope=dept&department_id=5", nil, "")
	var list struct {
		Total int64 `json:"total"`
	}
	decode(t, env.Data, &list)
	if list.Total != 2 {
		t.Fatalf("dept pool total = %d", list.Total)
	}

	// 领取一张 → 我的视图 +1，部门池 -1
	w, _ = call(t, r, http.MethodPost, fmt.Sprintf("/api/v1/crm/leads/%d/claim", ids[0]), nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("claim: %d %s", w.Code, w.Body.String())
	}
	// 重复领取被拒
	w, _ = call(t, r, http.MethodPost, fmt.Sprintf("/api/v1/crm/leads/%d/claim", ids[0]), nil, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("re-claim: %d", w.Code)
	}
	w, env = call(t, r, http.MethodGet, "/api/v1/crm/leads?scope=mine", nil, "")
	decode(t, env.Data, &list)
	if list.Total != 1 {
		t.Fatalf("mine total = %d", list.Total)
	}

	// 定向分给成员 77
	w, _ = call(t, r, http.MethodPost, "/api/v1/crm/leads/assign",
		map[string]any{"ids": []uint64{ids[1]}, "owner_user_id": 77, "department_id": 5}, "")
	if w.Code != http.StatusOK {
		t.Fatalf("assign owner: %d", w.Code)
	}

	// 分配日志留痕（名片0：assign + claim）
	w, env = call(t, r, http.MethodGet,
		fmt.Sprintf("/api/v1/crm/assign-logs?target_type=lead&target_id=%d", ids[0]), nil, "")
	var logs struct {
		List []struct {
			Action string `json:"action"`
		} `json:"list"`
	}
	decode(t, env.Data, &logs)
	if len(logs.List) != 2 || logs.List[0].Action != "claim" || logs.List[1].Action != "assign" {
		t.Fatalf("logs = %+v", logs.List)
	}
}

// 分配目标归属校验：owner/dept 不在本租户（或不存在）时拒绝分配。
func TestAssignLeadsTargetTenantValidation(t *testing.T) {
	r, srv := newTestRouterWithServer(t)
	db := srv.Store.DB()
	// 本租户部门 5 / 成员 77；租户 2 的部门 6 / 成员 88；禁用成员 66
	for _, ddl := range []string{
		`INSERT INTO departments (id, tenant_id, status) VALUES (5, 1, 1), (6, 2, 1)`,
		`INSERT INTO users (id, tenant_id, status) VALUES (77, 1, 1), (88, 2, 1), (66, 1, 0)`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	_, env := call(t, r, http.MethodPost, "/api/v1/crm/leads", map[string]any{"name": "名片"}, "")
	var l struct {
		ID uint64 `json:"id"`
	}
	decode(t, env.Data, &l)
	ids := []uint64{l.ID}

	// 跨租户成员 → 拒
	w, _ := call(t, r, http.MethodPost, "/api/v1/crm/leads/assign",
		map[string]any{"ids": ids, "owner_user_id": 88}, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("跨租户 owner 应拒绝，got %d", w.Code)
	}
	// 不存在的成员 → 拒
	w, _ = call(t, r, http.MethodPost, "/api/v1/crm/leads/assign",
		map[string]any{"ids": ids, "owner_user_id": 9999}, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("不存在 owner 应拒绝，got %d", w.Code)
	}
	// 禁用成员 → 拒
	w, _ = call(t, r, http.MethodPost, "/api/v1/crm/leads/assign",
		map[string]any{"ids": ids, "owner_user_id": 66}, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("禁用 owner 应拒绝，got %d", w.Code)
	}
	// 跨租户部门 → 拒
	w, _ = call(t, r, http.MethodPost, "/api/v1/crm/leads/assign",
		map[string]any{"ids": ids, "department_id": 6}, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("跨租户 dept 应拒绝，got %d", w.Code)
	}
	// 本租户成员+部门 → 通过
	w, _ = call(t, r, http.MethodPost, "/api/v1/crm/leads/assign",
		map[string]any{"ids": ids, "owner_user_id": 77, "department_id": 5}, "")
	if w.Code != http.StatusOK {
		t.Fatalf("本租户目标应通过，got %d %s", w.Code, w.Body.String())
	}
}

// Org 选择器只返回本租户的部门与成员。
func TestOrgTenantScoped(t *testing.T) {
	r, srv := newTestRouterWithServer(t)
	db := srv.Store.DB()
	for _, ddl := range []string{
		`INSERT INTO departments (id, tenant_id, name, status) VALUES (5, 1, '本租户部门', 1), (6, 2, '他租户部门', 1)`,
		`INSERT INTO users (id, tenant_id, username, status) VALUES (77, 1, 'mine', 1), (88, 2, 'theirs', 1)`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	w, env := call(t, r, http.MethodGet, "/api/v1/crm/org", nil, "1")
	if w.Code != http.StatusOK {
		t.Fatalf("org: %d", w.Code)
	}
	var org struct {
		Departments []struct {
			ID uint64 `json:"id"`
		} `json:"departments"`
		Users []struct {
			ID uint64 `json:"id"`
		} `json:"users"`
	}
	decode(t, env.Data, &org)
	for _, d := range org.Departments {
		if d.ID == 6 {
			t.Fatalf("org 泄漏他租户部门: %+v", org.Departments)
		}
	}
	for _, u := range org.Users {
		if u.ID == 88 {
			t.Fatalf("org 泄漏他租户成员: %+v", org.Users)
		}
	}
}

func TestPoolRulesAndRecycle(t *testing.T) {
	r, srv := newTestRouterWithServer(t)

	// 默认规则可读（无配置时补默认档）
	w, env := call(t, r, http.MethodGet, "/api/v1/crm/pool/rules", nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("rules: %d", w.Code)
	}
	var rules struct {
		Rules []struct {
			Level    string `json:"level"`
			IdleDays int    `json:"idle_days"`
		} `json:"rules"`
	}
	decode(t, env.Data, &rules)
	if len(rules.Rules) == 0 || rules.Rules[0].Level != "*" || rules.Rules[0].IdleDays != 30 {
		t.Fatalf("default rules = %+v", rules.Rules)
	}

	// 配置：默认 30 天，A 级 90 天
	w, _ = call(t, r, http.MethodPut, "/api/v1/crm/pool/rules",
		map[string]any{"level": "*", "idle_days": 30, "protect_active": true, "enabled": true}, "")
	if w.Code != http.StatusOK {
		t.Fatalf("save rule: %d %s", w.Code, w.Body.String())
	}
	w, _ = call(t, r, http.MethodPut, "/api/v1/crm/pool/rules",
		map[string]any{"level": "A", "idle_days": 90, "protect_active": true, "enabled": true}, "")
	if w.Code != http.StatusOK {
		t.Fatalf("save rule A: %d", w.Code)
	}

	// 三个客户：C 级 40 天没跟进(该收)、A 级 40 天没跟进(90 天档，不收)、
	// C 级 40 天没跟进但有进行中商机(保护，不收)
	mk := func(name, level string) uint64 {
		_, env := call(t, r, http.MethodPost, "/api/v1/crm/customers",
			map[string]any{"name": name, "level": level}, "")
		var cu struct {
			ID uint64 `json:"id"`
		}
		decode(t, env.Data, &cu)
		return cu.ID
	}
	c1, c2, c3 := mk("C级超期", "C"), mk("A级未超期", "A"), mk("C级但有商机", "C")
	old := time.Now().Add(-40 * 24 * time.Hour)
	srv.Store.DB().Exec("UPDATE crm_customers SET last_follow_at = ? WHERE id IN (?,?,?)", old, c1, c2, c3)
	w, _ = call(t, r, http.MethodPost, "/api/v1/crm/opportunities",
		map[string]any{"customer_id": c3, "name": "保护商机"}, "")
	if w.Code != http.StatusOK {
		t.Fatalf("opp: %d", w.Code)
	}

	recycled, err := srv.Store.RecycleIdleCustomers(1, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(recycled) != 1 || recycled[0].ID != c1 {
		t.Fatalf("recycled = %+v (want id=%d)", recycled, c1)
	}

	// 公海视图有 c1；领取后回到我的
	w, env = call(t, r, http.MethodGet, "/api/v1/crm/customers?scope=pool", nil, "")
	var list struct {
		Total int64 `json:"total"`
	}
	decode(t, env.Data, &list)
	if list.Total != 1 {
		t.Fatalf("pool total = %d", list.Total)
	}
	w, _ = call(t, r, http.MethodPost, fmt.Sprintf("/api/v1/crm/customers/%d/claim", c1), nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("claim: %d %s", w.Code, w.Body.String())
	}
	// 领取刷新计时 → 跨期视图为空
	w, env = call(t, r, http.MethodGet, "/api/v1/crm/customers?scope=overdue", nil, "")
	decode(t, env.Data, &list)
	// c2 是 A 级 40 天 < 90 天不算跨期；c3 被保护但口径上仍超期（跨期视图不含保护判断，
	// 用于提醒销售），owner 是本人 → total 应为 1（c3）
	if list.Total != 1 {
		t.Fatalf("overdue total = %d", list.Total)
	}

	// 手动释放回公海
	w, _ = call(t, r, http.MethodPost, fmt.Sprintf("/api/v1/crm/customers/%d/release", c1),
		map[string]any{"note": "跟不动了"}, "")
	if w.Code != http.StatusOK {
		t.Fatalf("release: %d", w.Code)
	}
	w, env = call(t, r, http.MethodGet, "/api/v1/crm/customers?scope=pool", nil, "")
	decode(t, env.Data, &list)
	if list.Total != 1 {
		t.Fatalf("pool after release = %d", list.Total)
	}
}

func TestRevisitScope(t *testing.T) {
	r := newTestRouter(t)
	_, env := call(t, r, http.MethodPost, "/api/v1/crm/customers", map[string]any{"name": "回访客户"}, "")
	var cu struct {
		ID uint64 `json:"id"`
	}
	decode(t, env.Data, &cu)

	// 跟进并设已过期的回访时间 → 回访视图命中
	w, _ := call(t, r, http.MethodPost, "/api/v1/crm/followups", map[string]any{
		"target_type": "customer", "target_id": cu.ID,
		"content": "初次沟通", "next_at": "2026-07-01 10:00",
	}, "")
	if w.Code != http.StatusOK {
		t.Fatalf("followup: %d", w.Code)
	}
	w, env = call(t, r, http.MethodGet, "/api/v1/crm/customers?scope=revisit", nil, "")
	var list struct {
		Total int64 `json:"total"`
	}
	decode(t, env.Data, &list)
	if list.Total != 1 {
		t.Fatalf("revisit total = %d", list.Total)
	}

	// 再记一条不带 next_at 的跟进（完成回访）→ 回访视图清零
	w, _ = call(t, r, http.MethodPost, "/api/v1/crm/followups", map[string]any{
		"target_type": "customer", "target_id": cu.ID, "content": "已回访",
	}, "")
	if w.Code != http.StatusOK {
		t.Fatalf("followup2: %d", w.Code)
	}
	w, env = call(t, r, http.MethodGet, "/api/v1/crm/customers?scope=revisit", nil, "")
	decode(t, env.Data, &list)
	if list.Total != 0 {
		t.Fatalf("revisit after done = %d", list.Total)
	}
}
