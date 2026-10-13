package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/pay/internal/channel"
	"github.com/go-admin-kit/services/pay/internal/model"
	"github.com/go-admin-kit/services/pay/internal/store/pay"
)

func storeErrStatus(err error) (int, string) {
	switch {
	case errors.Is(err, store.ErrOrderNotFound):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, store.ErrOrderTerminal), errors.Is(err, store.ErrOrderNotPaid),
		errors.Is(err, store.ErrRefundExceeds), errors.Is(err, store.ErrAmountInvalid),
		errors.Is(err, store.ErrChannelExists), errors.Is(err, store.ErrChannelDisabled),
		errors.Is(err, store.ErrTokenExpired):
		return http.StatusBadRequest, err.Error()
	default:
		return http.StatusInternalServerError, "内部错误"
	}
}

// ---- 管理面 ----

func (s *Server) StatsHandler(c *gin.Context) {
	claims, okk := s.requireUser(c)
	if !okk {
		return
	}
	stats, err := s.Store.Stats(claims.TenantID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "统计失败")
		return
	}
	ok(c, stats)
}

func (s *Server) ChannelCodes(c *gin.Context) {
	if _, okk := s.requireUser(c); !okk {
		return
	}
	ok(c, s.Registry.Codes())
}

func (s *Server) ListChannels(c *gin.Context) {
	claims, okk := s.requireUser(c)
	if !okk {
		return
	}
	list, err := s.Store.ListChannels(claims.TenantID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "查询失败")
		return
	}
	ok(c, list)
}

type channelReq struct {
	Code       string `json:"code" binding:"required"`
	Name       string `json:"name" binding:"required"`
	Status     int8   `json:"status"`
	FeeRateBps int    `json:"fee_rate_bps"`
	Remark     string `json:"remark"`
}

func (s *Server) CreateChannel(c *gin.Context) {
	claims, okk := s.requireUser(c)
	if !okk {
		return
	}
	var req channelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	if _, err := s.Registry.Get(req.Code); err != nil {
		fail(c, http.StatusBadRequest, "未知渠道类型")
		return
	}
	if req.Status == 0 {
		req.Status = 1
	}
	ch := &model.Channel{
		TenantID: claims.TenantID, Code: req.Code, Name: req.Name,
		Status: req.Status, FeeRateBps: req.FeeRateBps, Remark: req.Remark,
	}
	if err := s.Store.CreateChannel(c.Request.Context(), ch); err != nil {
		st, msg := storeErrStatus(err)
		fail(c, st, msg)
		return
	}
	ok(c, ch)
}

func (s *Server) UpdateChannel(c *gin.Context) {
	claims, okk := s.requireUser(c)
	if !okk {
		return
	}
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req channelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	if req.Status == 0 {
		req.Status = 1
	}
	if err := s.Store.UpdateChannel(c.Request.Context(), claims.TenantID, id, req.Name, req.Remark, req.Status, req.FeeRateBps); err != nil {
		fail(c, http.StatusInternalServerError, "更新失败")
		return
	}
	ok(c, gin.H{"id": id})
}

func (s *Server) DeleteChannel(c *gin.Context) {
	claims, okk := s.requireUser(c)
	if !okk {
		return
	}
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := s.Store.DeleteChannel(c.Request.Context(), claims.TenantID, id); err != nil {
		fail(c, http.StatusInternalServerError, "删除失败")
		return
	}
	ok(c, gin.H{"id": id})
}

func pageArgs(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	return page, size
}

func (s *Server) ListOrders(c *gin.Context) {
	claims, okk := s.requireUser(c)
	if !okk {
		return
	}
	page, size := pageArgs(c)
	list, total, err := s.Store.ListOrders(claims.TenantID, c.Query("status"), c.Query("biz_type"), page, size)
	if err != nil {
		fail(c, http.StatusInternalServerError, "查询失败")
		return
	}
	ok(c, gin.H{"list": list, "total": total})
}

func (s *Server) GetOrder(c *gin.Context) {
	claims, okk := s.requireUser(c)
	if !okk {
		return
	}
	o, err := s.Store.OrderByNo(claims.TenantID, c.Param("orderNo"))
	if err != nil {
		st, msg := storeErrStatus(err)
		fail(c, st, msg)
		return
	}
	ok(c, o)
}

func (s *Server) CloseOrder(c *gin.Context) {
	claims, okk := s.requireUser(c)
	if !okk {
		return
	}
	if err := s.Store.CloseOrder(c.Request.Context(), claims.TenantID, c.Param("orderNo")); err != nil {
		st, msg := storeErrStatus(err)
		fail(c, st, msg)
		return
	}
	ok(c, gin.H{"order_no": c.Param("orderNo"), "status": model.OrderClosed})
}

type refundReq struct {
	OrderNo     string `json:"order_no" binding:"required"`
	AmountCents int64  `json:"amount_cents" binding:"required"`
	Reason      string `json:"reason"`
}

func (s *Server) CreateRefund(c *gin.Context) {
	claims, okk := s.requireUser(c)
	if !okk {
		return
	}
	var req refundReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	r, err := s.refundFlow(c, claims.TenantID, req, claims.UserID)
	if err != nil {
		st, msg := storeErrStatus(err)
		fail(c, st, msg)
		return
	}
	ok(c, r)
}

// refundFlow 建退款单 → 调渠道退款 → mock 立即落终态（真渠道等回调）。
func (s *Server) refundFlow(c *gin.Context, tenantID uint64, req refundReq, operator uint64) (*model.Refund, error) {
	o, err := s.Store.OrderByNo(tenantID, req.OrderNo)
	if err != nil {
		return nil, err
	}
	r, err := s.Store.CreateRefund(c.Request.Context(), tenantID, req.OrderNo, req.AmountCents, req.Reason, operator)
	if err != nil {
		return nil, err
	}
	ch, err := s.Registry.Get(o.ChannelCode)
	if err != nil {
		// 渠道已不在注册表（不应发生）：退款单留 processing 等人工
		return r, nil
	}
	if err := ch.Refund(c.Request.Context(), o, r, ""); err != nil {
		return r, nil // 渠道侧失败：留 processing，补偿/回调续命
	}
	if o.ChannelCode == channel.MockCode {
		return s.Store.ApplyRefundResult(c.Request.Context(), tenantID, r.RefundNo, "MOCKRF"+r.RefundNo, true)
	}
	return r, nil
}

func (s *Server) ListRefunds(c *gin.Context) {
	claims, okk := s.requireUser(c)
	if !okk {
		return
	}
	page, size := pageArgs(c)
	list, total, err := s.Store.ListRefunds(claims.TenantID, c.Query("status"), page, size)
	if err != nil {
		fail(c, http.StatusInternalServerError, "查询失败")
		return
	}
	ok(c, gin.H{"list": list, "total": total})
}

func (s *Server) ListCallbackJobs(c *gin.Context) {
	claims, okk := s.requireUser(c)
	if !okk {
		return
	}
	list, err := s.Store.ListCallbackJobs(claims.TenantID, c.Query("status"), 100)
	if err != nil {
		fail(c, http.StatusInternalServerError, "查询失败")
		return
	}
	ok(c, list)
}

func (s *Server) ReplayCallbackJob(c *gin.Context) {
	claims, okk := s.requireUser(c)
	if !okk {
		return
	}
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := s.Store.ReviveCallbackJob(claims.TenantID, id); err != nil {
		fail(c, http.StatusInternalServerError, "重推失败")
		return
	}
	ok(c, gin.H{"id": id})
}

func (s *Server) ListNotifyLogs(c *gin.Context) {
	claims, okk := s.requireUser(c)
	if !okk {
		return
	}
	list, err := s.Store.ListNotifyLogs(claims.TenantID, 100)
	if err != nil {
		fail(c, http.StatusInternalServerError, "查询失败")
		return
	}
	ok(c, list)
}

// ---- 内部端点 ----

type internalOrderReq struct {
	TenantID    uint64 `json:"tenant_id"`
	BizType     string `json:"biz_type" binding:"required"`
	BizID       string `json:"biz_id" binding:"required"`
	Subject     string `json:"subject" binding:"required"`
	AmountCents int64  `json:"amount_cents" binding:"required"`
	// ExpireMinutes 可选，缺省用服务级 OrderTTL
	ExpireMinutes int `json:"expire_minutes"`
}

func (s *Server) InternalCreateOrder(c *gin.Context) {
	if !s.requireInternal(c) {
		return
	}
	var req internalOrderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	if req.TenantID == 0 {
		req.TenantID = 1
	}
	ttl := s.OrderTTL
	if req.ExpireMinutes > 0 {
		ttl = time.Duration(req.ExpireMinutes) * time.Minute
	}
	o, err := s.Store.CreateOrder(c.Request.Context(), req.TenantID, req.BizType, req.BizID, req.Subject, req.AmountCents, ttl)
	if err != nil {
		st, msg := storeErrStatus(err)
		fail(c, st, msg)
		return
	}
	ok(c, gin.H{
		"order_no":     o.OrderNo,
		"status":       o.Status,
		"amount_cents": o.AmountCents,
		"cashier_path": "/cashier/" + o.OrderToken,
		"expire_at":    o.ExpireAt,
	})
}

type internalRefundReq struct {
	TenantID    uint64 `json:"tenant_id"`
	OrderNo     string `json:"order_no" binding:"required"`
	AmountCents int64  `json:"amount_cents" binding:"required"`
	Reason      string `json:"reason"`
}

func (s *Server) InternalCreateRefund(c *gin.Context) {
	if !s.requireInternal(c) {
		return
	}
	var req internalRefundReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	if req.TenantID == 0 {
		req.TenantID = 1
	}
	r, err := s.refundFlow(c, req.TenantID, refundReq{OrderNo: req.OrderNo, AmountCents: req.AmountCents, Reason: req.Reason}, 0)
	if err != nil {
		st, msg := storeErrStatus(err)
		fail(c, st, msg)
		return
	}
	ok(c, r)
}

func (s *Server) InternalOrderByBiz(c *gin.Context) {
	if !s.requireInternal(c) {
		return
	}
	tenantID, _ := strconv.ParseUint(c.DefaultQuery("tenant_id", "1"), 10, 64)
	if tenantID == 0 {
		tenantID = 1
	}
	o, err := s.Store.OrderByBiz(tenantID, c.Query("biz_type"), c.Query("biz_id"))
	if err != nil {
		st, msg := storeErrStatus(err)
		fail(c, st, msg)
		return
	}
	ok(c, o)
}
