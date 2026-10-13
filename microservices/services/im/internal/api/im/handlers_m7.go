package api

// M7 处理器：访客跨会话历史、消息搜索。

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// AgentVisitorHistory 处理 GET /api/v1/im/agent/conversations/:public_id/visitor-history。
func (s *Server) AgentVisitorHistory(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	conv, ok := s.requireAgentConversation(c, agent)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	list, err := s.Store.ListVisitorConversations(conv.VisitorID, limit)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	OK(c, gin.H{"list": list, "visitor_id": conv.VisitorID})
}

// AgentVisitorConversations 处理 GET /api/v1/im/agent/visitors/:id/conversations ——
// 按访客 ID 列会话（CRM 侧 IM 对话历史入口，P4）；经站点租户校验。
func (s *Server) AgentVisitorConversations(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	visitorID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || visitorID == 0 {
		Fail(c, http.StatusBadRequest, "invalid visitor id")
		return
	}
	v, err := s.Store.GetVisitor(visitorID)
	if err != nil {
		Fail(c, http.StatusNotFound, "visitor not found")
		return
	}
	// 站点必须能查到且属于当前租户；查不到时不放行（fail closed）
	site, err := s.Store.GetSite(v.SiteID)
	if err != nil {
		Fail(c, http.StatusNotFound, "visitor not found")
		return
	}
	if t := site.TenantID; t != 0 && t != agent.TenantID {
		Fail(c, http.StatusForbidden, "visitor not in your tenant")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	list, err := s.Store.ListVisitorConversations(visitorID, limit)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	OK(c, gin.H{"list": list, "visitor": v})
}

// AgentSearchMessages 处理 GET /api/v1/im/agent/messages/search?keyword=。
func (s *Server) AgentSearchMessages(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "30"))
	hits, err := s.Store.SearchMessages(agent.TenantID, c.Query("keyword"), limit)
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	OK(c, gin.H{"list": hits, "keyword": c.Query("keyword")})
}
