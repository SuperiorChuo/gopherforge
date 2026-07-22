// Package engine 实现审批流推进：节点树上的单游标 + 任务展开器。
//
// 所有推进在单个 DB 事务内完成（更新任务 → 节点计数收敛 → 移动游标 →
// 展开下个节点任务 → 写日志）；实例行在 postgres 下加 SELECT ... FOR UPDATE
// 行锁防会签并发双推进（sqlite 测试库单连接天然串行），任务状态更新一律带
// WHERE status='pending' 条件更新做乐观兜底。
//
// 通知与回调等副作用不在事务内发出：引擎把它们收集进 Effects，由调用方
// （api 层）在事务提交后分发，保证"审批事实先落库"。
package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-admin-kit/services/bpm/internal/flow"
	"github.com/go-admin-kit/services/bpm/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrNoActiveDefinition = errors.New("流程没有已发布版本，无法发起")
	ErrDuplicateRunning   = errors.New("该业务对象已有在途审批，不可重复发起")
	ErrTaskNotFound       = errors.New("任务不存在")
	ErrNotAssignee        = errors.New("仅当前处理人可操作该任务")
	ErrTaskHandled        = errors.New("任务已被处理")
	ErrInstanceNotFound   = errors.New("流程实例不存在")
	ErrInstanceNotRunning = errors.New("流程实例不在审批中")
	ErrNotInitiator       = errors.New("仅发起人可操作")
	ErrCancelDenied       = errors.New("已有审批通过记录，不可撤销")
	ErrCommentRequired    = errors.New("拒绝必须填写意见")
	// M2 新增动作错误
	ErrTransferTarget     = errors.New("转办目标人不能为空")
	ErrTransferSelf       = errors.New("不能转办给自己")
	ErrReturnComment      = errors.New("退回必须填写意见")
	ErrReturnTarget       = errors.New("退回目标未知（仅支持 start / prev）")
	ErrBackPrevNotAllowed = errors.New("该节点未开启退回上一节点")
	ErrNotReturnedState   = errors.New("流程未处于退回待重提状态")
	ErrReturnStartTask    = errors.New("重新提交任务请使用重提或撤销，不支持该动作")
	// M3 新增动作错误
	ErrTerminateReason  = errors.New("终止必须填写原因")
	ErrInstanceFinished = errors.New("流程实例已结束")
)

type Engine struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Engine { return &Engine{db: db} }

// Effects 事务内收集、提交后由调用方分发的副作用。
type Effects struct {
	Instance *model.ProcessInstance
	// NewTasks 本次推进新展开的待办任务（发 bpm.task_assigned 站内信）
	NewTasks []model.Task
	// CcRecords 本次推进落地的抄送记录（发 bpm.cc 站内信）
	CcRecords []model.CcRecord
	// FinalResult 非空表示实例到达终态（approved|rejected|canceled），
	// 需发终态回调 + 给发起人发 bpm.result 站内信
	FinalResult string
	// ResultText 终态文案覆盖（管理员终止时区分于发起人撤销；空=按状态取）
	ResultText string
}

// instVars 实例运行期变量（M1：发起人自选的选人结果）。
type instVars struct {
	SelectedAssignees map[string][]uint64 `json:"selected_assignees"`
}

// ---------- 发起 ----------

type StartInput struct {
	TenantID      uint64
	DefinitionKey string
	Title         string
	BizType       string
	BizID         string
	FormSnapshot  []byte
	Variables     []byte
	InitiatorID   uint64
	InitiatorDept uint64
}

// Start 发起流程：冻结 active 定义版本 → 建实例 → 从发起节点推进。
func (e *Engine) Start(in StartInput) (*Effects, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, errors.New("标题不能为空")
	}
	if strings.TrimSpace(in.BizType) == "" || strings.TrimSpace(in.BizID) == "" {
		return nil, errors.New("biz_type / biz_id 不能为空")
	}
	if in.InitiatorID == 0 {
		return nil, errors.New("发起人不能为空")
	}
	var def model.ProcessDefinition
	err := e.db.Where("tenant_id = ? AND key = ? AND status = ?",
		in.TenantID, strings.TrimSpace(in.DefinitionKey), model.DefActive).
		First(&def).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNoActiveDefinition
		}
		return nil, err
	}
	sc, err := flow.Parse(def.NodeTree)
	if err != nil {
		return nil, err
	}
	// 发起前置校验：所有 self_select 审批节点必须已提供选人（发起时报错
	// 比运行中挂起更友好）。
	vars := parseVars(in.Variables)
	for _, n := range flow.Nodes(sc) {
		if n.Type == flow.TypeApproval && n.Assignee != nil &&
			n.Assignee.Type == flow.RuleSelfSelect &&
			len(dedupe(vars.SelectedAssignees[n.ID])) == 0 {
			return nil, fmt.Errorf("节点「%s」需要发起人选择审批人（variables.selected_assignees[%q]）", n.Name, n.ID)
		}
	}

	// 发起人部门兜底：调用方未传时直查同库 users.department_id（与 roles
	// 规则同一条"同库直读 identity 表"路径；查不到保持 0，dept_leader 规则
	// 届时走 emptyFallback）。
	if in.InitiatorDept == 0 {
		in.InitiatorDept = lookupUserDept(e.db, in.TenantID, in.InitiatorID)
	}

	eff := &Effects{}
	err = e.db.Transaction(func(tx *gorm.DB) error {
		// 应用层预查在途（可读报错）；部分唯一索引兜底并发窗口
		var cnt int64
		if err := tx.Model(&model.ProcessInstance{}).
			Where("tenant_id = ? AND biz_type = ? AND biz_id = ? AND status = ?",
				in.TenantID, in.BizType, in.BizID, model.InstRunning).
			Count(&cnt).Error; err != nil {
			return err
		}
		if cnt > 0 {
			return ErrDuplicateRunning
		}
		form := in.FormSnapshot
		if len(form) == 0 {
			form = []byte("{}")
		}
		inst := &model.ProcessInstance{
			TenantID:      in.TenantID,
			DefinitionID:  def.ID,
			DefinitionKey: def.Key,
			Title:         title,
			BizType:       strings.TrimSpace(in.BizType),
			BizID:         strings.TrimSpace(in.BizID),
			Status:        model.InstRunning,
			CurrentNodeID: sc.Start.ID,
			FormSnapshot:  model.JSONB(form),
			Variables:     model.JSONB(in.Variables),
			InitiatorID:   in.InitiatorID,
			InitiatorDept: in.InitiatorDept,
		}
		if err := tx.Create(inst).Error; err != nil {
			return err
		}
		eff.Instance = inst
		writeLog(tx, inst, sc.Start.ID, 0, model.ActionSubmit, in.InitiatorID,
			map[string]any{"title": title})
		return e.advanceFrom(tx, inst, sc, sc.Start, eff)
	})
	if err != nil {
		return nil, err
	}
	return eff, nil
}

// ---------- 同意 / 拒绝 ----------

// Approve 同意：任务置 approved → 节点计数收敛 → 通过则推进游标。
func (e *Engine) Approve(tenantID, taskID, userID uint64, comment string) (*Effects, error) {
	eff := &Effects{}
	err := e.db.Transaction(func(tx *gorm.DB) error {
		task, inst, err := e.lockTaskAndInstance(tx, tenantID, taskID, userID)
		if err != nil {
			return err
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
			writeLog(tx, inst, task.NodeID, task.ID, model.ActionReturnStart, userID,
				map[string]any{"to": "start", "trigger": "on_reject", "comment": comment})
			return e.createResubmitTask(tx, inst, sc, eff)
		}
		// 缺省 onReject=reject：其余 pending 置 skipped → 实例终态 rejected
		if err := skipPending(tx, inst.ID, task.NodeID, task.Round); err != nil {
			return err
		}
		return e.finish(tx, inst, model.InstRejected, eff)
	})
	if err != nil {
		return nil, err
	}
	return eff, nil
}

// ---------- 转办 / 退回 / 重新提交（M2） ----------

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

// ---------- 撤销 ----------

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
		return tx.Model(inst).Updates(map[string]any{
			"status": inst.Status, "current_node_id": "", "finished_at": now,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return eff, nil
}

// ---------- 终止（M3） ----------

// Terminate 管理员强制终止：running / suspended 实例均可（挂起实例的管理
// 出口），全部 pending 任务作废，实例落 canceled 终态——业务回调复用
// canceled 语义（业务方按撤销回滚），时间线以 terminate 日志区分于撤销。
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
		return tx.Model(inst).Updates(map[string]any{
			"status": inst.Status, "current_node_id": "", "finished_at": now,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return eff, nil
}

// ---------- 推进核心 ----------

// advanceFrom 从 done 节点（已完成）的执行后继继续推进游标（分支链尾
// 自动汇合回 condition 之后，见 flow.Successor）。
func (e *Engine) advanceFrom(tx *gorm.DB, inst *model.ProcessInstance, sc *flow.Schema, done *flow.Node, eff *Effects) error {
	return e.runFrom(tx, inst, sc, flow.Successor(sc, done.ID), eff)
}

// runFrom 从 node 本身开始推进游标（退回上一节点时直接从目标节点进入），
// 直到停在一个等待人工的审批节点、挂起、或到达链尾终态 approved。
func (e *Engine) runFrom(tx *gorm.DB, inst *model.ProcessInstance, sc *flow.Schema, node *flow.Node, eff *Effects) error {
	for steps := 0; ; steps++ {
		if steps > flow.MaxNodes { // 防御异常定义（发布校验已限节点数）
			return errors.New("流程推进步数超限，疑似定义异常")
		}
		if node == nil {
			return e.finish(tx, inst, model.InstApproved, eff)
		}
		switch node.Type {
		case flow.TypeCc:
			users, err := e.resolveRule(tx, inst, node.Targets, node.ID)
			if err != nil {
				return err
			}
			users = dedupe(users)
			for _, uid := range users {
				rec := model.CcRecord{
					TenantID: inst.TenantID, InstanceID: inst.ID,
					NodeID: node.ID, NodeName: node.Name, UserID: uid,
				}
				if err := tx.Create(&rec).Error; err != nil {
					return err
				}
				eff.CcRecords = append(eff.CcRecords, rec)
			}
			writeLog(tx, inst, node.ID, 0, model.ActionCc, 0,
				map[string]any{"user_ids": users})
			node = flow.Successor(sc, node.ID)

		case flow.TypeApproval:
			users, err := e.resolveRule(tx, inst, node.Assignee, node.ID)
			if err != nil {
				return err
			}
			users = dedupe(users)
			if len(users) == 0 {
				switch node.Assignee.EffectiveFallback() {
				case flow.FallbackAutoPass:
					writeLog(tx, inst, node.ID, 0, model.ActionAutoPass, 0,
						map[string]any{"reason": "审批人解析为空，按节点配置自动通过"})
					node = flow.Successor(sc, node.ID)
					continue
				case flow.FallbackToUsers:
					users = dedupe(node.Assignee.FallbackUserIDs)
				}
				if len(users) == 0 { // suspend（缺省）或兜底人也为空
					return e.suspend(tx, inst, node.ID, "审批人解析为空，实例挂起待管理员处理", nil)
				}
			}
			mode := node.EffectiveMultiMode()
			// 依次（M3）：只展开首位，后续顺位在 Approve 收敛时逐个补建
			if mode == flow.MultiSeq && len(users) > 1 {
				users = users[:1]
			}
			timeoutAt := nodeTimeoutAt(node)
			// round = 该节点历史最大轮次 +1：首次展开为 1；退回/重提后重新
			// 展开自动进入新一轮，旧 round 任务不参与新一轮计数、不复活。
			round, err := nodeNextRound(tx, inst.ID, node.ID)
			if err != nil {
				return err
			}
			for _, uid := range users {
				task := model.Task{
					TenantID: inst.TenantID, InstanceID: inst.ID,
					NodeID: node.ID, NodeName: node.Name, Round: round,
					AssigneeID: uid, MultiMode: mode, Status: model.TaskPending,
					TimeoutAt: timeoutAt,
				}
				if err := tx.Create(&task).Error; err != nil {
					return err
				}
				eff.NewTasks = append(eff.NewTasks, task)
			}
			inst.CurrentNodeID = node.ID
			return tx.Model(inst).Update("current_node_id", node.ID).Error

		case flow.TypeCondition:
			// 排他分支（M3）：按表单快照从上到下取第一个命中，default 兜底；
			// 求值失败挂起而非静默走 default（§3.2，避免错批）。
			br, err := pickBranch(node, inst.FormSnapshot)
			if err != nil {
				return e.suspend(tx, inst, node.ID, "条件求值失败，实例挂起待管理员处理",
					map[string]any{"error": err.Error()})
			}
			writeLog(tx, inst, node.ID, 0, model.ActionBranch, 0,
				map[string]any{"branch_id": br.ID, "branch_name": br.Name})
			if br.Next != nil {
				node = br.Next // 进入分支子链
			} else {
				node = flow.Successor(sc, node.ID) // 空分支直通汇合点
			}
		default:
			return fmt.Errorf("未知节点类型: %s", node.Type)
		}
	}
}

// suspend 实例挂起（审批人为空 / 条件求值失败）：非终态，游标停在问题节点，
// 待管理员终止或修复数据后人工处理。
func (e *Engine) suspend(tx *gorm.DB, inst *model.ProcessInstance, nodeID, reason string, extra map[string]any) error {
	detail := map[string]any{"reason": reason}
	for k, v := range extra {
		detail[k] = v
	}
	inst.Status = model.InstSuspended
	inst.CurrentNodeID = nodeID
	writeLog(tx, inst, nodeID, 0, model.ActionSuspend, 0, detail)
	return tx.Model(inst).Updates(map[string]any{
		"status": inst.Status, "current_node_id": inst.CurrentNodeID,
	}).Error
}

// pickBranch 按表单快照从上到下取第一个命中的分支；全不命中走 default。
func pickBranch(node *flow.Node, snapshot []byte) (*flow.Branch, error) {
	m := map[string]any{}
	if len(snapshot) > 0 {
		if err := json.Unmarshal(snapshot, &m); err != nil {
			return nil, fmt.Errorf("表单快照解析失败: %w", err)
		}
	}
	var def *flow.Branch
	for i := range node.Branches {
		b := &node.Branches[i]
		expr, err := flow.ParseExpr(b.Expr)
		if err != nil {
			return nil, err
		}
		if expr == nil {
			def = b
			continue
		}
		hit, err := flow.EvalExpr(expr, m)
		if err != nil {
			return nil, err
		}
		if hit {
			return b, nil
		}
	}
	if def == nil { // 发布校验保证 default 存在；防御历史/异常数据
		return nil, errors.New("无命中分支且缺少默认分支")
	}
	return def, nil
}

// nodeTimeoutAt 按节点 timeoutHours 计算超时提醒时间点（0=不提醒）。
func nodeTimeoutAt(node *flow.Node) *time.Time {
	if node.TimeoutHours > 0 {
		t := time.Now().Add(time.Duration(node.TimeoutHours) * time.Hour)
		return &t
	}
	return nil
}

// nextSeqAssignee 依次模式的下一顺位：按规则重解析候选序列（users 显式
// 顺序 / roles 按用户 id 升序），取第一个尚未在本节点本轮出过任务的人；
// 全部出过 → 0（节点通过）。转办后转入/转出人都视为已占位。
func (e *Engine) nextSeqAssignee(tx *gorm.DB, inst *model.ProcessInstance, node *flow.Node, siblings []model.Task) (uint64, error) {
	users, err := e.resolveRule(tx, inst, node.Assignee, node.ID)
	if err != nil {
		return 0, err
	}
	used := map[uint64]bool{}
	for _, t := range siblings {
		used[t.AssigneeID] = true
		if t.OriginAssignee != 0 {
			used[t.OriginAssignee] = true
		}
	}
	for _, uid := range dedupe(users) {
		if !used[uid] {
			return uid, nil
		}
	}
	return 0, nil
}

// prevApprovalNode 执行路径上的上一审批节点：按任务创建序回溯（任务只在
// 实际执行的节点上产生，条件分支下天然只回溯已走的分支）。排除当前节点
// 与 start 重提任务；无 → nil（等价退回发起人）。
func prevApprovalNode(tx *gorm.DB, inst *model.ProcessInstance, sc *flow.Schema, currentNodeID string) (*flow.Node, error) {
	exclude := []string{currentNodeID}
	if sc.Start != nil {
		exclude = append(exclude, sc.Start.ID)
	}
	var nodeIDs []string
	if err := tx.Model(&model.Task{}).
		Where("instance_id = ? AND node_id NOT IN ?", inst.ID, exclude).
		Order("id DESC").Limit(100).Pluck("node_id", &nodeIDs).Error; err != nil {
		return nil, err
	}
	for _, id := range nodeIDs {
		if n := flow.NodeByID(sc, id); n != nil && n.Type == flow.TypeApproval {
			return n, nil
		}
	}
	return nil, nil
}

// createResubmitTask 退回发起人：生成发起人的"重新提交"任务（node=start、
// round=全实例最大轮次+1），游标回 start，实例保持 running。
func (e *Engine) createResubmitTask(tx *gorm.DB, inst *model.ProcessInstance, sc *flow.Schema, eff *Effects) error {
	round, err := instanceNextRound(tx, inst.ID)
	if err != nil {
		return err
	}
	task := model.Task{
		TenantID: inst.TenantID, InstanceID: inst.ID,
		NodeID: sc.Start.ID, NodeName: sc.Start.Name, Round: round,
		AssigneeID: inst.InitiatorID, MultiMode: flow.MultiOr,
		Status: model.TaskPending,
	}
	if err := tx.Create(&task).Error; err != nil {
		return err
	}
	eff.NewTasks = append(eff.NewTasks, task)
	inst.CurrentNodeID = sc.Start.ID
	return tx.Model(inst).Update("current_node_id", sc.Start.ID).Error
}

// inReturnedState 实例是否处于"被退回待重提"状态（存在 pending 的 start
// 重提任务）。schema 加载失败时按 false 处理（从严）。
func (e *Engine) inReturnedState(tx *gorm.DB, inst *model.ProcessInstance) bool {
	sc, err := e.loadSchema(tx, inst)
	if err != nil || sc.Start == nil {
		return false
	}
	var cnt int64
	tx.Model(&model.Task{}).
		Where("instance_id = ? AND node_id = ? AND status = ?",
			inst.ID, sc.Start.ID, model.TaskPending).
		Count(&cnt)
	return cnt > 0
}

// finish 实例到达终态（approved/rejected），写终态日志并收集回调。
func (e *Engine) finish(tx *gorm.DB, inst *model.ProcessInstance, result string, eff *Effects) error {
	now := time.Now()
	inst.Status = result
	inst.CurrentNodeID = ""
	inst.FinishedAt = &now
	action := model.ActionFinishApproved
	if result == model.InstRejected {
		action = model.ActionFinishRejected
	}
	writeLog(tx, inst, "", 0, action, 0, nil)
	eff.FinalResult = result
	return tx.Model(inst).Updates(map[string]any{
		"status": result, "current_node_id": "", "finished_at": now,
	}).Error
}

// ---------- 审批人解析 ----------

// resolveRule 解析规则为候选人列表。roles 直读同库 identity 的
// users/user_roles 表（过滤禁用用户 + 租户）。
func (e *Engine) resolveRule(tx *gorm.DB, inst *model.ProcessInstance, rule *flow.AssigneeRule, nodeID string) ([]uint64, error) {
	if rule == nil {
		return nil, errors.New("节点缺少人员规则")
	}
	switch rule.Type {
	case flow.RuleUsers:
		return rule.UserIDs, nil
	case flow.RuleRoles:
		var ids []uint64
		err := tx.Table("users").
			Joins("JOIN user_roles ON user_roles.user_id = users.id").
			Where("user_roles.role_id IN ?", rule.RoleIDs).
			Where("users.status = 1 AND users.tenant_id = ?", inst.TenantID).
			Order("users.id ASC").
			Distinct().
			Pluck("users.id", &ids).Error
		if err != nil {
			return nil, fmt.Errorf("按角色解析审批人失败: %w", err)
		}
		return ids, nil
	case flow.RuleSelfSelect:
		vars := parseVars(inst.Variables)
		return vars.SelectedAssignees[nodeID], nil
	case flow.RuleDeptLeader:
		// M2：部门主管。基准部门二选一（缺省发起人部门），主管取同库
		// departments.leader_user_id（identity M2 新增列）；解析为空走节点
		// emptyFallback 三策略（调用方处理）。
		var deptID uint64
		switch rule.DeptLeaderBase {
		case "", flow.DeptBaseInitiator:
			deptID = inst.InitiatorDept
			if deptID == 0 { // 发起时未落部门（历史实例等），补查一次
				deptID = lookupUserDept(tx, inst.TenantID, inst.InitiatorID)
			}
		case flow.DeptBaseFormField:
			deptID = snapshotUint64(inst.FormSnapshot, rule.DeptFormField)
		default:
			return nil, fmt.Errorf("部门主管基准未知: %s", rule.DeptLeaderBase)
		}
		if deptID == 0 {
			return nil, nil
		}
		var leaderID uint64
		err := tx.Table("departments").
			Where("id = ? AND tenant_id = ?", deptID, inst.TenantID).
			Limit(1).Pluck("leader_user_id", &leaderID).Error
		if err != nil {
			return nil, fmt.Errorf("按部门主管解析审批人失败: %w", err)
		}
		if leaderID == 0 {
			return nil, nil
		}
		// 主管已禁用/跨租户视为解析为空（与 roles 规则同口径）
		var cnt int64
		if err := tx.Table("users").
			Where("id = ? AND status = 1 AND tenant_id = ?", leaderID, inst.TenantID).
			Count(&cnt).Error; err != nil {
			return nil, fmt.Errorf("校验部门主管账号失败: %w", err)
		}
		if cnt == 0 {
			return nil, nil
		}
		return []uint64{leaderID}, nil
	default:
		return nil, fmt.Errorf("审批人规则 %s 未支持", rule.Type)
	}
}

// lookupUserDept 同库直读 users.department_id（与 roles 规则同路径）；
// 查询失败（如测试库无该表/列）静默返回 0，由 emptyFallback 兜底。
func lookupUserDept(db *gorm.DB, tenantID, userID uint64) uint64 {
	var deptID uint64
	if err := db.Table("users").
		Where("id = ? AND tenant_id = ?", userID, tenantID).
		Limit(1).Pluck("department_id", &deptID).Error; err != nil {
		return 0
	}
	return deptID
}

// snapshotUint64 从表单快照 JSON 中取 uint64 字段（数字或数字字符串）。
func snapshotUint64(raw []byte, field string) uint64 {
	if len(raw) == 0 || field == "" {
		return 0
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return 0
	}
	switch v := m[field].(type) {
	case float64:
		if v > 0 {
			return uint64(v)
		}
	case string:
		if n, err := strconv.ParseUint(strings.TrimSpace(v), 10, 64); err == nil {
			return n
		}
	}
	return 0
}

// ---------- 事务内工具 ----------

// lockInstance 取实例并加行锁（postgres FOR UPDATE；sqlite 单连接串行免锁）。
func lockInstance(tx *gorm.DB, id, tenantID uint64) (*model.ProcessInstance, error) {
	q := tx.Where("id = ? AND tenant_id = ?", id, tenantID)
	if tx.Dialector.Name() == "postgres" {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var inst model.ProcessInstance
	if err := q.First(&inst).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInstanceNotFound
		}
		return nil, err
	}
	return &inst, nil
}

// lockTaskAndInstance 审批动作公共前置：任务存在 + 处理人校验 + 实例行锁 +
// running 校验。注意先锁实例再核对任务状态，避免与并发推进交错。
func (e *Engine) lockTaskAndInstance(tx *gorm.DB, tenantID, taskID, userID uint64) (*model.Task, *model.ProcessInstance, error) {
	var task model.Task
	if err := tx.Where("id = ? AND tenant_id = ?", taskID, tenantID).First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrTaskNotFound
		}
		return nil, nil, err
	}
	if task.AssigneeID != userID {
		return nil, nil, ErrNotAssignee
	}
	inst, err := lockInstance(tx, task.InstanceID, tenantID)
	if err != nil {
		return nil, nil, err
	}
	if inst.Status != model.InstRunning {
		return nil, nil, ErrInstanceNotRunning
	}
	return &task, inst, nil
}

// markTask 条件更新任务状态（WHERE status='pending' 乐观兜底并发）。
func markTask(tx *gorm.DB, taskID uint64, status, comment string) error {
	now := time.Now()
	res := tx.Model(&model.Task{}).
		Where("id = ? AND status = ?", taskID, model.TaskPending).
		Updates(map[string]any{
			"status": status, "comment": strings.TrimSpace(comment), "acted_at": now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrTaskHandled
	}
	return nil
}

// nodeTasks 当前节点当前 round 的任务集合（收敛判定对象）。
func nodeTasks(tx *gorm.DB, instanceID uint64, nodeID string, round int) ([]model.Task, error) {
	var list []model.Task
	err := tx.Where("instance_id = ? AND node_id = ? AND round = ?", instanceID, nodeID, round).
		Find(&list).Error
	return list, err
}

// skipPending 同节点同 round 其余 pending 任务置 skipped。
func skipPending(tx *gorm.DB, instanceID uint64, nodeID string, round int) error {
	return tx.Model(&model.Task{}).
		Where("instance_id = ? AND node_id = ? AND round = ? AND status = ?",
			instanceID, nodeID, round, model.TaskPending).
		Update("status", model.TaskSkipped).Error
}

// returnPending 同节点同 round 其余 pending 任务置 returned（退回专用）。
func returnPending(tx *gorm.DB, instanceID uint64, nodeID string, round int) error {
	return tx.Model(&model.Task{}).
		Where("instance_id = ? AND node_id = ? AND round = ? AND status = ?",
			instanceID, nodeID, round, model.TaskPending).
		Update("status", model.TaskReturned).Error
}

// nodeNextRound 节点在实例内的下一轮次（历史最大 round+1；首次为 1）。
func nodeNextRound(tx *gorm.DB, instanceID uint64, nodeID string) (int, error) {
	var maxRound int
	err := tx.Model(&model.Task{}).
		Where("instance_id = ? AND node_id = ?", instanceID, nodeID).
		Select("COALESCE(MAX(round),0)").Scan(&maxRound).Error
	return maxRound + 1, err
}

// instanceNextRound 全实例的下一轮次（重提任务用，保证时间线可回放）。
func instanceNextRound(tx *gorm.DB, instanceID uint64) (int, error) {
	var maxRound int
	err := tx.Model(&model.Task{}).
		Where("instance_id = ?", instanceID).
		Select("COALESCE(MAX(round),0)").Scan(&maxRound).Error
	return maxRound + 1, err
}

// loadSchema 加载实例冻结版本的定义节点树。
func (e *Engine) loadSchema(tx *gorm.DB, inst *model.ProcessInstance) (*flow.Schema, error) {
	var def model.ProcessDefinition
	if err := tx.Where("id = ?", inst.DefinitionID).First(&def).Error; err != nil {
		return nil, fmt.Errorf("加载流程定义失败: %w", err)
	}
	return flow.Parse(def.NodeTree)
}

// loadNode 加载实例冻结版本的定义并定位节点。
func (e *Engine) loadNode(tx *gorm.DB, inst *model.ProcessInstance, nodeID string) (*flow.Schema, *flow.Node, error) {
	sc, err := e.loadSchema(tx, inst)
	if err != nil {
		return nil, nil, err
	}
	node := flow.NodeByID(sc, nodeID)
	if node == nil {
		return nil, nil, fmt.Errorf("定义中找不到节点 %s", nodeID)
	}
	return sc, node, nil
}

func writeLog(tx *gorm.DB, inst *model.ProcessInstance, nodeID string, taskID uint64, action string, operatorID uint64, detail map[string]any) {
	var raw model.JSONB
	if detail != nil {
		if b, err := json.Marshal(detail); err == nil {
			raw = model.JSONB(b)
		}
	}
	// 日志写失败不阻断主流程（与全仓通知同理念），但在事务内尽力而为
	_ = tx.Create(&model.ProcessLog{
		TenantID: inst.TenantID, InstanceID: inst.ID, NodeID: nodeID,
		TaskID: taskID, Action: action, OperatorID: operatorID, Detail: raw,
	}).Error
}

func parseVars(raw []byte) instVars {
	var v instVars
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &v)
	}
	if v.SelectedAssignees == nil {
		v.SelectedAssignees = map[string][]uint64{}
	}
	return v
}

// dedupe 保序去重并剔除 0。
func dedupe(ids []uint64) []uint64 {
	seen := map[uint64]bool{}
	out := make([]uint64, 0, len(ids))
	for _, id := range ids {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}
