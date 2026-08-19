package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-admin-kit/services/bpm/internal/flow"
	"github.com/go-admin-kit/services/bpm/internal/model"
	"gorm.io/gorm"
	"time"
)

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
	if err := tx.Model(inst).Updates(map[string]any{
		"status": result, "current_node_id": "", "finished_at": now,
	}).Error; err != nil {
		return err
	}
	return enqueueCallback(tx, inst)
}

func enqueueCallback(tx *gorm.DB, inst *model.ProcessInstance) error {
	return tx.Create(&model.CallbackJob{
		TenantID: inst.TenantID, InstanceID: inst.ID, NextAt: time.Now(), Status: "pending",
	}).Error
}
