package api

import (
	"github.com/gin-gonic/gin"
	sharedaudit "github.com/go-admin-kit/services/shared/pkg/audittrail"
	"github.com/go-admin-kit/services/shared/pkg/gatewayauth"
)

// RegisterRoutes mounts all CRM routes（网关 auth-verify 已在 Traefik 层拦截，
// 服务内 requireUser 兜底裸连场景）。
func (s *Server) RegisterRoutes(r *gin.Engine) {
	// health (no auth)
	r.GET("/api/v1/health/live", s.HealthLive)
	r.GET("/api/v1/health/ready", s.HealthReady)
	r.GET("/api/v1/crm/health/live", s.HealthLive)
	r.GET("/api/v1/crm/health/ready", s.HealthReady)

	// 内部端点只信 X-Internal-Token，先注册以免套入控制台 RBAC。
	r.GET("/api/v1/crm/internal/lookup", s.InternalLookup)
	r.POST("/api/v1/crm/internal/followups", s.InternalCreateFollowUp)
	r.POST("/api/v1/crm/internal/bpm/callback", s.InternalBpmCallback)

	r.Use(gatewayauth.RequireReadWritePermissions("crm:read", "crm:write"))
	// 审计 actor/tenant 注入：从网关 forwardAuth 注入的 X-Auth-* 头把操作人写进
	// 请求 ctx，GORM 审计插件据此为业务写操作落 audit_logs（匿名/内部端点无
	// X-Auth-User-ID，不注入，插件静默跳过）。
	r.Use(sharedaudit.AuditHeaderMiddleware())

	// leads（所有名片）
	r.GET("/api/v1/crm/leads", s.ListLeads)
	r.POST("/api/v1/crm/leads", s.CreateLead)
	r.PUT("/api/v1/crm/leads/:id", s.UpdateLead)
	r.POST("/api/v1/crm/leads/:id/convert", s.ConvertLead)
	r.POST("/api/v1/crm/leads/assign", s.AssignLeads)
	r.POST("/api/v1/crm/leads/:id/claim", s.ClaimLead)

	// 组织数据 + 分配日志（P1）
	r.GET("/api/v1/crm/org", s.Org)
	r.GET("/api/v1/crm/assign-logs", s.ListAssignLogs)

	// 公海规则引擎 + 领取/释放（P2）
	r.GET("/api/v1/crm/pool/rules", s.GetPoolRules)
	r.PUT("/api/v1/crm/pool/rules", s.SavePoolRule)
	r.GET("/api/v1/crm/my-scope", s.MyScope)
	r.GET("/api/v1/crm/dup-groups", s.FindDupGroups)
	r.POST("/api/v1/crm/customers/merge", s.MergeCustomers)
	r.GET("/api/v1/crm/pool/stats", s.GetPoolStats)
	r.POST("/api/v1/crm/pool/recycle", s.RecycleNow)
	r.POST("/api/v1/crm/customers/:id/claim", s.ClaimCustomer)
	r.POST("/api/v1/crm/customers/:id/release", s.ReleaseCustomer)

	// customers + contacts
	r.GET("/api/v1/crm/customers", s.ListCustomers)
	r.GET("/api/v1/crm/customers/export", s.ExportCustomers) // CSV 导出（路线图第 11 项）
	r.POST("/api/v1/crm/customers", s.CreateCustomer)
	r.GET("/api/v1/crm/customers/:id", s.GetCustomerDetail)
	r.PUT("/api/v1/crm/customers/:id", s.UpdateCustomer)
	r.POST("/api/v1/crm/customers/:id/contacts", s.CreateContact)
	r.GET("/api/v1/crm/customers/:id/timeline", s.CustomerTimeline)
	r.DELETE("/api/v1/crm/contacts/:id", s.DeleteContact)

	// opportunities
	r.GET("/api/v1/crm/opportunities", s.ListOpportunities)
	r.POST("/api/v1/crm/opportunities", s.CreateOpportunity)
	r.GET("/api/v1/crm/opportunities/:id", s.GetOpportunity)
	r.PUT("/api/v1/crm/opportunities/:id", s.UpdateOpportunity)
	r.POST("/api/v1/crm/opportunities/:id/stage", s.AdvanceStage)

	// contracts + payments
	r.GET("/api/v1/crm/contracts", s.ListContracts)
	r.POST("/api/v1/crm/contracts", s.CreateContract)
	r.GET("/api/v1/crm/contracts/:id", s.GetContract)
	r.PUT("/api/v1/crm/contracts/:id", s.UpdateContract)
	r.POST("/api/v1/crm/contracts/:id/payments", s.AddPayment)

	// 合同审批流接入（BPM_ENABLE 开关；关闭时提交/进度端点 503）
	r.POST("/api/v1/crm/contracts/:id/submit-approval", s.SubmitContractApproval)
	r.GET("/api/v1/crm/contracts/:id/approval", s.ContractApprovalInstances)

	// follow-ups
	r.GET("/api/v1/crm/followups", s.ListFollowUps)
	r.POST("/api/v1/crm/followups", s.CreateFollowUp)

	// tags
	r.GET("/api/v1/crm/tags", s.ListTags)
	r.POST("/api/v1/crm/tags", s.CreateTag)
	r.PUT("/api/v1/crm/tags/:id", s.UpdateTag)
	r.DELETE("/api/v1/crm/tags/:id", s.DeleteTag)

	// dashboard stats
	r.GET("/api/v1/crm/stats", s.Stats)

	// phone lookup（来电弹屏）
	r.GET("/api/v1/crm/lookup", s.LookupPhone)

}
