package system

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	systemsvc "github.com/go-admin-kit/services/identity/internal/service/system"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

// InviteAPI 管理员邀请管理：创建（返回一次性 token/链接）/列表/撤销。
type InviteAPI struct {
	svc *systemsvc.InviteService
}

func NewInviteAPI() *InviteAPI {
	return &InviteAPI{}
}

func NewInviteAPIWithService(svc *systemsvc.InviteService) *InviteAPI {
	return &InviteAPI{svc: svc}
}

func (a *InviteAPI) CreateInvite(c *gin.Context) {
	var req systemsvc.CreateInviteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid body")
		return
	}
	result, err := a.svc.Create(c.Request.Context(), req)
	if err != nil {
		if errors.Is(err, systemsvc.ErrInviteRoleNotInTenant) {
			response.BadRequest(c, err.Error())
			return
		}
		response.InternalServerError(c, err.Error())
		return
	}
	response.Success(c, result)
}

func (a *InviteAPI) ListInvites(c *gin.Context) {
	list, err := a.svc.List(c.Request.Context())
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}
	response.Success(c, list)
}

func (a *InviteAPI) RevokeInvite(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.BadRequest(c, "invalid id")
		return
	}
	if err := a.svc.Revoke(c.Request.Context(), uint(id)); err != nil {
		if errors.Is(err, systemsvc.ErrInviteNotFound) {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalServerError(c, err.Error())
		return
	}
	response.SuccessWithMessage(c, "invite revoked successfully", nil)
}
