package engine

// 引擎核心用例：发起推进 / 会签收敛 / 或签收敛（含救回）/ 拒绝终止 /
// 角色解析与空候选人兜底 / 发起人自选 / 撤销 / 防重复发起 / 版本冻结 /
// 发布校验。sqlite 内存库基架。

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/bpm/internal/flow"
	"github.com/go-admin-kit/services/bpm/internal/model"
	"github.com/go-admin-kit/services/bpm/internal/store"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var dbSeq atomic.Int64

func openTest(t *testing.T) (*store.Store, *Engine) {
	t.Helper()
	dsn := fmt.Sprintf("file:bpmeng%d?mode=memory&cache=shared", dbSeq.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("sqlite: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	st, err := store.NewWithDB(db)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	// 角色/部门主管解析依赖的 identity 表（生产同库；测试手建最小结构）
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, tenant_id INTEGER DEFAULT 1,
			username TEXT DEFAULT '', nickname TEXT DEFAULT '', department_id INTEGER DEFAULT 0,
			status INTEGER DEFAULT 1)`,
		`CREATE TABLE IF NOT EXISTS user_roles (id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER, role_id INTEGER)`,
		`CREATE TABLE IF NOT EXISTS departments (id INTEGER PRIMARY KEY, tenant_id INTEGER DEFAULT 1,
			name TEXT DEFAULT '', leader_user_id INTEGER DEFAULT 0, status INTEGER DEFAULT 1)`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("identity ddl: %v", err)
		}
	}
	return st, New(db)
}

// ---- 节点树构造 helper ----

func startNode(next *flow.Node) *flow.Node {
	return &flow.Node{ID: "n-start", Name: "发起", Type: flow.TypeStart,
		FormFields: []string{"amount_cents"}, Next: next}
}

func approvalUsers(id, name, mode string, users []uint64, next *flow.Node) *flow.Node {
	return &flow.Node{ID: id, Name: name, Type: flow.TypeApproval,
		Assignee:  &flow.AssigneeRule{Type: flow.RuleUsers, UserIDs: users},
		MultiMode: mode, Next: next}
}

func mustTree(t *testing.T, s *flow.Schema) []byte {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal tree: %v", err)
	}
	return b
}

// seedDef 建定义并发布为 active。
func seedDef(t *testing.T, st *store.Store, tenantID uint64, key string, tree []byte) *model.ProcessDefinition {
	t.Helper()
	d, err := st.CreateDefinition(tenantID, store.CreateDefinitionInput{
		Key: key, Name: "测试流程-" + key, BizType: "demo", NodeTree: tree, CreatedBy: 1,
	})
	if err != nil {
		t.Fatalf("create def: %v", err)
	}
	if _, err := st.Publish(d.ID, tenantID); err != nil {
		t.Fatalf("publish def: %v", err)
	}
	return d
}

func startInst(t *testing.T, e *Engine, key, bizID string, vars []byte) *Effects {
	t.Helper()
	eff, err := e.Start(StartInput{
		TenantID: 1, DefinitionKey: key, Title: "测试单 " + bizID,
		BizType: "demo", BizID: bizID,
		FormSnapshot: []byte(`{"amount_cents":100000}`),
		Variables:    vars, InitiatorID: 1,
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	return eff
}

func taskOf(t *testing.T, eff *Effects, assignee uint64) *model.Task {
	t.Helper()
	for i := range eff.NewTasks {
		if eff.NewTasks[i].AssigneeID == assignee {
			return &eff.NewTasks[i]
		}
	}
	t.Fatalf("没有 assignee=%d 的新任务: %+v", assignee, eff.NewTasks)
	return nil
}

// ---- 用例 ----

// 单审批节点（users）+ 抄送节点：发起 → 同意 → 终态 approved，抄送落记录。
func TestStartApproveToFinish(t *testing.T) {
	st, e := openTest(t)
	cc := &flow.Node{ID: "n-cc", Name: "抄送财务", Type: flow.TypeCc,
		Targets: &flow.AssigneeRule{Type: flow.RuleUsers, UserIDs: []uint64{9}}}
	tree := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(approvalUsers("n-a1", "经理审批", flow.MultiOr, []uint64{2}, cc))})
	seedDef(t, st, 1, "flow_single", tree)

	eff := startInst(t, e, "flow_single", "biz-1", nil)
	if eff.Instance.Status != model.InstRunning || eff.Instance.CurrentNodeID != "n-a1" {
		t.Fatalf("发起后状态: %s @%s", eff.Instance.Status, eff.Instance.CurrentNodeID)
	}
	if len(eff.NewTasks) != 1 || eff.NewTasks[0].AssigneeID != 2 {
		t.Fatalf("展开任务: %+v", eff.NewTasks)
	}

	eff2, err := e.Approve(1, eff.NewTasks[0].ID, 2, "同意")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if eff2.Instance.Status != model.InstApproved || eff2.FinalResult != model.InstApproved {
		t.Fatalf("终态: %s / %s", eff2.Instance.Status, eff2.FinalResult)
	}
	if len(eff2.CcRecords) != 1 || eff2.CcRecords[0].UserID != 9 {
		t.Fatalf("抄送记录: %+v", eff2.CcRecords)
	}
	logs, _ := st.ListInstanceLogs(eff.Instance.ID, 1)
	actions := map[string]bool{}
	for _, l := range logs {
		actions[l.Action] = true
	}
	for _, want := range []string{model.ActionSubmit, model.ActionApprove, model.ActionCc, model.ActionFinishApproved} {
		if !actions[want] {
			t.Fatalf("缺少日志 %s，已有 %v", want, actions)
		}
	}
}

// 会签（AND）：一人同意仍等待，全员同意才通过；重复同意报已处理。
func TestAndCountersign(t *testing.T) {
	st, e := openTest(t)
	tree := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(approvalUsers("n-a1", "会签", flow.MultiAnd, []uint64{2, 3}, nil))})
	seedDef(t, st, 1, "flow_and", tree)

	eff := startInst(t, e, "flow_and", "biz-1", nil)
	if len(eff.NewTasks) != 2 {
		t.Fatalf("会签应展开 2 任务: %+v", eff.NewTasks)
	}
	t2 := taskOf(t, eff, 2)
	eff2, err := e.Approve(1, t2.ID, 2, "")
	if err != nil {
		t.Fatalf("first approve: %v", err)
	}
	if eff2.Instance.Status != model.InstRunning || eff2.FinalResult != "" {
		t.Fatalf("一人同意后应继续等待: %s", eff2.Instance.Status)
	}
	// 同一任务重复同意 → 已处理
	if _, err := e.Approve(1, t2.ID, 2, ""); !errors.Is(err, ErrTaskHandled) {
		t.Fatalf("重复同意应报已处理，got %v", err)
	}
	t3 := taskOf(t, eff, 3)
	eff3, err := e.Approve(1, t3.ID, 3, "")
	if err != nil {
		t.Fatalf("second approve: %v", err)
	}
	if eff3.Instance.Status != model.InstApproved {
		t.Fatalf("全员同意后应通过: %s", eff3.Instance.Status)
	}
}

// 或签（OR）：一人同意即通过，其余任务置 skipped。
func TestOrSignSkipsOthers(t *testing.T) {
	st, e := openTest(t)
	tree := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(approvalUsers("n-a1", "或签", flow.MultiOr, []uint64{2, 3}, nil))})
	seedDef(t, st, 1, "flow_or", tree)

	eff := startInst(t, e, "flow_or", "biz-1", nil)
	t2 := taskOf(t, eff, 2)
	eff2, err := e.Approve(1, t2.ID, 2, "")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if eff2.Instance.Status != model.InstApproved {
		t.Fatalf("或签一人同意应通过: %s", eff2.Instance.Status)
	}
	tasks, _ := st.ListInstanceTasks(eff.Instance.ID, 1)
	var skipped int
	for _, tk := range tasks {
		if tk.AssigneeID == 3 && tk.Status == model.TaskSkipped {
			skipped++
		}
	}
	if skipped != 1 {
		t.Fatalf("或签他人任务应 skipped: %+v", tasks)
	}
}

// 或签救回：一人拒绝后实例继续等待，另一人同意仍可通过；双拒则实例 rejected。
func TestOrSignRejectThenSave(t *testing.T) {
	st, e := openTest(t)
	tree := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(approvalUsers("n-a1", "或签", flow.MultiOr, []uint64{2, 3}, nil))})
	seedDef(t, st, 1, "flow_or_save", tree)

	// 场景 A：一拒一同意 → 通过
	effA := startInst(t, e, "flow_or_save", "biz-A", nil)
	effR, err := e.Reject(1, taskOf(t, effA, 2).ID, 2, "不同意")
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if effR.Instance.Status != model.InstRunning {
		t.Fatalf("或签一人拒后应继续等待: %s", effR.Instance.Status)
	}
	effS, err := e.Approve(1, taskOf(t, effA, 3).ID, 3, "")
	if err != nil {
		t.Fatalf("save approve: %v", err)
	}
	if effS.Instance.Status != model.InstApproved {
		t.Fatalf("或签救回应通过: %s", effS.Instance.Status)
	}

	// 场景 B：全部拒绝 → rejected
	effB := startInst(t, e, "flow_or_save", "biz-B", nil)
	if _, err := e.Reject(1, taskOf(t, effB, 2).ID, 2, "拒1"); err != nil {
		t.Fatalf("rejectB1: %v", err)
	}
	effB2, err := e.Reject(1, taskOf(t, effB, 3).ID, 3, "拒2")
	if err != nil {
		t.Fatalf("rejectB2: %v", err)
	}
	if effB2.Instance.Status != model.InstRejected || effB2.FinalResult != model.InstRejected {
		t.Fatalf("或签全拒应 rejected: %s", effB2.Instance.Status)
	}
}

// 会签拒绝即终止：实例 rejected，其余 pending 任务置 skipped；拒绝必填意见。
func TestRejectTerminatesAnd(t *testing.T) {
	st, e := openTest(t)
	tree := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(approvalUsers("n-a1", "会签", flow.MultiAnd, []uint64{2, 3}, nil))})
	seedDef(t, st, 1, "flow_and_rej", tree)

	eff := startInst(t, e, "flow_and_rej", "biz-1", nil)
	if _, err := e.Reject(1, taskOf(t, eff, 2).ID, 2, ""); !errors.Is(err, ErrCommentRequired) {
		t.Fatalf("拒绝空意见应报错, got %v", err)
	}
	eff2, err := e.Reject(1, taskOf(t, eff, 2).ID, 2, "金额有误")
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if eff2.Instance.Status != model.InstRejected {
		t.Fatalf("会签一拒应终止: %s", eff2.Instance.Status)
	}
	tasks, _ := st.ListInstanceTasks(eff.Instance.ID, 1)
	for _, tk := range tasks {
		if tk.AssigneeID == 3 && tk.Status != model.TaskSkipped {
			t.Fatalf("其余任务应 skipped: %+v", tk)
		}
	}
	// 终态后不可再操作
	if _, err := e.Approve(1, taskOf(t, eff, 3).ID, 3, ""); err == nil {
		t.Fatal("终态后 approve 应报错")
	}
}

// 两级审批（验收场景：两级、第二级或签）：一级过后二级任务出现。
func TestTwoLevelFlow(t *testing.T) {
	st, e := openTest(t)
	lvl2 := approvalUsers("n-a2", "总监或签", flow.MultiOr, []uint64{3, 4}, nil)
	tree := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(approvalUsers("n-a1", "经理审批", flow.MultiOr, []uint64{2}, lvl2))})
	seedDef(t, st, 1, "flow_two_level", tree)

	eff := startInst(t, e, "flow_two_level", "biz-1", nil)
	eff2, err := e.Approve(1, taskOf(t, eff, 2).ID, 2, "一级同意")
	if err != nil {
		t.Fatalf("lvl1 approve: %v", err)
	}
	if eff2.Instance.Status != model.InstRunning || eff2.Instance.CurrentNodeID != "n-a2" {
		t.Fatalf("一级过后应停在二级: %s @%s", eff2.Instance.Status, eff2.Instance.CurrentNodeID)
	}
	if len(eff2.NewTasks) != 2 {
		t.Fatalf("二级应展开 2 任务: %+v", eff2.NewTasks)
	}
	eff3, err := e.Approve(1, taskOf(t, eff2, 4).ID, 4, "二级同意")
	if err != nil {
		t.Fatalf("lvl2 approve: %v", err)
	}
	if eff3.Instance.Status != model.InstApproved {
		t.Fatalf("二级或签一人同意应通过: %s", eff3.Instance.Status)
	}
}

// 角色规则：直读同库 users/user_roles 解析，过滤禁用用户与跨租户用户。
func TestRolesAssignee(t *testing.T) {
	st, e := openTest(t)
	db := st.DB()
	db.Exec(`INSERT INTO users (id, tenant_id, status) VALUES (7,1,1),(8,1,1),(9,1,0),(10,2,1)`)
	db.Exec(`INSERT INTO user_roles (user_id, role_id) VALUES (7,5),(8,5),(9,5),(10,5)`)
	tree := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(&flow.Node{ID: "n-a1", Name: "角色审批", Type: flow.TypeApproval,
			Assignee:  &flow.AssigneeRule{Type: flow.RuleRoles, RoleIDs: []uint64{5}},
			MultiMode: flow.MultiAnd})})
	seedDef(t, st, 1, "flow_roles", tree)

	eff := startInst(t, e, "flow_roles", "biz-1", nil)
	if len(eff.NewTasks) != 2 {
		t.Fatalf("角色解析应得 2 人（7,8；排除禁用 9 与跨租户 10）: %+v", eff.NewTasks)
	}
	got := map[uint64]bool{}
	for _, tk := range eff.NewTasks {
		got[tk.AssigneeID] = true
	}
	if !got[7] || !got[8] {
		t.Fatalf("角色成员错误: %v", got)
	}
}

// 空候选人兜底三策略：auto_pass 跳过节点；to_users 换兜底人；缺省挂起。
func TestEmptyAssigneeFallback(t *testing.T) {
	st, e := openTest(t)
	mk := func(key string, rule *flow.AssigneeRule) {
		tree := mustTree(t, &flow.Schema{Version: 1,
			Start: startNode(&flow.Node{ID: "n-a1", Name: "空人节点", Type: flow.TypeApproval,
				Assignee: rule, MultiMode: flow.MultiOr})})
		seedDef(t, st, 1, key, tree)
	}
	// auto_pass：角色无人 → 节点自动通过 → 链尾终态 approved
	mk("flow_ap", &flow.AssigneeRule{Type: flow.RuleRoles, RoleIDs: []uint64{99},
		EmptyFallback: flow.FallbackAutoPass})
	eff := startInst(t, e, "flow_ap", "biz-ap", nil)
	if eff.Instance.Status != model.InstApproved {
		t.Fatalf("auto_pass 应直达终态: %s", eff.Instance.Status)
	}
	// to_users：兜底人接任务
	mk("flow_tu", &flow.AssigneeRule{Type: flow.RuleRoles, RoleIDs: []uint64{99},
		EmptyFallback: flow.FallbackToUsers, FallbackUserIDs: []uint64{6}})
	eff2 := startInst(t, e, "flow_tu", "biz-tu", nil)
	if len(eff2.NewTasks) != 1 || eff2.NewTasks[0].AssigneeID != 6 {
		t.Fatalf("to_users 兜底: %+v", eff2.NewTasks)
	}
	// 缺省 suspend：实例挂起
	mk("flow_sp", &flow.AssigneeRule{Type: flow.RuleRoles, RoleIDs: []uint64{99}})
	eff3 := startInst(t, e, "flow_sp", "biz-sp", nil)
	if eff3.Instance.Status != model.InstSuspended {
		t.Fatalf("缺省应挂起: %s", eff3.Instance.Status)
	}
}

// 发起人自选：未提供选人发起报错；提供后任务给所选人。
func TestSelfSelect(t *testing.T) {
	st, e := openTest(t)
	tree := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(&flow.Node{ID: "n-a1", Name: "自选审批", Type: flow.TypeApproval,
			Assignee: &flow.AssigneeRule{Type: flow.RuleSelfSelect}, MultiMode: flow.MultiAnd})})
	seedDef(t, st, 1, "flow_self", tree)

	if _, err := e.Start(StartInput{
		TenantID: 1, DefinitionKey: "flow_self", Title: "缺选人",
		BizType: "demo", BizID: "biz-x", InitiatorID: 1,
	}); err == nil {
		t.Fatal("未提供选人应发起失败")
	}
	vars := []byte(`{"selected_assignees":{"n-a1":[5,6]}}`)
	eff := startInst(t, e, "flow_self", "biz-1", vars)
	if len(eff.NewTasks) != 2 {
		t.Fatalf("自选应展开 2 任务: %+v", eff.NewTasks)
	}
}

// 撤销：无人审过可撤（pending 任务置 canceled）；有人已同意则拒绝撤销；
// 非发起人不可撤。
func TestCancel(t *testing.T) {
	st, e := openTest(t)
	lvl2 := approvalUsers("n-a2", "二级", flow.MultiOr, []uint64{3}, nil)
	tree := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(approvalUsers("n-a1", "一级", flow.MultiOr, []uint64{2}, lvl2))})
	seedDef(t, st, 1, "flow_cancel", tree)

	// 无人审过 → 可撤
	eff := startInst(t, e, "flow_cancel", "biz-1", nil)
	if _, err := e.Cancel(1, eff.Instance.ID, 99); !errors.Is(err, ErrNotInitiator) {
		t.Fatalf("非发起人应拒绝, got %v", err)
	}
	effC, err := e.Cancel(1, eff.Instance.ID, 1)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if effC.Instance.Status != model.InstCanceled || effC.FinalResult != model.InstCanceled {
		t.Fatalf("撤销终态: %s", effC.Instance.Status)
	}
	tasks, _ := st.ListInstanceTasks(eff.Instance.ID, 1)
	if len(tasks) != 1 || tasks[0].Status != model.TaskCanceled {
		t.Fatalf("撤销后任务应 canceled: %+v", tasks)
	}

	// 有人已同意 → 拒绝撤销
	eff2 := startInst(t, e, "flow_cancel", "biz-2", nil)
	if _, err := e.Approve(1, taskOf(t, eff2, 2).ID, 2, ""); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, err := e.Cancel(1, eff2.Instance.ID, 1); !errors.Is(err, ErrCancelDenied) {
		t.Fatalf("有人已审应拒绝撤销, got %v", err)
	}
}

// 防重复发起：同业务对象在途时再发起报错；终态后可再次发起。
func TestDuplicateRunning(t *testing.T) {
	st, e := openTest(t)
	tree := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(approvalUsers("n-a1", "审批", flow.MultiOr, []uint64{2}, nil))})
	seedDef(t, st, 1, "flow_dup", tree)

	eff := startInst(t, e, "flow_dup", "biz-1", nil)
	if _, err := e.Start(StartInput{
		TenantID: 1, DefinitionKey: "flow_dup", Title: "重复",
		BizType: "demo", BizID: "biz-1", InitiatorID: 1,
	}); !errors.Is(err, ErrDuplicateRunning) {
		t.Fatalf("在途重复发起应报错, got %v", err)
	}
	if _, err := e.Approve(1, taskOf(t, eff, 2).ID, 2, ""); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, err := e.Start(StartInput{
		TenantID: 1, DefinitionKey: "flow_dup", Title: "再来",
		BizType: "demo", BizID: "biz-1", InitiatorID: 1,
	}); err != nil {
		t.Fatalf("终态后再发起应允许: %v", err)
	}
}

// 版本冻结：改版发布后在途实例仍按旧版走完，新实例用新版。
func TestVersionFreeze(t *testing.T) {
	st, e := openTest(t)
	treeV1 := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(approvalUsers("n-a1", "V1审批", flow.MultiOr, []uint64{2}, nil))})
	d1 := seedDef(t, st, 1, "flow_ver", treeV1)

	eff := startInst(t, e, "flow_ver", "biz-1", nil)
	if eff.Instance.DefinitionID != d1.ID {
		t.Fatalf("实例应冻结 v1: %d != %d", eff.Instance.DefinitionID, d1.ID)
	}
	// 发新版：v2 两级
	d2, err := st.NewVersion(d1.ID, 1, 1)
	if err != nil {
		t.Fatalf("new version: %v", err)
	}
	lvl2 := approvalUsers("n-a2", "V2二级", flow.MultiOr, []uint64{4}, nil)
	treeV2 := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(approvalUsers("n-a1", "V2一级", flow.MultiOr, []uint64{3}, lvl2))})
	if _, err := st.UpdateDefinition(d2.ID, 1, store.UpdateDefinitionInput{NodeTree: treeV2}); err != nil {
		t.Fatalf("update v2: %v", err)
	}
	if _, err := st.Publish(d2.ID, 1); err != nil {
		t.Fatalf("publish v2: %v", err)
	}
	v1After, _ := st.GetDefinition(d1.ID, 1)
	if v1After.Status != model.DefArchived {
		t.Fatalf("旧 active 应 archived: %s", v1After.Status)
	}
	// 在途实例仍按 v1 走完（单级）
	eff2, err := e.Approve(1, taskOf(t, eff, 2).ID, 2, "")
	if err != nil {
		t.Fatalf("v1 approve: %v", err)
	}
	if eff2.Instance.Status != model.InstApproved {
		t.Fatalf("旧实例应按 v1 单级走完: %s", eff2.Instance.Status)
	}
	// 新实例用 v2（一级审批人 3）
	eff3 := startInst(t, e, "flow_ver", "biz-2", nil)
	if eff3.Instance.DefinitionID != d2.ID || eff3.NewTasks[0].AssigneeID != 3 {
		t.Fatalf("新实例应用 v2: def=%d tasks=%+v", eff3.Instance.DefinitionID, eff3.NewTasks)
	}
}

// 租户隔离：跨租户不可见、不可操作。
func TestTenantIsolation(t *testing.T) {
	st, e := openTest(t)
	tree := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(approvalUsers("n-a1", "审批", flow.MultiOr, []uint64{2}, nil))})
	seedDef(t, st, 1, "flow_tenant", tree)

	eff := startInst(t, e, "flow_tenant", "biz-1", nil)
	// 租户 2 无 active 定义
	if _, err := e.Start(StartInput{
		TenantID: 2, DefinitionKey: "flow_tenant", Title: "跨租户",
		BizType: "demo", BizID: "biz-9", InitiatorID: 1,
	}); !errors.Is(err, ErrNoActiveDefinition) {
		t.Fatalf("跨租户发起应无定义, got %v", err)
	}
	// 租户 2 操作租户 1 的任务
	if _, err := e.Approve(2, eff.NewTasks[0].ID, 2, ""); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("跨租户任务应不可见, got %v", err)
	}
	if _, err := st.GetInstance(eff.Instance.ID, 2); err == nil {
		t.Fatal("跨租户实例应不可见")
	}
	// 非 assignee 操作
	if _, err := e.Approve(1, eff.NewTasks[0].ID, 3, ""); !errors.Is(err, ErrNotAssignee) {
		t.Fatalf("非处理人应拒绝, got %v", err)
	}
}

// 发布校验：非法配置一律拒绝发布。
// dept_leader / back_to_start（M2）、SEQ / condition（M3）已放开，
// 拒绝清单转为结构性非法（分支缺 default / 表达式字段未声明等）。
func TestPublishValidationRejects(t *testing.T) {
	st, _ := openTest(t)
	cases := []struct {
		name string
		tree *flow.Schema
	}{
		{"dept_leader 表单基准缺字段名", &flow.Schema{Version: 1,
			Start: startNode(&flow.Node{ID: "a", Name: "主管", Type: flow.TypeApproval,
				Assignee: &flow.AssigneeRule{Type: flow.RuleDeptLeader,
					DeptLeaderBase: flow.DeptBaseFormField}})}},
		{"condition 无分支", &flow.Schema{Version: 1,
			Start: startNode(&flow.Node{ID: "a", Name: "分支", Type: flow.TypeCondition})}},
		{"condition 缺默认分支", &flow.Schema{Version: 1,
			Start: startNode(&flow.Node{ID: "a", Name: "分支", Type: flow.TypeCondition,
				Branches: []flow.Branch{
					{ID: "b1", Name: "高", Expr: []byte(`{"op":"gte","field":"amount_cents","value":1}`),
						Next: approvalUsers("a1", "审批A", flow.MultiOr, []uint64{2}, nil)},
					{ID: "b2", Name: "低", Expr: []byte(`{"op":"lt","field":"amount_cents","value":1}`),
						Next: approvalUsers("a2", "审批B", flow.MultiOr, []uint64{2}, nil)},
				}})}},
		{"condition 表达式字段未声明", &flow.Schema{Version: 1,
			Start: startNode(&flow.Node{ID: "a", Name: "分支", Type: flow.TypeCondition,
				Branches: []flow.Branch{
					{ID: "b1", Name: "命中", Expr: []byte(`{"op":"eq","field":"undeclared","value":1}`),
						Next: approvalUsers("a1", "审批A", flow.MultiOr, []uint64{2}, nil)},
					{ID: "b2", Name: "默认",
						Next: approvalUsers("a2", "审批B", flow.MultiOr, []uint64{2}, nil)},
				}})}},
		{"无审批节点", &flow.Schema{Version: 1, Start: startNode(nil)}},
		{"users 规则空人", &flow.Schema{Version: 1,
			Start: startNode(&flow.Node{ID: "a", Name: "审批", Type: flow.TypeApproval,
				Assignee: &flow.AssigneeRule{Type: flow.RuleUsers}})}},
	}
	for i, tc := range cases {
		d, err := st.CreateDefinition(1, store.CreateDefinitionInput{
			Key: fmt.Sprintf("bad_%d", i), Name: tc.name, NodeTree: mustTree(t, tc.tree),
		})
		if err != nil {
			t.Fatalf("%s create: %v", tc.name, err)
		}
		if _, err := st.Publish(d.ID, 1); err == nil {
			t.Fatalf("%s 应发布失败", tc.name)
		}
	}
}
