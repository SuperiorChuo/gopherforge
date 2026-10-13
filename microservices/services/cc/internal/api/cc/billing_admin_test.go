package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/cc/internal/store/cc"
	"github.com/go-admin-kit/services/shared/pkg/notifyclient"
)

// newBillingAdminServer 挂计费全量路由（含平台侧）的测试服务器。
func newBillingAdminServer(t *testing.T) (*Server, *gin.Engine) {
	srv, r := newTestServerWithBilling(t)
	r.POST("/api/v1/cc/billing/recharge", srv.Recharge)
	r.GET("/api/v1/cc/billing/txns", srv.ListAccountTxns)
	r.GET("/api/v1/cc/billing/bills", srv.ListBills)
	r.GET("/api/v1/cc/billing/bills/:id/export", srv.ExportBillCSV)
	r.GET("/api/v1/cc/billing/admin/overview", srv.AdminAccountsOverview)
	r.POST("/api/v1/cc/billing/admin/recharge", srv.AdminRecharge)
	r.POST("/api/v1/cc/billing/admin/status", srv.AdminSetStatus)
	r.POST("/api/v1/cc/billing/admin/bills/generate", srv.AdminGenerateBills)
	return srv, r
}

// doAs 以指定租户身份发请求（网关头范式）。
func doAs(r http.Handler, tenantID uint64, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Auth-User-ID", "9")
	req.Header.Set("X-Auth-Tenant-ID", strconv.FormatUint(tenantID, 10))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestRechargeWritesAuditTxn(t *testing.T) {
	srv, r := newBillingAdminServer(t)
	ctx := context.Background()

	// 租户 2 充值 100 元带备注，再调减 30 元。
	if w := doAs(r, 2, http.MethodPost, "/api/v1/cc/billing/recharge", `{"amount_fen":10000,"remark":"首充"}`); w.Code != http.StatusOK {
		t.Fatalf("recharge status = %d body=%s", w.Code, w.Body.String())
	}
	if w := doAs(r, 2, http.MethodPost, "/api/v1/cc/billing/recharge", `{"amount_fen":-3000,"remark":"冲正"}`); w.Code != http.StatusOK {
		t.Fatalf("adjust status = %d", w.Code)
	}

	txns, total, err := srv.Store.ListAccountTxns(ctx, 2, 1, 10)
	if err != nil || total != 2 {
		t.Fatalf("txns total = %d err=%v, want 2", total, err)
	}
	// 倒序：最新在前（调减）。
	if txns[0].Kind != "adjust" || txns[0].Amount != -3000 || txns[0].BalanceAfter != 7000 || txns[0].Remark != "冲正" {
		t.Fatalf("adjust txn = %+v", txns[0])
	}
	if txns[1].Kind != "recharge" || txns[1].Amount != 10000 || txns[1].BalanceAfter != 10000 || txns[1].OperatorID != 9 {
		t.Fatalf("recharge txn = %+v", txns[1])
	}
	// 租户隔离：租户 3 看不到。
	_, total3, _ := srv.Store.ListAccountTxns(ctx, 3, 1, 10)
	if total3 != 0 {
		t.Fatalf("tenant3 must see 0 txns, got %d", total3)
	}
}

func TestAdminEndpointsGatedToPlatformTenant(t *testing.T) {
	_, r := newBillingAdminServer(t)
	// 非平台租户 → 403。
	if w := doAs(r, 2, http.MethodGet, "/api/v1/cc/billing/admin/overview", ""); w.Code != http.StatusForbidden {
		t.Fatalf("tenant2 overview status = %d, want 403", w.Code)
	}
	if w := doAs(r, 2, http.MethodPost, "/api/v1/cc/billing/admin/recharge", `{"tenant_id":3,"amount_fen":100}`); w.Code != http.StatusForbidden {
		t.Fatalf("tenant2 admin recharge = %d, want 403", w.Code)
	}
	// 平台租户 → 200。
	if w := doAs(r, 1, http.MethodGet, "/api/v1/cc/billing/admin/overview", ""); w.Code != http.StatusOK {
		t.Fatalf("tenant1 overview status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestAdminRechargeAndOverview(t *testing.T) {
	srv, r := newBillingAdminServer(t)
	ctx := context.Background()

	// 平台管理员给租户 5 充 200 元、冻结租户 5。
	if w := doAs(r, 1, http.MethodPost, "/api/v1/cc/billing/admin/recharge", `{"tenant_id":5,"amount_fen":20000,"remark":"代充"}`); w.Code != http.StatusOK {
		t.Fatalf("admin recharge status = %d body=%s", w.Code, w.Body.String())
	}
	if w := doAs(r, 1, http.MethodPost, "/api/v1/cc/billing/admin/status", `{"tenant_id":5,"status":"frozen"}`); w.Code != http.StatusOK {
		t.Fatalf("admin freeze status = %d", w.Code)
	}
	acc, _ := srv.Store.GetOrCreateAccount(ctx, 5)
	if acc.Balance != 20000 || acc.Status != "frozen" {
		t.Fatalf("tenant5 account = %+v", acc)
	}
	// 总览包含租户 5。
	w := doAs(r, 1, http.MethodGet, "/api/v1/cc/billing/admin/overview", "")
	var resp struct {
		Data struct {
			List []store.AccountOverviewRow `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse overview: %v", err)
	}
	found := false
	for _, row := range resp.Data.List {
		if row.TenantID == 5 && row.Balance == 20000 && row.Status == "frozen" {
			found = true
		}
	}
	if !found {
		t.Fatalf("overview missing tenant5 row: %+v", resp.Data.List)
	}
}

func seedCharge(t *testing.T, srv *Server, tenantID uint64, callID string, amount int64, at time.Time) {
	t.Helper()
	if _, err := srv.Store.ChargeCall(context.Background(), &store.CCCallCharge{
		TenantID: tenantID, CallID: callID, BillSec: 60, Amount: amount, CreatedAt: at,
	}); err != nil {
		t.Fatalf("seed charge %s: %v", callID, err)
	}
}

func TestBillsGenerateListExport(t *testing.T) {
	srv, r := newBillingAdminServer(t)
	june := time.Date(2026, 6, 15, 10, 0, 0, 0, time.Local)

	seedCharge(t, srv, 2, "b1", 500, june)
	seedCharge(t, srv, 2, "b2", 700, june.Add(time.Hour))
	seedCharge(t, srv, 3, "b3", 900, june)
	seedCharge(t, srv, 2, "july", 999, june.AddDate(0, 1, 0)) // 下月，不进 6 月账单

	// 出账。
	w := doAs(r, 1, http.MethodPost, "/api/v1/cc/billing/admin/bills/generate", `{"period":"2026-06"}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"generated":2`) {
		t.Fatalf("generate status = %d body=%s", w.Code, w.Body.String())
	}
	// 幂等重算。
	if w := doAs(r, 1, http.MethodPost, "/api/v1/cc/billing/admin/bills/generate", `{"period":"2026-06"}`); w.Code != http.StatusOK {
		t.Fatalf("regenerate status = %d", w.Code)
	}

	// 平台看全部（2 行），租户 2 只看自己（1 行，合计 1200）。
	bills, err := srv.Store.ListBills(context.Background(), 0, "2026-06")
	if err != nil || len(bills) != 2 {
		t.Fatalf("platform bills = %d err=%v, want 2", len(bills), err)
	}
	own, _ := srv.Store.ListBills(context.Background(), 2, "2026-06")
	if len(own) != 1 || own[0].TotalAmount != 1200 || own[0].CallCount != 2 {
		t.Fatalf("tenant2 bill = %+v", own)
	}

	// 导出：本租户可导，含表头与明细行；他租户 403。
	billID := strconv.FormatUint(own[0].ID, 10)
	w = doAs(r, 2, http.MethodGet, "/api/v1/cc/billing/bills/"+billID+"/export", "")
	if w.Code != http.StatusOK {
		t.Fatalf("export status = %d body=%s", w.Code, w.Body.String())
	}
	csv := w.Body.String()
	if !strings.Contains(csv, "2026-06") || !strings.Contains(csv, "b1") || !strings.Contains(csv, "b2") || strings.Contains(csv, "b3") {
		t.Fatalf("csv content wrong:\n%s", csv)
	}
	if w := doAs(r, 3, http.MethodGet, "/api/v1/cc/billing/bills/"+billID+"/export", ""); w.Code != http.StatusForbidden {
		t.Fatalf("cross-tenant export status = %d, want 403", w.Code)
	}
}

func TestListWarnAccountsThreshold(t *testing.T) {
	srv, _ := newBillingAdminServer(t)
	ctx := context.Background()

	_, _ = srv.Store.Recharge(ctx, 2, 500, 1, "低额")   // 可用额 500 ≤ 1000 → 预警
	_, _ = srv.Store.Recharge(ctx, 3, 50000, 1, "充足") // 可用额 50000 → 不预警
	_, _ = srv.Store.Recharge(ctx, 4, 100, 1, "冻结")
	_, _ = srv.Store.SetAccountStatus(ctx, 4, "frozen") // 已冻结 → 不再预警

	list, err := srv.Store.ListWarnAccounts(ctx, 1000)
	if err != nil {
		t.Fatalf("warn scan: %v", err)
	}
	ids := map[uint64]bool{}
	for _, a := range list {
		ids[a.TenantID] = true
	}
	if !ids[2] || ids[3] || ids[4] {
		t.Fatalf("warn tenants = %v, want only tenant2 (and maybe tenant1 default)", ids)
	}
}

func TestAutoFreezeOnExhaustedBalance(t *testing.T) {
	srv, _ := newBillingAdminServer(t)
	ctx := context.Background()

	// 租户 7 账户建出来余额 0（可用额 0 ≤ 0）；租户 8 余额充足。
	_, _ = srv.Store.GetOrCreateAccount(ctx, 7)
	_, _ = srv.Store.Recharge(ctx, 8, 50000, 1, "充足")

	disabledNotify := notifyclient.New("", "") // 通知未配置：静默跳过
	// 开关关：不冻结。
	srv.scanBalanceWarn(ctx, disabledNotify, BalanceWarnOptions{ThresholdFen: 1000, WarnUserID: 1, AutoFreeze: false})
	acc, _ := srv.Store.GetOrCreateAccount(ctx, 7)
	if acc.Status != "normal" {
		t.Fatalf("auto_freeze off: status = %q, want normal", acc.Status)
	}
	// 开关开：欠费租户冻结，余额充足租户不动。
	srv.scanBalanceWarn(ctx, disabledNotify, BalanceWarnOptions{ThresholdFen: 1000, WarnUserID: 1, AutoFreeze: true})
	acc, _ = srv.Store.GetOrCreateAccount(ctx, 7)
	if acc.Status != "frozen" {
		t.Fatalf("auto_freeze on: status = %q, want frozen", acc.Status)
	}
	acc8, _ := srv.Store.GetOrCreateAccount(ctx, 8)
	if acc8.Status != "normal" {
		t.Fatalf("well-funded tenant must stay normal, got %q", acc8.Status)
	}
	// 再扫一轮幂等（已冻结不在 ListWarnAccounts 里，不重复处理）。
	srv.scanBalanceWarn(ctx, disabledNotify, BalanceWarnOptions{ThresholdFen: 1000, WarnUserID: 1, AutoFreeze: true})
}

func TestAdminSetStatusWithFSDisabled(t *testing.T) {
	srv, r := newBillingAdminServer(t)
	// FS 未配置：冻结照常生效，联动静默跳过（无 line_sync_error）。
	w := doAs(r, 1, http.MethodPost, "/api/v1/cc/billing/admin/status", `{"tenant_id":6,"status":"frozen"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("freeze status = %d body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "line_sync_error") {
		t.Fatalf("FS disabled must not report sync error: %s", w.Body.String())
	}
	acc, _ := srv.Store.GetOrCreateAccount(context.Background(), 6)
	if acc.Status != "frozen" {
		t.Fatalf("status = %q, want frozen", acc.Status)
	}
}

func TestPurgeWebhookLogs(t *testing.T) {
	srv, _ := newBillingAdminServer(t)
	ctx := context.Background()
	// 旧日志（40 天前）+ 新日志。
	old := &store.WebhookLog{Event: "call.hangup", CallID: "old-1", Body: "{}"}
	srv.Store.DB().Create(old)
	srv.Store.DB().Model(old).Update("created_at", time.Now().AddDate(0, 0, -40))
	_ = srv.Store.LogWebhook(ctx, "call.hangup", "new-1", "{}")

	n, err := srv.Store.PurgeWebhookLogs(ctx, time.Now().AddDate(0, 0, -30))
	if err != nil || n != 1 {
		t.Fatalf("purged = %d err=%v, want 1", n, err)
	}
	var left int64
	srv.Store.DB().Model(&store.WebhookLog{}).Count(&left)
	if left != 1 {
		t.Fatalf("remaining logs = %d, want 1 (recent kept)", left)
	}
}
