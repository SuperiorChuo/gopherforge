package api

import (
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"
)

// 会议室代理：认证 + 粗校验后透传 control-api（细校验在对端）。会议室是 FS 侧
// 全局配置（无租户归属），列查删改与操控一律仅平台管理员（fsProxyJSON /
// fsProxyDelete / fsAdminProxyGet 内置闸门）。
// 名字既覆盖固定会议室（support）也覆盖动态会议室（dyn-3901），故用同款
// ^[a-z0-9][a-z0-9_-]{1,31}$（含 - 已能匹配动态名）。
var fsConfNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,31}$`)

// ---- 会议室 ----

func (s *Server) FSConferences(c *gin.Context) { s.fsAdminProxyGet(c, "/v1/conferences") }

func (s *Server) FSCreateConference(c *gin.Context) {
	s.fsProxyJSON(c, http.MethodPost, "/v1/conferences")
}

func (s *Server) FSUpdateConference(c *gin.Context) {
	name := c.Param("name")
	if !fsConfNameRe.MatchString(name) {
		fail(c, http.StatusBadRequest, "invalid name")
		return
	}
	s.fsProxyJSON(c, http.MethodPut, "/v1/conferences/"+name)
}

func (s *Server) FSDeleteConference(c *gin.Context) {
	name := c.Param("name")
	if !fsConfNameRe.MatchString(name) {
		fail(c, http.StatusBadRequest, "invalid name")
		return
	}
	s.fsProxyDelete(c, "/v1/conferences/"+name)
}

func (s *Server) FSConferenceAction(c *gin.Context) {
	name := c.Param("name")
	if !fsConfNameRe.MatchString(name) {
		fail(c, http.StatusBadRequest, "invalid name")
		return
	}
	s.fsProxyJSON(c, http.MethodPost, "/v1/conferences/"+name+"/action")
}
