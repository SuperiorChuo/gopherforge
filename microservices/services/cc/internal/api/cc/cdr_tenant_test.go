package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/cc/internal/store/cc"
)

// newTestServerWithBilling 在 CDR 测试服务器上补挂计费三表（租户归属计费用例）。
func newTestServerWithBilling(t *testing.T) (*Server, *gin.Engine) {
	srv, r := newTestServerWithCDR(t)
	if err := srv.Store.DB().AutoMigrate(&store.CCRate{}, &store.CCBillingAccount{}, &store.CCCallCharge{}, &store.CCAccountTxn{}, &store.CCBill{}); err != nil {
		t.Fatalf("migrate billing tables: %v", err)
	}
	return srv, r
}

// bindExt 把分机绑到指定租户的用户（租户归属反查的事实源）。
func bindExt(t *testing.T, srv *Server, tenantID, userID uint64, ext string) {
	t.Helper()
	if _, err := srv.Store.BindAgent(context.Background(), &store.Agent{
		TenantID: tenantID, UserID: userID, Extension: ext,
	}); err != nil {
		t.Fatalf("bind ext %s tenant %d: %v", ext, tenantID, err)
	}
}

func TestCDRTenantResolvedByCallerExtension(t *testing.T) {
	srv, r := newTestServerWithBilling(t)
	ctx := context.Background()

	// 分机 1000 属租户 2；租户 2 配手机费率并充值。
	bindExt(t, srv, 2, 42, "1000")
	if _, err := srv.Store.UpsertRate(ctx, &store.CCRate{
		TenantID: 2, Prefix: "1", Name: "手机", RatePerMin: 60, BillingStep: 60,
	}); err != nil {
		t.Fatalf("seed rate: %v", err)
	}
	if _, err := srv.Store.Recharge(ctx, 2, 10000, 1, "seed"); err != nil {
		t.Fatalf("recharge: %v", err)
	}

	// 租户 2 坐席 1000 外呼手机号，通话 60 秒。
	w := postCDR(t, r, map[string]string{
		"uuid": "t2-call-1", "direction": "outbound",
		"caller_id_number": "1000", "destination_number": "13912345678",
		"billsec": "60", "hangup_cause": "NORMAL_CLEARING",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("cdr status = %d body=%s", w.Code, w.Body.String())
	}

	// 话单归到租户 2（租户 1 视角看不到）。
	call, err := srv.Store.GetCall(ctx, 2, "t2-call-1")
	if err != nil {
		t.Fatalf("call not under tenant 2: %v", err)
	}
	if call.TenantID != 2 {
		t.Fatalf("call tenant = %d, want 2", call.TenantID)
	}
	if _, _, err := srv.Store.ListCalls(ctx, store.ListCallsQuery{TenantID: 1}); err != nil {
		t.Fatalf("list tenant1: %v", err)
	}

	// 扣费落在租户 2 账上：60 秒 * 60 分/分钟 = 60 分。
	acc2, _ := srv.Store.GetOrCreateAccount(ctx, 2)
	if acc2.Balance != 10000-60 {
		t.Fatalf("tenant2 balance = %d, want %d", acc2.Balance, 10000-60)
	}
	// 租户 1 账户分文未动。
	acc1, _ := srv.Store.GetOrCreateAccount(ctx, 1)
	if acc1.Balance != 0 {
		t.Fatalf("tenant1 balance = %d, want 0 (must not be charged)", acc1.Balance)
	}
	charges, total, _ := srv.Store.ListCharges(ctx, store.ListChargesQuery{TenantID: 2})
	if total != 1 || charges[0].CallID != "t2-call-1" {
		t.Fatalf("tenant2 charges = %d %+v, want the call charge", total, charges)
	}
}

func TestCDRTenantExplicitVariableWins(t *testing.T) {
	srv, r := newTestServerWithBilling(t)

	// 分机 1000 绑租户 2，但 CDR 显式带 cc_tenant_id=3 → 显式值优先。
	bindExt(t, srv, 2, 42, "1000")
	w := postCDR(t, r, map[string]string{
		"uuid": "t3-call-1", "direction": "inbound",
		"caller_id_number": "13800000000", "dialed_extension": "1000",
		"cc_tenant_id": "3", "billsec": "10",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("cdr status = %d", w.Code)
	}
	call, err := srv.Store.GetCall(context.Background(), 3, "t3-call-1")
	if err != nil || call.TenantID != 3 {
		t.Fatalf("call = %+v err=%v, want tenant 3", call, err)
	}
}

func TestCDRTenantFallsBackToDefault(t *testing.T) {
	srv, r := newTestServerWithBilling(t)

	// 无任何绑定、无显式变量 → 默认租户 1（与旧行为一致）。
	w := postCDR(t, r, map[string]string{
		"uuid": "t1-call-1", "direction": "inbound",
		"caller_id_number": "13800000000", "dialed_extension": "1000", "billsec": "5",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("cdr status = %d", w.Code)
	}
	call, err := srv.Store.GetCall(context.Background(), 1, "t1-call-1")
	if err != nil || call.TenantID != 1 {
		t.Fatalf("call = %+v err=%v, want tenant 1", call, err)
	}
}

func TestWebhookTenantResolvedByAgentExt(t *testing.T) {
	srv, r := newTestServerWithBilling(t)
	bindExt(t, srv, 2, 42, "1001")

	// ESL webhook payload 不带 tenant_id → 按 agent_ext 反查归租户 2。
	body := envelope("call.answered", "wh-call-1", map[string]any{
		"caller": "13800000000", "callee": "5000", "agent_ext": "1001", "direction": "inbound",
	})
	if w := postWebhook(r, body, ""); w.Code != http.StatusOK {
		t.Fatalf("webhook status = %d body=%s", w.Code, w.Body.String())
	}
	call, err := srv.Store.GetCall(context.Background(), 2, "wh-call-1")
	if err != nil || call.TenantID != 2 {
		t.Fatalf("call = %+v err=%v, want tenant 2", call, err)
	}
	// 归因也应带上绑定用户。
	if call.AgentUserID == nil || *call.AgentUserID != 42 {
		t.Fatalf("agent_user_id = %v, want 42", call.AgentUserID)
	}
}

func TestApplyEventTenantProgressesButNeverRegresses(t *testing.T) {
	srv, r := newTestServerWithBilling(t)
	bindExt(t, srv, 2, 42, "1000")

	// 先来一条无归属线索的 ringing（落默认租户 1）……
	if w := postWebhook(r, envelope("call.ringing", "mix-call-1", map[string]any{
		"caller": "13800000000", "direction": "inbound",
	}), ""); w.Code != http.StatusOK {
		t.Fatalf("ringing status = %d", w.Code)
	}
	// ……随后 CDR 带出被叫分机 1000（租户 2）→ 同一行租户前进到 2。
	if w := postCDR(t, r, map[string]string{
		"uuid": "mix-call-1", "direction": "inbound",
		"caller_id_number": "13800000000", "dialed_extension": "1000", "billsec": "30",
	}); w.Code != http.StatusOK {
		t.Fatalf("cdr status = %d", w.Code)
	}
	call, err := srv.Store.GetCall(context.Background(), 2, "mix-call-1")
	if err != nil || call.TenantID != 2 {
		t.Fatalf("call = %+v err=%v, want tenant upgraded to 2", call, err)
	}
	// 迟到的无线索事件不得把租户打回 1。
	if w := postWebhook(r, envelope("recording.ready", "mix-call-1", map[string]any{
		"recording": "/recordings/mix-call-1.wav",
	}), ""); w.Code != http.StatusOK {
		t.Fatalf("recording.ready status = %d", w.Code)
	}
	call, err = srv.Store.GetCall(context.Background(), 2, "mix-call-1")
	if err != nil || call.TenantID != 2 {
		t.Fatalf("late event regressed tenant: %+v err=%v", call, err)
	}
}
