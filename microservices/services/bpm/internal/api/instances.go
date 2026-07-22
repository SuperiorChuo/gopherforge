package api

// 实例端 handler：发起（用户侧 / internal 业务侧）、我发起的、撤销、
// 详情、时间线、流转图、by-biz 反查。

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/bpm/internal/engine"
	"github.com/go-admin-kit/services/bpm/internal/flow"
	"github.com/go-admin-kit/services/bpm/internal/model"
)

type startInstanceReq struct {
	DefinitionKey string          `json:"definition_key"`
	Title         string          `json:"title"`
	BizType       string          `json:"biz_type"`
	BizID         string          `json:"biz_id"`
	FormSnapshot  json.RawMessage `json:"form_snapshot"`
	Variables     json.RawMessage `json:"variables"`
	// InitiatorID / InitiatorDept 仅 internal 变体生效（业务后端权威指定）
	InitiatorID   uint64 `json:"initiator_id"`
	InitiatorDept uint64 `json:"initiator_dept"`
}

// CreateInstance 用户侧发起（M1 主要走业务侧 internal 变体；此端点供
// 未来通用发起页 / 脚手架 demo 场景）。
func (s *Server) CreateInstance(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	var req startInstanceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	eff, err := s.Engine.Start(engine.StartInput{
		TenantID:      u.TenantID,
		DefinitionKey: req.DefinitionKey,
		Title:         req.Title,
		BizType:       req.BizType,
		BizID:         req.BizID,
		FormSnapshot:  req.FormSnapshot,
		Variables:     req.Variables,
		InitiatorID:   u.UserID,
	})
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	s.applyEffects(eff)
	ok(c, gin.H{"instance_id": eff.Instance.ID, "status": eff.Instance.Status})
}

// InternalCreateInstance 业务方服务端到服务端发起（X-Internal-Token；
// 表单快照由业务后端权威生成）。
func (s *Server) InternalCreateInstance(c *gin.Context) {
	if !s.requireInternal(c) {
		return
	}
	var req startInstanceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	if req.InitiatorID == 0 {
		fail(c, http.StatusBadRequest, "initiator_id 必填")
		return
	}
	eff, err := s.Engine.Start(engine.StartInput{
		TenantID:      internalTenant(c),
		DefinitionKey: req.DefinitionKey,
		Title:         req.Title,
		BizType:       req.BizType,
		BizID:         req.BizID,
		FormSnapshot:  req.FormSnapshot,
		Variables:     req.Variables,
		InitiatorID:   req.InitiatorID,
		InitiatorDept: req.InitiatorDept,
	})
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	s.applyEffects(eff)
	ok(c, gin.H{"instance_id": eff.Instance.ID, "status": eff.Instance.Status})
}

func (s *Server) MyInstances(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	list, total, err := s.Store.ListMyInstances(u.TenantID, u.UserID, c.Query("status"), pageOf(c))
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"list": list, "total": total})
}

func (s *Server) CancelInstance(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	eff, err := s.Engine.Cancel(u.TenantID, id, u.UserID)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	s.applyEffects(eff)
	ok(c, gin.H{"instance_id": id, "status": eff.Instance.Status})
}

// loadVisibleInstance 详情类接口公共前置：取实例 + 可见性校验
// （发起人 ∪ 参与者 ∪ 被抄送人 ∪ 平台管理员）。
func (s *Server) loadVisibleInstance(c *gin.Context) (*model.ProcessInstance, bool) {
	u, authed := s.requireUser(c)
	if !authed {
		return nil, false
	}
	id, valid := pathID(c, "id")
	if !valid {
		return nil, false
	}
	inst, err := s.Store.GetInstance(id, u.TenantID)
	if err != nil {
		notFoundOr(c, err, "流程实例不存在")
		return nil, false
	}
	if !u.PlatformAdmin && !s.Store.CanView(inst, u.UserID) {
		fail(c, http.StatusForbidden, "无权查看该流程实例")
		return nil, false
	}
	return inst, true
}

func (s *Server) GetInstance(c *gin.Context) {
	inst, okv := s.loadVisibleInstance(c)
	if !okv {
		return
	}
	ok(c, inst)
}

func (s *Server) InstanceTimeline(c *gin.Context) {
	inst, okv := s.loadVisibleInstance(c)
	if !okv {
		return
	}
	logs, err := s.Store.ListInstanceLogs(inst.ID, inst.TenantID)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"list": logs})
}

// diagramNode 流转图上单节点的运行时标注。
type diagramNode struct {
	State string       `json:"state"` // done|doing|todo|skipped
	Tasks []model.Task `json:"tasks,omitempty"`
}

// InstanceDiagram 流转图数据：定义 node_tree + 每个节点的运行时标注，
// 前端据此给纵向卡片流做进度着色。
func (s *Server) InstanceDiagram(c *gin.Context) {
	inst, okv := s.loadVisibleInstance(c)
	if !okv {
		return
	}
	var def model.ProcessDefinition
	if err := s.Store.DB().Where("id = ?", inst.DefinitionID).First(&def).Error; err != nil {
		fail(c, http.StatusInternalServerError, "加载流程定义失败")
		return
	}
	sc, err := flow.Parse(def.NodeTree)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	tasks, err := s.Store.ListInstanceTasks(inst.ID, inst.TenantID)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	tasksByNode := map[string][]model.Task{}
	for _, t := range tasks {
		tasksByNode[t.NodeID] = append(tasksByNode[t.NodeID], t)
	}
	ccRecords, _ := s.Store.ListInstanceCc(inst.ID, inst.TenantID)
	ccByNode := map[string]bool{}
	for _, r := range ccRecords {
		ccByNode[r.NodeID] = true
	}

	nodes := map[string]diagramNode{}
	finished := inst.Status == model.InstApproved ||
		inst.Status == model.InstRejected || inst.Status == model.InstCanceled
	passedCurrent := false
	for _, n := range flow.Nodes(sc) {
		dn := diagramNode{Tasks: tasksByNode[n.ID]}
		switch {
		case n.Type == flow.TypeStart:
			dn.State = "done"
		case n.ID == inst.CurrentNodeID && !finished:
			dn.State = "doing"
			passedCurrent = true
		case passedCurrent && !finished:
			dn.State = "todo"
		case len(dn.Tasks) > 0 || (n.Type == flow.TypeCc && ccByNode[n.ID]):
			// 有任务/抄送记录的节点：全 skipped 视为 skipped，否则 done
			dn.State = "done"
			if len(dn.Tasks) > 0 {
				allSkipped := true
				for _, t := range dn.Tasks {
					if t.Status != model.TaskSkipped && t.Status != model.TaskCanceled {
						allSkipped = false
						break
					}
				}
				if allSkipped {
					dn.State = "skipped"
				}
			}
		default:
			// 终态后未走到 / 运行中当前节点之前无任务的节点
			if finished {
				dn.State = "skipped"
			} else {
				dn.State = "todo"
			}
		}
		nodes[n.ID] = dn
	}
	ok(c, gin.H{
		"instance":  inst,
		"node_tree": json.RawMessage(def.NodeTree),
		"nodes":     nodes,
	})
}

// InternalInstancesByBiz 业务侧按 (biz_type,biz_id) 反查实例（在途+历史）。
func (s *Server) InternalInstancesByBiz(c *gin.Context) {
	if !s.requireInternal(c) {
		return
	}
	bizType, bizID := c.Query("biz_type"), c.Query("biz_id")
	if bizType == "" || bizID == "" {
		fail(c, http.StatusBadRequest, "biz_type / biz_id 必填")
		return
	}
	list, err := s.Store.FindByBiz(internalTenant(c), bizType, bizID)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"list": list})
}
