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
	ErrNotInitiator       = errors.New("仅发起人可撤销")
	ErrCancelDenied       = errors.New("已有审批通过记录，不可撤销")
	ErrCommentRequired    = errors.New("拒绝必须填写意见")
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
		sc, node, err := e.loadNode(tx, inst, task.NodeID)
		if err != nil {
			return err
		}
		return e.advanceFrom(tx, inst, sc, node, eff)
	})
	if err != nil {
		return nil, err
	}
	return eff, nil
}

// Reject 拒绝：任务置 rejected → 按计数规则判定节点是否拒绝 →
// 节点拒绝时（M1 onReject 恒为 reject）实例终态 rejected。
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
		case flow.MultiAnd: // 会签：任一拒绝即节点拒绝
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
		// M1 onReject 恒为 reject：其余 pending 置 skipped → 实例终态 rejected
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

// ---------- 撤销 ----------

// Cancel 撤销：仅发起人、实例 running 且尚无任何通过记录时允许（M1 从严）。
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
		if approved > 0 {
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

// ---------- 推进核心 ----------

// advanceFrom 从 done 节点（已完成）的下一个节点开始推进游标，直到
// 停在一个等待人工的审批节点、挂起、或到达链尾终态 approved。
func (e *Engine) advanceFrom(tx *gorm.DB, inst *model.ProcessInstance, sc *flow.Schema, done *flow.Node, eff *Effects) error {
	node := done.Next
	for {
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
					NodeID: node.ID, UserID: uid,
				}
				if err := tx.Create(&rec).Error; err != nil {
					return err
				}
				eff.CcRecords = append(eff.CcRecords, rec)
			}
			writeLog(tx, inst, node.ID, 0, model.ActionCc, 0,
				map[string]any{"user_ids": users})
			node = node.Next

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
					node = node.Next
					continue
				case flow.FallbackToUsers:
					users = dedupe(node.Assignee.FallbackUserIDs)
				}
				if len(users) == 0 { // suspend（缺省）或兜底人也为空
					inst.Status = model.InstSuspended
					inst.CurrentNodeID = node.ID
					writeLog(tx, inst, node.ID, 0, model.ActionSuspend, 0,
						map[string]any{"reason": "审批人解析为空，实例挂起待管理员处理"})
					return tx.Model(inst).Updates(map[string]any{
						"status": inst.Status, "current_node_id": inst.CurrentNodeID,
					}).Error
				}
			}
			mode := node.EffectiveMultiMode()
			var timeoutAt *time.Time
			if node.TimeoutHours > 0 {
				t := time.Now().Add(time.Duration(node.TimeoutHours) * time.Hour)
				timeoutAt = &t
			}
			for _, uid := range users {
				task := model.Task{
					TenantID: inst.TenantID, InstanceID: inst.ID,
					NodeID: node.ID, NodeName: node.Name, Round: 1,
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
			return errors.New("条件分支节点 M1 未启用（发布校验应已拦截）")
		default:
			return fmt.Errorf("未知节点类型: %s", node.Type)
		}
	}
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
	default:
		return nil, fmt.Errorf("审批人规则 %s M1 未支持", rule.Type)
	}
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

// loadNode 加载实例冻结版本的定义并定位节点。
func (e *Engine) loadNode(tx *gorm.DB, inst *model.ProcessInstance, nodeID string) (*flow.Schema, *flow.Node, error) {
	var def model.ProcessDefinition
	if err := tx.Where("id = ?", inst.DefinitionID).First(&def).Error; err != nil {
		return nil, nil, fmt.Errorf("加载流程定义失败: %w", err)
	}
	sc, err := flow.Parse(def.NodeTree)
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
