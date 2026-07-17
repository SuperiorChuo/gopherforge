package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/im/internal/authjwt"
	"github.com/go-admin-kit/services/im/internal/bot"
	"github.com/go-admin-kit/services/im/internal/hub"
	"github.com/go-admin-kit/services/im/internal/model"
	"github.com/go-admin-kit/services/im/internal/store"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Server struct {
	Store    *store.Store
	Hub      *hub.Hub
	AgentHub *hub.AgentHub
	Secret   string
	// Bot is optional AI / stub client (M4).
	Bot             bot.Client
	BotSystemPrompt string
	AIEnabled       bool
	// UploadDir is local storage for IM attachments (M2.1).
	UploadDir string
	// Limits throttles public embed endpoints; nil disables (unit tests).
	Limits *Limits
}

func OK(c *gin.Context, data any) {
	// 与脚手架前端约定：业务成功 code == 200
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": data})
}

func errorsIsNotFound(err error) bool {
	return err != nil && errors.Is(err, gorm.ErrRecordNotFound)
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

// resolveAgentTenantID prefers X-Auth-Tenant-ID (gateway), then JWT claim, else default 1.
func resolveAgentTenantID(c *gin.Context, jwtTenant uint64) uint64 {
	if h := c.GetHeader("X-Auth-Tenant-ID"); h != "" {
		if n, err := strconv.ParseUint(h, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return authjwt.NormalizeTenantID(jwtTenant)
}

func (s *Server) requireAgent(c *gin.Context) (*authjwt.AgentClaims, bool) {
	// Prefer gateway header if present
	if uid := c.GetHeader("X-Auth-User-ID"); uid != "" {
		id, err := strconv.ParseUint(uid, 10, 64)
		if err == nil && id > 0 {
			return &authjwt.AgentClaims{
				UserID:   id,
				Username: c.GetHeader("X-Auth-Username"),
				TenantID: resolveAgentTenantID(c, 0),
			}, true
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
	claims.TenantID = resolveAgentTenantID(c, claims.TenantID)
	return claims, true
}

// sameHostOrigin reports whether origin points at the host serving this
// request (demo page and self-hosted embeds); those never need whitelisting.
func sameHostOrigin(c *gin.Context, origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, c.Request.Host)
}

func originDenied(c *gin.Context, allowedJSON, origin string) bool {
	if origin == "" || origin == "null" {
		return false
	}
	if sameHostOrigin(c, origin) {
		return false
	}
	return !store.OriginAllowed(allowedJSON, origin)
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
	if originDenied(c, site.AllowedOrigins, origin) {
		Fail(c, http.StatusForbidden, "origin denied")
		return
	}
	OK(c, gin.H{
		"app_key":      site.AppKey,
		"name":         site.Name,
		"welcome_text": site.WelcomeText,
		"bot_enabled":  site.BotEnabled,
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
	if originDenied(c, site.AllowedOrigins, origin) {
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

	site, siteErr := s.Store.GetSite(guest.SiteID)
	var skillID *uint64
	botOn := false
	tenantID := uint64(1)
	if siteErr == nil {
		tenantID = authjwt.NormalizeTenantID(site.TenantID)
		botOn = site.BotEnabled && s.AIEnabled
		if req.SkillGroupID != nil && *req.SkillGroupID > 0 {
			// only accept skill groups in the site's tenant
			if sg, err := s.Store.GetSkillGroupForTenant(*req.SkillGroupID, tenantID); err == nil {
				skillID = &sg.ID
			}
		} else if req.SkillGroupCode != "" {
			if sg, err := s.Store.GetSkillGroupByCode(tenantID, req.SkillGroupCode); err == nil {
				skillID = &sg.ID
			}
		} else if site.DefaultSkillGroupID != nil {
			skillID = site.DefaultSkillGroupID
		}
	}

	// conversation.tenant_id inherits from site.tenant_id
	conv, err := s.Store.EnsureOpenConversation(tenantID, guest.SiteID, guest.VisitorID, req.Channel, ctxJSON, skillID, botOn)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	// refresh after possible auto-assign / system event
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
		if conv.TenantID != 0 && conv.TenantID != agent.TenantID {
			Fail(c, http.StatusForbidden, "conversation not in your tenant")
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
	// 幂等重放：直接返回首次落库的消息，不重播事件、不再触发 bot
	if msg.Replayed {
		OK(c, msg)
		return
	}
	// 发消息隐含已读之前的所有消息
	if _, err := s.Store.MarkRead(conv, senderType, msg.Seq); err == nil {
		s.publishRead(conv, senderType, msg.Seq)
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

	// M4: bot reply / transfer intent after visitor message
	if senderType == "visitor" && conv != nil {
		text := ""
		if t, ok := req.Content["text"].(string); ok {
			text = t
		}
		s.afterVisitorMessage(conv, text)
	}
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
	if agent, ok := s.requireAgent(c); ok {
		if conv.TenantID != 0 && conv.TenantID != agent.TenantID {
			Fail(c, http.StatusForbidden, "conversation not in your tenant")
			return false
		}
		return true
	}
	return false
}

// requireAgentConversation loads a conversation and ensures it belongs to the agent tenant.
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

// ---------- Read cursors ----------

type markReadReq struct {
	Seq int64 `json:"seq"` // <=0 → up to latest
}

// publishRead notifies both sides so "已读" markers update live.
func (s *Server) publishRead(conv *model.Conversation, reader string, seq int64) {
	payload := gin.H{
		"conversation_public_id": conv.PublicID.String(),
		"reader":                 reader,
		"seq":                    seq,
	}
	s.Hub.Publish(conv.PublicID.String(), gin.H{"type": "conversation.read", "payload": payload})
	s.AgentHub.Publish(gin.H{"type": "conversation.read", "payload": payload})
}

// POST /api/v1/im/conversations/:public_id/read  (visitor)
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

// POST /api/v1/im/agent/conversations/:public_id/read
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
			// skill binding to other-tenant group: omit from agent view
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
		s.tryAssignQueuedForAgent(agent.TenantID, agent.UserID)
	}
	OK(c, p)
}

func (s *Server) tryAssignQueuedForAgent(tenantID, agentUserID uint64) {
	skills, err := s.Store.ListAgentSkills(0, agentUserID)
	if err != nil {
		return
	}
	for _, sk := range skills {
		if sk.Status != 1 {
			continue
		}
		// only process skill groups in the agent's tenant
		if sg, err := s.Store.GetSkillGroupForTenant(sk.SkillGroupID, tenantID); err != nil || sg == nil {
			continue
		}
		list, err := s.Store.ListAgentConversations(tenantID, 0, "queue", sk.SkillGroupID, 20)
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

// GET /api/v1/im/agent/queue — queued only + counts
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
	online, _ := s.Store.ListPresence("online", "busy")
	OK(c, gin.H{
		"list":           list,
		"queue_size":     len(list),
		"online_agents":  online,
		"skill_group_id": sgID,
		"tenant_id":      agent.TenantID,
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
	s.AgentHub.Publish(gin.H{"type": "conversation.updated", "payload": conv})
	s.Hub.Publish(conv.PublicID.String(), gin.H{"type": "conversation.updated", "payload": conv})
	OK(c, conv)
}

// ---------- Admin sites ----------

// GET /api/v1/im/admin/sites
func (s *Server) AdminListSites(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	list, err := s.Store.ListSites(agent.TenantID)
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
	OK(c, gin.H{"list": out, "tenant_id": agent.TenantID})
}

type updateSiteReq struct {
	Name                *string  `json:"name"`
	WelcomeText         *string  `json:"welcome_text"`
	AllowedOrigins      []string `json:"allowed_origins"`
	Status              *int16   `json:"status"`
	DefaultSkillGroupID *uint64  `json:"default_skill_group_id"`
	BotEnabled          *bool    `json:"bot_enabled"`
	BotSystemPrompt     *string  `json:"bot_system_prompt"`
}

// PUT /api/v1/im/admin/sites/:id
func (s *Server) AdminUpdateSite(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
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
		BotEnabled:          req.BotEnabled,
		BotSystemPrompt:     req.BotSystemPrompt,
	}
	if req.AllowedOrigins != nil {
		raw := store.JSONText(req.AllowedOrigins)
		u.AllowedOrigins = &raw
	}
	site, err := s.Store.UpdateSite(id, agent.TenantID, u)
	if err != nil {
		if errorsIsNotFound(err) {
			Fail(c, http.StatusNotFound, "site not found")
			return
		}
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	OK(c, gin.H{"site": site, "snippet": embedSnippet(c, site.AppKey)})
}

// ---------- M4 bot / transfer human / summary ----------

type transferHumanReq struct {
	Reason string `json:"reason"`
}

// POST /api/v1/im/conversations/:public_id/transfer_human
func (s *Server) TransferHuman(c *gin.Context) {
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
	var req transferHumanReq
	_ = c.ShouldBindJSON(&req)
	reason := req.Reason
	if reason == "" {
		reason = "visitor"
	}
	if err := s.Store.TransferToHuman(conv, reason); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if refreshed, err := s.Store.GetConversationByPublicID(conv.PublicID.String()); err == nil {
		conv = refreshed
	}
	// notify visitor + agents
	s.Hub.Publish(conv.PublicID.String(), gin.H{"type": "conversation.updated", "payload": conv})
	s.AgentHub.Publish(gin.H{"type": "conversation.updated", "payload": conv})
	s.AgentHub.Publish(gin.H{"type": "queue.updated", "payload": gin.H{}})
	// push a short bot/system note to visitor
	note := &model.Message{
		ConversationID: conv.ID,
		SenderType:     "bot",
		MsgType:        "text",
		Content:        store.JSONText(map[string]any{"text": "已为您转接人工客服，请稍候…"}),
	}
	if err := s.Store.CreateMessage(note); err == nil {
		s.Hub.Publish(conv.PublicID.String(), gin.H{
			"type": "message.new",
			"payload": gin.H{
				"message":                note,
				"conversation_public_id": conv.PublicID.String(),
			},
		})
	}
	OK(c, conv)
}

// POST /api/v1/im/agent/conversations/:public_id/summary
func (s *Server) AgentSummary(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	conv, ok := s.requireAgentConversation(c, agent)
	if !ok {
		return
	}
	summary, err := s.generateSummary(c.Request.Context(), conv)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.Store.SaveSummary(conv, summary); err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.Store.AppendSystemEvent(conv.ID, "summary", map[string]any{"summary": summary})
	if refreshed, err := s.Store.GetConversationByPublicID(conv.PublicID.String()); err == nil {
		conv = refreshed
	}
	s.AgentHub.Publish(gin.H{"type": "conversation.updated", "payload": conv})
	OK(c, gin.H{"summary": summary, "conversation": conv})
}

// afterVisitorMessage runs transfer-intent check and bot reply (async).
func (s *Server) afterVisitorMessage(conv *model.Conversation, text string) {
	if conv == nil || conv.Status != "bot_serving" {
		return
	}
	// 图片/文件等非文本消息不触发机器人（模型只支持文本）
	if strings.TrimSpace(text) == "" {
		return
	}
	// copy public id for goroutine
	publicID := conv.PublicID.String()
	convID := conv.ID
	siteID := conv.SiteID
	go func() {
		// re-fetch latest
		c, err := s.Store.GetConversationByPublicID(publicID)
		if err != nil || c.Status != "bot_serving" {
			return
		}
		if bot.WantsHuman(text) {
			if err := s.Store.TransferToHuman(c, "keyword"); err != nil {
				return
			}
			if refreshed, err := s.Store.GetConversationByPublicID(publicID); err == nil {
				c = refreshed
			}
			s.Hub.Publish(publicID, gin.H{"type": "conversation.updated", "payload": c})
			s.AgentHub.Publish(gin.H{"type": "conversation.updated", "payload": c})
			s.AgentHub.Publish(gin.H{"type": "queue.updated", "payload": gin.H{}})
			note := &model.Message{
				ConversationID: convID,
				SenderType:     "bot",
				MsgType:        "text",
				Content:        store.JSONText(map[string]any{"text": "好的，正在为您转接人工客服，请稍候…"}),
			}
			if err := s.Store.CreateMessage(note); err == nil {
				s.Hub.Publish(publicID, gin.H{
					"type":    "message.new",
					"payload": gin.H{"message": note, "conversation_public_id": publicID},
				})
			}
			return
		}
		if s.Bot == nil {
			return
		}
		reply, err := s.botReply(context.Background(), siteID, convID, publicID)
		if err != nil {
			log.Printf("im bot reply: %v", err)
			// degrade: offer human
			reply = "抱歉，智能助手暂时无法回答。您可以回复「转人工」接入坐席。"
		}
		// skip if visitor transferred while model was running
		if latest, err := s.Store.GetConversationByPublicID(publicID); err != nil || latest.Status != "bot_serving" {
			return
		}
		msg := &model.Message{
			ConversationID: convID,
			SenderType:     "bot",
			MsgType:        "text",
			Content:        store.JSONText(map[string]any{"text": reply}),
		}
		if err := s.Store.CreateMessage(msg); err != nil {
			log.Printf("im bot save: %v", err)
			return
		}
		s.Hub.Publish(publicID, gin.H{
			"type":    "message.new",
			"payload": gin.H{"message": msg, "conversation_public_id": publicID},
		})
		if refreshed, err := s.Store.GetConversationByPublicID(publicID); err == nil {
			s.AgentHub.Publish(gin.H{"type": "conversation.updated", "payload": refreshed})
		}
	}()
}

func (s *Server) botReply(ctx context.Context, siteID, convID uint64, publicID string) (string, error) {
	system := s.BotSystemPrompt
	if site, err := s.Store.GetSite(siteID); err == nil && strings.TrimSpace(site.BotSystemPrompt) != "" {
		system = site.BotSystemPrompt
	}
	msgs, err := s.Store.ListMessages(convID, 0, 30)
	if err != nil {
		return "", err
	}
	history := make([]bot.Message, 0, len(msgs))
	for _, m := range msgs {
		if m.MsgType == "event" {
			continue
		}
		text := bot.ExtractText(m.Content)
		if text == "" {
			continue
		}
		role := m.SenderType
		switch role {
		case "visitor":
			role = "user"
		case "bot", "agent":
			role = "assistant"
		default:
			continue
		}
		history = append(history, bot.Message{Role: role, Content: text})
	}
	ctx, cancel := context.WithTimeout(ctx, 50*time.Second)
	defer cancel()
	return s.Bot.Complete(ctx, system, history)
}

func (s *Server) generateSummary(ctx context.Context, conv *model.Conversation) (string, error) {
	if s.Bot == nil {
		return "", nil
	}
	msgs, err := s.Store.ListMessages(conv.ID, 0, 50)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("请用中文写一段简短客服会话小结（3～6 句），含：访客诉求、处理过程、结果/待办。\n\n对话：\n")
	for _, m := range msgs {
		if m.MsgType == "event" {
			continue
		}
		text := bot.ExtractText(m.Content)
		if text == "" {
			continue
		}
		b.WriteString(m.SenderType)
		b.WriteString(": ")
		b.WriteString(text)
		b.WriteString("\n")
	}
	history := []bot.Message{{Role: "user", Content: b.String()}}
	ctx, cancel := context.WithTimeout(ctx, 50*time.Second)
	defer cancel()
	return s.Bot.Complete(ctx, "你是客服质检助手，只输出小结正文。", history)
}

// ---------- Admin skill groups (M3) ----------

// GET /api/v1/im/admin/skill-groups
func (s *Server) AdminListSkillGroups(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	list, err := s.Store.ListSkillGroups(agent.TenantID)
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
	OK(c, gin.H{"list": out, "tenant_id": agent.TenantID})
}

type skillGroupReq struct {
	Name     string `json:"name"`
	Code     string `json:"code"`
	Strategy string `json:"strategy"`
	Status   int16  `json:"status"`
}

// POST /api/v1/im/admin/skill-groups
func (s *Server) AdminCreateSkillGroup(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	var req skillGroupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	g, err := s.Store.CreateSkillGroup(store.SkillGroupInput{
		TenantID: agent.TenantID,
		Name:     req.Name,
		Code:     req.Code,
		Strategy: req.Strategy,
		Status:   req.Status,
	})
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	OK(c, g)
}

// PUT /api/v1/im/admin/skill-groups/:id
func (s *Server) AdminUpdateSkillGroup(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
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
	g, err := s.Store.UpdateSkillGroup(id, agent.TenantID, store.SkillGroupInput{
		Name: req.Name, Code: req.Code, Strategy: req.Strategy, Status: req.Status,
	})
	if err != nil {
		if errorsIsNotFound(err) {
			Fail(c, http.StatusNotFound, "skill group not found")
			return
		}
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	OK(c, g)
}

// GET /api/v1/im/admin/skill-groups/:id/agents
func (s *Server) AdminListSkillAgents(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		Fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	if _, err := s.Store.GetSkillGroupForTenant(id, agent.TenantID); err != nil {
		Fail(c, http.StatusNotFound, "skill group not found")
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
	agent, ok := s.requireAgent(c)
	if !ok {
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
	if _, err := s.Store.GetSkillGroupForTenant(req.SkillGroupID, agent.TenantID); err != nil {
		Fail(c, http.StatusNotFound, "skill group not found")
		return
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
