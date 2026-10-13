package api

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/crm/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/response"
	"net/http"
	"strconv"
	"time"
)

type followReq struct {
	TargetType string  `json:"target_type"`
	TargetID   uint64  `json:"target_id"`
	Content    string  `json:"content"`
	NextAt     *string `json:"next_at"` // "2026-08-01 10:00" 或 "2026-08-01"
}

func parseDateTime(s *string) *time.Time {
	if s == nil || *s == "" {
		return nil
	}
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, *s, time.Local); err == nil {
			return &t
		}
	}
	return nil
}

func (s *Server) CreateFollowUp(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	var req followReq
	if err := c.ShouldBindJSON(&req); err != nil || req.TargetID == 0 {
		response.Error(c, http.StatusBadRequest, "invalid body")
		return
	}
	f := model.FollowUp{
		TenantID: u.TenantID, TargetType: req.TargetType, TargetID: req.TargetID,
		Content: req.Content, NextAt: parseDateTime(req.NextAt), ByUserID: u.UserID,
	}
	if err := s.Store.CreateFollowUp(c.Request.Context(), &f); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, f)
}

func (s *Server) ListFollowUps(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	targetID, _ := strconv.ParseUint(c.Query("target_id"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	list, err := s.Store.ListFollowUps(u.TenantID, c.Query("target_type"), targetID, limit)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, gin.H{"list": list})
}
