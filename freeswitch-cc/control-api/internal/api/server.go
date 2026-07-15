package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/config"
	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/esl"
	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/store"
	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/webhook"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Server struct {
	Cfg   config.Config
	ESL   *esl.Client
	Store *store.Store
	Hook  *webhook.Client
}

func (s *Server) Router() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery(), gin.Logger())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "control-api"})
	})
	r.GET("/ready", s.ready)

	auth := r.Group("/")
	auth.Use(s.tokenAuth)
	{
		auth.GET("/v1/extensions", s.listExtensions)
		auth.GET("/v1/esl/status", s.eslStatus)
		auth.POST("/v1/esl/api", s.eslAPI)
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
	c.JSON(code, gin.H{
		"esl_ok": eslOK,
		"esl_error": eslErr,
		"db_ok":  dbOK,
	})
}

func (s *Server) listExtensions(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"extensions": []gin.H{
			{"ext": "1000", "password": "1234", "label": "Agent A"},
			{"ext": "1001", "password": "1234", "label": "Agent B"},
			{"ext": "1002", "password": "1234", "label": "Agent C"},
			{"ext": "1003", "password": "1234", "label": "Agent D"},
			{"ext": "5000", "password": "", "label": "Demo queue (dial)"},
		},
	})
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
