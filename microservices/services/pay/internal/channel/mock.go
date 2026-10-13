// mock 渠道：无外部依赖的全链路联调渠道。
// CreateTrade 返回本地收银台模拟页 URL；「支付成功/失败」由收银台
// mock 端点直接驱动订单状态机（不走 HTTP 回调自环，见 api/cashier.go）。
package channel

import (
	"context"
	"fmt"

	"github.com/go-admin-kit/services/pay/internal/model"
)

const MockCode = "mock"

type Mock struct{}

func (Mock) Code() string { return MockCode }

func (Mock) CreateTrade(_ context.Context, order *model.Order, _ string) (*TradeParams, error) {
	// 收银台前端识别 mock 渠道后展示「模拟支付」按钮，无需真实跳转
	return &TradeParams{RedirectURL: fmt.Sprintf("/cashier/%s?mock=1", order.OrderNo)}, nil
}

func (Mock) QueryTrade(_ context.Context, _ *model.Order, _ string) (*TradeStatus, error) {
	// mock 渠道无远端状态：查单恒未支付，终态只由模拟按钮驱动
	return &TradeStatus{Paid: false}, nil
}

func (Mock) Refund(_ context.Context, _ *model.Order, _ *model.Refund, _ string) error {
	// mock 退款立即成功（由 store 直接落终态）
	return nil
}
