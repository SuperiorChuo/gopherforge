package system

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	dao "github.com/go-admin-kit/services/audit/internal/dao/system"
	localmodel "github.com/go-admin-kit/services/audit/internal/model"
	systemsvc "github.com/go-admin-kit/services/audit/internal/service/system"
	"github.com/go-admin-kit/services/shared/pkg/pagination"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

type WebhookAPI struct{ service *systemsvc.WebhookService }

func NewWebhookAPI(service *systemsvc.WebhookService) *WebhookAPI {
	return &WebhookAPI{service: service}
}

type webhookMutationRequest struct {
	Name         string   `json:"name" binding:"required,max=128"`
	EndpointURL  string   `json:"endpoint_url" binding:"required,max=2048"`
	EventActions []string `json:"event_actions" binding:"required,min=1,max=50"`
	Status       *int8    `json:"status"`
}

func (a *WebhookAPI) List(c *gin.Context) {
	req := pagination.GetPageRequest(c)
	rows, total, err := a.service.List(c.Request.Context(), req.Page, req.PageSize)
	if err != nil {
		internalServerError(c, "failed to list webhook subscriptions", err)
		return
	}
	response.PageSuccess(c, rows, total, req.Page, req.PageSize)
}
func (a *WebhookAPI) ListDeliveries(c *gin.Context) {
	req := pagination.GetPageRequest(c)
	subscriptionID, _ := strconv.ParseUint(c.Query("subscription_id"), 10, 64)
	rows, total, err := a.service.ListDeliveries(c.Request.Context(), uint(subscriptionID), req.Page, req.PageSize)
	if writeWebhookError(c, err) {
		return
	}
	response.PageSuccess(c, rows, total, req.Page, req.PageSize)
}
func (a *WebhookAPI) Create(c *gin.Context) {
	var req webhookMutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid webhook request")
		return
	}
	status := localmodel.WebhookSubscriptionEnabled
	if req.Status != nil {
		status = *req.Status
	}
	userID, _ := c.Get("user_id")
	createdBy, _ := userID.(uint)
	result, err := a.service.Create(c.Request.Context(), systemsvc.WebhookMutation{Name: req.Name, EndpointURL: req.EndpointURL, EventActions: req.EventActions, Status: status, CreatedBy: createdBy})
	if writeWebhookError(c, err) {
		return
	}
	response.Success(c, result)
}
func (a *WebhookAPI) Update(c *gin.Context) {
	id, ok := webhookID(c)
	if !ok {
		return
	}
	var req webhookMutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid webhook request")
		return
	}
	status := localmodel.WebhookSubscriptionEnabled
	if req.Status != nil {
		status = *req.Status
	}
	row, err := a.service.Update(c.Request.Context(), id, systemsvc.WebhookMutation{Name: req.Name, EndpointURL: req.EndpointURL, EventActions: req.EventActions, Status: status})
	if writeWebhookError(c, err) {
		return
	}
	response.Success(c, row)
}
func (a *WebhookAPI) Delete(c *gin.Context) {
	id, ok := webhookID(c)
	if !ok {
		return
	}
	err := a.service.Delete(c.Request.Context(), id)
	if writeWebhookError(c, err) {
		return
	}
	response.Success(c, nil)
}
func (a *WebhookAPI) ResetSecret(c *gin.Context) {
	id, ok := webhookID(c)
	if !ok {
		return
	}
	result, err := a.service.ResetSecret(c.Request.Context(), id)
	if writeWebhookError(c, err) {
		return
	}
	response.Success(c, result)
}
func webhookID(c *gin.Context) (uint, bool) {
	value, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || value == 0 {
		response.BadRequest(c, "invalid webhook id")
		return 0, false
	}
	return uint(value), true
}
func writeWebhookError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, dao.ErrWebhookNotFound):
		response.NotFound(c, "webhook subscription not found")
	case errors.Is(err, systemsvc.ErrWebhookValidation):
		response.BadRequest(c, "invalid webhook subscription")
	case errors.Is(err, systemsvc.ErrWebhookSecret):
		response.Error(c, http.StatusServiceUnavailable, "webhook signing secret is not configured")
	default:
		internalServerError(c, "webhook operation failed", err)
	}
	return true
}
