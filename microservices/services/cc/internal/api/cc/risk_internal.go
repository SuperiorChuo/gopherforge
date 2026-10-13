package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type internalRiskReq struct {
	FromExt string `json:"from_ext"`
	To      string `json:"to"`
}

// InternalRiskCheck handles POST /api/v1/cc/internal/risk-check
// 给 control-api 软电话出局用：HMAC 与 webhook 同源（X-CC-Signature）。
func (s *Server) InternalRiskCheck(c *gin.Context) {
	if s.ProductionMode && s.WebhookSecret == "" {
		fail(c, http.StatusServiceUnavailable, "risk-check disabled: CC_WEBHOOK_SECRET is not configured")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<16))
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
	var req internalRiskReq
	if err := json.Unmarshal(raw, &req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	req.FromExt = strings.TrimSpace(req.FromExt)
	req.To = strings.TrimSpace(req.To)
	if !fsNumberRe.MatchString(req.To) {
		fail(c, http.StatusBadRequest, "invalid to")
		return
	}
	ctx := c.Request.Context()
	tenantID := uint64(1)
	if req.FromExt != "" {
		if bound, err := s.Store.GetAgentByExtensionAny(ctx, req.FromExt); err == nil && bound != nil {
			tenantID = bound.TenantID
		}
	}
	acc, err := s.Store.GetOrCreateAccount(ctx, tenantID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "load billing account failed")
		return
	}
	if acc.Status == "frozen" {
		fail(c, http.StatusForbidden, "账户已冻结")
		return
	}
	rate, err := s.Store.MatchRate(ctx, tenantID, req.To)
	if err != nil {
		fail(c, http.StatusInternalServerError, "match rate failed")
		return
	}
	var ratePerMin int64
	if rate != nil {
		ratePerMin = rate.RatePerMin
		if rate.RatePerMin > 0 {
			avail := acc.Balance + acc.CreditLimit
			if avail <= 0 {
				fail(c, http.StatusForbidden, "余额不足")
				return
			}
		}
	}
	fromExt := req.FromExt
	if fromExt == "" {
		fromExt = "softphone"
	}
	d, err := s.evaluateOutboundRisk(ctx, tenantID, fromExt, req.To, ratePerMin, true)
	if err != nil {
		fail(c, http.StatusInternalServerError, "risk check failed")
		return
	}
	if !d.Allow {
		status := d.Status
		if status == 0 {
			status = http.StatusForbidden
		}
		fail(c, status, d.Reason)
		return
	}
	ok(c, gin.H{"allow": true})
}
