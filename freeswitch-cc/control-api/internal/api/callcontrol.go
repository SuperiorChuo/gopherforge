package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/extdir"
	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/store"
	"github.com/gin-gonic/gin"
)

// 呼叫控制（FS-M2.5）：给中台的结构化端点，不走 /v1/esl/api 白名单。
// 所有入参先正则校验再拼 ESL 命令，防命令注入。

var (
	numberRe  = regexp.MustCompile(`^[0-9*#+]{2,32}$`)
	uuidRe    = regexp.MustCompile(`^[0-9a-fA-F-]{8,64}$`)
	causeRe   = regexp.MustCompile(`^[A-Z_]{3,64}$`)
	gatewayRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
)

// showJSON runs `show <what> as json` and returns rows (FS omits the rows
// key when row_count is 0).
func (s *Server) showJSON(c *gin.Context, what string) {
	out, err := s.ESL.API("show " + what + " as json")
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	var parsed struct {
		RowCount int              `json:"row_count"`
		Rows     []map[string]any `json:"rows"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("parse %s output: %v", what, err)})
		return
	}
	if parsed.Rows == nil {
		parsed.Rows = []map[string]any{}
	}
	c.JSON(http.StatusOK, gin.H{"total": parsed.RowCount, "list": parsed.Rows})
}

// listRegistrations handles GET /v1/registrations — 谁的分机在线。
func (s *Server) listRegistrations(c *gin.Context) {
	s.showJSON(c, "registrations")
}

// listChannels handles GET /v1/channels — 当前活动通话通道。
func (s *Server) listChannels(c *gin.Context) {
	s.showJSON(c, "channels")
}

type originateReq struct {
	FromExt string `json:"from_ext" binding:"required"`
	To      string `json:"to" binding:"required"`
	// Gateway 可选：指定 sofia 网关名（SIP 线路）直接桥接外呼（CRM 选线路）；
	// 为空保持原行为——接起后按 default 拨号计划路由。
	Gateway string `json:"gateway"`
	// ExecuteOnAnswer 接通后执行的 app（预付费限时挂断：cc-service 按余额算出
	// 最大通话秒数后传 `sched_hangup +N alloted_timeout`）。白名单校验防注入。
	ExecuteOnAnswer string `json:"execute_on_answer"`
	// TenantID 发起方租户（转售计费）：>0 时注入 cc_tenant_id 通道变量，
	// 随 CDR/事件回中台按租户入账。纯数字无注入面。
	TenantID uint64 `json:"tenant_id"`
}

// eoaRe 只放行预付费限时挂断这一种形态，其余一律拒绝（ESL 注入面收敛）。
var eoaRe = regexp.MustCompile(`^sched_hangup \+[0-9]{1,6} [a-z_]{1,32}$`)

// originateCall handles POST /v1/calls/originate：点击外呼——先振铃坐席分机，
// 接起后按 default 拨号计划路由被叫（内线/队列/回声均可）。bgapi 立即返回，
// 结果以 CHANNEL 事件回流话单。
func (s *Server) originateCall(c *gin.Context) {
	var req originateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from_ext and to required"})
		return
	}
	if !numberRe.MatchString(req.FromExt) || !numberRe.MatchString(req.To) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid extension or number"})
		return
	}
	extraVars := ""
	if req.ExecuteOnAnswer != "" {
		if !eoaRe.MatchString(req.ExecuteOnAnswer) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid execute_on_answer"})
			return
		}
		// 带空格的变量值必须单引号包（既有教训）
		extraVars = fmt.Sprintf(",execute_on_answer='%s'", req.ExecuteOnAnswer)
	}
	if req.TenantID > 0 {
		extraVars += fmt.Sprintf(",cc_tenant_id=%d", req.TenantID)
	}
	extraVars += ",cc_risk_checked=1"
	var cmd string
	if req.Gateway != "" {
		if !gatewayRe.MatchString(req.Gateway) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid gateway"})
			return
		}
		// 指定线路：坐席接起后经 sofia 网关桥接被叫
		cmd = fmt.Sprintf(
			"originate {origination_caller_id_number=%s,origination_caller_id_name=cc-crm,originate_timeout=30%s}user/%s &bridge([cc_early_media=1]sofia/gateway/%s/%s)",
			req.To, extraVars, req.FromExt, req.Gateway, req.To)
	} else {
		cmd = fmt.Sprintf(
			"originate {origination_caller_id_number=%s,origination_caller_id_name=cc-console,originate_timeout=30%s}user/%s %s XML default",
			req.To, extraVars, req.FromExt, req.To)
	}
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

type announceReq struct {
	To       string `json:"to" binding:"required"`
	File     string `json:"file" binding:"required"`
	Gateway  string `json:"gateway"`
	TenantID uint64 `json:"tenant_id"`
	VoiceID  uint64 `json:"voice_id"`
}

var announceFileRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}\.wav$`)

// announceCall handles POST /v1/calls/announce：TTS/语音通知——不振铃坐席，
// 直接经网关呼叫被叫，接通后 playback 提示音。网关可显式指定，否则 LCR 首选。
func (s *Server) announceCall(c *gin.Context) {
	var req announceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "to and file required"})
		return
	}
	if !numberRe.MatchString(req.To) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid number"})
		return
	}
	if !announceFileRe.MatchString(req.File) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file"})
		return
	}
	if req.Gateway != "" && !gatewayRe.MatchString(req.Gateway) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid gateway"})
		return
	}
	src := s.gateways()
	if src == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gateway store unavailable"})
		return
	}
	g, err := s.pickAnnounceGateway(src, req.To, req.Gateway)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	dest, cid := announceDest(*g, req.To)
	if !numberRe.MatchString(dest) || !numberRe.MatchString(cid) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rewritten number"})
		return
	}
	extra := ""
	if req.TenantID > 0 {
		extra += fmt.Sprintf(",cc_tenant_id=%d", req.TenantID)
	}
	if req.VoiceID > 0 {
		extra += fmt.Sprintf(",cc_voice_id=%d", req.VoiceID)
	}
	cmd := fmt.Sprintf(
		"originate {ignore_early_media=true,origination_caller_id_number=%s,origination_caller_id_name=cc-notify,originate_timeout=45,cc_announce=1,cc_early_media=1%s}sofia/gateway/%s/%s &playback(/etc/freeswitch/prompts/%s)",
		cid, extra, g.Name, dest, req.File)
	reply, err := s.ESL.Bgapi(cmd)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if strings.HasPrefix(reply, "-ERR") {
		c.JSON(http.StatusBadGateway, gin.H{"error": reply})
		return
	}
	c.JSON(http.StatusOK, gin.H{"queued": true, "reply": reply, "gateway": g.Name, "dest": dest})
}

func (s *Server) pickAnnounceGateway(src gatewaySource, number, named string) (*store.Gateway, error) {
	if named != "" {
		g, err := src.GetGateway(named)
		if err != nil {
			return nil, fmt.Errorf("gateway not found")
		}
		if g.Disabled {
			return nil, fmt.Errorf("gateway disabled")
		}
		return g, nil
	}
	all, err := src.ListGateways()
	if err != nil {
		return nil, err
	}
	rows := extdir.LCRPreview(all, number)
	for _, row := range rows {
		g, err := src.GetGateway(row.Gateway)
		if err != nil || g.Disabled {
			continue
		}
		return g, nil
	}
	return nil, fmt.Errorf("no outbound route")
}

func announceDest(g store.Gateway, number string) (dest, cid string) {
	dest = number
	if g.DialPrefix != "" && strings.HasPrefix(number, g.DialPrefix) && len(number) > len(g.DialPrefix) {
		dest = g.Prepend + number[len(g.DialPrefix):]
	} else if g.Prepend != "" {
		dest = g.Prepend + number
	}
	cid = g.CallerNumber
	if cid == "" {
		cid = dest
	}
	return dest, cid
}

type hangupReq struct {
	CallID string `json:"call_id" binding:"required"`
	Cause  string `json:"cause"`
}

// hangupCall handles POST /v1/calls/hangup — 强拆一路通道（uuid_kill）。
func (s *Server) hangupCall(c *gin.Context) {
	var req hangupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "call_id required"})
		return
	}
	if !uuidRe.MatchString(req.CallID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid call_id"})
		return
	}
	cause := req.Cause
	if cause == "" {
		cause = "MANAGER_REQUEST"
	} else if !causeRe.MatchString(cause) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cause"})
		return
	}
	out, err := s.ESL.API(fmt.Sprintf("uuid_kill %s %s", req.CallID, cause))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if strings.HasPrefix(out, "-ERR") {
		c.JSON(http.StatusBadGateway, gin.H{"error": strings.TrimSpace(out)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "reply": strings.TrimSpace(out)})
}
