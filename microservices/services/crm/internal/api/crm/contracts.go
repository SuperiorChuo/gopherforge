package api

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/crm/internal/model"
	"github.com/go-admin-kit/services/crm/internal/store/crm"
	"github.com/go-admin-kit/services/shared/pkg/response"
	"net/http"
	"strconv"
)

func (s *Server) ListContracts(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	customerID, _ := strconv.ParseUint(c.Query("customer_id"), 10, 64)
	list, total, err := s.Store.ListContracts(u.TenantID, store.ContractFilter{
		Status: c.Query("status"), CustomerID: customerID, Keyword: c.Query("keyword"),
		Access: s.scopeOf(c, u), Page: pageOf(c),
	})
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, gin.H{"list": list, "total": total})
}

type contractReq struct {
	No            string  `json:"no"`
	Title         string  `json:"title"`
	CustomerID    uint64  `json:"customer_id"`
	OpportunityID *uint64 `json:"opportunity_id"`
	AmountCents   int64   `json:"amount_cents"`
	Status        string  `json:"status"`
	SignedAt      *string `json:"signed_at"`
	StartAt       *string `json:"start_at"`
	EndAt         *string `json:"end_at"`
	Note          string  `json:"note"`
}

func (s *Server) CreateContract(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	var req contractReq
	if err := c.ShouldBindJSON(&req); err != nil || req.CustomerID == 0 {
		response.Error(c, http.StatusBadRequest, "invalid body")
		return
	}
	ct := model.Contract{
		TenantID: u.TenantID, No: req.No, Title: req.Title,
		CustomerID: req.CustomerID, OpportunityID: req.OpportunityID,
		AmountCents: req.AmountCents, Status: defaultStr(req.Status, "draft"),
		SignedAt: parseDate(req.SignedAt), StartAt: parseDate(req.StartAt), EndAt: parseDate(req.EndAt),
		Note: req.Note, OwnerUserID: u.UserID,
	}
	if err := s.Store.CreateContract(c.Request.Context(), &ct); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, ct)
}

func (s *Server) UpdateContract(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
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
	// BPM 收口：审批中的合同禁止编辑与改状态（BPM_ENABLE=false 时不生效，
	// 行为与接入前完全一致）
	if s.BPMEnable && ct.Status == store.ContractApproving {
		response.Error(c, http.StatusBadRequest, "合同审批中，禁止编辑；如需修改请先在审批流中撤销")
		return
	}
	var req contractReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Title != "" {
		ct.Title = req.Title
	}
	if req.AmountCents > 0 {
		ct.AmountCents = req.AmountCents
	}
	if req.Status != "" {
		if req.Status != "draft" && req.Status != "active" && req.Status != "void" {
			response.Error(c, http.StatusBadRequest, "无效合同状态")
			return
		}
		// BPM 收口：开启审批后合同生效必须走审批流，禁止裸改 active
		if s.BPMEnable && req.Status == "active" && ct.Status != "active" {
			response.Error(c, http.StatusBadRequest, "已启用合同审批，请通过「提交审批」使合同生效")
			return
		}
		ct.Status = req.Status
	}
	if d := parseDate(req.SignedAt); d != nil {
		ct.SignedAt = d
	}
	if d := parseDate(req.StartAt); d != nil {
		ct.StartAt = d
	}
	if d := parseDate(req.EndAt); d != nil {
		ct.EndAt = d
	}
	ct.Note = req.Note
	if err := s.Store.UpdateContract(c.Request.Context(), ct); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, ct)
}

func (s *Server) GetContract(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
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
	payments, _ := s.Store.ListPayments(ct.ID, u.TenantID)
	var paid int64
	for _, p := range payments {
		if p.Status == store.PaymentConfirmed { // 只计已确认（BPM M2 口径）
			paid += p.AmountCents
		}
	}
	response.Success(c, gin.H{"contract": ct, "payments": payments, "paid_cents": paid})
}

type paymentReq struct {
	AmountCents int64   `json:"amount_cents"`
	PaidAt      *string `json:"paid_at"`
	Method      string  `json:"method"`
	Note        string  `json:"note"`
}

func (s *Server) AddPayment(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	var req paymentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid body")
		return
	}
	// BPM M2 回款审批：开关开启且 client 可用 → 登记即 pending_approval 并
	// 发起审批实例；否则与接入前完全一致（直接 confirmed）。
	useBpm := s.BPMEnable && s.BPM != nil && s.BPM.Enabled()
	p := model.Payment{
		TenantID: u.TenantID, ContractID: id, AmountCents: req.AmountCents,
		Method: defaultStr(req.Method, "transfer"), Note: req.Note, ByUserID: u.UserID,
		Status: store.PaymentConfirmed,
	}
	if useBpm {
		p.Status = store.PaymentPendingApproval
	}
	if d := parseDate(req.PaidAt); d != nil {
		p.PaidAt = *d
	}
	if err := s.Store.AddPayment(c.Request.Context(), &p); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	if !useBpm {
		response.Success(c, p)
		return
	}
	instanceID, err := s.startPaymentApproval(c, u, &p)
	if err != nil {
		// 补偿：实例未发起成功，删除刚登记的 pending 回款，避免悬挂
		_ = s.Store.DeletePendingPayment(c.Request.Context(), p.ID, u.TenantID)
		response.Error(c, http.StatusBadGateway, "发起回款审批失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"payment": p, "instance_id": instanceID, "status": p.Status})
}
