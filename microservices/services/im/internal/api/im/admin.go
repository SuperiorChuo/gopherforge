package api

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/im/internal/model"
	"github.com/go-admin-kit/services/im/internal/store/im"
	"gorm.io/gorm"
	"net/http"
	"strconv"
)

// GET /api/v1/im/admin/sites（站点列表）
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
	// 为每个站点附带嵌入代码片段
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

type createSiteReq struct {
	Name                string   `json:"name"`
	WelcomeText         string   `json:"welcome_text"`
	AllowedOrigins      []string `json:"allowed_origins"`
	DefaultSkillGroupID *uint64  `json:"default_skill_group_id"`
	BotEnabled          *bool    `json:"bot_enabled"`
	BotKBEnabled        *bool    `json:"bot_kb_enabled"`
	BotSystemPrompt     string   `json:"bot_system_prompt"`
}

// POST /api/v1/im/admin/sites（创建站点）
func (s *Server) AdminCreateSite(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	var req createSiteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	origins := ""
	if req.AllowedOrigins != nil {
		origins = store.JSONText(req.AllowedOrigins)
	}
	site, err := s.Store.CreateSite(c.Request.Context(), agent.TenantID, store.SiteCreate{
		Name:                req.Name,
		WelcomeText:         req.WelcomeText,
		AllowedOrigins:      origins,
		DefaultSkillGroupID: req.DefaultSkillGroupID,
		BotEnabled:          req.BotEnabled,
		BotKBEnabled:        req.BotKBEnabled,
		BotSystemPrompt:     req.BotSystemPrompt,
	})
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	OK(c, gin.H{"site": site, "snippet": embedSnippet(c, site.AppKey)})
}

type updateSiteReq struct {
	Name                *string  `json:"name"`
	WelcomeText         *string  `json:"welcome_text"`
	AllowedOrigins      []string `json:"allowed_origins"`
	Status              *int16   `json:"status"`
	DefaultSkillGroupID *uint64  `json:"default_skill_group_id"`
	BotEnabled          *bool    `json:"bot_enabled"`
	BotKBEnabled        *bool    `json:"bot_kb_enabled"`
	BotSystemPrompt     *string  `json:"bot_system_prompt"`
	BusinessHours       *string  `json:"business_hours"`
}

// PUT /api/v1/im/admin/sites/:id（更新站点）
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
		BotKBEnabled:        req.BotKBEnabled,
		BotSystemPrompt:     req.BotSystemPrompt,
		BusinessHours:       req.BusinessHours,
	}
	if req.AllowedOrigins != nil {
		raw := store.JSONText(req.AllowedOrigins)
		u.AllowedOrigins = &raw
	}
	site, err := s.Store.UpdateSite(c.Request.Context(), id, agent.TenantID, u)
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

// GET /api/v1/im/admin/skill-groups（技能组列表）
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
	// 附带坐席技能数量
	type row struct {
		model.SkillGroup
		AgentCount int `json:"agent_count"`
	}
	// 一条 GROUP BY 换掉「每个技能组一次 ListAgentSkills」
	groupIDs := make([]uint64, 0, len(list))
	for _, g := range list {
		groupIDs = append(groupIDs, g.ID)
	}
	counts, _ := s.Store.CountAgentSkillsByGroup(groupIDs)
	out := make([]row, 0, len(list))
	for _, g := range list {
		out = append(out, row{SkillGroup: g, AgentCount: counts[g.ID]})
	}
	OK(c, gin.H{"list": out, "tenant_id": agent.TenantID})
}

type skillGroupReq struct {
	Name     string `json:"name"`
	Code     string `json:"code"`
	Strategy string `json:"strategy"`
	Status   int16  `json:"status"`
}

// POST /api/v1/im/admin/skill-groups（创建技能组）
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
	g, err := s.Store.CreateSkillGroup(c.Request.Context(), store.SkillGroupInput{
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

// PUT /api/v1/im/admin/skill-groups/:id（更新技能组）
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
	g, err := s.Store.UpdateSkillGroup(c.Request.Context(), id, agent.TenantID, store.SkillGroupInput{
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

// GET /api/v1/im/admin/skill-groups/:id/agents（技能组坐席列表）
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
	// 两条批量查询换掉「每个坐席 GetPresence + CountAssignedForAgent」
	agentIDs := make([]uint64, 0, len(list))
	for _, sk := range list {
		agentIDs = append(agentIDs, sk.AgentUserID)
	}
	presences, _ := s.Store.GetPresenceMap(agentIDs)
	loads, _ := s.Store.CountAssignedForAgents(agentIDs)
	out := make([]row, 0, len(list))
	for _, sk := range list {
		out = append(out, row{AgentSkill: sk, Presence: presences[sk.AgentUserID], Load: loads[sk.AgentUserID]})
	}
	OK(c, gin.H{"list": out})
}

type agentSkillReq struct {
	AgentUserID   uint64 `json:"agent_user_id"`
	SkillGroupID  uint64 `json:"skill_group_id"`
	MaxConcurrent int    `json:"max_concurrent"`
	Status        int16  `json:"status"`
}

// POST /api/v1/im/admin/agent-skills（新增/更新坐席技能）
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
	// 嵌套路径时允许从路径取 skill_group_id
	if req.SkillGroupID == 0 {
		if pid := c.Param("id"); pid != "" {
			req.SkillGroupID, _ = strconv.ParseUint(pid, 10, 64)
		}
	}
	if _, err := s.Store.GetSkillGroupForTenant(req.SkillGroupID, agent.TenantID); err != nil {
		Fail(c, http.StatusNotFound, "skill group not found")
		return
	}
	row, err := s.Store.UpsertAgentSkill(c.Request.Context(), req.AgentUserID, req.SkillGroupID, req.MaxConcurrent, req.Status)
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	OK(c, row)
}

// DELETE /api/v1/im/admin/agent-skills/:id（删除坐席技能）
func (s *Server) AdminDeleteAgentSkill(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		Fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.Store.DeleteAgentSkill(c.Request.Context(), id, agent.TenantID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 不属于当前租户或不存在：与邻近接口一致返回 404
			Fail(c, http.StatusNotFound, "agent skill not found")
			return
		}
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	OK(c, gin.H{"deleted": id})
}
