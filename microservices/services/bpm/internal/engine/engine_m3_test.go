package engine

// M3 用例：条件分支路由（命中/默认/汇合）/ 求值失败挂起 / SEQ 依次审批 /
// 管理员终止 / 分支内退回上一节点（执行路径回溯）。

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/go-admin-kit/services/bpm/internal/flow"
	"github.com/go-admin-kit/services/bpm/internal/model"
)

// condTree 构造：start → 条件分支（≥5万 → 总监审批[2]；默认 → 经理审批[3]）
// → 汇合后财务审批[4]。
func condTree(t *testing.T) []byte {
	t.Helper()
	cond := &flow.Node{
		ID: "n-cond", Name: "金额分流", Type: flow.TypeCondition,
		Branches: []flow.Branch{
			{ID: "b-hi", Name: "金额≥5万",
				Expr: []byte(`{"op":"gte","field":"amount_cents","value":5000000}`),
				Next: approvalUsers("n-hi", "总监审批", flow.MultiOr, []uint64{2}, nil)},
			{ID: "b-lo", Name: "默认",
				Next: approvalUsers("n-lo", "经理审批", flow.MultiOr, []uint64{3}, nil)},
		},
		Next: approvalUsers("n-join", "财务审批", flow.MultiOr, []uint64{4}, nil),
	}
	return mustTree(t, &flow.Schema{Version: 1, Start: startNode(cond)})
}

func startInstAmount(t *testing.T, e *Engine, key, bizID string, amountCents int64) *Effects {
	t.Helper()
	eff, err := e.Start(StartInput{
		TenantID: 1, DefinitionKey: key, Title: "测试单 " + bizID,
		BizType: "demo", BizID: bizID,
		FormSnapshot: []byte(`{"amount_cents":` + jsonInt(amountCents) + `}`),
		InitiatorID:  1,
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	return eff
}

func jsonInt(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// 条件分支：高额走总监链，低额走默认链，分支走完汇合到财务审批。
func TestConditionBranchRouting(t *testing.T) {
	st, e := openTest(t)
	seedDef(t, st, 1, "flow_cond", condTree(t))

	// 高额 → 命中 b-hi → 总监(2)
	hi := startInstAmount(t, e, "flow_cond", "biz-hi", 8000000)
	if hi.Instance.CurrentNodeID != "n-hi" {
		t.Fatalf("高额应停在总监节点, got %s", hi.Instance.CurrentNodeID)
	}
	// 总监同意 → 汇合到财务(4)
	eff, err := e.Approve(1, taskOf(t, hi, 2).ID, 2, "同意")
	if err != nil {
		t.Fatalf("approve hi: %v", err)
	}
	if eff.Instance.CurrentNodeID != "n-join" || taskOf(t, eff, 4) == nil {
		t.Fatalf("总监通过后应汇合到财务节点, got %s", eff.Instance.CurrentNodeID)
	}
	// 财务同意 → 终态 approved
	eff, err = e.Approve(1, taskOf(t, eff, 4).ID, 4, "")
	if err != nil {
		t.Fatalf("approve join: %v", err)
	}
	if eff.FinalResult != model.InstApproved {
		t.Fatalf("应终态 approved, got %q", eff.FinalResult)
	}

	// 低额 → 默认分支 → 经理(3)
	lo := startInstAmount(t, e, "flow_cond", "biz-lo", 100)
	if lo.Instance.CurrentNodeID != "n-lo" || taskOf(t, lo, 3) == nil {
		t.Fatalf("低额应停在经理节点, got %s", lo.Instance.CurrentNodeID)
	}

	// 分支命中写 branch 日志
	logs, _ := st.ListInstanceLogs(hi.Instance.ID, 1)
	found := false
	for _, lg := range logs {
		if lg.Action == model.ActionBranch {
			found = true
		}
	}
	if !found {
		t.Fatal("应有 branch 分支命中日志")
	}
}

// 条件求值失败（快照缺字段）→ 实例挂起而非静默走 default。
func TestConditionEvalFailureSuspends(t *testing.T) {
	st, e := openTest(t)
	seedDef(t, st, 1, "flow_cond_bad", condTree(t))

	eff, err := e.Start(StartInput{
		TenantID: 1, DefinitionKey: "flow_cond_bad", Title: "缺字段单",
		BizType: "demo", BizID: "biz-bad",
		FormSnapshot: []byte(`{"other":1}`), InitiatorID: 1,
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if eff.Instance.Status != model.InstSuspended || eff.Instance.CurrentNodeID != "n-cond" {
		t.Fatalf("求值失败应挂起在条件节点, got %s @%s",
			eff.Instance.Status, eff.Instance.CurrentNodeID)
	}
	if len(eff.NewTasks) != 0 {
		t.Fatalf("挂起不应展开任务: %+v", eff.NewTasks)
	}
}

// SEQ 依次：只出首位任务，逐个同意逐个补建，全部走完节点通过；中途拒绝即节点拒绝。
func TestSeqApproval(t *testing.T) {
	st, e := openTest(t)
	tree := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(approvalUsers("n-seq", "依次审批", flow.MultiSeq, []uint64{2, 3, 4}, nil))})
	seedDef(t, st, 1, "flow_seq", tree)

	eff := startInst(t, e, "flow_seq", "biz-seq", nil)
	if len(eff.NewTasks) != 1 || eff.NewTasks[0].AssigneeID != 2 || eff.NewTasks[0].SeqOrder != 0 {
		t.Fatalf("依次应只展开首位(2): %+v", eff.NewTasks)
	}
	// 2 同意 → 补建 3
	eff, err := e.Approve(1, eff.NewTasks[0].ID, 2, "")
	if err != nil {
		t.Fatalf("approve 2: %v", err)
	}
	if len(eff.NewTasks) != 1 || eff.NewTasks[0].AssigneeID != 3 || eff.NewTasks[0].SeqOrder != 1 {
		t.Fatalf("2 同意后应补建 3 的任务: %+v", eff.NewTasks)
	}
	// 3 同意 → 补建 4；4 同意 → 节点通过 → 终态
	eff, err = e.Approve(1, eff.NewTasks[0].ID, 3, "")
	if err != nil {
		t.Fatalf("approve 3: %v", err)
	}
	eff, err = e.Approve(1, eff.NewTasks[0].ID, 4, "")
	if err != nil {
		t.Fatalf("approve 4: %v", err)
	}
	if eff.FinalResult != model.InstApproved {
		t.Fatalf("全部顺位走完应终态 approved, got %q", eff.FinalResult)
	}

	// 中途拒绝：新实例 2 同意后 3 拒绝 → 实例 rejected，且不再出 4 的任务
	eff2 := startInst(t, e, "flow_seq", "biz-seq2", nil)
	eff2, err = e.Approve(1, eff2.NewTasks[0].ID, 2, "")
	if err != nil {
		t.Fatalf("approve2 2: %v", err)
	}
	eff2, err = e.Reject(1, eff2.NewTasks[0].ID, 3, "不同意")
	if err != nil {
		t.Fatalf("reject 3: %v", err)
	}
	if eff2.FinalResult != model.InstRejected || len(eff2.NewTasks) != 0 {
		t.Fatalf("依次中途拒绝应终态 rejected 且不再补建: %q %+v",
			eff2.FinalResult, eff2.NewTasks)
	}
}

// 管理员终止：原因必填；running 可终止（pending 全作废、回调 canceled、
// 终态文案"已终止"）；已结束实例不可再终止。
func TestTerminate(t *testing.T) {
	st, e := openTest(t)
	tree := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(approvalUsers("n-a1", "审批", flow.MultiAnd, []uint64{2, 3}, nil))})
	seedDef(t, st, 1, "flow_term", tree)
	eff := startInst(t, e, "flow_term", "biz-term", nil)

	if _, err := e.Terminate(1, eff.Instance.ID, 99, "  "); !errors.Is(err, ErrTerminateReason) {
		t.Fatalf("空原因应拒绝, got %v", err)
	}
	got, err := e.Terminate(1, eff.Instance.ID, 99, "流程配置有误，管理员终止")
	if err != nil {
		t.Fatalf("terminate: %v", err)
	}
	if got.Instance.Status != model.InstCanceled || got.FinalResult != model.InstCanceled {
		t.Fatalf("终止后应 canceled, got %s / %q", got.Instance.Status, got.FinalResult)
	}
	if got.ResultText != "已终止" {
		t.Fatalf("终态文案应为已终止, got %q", got.ResultText)
	}
	tasks, _ := st.ListInstanceTasks(eff.Instance.ID, 1)
	for _, tk := range tasks {
		if tk.Status == model.TaskPending {
			t.Fatalf("终止后不应有 pending 任务: %+v", tk)
		}
	}
	if _, err := e.Terminate(1, eff.Instance.ID, 99, "再来一次"); !errors.Is(err, ErrInstanceFinished) {
		t.Fatalf("已结束实例应拒绝终止, got %v", err)
	}
}

// 分支内退回上一节点：执行路径回溯——分支内首个审批节点退回时应回到
// condition 之前最近的已执行审批节点（round+1 重新展开）。
func TestReturnPrevAcrossBranch(t *testing.T) {
	st, e := openTest(t)
	inner := approvalUsers("n-in", "分支内审批", flow.MultiOr, []uint64{3}, nil)
	inner.AllowBackPrev = true
	cond := &flow.Node{
		ID: "n-cond", Name: "分流", Type: flow.TypeCondition,
		Branches: []flow.Branch{
			{ID: "b1", Name: "命中",
				Expr: []byte(`{"op":"gte","field":"amount_cents","value":1}`),
				Next: inner},
			{ID: "b2", Name: "默认"},
		},
	}
	tree := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(approvalUsers("n-first", "初审", flow.MultiOr, []uint64{2}, cond))})
	seedDef(t, st, 1, "flow_backprev", tree)

	eff := startInst(t, e, "flow_backprev", "biz-bp", nil)
	eff, err := e.Approve(1, taskOf(t, eff, 2).ID, 2, "")
	if err != nil {
		t.Fatalf("approve first: %v", err)
	}
	if eff.Instance.CurrentNodeID != "n-in" {
		t.Fatalf("应进入分支内审批, got %s", eff.Instance.CurrentNodeID)
	}
	// 分支内退回上一节点 → 初审 round 2 重新展开
	eff, err = e.Return(1, taskOf(t, eff, 3).ID, 3, "prev", "材料不全")
	if err != nil {
		t.Fatalf("return prev: %v", err)
	}
	if eff.Instance.CurrentNodeID != "n-first" {
		t.Fatalf("应退回初审节点, got %s", eff.Instance.CurrentNodeID)
	}
	nt := taskOf(t, eff, 2)
	if nt.Round != 2 {
		t.Fatalf("初审应 round+1 重新展开, got round=%d", nt.Round)
	}
	// 初审再次通过 → 再进分支（分支内审批 round 2）
	eff, err = e.Approve(1, nt.ID, 2, "")
	if err != nil {
		t.Fatalf("re-approve first: %v", err)
	}
	if eff.Instance.CurrentNodeID != "n-in" || taskOf(t, eff, 3).Round != 2 {
		t.Fatalf("重审后应再次进入分支内审批 round2, got %s", eff.Instance.CurrentNodeID)
	}
}

// 空分支（default 无子链）直通汇合点：条件节点为链尾时直接终态 approved。
func TestEmptyDefaultBranchJoins(t *testing.T) {
	st, e := openTest(t)
	cond := &flow.Node{
		ID: "n-cond", Name: "分流", Type: flow.TypeCondition,
		Branches: []flow.Branch{
			{ID: "b1", Name: "命中",
				Expr: []byte(`{"op":"gte","field":"amount_cents","value":99999999}`),
				Next: approvalUsers("n-x", "高额审批", flow.MultiOr, []uint64{2}, nil)},
			{ID: "b2", Name: "默认"}, // 空子链
		},
	}
	tree := mustTree(t, &flow.Schema{Version: 1,
		Start: startNode(approvalUsers("n-a", "初审", flow.MultiOr, []uint64{2}, cond))})
	seedDef(t, st, 1, "flow_empty_branch", tree)

	eff := startInst(t, e, "flow_empty_branch", "biz-eb", nil) // 金额 10 万分，未命中 b1
	eff, err := e.Approve(1, taskOf(t, eff, 2).ID, 2, "")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if eff.FinalResult != model.InstApproved {
		t.Fatalf("默认空分支应直通链尾终态 approved, got %q", eff.FinalResult)
	}
}
