package api

// M6 处理器：统计、访客禁言、消息撤回、敏感词打码。

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/im/internal/authjwt"
	"github.com/go-admin-kit/services/im/internal/model"
	"github.com/go-admin-kit/services/im/internal/store/im"
)

// ---------- 统计 ----------

// AgentStats 处理 GET /api/v1/im/agent/stats?days=7。
func (s *Server) AgentStats(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "7"))
	if days <= 0 || days > 365 {
		days = 7
	}
	since := time.Now().AddDate(0, 0, -days)
	stats, err := s.Store.Stats(agent.TenantID, since)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	OK(c, gin.H{"stats": stats, "days": days})
}

// AgentLowRatings 处理 GET /api/v1/im/agent/low-ratings
// 查询参数：max_score（默认 3）、days（默认 30）、agent_user_id、page、page_size。
func (s *Server) AgentLowRatings(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	maxScore, _ := strconv.Atoi(c.DefaultQuery("max_score", "3"))
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	ps, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	agentUID, _ := strconv.ParseUint(c.Query("agent_user_id"), 10, 64)
	list, total, err := s.Store.ListLowRatedConversations(agent.TenantID, store.LowRatedFilter{
		MaxScore:    maxScore,
		Days:        days,
		AgentUserID: agentUID,
		Page:        page,
		PageSize:    ps,
	})
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	// attach display names for agents：只批量取本页出现的坐席，
	// 原来是 ListPresence() 全表无过滤读（跨租户 + 无上限）。
	names := map[uint64]string{}
	agentIDs := make([]uint64, 0, len(list))
	for i := range list {
		if list[i].AgentUserID != nil {
			agentIDs = append(agentIDs, *list[i].AgentUserID)
		}
	}
	if pres, err := s.Store.GetPresenceMap(agentIDs); err == nil {
		for id, p := range pres {
			names[id] = p.DisplayName
		}
	}
	type row struct {
		model.Conversation
		AgentDisplayName string `json:"agent_display_name,omitempty"`
	}
	out := make([]row, 0, len(list))
	for i := range list {
		r := row{Conversation: list[i]}
		if list[i].AgentUserID != nil {
			r.AgentDisplayName = names[*list[i].AgentUserID]
		}
		out = append(out, r)
	}
	OK(c, gin.H{
		"list":      out,
		"total":     total,
		"max_score": maxScore,
		"days":      days,
	})
}

// ---------- 访客禁言 ----------

type banReq struct {
	Banned bool `json:"banned"`
}

// AgentSetVisitorBan 处理 PUT /api/v1/im/agent/conversations/:public_id/visitor-ban。
func (s *Server) AgentSetVisitorBan(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	conv, ok := s.requireAgentConversation(c, agent)
	if !ok {
		return
	}
	var req banReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.Store.SetVisitorBanned(conv.VisitorID, req.Banned); err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.Store.AppendSystemEvent(conv.ID, "visitor_ban", map[string]any{
		"banned": req.Banned, "by_agent_user_id": agent.UserID,
	})
	OK(c, gin.H{"banned": req.Banned})
}

// visitorBanned 读取禁言标记；失败时放行（消息收发不受影响）。
func (s *Server) visitorBanned(visitorID uint64) bool {
	v, err := s.Store.GetVisitor(visitorID)
	return err == nil && v.Banned
}

// ---------- 消息撤回 ----------

// RecallMessage 处理 POST /api/v1/im/conversations/:public_id/messages/:id/recall，
// 双方通用：先解析访客 bearer，失败则回退坐席（顺序与 SendMessage 相同）。
func (s *Server) RecallMessage(c *gin.Context) {
	conv, err := s.Store.GetConversationByPublicID(c.Param("public_id"))
	if err != nil {
		Fail(c, http.StatusNotFound, "conversation not found")
		return
	}
	msgID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || msgID == 0 {
		Fail(c, http.StatusBadRequest, "invalid message id")
		return
	}
	var senderType string
	var senderID uint64
	if tok := bearer(c); tok != "" {
		if g, gerr := authjwt.ParseGuest(s.Secret, tok); gerr == nil {
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
	}
	m, err := s.Store.RecallMessage(conv.ID, msgID, senderType, senderID)
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	payload := gin.H{
		"type": "message.recalled",
		"payload": gin.H{
			"conversation_public_id": conv.PublicID.String(),
			"message":                m,
		},
	}
	s.Hub.Publish(conv.PublicID.String(), payload)
	s.AgentHub.Publish(conv.TenantID, payload)
	OK(c, m)
}

// ---------- 敏感词 ----------

// maskSensitive 将已配置的屏蔽词子串替换为等 rune 长度的 *。
// 不区分大小写；列表为空时原样返回输入。
func (s *Server) maskSensitive(text string) string {
	if text == "" || len(s.SensitiveWords) == 0 {
		return text
	}
	lower := strings.ToLower(text)
	for _, w := range s.SensitiveWords {
		w = strings.ToLower(strings.TrimSpace(w))
		if w == "" {
			continue
		}
		for {
			idx := strings.Index(lower, w)
			if idx < 0 {
				break
			}
			mask := strings.Repeat("*", len([]rune(text[idx:idx+len(w)])))
			text = text[:idx] + mask + text[idx+len(w):]
			lower = lower[:idx] + mask + lower[idx+len(w):]
		}
	}
	return text
}
