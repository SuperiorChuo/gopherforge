package api

// 任务端 handler：待办 / 已办 / 任务详情 / 同意 / 拒绝。
// 任务动作只校验 assignee 身份，不设权限码（设计文档 Q6 建议）。

import (
	"net/http"

	"github.com/gin-gonic/gin"
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
	actions := []string{}
	if t.AssigneeID == u.UserID && t.Status == model.TaskPending &&
		inst.Status == model.InstRunning {
		actions = []string{"approve", "reject"}
	}
	ok(c, gin.H{"task": t, "instance": inst, "actions": actions})
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
