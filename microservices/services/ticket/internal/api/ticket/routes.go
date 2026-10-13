package api

import (
	"github.com/gin-gonic/gin"
	sharedaudit "github.com/go-admin-kit/services/shared/pkg/audittrail"
	"github.com/go-admin-kit/services/shared/pkg/gatewayauth"
)

// RegisterRoutes mounts all ticket routes.
// Gateway auth-verify intercepts at Traefik; requireUser is a bare-connect fallback.
func (s *Server) RegisterRoutes(r *gin.Engine) {
	r.GET("/api/v1/health/live", s.HealthLive)
	r.GET("/api/v1/health/ready", s.HealthReady)
	r.GET("/api/v1/ticket/health/live", s.HealthLive)
	r.GET("/api/v1/ticket/health/ready", s.HealthReady)

	r.Use(gatewayauth.RequireReadWritePermissions("ticket:read", "ticket:write"))
	r.Use(sharedaudit.AuditHeaderMiddleware())

	r.GET("/api/v1/ticket/stats", s.Stats)
	r.GET("/api/v1/ticket/sla", s.SLAPolicy)

	r.GET("/api/v1/ticket/tickets", s.ListTickets)
	r.POST("/api/v1/ticket/tickets", s.CreateTicket)
	r.GET("/api/v1/ticket/tickets/:id", s.GetTicket)
	r.PUT("/api/v1/ticket/tickets/:id", s.UpdateTicket)
	r.POST("/api/v1/ticket/tickets/:id/status", s.ChangeStatus)
	r.POST("/api/v1/ticket/tickets/:id/assign", s.Assign)
	r.POST("/api/v1/ticket/tickets/:id/comments", s.AddComment)
	r.GET("/api/v1/ticket/tickets/:id/events", s.ListEvents)
}
