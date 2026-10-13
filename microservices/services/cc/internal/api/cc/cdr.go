package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/cc/internal/store/cc"
)

// mod_json_cdr 话单接收：挂机后 FS POST 完整 CDR（Basic 认证，不走网关
// ForwardAuth）。作为 ESL webhook 链路的补充数据源——字段更全（billsec、
// hangup 原因、完整 variables），按 uuid 幂等回填 cc_calls。

type jsonCDR struct {
	Variables map[string]string `json:"variables"`
}

// CDRAuth 形如 "user:pass"（与 FS 侧 CC_CDR_CRED 一致）；开发环境空值禁用
// 鉴权，生产环境空值改为关端点（见 ReceiveCDR），不再无条件放行。
func (s *Server) cdrAuthorized(c *gin.Context) bool {
	if s.CDRAuth == "" {
		return !s.ProductionMode
	}
	user, pass, ok := c.Request.BasicAuth()
	if !ok {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(user+":"+pass), []byte(s.CDRAuth)) == 1
}

// ReceiveCDR handles POST /api/v1/cc/cdr.
func (s *Server) ReceiveCDR(c *gin.Context) {
	// 生产环境没配 Basic 凭据：话单是计费依据，关端点而不是任人投递。
	if s.ProductionMode && s.CDRAuth == "" {
		fail(c, http.StatusServiceUnavailable, "cdr intake disabled: CC_CDR_AUTH is not configured")
		return
	}
	if !s.cdrAuthorized(c) {
		fail(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 2<<20))
	if err != nil {
		fail(c, http.StatusBadRequest, "read body failed")
		return
	}
	var cdr jsonCDR
	if err := json.Unmarshal(raw, &cdr); err != nil || cdr.Variables == nil {
		fail(c, http.StatusBadRequest, "invalid cdr json")
		return
	}
	v := cdr.Variables
	uuid := firstNonEmpty(v["channel_call_uuid"], v["call_uuid"], v["uuid"])
	if uuid == "" {
		ok(c, gin.H{"accepted": true, "stored": false})
		return
	}
	// 留档原始 CDR
	_ = s.Store.LogWebhook(c.Request.Context(), "cdr.json", uuid, string(raw))

	rec := &store.Call{
		TenantID:  s.resolveCDRTenant(c.Request.Context(), v),
		CallID:    uuid,
		Direction: firstNonEmpty(v["direction"], v["call_direction"]),
		Caller:    firstNonEmpty(v["caller_id_number"], v["effective_caller_id_number"]),
		Callee:    firstNonEmpty(v["destination_number"], v["dialed_extension"]),
		AgentExt:  v["dialed_extension"],
		Queue:     v["cc_queue_joined"],
		Status:    "hangup",
		Recording: v["execute_on_answer_recording"],
		// IVR 收号：play_and_get_digits 存 cc_collected（export 到 A-leg），
		// mod_json_cdr 的 variables 里自然带出。
		CollectedDigits: v["cc_collected"],
	}
	rec.HangupCause = v["hangup_cause"]
	if sec, err := strconv.Atoi(v["billsec"]); err == nil {
		rec.DurationSec = sec
	}
	if ts := epochField(v, "start_epoch"); ts != nil {
		rec.StartedAt = ts
	}
	if ts := epochField(v, "answer_epoch"); ts != nil {
		rec.AnsweredAt = ts
	}
	if ts := epochField(v, "end_epoch"); ts != nil {
		rec.EndedAt = ts
	}
	saved, err := s.Store.ApplyEvent(c.Request.Context(), "cdr.json", rec)
	if err != nil {
		fail(c, http.StatusInternalServerError, "persist cdr failed")
		return
	}
	// 挂机后的三个挂钩统一用 saved（ApplyEvent 与既有 ESL 事件合并后的完整行）：
	// mod_json_cdr 顶层 variables 常缺 caller_id_number/destination_number（在
	// callflow 里），rec 的号码可能为空——用 rec 会让入账/推送/跟进静默漏掉。
	if saved.DurationSec == 0 {
		saved.DurationSec = rec.DurationSec
	}
	// 挂机入账：外呼且通话时长>0 时按费率计费；失败只 log 不影响 CDR 响应。
	s.chargeFromCDR(c, saved)
	// 挂机也推一条 call.hangup（带 billsec），坐席弹屏/监控台即时收尾。
	s.pushCallEvent("call.hangup", saved)
	if vid, err := strconv.ParseUint(v["cc_voice_id"], 10, 64); err == nil && vid > 0 {
		s.syncVoiceFromCall(c.Request.Context(), "cdr.json", saved, vid)
	}
	s.maybeEnqueueVoiceQC(c.Request.Context(), saved)
	// 挂机留痕：号码命中 CRM 客户则写一条通话跟进；失败只 log 不影响 CDR 响应。
	s.writeCRMFollowUp(c, saved)
	ok(c, gin.H{"accepted": true, "stored": true, "call_id": saved.CallID})
}

// writeCRMFollowUp 挂机后给命中的 CRM 客户写一条通话跟进（呼入/呼出方向任意）。
// 号码取向：outbound 取被叫 callee，其它取主叫 caller。经 crmclient 内部 HTTP
// 端点识客再写跟进——未接 CRM、未命中客户、任一步失败，都只 log、不影响 CDR。
// 归因坐席用户：rec.AgentUserID（分机绑定用户）带上，无绑定则 0=系统。
func (s *Server) writeCRMFollowUp(c *gin.Context, rec *store.Call) {
	if !s.CRM.Enabled() {
		return
	}
	number := rec.Caller
	if strings.EqualFold(rec.Direction, "outbound") {
		number = rec.Callee
	}
	if number == "" {
		return
	}
	ctx := c.Request.Context()
	res, err := s.CRM.Lookup(ctx, rec.TenantID, number)
	if err != nil {
		log.Printf("cdr crm follow: lookup failed call=%s number=%s: %v", rec.CallID, number, err)
		return
	}
	// 只给已建档的 CRM 客户留痕（线索尚未成客户，跟进挂客户维度）。
	if res == nil || res.Matched != "customer" || res.Customer == nil {
		return
	}
	var byUserID uint64
	if rec.AgentUserID != nil {
		byUserID = *rec.AgentUserID
	}
	content := crmFollowContent(rec)
	if err := s.CRM.CreateFollowUp(ctx, rec.TenantID, "customer", res.Customer.ID, byUserID, content); err != nil {
		log.Printf("cdr crm follow: create failed call=%s customer=%d: %v", rec.CallID, res.Customer.ID, err)
		return
	}
	log.Printf("cdr crm follow: call=%s customer=%d agent=%s written", rec.CallID, res.Customer.ID, rec.AgentExt)
}

// crmFollowContent 拼跟进正文：「[通话] 呼入/呼出 · 时长 X 秒 · 坐席分机 Y · 挂断原因 Z」。
func crmFollowContent(rec *store.Call) string {
	dir := "呼入"
	if strings.EqualFold(rec.Direction, "outbound") {
		dir = "呼出"
	}
	ext := rec.AgentExt
	if ext == "" {
		ext = "-"
	}
	cause := rec.HangupCause
	if cause == "" {
		cause = "-"
	}
	return fmt.Sprintf("[通话] %s · 时长 %d 秒 · 坐席分机 %s · 挂断原因 %s", dir, rec.DurationSec, ext, cause)
}

// resolveCDRTenant 解析一通话单的租户归属，优先级：
//  1. CDR variables 里显式的 cc_tenant_id（拨号计划/originate 注入时最可信）；
//  2. 主叫分机（caller_id_number，坐席外呼的源分机）在 cc_agents 的跨租户绑定；
//  3. 被叫分机（dialed_extension，呼入落到的坐席分机）的绑定；
//  4. 都查不到回默认租户 1。
//
// 查询失败只记日志按默认租户处理——话单入库不能因归属查询挂掉。
func (s *Server) resolveCDRTenant(ctx context.Context, v map[string]string) uint64 {
	if id, err := strconv.ParseUint(v["cc_tenant_id"], 10, 64); err == nil && id > 0 {
		return id
	}
	for _, ext := range []string{
		firstNonEmpty(v["caller_id_number"], v["effective_caller_id_number"]),
		v["dialed_extension"],
	} {
		if ext == "" {
			continue
		}
		bound, err := s.Store.GetAgentByExtensionAny(ctx, ext)
		if err != nil {
			log.Printf("cdr tenant: lookup ext=%s failed: %v", ext, err)
			continue
		}
		if bound != nil {
			return bound.TenantID
		}
	}
	return 1
}

// chargeFromCDR 按费率对一通已完成的外呼计费入账。
// 计费口径：outbound 且 billsec>0；MatchRate(callee) 命中→
//
//	amount = connectFee + ceil(billsec/step)*step/60*ratePerMin
//
// 整数运算：amount = connectFee + ((billsec+step-1)/step)*step*ratePerMin/60。
// 未命中费率 = 不计费（免费）。任何失败只记日志，不影响 CDR HTTP 响应。
// 租户归属由 resolveCDRTenant 解析（分机绑定 cc_agents），费率与扣费均按该租户。
func (s *Server) chargeFromCDR(c *gin.Context, rec *store.Call) {
	if !strings.EqualFold(rec.Direction, "outbound") || rec.DurationSec <= 0 {
		return
	}
	ctx := c.Request.Context()
	rate, err := s.Store.MatchRate(ctx, rec.TenantID, rec.Callee)
	if err != nil {
		log.Printf("cdr charge: match rate failed call=%s: %v", rec.CallID, err)
		return
	}
	if rate == nil {
		return // 未命中费率 = 免费不计费
	}
	amount := billAmount(int64(rec.DurationSec), rate)
	charged, err := s.Store.ChargeCall(ctx, &store.CCCallCharge{
		TenantID: rec.TenantID,
		CallID:   rec.CallID,
		Caller:   rec.Caller,
		Callee:   rec.Callee,
		BillSec:  rec.DurationSec,
		RateID:   rate.ID,
		Amount:   amount,
	})
	if err != nil {
		log.Printf("cdr charge: charge call failed call=%s: %v", rec.CallID, err)
		return
	}
	log.Printf("cdr charge: call=%s billsec=%d amount=%d balance_after=%d",
		rec.CallID, rec.DurationSec, charged.Amount, charged.BalanceAfter)
}

// billAmount 用整数运算算一通话费（分）。step 兜底 60，避免除零。
func billAmount(billSec int64, rate *store.CCRate) int64 {
	step := int64(rate.BillingStep)
	if step <= 0 {
		step = 60
	}
	// ceil(billSec/step)*step = 进位到步长的计费秒数
	billedSec := ((billSec + step - 1) / step) * step
	return rate.ConnectFee + billedSec*rate.RatePerMin/60
}

func epochField(v map[string]string, key string) *time.Time {
	sec, err := strconv.ParseInt(v[key], 10, 64)
	if err != nil || sec <= 0 {
		return nil
	}
	t := time.Unix(sec, 0)
	return &t
}

func firstNonEmpty(vals ...string) string {
	for _, s := range vals {
		if s != "" {
			return s
		}
	}
	return ""
}
