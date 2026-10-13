// Package api wires the AI service HTTP surface. Console endpoints use the
// existing JWT session; Hermes enrollment and signed device endpoints have
// their own narrowly scoped authentication chains.
package api

import (
	"time"

	"github.com/gin-gonic/gin"

	aihandler "github.com/go-admin-kit/services/ai/internal/api/ai"
	"github.com/go-admin-kit/services/ai/internal/api/common"
	"github.com/go-admin-kit/services/ai/internal/middleware"
	sharedapi "github.com/go-admin-kit/services/shared/pkg/sharedapi"
)

// agentEventStreamRateLimit 限制事件流的**新建连接**频率。
//
// 事件流是长连接，每个连接按 run 状态周期轮询事件表；没有这道闸时，一次
// 并发建连/疯狂重连就能把查询压到同一张表上。阈值取得很宽（正常开关抽屉
// 远低于它），只挡异常形态。提成变量供测试针对同一份阈值断言。
var agentEventStreamRateLimit = middleware.RateLimitConfig{
	Window: 1 * time.Minute, MaxRequests: 30, KeyPrefix: "agent_events",
}

// SetupRoutesWithDeps mounts the API with injected infrastructure handles
// and the AI endpoint handler assembled at the composition root.
func SetupRoutesWithDeps(router *gin.Engine, deps sharedapi.Dependencies, handler *aihandler.Handler, modelHandler *aihandler.AIModelHandler, usageHandler *aihandler.AIUsageHandler, quotaHandler *aihandler.AIQuotaHandler, hermesHandler *aihandler.HermesHandler, hermesOfficeHandler *aihandler.HermesOfficeHandler, hermesProxyHandler *aihandler.HermesProxyHandler, hermesUsageHandler *aihandler.HermesUsageHandler, hermesDeviceAuth gin.HandlerFunc) {
	api := router.Group("/api/v1")

	common.RegisterPublicRoutesWithDeps(api, deps)

	// Service-to-service (im-service → ai-service over docker network, no JWT).
	// Secured by X-Internal-Token == AI_INTERNAL_TOKEN.
	internal := api.Group("/ai/internal")
	{
		internal.POST("/kb/search", handler.InternalSearchDocuments)
	}

	// Enrollment starts before a console identity exists. The high-entropy
	// device code is later approved by an authenticated employee.
	hermesEnrollment := api.Group("/ai/hermes")
	{
		hermesEnrollment.POST("/device-authorizations",
			middleware.RateLimit(middleware.RateLimitConfig{Window: 1 * time.Minute, MaxRequests: 10, KeyPrefix: "hermes_enroll"}),
			hermesHandler.CreateDeviceAuthorization,
		)
		hermesEnrollment.POST("/device-authorizations/token",
			middleware.RateLimit(middleware.RateLimitConfig{Window: 1 * time.Minute, MaxRequests: 30, KeyPrefix: "hermes_token"}),
			hermesHandler.ExchangeDeviceCode,
		)
	}

	device := api.Group("/ai/hermes/device")
	device.Use(hermesDeviceAuth)
	{
		device.GET("/desired-state", hermesHandler.DesiredState)
		device.POST("/heartbeat", hermesHandler.Heartbeat)
		device.POST("/events", hermesHandler.RecordEvent)
		// manager 用设备签名身份换取给 Hermes 用的短期模型 token
		device.POST("/model-token", hermesProxyHandler.IssueToken)
		// 三期：设备端执行完任务后回传结果（completed/failed + result）
		device.POST("/task-runs", hermesHandler.ReportTaskRun)
		// M5 Cron：设备端执行完定时任务后回传结果（completed/failed + result）
		device.POST("/cron-runs", hermesHandler.ReportCronRun)
		// M2 技能市场：设备签名下载技能 tarball（manager 按 SourceURL 拉取）
		device.GET("/skills/:id/download", hermesHandler.DownloadSkill)
		// M3 MCP 管理：设备签名拉取 MCP server 密钥值（写进 .env）
		device.POST("/mcp-secrets", hermesHandler.IssueMcpSecrets)
	}

	// 企业模型代理：OpenAI 兼容入站接口，供受管 Hermes 直接调用。
	// 鉴权在 handler 内做（bearer token + 每请求实时权益校验），不走设备签名——
	// 原生 Hermes 只会发标准 OpenAI 请求，不实现我们的逐请求签名协议。
	// 路径与策略里下发的 model.base_url 对应：<host>/api/v1/ai/hermes/proxy/v1
	proxy := api.Group("/ai/hermes/proxy/v1")
	{
		proxy.POST("/chat/completions",
			middleware.RateLimit(middleware.RateLimitConfig{Window: 1 * time.Minute, MaxRequests: 120, KeyPrefix: "hermes_proxy"}),
			hermesProxyHandler.ChatCompletions,
		)
	}

	protected := api.Group("/ai")
	protected.Use(middleware.AuthMiddleware(), middleware.OperationLogger())
	{
		protected.GET("/status", handler.GetStatus)

		protected.POST("/chat", handler.Chat)
		protected.GET("/conversations", handler.ListConversations)
		protected.GET("/conversations/:id/messages", handler.ListConversationMessages)
		protected.DELETE("/conversations/:id", handler.DeleteConversation)

		protected.POST("/kb/documents", middleware.PermissionMiddleware("ai:kb:write"), handler.CreateDocument)
		protected.GET("/kb/documents", handler.ListDocuments)
		protected.GET("/kb/documents/:id", handler.GetDocument)
		protected.PUT("/kb/documents/:id", middleware.PermissionMiddleware("ai:kb:write"), handler.UpdateDocument)
		protected.GET("/kb/documents/:id/chunks", handler.ListDocumentChunks)
		protected.DELETE("/kb/documents/:id", middleware.PermissionMiddleware("ai:kb:write"), handler.DeleteDocument)
		protected.POST("/kb/search", handler.SearchDocuments)

		protected.POST("/logs/insight", handler.GenerateLogInsight)
		protected.POST("/compose", handler.Compose)
		protected.POST("/image", handler.Image)

		// AI 模型配置管理
		protected.GET("/models/enabled", modelHandler.ListEnabledModels)
		protected.GET("/models", middleware.PermissionMiddleware("ai:model:list"), modelHandler.ListModels)
		protected.GET("/models/:id", middleware.PermissionMiddleware("ai:model:list"), modelHandler.GetModel)
		protected.POST("/models", middleware.PermissionMiddleware("ai:model:create"), modelHandler.CreateModel)
		protected.PUT("/models/:id", middleware.PermissionMiddleware("ai:model:update"), modelHandler.UpdateModel)
		protected.DELETE("/models/:id", middleware.PermissionMiddleware("ai:model:delete"), modelHandler.DeleteModel)

		// AI 调用记录与统计
		protected.GET("/usage/logs", middleware.PermissionMiddleware("ai:usage:list"), usageHandler.ListUsageLogs)
		protected.GET("/usage/stats/daily", middleware.PermissionMiddleware("ai:usage:list"), usageHandler.GetDailyStats)
		protected.GET("/usage/stats/top-users", middleware.PermissionMiddleware("ai:usage:list"), usageHandler.GetTopUsers)
		protected.GET("/usage/stats/summary", middleware.PermissionMiddleware("ai:usage:list"), usageHandler.GetUsageSummary)

		// AI 配额管理
		protected.GET("/quotas", middleware.PermissionMiddleware("ai:quota:list"), quotaHandler.ListQuotas)
		protected.POST("/quotas", middleware.PermissionMiddleware("ai:quota:update"), quotaHandler.UpsertQuota)
		protected.DELETE("/quotas/:id", middleware.PermissionMiddleware("ai:quota:delete"), quotaHandler.DeleteQuota)

		// Agent Kernel：运行时中立的定义版本、可靠 Run 与事件流。
		protected.GET("/agent-kernel/tools", middleware.PermissionMiddleware("ai:agent:list"), handler.ListAgentTools)
		protected.GET("/agent-kernel/agents", middleware.PermissionMiddleware("ai:agent:list"), handler.ListAgentDefinitions)
		protected.POST("/agent-kernel/agents", middleware.PermissionMiddleware("ai:agent:write"), handler.CreateAgentDefinition)
		protected.GET("/agent-kernel/agents/:id", middleware.PermissionMiddleware("ai:agent:list"), handler.GetAgentDefinition)
		protected.PATCH("/agent-kernel/agents/:id", middleware.PermissionMiddleware("ai:agent:write"), handler.UpdateAgentDefinition)
		protected.GET("/agent-kernel/agents/:id/versions", middleware.PermissionMiddleware("ai:agent:list"), handler.ListAgentVersions)
		protected.POST("/agent-kernel/agents/:id/versions", middleware.PermissionMiddleware("ai:agent:write"), handler.PublishAgentVersion)
		protected.POST("/agent-kernel/agents/:id/runs", middleware.PermissionMiddleware("ai:agent:run"), handler.CreateAgentRun)
		protected.GET("/agent-kernel/runs", middleware.PermissionMiddleware("ai:agent:list"), handler.ListAgentRuns)
		protected.GET("/agent-kernel/runs/:id", middleware.PermissionMiddleware("ai:agent:list"), handler.GetAgentRun)
		protected.POST("/agent-kernel/runs/:id/cancel", middleware.PermissionMiddleware("ai:agent:run"), handler.CancelAgentRun)
		// 事件流是长连接且按 run 状态周期轮询：限的是**新建连接**频率，
		// 防疯狂重连/并发建连打爆事件表；正常开关抽屉远低于该阈值。
		// 配置提成变量是为了让测试针对**同一份**阈值断言，而不是另写一遍字面量。
		protected.GET("/agent-kernel/runs/:id/events",
			middleware.RateLimit(agentEventStreamRateLimit),
			middleware.PermissionMiddleware("ai:agent:list"), handler.StreamAgentRunEvents)

		// Hermes employee self-service. Identity always comes from the console
		// session; these endpoints never accept another user id in the body.
		protected.GET("/hermes/me", hermesHandler.GetMe)
		protected.GET("/hermes/downloads", hermesHandler.ListDownloads)
		protected.GET("/hermes/me/devices", hermesHandler.ListMyDevices)
		protected.POST("/hermes/device-authorizations/approve", hermesHandler.ApproveDeviceAuthorization)
		protected.POST("/hermes/me/devices/:device_id/revoke", hermesHandler.RevokeMyDevice)
		// M6 员工自助：技能目录/安装/卸载 + 个人用量。同样只信会话身份。
		protected.GET("/hermes/me/skills/catalog", hermesHandler.ListMySkillCatalog)
		protected.POST("/hermes/me/skills/:id", hermesHandler.InstallMySkill)
		protected.DELETE("/hermes/me/skills/:id", hermesHandler.UninstallMySkill)
		protected.GET("/hermes/me/usage", hermesUsageHandler.MyUsage)

		// Hermes tenant administration uses action-specific RBAC permissions.
		protected.GET("/hermes/entitlements", middleware.PermissionMiddleware("ai:hermes:entitlement"), hermesHandler.ListEntitlements)
		protected.PUT("/hermes/users/:user_id/entitlement", middleware.PermissionMiddleware("ai:hermes:entitlement"), hermesHandler.UpdateEntitlement)
		// Agent 实例：逻辑体，与承载它的设备（运行节点）分开管理
		protected.GET("/hermes/instances", middleware.PermissionMiddleware("ai:hermes:instance:list"), hermesHandler.ListInstances)
		protected.POST("/hermes/instances", middleware.PermissionMiddleware("ai:hermes:instance:write"), hermesHandler.CreateInstance)
		protected.PUT("/hermes/instances/:id", middleware.PermissionMiddleware("ai:hermes:instance:write"), hermesHandler.UpdateInstance)
		protected.DELETE("/hermes/instances/:id", middleware.PermissionMiddleware("ai:hermes:instance:write"), hermesHandler.DeleteInstance)

		protected.GET("/hermes/devices", middleware.PermissionMiddleware("ai:hermes:device:list"), hermesHandler.ListDevices)
		protected.GET("/hermes/devices/:device_id", middleware.PermissionMiddleware("ai:hermes:device:list"), hermesHandler.GetDevice)
		protected.POST("/hermes/devices/:device_id/revoke", middleware.PermissionMiddleware("ai:hermes:device:revoke"), hermesHandler.RevokeDevice)
		// 运维动作：只接受固定枚举（retry_update / apply_policy / restart），空串为撤销
		protected.POST("/hermes/devices/:device_id/actions", middleware.PermissionMiddleware("ai:hermes:device:action"), hermesHandler.RequestDeviceAction)

		// 多 Agent 协作任务（第二期）：集群 + 任务 + 执行记录，仅控制面编排。
		protected.GET("/hermes/clusters", middleware.PermissionMiddleware("ai:hermes:cluster:list"), hermesHandler.ListClusters)
		protected.POST("/hermes/clusters", middleware.PermissionMiddleware("ai:hermes:cluster:write"), hermesHandler.CreateCluster)
		protected.GET("/hermes/clusters/:id", middleware.PermissionMiddleware("ai:hermes:cluster:list"), hermesHandler.GetCluster)
		protected.PUT("/hermes/clusters/:id", middleware.PermissionMiddleware("ai:hermes:cluster:write"), hermesHandler.UpdateCluster)
		protected.DELETE("/hermes/clusters/:id", middleware.PermissionMiddleware("ai:hermes:cluster:write"), hermesHandler.DeleteCluster)
		protected.GET("/hermes/tasks", middleware.PermissionMiddleware("ai:hermes:task:list"), hermesHandler.ListTasks)
		protected.POST("/hermes/tasks", middleware.PermissionMiddleware("ai:hermes:task:write"), hermesHandler.CreateTask)
		protected.GET("/hermes/tasks/:id", middleware.PermissionMiddleware("ai:hermes:task:list"), hermesHandler.GetTask)
		protected.POST("/hermes/tasks/:id/activate", middleware.PermissionMiddleware("ai:hermes:task:write"), hermesHandler.ActivateTask)
		protected.POST("/hermes/tasks/:id/complete", middleware.PermissionMiddleware("ai:hermes:task:write"), hermesHandler.CompleteTask)
		protected.POST("/hermes/tasks/:id/cancel", middleware.PermissionMiddleware("ai:hermes:task:write"), hermesHandler.CancelTask)
		protected.PUT("/hermes/task-runs/:id", middleware.PermissionMiddleware("ai:hermes:task:write"), hermesHandler.UpdateTaskRun)

		// M5 Cron 定时任务
		protected.GET("/hermes/cron", middleware.PermissionMiddleware("ai:hermes:cron:list"), hermesHandler.ListCronJobs)
		protected.POST("/hermes/cron", middleware.PermissionMiddleware("ai:hermes:cron:write"), hermesHandler.CreateCronJob)
		protected.PUT("/hermes/cron/:id", middleware.PermissionMiddleware("ai:hermes:cron:write"), hermesHandler.UpdateCronJob)
		protected.PATCH("/hermes/cron/:id/toggle", middleware.PermissionMiddleware("ai:hermes:cron:write"), hermesHandler.ToggleCronJob)
		protected.DELETE("/hermes/cron/:id", middleware.PermissionMiddleware("ai:hermes:cron:write"), hermesHandler.DeleteCronJob)
		protected.GET("/hermes/cron/:id/runs", middleware.PermissionMiddleware("ai:hermes:cron:list"), hermesHandler.ListCronRuns)

		// M2 技能市场：录入与审批分权，集群绑定单独权限。
		protected.GET("/hermes/skills", middleware.PermissionMiddleware("ai:hermes:skill:list"), hermesHandler.ListSkills)
		protected.POST("/hermes/skills", middleware.PermissionMiddleware("ai:hermes:skill:write"), hermesHandler.CreateSkill)
		// 官方 Skills Hub：浏览 GitHub 系源 + 导入企业目录（落 pending 待审批）
		protected.GET("/hermes/skills/hub", middleware.PermissionMiddleware("ai:hermes:skill:list"), hermesHandler.BrowseHubSkills)
		protected.POST("/hermes/skills/hub/import", middleware.PermissionMiddleware("ai:hermes:skill:write"), hermesHandler.ImportHubSkill)
		protected.POST("/hermes/skills/:id/approve", middleware.PermissionMiddleware("ai:hermes:skill:approve"), hermesHandler.ApproveSkill)
		protected.POST("/hermes/skills/:id/reject", middleware.PermissionMiddleware("ai:hermes:skill:approve"), hermesHandler.RejectSkill)
		protected.POST("/hermes/skills/:id/revoke", middleware.PermissionMiddleware("ai:hermes:skill:approve"), hermesHandler.RevokeSkill)
		protected.GET("/hermes/clusters/:id/skills", middleware.PermissionMiddleware("ai:hermes:cluster:list"), hermesHandler.ListClusterSkills)
		protected.POST("/hermes/clusters/:id/skills", middleware.PermissionMiddleware("ai:hermes:skill:bind"), hermesHandler.BindClusterSkill)
		protected.DELETE("/hermes/clusters/:id/skills/:skill_id", middleware.PermissionMiddleware("ai:hermes:skill:bind"), hermesHandler.UnbindClusterSkill)

		// M3 MCP 管理：录入/审批/绑定分权，与技能市场同构。
		protected.GET("/hermes/mcp-servers", middleware.PermissionMiddleware("ai:hermes:mcp:list"), hermesHandler.ListMcpServers)
		protected.POST("/hermes/mcp-servers", middleware.PermissionMiddleware("ai:hermes:mcp:write"), hermesHandler.CreateMcpServer)
		protected.PUT("/hermes/mcp-servers/:id", middleware.PermissionMiddleware("ai:hermes:mcp:write"), hermesHandler.UpdateMcpServer)
		protected.POST("/hermes/mcp-servers/:id/approve", middleware.PermissionMiddleware("ai:hermes:mcp:approve"), hermesHandler.ApproveMcpServer)
		protected.POST("/hermes/mcp-servers/:id/reject", middleware.PermissionMiddleware("ai:hermes:mcp:approve"), hermesHandler.RejectMcpServer)
		protected.POST("/hermes/mcp-servers/:id/revoke", middleware.PermissionMiddleware("ai:hermes:mcp:approve"), hermesHandler.RevokeMcpServer)
		protected.GET("/hermes/clusters/:id/mcp-servers", middleware.PermissionMiddleware("ai:hermes:cluster:list"), hermesHandler.ListClusterMcpServers)
		protected.POST("/hermes/clusters/:id/mcp-servers", middleware.PermissionMiddleware("ai:hermes:mcp:bind"), hermesHandler.BindClusterMcpServer)
		protected.DELETE("/hermes/clusters/:id/mcp-servers/:server_id", middleware.PermissionMiddleware("ai:hermes:mcp:bind"), hermesHandler.UnbindClusterMcpServer)

		protected.GET("/hermes/policies", middleware.PermissionMiddleware("ai:hermes:policy:read"), hermesHandler.ListPolicies)
		protected.POST("/hermes/policies", middleware.PermissionMiddleware("ai:hermes:policy:write"), hermesHandler.CreatePolicy)
		protected.PUT("/hermes/policies/:id", middleware.PermissionMiddleware("ai:hermes:policy:write"), hermesHandler.UpdatePolicy)
		protected.DELETE("/hermes/policies/:id", middleware.PermissionMiddleware("ai:hermes:policy:write"), hermesHandler.DeletePolicy)
		protected.GET("/hermes/artifacts", middleware.PermissionMiddleware("ai:hermes:policy:read"), hermesHandler.ListArtifacts)
		// 录入是「发布」动作，与审批分权：录入的人不必然有权批准自己录的制品。
		protected.POST("/hermes/artifacts", middleware.PermissionMiddleware("ai:hermes:artifact:write"), hermesHandler.CreateArtifact)
		protected.POST("/hermes/artifacts/:id/approve", middleware.PermissionMiddleware("ai:hermes:artifact:approve"), hermesHandler.ApproveArtifact)
		protected.POST("/hermes/artifacts/:id/reject", middleware.PermissionMiddleware("ai:hermes:artifact:approve"), hermesHandler.RejectArtifact)
		protected.POST("/hermes/artifacts/:id/revoke", middleware.PermissionMiddleware("ai:hermes:artifact:approve"), hermesHandler.RevokeArtifact)

		// 模型代理用量：只读元数据（次数/token/费用/耗时/错误码），无请求正文。
		protected.GET("/hermes/usage/overview", middleware.PermissionMiddleware("ai:hermes:device:list"), hermesUsageHandler.Overview)
		protected.GET("/hermes/usage/devices/:device_id", middleware.PermissionMiddleware("ai:hermes:device:list"), hermesUsageHandler.DeviceUsage)

		// 3D 办公室地图：布局与员工绑定为管理员运维视图，读写分权。
		protected.GET("/hermes/office/floors", middleware.PermissionMiddleware("ai:hermes:office:read"), hermesOfficeHandler.ListFloors)
		protected.POST("/hermes/office/floors", middleware.PermissionMiddleware("ai:hermes:office:write"), hermesOfficeHandler.CreateFloor)
		protected.PUT("/hermes/office/floors/:id", middleware.PermissionMiddleware("ai:hermes:office:write"), hermesOfficeHandler.UpdateFloor)
		protected.DELETE("/hermes/office/floors/:id", middleware.PermissionMiddleware("ai:hermes:office:write"), hermesOfficeHandler.DeleteFloor)
		protected.GET("/hermes/office/floors/:id/seats", middleware.PermissionMiddleware("ai:hermes:office:read"), hermesOfficeHandler.ListSeats)
		// 整层一次性提交，避免拖拽编辑器逐个工位打请求
		protected.PUT("/hermes/office/floors/:id/seats", middleware.PermissionMiddleware("ai:hermes:office:write"), hermesOfficeHandler.ReplaceSeats)
		protected.DELETE("/hermes/office/seats/:seat_id", middleware.PermissionMiddleware("ai:hermes:office:write"), hermesOfficeHandler.DeleteSeat)
	}
}
