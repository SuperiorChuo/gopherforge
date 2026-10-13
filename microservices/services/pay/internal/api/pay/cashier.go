// 收银台端点：匿名访问，order_token 即凭据（随机 48 hex、终态后仅可读结果）。
// mock 模拟支付端点直接驱动状态机——不做 HTTP 回调自环，链路更短且便于单测。
package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/pay/internal/channel"
	"github.com/go-admin-kit/services/pay/internal/model"
	"github.com/go-admin-kit/services/pay/internal/store/pay"
)

// CashierGet 收银台取单：金额/标题/状态/可用渠道。
func (s *Server) CashierGet(c *gin.Context) {
	o, err := s.Store.OrderByToken(c.Param("token"))
	if err != nil {
		fail(c, http.StatusNotFound, "订单不存在")
		return
	}
	chs, err := s.Store.ListChannels(o.TenantID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "查询失败")
		return
	}
	enabled := make([]gin.H, 0, len(chs))
	for _, ch := range chs {
		if ch.Status != 1 {
			continue
		}
		if _, err := s.Registry.Get(ch.Code); err != nil {
			continue
		}
		enabled = append(enabled, gin.H{"code": ch.Code, "name": ch.Name})
	}
	ok(c, gin.H{
		"order_no":     o.OrderNo,
		"subject":      o.Subject,
		"amount_cents": o.AmountCents,
		"status":       o.Status,
		"channel_code": o.ChannelCode,
		"expire_at":    o.ExpireAt,
		"channels":     enabled,
		// mock_enabled 是前端渲染「模拟支付」按钮的唯一依据：不能靠 channel_code
		// 猜——历史订单的 channel_code 可能还留着 mock，而渠道早已下线。
		"mock_enabled": s.Registry.Has(channel.MockCode),
	})
}

type cashierPayReq struct {
	ChannelCode string `json:"channel_code" binding:"required"`
}

// CashierPay 选渠道拉起支付：waiting/paying → paying + 渠道 CreateTrade。
func (s *Server) CashierPay(c *gin.Context) {
	o, err := s.Store.OrderByToken(c.Param("token"))
	if err != nil {
		fail(c, http.StatusNotFound, "订单不存在")
		return
	}
	var req cashierPayReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	cfg, err := s.Store.EnabledChannel(o.TenantID, req.ChannelCode)
	if err != nil {
		st, msg := storeErrStatus(err)
		fail(c, st, msg)
		return
	}
	ch, err := s.Registry.Get(req.ChannelCode)
	if err != nil {
		fail(c, http.StatusBadRequest, "未知渠道")
		return
	}
	o, err = s.Store.MarkPaying(c.Request.Context(), o.TenantID, o.OrderNo, cfg.ID, cfg.Code)
	if err != nil {
		st, msg := storeErrStatus(err)
		fail(c, st, msg)
		return
	}
	params, err := ch.CreateTrade(c.Request.Context(), o, "")
	if err != nil {
		fail(c, http.StatusBadGateway, "渠道拉起失败")
		return
	}
	ok(c, gin.H{"order_no": o.OrderNo, "status": o.Status, "channel_code": cfg.Code, "trade": params})
}

type cashierMockReq struct {
	// Succeed true=模拟支付成功 false=模拟失败（订单留 paying，可关单/过期）
	Succeed bool `json:"succeed"`
}

// CashierMock mock 渠道专用：模拟渠道侧结果。仅当订单已选 mock 渠道时有效。
// 路由本身只在 mock 渠道注册时挂载（见 RegisterRoutes），这里再挡一道：
// 万一将来有人无条件挂了路由，也不能让匿名调用把订单改成已支付。
func (s *Server) CashierMock(c *gin.Context) {
	if !s.Registry.Has(channel.MockCode) {
		fail(c, http.StatusNotFound, "mock 渠道未启用")
		return
	}
	o, err := s.Store.OrderByToken(c.Param("token"))
	if err != nil {
		fail(c, http.StatusNotFound, "订单不存在")
		return
	}
	if o.ChannelCode != channel.MockCode {
		fail(c, http.StatusBadRequest, "非 mock 渠道订单")
		return
	}
	var req cashierMockReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	// 落回调日志（mock 也走同一审计通道）
	logResult := func(result string, signOK bool) {
		_ = s.Store.AddNotifyLog(&model.NotifyLog{
			TenantID: o.TenantID, Kind: "trade", ChannelCode: channel.MockCode,
			OrderNo: o.OrderNo, Body: `{"mock":true,"succeed":` + boolStr(req.Succeed) + `}`,
			SignOK: signOK, Result: result,
		})
	}
	if !req.Succeed {
		logResult("rejected", true)
		ok(c, gin.H{"order_no": o.OrderNo, "status": o.Status, "mock_result": "failed"})
		return
	}
	updated, err := s.Store.ApplyPaid(c.Request.Context(), o.TenantID, o.OrderNo, "MOCK"+o.OrderNo, o.AmountCents)
	switch {
	case errors.Is(err, store.ErrDuplicateApplied):
		logResult("duplicate", true)
	case err != nil:
		logResult("rejected", true)
		st, msg := storeErrStatus(err)
		fail(c, st, msg)
		return
	default:
		logResult("applied", true)
	}
	ok(c, gin.H{"order_no": updated.OrderNo, "status": updated.Status, "paid_at": updated.PaidAt})
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
