package auth

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	authsvc "github.com/go-admin-kit/services/auth/internal/service/auth"
	systemsvc "github.com/go-admin-kit/services/auth/internal/service/system"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

// OAuth2AdminAPI hosts the tenant-scoped application/token management endpoints
// (console UI). All responses use the repo's wrapped envelope.
type OAuth2AdminAPI struct {
	clients      authsvc.OAuth2ClientService
	auditService systemsvc.AuditLogService
}

func NewOAuth2AdminAPI(clients authsvc.OAuth2ClientService, audit systemsvc.AuditLogService) *OAuth2AdminAPI {
	return &OAuth2AdminAPI{clients: clients, auditService: audit}
}

type clientRequestBody struct {
	Name            string   `json:"name"`
	Logo            string   `json:"logo"`
	Description     string   `json:"description"`
	ClientType      int8     `json:"client_type"`
	RedirectURIs    []string `json:"redirect_uris"`
	Scopes          []string `json:"scopes"`
	GrantTypes      []string `json:"grant_types"`
	AccessTokenTTL  int      `json:"access_token_ttl"`
	RefreshTokenTTL int      `json:"refresh_token_ttl"`
	AutoApprove     bool     `json:"auto_approve"`
	Status          *int8    `json:"status"`
}

func (b clientRequestBody) toMutation() authsvc.ClientMutation {
	return authsvc.ClientMutation{
		Name: b.Name, Logo: b.Logo, Description: b.Description, ClientType: b.ClientType,
		RedirectURIs: b.RedirectURIs, Scopes: b.Scopes, GrantTypes: b.GrantTypes,
		AccessTokenTTL: b.AccessTokenTTL, RefreshTokenTTL: b.RefreshTokenTTL,
		AutoApprove: b.AutoApprove, Status: b.Status,
	}
}

func (a *OAuth2AdminAPI) audit(c *gin.Context, action, targetID, summary string, after map[string]any) {
	_ = a.auditService.Record(c, systemsvc.AuditRecordRequest{
		Action:     action,
		TargetType: "oauth2_client",
		TargetID:   targetID,
		After:      after,
		Summary:    summary,
	})
}

func (a *OAuth2AdminAPI) writeClientError(c *gin.Context, err error) {
	var validationErr authsvc.OAuth2ClientValidationError
	switch {
	case errors.Is(err, authsvc.ErrOAuth2ClientNotFound):
		response.NotFound(c, "应用不存在")
	case errors.As(err, &validationErr):
		response.BadRequest(c, validationErr.Message)
	default:
		response.InternalServerError(c, "操作失败")
	}
}

// ListClients GET /oauth2/clients
func (a *OAuth2AdminAPI) ListClients(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	clients, total, err := a.clients.List(c.Request.Context(), c.Query("keyword"), page, pageSize)
	if err != nil {
		response.InternalServerError(c, "查询失败")
		return
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	response.PageSuccess(c, clients, total, page, pageSize)
}

// GetClient GET /oauth2/clients/:id
func (a *OAuth2AdminAPI) GetClient(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	client, err := a.clients.Get(c.Request.Context(), uint(id))
	if err != nil {
		a.writeClientError(c, err)
		return
	}
	response.Success(c, client)
}

// GetCatalog GET /oauth2/catalog — static scope/grant options for the form.
func (a *OAuth2AdminAPI) GetCatalog(c *gin.Context) {
	scopes, grants := a.clients.SupportedCatalog()
	response.Success(c, gin.H{"scopes": scopes, "grant_types": grants})
}

// CreateClient POST /oauth2/clients — returns the plaintext secret once.
func (a *OAuth2AdminAPI) CreateClient(c *gin.Context) {
	var body clientRequestBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.BadRequest(c, "请求参数无效")
		return
	}
	tenantID, _ := c.Get("tenant_id")
	userID, _ := c.Get("user_id")
	tid, _ := tenantID.(uint)
	if tid == 0 {
		tid = 1
	}
	uid, _ := userID.(uint)
	result, err := a.clients.Create(c.Request.Context(), tid, uid, body.toMutation())
	if err != nil {
		a.writeClientError(c, err)
		return
	}
	a.audit(c, "oauth2_client.create", result.Client.ClientID, "创建 OAuth2 应用 "+result.Client.Name, map[string]any{"name": result.Client.Name})
	response.SuccessWithMessage(c, "创建成功", result)
}

// UpdateClient PUT /oauth2/clients/:id
func (a *OAuth2AdminAPI) UpdateClient(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var body clientRequestBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.BadRequest(c, "请求参数无效")
		return
	}
	client, err := a.clients.Update(c.Request.Context(), uint(id), body.toMutation())
	if err != nil {
		a.writeClientError(c, err)
		return
	}
	a.audit(c, "oauth2_client.update", client.ClientID, "更新 OAuth2 应用 "+client.Name, map[string]any{"name": client.Name})
	response.SuccessWithMessage(c, "更新成功", client)
}

// ResetSecret POST /oauth2/clients/:id/reset-secret — returns the new secret once.
func (a *OAuth2AdminAPI) ResetSecret(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	secret, client, err := a.clients.ResetSecret(c.Request.Context(), uint(id))
	if err != nil {
		a.writeClientError(c, err)
		return
	}
	a.audit(c, "oauth2_client.reset_secret", client.ClientID, "重置 OAuth2 应用密钥 "+client.Name, nil)
	response.SuccessWithMessage(c, "重置成功", gin.H{"client_secret": secret})
}

// DeleteClient DELETE /oauth2/clients/:id
func (a *OAuth2AdminAPI) DeleteClient(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	client, getErr := a.clients.Get(c.Request.Context(), uint(id))
	if err := a.clients.Delete(c.Request.Context(), uint(id)); err != nil {
		a.writeClientError(c, err)
		return
	}
	targetID := strconv.FormatUint(id, 10)
	if getErr == nil {
		targetID = client.ClientID
	}
	a.audit(c, "oauth2_client.delete", targetID, "删除 OAuth2 应用", nil)
	response.SuccessWithMessage(c, "删除成功", nil)
}

// ListTokens GET /oauth2/tokens
func (a *OAuth2AdminAPI) ListTokens(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	tokens, total, err := a.clients.ListTokens(c.Request.Context(), c.Query("client_id"), page, pageSize)
	if err != nil {
		response.InternalServerError(c, "查询失败")
		return
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	response.PageSuccess(c, tokens, total, page, pageSize)
}

// RevokeToken DELETE /oauth2/tokens/:id
func (a *OAuth2AdminAPI) RevokeToken(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := a.clients.RevokeToken(c.Request.Context(), uint(id)); err != nil {
		a.writeClientError(c, err)
		return
	}
	a.audit(c, "oauth2_token.revoke", strconv.FormatUint(id, 10), "吊销 OAuth2 令牌", nil)
	response.SuccessWithMessage(c, "已吊销", nil)
}
