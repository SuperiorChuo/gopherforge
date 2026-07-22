package api

// 实例端 handler：发起（用户侧 / internal 业务侧）、我发起的、撤销、
// 详情、时间线、流转图、by-biz 反查。

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/bpm/internal/engine"
	"github.com/go-admin-kit/services/bpm/internal/flow"
	"github.com/go-admin-kit/services/bpm/internal/form"
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

// CreateInstance 用户侧发起（表单构建器 M1，仅"流程表单"定义）：快照按
// form_schema 服务端权威校验；biz_type 强制 flow_form、biz_id 服务端生成，
// 客户端传入的业务锚点一律忽略——业务表单只能经 internal 端点由业务后端
// 发起，堵住用户侧伪造业务审批（终态回调打向业务方）的口子。
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
	def, err := s.Store.ActiveByKey(req.DefinitionKey, u.TenantID)
	if err != nil {
		fail(c, http.StatusBadRequest, "流程不存在或未发布")
		return
	}
	fs, err := form.Parse(def.FormSchema)
	if err != nil || fs == nil {
		fail(c, http.StatusBadRequest, "该流程未配置发起表单，请从业务入口发起")
		return
	}
	snapshot, err := fs.ValidateSnapshot(req.FormSnapshot)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = def.Name
		if u.Username != "" {
			title = def.Name + "（" + u.Username + "）"
		}
	}
	eff, err := s.Engine.Start(engine.StartInput{
		TenantID:      u.TenantID,
		DefinitionKey: def.Key,
		Title:         title,
		BizType:       model.BizTypeFlowForm,
		BizID:         randomBizID(),
		FormSnapshot:  snapshot,
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

// randomBizID flow_form 实例的业务锚点：随机 hex 串——同一表单可重复发起，
// 不撞 ux_bpm_inst_biz_running 防重索引。
func randomBizID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ListStartable GET /api/v1/bpm/startable —— 可发起流程列表（通用发起页；
// 登录即可访问，发起权是普适权）。
func (s *Server) ListStartable(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	list, err := s.Store.ListStartable(u.TenantID)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"list": list})
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

// ListInstances handles GET /api/v1/bpm/instances — 全部实例（M3 管理视图），
// 仅平台管理员（配合终止动作处置挂起/异常实例）。
func (s *Server) ListInstances(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	if !u.PlatformAdmin {
		fail(c, http.StatusForbidden, "仅平台管理员可查看全部实例")
		return
	}
	list, total, err := s.Store.ListAllInstances(u.TenantID, c.Query("status"), c.Query("keyword"), pageOf(c))
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"list": list, "total": total})
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

// ResubmitInstance handles POST /api/v1/bpm/instances/:id/resubmit —
// 被退回后发起人修改快照重新提交（M2）；form_snapshot 缺省沿用旧快照。
func (s *Server) ResubmitInstance(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	var req struct {
		FormSnapshot json.RawMessage `json:"form_snapshot"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && c.Request.ContentLength > 0 {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	eff, err := s.Engine.Resubmit(u.TenantID, id, u.UserID, req.FormSnapshot)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	s.applyEffects(eff)
	ok(c, gin.H{"instance_id": id, "status": eff.Instance.Status})
}

// TerminateInstance handles POST /api/v1/bpm/instances/:id/terminate —
// 管理员强制终止（M3）：仅平台管理员；running / suspended 均可终止（挂起
// 实例的管理出口），原因必填，业务回调按 canceled 语义处理。
func (s *Server) TerminateInstance(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	if !u.PlatformAdmin {
		fail(c, http.StatusForbidden, "仅平台管理员可终止流程")
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	var req struct {
		Comment string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "comment 必填")
		return
	}
	eff, err := s.Engine.Terminate(u.TenantID, id, u.UserID, req.Comment)
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
	// 附冻结版本的 form_schema（流程表单模式的详情展示 / 重提动态渲染）
	var formSchema json.RawMessage
	var def model.ProcessDefinition
	if err := s.Store.DB().Where("id = ?", inst.DefinitionID).First(&def).Error; err == nil {
		if fs, err := form.Parse(def.FormSchema); err == nil && fs != nil {
			formSchema = json.RawMessage(def.FormSchema)
		}
	}
	s.filterSnapshotForViewer(c, inst)
	ok(c, struct {
		*model.ProcessInstance
		FormSchema json.RawMessage `json:"form_schema,omitempty"`
	}{inst, formSchema})
}

// filterSnapshotForViewer 字段权限（M1 hidden）：非发起人/非管理员的参与者，
// 按其任务节点的 fieldPerms 并集过滤实例快照——实例详情/流转图与任务详情
// 同口径，防绕过（设计文档 bpm-form-builder.md §3）。
func (s *Server) filterSnapshotForViewer(c *gin.Context, inst *model.ProcessInstance) {
	u, authed := s.requireUser(c) // 调用方已过鉴权，此处必成功
	if !authed || inst.InitiatorID == u.UserID || u.PlatformAdmin {
		return
	}
	sc, err := s.Store.InstanceSchema(inst)
	if err != nil {
		return
	}
	merged := map[string]string{}
	for _, nodeID := range s.Store.ListUserTaskNodeIDs(inst.ID, inst.TenantID, u.UserID) {
		if node := flow.NodeByID(sc, nodeID); node != nil {
			for k, v := range node.FieldPerms {
				merged[k] = v
			}
		}
	}
	if len(merged) > 0 {
		inst.FormSnapshot = filterSnapshot(inst.FormSnapshot, merged)
	}
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

	// M3：状态判定改为"节点自身活动痕迹"口径（任务 / 抄送记录 / branch、
	// auto_pass 日志），不再依赖主链线性顺序——条件分支下未走的分支节点
	// 无任何痕迹，自然落 todo / skipped。
	logs, _ := s.Store.ListInstanceLogs(inst.ID, inst.TenantID)
	activityByNode := map[string]bool{}
	for _, lg := range logs {
		if lg.NodeID == "" {
			continue
		}
		if lg.Action == model.ActionBranch || lg.Action == model.ActionAutoPass {
			activityByNode[lg.NodeID] = true
		}
	}

	s.filterSnapshotForViewer(c, inst)
	nodes := map[string]diagramNode{}
	finished := inst.Status == model.InstApproved ||
		inst.Status == model.InstRejected || inst.Status == model.InstCanceled
	for _, n := range flow.Nodes(sc) {
		dn := diagramNode{Tasks: tasksByNode[n.ID]}
		switch {
		case n.ID == inst.CurrentNodeID && !finished:
			// 优先于 start 判断：被退回待重提时游标在 start，应显示 doing（M2）
			dn.State = "doing"
		case n.Type == flow.TypeStart:
			dn.State = "done"
		case len(dn.Tasks) > 0:
			// 有任务的节点：全 skipped/canceled 视为 skipped，否则 done
			dn.State = "done"
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
		case (n.Type == flow.TypeCc && ccByNode[n.ID]) || activityByNode[n.ID]:
			dn.State = "done"
		default:
			// 未走到的节点（含未命中的分支子链）：终态后定格 skipped，
			// 运行中显示 todo
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
