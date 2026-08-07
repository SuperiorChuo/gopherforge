package system

import (
	"github.com/gin-gonic/gin"
	dao "github.com/go-admin-kit/services/audit/internal/dao/system"
	"github.com/go-admin-kit/services/shared/pkg/pagination"
	"github.com/go-admin-kit/services/shared/pkg/response"
	"gorm.io/gorm"
)

// SecurityEventAPI exposes detected audit anomalies.
type SecurityEventAPI struct {
	eventDAO dao.SecurityEventDAO
}

func NewSecurityEventAPIWithDB(db *gorm.DB) *SecurityEventAPI {
	return &SecurityEventAPI{eventDAO: *dao.NewSecurityEventDAO(db)}
}

func (a *SecurityEventAPI) ListSecurityEvents(c *gin.Context) {
	req := pagination.GetPageRequest(c)
	filter := dao.SecurityEventFilter{
		Rule:     c.Query("rule"),
		Severity: c.Query("severity"),
	}
	events, total, err := a.eventDAO.ListContext(c.Request.Context(), req, filter)
	if err != nil {
		internalServerError(c, "failed to list security events", err)
		return
	}
	response.PageSuccess(c, events, total, req.Page, req.PageSize)
}
