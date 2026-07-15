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
	origin := c.GetHeader("Origin")
	if origin != "" && !store.OriginAllowed(site.AllowedOrigins, origin) {
		Fail(c, http.StatusForbidden, "origin denied")
		return
	}
	OK(c, gin.H{
		"app_key":      site.AppKey,
		"name":         site.Name,
		"welcome_text": site.WelcomeText,
	})
}

type visitorSessionReq struct {
	AppKey      string `json:"app_key"`
	GuestKey    string `json:"guest_key"`
	DisplayName string `json:"display_name"`
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
	origin := c.GetHeader("Origin")
	if origin != "" && !store.OriginAllowed(site.AllowedOrigins, origin) {
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
	Channel string         `json:"channel"`
	Context map[string]any `json:"context"`
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
	conv, err := s.Store.EnsureOpenConversation(guest.SiteID, guest.VisitorID, req.Channel, ctxJSON)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	s.AgentHub.Publish(gin.H{"type": "conversation.updated", "payload": conv})
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
			"message":               msg,
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

// GET /api/v1/im/agent/conversations
func (s *Server) AgentListConversations(c *gin.Context) {
	if _, ok := s.requireAgent(c); !ok {
		return
	}
	list, err := s.Store.ListAgentConversations(100)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	OK(c, gin.H{"list": list})
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
	if err := s.Store.AssignConversation(conv, agent.UserID); err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	s.AgentHub.Publish(gin.H{"type": "conversation.updated", "payload": conv})
	s.Hub.Publish(conv.PublicID.String(), gin.H{
		"type": "conversation.updated",
		"payload": conv,
	})
	OK(c, conv)
}

// POST /api/v1/im/agent/conversations/:public_id/close
func (s *Server) AgentClose(c *gin.Context) {
	if _, ok := s.requireAgent(c); !ok {
		return
	}
	conv, err := s.Store.GetConversationByPublicID(c.Param("public_id"))
	if err != nil {
		Fail(c, http.StatusNotFound, "conversation not found")
		return
	}
	if err := s.Store.CloseConversation(conv); err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	s.AgentHub.Publish(gin.H{"type": "conversation.updated", "payload": conv})
	s.Hub.Publish(conv.PublicID.String(), gin.H{"type": "conversation.updated", "payload": conv})
	OK(c, conv)
}

// GET /api/v1/im/health/*
func (s *Server) HealthLive(c *gin.Context)  { c.JSON(http.StatusOK, gin.H{"status": "ok"}) }
func (s *Server) HealthReady(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ready"}) }
