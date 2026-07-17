package api

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts all API routes; static assets (widget/visitor/uploads)
// stay in main.go because their paths depend on the process working directory.
func (s *Server) RegisterRoutes(r *gin.Engine) {
	// health (no auth)
	r.GET("/api/v1/health/live", s.HealthLive)
	r.GET("/api/v1/health/ready", s.HealthReady)
	r.GET("/api/v1/im/health/live", s.HealthLive)
	r.GET("/api/v1/im/health/ready", s.HealthReady)

	// visitor / widget (public embed surface → rate limited)
	r.GET("/api/v1/im/widget/config", s.WidgetConfig)
	r.POST("/api/v1/im/visitor/session", s.limitSession(), s.VisitorSession)
	r.POST("/api/v1/im/conversations", s.limitWrites(), s.CreateConversation)
	r.GET("/api/v1/im/conversations/:public_id/messages", s.ListMessages)
	r.POST("/api/v1/im/conversations/:public_id/messages", s.limitWrites(), s.SendMessage)
	r.POST("/api/v1/im/attachments", s.limitUploads(), s.UploadAttachment)
	r.POST("/api/v1/im/conversations/:public_id/transfer_human", s.limitWrites(), s.TransferHuman)
	r.POST("/api/v1/im/conversations/:public_id/read", s.limitWrites(), s.VisitorMarkRead)

	// agent (M1 + M3 + M4)
	r.GET("/api/v1/im/agent/me", s.AgentMe)
	r.PUT("/api/v1/im/agent/presence", s.AgentPresence)
	r.GET("/api/v1/im/agent/conversations", s.AgentListConversations)
	r.GET("/api/v1/im/agent/queue", s.AgentQueue)
	r.GET("/api/v1/im/agent/online", s.AgentOnlineList)
	r.POST("/api/v1/im/agent/conversations/:public_id/accept", s.AgentAccept)
	r.POST("/api/v1/im/agent/conversations/:public_id/transfer", s.AgentTransfer)
	r.POST("/api/v1/im/agent/conversations/:public_id/close", s.AgentClose)
	r.POST("/api/v1/im/agent/conversations/:public_id/summary", s.AgentSummary)
	r.POST("/api/v1/im/agent/conversations/:public_id/read", s.AgentMarkRead)

	// admin sites (M2 embed config)
	r.GET("/api/v1/im/admin/sites", s.AdminListSites)
	r.PUT("/api/v1/im/admin/sites/:id", s.AdminUpdateSite)

	// admin skill groups (M3)
	r.GET("/api/v1/im/admin/skill-groups", s.AdminListSkillGroups)
	r.POST("/api/v1/im/admin/skill-groups", s.AdminCreateSkillGroup)
	r.PUT("/api/v1/im/admin/skill-groups/:id", s.AdminUpdateSkillGroup)
	r.GET("/api/v1/im/admin/skill-groups/:id/agents", s.AdminListSkillAgents)
	r.POST("/api/v1/im/admin/skill-groups/:id/agents", s.AdminUpsertAgentSkill)
	r.POST("/api/v1/im/admin/agent-skills", s.AdminUpsertAgentSkill)
	r.DELETE("/api/v1/im/admin/agent-skills/:id", s.AdminDeleteAgentSkill)

	// websocket
	r.GET("/im/ws", s.WebSocket)
}
