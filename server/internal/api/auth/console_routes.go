package auth

import (
	"errors"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/response"
	authSvc "github.com/go-admin-kit/server/internal/service/auth"
	systemSvc "github.com/go-admin-kit/server/internal/service/system"
)

func (a *UserAPI) ListConsoleRoutes(c *gin.Context) {
	routes, err := a.consoleRouteService.ListRoutesContext(c.Request.Context())
	if err != nil {
		internalServerError(c, "failed to list console routes", err)
		return
	}
	response.Success(c, gin.H{"items": routes})
}

func (a *UserAPI) CreateConsoleRoute(c *gin.Context) {
	var req authSvc.ConsoleRouteCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, invalidRequestBodyMessage)
		return
	}

	route, err := a.consoleRouteService.CreateRouteContext(c.Request.Context(), req)
	if err != nil {
		respondConsoleRouteError(c, err)
		return
	}
	a.recordConsoleRouteAudit(c, "console_route.create", route.RouteKey, nil, authSvc.ConsoleRouteSnapshot(route))
	response.Success(c, route)
}

func (a *UserAPI) GetConsoleRoute(c *gin.Context) {
	route, err := a.consoleRouteService.GetRouteContext(c.Request.Context(), c.Param("route_key"))
	if err != nil {
		respondConsoleRouteError(c, err)
		return
	}
	response.Success(c, route)
}

func (a *UserAPI) UpdateConsoleRoute(c *gin.Context) {
	var req authSvc.ConsoleRouteUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, invalidRequestBodyMessage)
		return
	}

	routeKey := c.Param("route_key")
	before, err := a.consoleRouteService.GetRouteContext(c.Request.Context(), routeKey)
	if err != nil {
		respondConsoleRouteError(c, err)
		return
	}
	route, err := a.consoleRouteService.UpdateRouteContext(c.Request.Context(), routeKey, req)
	if err != nil {
		respondConsoleRouteError(c, err)
		return
	}
	a.recordConsoleRouteAudit(c, "console_route.update", route.RouteKey, authSvc.ConsoleRouteSnapshot(before), authSvc.ConsoleRouteSnapshot(route))
	response.Success(c, route)
}

func (a *UserAPI) DeleteConsoleRoute(c *gin.Context) {
	before, err := a.consoleRouteService.DeleteRouteContext(c.Request.Context(), c.Param("route_key"))
	if err != nil {
		respondConsoleRouteError(c, err)
		return
	}
	a.recordConsoleRouteAudit(c, "console_route.delete", before.RouteKey, authSvc.ConsoleRouteSnapshot(before), nil)
	response.Success(c, gin.H{"deleted": true, "route_key": before.RouteKey})
}

func respondConsoleRouteError(c *gin.Context, err error) {
	var validationErr authSvc.ConsoleRouteValidationError
	switch {
	case errors.Is(err, authSvc.ErrConsoleRouteNotFound):
		response.NotFound(c, err.Error())
	case errors.As(err, &validationErr):
		response.BadRequest(c, validationErr.Error())
	default:
		internalServerError(c, "failed to process console route", err)
	}
}

func (a *UserAPI) recordConsoleRouteAudit(c *gin.Context, action, targetID string, before, after map[string]any) {
	_ = a.auditService.Record(c, systemSvc.AuditRecordRequest{
		Action:     action,
		TargetType: "console_route",
		TargetID:   targetID,
		Before:     before,
		After:      after,
		Summary:    fmt.Sprintf("%s console route %s", consoleRouteAuditVerb(action), targetID),
	})
}

func consoleRouteAuditVerb(action string) string {
	switch action {
	case "console_route.create":
		return "Created"
	case "console_route.update":
		return "Updated"
	case "console_route.delete":
		return "Deleted"
	default:
		return "Changed"
	}
}
