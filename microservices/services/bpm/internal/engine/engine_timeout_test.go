package engine

// 收官项用例：超时自动通过 / 自动拒绝 / 缺省提醒策略（HandleTimeout）。

import (
	"testing"

	"github.com/go-admin-kit/services/bpm/internal/flow"
	"github.com/go-admin-kit/services/bpm/internal/model"
)

// timeoutTree 单审批节点（users=[2]）+ 指定超时动作，后接财务审批（users=[4]）。
func timeoutTree(t *testing.T, action string) []byte {
	t.Helper()
	next := approvalUsers("n-next", "财务审批", flow.MultiOr, []uint64{4}, nil)
	n := approvalUsers("n-a1", "经理审批", flow.MultiOr, []uint64{2}, next)
	n.TimeoutHours = 1
	n.TimeoutAction = action
	return mustTree(t, &flow.Schema{Version: 1, Start: startNode(n)})
}

func expireTask(t *testing.T, st interface {
	DB() interface {
		Exec(string, ...any) interface{ Error() error }
	}
}, taskID uint64) {
	t.Helper()
	_ = st
	_ = taskID
}

// 超时自动通过：任务置 approved（系统），推进到下一节点并产出新待办。
func TestTimeoutAutoPass(t *testing.T) {
	st, e := openTest(t)
	seedDef(t, st, 1, "flow_to_pass", timeoutTree(t, flow.TimeoutAutoPass))
	eff := startInst(t, e, "flow_to_pass", "biz-tp", nil)
	task := taskOf(t, eff, 2)

	// 模拟到期（ticker 按 timeout_at 扫描，这里直接调处理入口）
	outcome, eff2, err := e.HandleTimeout(1, task.ID)
	if err != nil {
		t.Fatalf("HandleTimeout: %v", err)
	}
	if outcome != TimeoutOutcomePass {
		t.Fatalf("应 auto_pass, got %s", outcome)
	}
	if eff2 == nil || taskOf(t, eff2, 4) == nil {
		t.Fatalf("自动通过后应产出下一节点待办: %+v", eff2)
	}
	got, _ := st.GetTask(task.ID, 1)
	if got.Status != model.TaskApproved || got.Comment != "超时自动通过" {
		t.Fatalf("任务应系统通过: %+v", got)
	}
	logs, _ := st.ListInstanceLogs(eff.Instance.ID, 1)
	found := false
	for _, lg := range logs {
		if lg.Action == model.ActionTimeoutPass && lg.OperatorID == 0 {
			found = true
		}
	}
	if !found {
		t.Fatal("应有 timeout_pass 系统日志")
	}
}

// 超时自动拒绝：节点拒绝 → 实例终态 rejected（onReject 缺省）。
func TestTimeoutAutoReject(t *testing.T) {
	st, e := openTest(t)
	seedDef(t, st, 1, "flow_to_reject", timeoutTree(t, flow.TimeoutAutoReject))
	eff := startInst(t, e, "flow_to_reject", "biz-tr", nil)
	task := taskOf(t, eff, 2)

	outcome, eff2, err := e.HandleTimeout(1, task.ID)
	if err != nil {
		t.Fatalf("HandleTimeout: %v", err)
	}
	if outcome != TimeoutOutcomeReject || eff2 == nil || eff2.FinalResult != model.InstRejected {
		t.Fatalf("应 auto_reject 且终态 rejected, got %s %+v", outcome, eff2)
	}
	inst, _ := st.GetInstance(eff.Instance.ID, 1)
	if inst.Status != model.InstRejected {
		t.Fatalf("实例应 rejected, got %s", inst.Status)
	}
}

// 缺省 / remind 策略：返回 remind，由调用方走提醒路径；已处理任务返回 skip。
func TestTimeoutRemindAndSkip(t *testing.T) {
	st, e := openTest(t)
	seedDef(t, st, 1, "flow_to_remind", timeoutTree(t, ""))
	eff := startInst(t, e, "flow_to_remind", "biz-trm", nil)
	task := taskOf(t, eff, 2)

	if outcome, _, _ := e.HandleTimeout(1, task.ID); outcome != TimeoutOutcomeRemind {
		t.Fatalf("缺省应 remind, got %s", outcome)
	}
	// 人工处理后再触发 → skip
	if _, err := e.Approve(1, task.ID, 2, ""); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if outcome, _, _ := e.HandleTimeout(1, task.ID); outcome != TimeoutOutcomeSkip {
		t.Fatalf("已处理任务应 skip, got %s", outcome)
	}
	_ = st

	// 发布校验：配自动动作但没配小时数应拒绝
	bad := approvalUsers("n-bad", "审批", flow.MultiOr, []uint64{2}, nil)
	bad.TimeoutAction = flow.TimeoutAutoPass
	if err := flow.Validate(&flow.Schema{Version: 1, Start: startNode(bad)}); err == nil {
		t.Fatal("自动动作缺小时数应校验失败")
	}
}
