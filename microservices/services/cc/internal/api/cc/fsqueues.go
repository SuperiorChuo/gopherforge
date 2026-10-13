package api

import (
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"
)

// 队列/质检/MOH/黑名单代理：认证 + 粗校验后透传 control-api（细校验在对端）。
// 越权收口：队列/MOH/黑名单/监控台等在 FS 侧是全局配置、无租户归属，列查删改
// 一律仅平台管理员（fsAdminProxy*）；坐席工作流端点（签入签出/监听/通话录音）
// 按分机或通话的租户归属校验后放行。

var fsQueueNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,31}$`)

// fsProxyJSON 统一的"绑 JSON→透传"代理（全局配置写操作，仅平台管理员）。
func (s *Server) fsProxyJSON(c *gin.Context, method, path string) {
	if _, authorized := s.requirePlatformAdmin(c); !authorized {
		return
	}
	if !s.FS.Enabled() {
		fail(c, http.StatusBadRequest, "CC_FS_API_BASE is not configured")
		return
	}
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	var (
		out map[string]any
		err error
	)
	switch method {
	case http.MethodPut:
		out, err = s.FS.Put(c.Request.Context(), path, body)
	default:
		out, err = s.FS.Post(c.Request.Context(), path, body)
	}
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	ok(c, out)
}

// fsProxyDelete 全局配置删除代理（仅平台管理员）。
func (s *Server) fsProxyDelete(c *gin.Context, path string) {
	if _, authorized := s.requirePlatformAdmin(c); !authorized {
		return
	}
	s.fsProxyDeleteAuthed(c, path)
}

// fsProxyDeleteAuthed 同 fsProxyDelete 但调用方已完成鉴权/归属校验。
func (s *Server) fsProxyDeleteAuthed(c *gin.Context, path string) {
	if !s.FS.Enabled() {
		fail(c, http.StatusBadRequest, "CC_FS_API_BASE is not configured")
		return
	}
	out, err := s.FS.Delete(c.Request.Context(), path)
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	ok(c, out)
}

// ---- 队列 ----

func (s *Server) FSCCQueues(c *gin.Context)      { s.fsAdminProxyGet(c, "/v1/cc-queues") }
func (s *Server) FSCreateCCQueue(c *gin.Context) { s.fsProxyJSON(c, http.MethodPost, "/v1/cc-queues") }

func (s *Server) FSUpdateCCQueue(c *gin.Context) {
	name := c.Param("name")
	if !fsQueueNameRe.MatchString(name) {
		fail(c, http.StatusBadRequest, "invalid name")
		return
	}
	s.fsProxyJSON(c, http.MethodPut, "/v1/cc-queues/"+name)
}

func (s *Server) FSDeleteCCQueue(c *gin.Context) {
	name := c.Param("name")
	if !fsQueueNameRe.MatchString(name) {
		fail(c, http.StatusBadRequest, "invalid name")
		return
	}
	s.fsProxyDelete(c, "/v1/cc-queues/"+name)
}

func (s *Server) FSUpsertQueueAgent(c *gin.Context) {
	name := c.Param("name")
	if !fsQueueNameRe.MatchString(name) {
		fail(c, http.StatusBadRequest, "invalid name")
		return
	}
	s.fsProxyJSON(c, http.MethodPut, "/v1/cc-queues/"+name+"/agents")
}

func (s *Server) FSRemoveQueueAgent(c *gin.Context) {
	name, ext := c.Param("name"), c.Param("ext")
	if !fsQueueNameRe.MatchString(name) || !fsNumberRe.MatchString(ext) {
		fail(c, http.StatusBadRequest, "invalid name or ext")
		return
	}
	s.fsProxyDelete(c, "/v1/cc-queues/"+name+"/agents/"+ext)
}

// FSSetAgentStatus handles POST /api/v1/cc/fs/agent-status — 签入/示忙/签出。
// 坐席工作流端点：校验目标分机绑定在本租户后放行（平台管理员任意分机）。
func (s *Server) FSSetAgentStatus(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	if !s.FS.Enabled() {
		fail(c, http.StatusBadRequest, "CC_FS_API_BASE is not configured")
		return
	}
	var body struct {
		Ext    string `json:"ext"`
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || !fsNumberRe.MatchString(body.Ext) {
		fail(c, http.StatusBadRequest, "invalid ext")
		return
	}
	if !s.extensionInTenant(c, agent, body.Ext) {
		return
	}
	out, err := s.FS.Post(c.Request.Context(), "/v1/cc-agents/status", gin.H{
		"ext": body.Ext, "status": body.Status,
	})
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	ok(c, out)
}

// FSMonitor handles GET /api/v1/cc/fs/monitor — 班长话务监控台聚合（队列+坐席+
// 通道，全平台数据，仅平台管理员）。
func (s *Server) FSMonitor(c *gin.Context) { s.fsAdminProxyGet(c, "/v1/cc-monitor") }

// ---- 质检 ----

// FSSupervise handles POST /api/v1/cc/fs/supervise — 监听/耳语/强插。
// 校验被监听通话属于本租户且班长分机绑定在本租户（平台管理员全放）。
func (s *Server) FSSupervise(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	if !s.FS.Enabled() {
		fail(c, http.StatusBadRequest, "CC_FS_API_BASE is not configured")
		return
	}
	var body struct {
		SupervisorExt string `json:"supervisor_ext"`
		CallID        string `json:"call_id"`
		Mode          string `json:"mode"`
	}
	if err := c.ShouldBindJSON(&body); err != nil ||
		!fsNumberRe.MatchString(body.SupervisorExt) || !fsUUIDRe.MatchString(body.CallID) {
		fail(c, http.StatusBadRequest, "invalid supervisor_ext or call_id")
		return
	}
	if !s.callInTenant(c, agent, body.CallID) {
		return
	}
	if !s.extensionInTenant(c, agent, body.SupervisorExt) {
		return
	}
	out, err := s.FS.Post(c.Request.Context(), "/v1/calls/supervise", gin.H{
		"supervisor_ext": body.SupervisorExt, "call_id": body.CallID, "mode": body.Mode,
	})
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	ok(c, out)
}

// FSUUIDRecord handles POST /api/v1/cc/fs/uuid-record — 对进行中的通话开/停录音。
// 校验通话属于本租户（平台管理员全放）。
func (s *Server) FSUUIDRecord(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	if !s.FS.Enabled() {
		fail(c, http.StatusBadRequest, "CC_FS_API_BASE is not configured")
		return
	}
	var body struct {
		CallID string `json:"call_id"`
		Action string `json:"action"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || !fsUUIDRe.MatchString(body.CallID) {
		fail(c, http.StatusBadRequest, "invalid call_id")
		return
	}
	if !s.callInTenant(c, agent, body.CallID) {
		return
	}
	out, err := s.FS.Post(c.Request.Context(), "/v1/calls/record", gin.H{
		"call_id": body.CallID, "action": body.Action,
	})
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	ok(c, out)
}

// ---- MOH ----
// 排队音乐是全局资源（无租户归属），列查删改一律仅平台管理员。

func (s *Server) FSMOHList(c *gin.Context) { s.fsAdminProxyGet(c, "/v1/moh") }

func (s *Server) FSMOHUpload(c *gin.Context) {
	if _, authorized := s.requirePlatformAdmin(c); !authorized {
		return
	}
	if !s.FS.Enabled() {
		fail(c, http.StatusBadRequest, "CC_FS_API_BASE is not configured")
		return
	}
	out, err := s.FS.PostMultipart(c.Request.Context(), "/v1/moh", c.GetHeader("Content-Type"), c.Request.Body)
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	ok(c, out)
}

func (s *Server) FSMOHFile(c *gin.Context) {
	if _, authorized := s.requirePlatformAdmin(c); !authorized {
		return
	}
	if !s.FS.Enabled() {
		fail(c, http.StatusBadRequest, "CC_FS_API_BASE is not configured")
		return
	}
	file := c.Param("file")
	if !fsMOHFileRe.MatchString(file) {
		fail(c, http.StatusBadRequest, "invalid file name")
		return
	}
	resp, err := s.FS.Stream(c.Request.Context(), "/v1/moh/"+file)
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()
	c.DataFromReader(resp.StatusCode, resp.ContentLength, resp.Header.Get("Content-Type"), resp.Body, map[string]string{
		"Content-Disposition": resp.Header.Get("Content-Disposition"),
	})
}

var fsMOHFileRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}\.(wav|mp3)$`)

func (s *Server) FSMOHDelete(c *gin.Context) {
	file := c.Param("file")
	if !fsMOHFileRe.MatchString(file) {
		fail(c, http.StatusBadRequest, "invalid file name")
		return
	}
	s.fsProxyDelete(c, "/v1/moh/"+file)
}

// ---- 黑名单 ----
// 全局呼入黑名单（无租户归属），仅平台管理员。

var fsBLNumberRe = regexp.MustCompile(`^\+?[0-9*#]{3,32}$`)

func (s *Server) FSBlacklist(c *gin.Context)    { s.fsAdminProxyGet(c, "/v1/blacklist") }
func (s *Server) FSBlacklistAdd(c *gin.Context) { s.fsProxyJSON(c, http.MethodPost, "/v1/blacklist") }

func (s *Server) FSBlacklistRemove(c *gin.Context) {
	number := c.Param("number")
	if !fsBLNumberRe.MatchString(number) {
		fail(c, http.StatusBadRequest, "invalid number")
		return
	}
	s.fsProxyDelete(c, "/v1/blacklist/"+number)
}
