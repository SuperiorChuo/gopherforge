package system

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	dao "github.com/go-admin-kit/services/audit/internal/dao/system"
	"github.com/go-admin-kit/services/audit/internal/pkg/authz"
	"github.com/go-admin-kit/services/shared/pkg/pagination"
	"github.com/go-admin-kit/services/shared/pkg/response"
	"gorm.io/gorm"
)

// LoginRiskEventAPI exposes abnormal-login events (new IP / new device).
type LoginRiskEventAPI struct {
	riskDAO *dao.LoginRiskEventDAO
}

func NewLoginRiskEventAPIWithDB(db *gorm.DB) *LoginRiskEventAPI {
	return &LoginRiskEventAPI{riskDAO: dao.NewLoginRiskEventDAO(db)}
}

// ListLoginRiskEvents GET /login-risk-events returns abnormal-login events with
// optional filters (username / ip / reason / processed).
func (a *LoginRiskEventAPI) ListLoginRiskEvents(c *gin.Context) {
	var processed *bool
	switch c.Query("processed") {
	case "true", "1":
		t := true
		processed = &t
	case "false", "0":
		f := false
		processed = &f
	}
	scope, err := authz.ResolveUserDataScopeFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "unable to resolve data scope")
		return
	}
	filter := dao.LoginRiskEventFilter{
		Username:  c.Query("username"),
		IP:        c.Query("ip"),
		Reason:    c.Query("reason"),
		Processed: processed,
		DataScope: scope,
	}
	req := pagination.GetPageRequest(c)
	events, total, err := a.riskDAO.ListContext(c.Request.Context(), req, filter)
	if err != nil {
		internalServerError(c, "failed to list login risk events", err)
		return
	}
	response.PageSuccess(c, events, total, req.Page, req.PageSize)
}

// ProcessLoginRiskEvent POST /login-risk-events/:id/process marks an event as
// handled by the current operator.
func (a *LoginRiskEventAPI) ProcessLoginRiskEvent(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid event id")
		return
	}
	scope, err := authz.ResolveUserDataScopeFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "unable to resolve operator")
		return
	}
	if err := a.riskDAO.MarkProcessedContext(c.Request.Context(), uint(id), scope.UserID); err != nil {
		internalServerError(c, "failed to mark login risk event", err)
		return
	}
	response.Success(c, gin.H{"id": id})
}
