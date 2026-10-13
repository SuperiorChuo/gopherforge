package api

// 审批流接入（BPM）：合同提交审批 / 审批进度反查 / 回款审批发起（M2）/
// 终态回调承接（合同 + 回款）。开关 BPM_ENABLE=false 时提交与反查端点返回
// 503、回款登记直接 confirmed，其余 CRM 行为与接入前完全一致；回调端点不看
// 开关（内部 token 仍必须）——开关灰度回退期间在途实例的回调仍可落地，
// 避免合同/回款卡死在审批中。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/crm/internal/authjwt"
	"github.com/go-admin-kit/services/crm/internal/bpmclient"
	"github.com/go-admin-kit/services/crm/internal/model"
	"github.com/go-admin-kit/services/crm/internal/store/crm"

	"github.com/go-admin-kit/services/shared/pkg/response"
)

// bpmReady 提交/反查端点公共前置：开关开启且 client 可用。
func (s *Server) bpmReady(c *gin.Context) bool {
	if !s.BPMEnable || s.BPM == nil || !s.BPM.Enabled() {
		response.Error(c, http.StatusServiceUnavailable, "合同审批流未启用（BPM_ENABLE）")
		return false
	}
	return true
}

// SubmitContractApproval handles POST /api/v1/crm/contracts/:id/submit-approval —
// 草拟合同提交审批：CRM 权威生成表单快照 → 调 bpm internal 发起 →
// 成功后合同 draft → approving（approving 态禁编辑，见 UpdateContract）。
func (s *Server) SubmitContractApproval(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	if !s.bpmReady(c) {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	ct, err := s.Store.GetContract(id, u.TenantID)
	if err != nil {
		notFoundOr(c, err, "合同不存在")
		return
	}
	if ct.Status != store.ContractDraft {
		response.Error(c, http.StatusBadRequest, store.ErrContractNotDraft.Error())
		return
	}
	// self_select 节点的选人结果由前端透传（可空）
	var body struct {
		Variables json.RawMessage `json:"variables"`
	}
	_ = c.ShouldBindJSON(&body)

	snapshot, _ := json.Marshal(map[string]any{
		"amount_cents":  ct.AmountCents,
		"customer_id":   ct.CustomerID,
		"owner_user_id": ct.OwnerUserID,
		"no":            ct.No,
		"title":         ct.Title,
	})
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	res, err := s.BPM.StartInstance(ctx, u.TenantID, bpmclient.StartInput{
		DefinitionKey: s.ContractFlowKey,
		Title:         fmt.Sprintf("合同审批：%s（¥%.2f）", ct.Title, float64(ct.AmountCents)/100),
		BizType:       "crm_contract",
		BizID:         strconv.FormatUint(ct.ID, 10),
		FormSnapshot:  snapshot,
		Variables:     body.Variables,
		InitiatorID:   u.UserID,
	})
	if err != nil {
		response.Error(c, http.StatusBadGateway, "发起审批失败: "+err.Error())
		return
	}
	if err := s.Store.MarkContractApproving(ct.ID, u.TenantID); err != nil {
		// 实例已发起但合同状态被并发抢改：明确报错，管理员可在 bpm 侧撤销
		response.Error(c, http.StatusConflict, err.Error()+"（审批实例 "+
			strconv.FormatUint(res.InstanceID, 10)+" 已发起，请联系管理员处理）")
		return
	}
	response.Success(c, gin.H{"instance_id": res.InstanceID, "contract_status": store.ContractApproving})
}

// ContractApprovalInstances handles GET /api/v1/crm/contracts/:id/approval —
// 合同详情页审批进度入口：经 bpm by-biz 反查实例（在途+历史），原样透传。
func (s *Server) ContractApprovalInstances(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	if !s.bpmReady(c) {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	if _, err := s.Store.GetContract(id, u.TenantID); err != nil {
		notFoundOr(c, err, "合同不存在")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	data, err := s.BPM.InstancesByBiz(ctx, u.TenantID, "crm_contract", strconv.FormatUint(id, 10))
	if err != nil {
		response.Error(c, http.StatusBadGateway, "查询审批实例失败: "+err.Error())
		return
	}
	response.Success(c, json.RawMessage(data))
}

// startPaymentApproval 回款登记后发起 bpm 审批实例（M2）：CRM 权威生成
// 表单快照（金额分/合同/方式/日期/登记人），biz_type=crm_payment。
// 返回实例 id；失败由调用方补偿删除 pending 回款。
func (s *Server) startPaymentApproval(c *gin.Context, u *authjwt.AgentClaims, p *model.Payment) (uint64, error) {
	title := fmt.Sprintf("回款审批：合同 #%d（¥%.2f）", p.ContractID, float64(p.AmountCents)/100)
	if ct, err := s.Store.GetContract(p.ContractID, u.TenantID); err == nil {
		title = fmt.Sprintf("回款审批：%s（¥%.2f）", ct.Title, float64(p.AmountCents)/100)
	}
	snapshot, _ := json.Marshal(map[string]any{
		"amount_cents": p.AmountCents,
		"contract_id":  p.ContractID,
		"method":       p.Method,
		"paid_at":      p.PaidAt.Format("2006-01-02"),
		"by_user_id":   p.ByUserID,
	})
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	res, err := s.BPM.StartInstance(ctx, u.TenantID, bpmclient.StartInput{
		DefinitionKey: s.PaymentFlowKey,
		Title:         title,
		BizType:       "crm_payment",
		BizID:         strconv.FormatUint(p.ID, 10),
		FormSnapshot:  snapshot,
		InitiatorID:   u.UserID,
	})
	if err != nil {
		return 0, err
	}
	return res.InstanceID, nil
}

// bpmCallbackReq bpm 终态回调体（契约见 bpm 设计文档 §3.6）。
type bpmCallbackReq struct {
	InstanceID    uint64          `json:"instance_id"`
	DefinitionKey string          `json:"definition_key"`
	BizType       string          `json:"biz_type"`
	BizID         string          `json:"biz_id"`
	Result        string          `json:"result"` // approved|rejected|canceled
	FormSnapshot  json.RawMessage `json:"form_snapshot"`
	FinishedAt    string          `json:"finished_at"`
}

// InternalBpmCallback handles POST /api/v1/crm/internal/bpm/callback —
// bpm 终态回写。合同：approved → active，rejected/canceled → draft；
// 回款（M2）：approved → confirmed，rejected/canceled → rejected。
// 按 (biz_type,biz_id) 幂等；X-Internal-Token 鉴权（CRM_INTERNAL_TOKEN）。
func (s *Server) InternalBpmCallback(c *gin.Context) {
	if !s.requireInternal(c) {
		return
	}
	var req bpmCallbackReq
	if err := c.ShouldBindJSON(&req); err != nil || req.BizID == "" || req.Result == "" {
		response.Error(c, http.StatusBadRequest, "invalid body")
		return
	}
	bizID, err := strconv.ParseUint(req.BizID, 10, 64)
	if err != nil || bizID == 0 {
		response.Error(c, http.StatusBadRequest, "invalid biz_id")
		return
	}
	switch req.BizType {
	case "crm_contract":
		if err := s.Store.ApplyContractApprovalResult(bizID, internalTenant(c), req.Result); err != nil {
			response.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		response.Success(c, gin.H{"contract_id": bizID, "result": req.Result})
	case "crm_payment":
		if err := s.Store.ApplyPaymentApprovalResult(bizID, internalTenant(c), req.Result); err != nil {
			response.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		response.Success(c, gin.H{"payment_id": bizID, "result": req.Result})
	default:
		response.Error(c, http.StatusBadRequest, "未知 biz_type: "+req.BizType)
	}
}
