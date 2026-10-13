package api

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/cc/internal/store/cc"
)

// 分机号沿用 FreeSWITCH directory 的宽松形态：字母数字与 ._-，2~32 位。
var extensionPattern = regexp.MustCompile(`^[0-9A-Za-z._-]{2,32}$`)

// ListAgents handles GET /api/v1/cc/agents.
func (s *Server) ListAgents(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	list, err := s.Store.ListAgents(c.Request.Context(), agent.TenantID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "list agents failed")
		return
	}
	ok(c, gin.H{"list": list, "total": len(list)})
}

type bindAgentReq struct {
	Extension   string `json:"extension"`
	DisplayName string `json:"display_name"`
}

// BindAgent handles PUT /api/v1/cc/agents/:user_id — 建立/更新用户与分机的绑定。
func (s *Server) BindAgent(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	userID, err := strconv.ParseUint(c.Param("user_id"), 10, 64)
	if err != nil || userID == 0 {
		fail(c, http.StatusBadRequest, "invalid user_id")
		return
	}
	var req bindAgentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	req.Extension = strings.TrimSpace(req.Extension)
	if !extensionPattern.MatchString(req.Extension) {
		fail(c, http.StatusBadRequest, "extension must be 2-32 chars of [0-9A-Za-z._-]")
		return
	}
	if len(req.DisplayName) > 64 {
		fail(c, http.StatusBadRequest, "display_name too long")
		return
	}

	bound, err := s.Store.BindAgent(c.Request.Context(), &store.Agent{
		TenantID:    agent.TenantID,
		UserID:      userID,
		Extension:   req.Extension,
		DisplayName: strings.TrimSpace(req.DisplayName),
	})
	switch {
	case errors.Is(err, store.ErrExtensionTaken):
		fail(c, http.StatusConflict, "extension already bound to another user")
		return
	case err != nil:
		fail(c, http.StatusInternalServerError, "bind agent failed")
		return
	}
	ok(c, bound)
}

// UnbindAgent handles DELETE /api/v1/cc/agents/:user_id.
func (s *Server) UnbindAgent(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	userID, err := strconv.ParseUint(c.Param("user_id"), 10, 64)
	if err != nil || userID == 0 {
		fail(c, http.StatusBadRequest, "invalid user_id")
		return
	}
	removed, err := s.Store.UnbindAgent(c.Request.Context(), agent.TenantID, userID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "unbind agent failed")
		return
	}
	if !removed {
		fail(c, http.StatusNotFound, "binding not found")
		return
	}
	ok(c, gin.H{"removed": true, "user_id": userID})
}
