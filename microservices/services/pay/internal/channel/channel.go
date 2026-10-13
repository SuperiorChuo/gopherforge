// Package channel 定义支付渠道抽象与注册表（设计文档 §4）。
// mock 渠道是联调/CI/演示专用渠道，是否注册由 PAY_DEMO_MODE 决定（生产默认不注册）：
// 注册表就是"本部署支持哪些渠道"的唯一事实源，未注册的渠道在建单、收银台
// 取单、拉起支付三处一律不可见、不可用，前端也据此决定渲染什么。
package channel

import (
	"context"
	"errors"
	"sort"

	"github.com/go-admin-kit/services/pay/internal/model"
)

var (
	ErrUnknownChannel = errors.New("未知支付渠道")
	// ErrNotSupported 渠道不支持该操作（如 mock 之外渠道 M1 未实现）
	ErrNotSupported = errors.New("渠道暂不支持该操作")
)

// TradeParams 拉起支付所需参数，按渠道类型三选一。
type TradeParams struct {
	// QRContent Native 扫码：二维码内容
	QRContent string `json:"qr_content,omitempty"`
	// RedirectURL 跳转式（支付宝 PC/WAP、mock 模拟页）
	RedirectURL string `json:"redirect_url,omitempty"`
	// JSAPIParams 公众号内拉起支付的参数包
	JSAPIParams map[string]string `json:"jsapi_params,omitempty"`
}

// NotifyResult 渠道异步通知的规范化结果。
type NotifyResult struct {
	OrderNo        string
	ChannelTradeNo string
	AmountCents    int64
	// Succeeded 渠道侧是否支付成功
	Succeeded bool
	// Ack 渠道要求的应答体（各渠道格式不同，adapter 自产）
	Ack AckFunc
}

// AckFunc 渠道回调应答（微信要 JSON、支付宝要纯文本 success）。
type AckFunc func(ok bool) (contentType string, body string)

// TradeStatus 主动查单结果（补偿 cron 用）。
type TradeStatus struct {
	Paid           bool
	ChannelTradeNo string
}

// Channel 支付渠道适配器。cfg 为解密后的渠道凭据 JSON（mock 恒空）。
type Channel interface {
	Code() string
	CreateTrade(ctx context.Context, order *model.Order, cfg string) (*TradeParams, error)
	QueryTrade(ctx context.Context, order *model.Order, cfg string) (*TradeStatus, error)
	Refund(ctx context.Context, order *model.Order, refund *model.Refund, cfg string) error
}

// Registry 渠道注册表（照 mp publisher Registry 先例）。
type Registry struct {
	channels map[string]Channel
}

func NewRegistry(chs ...Channel) *Registry {
	r := &Registry{channels: make(map[string]Channel, len(chs))}
	for _, ch := range chs {
		r.channels[ch.Code()] = ch
	}
	return r
}

func (r *Registry) Get(code string) (Channel, error) {
	ch, ok := r.channels[code]
	if !ok {
		return nil, ErrUnknownChannel
	}
	return ch, nil
}

// Has 渠道是否已注册（路由挂载、前端能力探测的判据）。
func (r *Registry) Has(code string) bool {
	_, ok := r.channels[code]
	return ok
}

// Codes 已注册渠道类型（渠道配置页下拉数据源）。空切片 = 本部署没有任何
// 可用渠道，调用方应如实告知而不是兜底成 mock。
func (r *Registry) Codes() []string {
	out := make([]string, 0, len(r.channels))
	for code := range r.channels {
		out = append(out, code)
	}
	sort.Strings(out)
	return out
}
