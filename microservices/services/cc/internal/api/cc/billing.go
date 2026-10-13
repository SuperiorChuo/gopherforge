package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/cc/internal/store/cc"
)

// 计费管理 API：费率 CRUD、账户余额/充值、话费流水。金额一律以「分」传输。
// 鉴权走 requireAgent（网关头优先，Bearer JWT 兜底），租户从 agent.TenantID 取。

// ---- 费率 ----

// ListRates handles GET /api/v1/cc/billing/rates。
func (s *Server) ListRates(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	list, err := s.Store.ListRates(c.Request.Context(), agent.TenantID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "list rates failed")
		return
	}
	ok(c, gin.H{"list": list})
}

type upsertRateReq struct {
	Prefix      string `json:"prefix"`
	Name        string `json:"name"`
	RatePerMin  int64  `json:"rate_per_min"`
	BillingStep int    `json:"billing_step"`
	ConnectFee  int64  `json:"connect_fee"`
}

// UpsertRate handles POST /api/v1/cc/billing/rates。
func (s *Server) UpsertRate(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	var req upsertRateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	req.Prefix = strings.TrimSpace(req.Prefix)
	if req.Prefix == "" || req.RatePerMin < 0 || req.ConnectFee < 0 {
		fail(c, http.StatusBadRequest, "invalid prefix or amount")
		return
	}
	out, err := s.Store.UpsertRate(c.Request.Context(), &store.CCRate{
		TenantID:    agent.TenantID,
		Prefix:      req.Prefix,
		Name:        req.Name,
		RatePerMin:  req.RatePerMin,
		BillingStep: req.BillingStep,
		ConnectFee:  req.ConnectFee,
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, "save rate failed")
		return
	}
	ok(c, out)
}

// DeleteRate handles DELETE /api/v1/cc/billing/rates/:id。
func (s *Server) DeleteRate(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	deleted, err := s.Store.DeleteRate(c.Request.Context(), agent.TenantID, id)
	if err != nil {
		fail(c, http.StatusInternalServerError, "delete rate failed")
		return
	}
	ok(c, gin.H{"deleted": deleted})
}

// ---- 账户 ----

// GetAccount handles GET /api/v1/cc/billing/account — 余额 + 本月/今日消费合计。
func (s *Server) GetAccount(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	ctx := c.Request.Context()
	acc, err := s.Store.GetOrCreateAccount(ctx, agent.TenantID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "load account failed")
		return
	}
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	monthSpent, err := s.Store.SumCharges(ctx, agent.TenantID, monthStart, now)
	if err != nil {
		fail(c, http.StatusInternalServerError, "sum month failed")
		return
	}
	todaySpent, err := s.Store.SumCharges(ctx, agent.TenantID, dayStart, now)
	if err != nil {
		fail(c, http.StatusInternalServerError, "sum today failed")
		return
	}
	ok(c, gin.H{
		"account":     acc,
		"month_spent": monthSpent, // 分
		"today_spent": todaySpent, // 分
	})
}

type rechargeReq struct {
	AmountFen int64  `json:"amount_fen"` // 正=充值，负=调减
	Remark    string `json:"remark"`     // 留痕备注（进资金流水）
}

// Recharge handles POST /api/v1/cc/billing/recharge。
func (s *Server) Recharge(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	var req rechargeReq
	if err := c.ShouldBindJSON(&req); err != nil || req.AmountFen == 0 {
		fail(c, http.StatusBadRequest, "invalid amount_fen")
		return
	}
	acc, err := s.Store.Recharge(c.Request.Context(), agent.TenantID, req.AmountFen, agent.UserID, req.Remark)
	if err != nil {
		fail(c, http.StatusInternalServerError, "recharge failed")
		return
	}
	ok(c, acc)
}

// ListAccountTxns handles GET /api/v1/cc/billing/txns — 本租户资金流水（充值/调减留痕）。
func (s *Server) ListAccountTxns(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	list, total, err := s.Store.ListAccountTxns(c.Request.Context(), agent.TenantID, page, pageSize)
	if err != nil {
		fail(c, http.StatusInternalServerError, "list txns failed")
		return
	}
	ok(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

// ---- 话费流水 ----

// ListCharges handles GET /api/v1/cc/billing/charges?page=&page_size=&from=&to=。
func (s *Server) ListCharges(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	q := store.ListChargesQuery{TenantID: agent.TenantID, Page: page, PageSize: pageSize}
	if from := parseTimeParam(c.Query("from")); from != nil {
		q.From = from
	}
	if to := parseTimeParam(c.Query("to")); to != nil {
		q.To = to
	}
	list, total, err := s.Store.ListCharges(c.Request.Context(), q)
	if err != nil {
		fail(c, http.StatusInternalServerError, "list charges failed")
		return
	}
	ok(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

// parseTimeParam 接受 RFC3339 或 "YYYY-MM-DD HH:mm:ss" / "YYYY-MM-DD"，解析失败返回 nil。
func parseTimeParam(v string) *time.Time {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, v); err == nil {
			return &t
		}
	}
	return nil
}
