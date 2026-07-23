package engine

// M3+ 加签 / 委派用例：并加签入既有收敛（AND 等新人 / OR 可被跳过）、
// SEQ 拒加、重复加人拒绝、委派→办结→原人审批全链路、委派中动作限制、
// 委派中被或签兄弟跳过（无特判）。复用 engine_test.go 基架与 m2 辅助函数。

import (
	"errors"
	"testing"

	"github.com/go-admin-kit/services/bpm/internal/flow"
	"github.com/go-admin-kit/services/bpm/internal/model"
	"github.com/go-admin-kit/services/bpm/internal/store"
)

// AND 会签加签：原有二人全同意后实例仍等待加签人，加签人同意才通过；
// 新任务同 node/round、AddSignBy 记操作人、沿用 AND 模式。
func TestAddSignAndWaitsNewApprover(t *testing.T) {
	st, e := openTest(t)
	tree := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(approvalUsers("n-a1", "会签", flow.MultiAnd, []uint64{2, 3}, nil))})
	seedDef(t, st, 1, "flow_as_and", tree)

	eff := startInst(t, e, "flow_as_and", "biz-1", nil)
	t2 := taskOf(t, eff, 2)
	effA, err := e.AddSign(1, t2.ID, 2, []uint64{5}, "拉总监一起看")
	if err != nil {
		t.Fatalf("add sign: %v", err)
	}
	if len(effA.NewTasks) != 1 {
		t.Fatalf("加签应展开 1 任务: %+v", effA.NewTasks)
	}
	nt := effA.NewTasks[0]
	if nt.AssigneeID != 5 || nt.NodeID != t2.NodeID || nt.Round != t2.Round ||
		nt.MultiMode != flow.MultiAnd || nt.AddSignBy != 2 {
		t.Fatalf("加签任务字段: %+v", nt)
	}
	if !hasAction(t, st, eff.Instance.ID, model.ActionAddSign) {
		t.Fatal("缺少 add_sign 日志")
	}
	// 原有二人全同意后仍需等加签人
	if _, err := e.Approve(1, t2.ID, 2, ""); err != nil {
		t.Fatalf("approve 2: %v", err)
	}
	eff3, err := e.Approve(1, taskOf(t, eff, 3).ID, 3, "")
	if err != nil {
		t.Fatalf("approve 3: %v", err)
	}
	if eff3.Instance.Status != model.InstRunning {
		t.Fatalf("加签人未同意前应继续等待: %s", eff3.Instance.Status)
	}
	eff5, err := e.Approve(1, nt.ID, 5, "补充同意")
	if err != nil {
		t.Fatalf("approve 5: %v", err)
	}
	if eff5.Instance.Status != model.InstApproved {
		t.Fatalf("加签人同意后应通过: %s", eff5.Instance.Status)
	}
}

// OR 或签加签：原人同意即节点通过，加签任务被置 skipped。
func TestAddSignOrConverges(t *testing.T) {
	st, e := openTest(t)
	tree := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(approvalUsers("n-a1", "或签", flow.MultiOr, []uint64{2}, nil))})
	seedDef(t, st, 1, "flow_as_or", tree)

	eff := startInst(t, e, "flow_as_or", "biz-1", nil)
	t2 := taskOf(t, eff, 2)
	effA, err := e.AddSign(1, t2.ID, 2, []uint64{5}, "")
	if err != nil {
		t.Fatalf("add sign: %v", err)
	}
	eff2, err := e.Approve(1, t2.ID, 2, "")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if eff2.Instance.Status != model.InstApproved {
		t.Fatalf("或签原人同意应通过: %s", eff2.Instance.Status)
	}
	var nt model.Task
	if err := st.DB().First(&nt, effA.NewTasks[0].ID).Error; err != nil {
		t.Fatalf("load add-sign task: %v", err)
	}
	if nt.Status != model.TaskSkipped {
		t.Fatalf("或签通过后加签任务应 skipped: %+v", nt)
	}
}

// 加签校验：SEQ 节点拒加、空目标、目标全为已在审人（含自己）、重提任务拒加。
func TestAddSignValidation(t *testing.T) {
	st, e := openTest(t)
	// SEQ 节点
	seqTree := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(approvalUsers("n-a1", "依次", flow.MultiSeq, []uint64{2, 3}, nil))})
	seedDef(t, st, 1, "flow_as_seq", seqTree)
	effS := startInst(t, e, "flow_as_seq", "biz-s", nil)
	if _, err := e.AddSign(1, taskOf(t, effS, 2).ID, 2, []uint64{5}, ""); !errors.Is(err, ErrAddSignSeq) {
		t.Fatalf("SEQ 加签应拒绝, got %v", err)
	}

	// AND 节点：空目标 / 重复目标
	tree := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(approvalUsers("n-a1", "会签", flow.MultiAnd, []uint64{2, 3}, nil))})
	seedDef(t, st, 1, "flow_as_val", tree)
	eff := startInst(t, e, "flow_as_val", "biz-1", nil)
	t2 := taskOf(t, eff, 2)
	if _, err := e.AddSign(1, t2.ID, 2, nil, ""); !errors.Is(err, ErrAddSignTarget) {
		t.Fatalf("空目标应拒绝, got %v", err)
	}
	if _, err := e.AddSign(1, t2.ID, 2, []uint64{0}, ""); !errors.Is(err, ErrAddSignTarget) {
		t.Fatalf("全 0 目标应拒绝, got %v", err)
	}
	// 目标全是同节点在审人（自己 + 兄弟）→ 重复
	if _, err := e.AddSign(1, t2.ID, 2, []uint64{2, 3}, ""); !errors.Is(err, ErrAddSignDuplicate) {
		t.Fatalf("重复加人应拒绝, got %v", err)
	}
	// 部分重复：只加未在审的 5
	effA, err := e.AddSign(1, t2.ID, 2, []uint64{3, 5}, "")
	if err != nil {
		t.Fatalf("部分重复应只过滤: %v", err)
	}
	if len(effA.NewTasks) != 1 || effA.NewTasks[0].AssigneeID != 5 {
		t.Fatalf("应只新增 5: %+v", effA.NewTasks)
	}

	// 重提任务（start 节点）不支持加签
	rtTree := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(approvalUsers("n-a1", "审批", flow.MultiOr, []uint64{2}, nil))})
	seedDef(t, st, 1, "flow_as_rt", rtTree)
	effR := startInst(t, e, "flow_as_rt", "biz-r", nil)
	if _, err := e.Return(1, taskOf(t, effR, 2).ID, 2, "start", "退回补材料"); err != nil {
		t.Fatalf("return: %v", err)
	}
	rts := pendingTasks(t, st, effR.Instance.ID)
	if len(rts) != 1 {
		t.Fatalf("退回后应有 1 个重提任务: %+v", rts)
	}
	if _, err := e.AddSign(1, rts[0].ID, 1, []uint64{5}, ""); !errors.Is(err, ErrReturnStartTask) {
		t.Fatalf("重提任务加签应拒绝, got %v", err)
	}
}

// 委派全链路：A 委派 B → 待办/已办可见性切换 → B 办结（意见落日志）→
// 任务回 A（字段还原）→ A 同意走完。
func TestDelegateResolveThenApprove(t *testing.T) {
	st, e := openTest(t)
	tree := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(approvalUsers("n-a1", "审批", flow.MultiOr, []uint64{2}, nil))})
	seedDef(t, st, 1, "flow_dlg", tree)

	eff := startInst(t, e, "flow_dlg", "biz-1", nil)
	t2 := taskOf(t, eff, 2)
	effD, err := e.Delegate(1, t2.ID, 2, 5, "帮我核实数据")
	if err != nil {
		t.Fatalf("delegate: %v", err)
	}
	if len(effD.DelegatedTasks) != 1 || effD.DelegatedTasks[0].AssigneeID != 5 ||
		effD.DelegatedTasks[0].DelegatedBy != 2 {
		t.Fatalf("委派 Effects: %+v", effD.DelegatedTasks)
	}
	// 待办：A 消失、B 可见；A 已办可追踪（delegated_by 分支）
	todoA, _, _ := st.ListTodo(1, 2, store.Page{})
	if len(todoA) != 0 {
		t.Fatalf("委派后 A 待办应为空: %+v", todoA)
	}
	todoB, _, _ := st.ListTodo(1, 5, store.Page{})
	if len(todoB) != 1 || todoB[0].DelegatedBy != 2 {
		t.Fatalf("委派后 B 待办: %+v", todoB)
	}
	doneA, _, _ := st.ListDone(1, 2, store.Page{})
	if len(doneA) != 1 {
		t.Fatalf("委派期间 A 已办应可追踪: %+v", doneA)
	}

	// B 办结：意见必填、任务回 A、delegate_resolved_by 记 B
	if _, err := e.ResolveDelegate(1, t2.ID, 5, " "); !errors.Is(err, ErrDelegateComment) {
		t.Fatalf("空意见办结应拒绝, got %v", err)
	}
	effR, err := e.ResolveDelegate(1, t2.ID, 5, "数据已核实无误")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(effR.DelegateResolvedTasks) != 1 || effR.DelegateResolvedTasks[0].AssigneeID != 2 {
		t.Fatalf("办结 Effects 应发给 A: %+v", effR.DelegateResolvedTasks)
	}
	var row model.Task
	if err := st.DB().First(&row, t2.ID).Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if row.AssigneeID != 2 || row.DelegatedBy != 0 || row.DelegateResolvedBy != 5 ||
		row.Status != model.TaskPending {
		t.Fatalf("办结后任务字段: %+v", row)
	}
	// B 已办可见（delegate_resolved_by 分支）
	doneB, _, _ := st.ListDone(1, 5, store.Page{})
	if len(doneB) != 1 {
		t.Fatalf("办结后 B 已办应可见: %+v", doneB)
	}
	if !hasAction(t, st, eff.Instance.ID, model.ActionDelegate) ||
		!hasAction(t, st, eff.Instance.ID, model.ActionDelegateResolve) {
		t.Fatal("缺少 delegate / delegate_resolve 日志")
	}
	// A 继续同意走完
	eff2, err := e.Approve(1, t2.ID, 2, "同意")
	if err != nil {
		t.Fatalf("approve after resolve: %v", err)
	}
	if eff2.Instance.Status != model.InstApproved {
		t.Fatalf("办结后 A 同意应通过: %s", eff2.Instance.Status)
	}
}

// 委派中动作限制：B 只能办结（approve/reject/transfer/return/加签/再委派
// 全拒）；A 非 assignee 不可操作；自委派 / 非委派办结拒绝。
func TestDelegatedTaskActionLimits(t *testing.T) {
	st, e := openTest(t)
	tree := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(approvalUsers("n-a1", "审批", flow.MultiOr, []uint64{2}, nil))})
	seedDef(t, st, 1, "flow_dlg_lim", tree)

	eff := startInst(t, e, "flow_dlg_lim", "biz-1", nil)
	t2 := taskOf(t, eff, 2)
	if _, err := e.Delegate(1, t2.ID, 2, 2, ""); !errors.Is(err, ErrDelegateSelf) {
		t.Fatalf("自委派应拒绝, got %v", err)
	}
	if _, err := e.Delegate(1, t2.ID, 2, 0, ""); !errors.Is(err, ErrDelegateTarget) {
		t.Fatalf("空目标应拒绝, got %v", err)
	}
	if _, err := e.ResolveDelegate(1, t2.ID, 2, "意见"); !errors.Is(err, ErrNotDelegated) {
		t.Fatalf("非委派任务办结应拒绝, got %v", err)
	}
	if _, err := e.Delegate(1, t2.ID, 2, 5, ""); err != nil {
		t.Fatalf("delegate: %v", err)
	}
	// B 的受限动作
	if _, err := e.Approve(1, t2.ID, 5, ""); !errors.Is(err, ErrTaskDelegated) {
		t.Fatalf("委派中 approve 应拒绝, got %v", err)
	}
	if _, err := e.Reject(1, t2.ID, 5, "拒"); !errors.Is(err, ErrTaskDelegated) {
		t.Fatalf("委派中 reject 应拒绝, got %v", err)
	}
	if _, err := e.Transfer(1, t2.ID, 5, 6, ""); !errors.Is(err, ErrTaskDelegated) {
		t.Fatalf("委派中 transfer 应拒绝, got %v", err)
	}
	if _, err := e.Return(1, t2.ID, 5, "start", "退"); !errors.Is(err, ErrTaskDelegated) {
		t.Fatalf("委派中 return 应拒绝, got %v", err)
	}
	if _, err := e.AddSign(1, t2.ID, 5, []uint64{6}, ""); !errors.Is(err, ErrTaskDelegated) {
		t.Fatalf("委派中加签应拒绝, got %v", err)
	}
	if _, err := e.Delegate(1, t2.ID, 5, 6, ""); !errors.Is(err, ErrTaskDelegated) {
		t.Fatalf("链式委派应拒绝, got %v", err)
	}
	// A 已非 assignee
	if _, err := e.Approve(1, t2.ID, 2, ""); !errors.Is(err, ErrNotAssignee) {
		t.Fatalf("委派后 A 操作应拒绝, got %v", err)
	}
}

// OR 下委派中被兄弟同意跳过：无需特判，skipPending 盲更新覆盖委派中任务。
func TestDelegatedSkippedByOrSibling(t *testing.T) {
	st, e := openTest(t)
	tree := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(approvalUsers("n-a1", "或签", flow.MultiOr, []uint64{2, 3}, nil))})
	seedDef(t, st, 1, "flow_dlg_skip", tree)

	eff := startInst(t, e, "flow_dlg_skip", "biz-1", nil)
	t2 := taskOf(t, eff, 2)
	if _, err := e.Delegate(1, t2.ID, 2, 5, ""); err != nil {
		t.Fatalf("delegate: %v", err)
	}
	eff3, err := e.Approve(1, taskOf(t, eff, 3).ID, 3, "")
	if err != nil {
		t.Fatalf("sibling approve: %v", err)
	}
	if eff3.Instance.Status != model.InstApproved {
		t.Fatalf("或签兄弟同意应通过: %s", eff3.Instance.Status)
	}
	var row model.Task
	if err := st.DB().First(&row, t2.ID).Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if row.Status != model.TaskSkipped {
		t.Fatalf("委派中任务应被跳过: %+v", row)
	}
	// 实例已终态，B 办结被实例状态拦截
	if _, err := e.ResolveDelegate(1, t2.ID, 5, "办完了"); !errors.Is(err, ErrInstanceNotRunning) {
		t.Fatalf("终态后办结应报实例不在审批中, got %v", err)
	}
}
