package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-admin-kit/services/bpm/internal/flow"
	"github.com/go-admin-kit/services/bpm/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

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
