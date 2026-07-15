package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/im/internal/authjwt"
	"github.com/go-admin-kit/services/im/internal/hub"
	"github.com/go-admin-kit/services/im/internal/model"
	"github.com/go-admin-kit/services/im/internal/store"
	"github.com/google/uuid"
)

type Server struct {
	Store    *store.Store
	Hub      *hub.Hub
	AgentHub *hub.AgentHub
	Secret   string
}

func OK(c *gin.Context, data any) {
	// 与脚手架前端约定：业务成功 code == 200
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": data})
}

func Fail(c *gin.Context, httpCode int, msg string) {
	c.JSON(httpCode, gin.H{"code": httpCode, "message": msg, "data": nil})
}

func bearer(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func (s *Server) requireGuest(c *gin.Context) (*authjwt.GuestClaims, bool) {
	tok := bearer(c)
	if tok == "" {
		Fail(c, http.StatusUnauthorized, "missing guest token")
		return nil, false
	}
	claims, err := authjwt.ParseGuest(s.Secret, tok)
	if err != nil {
		Fail(c, http.StatusUnauthorized, "invalid guest token")
		return nil, false
	}
	return claims, true
}

func (s *Server) requireAgent(c *gin.Context) (*authjwt.AgentClaims, bool) {
	// Prefer gateway header if present
	if uid := c.GetHeader("X-Auth-User-ID"); uid != "" {
		id, err := strconv.ParseUint(uid, 10, 64)
		if err == nil && id > 0 {
			return &authjwt.AgentClaims{UserID: id, Username: c.GetHeader("X-Auth-Username")}, true
		}
	}
	tok := bearer(c)
	if tok == "" {
		Fail(c, http.StatusUnauthorized, "missing agent token")
		return nil, false
	}
	claims, err := authjwt.ParseAgent(s.Secret, tok)
	if err != nil {
		Fail(c, http.StatusUnauthorized, "invalid agent token")
		return nil, false
	}
	return claims, true
}

func parentOrigin(c *gin.Context, bodyOrigin string) string {
	if bodyOrigin != "" {
		return bodyOrigin
	}
	if v := c.Query("parent_origin"); v != "" {
		return v
	}
	if v := c.GetHeader("X-Parent-Origin"); v != "" {
		return v
	}
	return c.GetHeader("Origin")
}

// GET /api/v1/im/widget/config?app_key=demo
func (s *Server) WidgetConfig(c *gin.Context) {
	appKey := c.Query("app_key")
	if appKey == "" {
		appKey = "demo"
	}
	site, err := s.Store.GetSiteByAppKey(appKey)
	if err != nil {
		Fail(c, http.StatusNotFound, "site not found")
		return
	}
	// iframe 场景 Origin 是 IM 域；用 parent_origin 校验客户站域名
	origin := parentOrigin(c, "")
	if origin != "" && origin != "null" && !store.OriginAllowed(site.AllowedOrigins, origin) {
		Fail(c, http.StatusForbidden, "origin denied")
		return
	}
	OK(c, gin.H{
		"app_key":      site.AppKey,
		"name":         site.Name,
		"welcome_text": site.WelcomeText,
		"snippet":      embedSnippet(c, site.AppKey),
	})
}

func embedSnippet(c *gin.Context, appKey string) string {
	// Prefer public gateway host when behind reverse proxy
	host := c.Request.Host
	scheme := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	base := scheme + "://" + host
	return `<script src="` + base + `/im/widget/widget.js" data-app-key="` + appKey + `" async></script>`
}

type visitorSessionReq struct {
	AppKey       string `json:"app_key"`
	GuestKey     string `json:"guest_key"`
	DisplayName  string `json:"display_name"`
	ParentOrigin string `json:"parent_origin"`
}

// POST /api/v1/im/visitor/session
func (s *Server) VisitorSession(c *gin.Context) {
	var req visitorSessionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	if req.AppKey == "" {
		req.AppKey = "demo"
	}
	if req.GuestKey == "" {
		req.GuestKey = uuid.NewString()
	}
	site, err := s.Store.GetSiteByAppKey(req.AppKey)
	if err != nil {
		Fail(c, http.StatusNotFound, "site not found")
		return
	}
	origin := parentOrigin(c, req.ParentOrigin)
	if origin != "" && origin != "null" && !store.OriginAllowed(site.AllowedOrigins, origin) {
		Fail(c, http.StatusForbidden, "origin denied")
		return
	}
	v, err := s.Store.UpsertVisitor(site.ID, req.GuestKey, req.DisplayName)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	token, err := authjwt.MintGuest(s.Secret, v.ID, site.ID, v.GuestKey, 24*time.Hour)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	OK(c, gin.H{
		"guest_token":  token,
		"guest_key":    v.GuestKey,
		"visitor_id":   v.ID,
		"display_name": v.DisplayName,
		"welcome_text": site.WelcomeText,
	})
}

type createConvReq struct {
	Channel        string         `json:"channel"`
	Context        map[string]any `json:"context"`
	SkillGroupID   *uint64        `json:"skill_group_id"`
	SkillGroupCode string         `json:"skill_group_code"`
}

// POST /api/v1/im/conversations
func (s *Server) CreateConversation(c *gin.Context) {
	guest, ok := s.requireGuest(c)
	if !ok {
		return
	}
	var req createConvReq
	_ = c.ShouldBindJSON(&req)
	ctxJSON := store.JSONText(req.Context)

	var skillID *uint64
	if req.SkillGroupID != nil && *req.SkillGroupID > 0 {
		skillID = req.SkillGroupID
	} else if req.SkillGroupCode != "" {
		if sg, err := s.Store.GetSkillGroupByCode(req.SkillGroupCode); err == nil {
			skillID = &sg.ID
		}
	} else if site, err := s.Store.GetSite(guest.SiteID); err == nil && site.DefaultSkillGroupID != nil {
		skillID = site.DefaultSkillGroupID
	}

	conv, err := s.Store.EnsureOpenConversation(guest.SiteID, guest.VisitorID, req.Channel, ctxJSON, skillID)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	// refresh after possible auto-assign
	if refreshed, err := s.Store.GetConversationByPublicID(conv.PublicID.String()); err == nil {
		conv = refreshed
	}
	s.AgentHub.Publish(gin.H{"type": "conversation.updated", "payload": conv})
	s.AgentHub.Publish(gin.H{"type": "queue.updated", "payload": gin.H{"skill_group_id": conv.SkillGroupID}})
	OK(c, conv)
}

// GET /api/v1/im/conversations/:public_id/messages
func (s *Server) ListMessages(c *gin.Context) {
	publicID := c.Param("public_id")
	conv, err := s.Store.GetConversationByPublicID(publicID)
	if err != nil {
		Fail(c, http.StatusNotFound, "conversation not found")
		return
	}
	if !s.canAccessConversation(c, conv) {
		return
	}
	after, _ := strconv.ParseInt(c.DefaultQuery("after_seq", "0"), 10, 64)
	list, err := s.Store.ListMessages(conv.ID, after, 100)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	OK(c, gin.H{"messages": list, "conversation": conv})
}

type sendMsgReq struct {
	ClientMsgID string         `json:"client_msg_id"`
	MsgType     string         `json:"msg_type"`
	Content     map[string]any `json:"content"`
}

// POST /api/v1/im/conversations/:public_id/messages  (visitor or agent)
func (s *Server) SendMessage(c *gin.Context) {
	publicID := c.Param("public_id")
	conv, err := s.Store.GetConversationByPublicID(publicID)
	if err != nil {
		Fail(c, http.StatusNotFound, "conversation not found")
		return
	}
	var req sendMsgReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Content == nil {
		Fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	if req.MsgType == "" {
		req.MsgType = "text"
	}

	var senderType string
	var senderID uint64

	// try guest first
	if tok := bearer(c); tok != "" {
		if g, err := authjwt.ParseGuest(s.Secret, tok); err == nil {
			if g.VisitorID != conv.VisitorID {
				Fail(c, http.StatusForbidden, "not your conversation")
				return
			}
			senderType = "visitor"
			senderID = g.VisitorID
		}
	}
	if senderType == "" {
		agent, ok := s.requireAgent(c)
		if !ok {
			return
		}
		senderType = "agent"
		senderID = agent.UserID
		if conv.Status == "queued" || conv.AgentUserID == nil {
			_ = s.Store.AssignConversation(conv, agent.UserID)
			_ = s.Store.AppendSystemEvent(conv.ID, "assigned", map[string]any{
				"agent_user_id": agent.UserID,
				"auto":          false,
			})
		}
	}

	content := store.JSONText(req.Content)
	var clientID *string
	if req.ClientMsgID != "" {
		clientID = &req.ClientMsgID
	}
	msg := &model.Message{
		ConversationID: conv.ID,
		ClientMsgID:    clientID,
		SenderType:     senderType,
		SenderID:       &senderID,
		MsgType:        req.MsgType,
		Content:        content,
	}
	if err := s.Store.CreateMessage(msg); err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	// refresh conv preview
	conv, _ = s.Store.GetConversationByPublicID(publicID)
	payload := gin.H{
		"type": "message.new",
		"payload": gin.H{
			"message":                msg,
			"conversation_public_id": publicID,
		},
	}
	s.Hub.Publish(publicID, payload)
	s.AgentHub.Publish(gin.H{"type": "conversation.updated", "payload": conv})
	OK(c, msg)
}

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
	if _, ok := s.requireAgent(c); ok {
		return true
	}
	return false
}

// ---------- Agent (M3) ----------

// GET /api/v1/im/agent/me
func (s *Server) AgentMe(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	presence, _ := s.Store.GetPresence(agent.UserID)
	skills, _ := s.Store.ListAgentSkills(0, agent.UserID)
	load, _ := s.Store.CountAssignedForAgent(agent.UserID)
	groups, _ := s.Store.ListSkillGroups()
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
		}
		views = append(views, v)
	}
	OK(c, gin.H{
		"user_id":          agent.UserID,
		"username":         agent.Username,
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

// PUT /api/v1/im/agent/presence
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
	s.AgentHub.Publish(gin.H{"type": "presence.updated", "payload": p})
	// when going online, try assign queued items for this agent's skill groups
	if p.Status == "online" {
		s.tryAssignQueuedForAgent(agent.UserID)
	}
	OK(c, p)
}

func (s *Server) tryAssignQueuedForAgent(agentUserID uint64) {
	skills, err := s.Store.ListAgentSkills(0, agentUserID)
	if err != nil {
		return
	}
	for _, sk := range skills {
		if sk.Status != 1 {
			continue
		}
		list, err := s.Store.ListAgentConversations(0, "queue", sk.SkillGroupID, 20)
		if err != nil {
			continue
		}
		for i := range list {
			_ = s.Store.TryAutoAssign(&list[i])
			if refreshed, err := s.Store.GetConversationByPublicID(list[i].PublicID.String()); err == nil {
				s.AgentHub.Publish(gin.H{"type": "conversation.updated", "payload": refreshed})
			}
		}
	}
	s.AgentHub.Publish(gin.H{"type": "queue.updated", "payload": gin.H{}})
}

// GET /api/v1/im/agent/conversations?scope=all|mine|queue&skill_group_id=
func (s *Server) AgentListConversations(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	scope := c.DefaultQuery("scope", "all")
	sgID, _ := strconv.ParseUint(c.DefaultQuery("skill_group_id", "0"), 10, 64)
	list, err := s.Store.ListAgentConversations(agent.UserID, scope, sgID, 100)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	OK(c, gin.H{"list": list, "scope": scope})
}

// GET /api/v1/im/agent/queue — queued only + counts
func (s *Server) AgentQueue(c *gin.Context) {
	if _, ok := s.requireAgent(c); !ok {
		return
	}
	sgID, _ := strconv.ParseUint(c.DefaultQuery("skill_group_id", "0"), 10, 64)
	list, err := s.Store.ListAgentConversations(0, "queue", sgID, 100)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	online, _ := s.Store.ListPresence("online", "busy")
	OK(c, gin.H{
		"list":           list,
		"queue_size":     len(list),
		"online_agents":  online,
		"skill_group_id": sgID,
	})
}

// GET /api/v1/im/agent/online — list online/busy agents for transfer picker
func (s *Server) AgentOnlineList(c *gin.Context) {
	if _, ok := s.requireAgent(c); !ok {
		return
	}
	list, err := s.Store.ListPresence("online", "busy", "offline")
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	type row struct {
		model.AgentPresence
		AssignedCount int64 `json:"assigned_count"`
	}
	out := make([]row, 0, len(list))
	for _, p := range list {
		n, _ := s.Store.CountAssignedForAgent(p.AgentUserID)
		out = append(out, row{AgentPresence: p, AssignedCount: n})
	}
	OK(c, gin.H{"list": out})
}

// POST /api/v1/im/agent/conversations/:public_id/accept
func (s *Server) AgentAccept(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	conv, err := s.Store.GetConversationByPublicID(c.Param("public_id"))
	if err != nil {
		Fail(c, http.StatusNotFound, "conversation not found")
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
	s.AgentHub.Publish(gin.H{"type": "conversation.updated", "payload": conv})
	s.AgentHub.Publish(gin.H{"type": "queue.updated", "payload": gin.H{}})
	s.Hub.Publish(conv.PublicID.String(), gin.H{
		"type":    "conversation.updated",
		"payload": conv,
	})
	OK(c, conv)
}

type transferReq struct {
	TargetAgentUserID *uint64 `json:"target_agent_user_id"` // nil or 0 = re-queue
	SkillGroupID      *uint64 `json:"skill_group_id"`
	Note              string  `json:"note"`
}

// POST /api/v1/im/agent/conversations/:public_id/transfer
func (s *Server) AgentTransfer(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	conv, err := s.Store.GetConversationByPublicID(c.Param("public_id"))
	if err != nil {
		Fail(c, http.StatusNotFound, "conversation not found")
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
	s.AgentHub.Publish(gin.H{"type": "conversation.updated", "payload": conv})
	s.AgentHub.Publish(gin.H{"type": "queue.updated", "payload": gin.H{}})
	s.Hub.Publish(conv.PublicID.String(), gin.H{"type": "conversation.updated", "payload": conv})
	// broadcast transfer system messages to conversation subscribers via message poll or event
	msgs, _ := s.Store.ListMessages(conv.ID, 0, 1)
	_ = msgs
	OK(c, conv)
}

type closeReq struct {
	Reason string `json:"reason"`
}

// POST /api/v1/im/agent/conversations/:public_id/close
func (s *Server) AgentClose(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	conv, err := s.Store.GetConversationByPublicID(c.Param("public_id"))
	if err != nil {
		Fail(c, http.StatusNotFound, "conversation not found")
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
	s.AgentHub.Publish(gin.H{"type": "conversation.updated", "payload": conv})
	s.Hub.Publish(conv.PublicID.String(), gin.H{"type": "conversation.updated", "payload": conv})
	OK(c, conv)
}

// ---------- Admin sites ----------

// GET /api/v1/im/admin/sites
func (s *Server) AdminListSites(c *gin.Context) {
	if _, ok := s.requireAgent(c); !ok {
		return
	}
	list, err := s.Store.ListSites()
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	// attach embed snippet per site
	type row struct {
		model.Site
		Snippet string `json:"snippet"`
	}
	out := make([]row, 0, len(list))
	for _, site := range list {
		out = append(out, row{Site: site, Snippet: embedSnippet(c, site.AppKey)})
	}
	OK(c, gin.H{"list": out})
}

type updateSiteReq struct {
	Name                *string  `json:"name"`
	WelcomeText         *string  `json:"welcome_text"`
	AllowedOrigins      []string `json:"allowed_origins"`
	Status              *int16   `json:"status"`
	DefaultSkillGroupID *uint64  `json:"default_skill_group_id"`
}

// PUT /api/v1/im/admin/sites/:id
func (s *Server) AdminUpdateSite(c *gin.Context) {
	if _, ok := s.requireAgent(c); !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		Fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req updateSiteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	u := store.SiteUpdate{
		Name:                req.Name,
		WelcomeText:         req.WelcomeText,
		Status:              req.Status,
		DefaultSkillGroupID: req.DefaultSkillGroupID,
	}
	if req.AllowedOrigins != nil {
		raw := store.JSONText(req.AllowedOrigins)
		u.AllowedOrigins = &raw
	}
	site, err := s.Store.UpdateSite(id, u)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	OK(c, gin.H{"site": site, "snippet": embedSnippet(c, site.AppKey)})
}

// ---------- Admin skill groups (M3) ----------

// GET /api/v1/im/admin/skill-groups
func (s *Server) AdminListSkillGroups(c *gin.Context) {
	if _, ok := s.requireAgent(c); !ok {
		return
	}
	list, err := s.Store.ListSkillGroups()
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	// attach agent skill counts
	type row struct {
		model.SkillGroup
		AgentCount int `json:"agent_count"`
	}
	out := make([]row, 0, len(list))
	for _, g := range list {
		skills, _ := s.Store.ListAgentSkills(g.ID, 0)
		out = append(out, row{SkillGroup: g, AgentCount: len(skills)})
	}
	OK(c, gin.H{"list": out})
}

type skillGroupReq struct {
	Name     string `json:"name"`
	Code     string `json:"code"`
	Strategy string `json:"strategy"`
	Status   int16  `json:"status"`
}

// POST /api/v1/im/admin/skill-groups
func (s *Server) AdminCreateSkillGroup(c *gin.Context) {
	if _, ok := s.requireAgent(c); !ok {
		return
	}
	var req skillGroupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	g, err := s.Store.CreateSkillGroup(store.SkillGroupInput{
		Name: req.Name, Code: req.Code, Strategy: req.Strategy, Status: req.Status,
	})
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	OK(c, g)
}

// PUT /api/v1/im/admin/skill-groups/:id
func (s *Server) AdminUpdateSkillGroup(c *gin.Context) {
	if _, ok := s.requireAgent(c); !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		Fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req skillGroupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	g, err := s.Store.UpdateSkillGroup(id, store.SkillGroupInput{
		Name: req.Name, Code: req.Code, Strategy: req.Strategy, Status: req.Status,
	})
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	OK(c, g)
}

// GET /api/v1/im/admin/skill-groups/:id/agents
func (s *Server) AdminListSkillAgents(c *gin.Context) {
	if _, ok := s.requireAgent(c); !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		Fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	list, err := s.Store.ListAgentSkills(id, 0)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	type row struct {
		model.AgentSkill
		Presence *model.AgentPresence `json:"presence,omitempty"`
		Load     int64                `json:"assigned_count"`
	}
	out := make([]row, 0, len(list))
	for _, sk := range list {
		p, _ := s.Store.GetPresence(sk.AgentUserID)
		n, _ := s.Store.CountAssignedForAgent(sk.AgentUserID)
		out = append(out, row{AgentSkill: sk, Presence: p, Load: n})
	}
	OK(c, gin.H{"list": out})
}

type agentSkillReq struct {
	AgentUserID   uint64 `json:"agent_user_id"`
	SkillGroupID  uint64 `json:"skill_group_id"`
	MaxConcurrent int    `json:"max_concurrent"`
	Status        int16  `json:"status"`
}

// POST /api/v1/im/admin/agent-skills
func (s *Server) AdminUpsertAgentSkill(c *gin.Context) {
	if _, ok := s.requireAgent(c); !ok {
		return
	}
	var req agentSkillReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	// allow skill_group_id from path when nested
	if req.SkillGroupID == 0 {
		if pid := c.Param("id"); pid != "" {
			req.SkillGroupID, _ = strconv.ParseUint(pid, 10, 64)
		}
	}
	row, err := s.Store.UpsertAgentSkill(req.AgentUserID, req.SkillGroupID, req.MaxConcurrent, req.Status)
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	OK(c, row)
}

// DELETE /api/v1/im/admin/agent-skills/:id
func (s *Server) AdminDeleteAgentSkill(c *gin.Context) {
	if _, ok := s.requireAgent(c); !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		Fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.Store.DeleteAgentSkill(id); err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	OK(c, gin.H{"deleted": id})
}

// GET /api/v1/im/health/*
func (s *Server) HealthLive(c *gin.Context)  { c.JSON(http.StatusOK, gin.H{"status": "ok"}) }
func (s *Server) HealthReady(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ready"}) }
