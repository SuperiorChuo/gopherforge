package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/im/internal/store/im"
)

// AgentListKBMisses GET /api/v1/im/agent/kb-misses（知识库未命中列表）
func (s *Server) AgentListKBMisses(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	list, total, err := s.Store.ListKBMisses(agent.TenantID, store.KBMissFilter{
		Status:  c.Query("status"),
		Keyword: c.Query("keyword"),
		Page:    page,
		Size:    size,
	})
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	openN, _ := s.Store.CountOpenKBMisses(agent.TenantID)
	OK(c, gin.H{"list": list, "total": total, "open_count": openN})
}

// AgentKBStats GET /api/v1/im/agent/kb-stats?days=7（知识库统计）
func (s *Server) AgentKBStats(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "7"))
	sum, err := s.Store.KBStats(agent.TenantID, days)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	vectorOn := s.KBVector != nil && s.KBVector.Enabled()
	OK(c, gin.H{"stats": sum, "vector_enabled": vectorOn})
}

// AgentUpdateKBMiss PUT /api/v1/im/agent/kb-misses/:id（更新未命中记录）
func (s *Server) AgentUpdateKBMiss(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		Fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		Status *string `json:"status"`
		Note   *string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	m, err := s.Store.UpdateKBMiss(id, agent.TenantID, req.Status, req.Note)
	if err != nil {
		if errorsIsNotFound(err) {
			Fail(c, http.StatusNotFound, "not found")
			return
		}
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	OK(c, m)
}
