package api

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/pay/internal/channel"
)

func mountedPaths(t *testing.T, registry *channel.Registry) map[string]bool {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	(&Server{Registry: registry}).RegisterRoutes(r)
	out := map[string]bool{}
	for _, ri := range r.Routes() {
		out[ri.Method+" "+ri.Path] = true
	}
	return out
}

const mockRoute = "POST /api/v1/pay/cashier/:token/mock"

// 收银台是匿名页面，模拟支付端点能把订单直接标成已支付。mock 渠道未注册
// （生产默认）时，这条路由必须根本不存在——不能只靠前端不渲染按钮。
func TestMockCashierRouteIsNotMountedWithoutMockChannel(t *testing.T) {
	paths := mountedPaths(t, channel.NewRegistry())
	if paths[mockRoute] {
		t.Fatal("mock cashier route mounted without the mock channel registered")
	}
	// 真实收银台端点不受影响，付款人照常取单/拉起支付。
	for _, want := range []string{"GET /api/v1/pay/cashier/:token", "POST /api/v1/pay/cashier/:token/pay"} {
		if !paths[want] {
			t.Fatalf("route %q missing; real cashier endpoints must stay mounted", want)
		}
	}
}

// 演示模式下（PAY_DEMO_MODE=true → 注册 mock 渠道）才挂载。
func TestMockCashierRouteIsMountedWithMockChannel(t *testing.T) {
	if !mountedPaths(t, channel.NewRegistry(channel.Mock{}))[mockRoute] {
		t.Fatal("mock cashier route missing while the mock channel is registered")
	}
}
