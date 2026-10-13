package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/config"
	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/esl"
	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/metrics"
	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/store"
	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/webhook"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Server struct {
	Cfg      config.Config
	ESL      *esl.Client
	Store    *store.Store
	Hook     *webhook.Client
	Listener *esl.Listener
	// ExtLookup 覆盖 fs-xml directory 查询的分机来源（测试注入用）；
	// 生产留空时回落到 Store.GetExtension。
	ExtLookup extGetter
	// GwSource 覆盖 announce 选路用的网关来源（测试注入）；生产留空回落 Store。
	GwSource gatewaySource
	// Metrics 非空时暴露 /metrics（Prometheus 采集）
	Metrics *metrics.Metrics
	// killChannel 覆盖 uuid_kill（单测注入）；生产走 ESL.API。
	killChannel func(uuid string) error
	riskHTTP    *http.Client
	// earlyMedia 只管理显式标记的出局早媒体采样；测试可注入命令与 HTTP。
	earlyMedia          *earlyMediaState
	earlyHTTP           *http.Client
	earlyCommand        func(string) error
	earlySampleDuration time.Duration // 单测覆盖；生产为 0 时读配置秒数
}

// extGetter 抽出 fs-xml 查分机所需的最小能力，便于单测注入内存实现，
// 无需真库/sqlite 驱动。*store.Store 天然满足。
type extGetter interface {
	GetExtension(ext string) (*store.Extension, error)
}

// gatewaySource 抽出 announce 选路所需的网关查询，便于单测注入。
type gatewaySource interface {
	ListGateways() ([]store.Gateway, error)
	GetGateway(name string) (*store.Gateway, error)
}

func (s *Server) gateways() gatewaySource {
	if s.GwSource != nil {
		return s.GwSource
	}
	return s.Store
}

// extLookup 返回本次请求应使用的分机查询源。
func (s *Server) extLookup() extGetter {
	if s.ExtLookup != nil {
		return s.ExtLookup
	}
	return s.Store
}

func (s *Server) Router() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery(), gin.Logger())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "control-api"})
	})
	r.GET("/ready", s.ready)

	// mod_xml_curl 动态目录查询（FS 自带 BasicAuth，故挂在 token 鉴权组之外）
	r.POST("/v1/fs-xml", s.fsXML)
	// Prometheus 抓取端点：鉴权组外（Prometheus 不带 X-CC-Token；服务仅内网暴露）。
	if s.Metrics != nil {
		r.GET("/metrics", gin.WrapH(promhttp.HandlerFor(s.Metrics.Registry(), promhttp.HandlerOpts{})))
	}

	auth := r.Group("/")
	auth.Use(s.tokenAuth)
	{
		// 分机管理（extensions.go）
		auth.GET("/v1/extensions", s.listExtensions)
		auth.POST("/v1/extensions", s.createExtension)
		auth.PUT("/v1/extensions/:ext", s.updateExtension)
		auth.DELETE("/v1/extensions/:ext", s.deleteExtension)
		// 录音（recordings.go）
		auth.GET("/v1/recordings", s.listRecordings)
		auth.GET("/v1/recordings/:file", s.getRecording)
		// 语音信箱留言（voicemail.go）
		auth.GET("/v1/voicemails", s.listVoicemails)
		auth.GET("/v1/voicemails/:ext/:file", s.getVoicemail)
		auth.DELETE("/v1/voicemails/:ext/:file", s.deleteVoicemail)
		// SIP 网关/线路（gateways.go）
		auth.GET("/v1/gateways", s.listGateways)
		auth.POST("/v1/gateways", s.createGateway)
		auth.PUT("/v1/gateways/:name", s.updateGateway)
		auth.DELETE("/v1/gateways/:name", s.deleteGateway)
		// 租户线路批量停用/恢复（欠费冻结联动，gateways.go）
		auth.POST("/v1/tenants/:tenant_id/gateways/state", s.setTenantGatewaysState)
		// LCR 选路预览（纯内存计算，不打 FS）
		auth.GET("/v1/lcr-preview", s.lcrPreview)
		// IVR（ivrs.go）
		auth.GET("/v1/ivrs", s.listIVRs)
		auth.POST("/v1/ivrs", s.createIVR)
		auth.PUT("/v1/ivrs/:name", s.updateIVR)
		auth.DELETE("/v1/ivrs/:name", s.deleteIVR)
		// callcenter 队列（ccqueues.go）
		auth.GET("/v1/cc-queues", s.listCCQueues)
		auth.POST("/v1/cc-queues", s.createCCQueue)
		auth.PUT("/v1/cc-queues/:name", s.updateCCQueue)
		auth.DELETE("/v1/cc-queues/:name", s.deleteCCQueue)
		auth.PUT("/v1/cc-queues/:name/agents", s.upsertQueueAgent)
		auth.DELETE("/v1/cc-queues/:name/agents/:ext", s.removeQueueAgent)
		auth.POST("/v1/cc-agents/status", s.setAgentStatus)
		// 营业时间路由（timeroutes.go）
		auth.GET("/v1/time-routes", s.listTimeRoutes)
		auth.POST("/v1/time-routes", s.createTimeRoute)
		auth.PUT("/v1/time-routes/:name", s.updateTimeRoute)
		auth.DELETE("/v1/time-routes/:name", s.deleteTimeRoute)
		// 响铃组（ringgroups.go）
		auth.GET("/v1/ring-groups", s.listRingGroups)
		auth.POST("/v1/ring-groups", s.createRingGroup)
		auth.PUT("/v1/ring-groups/:name", s.updateRingGroup)
		auth.DELETE("/v1/ring-groups/:name", s.deleteRingGroup)
		// 班长话务监控台聚合（monitor.go）
		auth.GET("/v1/cc-monitor", s.ccMonitor)
		// 会议室（conferences.go）
		auth.GET("/v1/conferences", s.listConferences)
		auth.POST("/v1/conferences", s.createConference)
		auth.PUT("/v1/conferences/:name", s.updateConference)
		auth.DELETE("/v1/conferences/:name", s.deleteConference)
		auth.POST("/v1/conferences/:name/action", s.conferenceAction)
		// 班长质检（supervise.go）
		auth.POST("/v1/calls/supervise", s.superviseCall)
		auth.POST("/v1/calls/record", s.uuidRecord)
		// 坐席通话操控（callctl.go）：转移/保持/停泊/DTMF
		auth.POST("/v1/calls/transfer", s.transferCall)
		auth.POST("/v1/calls/hold", s.holdCall)
		auth.POST("/v1/calls/park", s.parkCall)
		auth.GET("/v1/parked", s.listParked)
		auth.POST("/v1/calls/dtmf", s.sendDTMF)
		// 排队音乐（moh.go）
		auth.GET("/v1/moh", s.listMOH)
		auth.POST("/v1/moh", s.uploadMOH)
		auth.GET("/v1/moh/:file", s.getMOH)
		auth.DELETE("/v1/moh/:file", s.deleteMOH)
		// 提示音库（prompts.go）
		auth.GET("/v1/prompts", s.listPrompts)
		auth.POST("/v1/prompts", s.uploadPrompt)
		auth.GET("/v1/prompts/:file", s.getPrompt)
		auth.DELETE("/v1/prompts/:file", s.deletePrompt)
		// 黑名单（blacklist.go）
		auth.GET("/v1/blacklist", s.listBlacklist)
		auth.POST("/v1/blacklist", s.addBlacklist)
		auth.DELETE("/v1/blacklist/:number", s.removeBlacklist)
		auth.GET("/v1/esl/status", s.eslStatus)
		auth.POST("/v1/esl/api", s.eslAPI)
		// 呼叫控制（callcontrol.go）
		auth.GET("/v1/registrations", s.listRegistrations)
		auth.GET("/v1/channels", s.listChannels)
		auth.POST("/v1/calls/originate", s.originateCall)
		auth.POST("/v1/calls/announce", s.announceCall)
		auth.POST("/v1/calls/hangup", s.hangupCall)
		auth.GET("/v1/cdr", s.listCDR)
		auth.POST("/v1/webhooks/test", s.webhookTest)
		auth.POST("/v1/events/ingest", s.ingestEvent)
	}
	return r
}

func (s *Server) tokenAuth(c *gin.Context) {
	tok := c.GetHeader("X-CC-Token")
	if tok == "" {
		tok = strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	}
	if s.Cfg.APIToken != "" && tok != s.Cfg.APIToken {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	c.Next()
}

func (s *Server) ready(c *gin.Context) {
	eslOK := true
	eslErr := ""
	if err := s.ESL.Ping(); err != nil {
		eslOK = false
		eslErr = err.Error()
	}
	dbOK := true
	if s.Store == nil {
		dbOK = false
	}
	code := http.StatusOK
	if !eslOK || !dbOK {
		code = http.StatusServiceUnavailable
	}
	out := gin.H{
		"esl_ok":    eslOK,
		"esl_error": eslErr,
		"db_ok":     dbOK,
	}
	if s.Listener != nil {
		connected, lastEventAt, lastErr := s.Listener.Status()
		out["listener_connected"] = connected
		out["listener_error"] = lastErr
		if !lastEventAt.IsZero() {
			out["listener_last_event_at"] = lastEventAt.Format(time.RFC3339)
		}
	}
	c.JSON(code, out)
}

func (s *Server) eslStatus(c *gin.Context) {
	out, err := s.ESL.API("status")
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"output": out})
}

type eslAPIReq struct {
	Command string `json:"command" binding:"required"`
}

func (s *Server) eslAPI(c *gin.Context) {
	var req eslAPIReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "command required"})
		return
	}
	// M1 safety: allow only a small allowlist
	cmd := strings.TrimSpace(req.Command)
	allowed := false
	for _, p := range []string{"status", "show channels", "show calls", "sofia status", "version"} {
		if strings.EqualFold(cmd, p) || strings.HasPrefix(strings.ToLower(cmd), p+" ") {
			allowed = true
			break
		}
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "command not allowed in M1"})
		return
	}
	out, err := s.ESL.API(cmd)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"output": out})
}

func (s *Server) listCDR(c *gin.Context) {
	list, err := s.Store.ListCalls(100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"list": list})
}

func (s *Server) webhookTest(c *gin.Context) {
	if !s.Hook.Enabled() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "CC_WEBHOOK_URL empty"})
		return
	}
	payload := gin.H{
		"call_id":   uuid.NewString(),
		"direction": "inbound",
		"caller":    "1000",
		"callee":    "5000",
		"note":      "test event from control-api",
	}
	if err := s.Hook.Send("call.test", payload); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "payload": payload})
}

type ingestReq struct {
	Event   string         `json:"event" binding:"required"`
	Payload map[string]any `json:"payload"`
}

func (s *Server) ingestEvent(c *gin.Context) {
	var req ingestReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if req.Payload == nil {
		req.Payload = map[string]any{}
	}
	callID, _ := req.Payload["call_id"].(string)
	if callID == "" {
		callID = uuid.NewString()
		req.Payload["call_id"] = callID
	}
	now := time.Now()
	rec := &store.Call{
		CallID:    callID,
		Direction: str(req.Payload["direction"]),
		Caller:    str(req.Payload["caller"]),
		Callee:    str(req.Payload["callee"]),
		AgentExt:  str(req.Payload["agent_ext"]),
		Queue:     str(req.Payload["queue"]),
		Status:    req.Event,
		Recording: str(req.Payload["recording"]),
		RawEvent:  mustJSON(req),
	}
	switch req.Event {
	case "call.ringing":
		rec.StartedAt = &now
	case "call.answered":
		rec.AnsweredAt = &now
		rec.Status = "answered"
	case "call.hangup":
		rec.EndedAt = &now
		rec.Status = "hangup"
		if d, ok := req.Payload["duration_sec"].(float64); ok {
			rec.DurationSec = int(d)
		}
	case "recording.ready":
		rec.Status = "recording_ready"
	}
	if err := s.Store.UpsertCall(rec); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if s.Hook.Enabled() {
		_ = s.Hook.Send(req.Event, req.Payload)
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "call": rec})
}

func str(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func mustJSON(v any) string {
	b, _ := jsonMarshal(v)
	return string(b)
}
