package api

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/response"
	"net/http"
	"strconv"
	"time"
)

// LookupPhone handles GET /api/v1/crm/lookup?phone=.
func (s *Server) LookupPhone(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	res, err := s.Store.LookupByPhone(u.TenantID, c.Query("phone"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, res)
}

func (s *Server) Stats(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	if days <= 0 || days > 365 {
		days = 30
	}
	stats, err := s.Store.Stats(c.Request.Context(), u.TenantID, time.Now().AddDate(0, 0, -days))
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, gin.H{"stats": stats, "days": days})
}

// CustomerTimeline returns the activity timeline for a customer.
func (s *Server) CustomerTimeline(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	events, err := s.Store.CustomerTimeline(c.Request.Context(), u.TenantID, id)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, gin.H{"events": events})
}
