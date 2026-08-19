package engine

import (
	"github.com/go-admin-kit/services/bpm/internal/flow"
	"github.com/go-admin-kit/services/bpm/internal/model"
	"gorm.io/gorm"
	"strings"
)

// Approve 同意：任务置 approved → 节点计数收敛 → 通过则推进游标。
func (e *Engine) Approve(tenantID, taskID, userID uint64, comment string) (*Effects, error) {
	eff := &Effects{}
	err := e.db.Transaction(func(tx *gorm.DB) error {
		task, inst, err := e.lockTaskAndInstance(tx, tenantID, taskID, userID)
		if err != nil {
			return err
		}
		if task.DelegatedBy != 0 { // 委派办理中：受托人只能办理完成
			return ErrTaskDelegated
		}
		sc, node, err := e.loadNode(tx, inst, task.NodeID)
		if err != nil {
			return err
		}
		if node.Type == flow.TypeStart { // 重提任务只能走 Resubmit / Cancel
			return ErrReturnStartTask
		}
		if err := markTask(tx, task.ID, model.TaskApproved, comment); err != nil {
			return err
		}
		eff.Instance = inst
		writeLog(tx, inst, task.NodeID, task.ID, model.ActionApprove, userID,
			map[string]any{"comment": comment})
		return e.settleApproved(tx, inst, sc, node, task, eff)
	})
	if err != nil {
		return nil, err
	}
	return eff, nil
}

// Reject 拒绝：任务置 rejected → 按计数规则判定节点是否拒绝 → 节点拒绝时按
// 节点 onReject 分派：reject（缺省）→ 实例终态 rejected；back_to_start（M2）
// → 走退回发起人流程（实例仍 running，生成重提任务）。
func (e *Engine) Reject(tenantID, taskID, userID uint64, comment string) (*Effects, error) {
	if strings.TrimSpace(comment) == "" {
		return nil, ErrCommentRequired
	}
	eff := &Effects{}
	err := e.db.Transaction(func(tx *gorm.DB) error {
		task, inst, err := e.lockTaskAndInstance(tx, tenantID, taskID, userID)
		if err != nil {
			return err
		}
		if task.DelegatedBy != 0 {
			return ErrTaskDelegated
		}
		sc, node, err := e.loadNode(tx, inst, task.NodeID)
		if err != nil {
			return err
		}
		if node.Type == flow.TypeStart {
			return ErrReturnStartTask
		}
		if err := markTask(tx, task.ID, model.TaskRejected, comment); err != nil {
			return err
		}
		eff.Instance = inst
		writeLog(tx, inst, task.NodeID, task.ID, model.ActionReject, userID,
			map[string]any{"comment": comment})
		return e.settleRejected(tx, inst, sc, node, task, userID, comment, eff)
	})
	if err != nil {
		return nil, err
	}
	return eff, nil
}

// Transfer 转办：任务换处理人（origin_assignee 记转出人）、保持 pending、
// 重发待办通知。不改变节点计数（换人不换任务，round 不变）。
func (e *Engine) Transfer(tenantID, taskID, userID, targetUserID uint64, comment string) (*Effects, error) {
	if targetUserID == 0 {
		return nil, ErrTransferTarget
	}
	if targetUserID == userID {
		return nil, ErrTransferSelf
	}
	eff := &Effects{}
	err := e.db.Transaction(func(tx *gorm.DB) error {
		task, inst, err := e.lockTaskAndInstance(tx, tenantID, taskID, userID)
		if err != nil {
			return err
		}
		if task.DelegatedBy != 0 {
			return ErrTaskDelegated
		}
		_, node, err := e.loadNode(tx, inst, task.NodeID)
		if err != nil {
			return err
		}
		if node.Type == flow.TypeStart {
			return ErrReturnStartTask
		}
		// 目标人有效性不做存在性校验（与 users 规则同口径：ID 由前端选人
		// 组件保证）；仅拦自转。
		res := tx.Model(&model.Task{}).
			Where("id = ? AND status = ?", task.ID, model.TaskPending).
			Updates(map[string]any{"assignee_id": targetUserID, "origin_assignee": userID})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrTaskHandled
		}
		eff.Instance = inst
		writeLog(tx, inst, task.NodeID, task.ID, model.ActionTransfer, userID,
			map[string]any{"target_user_id": targetUserID, "from_user_id": userID, "comment": comment})
		task.AssigneeID = targetUserID
		task.OriginAssignee = userID
		eff.NewTasks = append(eff.NewTasks, *task) // 重发 bpm.task_assigned 给新处理人
		return nil
	})
	if err != nil {
		return nil, err
	}
	return eff, nil
}

// AddSign 并加签：往当前节点当前 round 增加审批人，新任务沿用节点
// MultiMode 参与既有收敛（AND 全过才过 / OR 一人过即过）；SEQ 不支持。
// 不动游标不调收敛——settleApproved 按 nodeTasks 任务集判定，插行天然纳入。
func (e *Engine) AddSign(tenantID, taskID, userID uint64, targetUserIDs []uint64, comment string) (*Effects, error) {
	targets := dedupe(targetUserIDs)
	if len(targets) == 0 {
		return nil, ErrAddSignTarget
	}
	eff := &Effects{}
	err := e.db.Transaction(func(tx *gorm.DB) error {
		task, inst, err := e.lockTaskAndInstance(tx, tenantID, taskID, userID)
		if err != nil {
			return err
		}
		if task.DelegatedBy != 0 {
			return ErrTaskDelegated
		}
		_, node, err := e.loadNode(tx, inst, task.NodeID)
		if err != nil {
			return err
		}
		if node.Type == flow.TypeStart { // 重提任务不支持加签
			return ErrReturnStartTask
		}
		if task.MultiMode == flow.MultiSeq {
			return ErrAddSignSeq
		}
		// 同节点同 round 已有 pending 任务的人不能重复加（含操作人自己）
		siblings, err := nodeTasks(tx, inst.ID, task.NodeID, task.Round)
		if err != nil {
			return err
		}
		pending := map[uint64]bool{}
		for _, t := range siblings {
			if t.Status == model.TaskPending {
				pending[t.AssigneeID] = true
			}
		}
		added := make([]uint64, 0, len(targets))
		for _, uid := range targets {
			if !pending[uid] {
				added = append(added, uid)
			}
		}
		if len(added) == 0 {
			return ErrAddSignDuplicate
		}
		// 目标人存在性不做校验（与转办同口径：ID 由前端选人组件保证）
		for _, uid := range added {
			nt := model.Task{
				TenantID: inst.TenantID, InstanceID: inst.ID,
				NodeID: task.NodeID, NodeName: task.NodeName, Round: task.Round,
				AssigneeID: uid, MultiMode: task.MultiMode,
				Status: model.TaskPending, TimeoutAt: nodeTimeoutAt(node),
				AddSignBy: userID,
			}
			if err := tx.Create(&nt).Error; err != nil {
				return err
			}
			eff.NewTasks = append(eff.NewTasks, nt) // 复用 bpm.task_assigned 待办通知
		}
		eff.Instance = inst
		writeLog(tx, inst, task.NodeID, task.ID, model.ActionAddSign, userID,
			map[string]any{"target_user_ids": added, "from_user_id": userID, "comment": comment})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return eff, nil
}

// Delegate 委派：任务交给受托人办理（assignee 换为受托人、delegated_by 记
// 委派人），受托人办结后回到委派人再做决定。不改变节点计数；禁止链式委派；
// 与转办正交（origin_assignee 不动，转办后再委派两字段并存）。
func (e *Engine) Delegate(tenantID, taskID, userID, targetUserID uint64, comment string) (*Effects, error) {
	if targetUserID == 0 {
		return nil, ErrDelegateTarget
	}
	if targetUserID == userID {
		return nil, ErrDelegateSelf
	}
	eff := &Effects{}
	err := e.db.Transaction(func(tx *gorm.DB) error {
		task, inst, err := e.lockTaskAndInstance(tx, tenantID, taskID, userID)
		if err != nil {
			return err
		}
		if task.DelegatedBy != 0 { // 禁止链式委派
			return ErrTaskDelegated
		}
		_, node, err := e.loadNode(tx, inst, task.NodeID)
		if err != nil {
			return err
		}
		if node.Type == flow.TypeStart {
			return ErrReturnStartTask
		}
		res := tx.Model(&model.Task{}).
			Where("id = ? AND status = ?", task.ID, model.TaskPending).
			Updates(map[string]any{"assignee_id": targetUserID, "delegated_by": userID})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrTaskHandled
		}
		eff.Instance = inst
		writeLog(tx, inst, task.NodeID, task.ID, model.ActionDelegate, userID,
			map[string]any{"target_user_id": targetUserID, "from_user_id": userID, "comment": comment})
		task.AssigneeID = targetUserID
		task.DelegatedBy = userID
		eff.DelegatedTasks = append(eff.DelegatedTasks, *task)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return eff, nil
}

// ResolveDelegate 委派办结：受托人填写办理意见后任务回到委派人（assignee
// 还原、delegated_by 清零、delegate_resolved_by 记受托人）。受托人意见落
// 日志 detail（时间线可见）；task.Comment 留给委派人终审时写。
func (e *Engine) ResolveDelegate(tenantID, taskID, userID uint64, comment string) (*Effects, error) {
	if strings.TrimSpace(comment) == "" {
		return nil, ErrDelegateComment
	}
	eff := &Effects{}
	err := e.db.Transaction(func(tx *gorm.DB) error {
		task, inst, err := e.lockTaskAndInstance(tx, tenantID, taskID, userID)
		if err != nil {
			return err
		}
		if task.DelegatedBy == 0 {
			return ErrNotDelegated
		}
		res := tx.Model(&model.Task{}).
			Where("id = ? AND status = ?", task.ID, model.TaskPending).
			Updates(map[string]any{
				"assignee_id": task.DelegatedBy, "delegated_by": 0,
				"delegate_resolved_by": userID,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrTaskHandled
		}
		eff.Instance = inst
		writeLog(tx, inst, task.NodeID, task.ID, model.ActionDelegateResolve, userID,
			map[string]any{"target_user_id": task.DelegatedBy, "from_user_id": userID, "comment": comment})
		task.AssigneeID = task.DelegatedBy
		task.DelegatedBy = 0
		task.DelegateResolvedBy = userID
		eff.DelegateResolvedTasks = append(eff.DelegateResolvedTasks, *task)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return eff, nil
}

// Return 退回：to=start 退回发起人（生成重提任务）；to=prev 退回上一审批
// 节点（round+1 重新展开；节点需 allowBackPrev；无上一审批节点时等价退回
// 发起人）。当前节点所有 pending 任务置 returned。
func (e *Engine) Return(tenantID, taskID, userID uint64, to, comment string) (*Effects, error) {
	if strings.TrimSpace(comment) == "" {
		return nil, ErrReturnComment
	}
	if to != "start" && to != "prev" {
		return nil, ErrReturnTarget
	}
	eff := &Effects{}
	err := e.db.Transaction(func(tx *gorm.DB) error {
		task, inst, err := e.lockTaskAndInstance(tx, tenantID, taskID, userID)
		if err != nil {
			return err
		}
		if task.DelegatedBy != 0 {
			return ErrTaskDelegated
		}
		sc, node, err := e.loadNode(tx, inst, task.NodeID)
		if err != nil {
			return err
		}
		if node.Type == flow.TypeStart {
			return ErrReturnStartTask
		}
		var prev *flow.Node
		if to == "prev" {
			if !node.AllowBackPrev {
				return ErrBackPrevNotAllowed
			}
			// M3：按执行路径（任务创建序）回溯上一审批节点——任务只在实际
			// 走过的节点上产生，条件分支下天然只回溯已执行的分支；auto_pass
			// 的空审批节点无任务，也天然跳过。无 → 等价退回发起人。
			prev, err = prevApprovalNode(tx, inst, sc, node.ID)
			if err != nil {
				return err
			}
		}
		// 操作者任务带意见置 returned（记 acted_at），其余 pending 同置 returned
		if err := markTask(tx, task.ID, model.TaskReturned, comment); err != nil {
			return err
		}
		if err := returnPending(tx, inst.ID, task.NodeID, task.Round); err != nil {
			return err
		}
		eff.Instance = inst
		if to == "prev" && prev != nil {
			writeLog(tx, inst, task.NodeID, task.ID, model.ActionReturnPrev, userID,
				map[string]any{"to": "prev", "target_node_id": prev.ID, "comment": comment})
			// 上一审批节点 round+1 重新展开（runFrom 从该节点本身进入）
			return e.runFrom(tx, inst, sc, prev, eff)
		}
		action := model.ActionReturnStart
		detail := map[string]any{"to": to, "comment": comment}
		if to == "prev" { // 无上一审批节点 → 等价退回发起人
			action = model.ActionReturnPrev
			detail["effective"] = "start"
		}
		writeLog(tx, inst, task.NodeID, task.ID, action, userID, detail)
		return e.createResubmitTask(tx, inst, sc, eff)
	})
	if err != nil {
		return nil, err
	}
	return eff, nil
}
