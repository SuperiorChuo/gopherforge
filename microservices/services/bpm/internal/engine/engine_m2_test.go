package engine

// M2 用例：转办（计数不变 / 或签转办后他人同意仍收敛）/ 退回发起人 + 重提
//（round+1 重展开、旧 round 不复活、快照更新）/ 退回上一节点 / 退回后撤销
// 放宽 / onReject=back_to_start / dept_leader 解析 / 超时扫描只提醒一次。
// 基架沿用 engine_test.go 的 sqlite 内存库。

import (
	"errors"
	"testing"
	"time"

	"github.com/go-admin-kit/services/bpm/internal/flow"
	"github.com/go-admin-kit/services/bpm/internal/model"
	"github.com/go-admin-kit/services/bpm/internal/store"
)

// pendingTasks 实例当前 pending 任务。
func pendingTasks(t *testing.T, st *store.Store, instanceID uint64) []model.Task {
	t.Helper()
	all, err := st.ListInstanceTasks(instanceID, 1)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	var out []model.Task
	for _, tk := range all {
		if tk.Status == model.TaskPending {
			out = append(out, tk)
		}
	}
	return out
}

func hasAction(t *testing.T, st *store.Store, instanceID uint64, action string) bool {
	t.Helper()
	logs, _ := st.ListInstanceLogs(instanceID, 1)
	for _, l := range logs {
		if l.Action == action {
			return true
		}
	}
	return false
}

// 转办：会签一人转办后计数不变——新人 + 另一原审批人全同意才通过；
// 原人已办可见（origin_assignee），转办自校验与非法目标校验。
func TestTransferKeepsCounting(t *testing.T) {
	st, e := openTest(t)
	tree := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(approvalUsers("n-a1", "会签", flow.MultiAnd, []uint64{2, 3}, nil))})
	seedDef(t, st, 1, "flow_transfer", tree)

	eff := startInst(t, e, "flow_transfer", "biz-1", nil)
	t2 := taskOf(t, eff, 2)
	if _, err := e.Transfer(1, t2.ID, 2, 2, ""); !errors.Is(err, ErrTransferSelf) {
		t.Fatalf("转给自己应拒绝, got %v", err)
	}
	if _, err := e.Transfer(1, t2.ID, 2, 0, ""); !errors.Is(err, ErrTransferTarget) {
		t.Fatalf("空目标应拒绝, got %v", err)
	}
	if _, err := e.Transfer(1, t2.ID, 9, 5, ""); !errors.Is(err, ErrNotAssignee) {
		t.Fatalf("非处理人转办应拒绝, got %v", err)
	}
	effT, err := e.Transfer(1, t2.ID, 2, 5, "我出差，转给老王")
	if err != nil {
		t.Fatalf("transfer: %v", err)
	}
	if len(effT.NewTasks) != 1 || effT.NewTasks[0].AssigneeID != 5 ||
		effT.NewTasks[0].OriginAssignee != 2 {
		t.Fatalf("转办后任务: %+v", effT.NewTasks)
	}
	// 原人被转出的任务不再是待办，但已办可见
	if todo, _, _ := st.ListTodo(1, 2, store.Page{}); len(todo) != 0 {
		t.Fatalf("原人待办应为空: %+v", todo)
	}
	if done, _, _ := st.ListDone(1, 2, store.Page{}); len(done) != 1 {
		t.Fatalf("原人已办应含转办出去的任务: %+v", done)
	}
	if todo, _, _ := st.ListTodo(1, 5, store.Page{}); len(todo) != 1 {
		t.Fatalf("新人待办应可见: %+v", todo)
	}
	// 计数不变：会签仍需 5 + 3 全同意
	if effA, err := e.Approve(1, t2.ID, 5, ""); err != nil || effA.Instance.Status != model.InstRunning {
		t.Fatalf("新人同意后应继续等待: err=%v", err)
	}
	effB, err := e.Approve(1, taskOf(t, eff, 3).ID, 3, "")
	if err != nil || effB.Instance.Status != model.InstApproved {
		t.Fatalf("全员同意后应通过: err=%v status=%v", err, effB)
	}
	if !hasAction(t, st, eff.Instance.ID, model.ActionTransfer) {
		t.Fatal("缺少 transfer 日志")
	}
}

// 或签：一人被转办后，另一原审批人同意仍收敛（新人任务置 skipped）。
func TestOrTransferThenOtherApproves(t *testing.T) {
	st, e := openTest(t)
	tree := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(approvalUsers("n-a1", "或签", flow.MultiOr, []uint64{2, 3}, nil))})
	seedDef(t, st, 1, "flow_or_transfer", tree)

	eff := startInst(t, e, "flow_or_transfer", "biz-1", nil)
	if _, err := e.Transfer(1, taskOf(t, eff, 2).ID, 2, 5, ""); err != nil {
		t.Fatalf("transfer: %v", err)
	}
	effA, err := e.Approve(1, taskOf(t, eff, 3).ID, 3, "")
	if err != nil || effA.Instance.Status != model.InstApproved {
		t.Fatalf("或签他人同意应收敛: err=%v", err)
	}
	tasks, _ := st.ListInstanceTasks(eff.Instance.ID, 1)
	for _, tk := range tasks {
		if tk.AssigneeID == 5 && tk.Status != model.TaskSkipped {
			t.Fatalf("被转入的任务应 skipped: %+v", tk)
		}
	}
}

// 退回发起人 + 重提：全链路 round+1 重展开；旧 round 任务不复活；
// 快照可更新；非发起人不可重提；意见必填。
func TestReturnStartAndResubmit(t *testing.T) {
	st, e := openTest(t)
	lvl2 := approvalUsers("n-a2", "二级", flow.MultiOr, []uint64{3}, nil)
	tree := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(approvalUsers("n-a1", "一级", flow.MultiOr, []uint64{2}, lvl2))})
	seedDef(t, st, 1, "flow_return", tree)

	eff := startInst(t, e, "flow_return", "biz-1", nil)
	if _, err := e.Approve(1, taskOf(t, eff, 2).ID, 2, ""); err != nil {
		t.Fatalf("lvl1 approve: %v", err)
	}
	tasks, _ := st.ListInstanceTasks(eff.Instance.ID, 1)
	var lvl2Task *model.Task
	for i := range tasks {
		if tasks[i].NodeID == "n-a2" && tasks[i].Status == model.TaskPending {
			lvl2Task = &tasks[i]
		}
	}
	if lvl2Task == nil {
		t.Fatal("二级任务未展开")
	}
	if _, err := e.Return(1, lvl2Task.ID, 3, "start", ""); !errors.Is(err, ErrReturnComment) {
		t.Fatalf("退回空意见应拒绝, got %v", err)
	}
	if _, err := e.Return(1, lvl2Task.ID, 3, "boss", "x"); !errors.Is(err, ErrReturnTarget) {
		t.Fatalf("未知退回目标应拒绝, got %v", err)
	}
	effR, err := e.Return(1, lvl2Task.ID, 3, "start", "材料不全，退回补充")
	if err != nil {
		t.Fatalf("return start: %v", err)
	}
	if effR.Instance.Status != model.InstRunning || effR.Instance.CurrentNodeID != "n-start" {
		t.Fatalf("退回后实例: %s @%s", effR.Instance.Status, effR.Instance.CurrentNodeID)
	}
	// 生成发起人的重提任务（round = 全实例 max+1 = 2）
	if len(effR.NewTasks) != 1 || effR.NewTasks[0].AssigneeID != 1 ||
		effR.NewTasks[0].NodeID != "n-start" || effR.NewTasks[0].Round != 2 {
		t.Fatalf("重提任务: %+v", effR.NewTasks)
	}
	// 重提任务不可走普通同意/拒绝/转办/退回
	resubmitID := effR.NewTasks[0].ID
	if _, err := e.Approve(1, resubmitID, 1, ""); !errors.Is(err, ErrReturnStartTask) {
		t.Fatalf("重提任务 approve 应拒绝, got %v", err)
	}
	if _, err := e.Transfer(1, resubmitID, 1, 5, ""); !errors.Is(err, ErrReturnStartTask) {
		t.Fatalf("重提任务 transfer 应拒绝, got %v", err)
	}
	// 非发起人不可重提
	if _, err := e.Resubmit(1, eff.Instance.ID, 3, nil); !errors.Is(err, ErrNotInitiator) {
		t.Fatalf("非发起人重提应拒绝, got %v", err)
	}
	// 重提（带新快照）→ 一级 round+1 重新展开
	effS, err := e.Resubmit(1, eff.Instance.ID, 1, []byte(`{"amount_cents":200000}`))
	if err != nil {
		t.Fatalf("resubmit: %v", err)
	}
	if len(effS.NewTasks) != 1 || effS.NewTasks[0].NodeID != "n-a1" ||
		effS.NewTasks[0].Round != 2 || effS.NewTasks[0].AssigneeID != 2 {
		t.Fatalf("重提后一级任务: %+v", effS.NewTasks)
	}
	inst, _ := st.GetInstance(eff.Instance.ID, 1)
	if string(inst.FormSnapshot) != `{"amount_cents":200000}` {
		t.Fatalf("快照未更新: %s", inst.FormSnapshot)
	}
	// 旧 round 任务不复活：pending 只剩新一轮的一级任务
	pend := pendingTasks(t, st, eff.Instance.ID)
	if len(pend) != 1 || pend[0].NodeID != "n-a1" || pend[0].Round != 2 {
		t.Fatalf("pending 集合: %+v", pend)
	}
	// 未处于退回态时重提被拒
	if _, err := e.Resubmit(1, eff.Instance.ID, 1, nil); !errors.Is(err, ErrNotReturnedState) {
		t.Fatalf("非退回态重提应拒绝, got %v", err)
	}
	// 新一轮走完：一级(round2) → 二级(round2) → approved，时间线含关键动作
	if _, err := e.Approve(1, pend[0].ID, 2, ""); err != nil {
		t.Fatalf("round2 lvl1: %v", err)
	}
	pend = pendingTasks(t, st, eff.Instance.ID)
	if len(pend) != 1 || pend[0].NodeID != "n-a2" || pend[0].Round != 2 {
		t.Fatalf("round2 二级任务: %+v", pend)
	}
	effF, err := e.Approve(1, pend[0].ID, 3, "")
	if err != nil || effF.Instance.Status != model.InstApproved {
		t.Fatalf("round2 走完应通过: err=%v", err)
	}
	for _, want := range []string{model.ActionReturnStart, model.ActionResubmit} {
		if !hasAction(t, st, eff.Instance.ID, want) {
			t.Fatalf("缺少日志 %s", want)
		}
	}
}

// 退回上一节点：allowBackPrev 才允许；上一审批节点 round+1 重展开并可走完；
// 无上一审批节点时等价退回发起人。
func TestReturnPrev(t *testing.T) {
	st, e := openTest(t)
	lvl2 := &flow.Node{ID: "n-a2", Name: "二级", Type: flow.TypeApproval,
		Assignee:      &flow.AssigneeRule{Type: flow.RuleUsers, UserIDs: []uint64{3}},
		MultiMode:     flow.MultiOr,
		AllowBackPrev: true}
	tree := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(approvalUsers("n-a1", "一级", flow.MultiOr, []uint64{2}, lvl2))})
	seedDef(t, st, 1, "flow_prev", tree)

	eff := startInst(t, e, "flow_prev", "biz-1", nil)
	// 一级未开 allowBackPrev → 退回上一节点被拒
	if _, err := e.Return(1, taskOf(t, eff, 2).ID, 2, "prev", "x"); !errors.Is(err, ErrBackPrevNotAllowed) {
		t.Fatalf("未开 allowBackPrev 应拒绝, got %v", err)
	}
	if _, err := e.Approve(1, taskOf(t, eff, 2).ID, 2, ""); err != nil {
		t.Fatalf("lvl1 approve: %v", err)
	}
	pend := pendingTasks(t, st, eff.Instance.ID)
	effR, err := e.Return(1, pend[0].ID, 3, "prev", "一级把关不严，退回重审")
	if err != nil {
		t.Fatalf("return prev: %v", err)
	}
	// 一级 round+1 重展开，游标回一级
	if effR.Instance.CurrentNodeID != "n-a1" {
		t.Fatalf("游标应回 n-a1: %s", effR.Instance.CurrentNodeID)
	}
	if len(effR.NewTasks) != 1 || effR.NewTasks[0].NodeID != "n-a1" ||
		effR.NewTasks[0].Round != 2 || effR.NewTasks[0].AssigneeID != 2 {
		t.Fatalf("重展开任务: %+v", effR.NewTasks)
	}
	if !hasAction(t, st, eff.Instance.ID, model.ActionReturnPrev) {
		t.Fatal("缺少 return_prev 日志")
	}
	// round2 一级 → round2 二级 → approved
	if _, err := e.Approve(1, effR.NewTasks[0].ID, 2, ""); err != nil {
		t.Fatalf("round2 lvl1: %v", err)
	}
	pend = pendingTasks(t, st, eff.Instance.ID)
	if len(pend) != 1 || pend[0].NodeID != "n-a2" || pend[0].Round != 2 {
		t.Fatalf("round2 二级: %+v", pend)
	}
	effF, err := e.Approve(1, pend[0].ID, 3, "")
	if err != nil || effF.Instance.Status != model.InstApproved {
		t.Fatalf("走完应通过: err=%v", err)
	}

	// 场景 B：首个审批节点开 allowBackPrev、无上一审批节点 → 等价退回发起人
	treeB := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(&flow.Node{ID: "n-b1", Name: "唯一审批", Type: flow.TypeApproval,
			Assignee:      &flow.AssigneeRule{Type: flow.RuleUsers, UserIDs: []uint64{2}},
			MultiMode:     flow.MultiOr,
			AllowBackPrev: true})})
	seedDef(t, st, 1, "flow_prev_first", treeB)
	effB := startInst(t, e, "flow_prev_first", "biz-B", nil)
	effBR, err := e.Return(1, taskOf(t, effB, 2).ID, 2, "prev", "退回")
	if err != nil {
		t.Fatalf("first-node return prev: %v", err)
	}
	if len(effBR.NewTasks) != 1 || effBR.NewTasks[0].NodeID != "n-start" ||
		effBR.NewTasks[0].AssigneeID != 1 {
		t.Fatalf("应等价生成重提任务: %+v", effBR.NewTasks)
	}
}

// 撤销放宽（M2）：退回待重提期间即便已有节点通过也允许撤销；
// 正常在途且已有人通过仍拒绝（沿用 M1 从严）。
func TestCancelAfterReturn(t *testing.T) {
	st, e := openTest(t)
	lvl2 := approvalUsers("n-a2", "二级", flow.MultiOr, []uint64{3}, nil)
	tree := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(approvalUsers("n-a1", "一级", flow.MultiOr, []uint64{2}, lvl2))})
	seedDef(t, st, 1, "flow_cancel_ret", tree)

	eff := startInst(t, e, "flow_cancel_ret", "biz-1", nil)
	if _, err := e.Approve(1, taskOf(t, eff, 2).ID, 2, ""); err != nil {
		t.Fatalf("approve: %v", err)
	}
	// 在途且已有人通过 → 拒绝撤销（M1 规则仍有效）
	if _, err := e.Cancel(1, eff.Instance.ID, 1); !errors.Is(err, ErrCancelDenied) {
		t.Fatalf("在途已审应拒绝撤销, got %v", err)
	}
	pend := pendingTasks(t, st, eff.Instance.ID)
	if _, err := e.Return(1, pend[0].ID, 3, "start", "退回"); err != nil {
		t.Fatalf("return: %v", err)
	}
	// 退回待重提 → 放行撤销
	effC, err := e.Cancel(1, eff.Instance.ID, 1)
	if err != nil {
		t.Fatalf("退回后撤销应放行: %v", err)
	}
	if effC.Instance.Status != model.InstCanceled {
		t.Fatalf("撤销终态: %s", effC.Instance.Status)
	}
	if len(pendingTasks(t, st, eff.Instance.ID)) != 0 {
		t.Fatal("撤销后不应有 pending 任务")
	}
}

// onReject=back_to_start（M2）：节点拒绝不终态，退回发起人生成重提任务。
func TestRejectBackToStart(t *testing.T) {
	st, e := openTest(t)
	tree := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(&flow.Node{ID: "n-a1", Name: "审批", Type: flow.TypeApproval,
			Assignee:  &flow.AssigneeRule{Type: flow.RuleUsers, UserIDs: []uint64{2}},
			MultiMode: flow.MultiOr,
			OnReject:  flow.OnRejectBackToStart})})
	seedDef(t, st, 1, "flow_back", tree)

	eff := startInst(t, e, "flow_back", "biz-1", nil)
	effR, err := e.Reject(1, taskOf(t, eff, 2).ID, 2, "材料不对")
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if effR.Instance.Status != model.InstRunning || effR.FinalResult != "" {
		t.Fatalf("back_to_start 拒绝不应终态: %s", effR.Instance.Status)
	}
	if len(effR.NewTasks) != 1 || effR.NewTasks[0].NodeID != "n-start" {
		t.Fatalf("应生成重提任务: %+v", effR.NewTasks)
	}
	// 重提后原节点 round+1 重新出任务
	effS, err := e.Resubmit(1, eff.Instance.ID, 1, nil)
	if err != nil || len(effS.NewTasks) != 1 || effS.NewTasks[0].Round != 2 {
		t.Fatalf("resubmit: err=%v tasks=%+v", err, effS.NewTasks)
	}
}

// dept_leader 解析：基准发起人部门（含发起时未传 dept 的同库补查）、
// 基准表单字段；主管缺失/禁用走 fallback。
func TestDeptLeaderRule(t *testing.T) {
	st, e := openTest(t)
	db := st.DB()
	db.Exec(`INSERT INTO users (id, tenant_id, department_id, status) VALUES
		(1,1,10,1),(7,1,0,1),(8,1,0,1),(9,1,0,0)`)
	db.Exec(`INSERT INTO departments (id, tenant_id, leader_user_id) VALUES
		(10,1,7),(20,1,8),(30,1,0),(40,1,9)`)

	mk := func(key string, rule *flow.AssigneeRule) {
		tree := mustTree(t, &flow.Schema{Version: 1,
			Start: startNode(&flow.Node{ID: "n-a1", Name: "主管审批", Type: flow.TypeApproval,
				Assignee: rule, MultiMode: flow.MultiOr})})
		seedDef(t, st, 1, key, tree)
	}
	// 基准=发起人部门（发起时未传 InitiatorDept，引擎同库补查 users.department_id=10 → 主管 7）
	mk("flow_dl_init", &flow.AssigneeRule{Type: flow.RuleDeptLeader})
	eff := startInst(t, e, "flow_dl_init", "biz-1", nil)
	if len(eff.NewTasks) != 1 || eff.NewTasks[0].AssigneeID != 7 {
		t.Fatalf("发起人部门主管: %+v", eff.NewTasks)
	}
	inst, _ := st.GetInstance(eff.Instance.ID, 1)
	if inst.InitiatorDept != 10 {
		t.Fatalf("发起时应补落 initiator_dept: %d", inst.InitiatorDept)
	}
	// 基准=表单字段
	mk("flow_dl_form", &flow.AssigneeRule{Type: flow.RuleDeptLeader,
		DeptLeaderBase: flow.DeptBaseFormField, DeptFormField: "dept_id"})
	eff2, err := e.Start(StartInput{
		TenantID: 1, DefinitionKey: "flow_dl_form", Title: "表单部门",
		BizType: "demo", BizID: "biz-2",
		FormSnapshot: []byte(`{"dept_id":20}`), InitiatorID: 1,
	})
	if err != nil {
		t.Fatalf("start form_field: %v", err)
	}
	if len(eff2.NewTasks) != 1 || eff2.NewTasks[0].AssigneeID != 8 {
		t.Fatalf("表单部门主管: %+v", eff2.NewTasks)
	}
	// 主管未设（leader_user_id=0）→ 缺省 suspend
	mk("flow_dl_none", &flow.AssigneeRule{Type: flow.RuleDeptLeader,
		DeptLeaderBase: flow.DeptBaseFormField, DeptFormField: "dept_id"})
	eff3, err := e.Start(StartInput{
		TenantID: 1, DefinitionKey: "flow_dl_none", Title: "无主管",
		BizType: "demo", BizID: "biz-3",
		FormSnapshot: []byte(`{"dept_id":30}`), InitiatorID: 1,
	})
	if err != nil || eff3.Instance.Status != model.InstSuspended {
		t.Fatalf("无主管应挂起: err=%v status=%v", err, eff3)
	}
	// 主管已禁用 → 视为空，走 to_users 兜底
	mk("flow_dl_disabled", &flow.AssigneeRule{Type: flow.RuleDeptLeader,
		DeptLeaderBase: flow.DeptBaseFormField, DeptFormField: "dept_id",
		EmptyFallback: flow.FallbackToUsers, FallbackUserIDs: []uint64{7}})
	eff4, err := e.Start(StartInput{
		TenantID: 1, DefinitionKey: "flow_dl_disabled", Title: "主管禁用",
		BizType: "demo", BizID: "biz-4",
		FormSnapshot: []byte(`{"dept_id":40}`), InitiatorID: 1,
	})
	if err != nil || len(eff4.NewTasks) != 1 || eff4.NewTasks[0].AssigneeID != 7 {
		t.Fatalf("禁用主管应走兜底: err=%v tasks=%+v", err, eff4.NewTasks)
	}
}

// 超时扫描：到点任务只提醒一次（条件更新防重），日志 operator=0。
func TestTimeoutRemindOnce(t *testing.T) {
	st, e := openTest(t)
	tree := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(&flow.Node{ID: "n-a1", Name: "限时审批", Type: flow.TypeApproval,
			Assignee:     &flow.AssigneeRule{Type: flow.RuleUsers, UserIDs: []uint64{2}},
			MultiMode:    flow.MultiOr,
			TimeoutHours: 2})})
	seedDef(t, st, 1, "flow_timeout", tree)

	eff := startInst(t, e, "flow_timeout", "biz-1", nil)
	task := taskOf(t, eff, 2)
	if task.TimeoutAt == nil {
		t.Fatal("建任务时应按 timeoutHours 落 timeout_at")
	}
	// 尚未到点 → 不在扫描结果里
	if rows, err := st.ListTimeoutDue(10); err != nil || len(rows) != 0 {
		t.Fatalf("未到点不应命中: err=%v rows=%+v", err, rows)
	}
	// 把 timeout_at 拨到过去
	past := time.Now().Add(-time.Minute)
	if err := st.DB().Model(&model.Task{}).Where("id = ?", task.ID).
		Update("timeout_at", past).Error; err != nil {
		t.Fatalf("拨时间: %v", err)
	}
	rows, err := st.ListTimeoutDue(10)
	if err != nil || len(rows) != 1 || rows[0].ID != task.ID || rows[0].InstanceTitle == "" {
		t.Fatalf("到点应命中: err=%v rows=%+v", err, rows)
	}
	first, err := st.MarkTaskReminded(rows[0], 2)
	if err != nil || !first {
		t.Fatalf("首次标记应成功: %v %v", first, err)
	}
	// 只提醒一次：再扫描无命中，重复标记返回 false
	if rows2, _ := st.ListTimeoutDue(10); len(rows2) != 0 {
		t.Fatalf("已提醒不应再命中: %+v", rows2)
	}
	if again, err := st.MarkTaskReminded(rows[0], 2); err != nil || again {
		t.Fatalf("重复标记应为 false: %v %v", again, err)
	}
	if !hasAction(t, st, eff.Instance.ID, model.ActionTimeoutRemind) {
		t.Fatal("缺少 timeout_remind 日志")
	}
	logs, _ := st.ListInstanceLogs(eff.Instance.ID, 1)
	for _, l := range logs {
		if l.Action == model.ActionTimeoutRemind && l.OperatorID != 0 {
			t.Fatalf("超时提醒日志 operator 应为 0: %+v", l)
		}
	}
	// 提醒不影响正常审批
	if effA, err := e.Approve(1, task.ID, 2, ""); err != nil || effA.Instance.Status != model.InstApproved {
		t.Fatalf("提醒后审批: %v", err)
	}
}
