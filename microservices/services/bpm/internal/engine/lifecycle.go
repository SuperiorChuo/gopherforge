package engine

import (
	"errors"
	"github.com/go-admin-kit/services/bpm/internal/flow"
	"github.com/go-admin-kit/services/bpm/internal/model"
	"gorm.io/gorm"
	"strings"
	"time"
)

// Resubmit 重新提交：仅发起人、且实例存在 pending 的重提任务（start 节点）
// 时允许。可带新表单快照（缺省沿用旧快照），随后从 start.next 重新推进，
// 各审批节点以"该节点历史最大 round+1"重新展开，旧 round 任务不复活。
func (e *Engine) Resubmit(tenantID, instanceID, userID uint64, formSnapshot []byte) (*Effects, error) {
	eff := &Effects{}
	err := e.db.Transaction(func(tx *gorm.DB) error {
		inst, err := lockInstance(tx, instanceID, tenantID)
		if err != nil {
			return err
		}
		if inst.InitiatorID != userID {
			return ErrNotInitiator
		}
		if inst.Status != model.InstRunning {
			return ErrInstanceNotRunning
		}
		sc, err := e.loadSchema(tx, inst)
		if err != nil {
			return err
		}
		var task model.Task
		err = tx.Where("instance_id = ? AND node_id = ? AND status = ?",
			inst.ID, sc.Start.ID, model.TaskPending).
			Order("id DESC").First(&task).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotReturnedState
			}
			return err
		}
		// 重提任务置 approved（发起人完成了"重新提交"这一步，状态集合里
		// 最贴近语义；时间线以 resubmit 日志区分于普通同意）
		if err := markTask(tx, task.ID, model.TaskApproved, "重新提交"); err != nil {
			return err
		}
		formUpdated := len(formSnapshot) > 0
		if formUpdated {
			inst.FormSnapshot = model.JSONB(formSnapshot)
			if err := tx.Model(inst).Update("form_snapshot", inst.FormSnapshot).Error; err != nil {
				return err
			}
		}
		eff.Instance = inst
		writeLog(tx, inst, sc.Start.ID, task.ID, model.ActionResubmit, userID,
			map[string]any{"form_updated": formUpdated})
		return e.advanceFrom(tx, inst, sc, sc.Start, eff)
	})
	if err != nil {
		return nil, err
	}
	return eff, nil
}

// Cancel 撤销：仅发起人、实例 running 且尚无任何通过记录时允许（M1 从严）。
// M2 放宽：实例处于"被退回待重提"状态（存在 pending 的 start 重提任务）时，
// 即便此前已有节点通过也允许撤销——退回已使前序通过失效，发起人此时应能
// 选择放弃而不是被迫重提（§3.3 M2 放宽方向）。
func (e *Engine) Cancel(tenantID, instanceID, userID uint64) (*Effects, error) {
	eff := &Effects{}
	err := e.db.Transaction(func(tx *gorm.DB) error {
		inst, err := lockInstance(tx, instanceID, tenantID)
		if err != nil {
			return err
		}
		if inst.InitiatorID != userID {
			return ErrNotInitiator
		}
		if inst.Status != model.InstRunning {
			return ErrInstanceNotRunning
		}
		var approved int64
		if err := tx.Model(&model.Task{}).
			Where("instance_id = ? AND status = ?", inst.ID, model.TaskApproved).
			Count(&approved).Error; err != nil {
			return err
		}
		if approved > 0 && !e.inReturnedState(tx, inst) {
			return ErrCancelDenied
		}
		if err := tx.Model(&model.Task{}).
			Where("instance_id = ? AND status = ?", inst.ID, model.TaskPending).
			Update("status", model.TaskCanceled).Error; err != nil {
			return err
		}
		eff.Instance = inst
		writeLog(tx, inst, "", 0, model.ActionCancel, userID, nil)
		now := time.Now()
		inst.Status = model.InstCanceled
		inst.CurrentNodeID = ""
		inst.FinishedAt = &now
		eff.FinalResult = model.InstCanceled
		if err := tx.Model(inst).Updates(map[string]any{
			"status": inst.Status, "current_node_id": "", "finished_at": now,
		}).Error; err != nil {
			return err
		}
		return enqueueCallback(tx, inst)
	})
	if err != nil {
		return nil, err
	}
	return eff, nil
}

// settleApproved 任务通过后的节点计数收敛与推进（同意 / 超时自动通过共用）。
func (e *Engine) settleApproved(tx *gorm.DB, inst *model.ProcessInstance, sc *flow.Schema, node *flow.Node, task *model.Task, eff *Effects) error {
	// 节点计数收敛：当前节点当前 round 的任务集合
	siblings, err := nodeTasks(tx, inst.ID, task.NodeID, task.Round)
	if err != nil {
		return err
	}
	pass := false
	switch task.MultiMode {
	case flow.MultiAnd: // 会签：全部同意才通过
		pass = true
		for _, t := range siblings {
			if t.Status != model.TaskApproved {
				pass = false
				break
			}
		}
	case flow.MultiSeq: // 依次（M3）：还有下一顺位则补建任务，否则节点通过
		nextUID, err := e.nextSeqAssignee(tx, inst, node, siblings)
		if err != nil {
			return err
		}
		if nextUID != 0 {
			nt := model.Task{
				TenantID: inst.TenantID, InstanceID: inst.ID,
				NodeID: node.ID, NodeName: node.Name, Round: task.Round,
				AssigneeID: nextUID, MultiMode: flow.MultiSeq,
				SeqOrder: task.SeqOrder + 1, Status: model.TaskPending,
				TimeoutAt: nodeTimeoutAt(node),
			}
			if err := tx.Create(&nt).Error; err != nil {
				return err
			}
			eff.NewTasks = append(eff.NewTasks, nt)
			return nil // 等待下一顺位处理
		}
		pass = true
	default: // 或签：一人同意即通过，其余 pending 置 skipped
		pass = true
		if err := skipPending(tx, inst.ID, task.NodeID, task.Round); err != nil {
			return err
		}
	}
	if !pass {
		return nil // 会签继续等待其他人
	}
	// 节点通过 → 从该节点继续推进
	return e.advanceFrom(tx, inst, sc, node, eff)
}

// settleRejected 任务拒绝后的节点计数收敛与 onReject 分派（拒绝 / 超时自动
// 拒绝共用；operatorID=0 表示系统动作）。
func (e *Engine) settleRejected(tx *gorm.DB, inst *model.ProcessInstance, sc *flow.Schema, node *flow.Node, task *model.Task, operatorID uint64, comment string, eff *Effects) error {
	siblings, err := nodeTasks(tx, inst.ID, task.NodeID, task.Round)
	if err != nil {
		return err
	}
	nodeRejected := false
	switch task.MultiMode {
	case flow.MultiAnd, flow.MultiSeq:
		// 会签：任一拒绝即节点拒绝；依次（M3）：当前人拒绝即节点拒绝，
		// 后续顺位不再创建
		nodeRejected = true
	default: // 或签：全部拒绝才节点拒绝；尚有 pending 时继续等（他人可救回）
		nodeRejected = true
		for _, t := range siblings {
			if t.Status == model.TaskPending || t.Status == model.TaskApproved {
				nodeRejected = false
				break
			}
		}
	}
	if !nodeRejected {
		return nil
	}
	if node.OnReject == flow.OnRejectBackToStart {
		// M2：节点拒绝 → 退回发起人重提（其余 pending 置 returned，
		// 与手动退回一致；实例保持 running）
		if err := returnPending(tx, inst.ID, task.NodeID, task.Round); err != nil {
			return err
		}
		writeLog(tx, inst, task.NodeID, task.ID, model.ActionReturnStart, operatorID,
			map[string]any{"to": "start", "trigger": "on_reject", "comment": comment})
		return e.createResubmitTask(tx, inst, sc, eff)
	}
	// 缺省 onReject=reject：其余 pending 置 skipped → 实例终态 rejected
	if err := skipPending(tx, inst.ID, task.NodeID, task.Round); err != nil {
		return err
	}
	return e.finish(tx, inst, model.InstRejected, eff)
}

// 超时处理结果。
const (
	TimeoutOutcomeRemind = "remind" // 节点为提醒策略：由调用方记录提醒并发信
	TimeoutOutcomeSkip   = "skip"   // 任务/实例状态已变化或节点不适用
	TimeoutOutcomePass   = "auto_pass"
	TimeoutOutcomeReject = "auto_reject"
)

// HandleTimeout 处理一条到期任务：按节点 timeoutAction 返回 remind（调用方
// 发提醒）或执行自动通过/拒绝（系统操作人 0，效果经 Effects 完整分发——
// 下一节点待办通知与终态回调都不缺）。
func (e *Engine) HandleTimeout(tenantID, taskID uint64) (string, *Effects, error) {
	// 无锁预读决策；自动动作在事务内二次校验（markTask 条件更新兜底并发）
	var task model.Task
	if err := e.db.Where("id = ? AND tenant_id = ?", taskID, tenantID).First(&task).Error; err != nil {
		return TimeoutOutcomeSkip, nil, nil
	}
	if task.Status != model.TaskPending {
		return TimeoutOutcomeSkip, nil, nil
	}
	var inst model.ProcessInstance
	if err := e.db.Where("id = ?", task.InstanceID).First(&inst).Error; err != nil ||
		inst.Status != model.InstRunning {
		return TimeoutOutcomeSkip, nil, nil
	}
	sc, node, err := e.loadNode(e.db, &inst, task.NodeID)
	if err != nil || node == nil || node.Type != flow.TypeApproval {
		return TimeoutOutcomeSkip, nil, nil
	}
	action := node.EffectiveTimeoutAction()
	if action == flow.TimeoutRemind {
		return TimeoutOutcomeRemind, nil, nil
	}

	eff := &Effects{}
	err = e.db.Transaction(func(tx *gorm.DB) error {
		lockedInst, err := lockInstance(tx, inst.ID, tenantID)
		if err != nil {
			return err
		}
		if lockedInst.Status != model.InstRunning {
			return ErrInstanceNotRunning
		}
		eff.Instance = lockedInst
		hours := node.TimeoutHours
		if action == flow.TimeoutAutoPass {
			if err := markTask(tx, task.ID, model.TaskApproved, "超时自动通过"); err != nil {
				return err
			}
			writeLog(tx, lockedInst, task.NodeID, task.ID, model.ActionTimeoutPass, 0,
				map[string]any{"timeout_hours": hours})
			return e.settleApproved(tx, lockedInst, sc, node, &task, eff)
		}
		if err := markTask(tx, task.ID, model.TaskRejected, "超时自动拒绝"); err != nil {
			return err
		}
		writeLog(tx, lockedInst, task.NodeID, task.ID, model.ActionTimeoutReject, 0,
			map[string]any{"timeout_hours": hours})
		return e.settleRejected(tx, lockedInst, sc, node, &task, 0, "超时自动拒绝", eff)
	})
	if err != nil {
		if errors.Is(err, ErrTaskHandled) || errors.Is(err, ErrInstanceNotRunning) {
			return TimeoutOutcomeSkip, nil, nil // 并发下已被人工处理
		}
		return TimeoutOutcomeSkip, nil, err
	}
	if action == flow.TimeoutAutoPass {
		return TimeoutOutcomePass, eff, nil
	}
	return TimeoutOutcomeReject, eff, nil
}

// Terminate 管理员强制终止：running / suspended 实例均可（挂起实例的管理
// 出口），全部 pending 任务作废，实例落 canceled 终态——业务回调复用
// canceled 语义（CRM 等按撤销回滚），时间线以 terminate 日志区分于撤销。
func (e *Engine) Terminate(tenantID, instanceID, operatorID uint64, reason string) (*Effects, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, ErrTerminateReason
	}
	eff := &Effects{}
	err := e.db.Transaction(func(tx *gorm.DB) error {
		inst, err := lockInstance(tx, instanceID, tenantID)
		if err != nil {
			return err
		}
		if inst.Status != model.InstRunning && inst.Status != model.InstSuspended {
			return ErrInstanceFinished
		}
		if err := tx.Model(&model.Task{}).
			Where("instance_id = ? AND status = ?", inst.ID, model.TaskPending).
			Update("status", model.TaskCanceled).Error; err != nil {
			return err
		}
		eff.Instance = inst
		writeLog(tx, inst, "", 0, model.ActionTerminate, operatorID,
			map[string]any{"reason": strings.TrimSpace(reason)})
		now := time.Now()
		inst.Status = model.InstCanceled
		inst.CurrentNodeID = ""
		inst.FinishedAt = &now
		eff.FinalResult = model.InstCanceled
		eff.ResultText = "已终止"
		if err := tx.Model(inst).Updates(map[string]any{
			"status": inst.Status, "current_node_id": "", "finished_at": now,
		}).Error; err != nil {
			return err
		}
		return enqueueCallback(tx, inst)
	})
	if err != nil {
		return nil, err
	}
	return eff, nil
}
