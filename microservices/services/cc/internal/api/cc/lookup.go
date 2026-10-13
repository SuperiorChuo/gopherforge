package api

// 号码识客聚合（坐席工作台来电弹屏）：一个端点把 cc 侧联系人备注、CRM 客户/
// 线索归属、近 5 通历史合到一起返回，前端识客卡直接渲染。CRM 部分经内部
// HTTP 端点（crmclient）取，未接 CRM 时 has_crm=false、字段留空，不影响其余。

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/cc/internal/store/cc"
)

// lookupContact 是识客卡里的联系人备注块（cc_contacts 命中）。
type lookupContact struct {
	Name   string `json:"name"`
	Remark string `json:"remark"`
}

// lookupRecentCall 是近 5 通历史的精简行（时间/方向/时长/坐席）。
type lookupRecentCall struct {
	CallID      string `json:"call_id"`
	Direction   string `json:"direction"`
	Status      string `json:"status"`
	DurationSec int    `json:"duration_sec"`
	AgentExt    string `json:"agent_ext"`
	StartedAt   string `json:"started_at,omitempty"`
}

// Lookup handles GET /api/v1/cc/lookup?number= — 聚合识客。
func (s *Server) Lookup(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	number := c.Query("number")
	if number == "" {
		fail(c, http.StatusBadRequest, "number is required")
		return
	}
	ctx := c.Request.Context()

	// 1) cc 联系人备注名（LookupContactName 已按号码归一）
	var contact *lookupContact
	if name, err := s.Store.LookupContactName(ctx, agent.TenantID, number); err == nil && name != "" {
		contact = &lookupContact{Name: name}
	}

	// 2) CRM 客户/线索归属（内部 HTTP；未接 CRM 或查失败=留空，不阻塞识客）
	var crmCustomer any
	var crmLead any
	hasCRM := false
	if s.CRM.Enabled() {
		if res, err := s.CRM.Lookup(ctx, agent.TenantID, number); err == nil && res != nil {
			switch res.Matched {
			case "customer":
				if res.Customer != nil {
					crmCustomer = res.Customer
					hasCRM = true
				}
			case "lead":
				if res.Lead != nil {
					crmLead = res.Lead
					hasCRM = true
				}
			}
		}
	}

	// 3) 近 5 通历史（caller 或 callee 命中 number）
	calls, _, err := s.Store.ListCalls(ctx, store.ListCallsQuery{
		TenantID: agent.TenantID,
		Number:   number,
		Page:     1,
		PageSize: 5,
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, "list recent calls failed")
		return
	}
	recent := make([]lookupRecentCall, 0, len(calls))
	for _, call := range calls {
		row := lookupRecentCall{
			CallID:      call.CallID,
			Direction:   call.Direction,
			Status:      call.Status,
			DurationSec: call.DurationSec,
			AgentExt:    call.AgentExt,
		}
		if call.StartedAt != nil {
			row.StartedAt = call.StartedAt.Format("2006-01-02 15:04:05")
		}
		recent = append(recent, row)
	}

	ok(c, gin.H{
		"number":       number,
		"contact":      contact,
		"crm_customer": crmCustomer,
		"crm_lead":     crmLead,
		"has_crm":      hasCRM,
		"recent_calls": recent,
	})
}
