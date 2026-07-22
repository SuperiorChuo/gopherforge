package api

// 任务端 handler：待办 / 已办 / 任务详情 / 同意 / 拒绝 / 转办 / 退回（M2）。
// 任务动作只校验 assignee 身份，不设权限码（设计文档 Q6 建议）。

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/bpm/internal/flow"
	"github.com/go-admin-kit/services/bpm/internal/model"
)

func (s *Server) ListTodo(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	list, total, err := s.Store.ListTodo(u.TenantID, u.UserID, pageOf(c))
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"list": list, "total": total})
}

func (s *Server) ListDone(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	list, total, err := s.Store.ListDone(u.TenantID, u.UserID, pageOf(c))
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"list": list, "total": total})
}

// GetTask 任务详情：任务 + 实例摘要（含 form_snapshot）+ 我可用的动作。
func (s *Server) GetTask(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	t, err := s.Store.GetTask(id, u.TenantID)
	if err != nil {
		notFoundOr(c, err, "任务不存在")
		return
	}
	inst, err := s.Store.GetInstance(t.InstanceID, u.TenantID)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	// 可见性：任务处理人天然可见；其余按实例可见性规则
	if t.AssigneeID != u.UserID && !u.PlatformAdmin && !s.Store.CanView(inst, u.UserID) {
		fail(c, http.StatusForbidden, "无权查看该任务")
		return
	}
	actions := s.taskActions(t, inst, u.UserID)
	ok(c, gin.H{"task": t, "instance": inst, "actions": actions})
}

// taskActions 当前用户对任务可用的动作列表（前端契约：approve / reject /
// transfer / return_start / return_prev / resubmit，勿改动作名）。
func (s *Server) taskActions(t *model.Task, inst *model.ProcessInstance, userID uint64) []string {
	actions := []string{}
	if t.AssigneeID != userID || t.Status != model.TaskPending ||
		inst.Status != model.InstRunning {
		return actions
	}
	sc, err := s.Store.InstanceSchema(inst)
	if err != nil {
		// 定义解析异常时回退 M1 基础动作，不因动作列表阻断详情
		return []string{"approve", "reject"}
	}
	node := flow.NodeByID(sc, t.NodeID)
	if node != nil && node.Type == flow.TypeStart {
		// 被退回后的"重新提交"任务：发起人走重提（或实例撤销）
		return []string{"resubmit"}
	}
	actions = append(actions, "approve", "reject", "transfer", "return_start")
	// return_prev 可用性按执行路径判定（M3）：本实例存在当前节点之外的
	// 历史审批任务（与引擎 prevApprovalNode 同口径的存在性探测）
	if node != nil && node.AllowBackPrev && sc.Start != nil &&
		s.Store.HasPrevApprovalTask(inst.ID, inst.TenantID, node.ID, sc.Start.ID) {
		actions = append(actions, "return_prev")
	}
	return actions
}

type actReq struct {
	Comment string `json:"comment"`
}

func (s *Server) ApproveTask(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	var req actReq
	if err := c.ShouldBindJSON(&req); err != nil && c.Request.ContentLength > 0 {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	eff, err := s.Engine.Approve(u.TenantID, id, u.UserID, req.Comment)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	s.applyEffects(eff)
	ok(c, gin.H{
		"task_id":         id,
		"instance_id":     eff.Instance.ID,
		"instance_status": eff.Instance.Status,
	})
}

func (s *Server) RejectTask(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	var req actReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "comment 必填")
		return
	}
	eff, err := s.Engine.Reject(u.TenantID, id, u.UserID, req.Comment)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	s.applyEffects(eff)
	ok(c, gin.H{
		"task_id":         id,
		"instance_id":     eff.Instance.ID,
		"instance_status": eff.Instance.Status,
	})
}

// ---------- M2：转办 / 退回 ----------

type transferReq struct {
	TargetUserID uint64 `json:"target_user_id"`
	Comment      string `json:"comment"`
}

// TransferTask handles POST /api/v1/bpm/tasks/:id/transfer
func (s *Server) TransferTask(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	var req transferReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	eff, err := s.Engine.Transfer(u.TenantID, id, u.UserID, req.TargetUserID, req.Comment)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	s.applyEffects(eff)
	ok(c, gin.H{
		"task_id":         id,
		"target_user_id":  req.TargetUserID,
		"instance_id":     eff.Instance.ID,
		"instance_status": eff.Instance.Status,
	})
}

type returnReq struct {
	To      string `json:"to"` // start | prev
	Comment string `json:"comment"`
}

// ReturnTask handles POST /api/v1/bpm/tasks/:id/return
func (s *Server) ReturnTask(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	var req returnReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	eff, err := s.Engine.Return(u.TenantID, id, u.UserID, req.To, req.Comment)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	s.applyEffects(eff)
	ok(c, gin.H{
		"task_id":         id,
		"to":              req.To,
		"instance_id":     eff.Instance.ID,
		"instance_status": eff.Instance.Status,
	})
}
