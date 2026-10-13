// Package api wires cc-service HTTP handlers: the freeswitch-cc webhook
// receiver (HMAC-signed, no JWT) and admin endpoints (gateway/JWT auth).
package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"encoding/json"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/cc/internal/authjwt"
	"github.com/go-admin-kit/services/cc/internal/config"
	"github.com/go-admin-kit/services/cc/internal/crmclient"
	"github.com/go-admin-kit/services/cc/internal/fsclient"
	"github.com/go-admin-kit/services/cc/internal/hub"
	"github.com/go-admin-kit/services/cc/internal/store/cc"
)

type Server struct {
	Store         *store.Store
	FS            *fsclient.Client
	Secret        string // JWT secret（auth-service 同源）
	WebhookSecret string // HMAC secret（freeswitch-cc 同源）
	CDRAuth       string // mod_json_cdr Basic 凭据 "user:pass"；空=不鉴权（dev）
	// ProductionMode 生产环境标记。为 true 时，上面两个密钥缺失的端点直接
	// 拒绝请求（fail closed），不再沿用开发环境"空密钥即免鉴权"的便利——
	// 否则少配一个变量就等于把写话单/写账的入口开放给任何人。
	ProductionMode bool

	// Hub 把话务事件实时推坐席浏览器（来电弹屏底座）。为空时推送静默跳过，
	// 便于单测复用 newTestServer 不必装 hub。
	Hub *hub.Hub
	// CRM 是 crm-service 内网客户端（号码识客聚合、挂机写跟进）。未配置时
	// CRM.Enabled()==false，识客只返回 cc 侧数据、挂机不写 CRM。
	CRM *crmclient.Client

	// Speech TTS/ASR 运行时配置：system_settings（cc.tts/cc.asr）热生效，
	// CC_TTS_*/CC_ASR_* 环境变量兜底（控制台「系统设置→呼叫中心」页配置）。
	Speech *config.SpeechReader
	// InternalToken 给 notify-service 调语音通知内部端点（X-Internal-Token）。
	// 空=内部端点 503，拒绝裸奔。
	InternalToken string
}

// ---- envelope helpers (统一 {code,message,data}，与核心服务一致) ----

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": data})
}

func fail(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"code": status, "message": msg})
}

// ---- health ----

func (s *Server) HealthLive(c *gin.Context) {
	ok(c, gin.H{"status": "alive", "service": "cc-service", "time": time.Now().Format(time.RFC3339)})
}

func (s *Server) HealthReady(c *gin.Context) {
	sqlDB, err := s.Store.DB().DB()
	if err != nil {
		fail(c, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		fail(c, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	ok(c, gin.H{"status": "ready"})
}

// ---- webhook (freeswitch-cc → 本服务；HMAC 验签，非 JWT) ----

type webhookEnvelope struct {
	Event   string         `json:"event"`
	Source  string         `json:"source"`
	SentAt  string         `json:"sent_at"`
	Payload map[string]any `json:"payload"`
}

// Webhook handles POST /api/v1/cc/webhook.
func (s *Server) Webhook(c *gin.Context) {
	// 生产环境没配 HMAC 密钥：验签无从进行，直接关端点而不是放行明文事件。
	if s.ProductionMode && s.WebhookSecret == "" {
		fail(c, http.StatusServiceUnavailable, "webhook disabled: CC_WEBHOOK_SECRET is not configured")
		return
	}

	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		fail(c, http.StatusBadRequest, "read body failed")
		return
	}

	if s.WebhookSecret != "" {
		sig := strings.TrimPrefix(c.GetHeader("X-CC-Signature"), "sha256=")
		mac := hmac.New(sha256.New, []byte(s.WebhookSecret))
		_, _ = mac.Write(raw)
		want := hex.EncodeToString(mac.Sum(nil))
		if sig == "" || !hmac.Equal([]byte(sig), []byte(want)) {
			fail(c, http.StatusUnauthorized, "invalid signature")
			return
		}
	}

	var env webhookEnvelope
	if err := json.Unmarshal(raw, &env); err != nil || env.Event == "" {
		fail(c, http.StatusBadRequest, "invalid envelope")
		return
	}
	if env.Payload == nil {
		env.Payload = map[string]any{}
	}
	callID, _ := env.Payload["call_id"].(string)

	// 原始事件留档（含 call.test 等非话单事件）
	_ = s.Store.LogWebhook(c.Request.Context(), env.Event, callID, string(raw))

	if !strings.HasPrefix(env.Event, "call.") && env.Event != "recording.ready" {
		ok(c, gin.H{"accepted": true, "stored": false})
		return
	}
	if env.Event == "call.test" || callID == "" {
		ok(c, gin.H{"accepted": true, "stored": false})
		return
	}

	now := time.Now()
	rec := &store.Call{
		TenantID:  store.EnsureTenant(uintFrom(env.Payload["tenant_id"])),
		CallID:    callID,
		Direction: strFrom(env.Payload["direction"]),
		Caller:    strFrom(env.Payload["caller"]),
		Callee:    strFrom(env.Payload["callee"]),
		AgentExt:  strFrom(env.Payload["agent_ext"]),
		Queue:     strFrom(env.Payload["queue"]),
		Recording: strFrom(env.Payload["recording"]),
	}
	// 租户归属：payload 未带 tenant_id 时按分机跨租户反查（坐席分机 > 主叫分机，
	// 与 CDR 路径 resolveCDRTenant 同口径），保证话单/计费归对租户账。
	if uintFrom(env.Payload["tenant_id"]) == 0 {
		for _, ext := range []string{rec.AgentExt, rec.Caller} {
			if ext == "" {
				continue
			}
			if bound, err := s.Store.GetAgentByExtensionAny(c.Request.Context(), ext); err == nil && bound != nil {
				rec.TenantID = bound.TenantID
				break
			}
		}
	}
	// 话单归因：分机绑定了用户就带上 agent_user_id；查失败不阻塞话单入库。
	if rec.AgentExt != "" {
		if bound, err := s.Store.GetAgentByExtension(c.Request.Context(), rec.TenantID, rec.AgentExt); err == nil && bound != nil {
			rec.AgentUserID = &bound.UserID
		}
	}
	switch env.Event {
	case "call.ringing":
		rec.Status = "ringing"
		rec.StartedAt = &now
	case "call.answered":
		rec.Status = "answered"
		rec.AnsweredAt = &now
	case "call.hangup":
		rec.Status = "hangup"
		rec.EndedAt = &now
		rec.HangupCause = strFrom(env.Payload["cause"])
		if d, ok := env.Payload["duration_sec"].(float64); ok {
			rec.DurationSec = int(d)
		}
	case "recording.ready":
		rec.Status = "recording_ready"
	case "call.early_media":
		rec.HangupClass = strFrom(env.Payload["class"])
		if !validEarlyMediaClass(rec.HangupClass) {
			fail(c, http.StatusBadRequest, "invalid early media class")
			return
		}
	default:
		ok(c, gin.H{"accepted": true, "stored": false})
		return
	}

	saved, err := s.Store.ApplyEvent(c.Request.Context(), env.Event, rec)
	if err != nil {
		fail(c, http.StatusInternalServerError, "persist call failed")
		return
	}
	if vid := uintFrom(env.Payload["voice_id"]); vid > 0 {
		if env.Event == "call.early_media" {
			_ = s.Store.SetVoiceNotifyHangupClass(c.Request.Context(), vid, saved.HangupClass)
		} else {
			s.syncVoiceFromCall(c.Request.Context(), env.Event, saved, vid)
		}
	}
	if env.Event == "recording.ready" {
		s.maybeEnqueueVoiceQC(c.Request.Context(), saved)
	}
	// 入库成功后推 WS（毫秒弹屏）。仅推 ringing/answered/hangup 三类实时事件，
	// recording.ready 等非坐席关心的不推。推送异步、失败不影响 webhook 响应。
	if pushType := realtimeEventType(env.Event); pushType != "" {
		s.pushCallEvent(pushType, saved)
	}
	ok(c, gin.H{"accepted": true, "stored": true, "call_id": saved.CallID})
}

// realtimeEventType 把 webhook event 映射为推给前端的实时事件类型；非实时事件返回空。
func realtimeEventType(event string) string {
	switch event {
	case "call.ringing", "call.answered", "call.hangup":
		return event
	default:
		return ""
	}
}

// pushCallEvent 组装并按事件归属租户投递一条话务事件（含号码/客户名，绝不全平台
// 广播）。caller_name 从 cc_contacts 富化（入方向按主叫号，出方向按被叫号；查不到
// 留空）。放 goroutine 里跑，绝不阻塞 webhook 热路径；Hub 为空（单测）直接跳过。
func (s *Server) pushCallEvent(eventType string, rec *store.Call) {
	if s.Hub == nil {
		return
	}
	ev := hub.Event{
		Type:      eventType,
		CallID:    rec.CallID,
		Direction: rec.Direction,
		Caller:    rec.Caller,
		Callee:    rec.Callee,
		AgentExt:  rec.AgentExt,
		Queue:     rec.Queue,
		BillSec:   rec.DurationSec,
		TS:        time.Now().UnixMilli(),
	}
	// 弹屏显名：入方向查主叫、出方向查被叫。查库放同一 goroutine，用后台 ctx。
	lookupNum := rec.Caller
	if strings.EqualFold(rec.Direction, "outbound") {
		lookupNum = rec.Callee
	}
	tenantID := store.EnsureTenant(rec.TenantID)
	go func() {
		if lookupNum != "" {
			if name, err := s.Store.LookupContactName(context.Background(), tenantID, lookupNum); err == nil {
				ev.CallerName = name
			}
		}
		s.Hub.BroadcastTenant(tenantID, ev)
	}()
}

// ---- admin endpoints (网关头优先，JWT 兜底；与 im-service 相同约定) ----

func (s *Server) requireAgent(c *gin.Context) (*authjwt.AgentClaims, bool) {
	if uid := c.GetHeader("X-Auth-User-ID"); uid != "" {
		id, err := strconv.ParseUint(uid, 10, 64)
		if err == nil && id > 0 {
			return &authjwt.AgentClaims{
				UserID:   id,
				Username: c.GetHeader("X-Auth-Username"),
				TenantID: tenantFromHeader(c),
			}, true
		}
	}
	tok := bearer(c)
	if tok == "" {
		fail(c, http.StatusUnauthorized, "missing token")
		return nil, false
	}
	claims, err := authjwt.ParseAgent(s.Secret, tok)
	if err != nil {
		fail(c, http.StatusUnauthorized, "invalid token")
		return nil, false
	}
	return claims, true
}

// ListCalls handles GET /api/v1/cc/calls.
func (s *Server) ListCalls(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	list, total, err := s.Store.ListCalls(c.Request.Context(), store.ListCallsQuery{
		TenantID: agent.TenantID,
		Status:   c.Query("status"),
		Caller:   c.Query("caller"),
		AgentExt: c.Query("agent_ext"),
		Number:   c.Query("number"),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, "list calls failed")
		return
	}
	ok(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

// FSStatus handles GET /api/v1/cc/fs/status — proxies control-api readiness
// plus FreeSWITCH ESL status so运维不用直连媒体面。
func (s *Server) FSStatus(c *gin.Context) {
	if _, authorized := s.requireAgent(c); !authorized {
		return
	}
	if !s.FS.Enabled() {
		ok(c, gin.H{"configured": false})
		return
	}
	out, err := s.FS.Get(c.Request.Context(), "/v1/esl/status")
	if err != nil {
		ok(c, gin.H{"configured": true, "reachable": false, "error": err.Error()})
		return
	}
	ok(c, gin.H{"configured": true, "reachable": true, "status": out})
}

// WebhookTest handles POST /api/v1/cc/fs/webhook-test — asks control-api to
// fire a call.test back at us, verifying the full loop in one click（纯运维
// 动作，仅平台管理员）.
func (s *Server) WebhookTest(c *gin.Context) {
	if _, authorized := s.requirePlatformAdmin(c); !authorized {
		return
	}
	if !s.FS.Enabled() {
		fail(c, http.StatusBadRequest, "CC_FS_API_BASE is not configured")
		return
	}
	out, err := s.FS.Post(c.Request.Context(), "/v1/webhooks/test", gin.H{})
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	ok(c, out)
}

// ---- helpers ----

func bearer(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}

func tenantFromHeader(c *gin.Context) uint64 {
	if v := c.GetHeader("X-Auth-Tenant-ID"); v != "" {
		if id, err := strconv.ParseUint(v, 10, 64); err == nil {
			return authjwt.NormalizeTenantID(id)
		}
	}
	return 1
}

func strFrom(v any) string {
	if s, isStr := v.(string); isStr {
		return s
	}
	return ""
}

func uintFrom(v any) uint64 {
	switch n := v.(type) {
	case float64:
		if n > 0 {
			return uint64(n)
		}
	case string:
		if id, err := strconv.ParseUint(n, 10, 64); err == nil {
			return id
		}
	}
	return 0
}
