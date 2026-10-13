package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	aihandler "github.com/go-admin-kit/services/ai/internal/api/ai"
	"github.com/go-admin-kit/services/ai/internal/middleware"
	aiservice "github.com/go-admin-kit/services/ai/internal/service/ai"
	redisstore "github.com/go-admin-kit/services/shared/pkg/redis"
	sharedapi "github.com/go-admin-kit/services/shared/pkg/sharedapi"
	goredis "github.com/redis/go-redis/v9"
)

func TestAgentKernelRoutesAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := aihandler.NewHandler(func(context.Context) aihandler.StatusInfo { return aihandler.StatusInfo{} }, aiservice.ChatService{}, aiservice.KnowledgeService{}, aiservice.InsightService{})
	SetupRoutesWithDeps(
		router, sharedapi.Dependencies{}, handler,
		aihandler.NewAIModelHandler(nil), aihandler.NewAIUsageHandler(nil), aihandler.NewAIQuotaHandler(nil),
		aihandler.NewHermesHandler(nil), aihandler.NewHermesOfficeHandler(nil), aihandler.NewHermesProxyHandler(nil), aihandler.NewHermesUsageHandler(nil),
		func(*gin.Context) {},
	)
	want := map[string]bool{
		"GET /api/v1/ai/agent-kernel/tools":                false,
		"GET /api/v1/ai/agent-kernel/agents":               false,
		"POST /api/v1/ai/agent-kernel/agents":              false,
		"POST /api/v1/ai/agent-kernel/agents/:id/versions": false,
		"POST /api/v1/ai/agent-kernel/agents/:id/runs":     false,
		"GET /api/v1/ai/agent-kernel/runs/:id/events":      false,
	}
	for _, route := range router.Routes() {
		key := route.Method + " " + route.Path
		if _, exists := want[key]; exists {
			want[key] = true
		}
	}
	for route, found := range want {
		if !found {
			t.Errorf("route registration is missing: %s", route)
		}
	}
}

// 事件流的建连限流挂了什么阈值，用**同一份配置**验行为：第 MaxRequests+1 次必须 429。
// 复制一份字面量会让测试与真实配置脱钩，所以这里直接引用 routes.go 的变量。
func TestAgentEventStreamRateLimitConfigRejectsOverLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	oldClient := redisstore.Client
	client := goredis.NewClient(&goredis.Options{Addr: store.Addr()})
	redisstore.Client = client
	t.Cleanup(func() {
		_ = client.Close()
		redisstore.Client = oldClient
		store.Close()
	})

	router := gin.New()
	router.GET("/probe", middleware.RateLimit(agentEventStreamRateLimit), func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < agentEventStreamRateLimit.MaxRequests; i++ {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/probe", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("第 %d 次请求就被拒（%d），阈值比预期小", i+1, recorder.Code)
		}
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/probe", nil))
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("超出阈值后返回 %d，限流未生效", recorder.Code)
	}
}

// 加限流不能把鉴权顺序弄丢：事件流必须仍然要求登录态。
// 这条同时挡住"为了把限流提前到鉴权之前而把路由挪出 protected 组"的改法。
func TestAgentKernelEventStreamStillRequiresAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := aihandler.NewHandler(func(context.Context) aihandler.StatusInfo { return aihandler.StatusInfo{} }, aiservice.ChatService{}, aiservice.KnowledgeService{}, aiservice.InsightService{})
	SetupRoutesWithDeps(
		router, sharedapi.Dependencies{}, handler,
		aihandler.NewAIModelHandler(nil), aihandler.NewAIUsageHandler(nil), aihandler.NewAIQuotaHandler(nil),
		aihandler.NewHermesHandler(nil), aihandler.NewHermesOfficeHandler(nil), aihandler.NewHermesProxyHandler(nil), aihandler.NewHermesUsageHandler(nil),
		func(*gin.Context) {},
	)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/ai/agent-kernel/runs/1/events", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("未认证请求返回 %d，事件流路由必须仍受鉴权保护", recorder.Code)
	}
}

func TestHermesApprovalRouteDoesNotExposeUserCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := aihandler.NewHandler(func(context.Context) aihandler.StatusInfo {
		return aihandler.StatusInfo{}
	}, aiservice.ChatService{}, aiservice.KnowledgeService{}, aiservice.InsightService{})

	SetupRoutesWithDeps(
		router,
		sharedapi.Dependencies{},
		handler,
		aihandler.NewAIModelHandler(nil),
		aihandler.NewAIUsageHandler(nil),
		aihandler.NewAIQuotaHandler(nil),
		aihandler.NewHermesHandler(nil),
		aihandler.NewHermesOfficeHandler(nil),
		aihandler.NewHermesProxyHandler(nil),
		aihandler.NewHermesUsageHandler(nil),
		func(*gin.Context) {},
	)

	want := "POST /api/v1/ai/hermes/device-authorizations/approve"
	found := false
	for _, route := range router.Routes() {
		registered := route.Method + " " + route.Path
		if registered == want {
			found = true
		}
		if strings.Contains(route.Path, "device-authorizations/:user_code") {
			t.Fatalf("Hermes user code must not be part of a registered URL: %s", registered)
		}
	}
	if !found {
		t.Fatalf("route registration is missing: %s", want)
	}
}
