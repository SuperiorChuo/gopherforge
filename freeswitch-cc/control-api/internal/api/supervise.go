package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// 班长质检：监听（eavesdrop）与通话中录音开关（uuid_record）。
// 监听实现：originate 班长分机，接起后对目标通道执行 eavesdrop——
// 班长话机上按 DTMF 切模式：1=只对坐席说（耳语）、2=对客户说、3=三方强插、0=静默。

type superviseReq struct {
	// SupervisorExt 班长分机（先振铃它）
	SupervisorExt string `json:"supervisor_ext" binding:"required"`
	// CallID 被监听的通道 uuid（活动通话列表里的 uuid）
	CallID string `json:"call_id" binding:"required"`
	// Mode 初始模式：listen(默认静默) | whisper(耳语坐席) | barge(强插)
	Mode string `json:"mode"`
}

// superviseCall handles POST /v1/calls/supervise.
func (s *Server) superviseCall(c *gin.Context) {
	var req superviseReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "supervisor_ext and call_id required"})
		return
	}
	if !numberRe.MatchString(req.SupervisorExt) || !uuidRe.MatchString(req.CallID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid supervisor_ext or call_id"})
		return
	}
	// eavesdrop_bridge_aleg/bleg 控制初始进场模式；DTMF 0-3 运行时切换
	var vars string
	switch req.Mode {
	case "", "listen":
		vars = ""
	case "whisper":
		vars = "eavesdrop_whisper_bleg=true," // b-leg = 坐席侧
	case "barge":
		vars = "eavesdrop_bridge_aleg=true,eavesdrop_bridge_bleg=true,"
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "mode must be listen/whisper/barge"})
		return
	}
	cmd := fmt.Sprintf(
		"originate {%sorigination_caller_id_name=supervisor,origination_caller_id_number=%s}user/%s &eavesdrop(%s)",
		vars, req.SupervisorExt, req.SupervisorExt, req.CallID)
	reply, err := s.ESL.Bgapi(cmd)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if strings.HasPrefix(reply, "-ERR") {
		c.JSON(http.StatusBadGateway, gin.H{"error": reply})
		return
	}
	c.JSON(http.StatusOK, gin.H{"queued": true, "reply": reply})
}

type uuidRecordReq struct {
	CallID string `json:"call_id" binding:"required"`
	Action string `json:"action" binding:"required"` // start | stop
}

// uuidRecord handles POST /v1/calls/record — 对进行中的通话开/停录音。
func (s *Server) uuidRecord(c *gin.Context) {
	var req uuidRecordReq
	if err := c.ShouldBindJSON(&req); err != nil || !uuidRe.MatchString(req.CallID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "call_id and action required"})
		return
	}
	if req.Action != "start" && req.Action != "stop" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "action must be start/stop"})
		return
	}
	// 手动录音文件带 manual- 前缀区分自动录音
	path := fmt.Sprintf("/var/lib/freeswitch/recordings/manual-%s.wav", req.CallID)
	out, err := s.ESL.API(fmt.Sprintf("uuid_record %s %s %s", req.CallID, req.Action, path))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if strings.HasPrefix(out, "-ERR") {
		c.JSON(http.StatusBadGateway, gin.H{"error": strings.TrimSpace(out)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "file": "manual-" + req.CallID + ".wav"})
}
