package api

import "github.com/gin-gonic/gin"

// RegisterRoutes 挂载全部 bpm 路由。
// 网关 auth-verify 在 Traefik 层拦截；requireUser 兜底裸连场景。
// internal 端点不经网关（无 Traefik router 放行 /internal），仅内网直连 +
// X-Internal-Token。
func (s *Server) RegisterRoutes(r *gin.Engine) {
	// health (no auth)
	r.GET("/api/v1/health/live", s.HealthLive)
	r.GET("/api/v1/health/ready", s.HealthReady)
	r.GET("/api/v1/bpm/health/live", s.HealthLive)
	r.GET("/api/v1/bpm/health/ready", s.HealthReady)

	// ---- 管理端：流程定义 ----
	r.GET("/api/v1/bpm/definitions", s.ListDefinitions)
	r.POST("/api/v1/bpm/definitions", s.CreateDefinition)
	r.GET("/api/v1/bpm/definitions/keys/:key/active", s.ActiveDefinitionByKey)
	r.GET("/api/v1/bpm/definitions/:id", s.GetDefinition)
	r.PUT("/api/v1/bpm/definitions/:id", s.UpdateDefinition)
	r.POST("/api/v1/bpm/definitions/:id/publish", s.PublishDefinition)
	r.POST("/api/v1/bpm/definitions/:id/new-version", s.NewDefinitionVersion)
	r.POST("/api/v1/bpm/definitions/:id/suspend", s.SuspendDefinition)

	// ---- 发起端 / 实例端 ----
	r.GET("/api/v1/bpm/startable", s.ListStartable) // 通用发起页（表单构建器 M1）
	r.POST("/api/v1/bpm/instances", s.CreateInstance)
	r.GET("/api/v1/bpm/instances", s.ListInstances) // M3：仅平台管理员（管理视图）
	r.GET("/api/v1/bpm/stats", s.Stats)             // 收官项：审批统计（仅平台管理员）
	r.GET("/api/v1/bpm/instances/my", s.MyInstances)
	r.GET("/api/v1/bpm/instances/:id", s.GetInstance)
	r.POST("/api/v1/bpm/instances/:id/cancel", s.CancelInstance)
	r.POST("/api/v1/bpm/instances/:id/resubmit", s.ResubmitInstance)
	r.POST("/api/v1/bpm/instances/:id/terminate", s.TerminateInstance) // M3：仅平台管理员
	r.GET("/api/v1/bpm/instances/:id/timeline", s.InstanceTimeline)
	r.GET("/api/v1/bpm/instances/:id/diagram", s.InstanceDiagram)

	// ---- 任务端（审批人视角）----
	r.GET("/api/v1/bpm/tasks/todo", s.ListTodo)
	r.GET("/api/v1/bpm/tasks/done", s.ListDone)
	r.GET("/api/v1/bpm/tasks/:id", s.GetTask)
	r.POST("/api/v1/bpm/tasks/:id/approve", s.ApproveTask)
	r.POST("/api/v1/bpm/tasks/:id/reject", s.RejectTask)
	r.POST("/api/v1/bpm/tasks/:id/transfer", s.TransferTask)
	r.POST("/api/v1/bpm/tasks/:id/return", s.ReturnTask)

	// ---- 抄送端（M2）----
	r.GET("/api/v1/bpm/cc/my", s.MyCc)
	r.POST("/api/v1/bpm/cc/:id/read", s.ReadCc)

	// ---- 内部端点（业务方服务端到服务端，X-Internal-Token）----
	r.POST("/api/v1/bpm/internal/instances", s.InternalCreateInstance)
	r.GET("/api/v1/bpm/internal/instances/by-biz", s.InternalInstancesByBiz)
}
