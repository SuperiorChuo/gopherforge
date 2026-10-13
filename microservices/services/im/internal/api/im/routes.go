package api

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/gatewayauth"
)

// RegisterRoutes 挂载所有 API 路由；静态资源（widget/visitor/uploads）
// 留在 main.go，因为其路径依赖进程的工作目录。
func (s *Server) RegisterRoutes(r *gin.Engine) {
	// 健康检查（无需认证）
	r.GET("/api/v1/health/live", s.HealthLive)
	r.GET("/api/v1/health/ready", s.HealthReady)
	r.GET("/api/v1/im/health/live", s.HealthLive)
	r.GET("/api/v1/im/health/ready", s.HealthReady)

	// 访客 / 组件（公共嵌入面 → 限流）
	r.GET("/api/v1/im/widget/config", s.WidgetConfig)
	r.POST("/api/v1/im/visitor/session", s.limitSession(), s.VisitorSession)
	r.PUT("/api/v1/im/visitor/profile", s.limitWrites(), s.VisitorUpdateProfile)
	r.POST("/api/v1/im/conversations", s.limitWrites(), s.CreateConversation)
	r.GET("/api/v1/im/conversations/:public_id/messages", s.ListMessages)
	r.POST("/api/v1/im/conversations/:public_id/messages", s.limitWrites(), s.SendMessage)
	r.POST("/api/v1/im/attachments", s.limitUploads(), s.UploadAttachment)
	r.POST("/api/v1/im/conversations/:public_id/transfer_human", s.limitWrites(), s.TransferHuman)
	r.POST("/api/v1/im/conversations/:public_id/read", s.limitWrites(), s.VisitorMarkRead)
	r.POST("/api/v1/im/conversations/:public_id/rating", s.limitWrites(), s.VisitorRateConversation)
	r.POST("/api/v1/im/conversations/:public_id/messages/:id/recall", s.limitWrites(), s.RecallMessage)

	// 坐席（M1 + M3 + M4）
	agent := r.Group("/api/v1/im/agent", gatewayauth.RequireAnyPermission("im:agent:access", "im:admin:manage"))
	agent.GET("/me", s.AgentMe)
	agent.PUT("/presence", s.AgentPresence)
	agent.GET("/conversations", s.AgentListConversations)
	agent.GET("/queue", s.AgentQueue)
	agent.GET("/online", s.AgentOnlineList)
	agent.POST("/conversations/:public_id/accept", s.AgentAccept)
	agent.POST("/conversations/:public_id/transfer", s.AgentTransfer)
	agent.POST("/conversations/:public_id/close", s.AgentClose)
	agent.POST("/conversations/:public_id/summary", s.AgentSummary)
	agent.POST("/conversations/:public_id/read", s.AgentMarkRead)

	// 坐席访客名片 + 快捷回复（M5）
	agent.GET("/conversations/:public_id/visitor", s.AgentGetVisitor)
	agent.PUT("/conversations/:public_id/visitor-note", s.AgentSetVisitorNote)
	agent.PUT("/conversations/:public_id/visitor-contact", s.AgentSetVisitorContact)

	// 坐席统计 + 访客禁言（M6）+ 低分质检列表
	agent.GET("/stats", s.AgentStats)
	agent.GET("/low-ratings", s.AgentLowRatings)
	agent.PUT("/conversations/:public_id/visitor-ban", s.AgentSetVisitorBan)

	// 坐席访客历史 + 消息搜索（M7）
	agent.GET("/conversations/:public_id/visitor-history", s.AgentVisitorHistory)
	agent.GET("/messages/search", s.AgentSearchMessages)
	// by-visitor conversations（CRM IM 对话历史，P4）
	agent.GET("/visitors/:id/conversations", s.AgentVisitorConversations)
	agent.GET("/quick-replies", s.AgentListQuickReplies)
	agent.POST("/quick-replies", s.AgentCreateQuickReply)
	agent.PUT("/quick-replies/:id", s.AgentUpdateQuickReply)
	agent.DELETE("/quick-replies/:id", s.AgentDeleteQuickReply)

	// 知识库未命中回流 + 命中统计（RAG M1 / M1.5）
	agent.GET("/kb-misses", s.AgentListKBMisses)
	agent.PUT("/kb-misses/:id", s.AgentUpdateKBMiss)
	agent.GET("/kb-stats", s.AgentKBStats)

	// 管理端站点（M2 嵌入配置）
	admin := r.Group("/api/v1/im/admin", gatewayauth.RequireAnyPermission("im:admin:manage"))
	admin.GET("/sites", s.AdminListSites)
	admin.POST("/sites", s.AdminCreateSite)
	admin.PUT("/sites/:id", s.AdminUpdateSite)

	// admin sensitive words（控制台词库，热生效）
	admin.GET("/sensitive-words", s.AdminListSensitiveWords)
	admin.POST("/sensitive-words", s.AdminCreateSensitiveWords)
	admin.POST("/sensitive-words/test", s.AdminTestSensitiveText)
	admin.PUT("/sensitive-words/:id", s.AdminUpdateSensitiveWord)
	admin.DELETE("/sensitive-words/:id", s.AdminDeleteSensitiveWord)

	// 管理端技能组（M3）
	admin.GET("/skill-groups", s.AdminListSkillGroups)
	admin.POST("/skill-groups", s.AdminCreateSkillGroup)
	admin.PUT("/skill-groups/:id", s.AdminUpdateSkillGroup)
	admin.GET("/skill-groups/:id/agents", s.AdminListSkillAgents)
	admin.POST("/skill-groups/:id/agents", s.AdminUpsertAgentSkill)
	admin.POST("/agent-skills", s.AdminUpsertAgentSkill)
	admin.DELETE("/agent-skills/:id", s.AdminDeleteAgentSkill)

	// WebSocket
	r.GET("/im/ws", s.WebSocket)

	// 附件下载（对象存储，带遗留磁盘回退）
	r.GET("/im/uploads/*key", s.ServeAttachment)
}
