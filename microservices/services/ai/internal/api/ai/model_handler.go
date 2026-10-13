package ai

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/ai/internal/dao"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/ai/internal/service/hermes"
	"github.com/go-admin-kit/services/shared/pkg/mask"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
)

// isPlatformAdmin 读取认证中间件写入的平台管理员标志（middleware/auth.go）。
func isPlatformAdmin(c *gin.Context) bool {
	v, _ := c.Get("platform_admin")
	b, _ := v.(bool)
	return b
}

// resolveStatsTenantID 解析统计接口的租户过滤：
// 平台管理员可用 tenant_id 查询参数跨租户查（缺省为全平台）；
// 普通用户忽略查询参数，一律强制当前租户。
func resolveStatsTenantID(c *gin.Context) *uint64 {
	if isPlatformAdmin(c) {
		if tid := c.Query("tenant_id"); tid != "" {
			if v, err := strconv.ParseUint(tid, 10, 64); err == nil {
				return &v
			}
		}
		return nil
	}
	v := uint64(tenant.FromContextOrDefault(c.Request.Context()))
	return &v
}

// AIModelHandler 模型配置管理接口
type AIModelHandler struct {
	service *service.AIModelService
}

func NewAIModelHandler(svc *service.AIModelService) *AIModelHandler {
	return &AIModelHandler{service: svc}
}

// ListModels 查询模型列表
// GET /api/v1/ai/models?page=1&page_size=20&provider=openai&status=1
func (h *AIModelHandler) ListModels(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	provider := c.Query("provider")

	var status *int
	if s := c.Query("status"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			status = &v
		}
	}

	models, total, err := h.service.ListModels(c.Request.Context(), page, pageSize, provider, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "success",
		"data": gin.H{
			"list":  models,
			"total": total,
		},
	})
}

// GetModel 查询模型详情
// GET /api/v1/ai/models/:id
func (h *AIModelHandler) GetModel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid id"})
		return
	}

	m, err := h.service.GetModelForDisplay(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "model not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": m})
}

// CreateModel 创建模型
// POST /api/v1/ai/models
func (h *AIModelHandler) CreateModel(c *gin.Context) {
	var req localmodel.AIModel
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request body"})
		return
	}

	if err := h.service.CreateModel(c.Request.Context(), &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	responseModel := req
	if responseModel.APIKey != "" {
		responseModel.APIKey = mask.RedactedValue
	}
	responseModel.APIKeyIV = ""
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": responseModel})
}

// UpdateModel 更新模型
// PUT /api/v1/ai/models/:id
func (h *AIModelHandler) UpdateModel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid id"})
		return
	}

	var req localmodel.AIModel
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request body"})
		return
	}

	if err := h.service.UpdateModel(c.Request.Context(), id, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success"})
}

// DeleteModel 删除模型
// DELETE /api/v1/ai/models/:id
func (h *AIModelHandler) DeleteModel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid id"})
		return
	}

	if err := h.service.DeleteModel(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success"})
}

// ListEnabledModels 查询所有启用的模型（用于下拉选择）
// GET /api/v1/ai/models/enabled
func (h *AIModelHandler) ListEnabledModels(c *gin.Context) {
	models, err := h.service.ListEnabledModels(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": models})
}

// AIUsageHandler 调用记录与统计接口
type AIUsageHandler struct {
	service *service.AIUsageService
}

func NewAIUsageHandler(svc *service.AIUsageService) *AIUsageHandler {
	return &AIUsageHandler{service: svc}
}

// ListUsageLogs 查询调用记录
// GET /api/v1/ai/usage/logs?page=1&page_size=20&model_id=1&user_id=1&status=success&start_time=2024-01-01&end_time=2024-01-31
func (h *AIUsageHandler) ListUsageLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := c.Query("status")

	var modelID, userID *uint64
	if mid := c.Query("model_id"); mid != "" {
		if v, err := strconv.ParseUint(mid, 10, 64); err == nil {
			modelID = &v
		}
	}
	if uid := c.Query("user_id"); uid != "" {
		if v, err := strconv.ParseUint(uid, 10, 64); err == nil {
			userID = &v
		}
	}

	var startTime, endTime *time.Time
	if st := c.Query("start_time"); st != "" {
		if t, err := time.Parse("2006-01-02", st); err == nil {
			startTime = &t
		}
	}
	if et := c.Query("end_time"); et != "" {
		if t, err := time.Parse("2006-01-02", et); err == nil {
			// 包含结束日期的整天
			t = t.Add(24 * time.Hour).Add(-time.Second)
			endTime = &t
		}
	}

	logs, total, err := h.service.ListUsageLogs(c.Request.Context(), page, pageSize, modelID, userID, status, startTime, endTime)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "success",
		"data": gin.H{
			"list":  logs,
			"total": total,
		},
	})
}

// GetDailyStats 获取每日统计
// GET /api/v1/ai/usage/stats/daily?days=7&user_id=1&tenant_id=1
// tenant_id 查询参数仅平台管理员可用；普通用户强制当前租户。
func (h *AIUsageHandler) GetDailyStats(c *gin.Context) {
	days, _ := strconv.Atoi(c.DefaultQuery("days", "7"))

	var userID *uint64
	if uid := c.Query("user_id"); uid != "" {
		if v, err := strconv.ParseUint(uid, 10, 64); err == nil {
			userID = &v
		}
	}
	tenantID := resolveStatsTenantID(c)

	stats, err := h.service.GetDailyStats(c.Request.Context(), days, userID, tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": stats})
}

// GetTopUsers 获取Top用户排行
// GET /api/v1/ai/usage/stats/top-users?days=30&limit=10
func (h *AIUsageHandler) GetTopUsers(c *gin.Context) {
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	stats, err := h.service.GetTopUsers(c.Request.Context(), days, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": stats})
}

// GetUsageSummary 获取用量汇总
// GET /api/v1/ai/usage/stats/summary?start_time=2024-01-01&end_time=2024-01-31&user_id=1&tenant_id=1
// tenant_id 查询参数仅平台管理员可用；普通用户强制当前租户。
func (h *AIUsageHandler) GetUsageSummary(c *gin.Context) {
	startTime, _ := time.Parse("2006-01-02", c.DefaultQuery("start_time", time.Now().AddDate(0, 0, -30).Format("2006-01-02")))
	endTime, _ := time.Parse("2006-01-02", c.DefaultQuery("end_time", time.Now().Format("2006-01-02")))
	endTime = endTime.Add(24 * time.Hour).Add(-time.Second)

	var userID *uint64
	if uid := c.Query("user_id"); uid != "" {
		if v, err := strconv.ParseUint(uid, 10, 64); err == nil {
			userID = &v
		}
	}
	tenantID := resolveStatsTenantID(c)

	summary, err := h.service.GetUsageSummary(c.Request.Context(), startTime, endTime, userID, tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": summary})
}

// AIQuotaHandler 配额管理接口
type AIQuotaHandler struct {
	service *service.AIQuotaService
}

func NewAIQuotaHandler(svc *service.AIQuotaService) *AIQuotaHandler {
	return &AIQuotaHandler{service: svc}
}

// ListQuotas 查询配额列表
// GET /api/v1/ai/quotas?scope=user
func (h *AIQuotaHandler) ListQuotas(c *gin.Context) {
	scope := c.Query("scope")

	quotas, err := h.service.ListQuotas(c.Request.Context(), scope)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": quotas})
}

// upsertQuotaRequest 配额写入请求体（专用绑定结构，防止透传改写他租户配额）。
type upsertQuotaRequest struct {
	Scope               string   `json:"scope" binding:"required,oneof=global tenant user"`
	ScopeID             *uint64  `json:"scope_id,omitempty"`
	DailyTokenLimit     *int     `json:"daily_token_limit,omitempty"`
	DailyCostLimit      *float64 `json:"daily_cost_limit,omitempty"`
	DailyRequestLimit   *int     `json:"daily_request_limit,omitempty"`
	MonthlyTokenLimit   *int     `json:"monthly_token_limit,omitempty"`
	MonthlyCostLimit    *float64 `json:"monthly_cost_limit,omitempty"`
	MonthlyRequestLimit *int     `json:"monthly_request_limit,omitempty"`
	ResetDayOfMonth     int      `json:"reset_day_of_month"`
	Enabled             int      `json:"enabled"`
}

// UpsertQuota 创建或更新配额
// POST /api/v1/ai/quotas
// 归属规则：非平台管理员写 tenant 配额时 scope_id 强制为当前租户，
// 写 user 配额时校验该用户属于当前租户，global 仅平台管理员可写（DAO 层强制）。
func (h *AIQuotaHandler) UpsertQuota(c *gin.Context) {
	var req upsertQuotaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request body"})
		return
	}
	if req.Scope != localmodel.AIQuotaScopeGlobal && req.ScopeID == nil && (isPlatformAdmin(c) || req.Scope == localmodel.AIQuotaScopeUser) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "scope_id is required for tenant/user scope"})
		return
	}
	if req.Scope == localmodel.AIQuotaScopeGlobal {
		req.ScopeID = nil
	}

	quota := localmodel.AIQuota{
		Scope:               req.Scope,
		ScopeID:             req.ScopeID,
		DailyTokenLimit:     req.DailyTokenLimit,
		DailyCostLimit:      req.DailyCostLimit,
		DailyRequestLimit:   req.DailyRequestLimit,
		MonthlyTokenLimit:   req.MonthlyTokenLimit,
		MonthlyCostLimit:    req.MonthlyCostLimit,
		MonthlyRequestLimit: req.MonthlyRequestLimit,
		ResetDayOfMonth:     req.ResetDayOfMonth,
		Enabled:             req.Enabled,
	}

	if err := h.service.UpsertQuota(c.Request.Context(), &quota); err != nil {
		if errors.Is(err, dao.ErrQuotaScopeForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "quota scope not accessible"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": quota})
}

// DeleteQuota 删除配额（先校验归属再删除，见 DAO 层）
// DELETE /api/v1/ai/quotas/:id
func (h *AIQuotaHandler) DeleteQuota(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid id"})
		return
	}

	if err := h.service.DeleteQuota(c.Request.Context(), id); err != nil {
		if errors.Is(err, dao.ErrQuotaScopeForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "quota scope not accessible"})
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "quota not found"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success"})
}
