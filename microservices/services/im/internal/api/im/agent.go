package api

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/im/internal/authjwt"
	"github.com/go-admin-kit/services/im/internal/model"
	"github.com/go-admin-kit/services/im/internal/store/im"
	"net/http"
	"strconv"
)

func (s *Server) canAccessConversation(c *gin.Context, conv *model.Conversation) bool {
	tok := bearer(c)
	if tok != "" {
		if g, err := authjwt.ParseGuest(s.Secret, tok); err == nil {
			if g.VisitorID != conv.VisitorID {
				Fail(c, http.StatusForbidden, "not your conversation")
				return false
			}
			return true
		}
	}
	if agent, ok := s.requireAgent(c); ok {
		if conv.TenantID != 0 && conv.TenantID != agent.TenantID {
			Fail(c, http.StatusForbidden, "conversation not in your tenant")
			return false
		}
		return true
	}
	return false
}

// requireAgentConversation 加载会话并确保其属于坐席所在租户。
func (s *Server) requireAgentConversation(c *gin.Context, agent *authjwt.AgentClaims) (*model.Conversation, bool) {
	conv, err := s.Store.GetConversationByPublicID(c.Param("public_id"))
	if err != nil {
		Fail(c, http.StatusNotFound, "conversation not found")
		return nil, false
	}
	if conv.TenantID != 0 && conv.TenantID != agent.TenantID {
		Fail(c, http.StatusNotFound, "conversation not found")
		return nil, false
	}
	return conv, true
}

type markReadReq struct {
	Seq int64 `json:"seq"` // <=0 → 读至最新
}

// publishRead 通知双方，使「已读」标记实时更新。
func (s *Server) publishRead(conv *model.Conversation, reader string, seq int64) {
	payload := gin.H{
		"conversation_public_id": conv.PublicID.String(),
		"reader":                 reader,
		"seq":                    seq,
	}
	s.Hub.Publish(conv.PublicID.String(), gin.H{"type": "conversation.read", "payload": payload})
	s.AgentHub.Publish(conv.TenantID, gin.H{"type": "conversation.read", "payload": payload})
}

// POST /api/v1/im/conversations/:public_id/read（访客标记已读）
func (s *Server) VisitorMarkRead(c *gin.Context) {
	guest, ok := s.requireGuest(c)
	if !ok {
		return
	}
	conv, err := s.Store.GetConversationByPublicID(c.Param("public_id"))
	if err != nil {
		Fail(c, http.StatusNotFound, "conversation not found")
		return
	}
	if conv.VisitorID != guest.VisitorID {
		Fail(c, http.StatusForbidden, "not your conversation")
		return
	}
	var req markReadReq
	_ = c.ShouldBindJSON(&req)
	seq, err := s.Store.MarkRead(conv, "visitor", req.Seq)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	s.publishRead(conv, "visitor", seq)
	OK(c, gin.H{"reader": "visitor", "seq": seq})
}

// POST /api/v1/im/agent/conversations/:public_id/read（坐席标记已读）
func (s *Server) AgentMarkRead(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	conv, ok := s.requireAgentConversation(c, agent)
	if !ok {
		return
	}
	var req markReadReq
	_ = c.ShouldBindJSON(&req)
	seq, err := s.Store.MarkRead(conv, "agent", req.Seq)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	s.publishRead(conv, "agent", seq)
	OK(c, gin.H{"reader": "agent", "seq": seq})
}

// GET /api/v1/im/agent/me（坐席个人信息）
func (s *Server) AgentMe(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	presence, _ := s.Store.GetPresence(agent.UserID)
	skills, _ := s.Store.ListAgentSkills(0, agent.UserID)
	load, _ := s.Store.CountAssignedForAgent(agent.UserID)
	groups, _ := s.Store.ListSkillGroups(agent.TenantID)
	groupMap := map[uint64]model.SkillGroup{}
	for _, g := range groups {
		groupMap[g.ID] = g
	}
	type skillView struct {
		model.AgentSkill
		SkillGroup *model.SkillGroup `json:"skill_group,omitempty"`
	}
	views := make([]skillView, 0, len(skills))
	for _, sk := range skills {
		v := skillView{AgentSkill: sk}
		if g, ok := groupMap[sk.SkillGroupID]; ok {
			gg := g
			v.SkillGroup = &gg
		} else {
			// 绑定到其他租户的技能组：从坐席视角省略
			continue
		}
		views = append(views, v)
	}
	OK(c, gin.H{
		"user_id":          agent.UserID,
		"username":         agent.Username,
		"tenant_id":        agent.TenantID,
		"presence":         presence,
		"skills":           views,
		"assigned_count":   load,
		"skill_groups_all": groups,
	})
}

type presenceReq struct {
	Status      string `json:"status"`
	DisplayName string `json:"display_name"`
}

// PUT /api/v1/im/agent/presence（更新坐席状态）
func (s *Server) AgentPresence(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	var req presenceReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Status == "" {
		Fail(c, http.StatusBadRequest, "status required")
		return
	}
	name := req.DisplayName
	if name == "" {
		name = agent.Username
	}
	p, err := s.Store.UpsertPresence(agent.UserID, req.Status, name)
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	s.AgentHub.Publish(agent.TenantID, gin.H{"type": "presence.updated", "payload": p})
	// 上线时，尝试为坐席的技能组分配排队会话
	if p.Status == "online" {
		s.tryAssignQueuedForAgent(agent.TenantID, agent.UserID)
	}
	OK(c, p)
}

// tryAssignQueuedForAgent 坐席上线后给它的技能组做一轮派单。
// 查询数从「每技能组 1 次组查询 + 1 次会话列表 + 每会话 1 次重读」收敛为
// 1 次租户技能组列表 + 1 次跨组排队会话列表 + 仅对真正派出去的会话重读。
func (s *Server) tryAssignQueuedForAgent(tenantID, agentUserID uint64) {
	skills, err := s.Store.ListAgentSkills(0, agentUserID)
	if err != nil {
		return
	}
	// 租户归属校验保持不变（跨租户技能组一律跳过），但改成一次列表 + 集合判定，
	// 而不是每个绑定一次 GetSkillGroupForTenant。
	tenantGroups, err := s.Store.ListSkillGroups(tenantID)
	if err != nil {
		return
	}
	owned := make(map[uint64]struct{}, len(tenantGroups))
	for _, g := range tenantGroups {
		owned[g.ID] = struct{}{}
	}
	groupIDs := make([]uint64, 0, len(skills))
	for _, sk := range skills {
		if sk.Status != 1 {
			continue
		}
		if _, ok := owned[sk.SkillGroupID]; !ok {
			continue
		}
		groupIDs = append(groupIDs, sk.SkillGroupID)
	}
	if len(groupIDs) > 0 {
		list, err := s.Store.ListQueuedForSkillGroups(tenantID, groupIDs, 20*len(groupIDs))
		if err == nil {
			for i := range list {
				before := list[i].Status
				_ = s.Store.TryAutoAssign(&list[i])
				// 只有真正改了状态才重读发广播；没派出去的会话没有新状态可播。
				if list[i].Status == before {
					continue
				}
				if refreshed, err := s.Store.GetConversationByPublicID(list[i].PublicID.String()); err == nil {
					s.AgentHub.Publish(refreshed.TenantID, gin.H{"type": "conversation.updated", "payload": refreshed})
				}
			}
		}
	}
	s.AgentHub.Publish(tenantID, gin.H{"type": "queue.updated", "payload": gin.H{}})
}

// GET /api/v1/im/agent/conversations?scope=all|mine|queue&skill_group_id=（坐席会话列表）
func (s *Server) AgentListConversations(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	scope := c.DefaultQuery("scope", "all")
	sgID, _ := strconv.ParseUint(c.DefaultQuery("skill_group_id", "0"), 10, 64)
	list, err := s.Store.ListAgentConversations(agent.TenantID, agent.UserID, scope, sgID, 100)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ids := make([]uint64, 0, len(list))
	for _, cv := range list {
		ids = append(ids, cv.ID)
	}
	unread, _ := s.Store.AgentUnreadCounts(ids)
	type row struct {
		model.Conversation
		UnreadCount int64 `json:"unread_count"`
	}
	rows := make([]row, 0, len(list))
	for _, cv := range list {
		rows = append(rows, row{Conversation: cv, UnreadCount: unread[cv.ID]})
	}
	OK(c, gin.H{"list": rows, "scope": scope, "tenant_id": agent.TenantID})
}

// GET /api/v1/im/agent/queue — 仅排队会话 + 计数
func (s *Server) AgentQueue(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	sgID, _ := strconv.ParseUint(c.DefaultQuery("skill_group_id", "0"), 10, 64)
	list, err := s.Store.ListAgentConversations(agent.TenantID, 0, "queue", sgID, 100)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	online, _ := s.Store.ListPresence(agent.TenantID, "online", "busy")
	OK(c, gin.H{
		"list":           list,
		"queue_size":     len(list),
		"online_agents":  online,
		"skill_group_id": sgID,
		"tenant_id":      agent.TenantID,
	})
}

// GET /api/v1/im/agent/online — 列出在线/忙碌坐席供转接选择
func (s *Server) AgentOnlineList(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	list, err := s.Store.ListPresence(agent.TenantID, "online", "busy", "offline")
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	type row struct {
		model.AgentPresence
		AssignedCount int64 `json:"assigned_count"`
	}
	// 一条 GROUP BY 换掉「每个坐席一次 CountAssignedForAgent」
	agentIDs := make([]uint64, 0, len(list))
	for _, p := range list {
		agentIDs = append(agentIDs, p.AgentUserID)
	}
	loads, _ := s.Store.CountAssignedForAgents(agentIDs)
	out := make([]row, 0, len(list))
	for _, p := range list {
		out = append(out, row{AgentPresence: p, AssignedCount: loads[p.AgentUserID]})
	}
	OK(c, gin.H{"list": out})
}

// POST /api/v1/im/agent/conversations/:public_id/accept（接单/接受会话）
func (s *Server) AgentAccept(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	conv, ok := s.requireAgentConversation(c, agent)
	if !ok {
		return
	}
	if conv.Status == "closed" {
		Fail(c, http.StatusBadRequest, "conversation closed")
		return
	}
	if err := s.Store.AssignConversation(conv, agent.UserID); err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.Store.AppendSystemEvent(conv.ID, "assigned", map[string]any{
		"agent_user_id": agent.UserID,
		"auto":          false,
	})
	if refreshed, err := s.Store.GetConversationByPublicID(conv.PublicID.String()); err == nil {
		conv = refreshed
	}
	s.AgentHub.Publish(conv.TenantID, gin.H{"type": "conversation.updated", "payload": conv})
	s.AgentHub.Publish(conv.TenantID, gin.H{"type": "queue.updated", "payload": gin.H{}})
	s.Hub.Publish(conv.PublicID.String(), gin.H{
		"type":    "conversation.updated",
		"payload": conv,
	})
	OK(c, conv)
}

type transferReq struct {
	TargetAgentUserID *uint64 `json:"target_agent_user_id"` // nil 或 0 = 重新排队
	SkillGroupID      *uint64 `json:"skill_group_id"`
	Note              string  `json:"note"`
}

// POST /api/v1/im/agent/conversations/:public_id/transfer（转接会话）
func (s *Server) AgentTransfer(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	conv, ok := s.requireAgentConversation(c, agent)
	if !ok {
		return
	}
	var req transferReq
	_ = c.ShouldBindJSON(&req)
	target := uint64(0)
	if req.TargetAgentUserID != nil {
		target = *req.TargetAgentUserID
	}
	if err := s.Store.TransferConversation(conv, agent.UserID, target, req.SkillGroupID, req.Note); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if refreshed, err := s.Store.GetConversationByPublicID(conv.PublicID.String()); err == nil {
		conv = refreshed
	}
	s.AgentHub.Publish(conv.TenantID, gin.H{"type": "conversation.updated", "payload": conv})
	s.AgentHub.Publish(conv.TenantID, gin.H{"type": "queue.updated", "payload": gin.H{}})
	s.Hub.Publish(conv.PublicID.String(), gin.H{"type": "conversation.updated", "payload": conv})
	// 通过消息轮询或事件，向会话订阅者广播转接系统消息
	msgs, _ := s.Store.ListMessages(conv.ID, 0, 1)
	_ = msgs
	OK(c, conv)
}

type closeReq struct {
	Reason string `json:"reason"`
}

// POST /api/v1/im/agent/conversations/:public_id/close（关闭会话）
func (s *Server) AgentClose(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	conv, ok := s.requireAgentConversation(c, agent)
	if !ok {
		return
	}
	var req closeReq
	_ = c.ShouldBindJSON(&req)
	reason := req.Reason
	if reason == "" {
		reason = "agent"
	}
	if err := s.Store.CloseConversation(conv, store.CloseOpts{Reason: reason}); err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.Store.AppendSystemEvent(conv.ID, "closed", map[string]any{
		"by_agent_user_id": agent.UserID,
		"reason":           reason,
	})
	if refreshed, err := s.Store.GetConversationByPublicID(conv.PublicID.String()); err == nil {
		conv = refreshed
	}
	s.AgentHub.Publish(conv.TenantID, gin.H{"type": "conversation.updated", "payload": conv})
	s.Hub.Publish(conv.PublicID.String(), gin.H{"type": "conversation.updated", "payload": conv})
	OK(c, conv)
}
