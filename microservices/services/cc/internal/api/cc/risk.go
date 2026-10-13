package api

import (
	"context"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/cc/internal/store/cc"
)

var (
	riskNumberRe      = regexp.MustCompile(`^\+?[0-9*#]{3,32}$`)
	riskListPrefixRe  = regexp.MustCompile(`^\+?[0-9*#]{1,16}$`)
	riskHMRe          = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
	riskBlockedPrefRe = regexp.MustCompile(`^[0-9]{1,16}$`)
)

// GetRiskPolicy handles GET /api/v1/cc/risk/policy — 不存在则建默认空策略。
func (s *Server) GetRiskPolicy(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	p, err := s.Store.GetOrCreatePolicy(c.Request.Context(), agent.TenantID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "load risk policy failed")
		return
	}
	ok(c, p)
}

type saveRiskPolicyReq struct {
	OutboundStart            string `json:"outbound_start"`
	OutboundEnd              string `json:"outbound_end"`
	OutboundWeekdays         string `json:"outbound_weekdays"`
	MaxCallsPerNumberPerHour int    `json:"max_calls_per_number_per_hour"`
	MaxCallsPerAgentPerHour  int    `json:"max_calls_per_agent_per_hour"`
	MaxCallsPerTenantPerMin  int    `json:"max_calls_per_tenant_per_min"`
	BlockedPrefixes          string `json:"blocked_prefixes"`
	MaxRatePerMin            int64  `json:"max_rate_per_min"`
}

// SaveRiskPolicy handles PUT /api/v1/cc/risk/policy。
func (s *Server) SaveRiskPolicy(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	var req saveRiskPolicyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	req.OutboundStart = strings.TrimSpace(req.OutboundStart)
	req.OutboundEnd = strings.TrimSpace(req.OutboundEnd)
	req.OutboundWeekdays = strings.TrimSpace(req.OutboundWeekdays)
	req.BlockedPrefixes = strings.TrimSpace(req.BlockedPrefixes)
	if req.OutboundStart != "" && !riskHMRe.MatchString(req.OutboundStart) {
		fail(c, http.StatusBadRequest, "invalid outbound_start")
		return
	}
	if req.OutboundEnd != "" && !riskHMRe.MatchString(req.OutboundEnd) {
		fail(c, http.StatusBadRequest, "invalid outbound_end")
		return
	}
	if req.MaxCallsPerNumberPerHour < 0 || req.MaxCallsPerNumberPerHour > 100000 ||
		req.MaxCallsPerAgentPerHour < 0 || req.MaxCallsPerAgentPerHour > 100000 ||
		req.MaxCallsPerTenantPerMin < 0 || req.MaxCallsPerTenantPerMin > 10000 ||
		req.MaxRatePerMin < 0 {
		fail(c, http.StatusBadRequest, "invalid limit")
		return
	}
	if req.OutboundWeekdays != "" {
		for _, p := range strings.Split(req.OutboundWeekdays, ",") {
			p = strings.TrimSpace(p)
			n, err := strconv.Atoi(p)
			if err != nil || n < 1 || n > 7 {
				fail(c, http.StatusBadRequest, "invalid outbound_weekdays")
				return
			}
		}
	}
	if req.BlockedPrefixes != "" {
		parts := strings.Split(req.BlockedPrefixes, ",")
		clean := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if !riskBlockedPrefRe.MatchString(p) {
				fail(c, http.StatusBadRequest, "invalid blocked_prefixes")
				return
			}
			clean = append(clean, p)
		}
		req.BlockedPrefixes = strings.Join(clean, ",")
	}
	out, err := s.Store.SavePolicy(c.Request.Context(), &store.CCRiskPolicy{
		TenantID:                 agent.TenantID,
		OutboundStart:            req.OutboundStart,
		OutboundEnd:              req.OutboundEnd,
		OutboundWeekdays:         req.OutboundWeekdays,
		MaxCallsPerNumberPerHour: req.MaxCallsPerNumberPerHour,
		MaxCallsPerAgentPerHour:  req.MaxCallsPerAgentPerHour,
		MaxCallsPerTenantPerMin:  req.MaxCallsPerTenantPerMin,
		BlockedPrefixes:          req.BlockedPrefixes,
		MaxRatePerMin:            req.MaxRatePerMin,
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, "save risk policy failed")
		return
	}
	ok(c, out)
}

// ListRiskLists handles GET /api/v1/cc/risk/lists。
func (s *Server) ListRiskLists(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	list, err := s.Store.ListRiskLists(c.Request.Context(), agent.TenantID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "list risk lists failed")
		return
	}
	ok(c, gin.H{"list": list, "total": len(list)})
}

type createRiskListReq struct {
	Number   string `json:"number"`
	Match    string `json:"match"`
	ListType string `json:"list_type"`
	Remark   string `json:"remark"`
}

// CreateRiskList handles POST /api/v1/cc/risk/lists。
func (s *Server) CreateRiskList(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	var req createRiskListReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	req.Number = strings.TrimSpace(req.Number)
	req.Match = strings.TrimSpace(req.Match)
	req.ListType = strings.TrimSpace(req.ListType)
	if req.Match == "" {
		req.Match = "exact"
	}
	numRe := riskNumberRe
	if req.Match == "prefix" {
		numRe = riskListPrefixRe
	}
	if !numRe.MatchString(req.Number) {
		fail(c, http.StatusBadRequest, "invalid number")
		return
	}
	if len(req.Remark) > 128 {
		req.Remark = req.Remark[:128]
	}
	out, err := s.Store.CreateRiskList(c.Request.Context(), &store.CCRiskListEntry{
		TenantID: agent.TenantID,
		Number:   req.Number,
		Match:    req.Match,
		ListType: req.ListType,
		Remark:   strings.TrimSpace(req.Remark),
	})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") ||
			strings.Contains(strings.ToLower(err.Error()), "unique") {
			fail(c, http.StatusConflict, "number already in list")
			return
		}
		if strings.Contains(err.Error(), "match") || strings.Contains(err.Error(), "list_type") || strings.Contains(err.Error(), "number") {
			fail(c, http.StatusBadRequest, err.Error())
			return
		}
		fail(c, http.StatusInternalServerError, "save risk list failed")
		return
	}
	ok(c, out)
}

// DeleteRiskList handles DELETE /api/v1/cc/risk/lists/:id。
func (s *Server) DeleteRiskList(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	deleted, err := s.Store.DeleteRiskList(c.Request.Context(), agent.TenantID, id)
	if err != nil {
		fail(c, http.StatusInternalServerError, "delete risk list failed")
		return
	}
	ok(c, gin.H{"deleted": deleted})
}

// evaluateOutboundRisk 纯判定，不写 HTTP。record=true 时在放行后记一次尝试。
func (s *Server) evaluateOutboundRisk(ctx context.Context, tenantID uint64, fromExt, to string, ratePerMin int64, record bool) (store.RiskDecision, error) {
	policy, err := s.Store.GetOrCreatePolicy(ctx, tenantID)
	if err != nil {
		return store.RiskDecision{}, err
	}
	lists, err := s.Store.ListRiskLists(ctx, tenantID)
	if err != nil {
		return store.RiskDecision{}, err
	}
	now := time.Now()
	counts, err := s.Store.CountOutboundAttempts(ctx, tenantID, fromExt, to, now)
	if err != nil {
		return store.RiskDecision{}, err
	}
	d := store.EvaluateOutbound(now, store.AsiaShanghai(), store.OutboundRiskInput{
		Callee:     to,
		FromExt:    fromExt,
		RatePerMin: ratePerMin,
		Policy:     *policy,
		Lists:      lists,
		Counts:     counts,
	})
	if d.Allow && record {
		if err := s.Store.RecordRiskAttempt(ctx, tenantID, fromExt, to, now); err != nil {
			return store.RiskDecision{}, err
		}
	}
	return d, nil
}

// enforceOutboundRisk 在 originate 计费检查之后、打 control-api 之前执行。
// 返回 false 时已写响应。空策略必须放行。
func (s *Server) enforceOutboundRisk(c *gin.Context, tenantID uint64, fromExt, to string, ratePerMin int64) bool {
	d, err := s.evaluateOutboundRisk(c.Request.Context(), tenantID, fromExt, to, ratePerMin, true)
	if err != nil {
		fail(c, http.StatusInternalServerError, "risk check failed")
		return false
	}
	if !d.Allow {
		status := d.Status
		if status == 0 {
			status = http.StatusForbidden
		}
		fail(c, status, d.Reason)
		return false
	}
	return true
}
