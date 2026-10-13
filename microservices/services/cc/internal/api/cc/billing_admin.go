package api

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/cc/internal/authjwt"
	"github.com/go-admin-kit/services/cc/internal/store/cc"
)

// 平台侧账务管理：全租户总览 / 代充值 / 冻结 / 月账单出账与导出。
// 权限闸门：仅平台租户（tenant=1）的用户可用，其余 403。

// requirePlatformAdmin 在 requireAgent 之上再收口到平台租户。
func (s *Server) requirePlatformAdmin(c *gin.Context) (*authjwt.AgentClaims, bool) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return nil, false
	}
	if agent.TenantID != 1 {
		fail(c, http.StatusForbidden, "platform admin only")
		return nil, false
	}
	return agent, true
}

// AdminAccountsOverview handles GET /api/v1/cc/billing/admin/overview。
func (s *Server) AdminAccountsOverview(c *gin.Context) {
	if _, authorized := s.requirePlatformAdmin(c); !authorized {
		return
	}
	list, err := s.Store.ListAccountsOverview(c.Request.Context())
	if err != nil {
		fail(c, http.StatusInternalServerError, "load overview failed")
		return
	}
	ok(c, gin.H{"list": list, "total": len(list)})
}

type adminRechargeReq struct {
	TenantID  uint64 `json:"tenant_id"`
	AmountFen int64  `json:"amount_fen"`
	Remark    string `json:"remark"`
}

// AdminRecharge handles POST /api/v1/cc/billing/admin/recharge — 给任意租户充值/调减。
func (s *Server) AdminRecharge(c *gin.Context) {
	agent, authorized := s.requirePlatformAdmin(c)
	if !authorized {
		return
	}
	var req adminRechargeReq
	if err := c.ShouldBindJSON(&req); err != nil || req.TenantID == 0 || req.AmountFen == 0 {
		fail(c, http.StatusBadRequest, "tenant_id and non-zero amount_fen required")
		return
	}
	acc, err := s.Store.Recharge(c.Request.Context(), req.TenantID, req.AmountFen, agent.UserID, req.Remark)
	if err != nil {
		fail(c, http.StatusInternalServerError, "recharge failed")
		return
	}
	ok(c, acc)
}

type adminStatusReq struct {
	TenantID uint64 `json:"tenant_id"`
	Status   string `json:"status"` // normal / frozen
}

// AdminSetStatus handles POST /api/v1/cc/billing/admin/status — 冻结/解冻任意租户。
// 联动媒体面：冻结批量停用该租户名下线路（注册断+直拨断），解冻恢复；联动失败
// 不阻塞冻结本身（响应带 line_sync_error，媒体面重启自愈会按 DB 对齐）。
func (s *Server) AdminSetStatus(c *gin.Context) {
	if _, authorized := s.requirePlatformAdmin(c); !authorized {
		return
	}
	var req adminStatusReq
	if err := c.ShouldBindJSON(&req); err != nil || req.TenantID == 0 ||
		(req.Status != "normal" && req.Status != "frozen") {
		fail(c, http.StatusBadRequest, "tenant_id and status(normal|frozen) required")
		return
	}
	acc, err := s.Store.SetAccountStatus(c.Request.Context(), req.TenantID, req.Status)
	if err != nil {
		fail(c, http.StatusInternalServerError, "set status failed")
		return
	}
	lines, syncErr := s.syncTenantLineState(c.Request.Context(), req.TenantID, req.Status == "frozen")
	resp := gin.H{"account": acc, "lines_affected": lines}
	if syncErr != nil {
		log.Printf("cc billing: tenant=%d line sync failed (status=%s applied anyway): %v", req.TenantID, req.Status, syncErr)
		resp["line_sync_error"] = syncErr.Error()
	}
	ok(c, resp)
}

// AdminListTxns handles GET /api/v1/cc/billing/admin/txns?tenant_id= — 任意租户资金流水。
func (s *Server) AdminListTxns(c *gin.Context) {
	if _, authorized := s.requirePlatformAdmin(c); !authorized {
		return
	}
	tenantID, _ := strconv.ParseUint(c.Query("tenant_id"), 10, 64)
	if tenantID == 0 {
		fail(c, http.StatusBadRequest, "tenant_id required")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	list, total, err := s.Store.ListAccountTxns(c.Request.Context(), tenantID, page, pageSize)
	if err != nil {
		fail(c, http.StatusInternalServerError, "list txns failed")
		return
	}
	ok(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

// ---- 月账单 ----

type generateBillsReq struct {
	Period string `json:"period"` // YYYY-MM
}

// AdminGenerateBills handles POST /api/v1/cc/billing/admin/bills/generate。
// 幂等重算：同账期重复生成按最新流水覆盖。
func (s *Server) AdminGenerateBills(c *gin.Context) {
	if _, authorized := s.requirePlatformAdmin(c); !authorized {
		return
	}
	var req generateBillsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	if _, _, err := store.PeriodRange(req.Period); err != nil {
		fail(c, http.StatusBadRequest, "period must be YYYY-MM")
		return
	}
	n, err := s.Store.GenerateBills(c.Request.Context(), req.Period)
	if err != nil {
		fail(c, http.StatusInternalServerError, "generate bills failed")
		return
	}
	ok(c, gin.H{"period": req.Period, "generated": n})
}

// ListBills handles GET /api/v1/cc/billing/bills?period= — 平台租户看全部账单，
// 普通租户只看自己的（同一端点双态，省一条路由）。
func (s *Server) ListBills(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	scope := agent.TenantID
	if scope == 1 {
		scope = 0 // 平台侧看全部
	}
	list, err := s.Store.ListBills(c.Request.Context(), scope, strings.TrimSpace(c.Query("period")))
	if err != nil {
		fail(c, http.StatusInternalServerError, "list bills failed")
		return
	}
	ok(c, gin.H{"list": list, "total": len(list)})
}

// ExportBillCSV handles GET /api/v1/cc/billing/bills/:id/export — 账单明细 CSV。
// 平台租户可导任意账单，普通租户仅自己的。带 UTF-8 BOM 便于 Excel 直开。
func (s *Server) ExportBillCSV(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	bill, err := s.Store.GetBill(c.Request.Context(), id)
	if err != nil {
		if store.IsNotFound(err) {
			fail(c, http.StatusNotFound, "bill not found")
			return
		}
		fail(c, http.StatusInternalServerError, "load bill failed")
		return
	}
	if agent.TenantID != 1 && bill.TenantID != agent.TenantID {
		fail(c, http.StatusForbidden, "not your bill")
		return
	}
	from, to, err := store.PeriodRange(bill.Period)
	if err != nil {
		fail(c, http.StatusInternalServerError, "bad period on bill")
		return
	}
	charges, err := s.Store.ChargesInRange(c.Request.Context(), bill.TenantID, from, to)
	if err != nil {
		fail(c, http.StatusInternalServerError, "load charges failed")
		return
	}

	var b strings.Builder
	b.WriteString("\uFEFF") // BOM：Excel 识别 UTF-8
	fmt.Fprintf(&b, "账单,%s,租户,%d,通话数,%d,计费时长(秒),%d,合计(元),%.2f\n",
		bill.Period, bill.TenantID, bill.CallCount, bill.TotalBillSec, float64(bill.TotalAmount)/100)
	b.WriteString("时间,通话ID,主叫,被叫,计费时长(秒),金额(元),扣后余额(元)\n")
	for _, ch := range charges {
		fmt.Fprintf(&b, "%s,%s,%s,%s,%d,%.2f,%.2f\n",
			ch.CreatedAt.Format("2006-01-02 15:04:05"), csvSafe(ch.CallID),
			csvSafe(ch.Caller), csvSafe(ch.Callee), ch.BillSec,
			float64(ch.Amount)/100, float64(ch.BalanceAfter)/100)
	}
	filename := fmt.Sprintf("cc-bill-%d-%s.csv", bill.TenantID, bill.Period)
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Data(http.StatusOK, "text/csv; charset=utf-8", []byte(b.String()))
}

// csvSafe 防 CSV 公式注入 + 去掉分隔符干扰（号码/UUID 本应无逗号，纵深防御）。
func csvSafe(v string) string {
	v = strings.NewReplacer(",", " ", "\n", " ", "\r", " ").Replace(v)
	if v != "" && strings.ContainsRune("=+-@", rune(v[0])) {
		v = "'" + v
	}
	return v
}
