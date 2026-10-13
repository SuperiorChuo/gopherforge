package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// 时间路由/响铃组代理：认证 + 粗校验后透传 control-api（细校验在对端）。
// 二者都是 FS 侧全局拨号计划配置（无租户归属），列查删改一律仅平台管理员
// （fsProxyJSON / fsProxyDelete / fsAdminProxyGet 内置闸门）。

// ---- 营业时间路由 ----

func (s *Server) FSTimeRoutes(c *gin.Context) { s.fsAdminProxyGet(c, "/v1/time-routes") }

func (s *Server) FSCreateTimeRoute(c *gin.Context) {
	s.fsProxyJSON(c, http.MethodPost, "/v1/time-routes")
}

func (s *Server) FSUpdateTimeRoute(c *gin.Context) {
	name := c.Param("name")
	if !fsQueueNameRe.MatchString(name) {
		fail(c, http.StatusBadRequest, "invalid name")
		return
	}
	s.fsProxyJSON(c, http.MethodPut, "/v1/time-routes/"+name)
}

func (s *Server) FSDeleteTimeRoute(c *gin.Context) {
	name := c.Param("name")
	if !fsQueueNameRe.MatchString(name) {
		fail(c, http.StatusBadRequest, "invalid name")
		return
	}
	s.fsProxyDelete(c, "/v1/time-routes/"+name)
}

// ---- 响铃组 ----

func (s *Server) FSRingGroups(c *gin.Context) { s.fsAdminProxyGet(c, "/v1/ring-groups") }

func (s *Server) FSCreateRingGroup(c *gin.Context) {
	s.fsProxyJSON(c, http.MethodPost, "/v1/ring-groups")
}

func (s *Server) FSUpdateRingGroup(c *gin.Context) {
	name := c.Param("name")
	if !fsQueueNameRe.MatchString(name) {
		fail(c, http.StatusBadRequest, "invalid name")
		return
	}
	s.fsProxyJSON(c, http.MethodPut, "/v1/ring-groups/"+name)
}

func (s *Server) FSDeleteRingGroup(c *gin.Context) {
	name := c.Param("name")
	if !fsQueueNameRe.MatchString(name) {
		fail(c, http.StatusBadRequest, "invalid name")
		return
	}
	s.fsProxyDelete(c, "/v1/ring-groups/"+name)
}
