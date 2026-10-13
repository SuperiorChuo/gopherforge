package api

// 回款审批接入（BPM M2）用例：BPM_ENABLE 关闭时零影响（登记即 confirmed）/
// 开启后登记 pending_approval + 发起实例（契约体校验）/ approved 前不计入
// 回款统计（stats + 合同详情 paid_cents）/ 回调回写（approved→confirmed、
// rejected→rejected、幂等）/ 发起失败补偿删除 / 超额校验计入审批中金额。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/crm/internal/bpmclient"
	"github.com/go-admin-kit/services/crm/internal/model"
	"github.com/go-admin-kit/services/crm/internal/store/crm"
)

// activateContract 直接置合同 active（绕过 BPM 开启后的裸改收口，测试铺数据用）。
func activateContract(t *testing.T, srv *Server, id uint64) {
	t.Helper()
	if err := srv.Store.DB().Model(&model.Contract{}).
		Where("id = ?", id).Update("status", "active").Error; err != nil {
		t.Fatalf("activate contract: %v", err)
	}
}

func statsPaidCents(t *testing.T, r *gin.Engine) int64 {
	t.Helper()
	w, env := call(t, r, http.MethodGet, "/api/v1/crm/stats?days=30", nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("stats: %d", w.Code)
	}
	var res struct {
		Stats struct {
			PaidCents int64 `json:"paid_cents"`
		} `json:"stats"`
	}
	decode(t, env.Data, &res)
	return res.Stats.PaidCents
}

func contractPaidCents(t *testing.T, r *gin.Engine, id uint64) int64 {
	t.Helper()
	w, env := call(t, r, http.MethodGet, "/api/v1/crm/contracts/"+u64s(id), nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("get contract: %d", w.Code)
	}
	var res struct {
		PaidCents int64 `json:"paid_cents"`
	}
	decode(t, env.Data, &res)
	return res.PaidCents
}

// BPM 关闭（默认）：登记即 confirmed，统计立即计入——行为与接入前一致。
func TestPaymentBpmDisabled(t *testing.T) {
	r, srv := newTestRouterWithServer(t)
	id := seedContract(t, r, "PAY-D01")
	activateContract(t, srv, id)

	w, env := call(t, r, http.MethodPost, "/api/v1/crm/contracts/"+u64s(id)+"/payments",
		map[string]any{"amount_cents": 50000}, "")
	if w.Code != http.StatusOK {
		t.Fatalf("payment: %d %s", w.Code, w.Body.String())
	}
	var p model.Payment
	decode(t, env.Data, &p)
	if p.Status != store.PaymentConfirmed {
		t.Fatalf("关闭时应直接 confirmed: %s", p.Status)
	}
	if got := statsPaidCents(t, r); got != 50000 {
		t.Fatalf("stats paid_cents = %d", got)
	}
	if got := contractPaidCents(t, r, id); got != 50000 {
		t.Fatalf("contract paid_cents = %d", got)
	}
}

// BPM 开启：登记 → pending_approval + 发起实例；approved 前不计入统计；
// 回调 approved → confirmed 计入；rejected → rejected 不计入；幂等；
// 超额校验计入审批中的金额。
func TestPaymentApprovalFlow(t *testing.T) {
	var gotStart struct {
		DefinitionKey string          `json:"definition_key"`
		BizType       string          `json:"biz_type"`
		BizID         string          `json:"biz_id"`
		InitiatorID   uint64          `json:"initiator_id"`
		FormSnapshot  json.RawMessage `json:"form_snapshot"`
	}
	bpmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_ = json.NewDecoder(req.Body).Decode(&gotStart)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 200, "message": "success",
			"data": map[string]any{"instance_id": 88, "status": "running"},
		})
	}))
	defer bpmSrv.Close()

	r, srv := newTestRouterWithServer(t)
	srv.BPMEnable = true
	srv.InternalToken = "itok"
	srv.BPM = bpmclient.New(bpmSrv.URL, "btok")
	srv.PaymentFlowKey = "crm_payment_approval"
	id := seedContract(t, r, "PAY-101") // 合同额 120000
	activateContract(t, srv, id)

	// 登记回款 → pending_approval + 实例发起。日期保持在 30 天统计窗口内，
	// 避免固定日期随时间推移使审批状态断言被统计区间前提干扰。
	paidAt := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	w, env := call(t, r, http.MethodPost, "/api/v1/crm/contracts/"+u64s(id)+"/payments",
		map[string]any{"amount_cents": 50000, "method": "transfer", "paid_at": paidAt}, "")
	if w.Code != http.StatusOK {
		t.Fatalf("payment: %d %s", w.Code, w.Body.String())
	}
	var res struct {
		Payment    model.Payment `json:"payment"`
		InstanceID uint64        `json:"instance_id"`
		Status     string        `json:"status"`
	}
	decode(t, env.Data, &res)
	if res.InstanceID != 88 || res.Status != store.PaymentPendingApproval ||
		res.Payment.Status != store.PaymentPendingApproval {
		t.Fatalf("登记结果: %+v", res)
	}
	// bpm 收到的契约体
	if gotStart.DefinitionKey != "crm_payment_approval" || gotStart.BizType != "crm_payment" ||
		gotStart.BizID != u64s(res.Payment.ID) || gotStart.InitiatorID != 9 {
		t.Fatalf("bpm start body: %+v", gotStart)
	}
	var form struct {
		AmountCents int64  `json:"amount_cents"`
		ContractID  uint64 `json:"contract_id"`
		Method      string `json:"method"`
		PaidAt      string `json:"paid_at"`
		ByUserID    uint64 `json:"by_user_id"`
	}
	decode(t, gotStart.FormSnapshot, &form)
	if form.AmountCents != 50000 || form.ContractID != id || form.Method != "transfer" ||
		form.PaidAt != paidAt || form.ByUserID != 9 {
		t.Fatalf("form snapshot: %+v", form)
	}
	// approved 前不计入回款统计（M2 验收）
	if got := statsPaidCents(t, r); got != 0 {
		t.Fatalf("审批前 stats paid_cents 应为 0, got %d", got)
	}
	if got := contractPaidCents(t, r, id); got != 0 {
		t.Fatalf("审批前合同 paid_cents 应为 0, got %d", got)
	}
	// 超额校验计入审批中金额：50000 pending + 80000 > 120000 → 拒绝
	if w, _ := call(t, r, http.MethodPost, "/api/v1/crm/contracts/"+u64s(id)+"/payments",
		map[string]any{"amount_cents": 80000}, ""); w.Code != http.StatusBadRequest {
		t.Fatalf("超额（含审批中）应 400, got %d", w.Code)
	}

	// 回调 approved → confirmed，计入统计；重复回调幂等
	hdr := map[string]string{"X-Internal-Token": "itok", "X-Tenant-ID": "1"}
	cbBody := func(result string, payID uint64) map[string]any {
		return map[string]any{
			"instance_id": 88, "definition_key": "crm_payment_approval",
			"biz_type": "crm_payment", "biz_id": u64s(payID), "result": result,
		}
	}
	for i := 0; i < 2; i++ {
		if w, _ := callHdr(t, r, http.MethodPost, "/api/v1/crm/internal/bpm/callback",
			cbBody("approved", res.Payment.ID), hdr); w.Code != http.StatusOK {
			t.Fatalf("callback approved 第 %d 次: %d", i+1, w.Code)
		}
	}
	if got := statsPaidCents(t, r); got != 50000 {
		t.Fatalf("审批后 stats paid_cents = %d", got)
	}
	if got := contractPaidCents(t, r, id); got != 50000 {
		t.Fatalf("审批后合同 paid_cents = %d", got)
	}

	// 第二笔 → rejected：保留记录但不计入统计
	w, env = call(t, r, http.MethodPost, "/api/v1/crm/contracts/"+u64s(id)+"/payments",
		map[string]any{"amount_cents": 30000}, "")
	if w.Code != http.StatusOK {
		t.Fatalf("payment2: %d %s", w.Code, w.Body.String())
	}
	decode(t, env.Data, &res)
	if w, _ := callHdr(t, r, http.MethodPost, "/api/v1/crm/internal/bpm/callback",
		cbBody("rejected", res.Payment.ID), hdr); w.Code != http.StatusOK {
		t.Fatalf("callback rejected: %d", w.Code)
	}
	if got := statsPaidCents(t, r); got != 50000 {
		t.Fatalf("驳回后 stats paid_cents = %d", got)
	}
	payments, err := srv.Store.ListPayments(id, 1)
	if err != nil || len(payments) != 2 {
		t.Fatalf("payments: err=%v n=%d", err, len(payments))
	}
	statuses := map[string]int{}
	for _, p := range payments {
		statuses[p.Status]++
	}
	if statuses[store.PaymentConfirmed] != 1 || statuses[store.PaymentRejected] != 1 {
		t.Fatalf("回款状态分布: %+v", statuses)
	}
	// 驳回释放额度：30000 可重新登记（confirmed 50000 + rejected 30000 排除 + 新 70000 = 120000）
	if w, _ := call(t, r, http.MethodPost, "/api/v1/crm/contracts/"+u64s(id)+"/payments",
		map[string]any{"amount_cents": 70000}, ""); w.Code != http.StatusOK {
		t.Fatalf("驳回后重新登记应允许, got %d", w.Code)
	}
}

// 发起实例失败：登记补偿删除，不留悬挂的 pending 回款。
func TestPaymentApprovalStartFailureCompensates(t *testing.T) {
	bpmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bpmSrv.Close()

	r, srv := newTestRouterWithServer(t)
	srv.BPMEnable = true
	srv.BPM = bpmclient.New(bpmSrv.URL, "btok")
	srv.PaymentFlowKey = "crm_payment_approval"
	id := seedContract(t, r, "PAY-201")
	activateContract(t, srv, id)

	if w, _ := call(t, r, http.MethodPost, "/api/v1/crm/contracts/"+u64s(id)+"/payments",
		map[string]any{"amount_cents": 50000}, ""); w.Code != http.StatusBadGateway {
		t.Fatalf("发起失败应 502, got %d", w.Code)
	}
	payments, err := srv.Store.ListPayments(id, 1)
	if err != nil || len(payments) != 0 {
		t.Fatalf("补偿后不应残留回款: err=%v n=%d", err, len(payments))
	}
}
